# 09. Base image 怎麼選：debian-slim／alpine／distroless／scratch

> 範圍：runtime base image 比較（debian-slim／alpine／distroless／scratch）、glibc vs musl、`CGO_ENABLED=0` 與靜態連結、CA 憑證、時區資料（tzdata）、`-trimpath -ldflags "-s -w"`、distroless 怎麼 debug；Rust 與 Node 各自適合的 base image

> 🧸 想先用故事直覺理解這章？看 [STORY.md](./STORY.md)（小學生都懂的比喻版）。

## 為什麼需要這個

第 08 章把 runtime stage 隨手寫成 `alpine:3.22`，還照抄了一個 `CGO_ENABLED=0`。這兩個決定其實綁在一起，背後藏著三個上線後才會爆的問題：

- 把 build stage 換成 Debian 版的 `golang:1.26`、拿掉 `CGO_ENABLED=0`，image 照樣 build 得起來，一跑就出現 `exec /shorty: no such file or directory` —— 檔案明明就在那裡。
- 改用更小的 `scratch`，連雲端資料庫時噴 `x509: certificate signed by unknown authority`。
- 程式裡 `time.LoadLocation("Asia/Taipei")`，本機正常，container 裡報 `unknown time zone`。

base image 決定了 runtime 裡**有什麼、沒有什麼**。選之前要知道你的程式需要什麼。

## 核心概念

### runtime 裡可能有的東西

- **libc**（glibc 或 musl）：動態連結的程式需要它 —— CGO 開著的 Go、預設的 Rust、Node.js 本體。
- **shell、套件管理器**：`docker exec -it sh` 進去 debug、shell form 的 `CMD`、`RUN apt-get install`。
- **CA 憑證**：任何 HTTPS／TLS 連線。**tzdata**：用到 `Asia/Taipei` 這類時區名稱。**`/etc/passwd`**：用名字指定 `USER`（第 21 章）。

東西越多越好 debug；越少，image 越小、能被利用的漏洞越少。

### 四種候選

| base | 大小（範例） | libc | shell | 套件管理 | CA／tzdata | 適合 |
|------|------|------|------|------|------|------|
| `debian:trixie-slim` | ~78 MB | glibc | bash | apt | 要自己裝 | 需要 glibc 和完整工具、debug 優先 |
| `alpine:3.22` | ~8 MB | **musl** | busybox sh | apk | 要自己裝 | 想要小、又想要 shell |
| `gcr.io/distroless/static-debian12` | ~2 MB | **沒有** | 沒有 | 沒有 | **內建** | 靜態執行檔（Go `CGO_ENABLED=0`、Rust musl） |
| `scratch` | 0 | 沒有 | 沒有 | 沒有 | 沒有 | 靜態執行檔，而且你願意自己搬 CA／tzdata |

distroless 還有其他口味：`base-debian12`（多了 glibc、OpenSSL）、`cc-debian12`（再多 libgcc、libstdc++，給 Rust／C++ 用）、`nodejs24-debian12`（Node runtime）。

### glibc vs musl，以及 `CGO_ENABLED=0` 在防什麼

Go 預設會用 **cgo** 呼叫系統 C 函式庫處理部分功能（`net` 的 DNS 解析、`os/user`）。cgo 開著時，編出來的是**動態連結**的執行檔，執行時要去找系統的 libc 和動態載入器（例如 `/lib64/ld-linux-x86-64.so.2`）。

| build stage | cgo 預設 | 結果 |
|------|------|------|
| `golang:1.26`（Debian，有 gcc） | **開** | 動態連結 glibc |
| `golang:1.26-alpine`（沒有 gcc） | 找不到 C 編譯器就自動關 | 靜態 |
| 任何一個 + `CGO_ENABLED=0` | 關 | **純 Go 靜態執行檔** |

動態連結 glibc 的執行檔丟進 alpine（只有 musl）或 scratch（什麼都沒有），kernel 找不到它要求的動態載入器，就回報 `no such file or directory` —— 說的不是你的執行檔，是那個載入器。

`CGO_ENABLED=0` 讓 Go 改用純 Go 實作的 DNS resolver 和 user 查詢，編出不依賴任何 libc 的執行檔。shorty 只用標準庫 + pgx + go-redis，全是純 Go，關掉 cgo 沒有任何損失。**寫明 `CGO_ENABLED=0`，不要依賴「alpine 剛好沒有 gcc」這種巧合。**

### CA 憑證與時區

- **CA 憑證**：Go 在 Linux 上會讀 `/etc/ssl/certs/ca-certificates.crt` 等路徑來驗證 TLS。scratch 沒有這個檔案。shorty 在 compose 裡連的是同網路、不加密的 Postgres／Redis，暫時不需要；但只要換成雲端資料庫（`sslmode=require`）或呼叫外部 API，就一定要有。
- **tzdata**：Go 會讀 `/usr/share/zoneinfo`。shorty 的 log 用 UTC，不需要；但很多服務需要。純 Go 還有兩個替代方案：build 時加 `-tags timetzdata`，或在程式裡 `import _ "time/tzdata"`，把時區資料（約 450 KB）直接編進執行檔。

distroless/static 兩樣都內建；alpine、debian-slim 要 `apk add` / `apt-get install`；scratch 要從 build stage `COPY` 過去。

### `-trimpath -ldflags "-s -w"`：順手把執行檔變小

這章的 build 指令多了兩個參數：

- `-ldflags "-s -w"`：拿掉符號表（`-s`）和 DWARF 除錯資訊（`-w`），shorty 從 ~18 MB 變成 ~12.5 MB。panic 的 stack trace 仍然有函式名稱和行號，只是不能再用 delve 對 image 裡的執行檔 debug。
- `-trimpath`：把執行檔裡記錄的本機路徑（`/src/...`、`/go/pkg/mod/...`）拿掉，讓 build 結果跟在哪台機器、哪個目錄 build 無關。

### distroless 沒有 shell，怎麼 debug？

`docker run --entrypoint sh shorty:ch09` 會失敗：`exec: "sh": executable file not found in $PATH`。這是 feature：攻擊者拿到 RCE 也沒有 shell 可用。要 debug 時，每個 distroless image 都有 **`:debug` tag**（多了 busybox shell），暫時把 `FROM` 換成 `gcr.io/distroless/static-debian12:debug` 就能進去看；或用 alpine 版在本機重現。`HEALTHCHECK` 沒有 curl 的問題留到第 16 章。

## shorty 在這章的演進

本章選定 **distroless/static** 當 shorty 的 runtime，[`Dockerfile`](./Dockerfile)：

```dockerfile
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/shorty .

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/shorty /shorty
EXPOSE 8080
ENTRYPOINT ["/shorty"]
```

跟第 08 章的差異：

```diff
-RUN CGO_ENABLED=0 go build -o /out/shorty .
+RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/shorty .
 
-FROM alpine:3.22
-COPY --from=build /out/shorty /usr/local/bin/shorty
+FROM gcr.io/distroless/static-debian12
+COPY --from=build /out/shorty /shorty
 EXPOSE 8080
-CMD ["shorty"]
+ENTRYPOINT ["/shorty"]
```

改用 `ENTRYPOINT` 和絕對路徑，是因為 distroless 沒有 shell、image 本身也沒有預設指令，寫死「啟動就是 `/shorty`」最直接。`ENTRYPOINT` 和 `CMD` 的分工第 16 章細講。

其他三個候選放在同目錄，build stage 完全相同，只換 runtime：

| 檔案 | runtime base | shorty image（範例） |
|------|------|------|
| [`Dockerfile.debian-slim`](./Dockerfile.debian-slim) | `debian:trixie-slim` + `apt-get install ca-certificates tzdata` | ~95 MB |
| [`Dockerfile.alpine`](./Dockerfile.alpine) | `alpine:3.22` + `apk add ca-certificates tzdata` | ~23 MB |
| **[`Dockerfile`](./Dockerfile)** | **`gcr.io/distroless/static-debian12`** | **~15 MB** |
| [`Dockerfile.scratch`](./Dockerfile.scratch) | `scratch` + 從 build stage `COPY` CA 和 zoneinfo | ~14 MB |

**為什麼選 distroless 而不是 scratch？** 只差 1～2 MB，distroless 卻多了：Google 維護、會更新的 CA 憑證和 tzdata、有 `nonroot` 使用者的 `/etc/passwd`（第 21 章用 `:nonroot` tag），以及隨時可以換成 `:debug` 的退路。scratch 每一樣都要自己搬、自己更新。

**為什麼不選 alpine？** 多 8 MB 換來一個 shell 和 apk —— debug 方便，但上線時多出來的工具也是攻擊面，掃描器（第 21 章）會掃出更多要處理的套件。alpine 很適合「想要小、但需要 shell」的情境。

## Rust 與 Node 各自適合的 base image

### Rust：先決定 glibc 還是 musl

| 做法 | runtime base | 大小（範例） | 注意 |
|------|------|------|------|
| 預設（glibc 動態連結） | `distroless/cc-debian12` | ~25 MB | builder 與 runtime 要同一代 Debian（第 08 章的做法） |
| 預設（glibc） | `debian:bookworm-slim` | ~80 MB | 需要 apt 裝東西、要 shell 時 |
| musl 靜態連結 | `scratch` 或 `distroless/static-debian12` | ~3 MB | 見 [`Dockerfile.rust-scratch`](./Dockerfile.rust-scratch) |

`rust:1.95-alpine` 的預設 target 就是 `*-linux-musl`，而且預設靜態連結，所以 `cargo build --release` 出來的執行檔可以直接丟進 scratch。musl 的取捨：

- 內建的記憶體配置器在多執行緒高負載時比 glibc 慢，吃重的服務常換成 mimalloc／jemalloc。
- 依賴 OpenSSL 的 crate（`openssl-sys`）在 musl 上很麻煩，改用 rustls。
- hello-rs 只有 axum／tokio，沒有這些問題。

### Node：一定要有 runtime

| base | 大小（範例） | libc | 注意 |
|------|------|------|------|
| `node:24-slim` | ~230 MB | glibc | **預設推薦**，相容性最好（第 08 章的做法） |
| `node:24-alpine` | ~165 MB | musl | 小一些；Node 官方把 musl 版列為 experimental，native addon 要確認有 musl 版本。見 [`Dockerfile.node-alpine`](./Dockerfile.node-alpine) |
| `gcr.io/distroless/nodejs24-debian12` | ~170 MB | glibc | 沒有 npm、沒有 shell，`ENTRYPOINT` 已經是 node。見 [`Dockerfile.node-distroless`](./Dockerfile.node-distroless) |

Node 不管怎麼選都是一百多 MB 起跳，因為 runtime 本身就這麼大。選擇的重點不是大小，而是**相容性**（glibc 最穩）和**攻擊面**（distroless 最少）。

## 動手做

以下指令都在 repo 根目錄執行。

### 1. build 四個 shorty 版本

```bash
docker build -f tutorials/09-base-images/Dockerfile             -t shorty:ch09         app
docker build -f tutorials/09-base-images/Dockerfile.debian-slim -t shorty:ch09-slim    app
docker build -f tutorials/09-base-images/Dockerfile.alpine      -t shorty:ch09-alpine  app
docker build -f tutorials/09-base-images/Dockerfile.scratch     -t shorty:ch09-scratch app

docker images shorty
```

範例輸出：

```
REPOSITORY   TAG            IMAGE ID       CREATED          SIZE
shorty       ch09-scratch   7c1d2e3f4a5b   5 seconds ago    14.1MB
shorty       ch09-alpine    6b0c1d2e3f4a   10 seconds ago   23.2MB
shorty       ch09-slim      5a9b0c1d2e3f   20 seconds ago   95.4MB
shorty       ch09           4f8a9b0c1d2e   30 seconds ago   14.6MB
shorty       ch08           9b7e4a2c1f30   1 hour ago       26.3MB
```

四個都跑跑看：

```bash
docker run --rm -d -p 8080:8080 --name s shorty:ch09
curl localhost:8080/healthz
docker stop s
```

範例輸出：`{"status":"ok"}`

### 2. 哪些有 shell？

```bash
docker run --rm --entrypoint sh shorty:ch09-alpine -c 'echo hi from alpine'
docker run --rm --entrypoint sh shorty:ch09-slim   -c 'echo hi from debian'
docker run --rm --entrypoint sh shorty:ch09                       # distroless：失敗
docker run --rm --entrypoint sh shorty:ch09-scratch               # scratch：失敗
```

### 3. 重現 `no such file or directory`

用 `-f-` 從 stdin 餵一份「錯誤示範」的 Dockerfile：Debian 版 Go、沒有 `CGO_ENABLED=0`、runtime 是 alpine。

```bash
docker build -t shorty:ch09-broken -f- app <<'EOF'
FROM golang:1.26 AS build
WORKDIR /src
COPY . .
RUN go build -o /out/shorty .
FROM alpine:3.22
COPY --from=build /out/shorty /shorty
CMD ["/shorty"]
EOF

docker run --rm shorty:ch09-broken
```

範例輸出：

```
exec /shorty: no such file or directory
```

用 `file` 指令看的話（例如在 build stage 裡 `apk add file`），正確版是 `statically linked`，錯誤示範版是 `dynamically linked, interpreter /lib64/ld-linux-x86-64.so.2` —— alpine 裡沒有那個載入器。

### 4. Rust 與 Node

build context 是第 08 章的專案目錄：

```bash
docker build -f tutorials/09-base-images/Dockerfile.rust-scratch    -t hello-rs:scratch      tutorials/08-multi-stage/rust
docker build -f tutorials/09-base-images/Dockerfile.node-alpine     -t hello-node:alpine     tutorials/08-multi-stage/node
docker build -f tutorials/09-base-images/Dockerfile.node-distroless -t hello-node:distroless tutorials/08-multi-stage/node

docker images --format "table {{.Repository}}:{{.Tag}}\t{{.Size}}" | grep hello
```

範例輸出：

```
hello-node:distroless   172MB
hello-node:alpine       167MB
hello-node:ch08         231MB
hello-rs:scratch        3.1MB
hello-rs:ch08           25.1MB
```

## 對照 Node／TypeScript

Node 開發者最常問：「`node:24-alpine` 比較小，為什麼不直接用？」

- **native addon**：`bcrypt`、`sharp` 這類套件含 C++ 程式，要有 musl 的預編譯檔，不然得在 image 裡裝 `python3 make g++` 現場編。
- **build 與 runtime 要同一種 libc**：glibc 上裝好的 native addon 搬到 musl 會載入失敗。所以 `Dockerfile.node-alpine` 兩個 stage 都用 alpine，`Dockerfile.node-distroless` 的 build stage 用 `node:24-bookworm-slim` 對齊 debian12。
- **distroless/nodejs 沒有 npm**：`CMD ["npm", "start"]` 不能用，只能 `CMD ["server.js"]`（它的 `ENTRYPOINT` 已經是 node）。這反而是好事：npm 當 PID 1 本來就不建議（第 16 章）。

純 JS 依賴、沒有 native addon 時，alpine 和 distroless 都能用；有疑慮就從 `node:24-slim` 開始。

## 常見陷阱

1. **`exec ...: no such file or directory`**：九成是動態連結的執行檔放進了沒有對應 libc 的 base。檢查 `CGO_ENABLED=0`、build 和 runtime 的 libc 是否一致。
2. **`x509: certificate signed by unknown authority`**：scratch 沒有 CA 憑證；alpine／debian-slim 忘了裝 `ca-certificates`。
3. **`unknown time zone Asia/Taipei`**：沒有 tzdata。裝 `tzdata`、從 build stage 複製 `/usr/share/zoneinfo`，或 `-tags timetzdata`。
4. **`apt-get install` 沒清 cache**：沒加 `--no-install-recommends` 和 `rm -rf /var/lib/apt/lists/*`，debian-slim 版會多出幾十 MB。alpine 的對應做法是 `apk add --no-cache`。
5. **用 `latest` 或不寫版本**：`alpine:latest` 某天從 3.22 跳到 3.23，`debian:stable-slim` 某天從 trixie 跳到下一版。至少寫到 minor（`alpine:3.22`）或代號（`trixie-slim`）；釘 digest 在第 21 章。
6. **在 distroless 用 shell form**：`CMD shorty` 會變成 `/bin/sh -c shorty`，distroless 沒有 `/bin/sh`，直接起不來。一律 exec form。

## 練習

1. 照「動手做」build 四個版本，把你實際看到的大小填進一張表。
2. 用 `docker history shorty:ch09-slim` 找出 `apt-get install` 那層多大；試著拿掉 `rm -rf /var/lib/apt/lists/*` 再 build，比較差多少。
3. 把 `Dockerfile` 的 runtime 暫時換成 `gcr.io/distroless/static-debian12:debug`，用 `docker run --rm -it --entrypoint sh` 進去，看看 `/etc/ssl/certs`、`/usr/share/zoneinfo`、`/etc/passwd` 裡有什麼。
4. 拿掉 `Dockerfile.scratch` 裡複製 CA 憑證的那一行，想想 shorty 在什麼情況下才會出錯（提示：`DATABASE_URL` 加上 `sslmode=require`）。
5. 比較 `-ldflags "-s -w"` 前後的執行檔大小：在 build stage 裡 `ls -l /out/shorty`。

## 下一章

image 的「內容」都定案了。[第 10 章 build-context-args](../10-build-context-args) 回頭看 `docker build` 最後那個 `app`：build context 到底送了什麼、`app/.dockerignore` 每一行在擋什麼，以及用 `ARG VERSION` + `-ldflags -X main.version` 把版本號編進 shorty，讓 `GET /version` 不再只回 `dev`。
