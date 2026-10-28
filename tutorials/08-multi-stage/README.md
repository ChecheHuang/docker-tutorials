# 08. Multi-stage build（Go／Rust／Node 對照）

> 範圍：multi-stage build、`FROM ... AS <name>`、`COPY --from=<stage>`、`--target`、build stage vs runtime stage、image 從 ~1GB 降到 ~20MB；Rust 版（cargo-chef）與 Node 版（`npm ci` + `npm prune --omit=dev`）對照

> 🧸 想先用故事直覺理解這章？看 [STORY.md](./STORY.md)（小學生都懂的比喻版）。

## 為什麼需要這個

第 07 章結束時 shorty image 還是 ~1.1GB，但真正上線要跑的只有一個 ~18MB 的執行檔。剩下的都是**編譯時才需要**的東西：

- Go 工具鏈、gcc、git
- 原始碼
- module cache（`/go/pkg/mod`）、build cache

第 07 章也試過了：後面加 `RUN rm -rf` 沒用，layer 只會往上加。

以前的做法是寫兩份 Dockerfile + 一個 shell script：先用大 image 編譯、`docker cp` 把執行檔拿出來、再用另一份 Dockerfile 包進小 image。**multi-stage build** 把這整套流程收進一份 Dockerfile：在一個 stage 編譯，在另一個乾淨的 stage 只放成品。

## 核心概念

### 一份 Dockerfile，好幾個 `FROM`

```dockerfile
FROM golang:1.26-alpine AS build     # stage 1：取名叫 build
...                                  # 在這裡編譯
FROM alpine:3.22                     # stage 2：全新、乾淨的起點
COPY --from=build /out/shorty /usr/local/bin/shorty   # 只從 build 拿一個檔案
```

規則很簡單：

- **每個 `FROM` 開始一個新的 stage**，跟前面的 stage 檔案系統完全無關。
- **`AS <name>`** 幫 stage 取名，之後用名字引用（也可以用編號 `--from=0`，但不好讀）。
- **`COPY --from=<stage>`** 從別的 stage 複製檔案過來 —— 這是 stage 之間唯一的通道。
- **最後一個 stage 就是最終 image**。前面的 stage 只存在 build 過程裡，不會出現在結果中。

```
 stage "build"  (golang:1.26-alpine, ~300MB)
 ┌─────────────────────────────────┐
 │ Go 工具鏈 / 原始碼 / module cache │
 │            /out/shorty ─────────┼──┐  COPY --from=build
 └─────────────────────────────────┘  │
                                      ▼
 最終 image  (alpine:3.22)  ┌──────────────────────┐
                            │ /usr/local/bin/shorty │  ~26MB
                            └──────────────────────┘
```

### `COPY --from` 也可以指向別的 image

```dockerfile
COPY --from=alpine:3.22 /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
```

`--from` 後面不是 stage 名字時，會被當成 image 名稱，直接從那個 image 拿檔案。第 09 章的 scratch 版會用到類似的技巧。

### `--target`：只 build 到某個 stage

```bash
docker build --target build -f tutorials/08-multi-stage/Dockerfile -t shorty:ch08-build app
```

停在 `build` stage，得到一個有完整工具鏈的 image，方便進去 debug 編譯問題。BuildKit 也很聰明：最終 stage 用不到的 stage 會直接跳過不 build，彼此不相依的 stage 還會平行 build。

## shorty 在這章的演進

完整檔案在 [`Dockerfile`](./Dockerfile)：

```dockerfile
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/shorty .

FROM alpine:3.22
COPY --from=build /out/shorty /usr/local/bin/shorty
EXPOSE 8080
CMD ["shorty"]
```

跟第 07 章的差異：

```diff
-FROM golang:1.26
+FROM golang:1.26-alpine AS build
 WORKDIR /src
 COPY go.mod go.sum ./
 RUN go mod download
 COPY . .
-RUN go build -o /usr/local/bin/shorty .
+RUN CGO_ENABLED=0 go build -o /out/shorty .
 
+FROM alpine:3.22
+COPY --from=build /out/shorty /usr/local/bin/shorty
 EXPOSE 8080
 CMD ["shorty"]
```

三個變化：

1. **build stage 換成 `golang:1.26-alpine`**：比 Debian 版小很多、下載快。build stage 多大其實不影響最終 image，但會影響 CI 拉 image 的時間。
2. **`CGO_ENABLED=0`**：讓 Go 編出不依賴 C 函式庫的靜態執行檔，這樣放進哪種 runtime image 都能跑。為什麼「不加會出事」是第 09 章的主題，這裡先照抄。
3. **runtime stage 是 `alpine:3.22`**：只有 ~8MB 的小 Linux，加上 shorty 執行檔就是全部。依賴層拆分（第 07 章）原封不動留在 build stage。

**image 大小（範例）**：

| image | 大小 |
|------|------|
| `shorty:ch07` | ~1.1 GB |
| `shorty:ch08` | **~26 MB** |

縮小四十倍。第 09 章換 base image、拿掉除錯符號，還能再砍一半。

## Rust 與 Node 對照

這章附了兩個最小的對照服務（第 09、23 章也會用到）：

| 目錄 | 內容 |
|------|------|
| [`rust/`](./rust) | `hello-rs`：axum 0.8 + tokio，`GET /` 回一行字，聽 `0.0.0.0:8080` |
| [`node/`](./node) | `hello-node`：express，`GET /` 回一行字，聽 `8080`，`npm start` 啟動 |

注意它們的 **build context 是自己的資料夾**（不是 `app/`），各自也有 `.dockerignore` 擋掉 `target/`、`node_modules/`。

### Rust：cargo-chef 解決「依賴層」問題

Go 有 `go mod download` 可以「只下載依賴」，Rust 沒有對應的 `cargo` 指令 —— `cargo build` 一定要有原始碼才能跑。[cargo-chef](https://github.com/LukeMathWalker/cargo-chef) 把這件事拆成兩步：

```dockerfile
FROM rust:1.95-slim-bookworm AS chef
RUN cargo install cargo-chef --locked
WORKDIR /app

FROM chef AS planner
COPY . .
RUN cargo chef prepare --recipe-path recipe.json     # 只萃取依賴資訊

FROM chef AS builder
COPY --from=planner /app/recipe.json recipe.json
RUN cargo chef cook --release --recipe-path recipe.json   # 只編依賴 → 這層會被快取
COPY . .
RUN cargo build --release --bin hello-rs

FROM gcr.io/distroless/cc-debian12 AS runtime
COPY --from=builder /app/target/release/hello-rs /hello-rs
EXPOSE 8080
ENTRYPOINT ["/hello-rs"]
```

- **planner** 讀 `Cargo.toml`／`Cargo.lock`，產出 `recipe.json`。即使原始碼變了，只要依賴沒變，`recipe.json` 的內容就不變。
- **builder** 先 `COPY` 那份 `recipe.json` 再 `cook` —— 跟 Go 的「先 COPY go.mod」是同一招，只是多一個 stage 幫你算出「依賴清單」。
- **runtime** 用 `distroless/cc`：Rust 預設動態連結 glibc，所以需要一個有 glibc 的 runtime。builder 用 `bookworm`（Debian 12）、runtime 用 `debian12`，兩邊 glibc 同一代。第 09 章會介紹 musl 靜態連結 + `scratch` 的做法。

（`ENTRYPOINT` 跟 `CMD` 的差別第 16 章再講，這裡當成「啟動時執行這個程式」就好。）

### Node：`npm ci` + `npm prune --omit=dev`

```dockerfile
FROM node:24-slim AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
# 有 TypeScript / bundler 的話在這裡跑：RUN npm run build
RUN npm prune --omit=dev

FROM node:24-slim AS runtime
ENV NODE_ENV=production
WORKDIR /app
COPY --from=build --chown=node:node /app/node_modules ./node_modules
COPY --chown=node:node package.json server.js ./
USER node
EXPOSE 8080
CMD ["node", "server.js"]
```

- **build stage** 用 `npm ci` 裝**全部**依賴（包含 TypeScript、測試工具這些 devDependencies），build 完再 `npm prune --omit=dev` 砍掉開發用的。
- **runtime stage** 只拿 `node_modules`（已瘦身）和程式碼。npm 的下載快取、devDependencies、原始 `.ts` 檔都留在 build stage。
- **`USER node`**：官方 node image 內建 uid 1000 的 `node` 使用者，搭配 `--chown` 讓檔案屬於它。為什麼不要用 root 跑留到第 21 章。
- **關鍵差異**：runtime **還是需要 `node` 本體**，所以 runtime base 不能是空的。

### 三語對照

| | Go（shorty） | Rust（hello-rs） | Node（hello-node） |
|------|------|------|------|
| build stage | `golang:1.26-alpine` | `rust:1.95-slim-bookworm` + cargo-chef | `node:24-slim` |
| 依賴層 | `COPY go.mod go.sum` + `go mod download` | `cargo chef prepare` → `cook` | `COPY package*.json` + `npm ci` |
| 產物 | 一個靜態執行檔 | 一個執行檔（動態連結 glibc） | `.js` + `node_modules` |
| runtime 需要什麼 | 什麼都不用 | glibc | Node.js runtime |
| runtime base | `alpine:3.22` | `distroless/cc-debian12` | `node:24-slim` |
| 最終大小（範例） | ~26 MB | ~25 MB | ~230 MB |
| 單階段時（範例） | ~1.1 GB | ~1.5 GB | ~1.1 GB |

編譯成原生執行檔的語言（Go、Rust）從 multi-stage 得到的好處最大。Node 也省下不少，但天花板卡在 runtime 本身的大小 —— 第 09 章看 alpine 和 distroless 能再省多少。

## 動手做

以下指令都在 repo 根目錄執行。

### 1. shorty

```bash
docker build -f tutorials/08-multi-stage/Dockerfile -t shorty:ch08 app
docker images shorty
```

範例輸出：

```
REPOSITORY   TAG    IMAGE ID       CREATED          SIZE
shorty       ch08   9b7e4a2c1f30   10 seconds ago   26.3MB
shorty       ch07   5d1e0b7c9a42   1 hour ago       1.12GB
shorty       ch06   3f2a9c1d8e7b   2 hours ago      1.12GB
```

```bash
docker run --rm -p 8080:8080 shorty:ch08
curl localhost:8080/healthz          # 另開終端機
```

範例輸出：`{"status":"ok"}`

確認 image 裡真的只剩執行檔：

```bash
docker run --rm shorty:ch08 ls /src          # ls: /src: No such file or directory
docker run --rm shorty:ch08 which go         # 沒有輸出，exit code 1
```

### 2. 只 build 到 build stage

```bash
docker build --target build -f tutorials/08-multi-stage/Dockerfile -t shorty:ch08-build app
docker run --rm shorty:ch08-build ls -la /out
```

範例輸出：

```
-rwxr-xr-x    1 root     root      17865444 ... shorty
```

### 3. Rust 與 Node

```bash
docker build -t hello-rs:ch08   tutorials/08-multi-stage/rust
docker build -t hello-node:ch08 tutorials/08-multi-stage/node

docker run --rm -d -p 8081:8080 --name rs   hello-rs:ch08
docker run --rm -d -p 8082:8080 --name node hello-node:ch08
curl localhost:8081/
curl localhost:8082/
docker stop rs node
```

範例輸出：

```
hello from Rust (axum)
hello from Node (express)
```

第一次 build Rust 會比較久（`cargo install cargo-chef` 加上編譯 tokio／axum，幾分鐘跑不掉）。改一下 `rust/src/main.rs` 再 build，會看到 `cargo chef cook` 那層 `CACHED`，只重編 `hello-rs` 本身。

### 4. 比大小

```bash
docker images --format "table {{.Repository}}:{{.Tag}}\t{{.Size}}" | grep -E "shorty|hello"
```

範例輸出：

```
shorty:ch08          26.3MB
hello-rs:ch08        25.1MB
hello-node:ch08      231MB
```

## 對照 Node／TypeScript

上面已經是完整的 Node 版本。TypeScript 專案的 multi-stage 通常長這樣：

```dockerfile
FROM node:24-slim AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci                         # 含 typescript、@types/*
COPY . .
RUN npm run build                  # tsc → dist/
RUN npm prune --omit=dev

FROM node:24-slim
WORKDIR /app
COPY --from=build /app/node_modules ./node_modules
COPY --from=build /app/dist ./dist
COPY package.json ./
USER node
CMD ["node", "dist/server.js"]
```

TypeScript 的 `tsc` 就像 Go 的 `go build`：只在 build stage 需要，runtime 只要編出來的 `.js`。如果用 esbuild／tsup 把程式和依賴打包成單一檔案，runtime 甚至可以不需要 `node_modules`，概念上就更接近 Go 的單一執行檔了。

## 常見陷阱

1. **`COPY --from` 的路徑寫錯**：來源路徑是**那個 stage 裡的絕對路徑**，不是 build context 的路徑。寫 `COPY --from=build shorty ...` 會找不到，要寫 `/out/shorty`（或相對於那個 stage 的 `WORKDIR`）。
2. **在 alpine 跑 glibc 執行檔**：拿掉 `CGO_ENABLED=0`、build stage 改成 Debian 版 `golang:1.26`，執行檔可能會動態連結 glibc，丟進 alpine 會出現看似荒謬的 `exec /usr/local/bin/shorty: no such file or directory`。第 09 章解釋。
3. **Rust builder 和 runtime 的 glibc 世代不同**：`rust:1.95`（預設是較新的 Debian）配 `distroless/cc-debian12`，可能出現 `GLIBC_2.xx not found`。兩邊對齊同一代 Debian。
4. **Node 的 native addon 跨 libc**：在 Debian（glibc）stage 裝好的 `node_modules`（`bcrypt`、`sharp`），搬到 alpine（musl）runtime 會載入失敗。build 和 runtime 用同一個系列的 base。
5. **以為前面的 stage 會留在 image 裡**：不會。但它們會留在本機的 build cache，`docker system df` 看得到，`docker builder prune` 可以清。
6. **`ARG`、`ENV` 不會跨 stage**：在 build stage 設的 `ENV` 到 runtime stage 就沒了，每個 stage 要各自宣告（第 10 章）。
7. **忘了 Rust／Node 的 `.dockerignore`**：`target/` 可以有好幾 GB，送進 context 光傳輸就要等很久，還會讓 `COPY . .` 快取一直失效。

## 練習

1. build `shorty:ch08`，用 `docker history shorty:ch08` 看看最終 image 只剩哪幾層。
2. 用 `--target build` build 出 build stage，用 `docker images` 比較它和最終 image 的大小。
3. 把 runtime stage 從 `alpine:3.22` 換成 `golang:1.26-alpine`，看 image 變多大。這說明了什麼？
4. 在 Rust 版改一行 `main.rs` 後重 build，確認 `cargo chef cook` 那層被快取。再改 `Cargo.toml` 加一個依賴（例如 `serde = "1"`），看從哪一層開始重做。
5. 在 Node 版加一個 devDependency（`npm install -D typescript`），build 後用 `docker run --rm hello-node:ch08 ls node_modules` 確認它沒有出現在 runtime image 裡。做完用 `npm uninstall -D typescript` 還原，`rust/`、`node/` 第 09、23 章還會用到。

## 下一章

runtime 我們隨手選了 `alpine:3.22`，Rust 用了 `distroless/cc`、Node 用了 `node:24-slim`。[第 09 章 base-images](../09-base-images) 比較 debian-slim、alpine、distroless、scratch 各有什麼、缺什麼，說清楚 `CGO_ENABLED=0` 到底在防什麼，以及 CA 憑證、時區資料要怎麼處理。
