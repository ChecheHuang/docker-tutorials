# 23. 多平台 build：amd64 與 arm64

> 範圍：amd64 vs arm64（Apple Silicon、Graviton）、`docker buildx create --use`、`--platform linux/amd64,linux/arm64`、QEMU（`tonistiigi/binfmt`）vs 交叉編譯（`$BUILDPLATFORM`／`$TARGETOS`／`$TARGETARCH`）、manifest list、`--push` vs `--load` 的限制；附 Rust 與 Node 的多平台做法

> 🧸 想先用故事直覺理解這章？看 [STORY.md](./STORY.md)（小學生都懂的比喻版）。

## 為什麼需要這個

container 共用 host 的 kernel，**也共用 host 的 CPU**。image 裡的執行檔是某一種 CPU 的機器碼，換一種 CPU 就看不懂：

```
$ docker run --rm ghcr.io/chechehuang/shorty:1.2.0
WARNING: The requested image's platform (linux/arm64) does not match the detected host platform (linux/amd64/v3) and no specific platform was requested
exec /shorty: exec format error
```

今天這兩種 CPU 都很常見：

| 架構 | Docker 名稱 | 常見在哪裡 |
|------|------|------|
| x86-64 | `linux/amd64` | 大部分雲端 VM、CI runner、Intel／AMD 筆電 |
| ARM 64 | `linux/arm64` | Apple Silicon Mac、AWS Graviton、GCP Axion、Azure Cobalt、Raspberry Pi 4/5 |

情境：你在 M 系列 Mac 上 `docker build` + `docker push`，推上去的是 arm64；正式機是 x86 → 跑不起來。反過來，CI 在 amd64 build，同事 Mac 上就只能用模擬跑，又慢又可能有怪問題。

解法：**一個 tag 同時放兩種架構**，`docker pull` 時自動選對的那一份。

## 核心概念

### 1. manifest list（image index）

多平台 image 其實是「一張目錄 + 好幾份 image」：

```
ghcr.io/chechehuang/shorty:1.2.0   ← index（manifest list），有自己的 digest
├── linux/amd64 → manifest sha256:aaa... → layers
├── linux/arm64 → manifest sha256:bbb... → layers
└── unknown/unknown → attestation（provenance / SBOM，buildx 預設會附）
```

`docker pull` 先拿到 index，再依照本機的平台挑對應的 manifest。第 21 章 pin digest 時要 pin **index 的 digest**，就是這個原因。

### 2. buildx 與 builder

`docker buildx` 是 BuildKit 的前端（第 11 章）。多平台 build 需要一個支援它的 builder：

| builder driver | 多平台 | 說明 |
|------|------|------|
| `docker`（預設） | 只有開 containerd image store 時才行 | 直接用 Docker Engine 內建的 BuildKit |
| `docker-container` | 可以 | 在一個 container 裡跑獨立的 BuildKit，`buildx create` 預設就是它 |

### 3. 兩種做法：QEMU 模擬 vs 交叉編譯

**QEMU 模擬**：kernel 的 binfmt_misc 遇到 arm64 執行檔時，交給 QEMU 一條一條翻譯執行。Dockerfile 完全不用改，但**所有 `RUN` 都在模擬器裡跑**，編譯這種 CPU 密集的工作會慢 5–20 倍。

```bash
docker run --privileged --rm tonistiigi/binfmt --install all   # Linux 上註冊 QEMU
```

Docker Desktop（Mac／Windows）已經內建，不用跑這行。

**交叉編譯**：編譯器跑在 build 機器的原生架構，只是「輸出」另一種架構的執行檔。Go 天生支援，速度跟原生一樣。

BuildKit 自動提供的 build arg：

| ARG | 例子 | 意思 |
|------|------|------|
| `BUILDPLATFORM` | `linux/amd64` | 正在執行 build 的機器 |
| `TARGETPLATFORM` | `linux/arm64` | 這一輪要產出的平台 |
| `TARGETOS` | `linux` | 目標 OS |
| `TARGETARCH` | `arm64` | 目標架構 |
| `TARGETVARIANT` | `v7`（arm/v7 才有） | 架構變體 |
| `BUILDOS`／`BUILDARCH` | `linux`／`amd64` | build 機器的 OS／架構 |

`FROM` 可以直接用它們；stage 裡面要用就得先 `ARG TARGETARCH` 宣告。

### 4. app/Dockerfile 怎麼做交叉編譯

```dockerfile
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build   # ① 編譯器用原生架構
...
ARG TARGETOS
ARG TARGETARCH                                               # ② 拿到目標架構
RUN ... CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build ... -o /out/shorty .                            # ③ Go 交叉編譯

FROM gcr.io/distroless/static-debian12:nonroot               # ④ 沒寫 --platform = 目標平台
COPY --from=build /out/shorty /shorty
```

`--platform linux/amd64,linux/arm64` 時，BuildKit 跑兩輪：

| | build stage 的 image | 執行 go build 的 CPU | 輸出 | runtime stage |
|------|------|------|------|------|
| 第 1 輪 | `golang` amd64 | 原生 | amd64 shorty | distroless amd64 |
| 第 2 輪 | `golang` amd64（同一份！） | 原生 | arm64 shorty | distroless arm64 |

因為兩輪的 build stage 前半段完全一樣（`go mod download` 那層），還能共用快取。runtime stage 只有 `COPY`，不執行任何東西，所以就算是 arm64 也不需要 QEMU。

關鍵是 `CGO_ENABLED=0`：一旦需要 cgo，就得準備目標架構的 C 編譯器和 libc，難度跟 Rust 一樣（見下面）。這種情況可以用 `tonistiigi/xx` 這類 helper image。

### 5. `--push` vs `--load`

多平台 image 要放到哪裡？

| 輸出 | 指令 | 多平台 |
|------|------|------|
| 推到 registry | `--push` | 可以，最常用 |
| 載入本機 Docker（classic image store） | `--load` | **不行**，只能單一平台 |
| 載入本機 Docker（containerd image store） | `--load` | 可以 |
| 只留在 build cache | 都不寫 | 可以，但 `docker images` 看不到 |

classic image store（overlay2）一個 tag 只能存一個平台的 image，所以會報錯：

```
ERROR: docker exporter does not currently support exporting manifest lists
```

containerd image store 能存 index。Docker Desktop 新版預設開啟（Settings → General → *Use containerd for pulling and storing images*），Docker Engine 29 起新安裝也預設使用。舊的 Engine 可以在 `/etc/docker/daemon.json` 加上：

```json
{ "features": { "containerd-snapshotter": true } }
```

確認目前用哪一種：

```bash
docker info -f '{{ .DriverStatus }}'
# [[driver-type io.containerd.snapshotter.v1]]   ← containerd image store
```

## shorty 在這章

shorty 的最終版 `app/Dockerfile` 就是交叉編譯版，第 21、22 章的版本再加上 `--platform=$BUILDPLATFORM` 與 `GOOS`／`GOARCH`。本章目錄放對照組：

| 檔案 | 用途 |
|------|------|
| `Dockerfile.qemu` | shorty 的「全靠 QEMU」版，拿來跟 `app/Dockerfile` 比 build 時間 |
| `Dockerfile.rust` + `.dockerignore` | `hello-rs` 交叉編譯：`rustup target add` + Debian cross gcc |
| `Dockerfile.node` + `.dockerignore` | `hello-node`：純 JS 依賴在原生架構裝，runtime 用目標平台的 node |

Rust／Node 的 build context 是第 08 章的範例 `tutorials/08-multi-stage/rust`、`tutorials/08-multi-stage/node`。

## 動手做

### 1. 看看你現在有哪些 builder 與平台

```bash
docker buildx ls
```

範例輸出：

```
NAME/NODE        DRIVER/ENDPOINT   STATUS    BUILDKIT   PLATFORMS
default*         docker
 \_ default       \_ default       running   v0.25.1    linux/amd64 (+3), linux/arm64, linux/386
```

`PLATFORMS` 裡有 `linux/arm64` 表示 QEMU 已經註冊好。沒有的話（Linux 主機）：

```bash
docker run --privileged --rm tonistiigi/binfmt --install all
docker run --rm --platform linux/arm64 alpine uname -m
# aarch64
```

### 2. 建立一個 docker-container builder

```bash
docker buildx create --name multi --driver docker-container --use --bootstrap
docker buildx ls
```

> 已經開了 containerd image store 的話，預設 `docker` builder 就能做多平台，這步可以跳過。

### 3. 多平台 build shorty

```bash
# 只 build，不輸出（驗證兩個平台都編得過）
docker buildx build --platform linux/amd64,linux/arm64 -t shorty:multi app

# containerd image store：可以直接載入本機
docker buildx build --platform linux/amd64,linux/arm64 -t shorty:multi --load app

# 正式做法：直接推到 registry
docker buildx build --platform linux/amd64,linux/arm64 \
  --build-arg VERSION=1.3.0 \
  -t ghcr.io/<你的帳號小寫>/shorty:1.3.0 --push app
```

classic image store 下想在本機試某一個平台，就單一平台 `--load`：

```bash
docker buildx build --platform linux/arm64 -t shorty:arm64 --load app
docker image inspect --format '{{.Os}}/{{.Architecture}}' shorty:arm64
# linux/arm64
docker run --rm -p 8080:8080 shorty:arm64     # 在 amd64 上透過 QEMU 跑
```

### 4. 看 manifest list

```bash
docker buildx imagetools inspect ghcr.io/<你的帳號小寫>/shorty:1.3.0
```

範例輸出：

```
Name:      ghcr.io/chechehuang/shorty:1.3.0
MediaType: application/vnd.oci.image.index.v1+json
Digest:    sha256:3c4d...（index digest）

Manifests:
  Name:        ghcr.io/chechehuang/shorty:1.3.0@sha256:aaa1...
  MediaType:   application/vnd.oci.image.manifest.v1+json
  Platform:    linux/amd64

  Name:        ghcr.io/chechehuang/shorty:1.3.0@sha256:bbb2...
  MediaType:   application/vnd.oci.image.manifest.v1+json
  Platform:    linux/arm64

  Name:        ghcr.io/chechehuang/shorty:1.3.0@sha256:ccc3...
  MediaType:   application/vnd.oci.image.manifest.v1+json
  Platform:    unknown/unknown
  Annotations:
    vnd.docker.reference.type: attestation-manifest
```

### 5. 比較：交叉編譯 vs QEMU

```bash
docker buildx build --no-cache --platform linux/amd64,linux/arm64 -t shorty:cross app
docker buildx build --no-cache --platform linux/amd64,linux/arm64 \
  -f tutorials/23-multi-platform/Dockerfile.qemu -t shorty:qemu app
```

範例結果（amd64 筆電，數字會依機器而異）：

| 版本 | 時間 |
|------|------|
| `app/Dockerfile`（交叉編譯） | ~40 秒 |
| `Dockerfile.qemu` | ~4 分鐘（幾乎都花在 arm64 的 `go build`） |

加 `--progress=plain` 可以看到 QEMU 版的 `[linux/arm64 build 6/6] RUN ... go build` 那一步特別慢。

### 6. Rust 與 Node

```bash
docker buildx build --platform linux/amd64,linux/arm64 \
  -f tutorials/23-multi-platform/Dockerfile.rust -t hello-rs:multi \
  tutorials/08-multi-stage/rust

docker buildx build --platform linux/amd64,linux/arm64 \
  -f tutorials/23-multi-platform/Dockerfile.node -t hello-node:multi \
  tutorials/08-multi-stage/node
```

## 對照 Node／TypeScript（以及 Rust）

三種語言的多平台策略：

| | Go（shorty） | Rust（hello-rs） | Node（hello-node） |
|------|------|------|------|
| 產物 | 單一靜態執行檔 | 執行檔（gnu 需 glibc；musl 可全靜態） | JS 原始碼 + node 執行檔 |
| 交叉編譯 | `GOARCH=arm64`，零設定 | `rustup target add` + 目標 linker／libc | 不用編譯 |
| 架構相關的部分 | 只有執行檔 | 執行檔、`-sys` crate 的 C 程式碼 | `node` 本身、native module |
| 推薦做法 | `$BUILDPLATFORM` 交叉編譯 | `$BUILDPLATFORM` + cross gcc／cargo-zigbuild | 純 JS：原生裝依賴；有 native module：QEMU |
| QEMU 代價 | 慢但能用 | 非常慢（Rust 編譯本來就重） | `npm ci` 慢一點，通常可接受 |

**Rust 的選擇：**

1. **Debian cross gcc**（本章 `Dockerfile.rust`）：`gcc-aarch64-linux-gnu` + `libc6-dev-arm64-cross`，設 `CARGO_TARGET_AARCH64_UNKNOWN_LINUX_GNU_LINKER`。runtime 用 `distroless/cc-debian12`，builder 也用 bookworm，glibc 版本才對得上。
2. **cargo-zigbuild**：用 zig 當 linker，一套工具搞定所有 target，還能指定最低 glibc 版本（例如 `--target aarch64-unknown-linux-gnu.2.17`）。
3. **musl 靜態**：`x86_64-unknown-linux-musl` / `aarch64-unknown-linux-musl`，可以放進 `distroless/static` 或 `scratch`。
4. **QEMU**：拿掉 `--platform=$BUILDPLATFORM`，Dockerfile 最簡單，但大專案可能一小時起跳。

**Node 的重點：** 直譯語言本身沒有「編譯成哪個架構」的問題，`FROM node:24-slim` 會自動選對架構。真正的坑是 native module：`bcrypt`、`sharp`、`better-sqlite3` 會在 `npm install` 時下載或編譯**當下架構**的 `.node` 檔。所以：

- 只有純 JS 依賴 → 可以在 `$BUILDPLATFORM` 跑 `npm ci`（快）。
- 有 native module → `npm ci` 必須在目標平台跑（拿掉 `--platform=$BUILDPLATFORM`，走 QEMU）。
- 永遠不要把本機的 `node_modules` COPY 進 image（`Dockerfile.node.dockerignore` 已排除）——那是你 Windows／Mac 的版本。

## 常見陷阱

1. **`exec format error`**：image 架構跟 host 不合。先用 `docker image inspect --format '{{.Os}}/{{.Architecture}}'` 確認。
2. **`--load` 多平台失敗**：classic image store 不支援 manifest list。改用 `--push`、單一平台 `--load`，或打開 containerd image store。
3. **`FROM --platform=$BUILDPLATFORM` 寫在 runtime stage**：runtime 會變成 build 機器的架構，arm64 的 image 裡裝著 amd64 的 distroless，跑起來一樣 `exec format error`。只有 build stage 要加。
4. **忘了 `ARG TARGETARCH`**：stage 裡沒宣告，`$TARGETARCH` 是空字串，`GOARCH=` 等於沒設，兩個平台都編出 build 機器的架構——而且不會報錯。
5. **cgo 偷偷開著**：`CGO_ENABLED` 沒設時，交叉編譯 Go 預設會關 cgo，但原生編譯在找得到 C 編譯器時會開；兩個平台行為不同。明確寫 `CGO_ENABLED=0`。
6. **pin 到單一平台的 digest**：`FROM image@sha256:` 寫成 amd64 那份的 digest，arm64 build 就會拿到 amd64 的 base。pin index digest。
7. **QEMU 下的怪問題**：少數程式（JIT、某些 syscall）在模擬下會 segfault 或特別慢。正式 arm64 環境上測一次最保險，CI 可以用原生 arm64 runner（GitHub 有 `ubuntu-24.04-arm`）。
8. **`unknown/unknown` 平台**：那是 buildx 預設附上的 provenance attestation，不是 bug。不想要可以 `--provenance=false`。

## 練習

1. 在本機 build `shorty:arm64` 和 `shorty:amd64` 兩個單平台 image，用 `docker image inspect` 確認架構，各自 `docker run` 後打 `/version`。
2. 故意把 `app/Dockerfile` 複製一份、刪掉 `ARG TARGETARCH`，build 多平台後用 `imagetools inspect` 和 `docker run --platform linux/arm64` 找出問題。
3. 把 `Dockerfile.rust` 改成 musl target + `gcr.io/distroless/static-debian12:nonroot`，比較 image 大小。
4. 在 `hello-node` 加入 `bcrypt`，分別用目前的 `Dockerfile.node` 和「拿掉 deps stage 的 `--platform`」的版本 build arm64，跑起來看差在哪。
5. 用 `time` 量 `Dockerfile.rust` 交叉編譯版和 QEMU 版（自己改一份）的 build 時間。

## 下一章

手動下 `buildx --push` 還是要記得 tag、登入、平台參數。第 24 章 [github-actions](../24-github-actions) 把第 21–23 章全部自動化：push 到 main 或打 tag，就自動測試、掃描、build 多平台 image 推上 GHCR。
