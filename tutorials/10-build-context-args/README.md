# 10. Build context、`.dockerignore`、ARG 與 ENV

> 範圍：build context 是什麼、`-f` 與 context 的關係、`.dockerignore` 語法與 `app/.dockerignore` 逐行解析、`ARG` vs `ENV`、ARG 的作用範圍與快取影響、`--build-arg VERSION` + `-ldflags -X main.version`、用 `GET /version` 驗證

> 🧸 想先用故事直覺理解這章？看 [STORY.md](./STORY.md)（小學生都懂的比喻版）。

## 為什麼需要這個

到第 09 章為止，shorty 的 image 只剩 ~23 MB，看起來很完美。但還有兩個問題：

1. **你不確定送了什麼進去**。每次 `docker build ... app`，Docker 都會把 `app/` 交給 builder。如果裡面躺著 `.git`、本機編好的 `shorty.exe`、寫了資料庫密碼的 `.env`，它們都可能被 `COPY . .` 帶進 image，或至少讓每次 build 的快取無故失效。
2. **你不知道 container 裡跑的是哪一版**。`curl localhost:8080/version` 永遠回 `{"version":"dev"}`。線上出事時，「現在跑的是哪個 commit？」答不出來。

這章處理「送進去的東西」和「從外面帶進 build 的參數」。

## 核心概念

### build context：builder 唯一看得到的資料夾

```bash
docker build -f tutorials/10-build-context-args/Dockerfile -t shorty:ch10 app
#                                                                          └─ context
```

- `docker build` 會把 context（`app/`）裡的檔案交給 BuildKit。**Dockerfile 裡所有 `COPY` 的來源都只能從 context 裡拿**，`COPY ../compose.yaml .` 這種往外跳的寫法拿不到東西。
- `-f` 指定的 Dockerfile 可以放在 context 外面。本系列每章的 Dockerfile 在 `tutorials/NN-*/`，context 一律是 `app/`。
- BuildKit 只會傳 Dockerfile 實際用到的檔案，而且會做增量傳輸；但 `COPY . .` 用到的就是「全部」，所以 context 裡有什麼還是很重要。

輸出裡這一行就是 context 傳輸量：

```
 => [internal] load build context                    0.0s
 => => transferring context: 21.45kB                 0.0s
```

如果這裡出現幾百 MB，通常是 `node_modules`、`target/`、`.git` 混進來了。

### `.dockerignore`：不要送進 context 的清單

放在 **context 的根目錄**（對 shorty 來說是 `app/.dockerignore`，不是 repo 根目錄），語法類似 `.gitignore`：

| 寫法 | 意思 |
|------|------|
| `# ...` | 註解 |
| `.git` | 排除 context 根目錄下的 `.git`（檔案或整個資料夾） |
| `*.md` | 排除**根目錄**的 `.md`；`*` 不會跨越 `/` |
| `**/*_test.go` | `**` 代表任意層目錄，排除所有層的 `_test.go` |
| `!README.md` | 例外：把前面排除掉的再加回來（後面的規則優先） |

一個比較少人知道的功能：如果 Dockerfile 旁邊有一份 **`<Dockerfile 檔名>.dockerignore`**（例如 `tutorials/10-build-context-args/Dockerfile.dockerignore`），BuildKit 會用它**取代** context 根目錄的 `.dockerignore`。本系列沒有用這個功能，所有章節共用 `app/.dockerignore`。

### `app/.dockerignore` 逐行解析

```
.git
**/*_test.go
*.exe
/shorty
*.md
Dockerfile
.dockerignore
.env
.env.*
```

| 規則 | 為什麼要排除 |
|------|------|
| `.git` | 可能有幾十 MB；每次 commit 都會變，讓 `COPY . .` 快取永遠失效；還帶著完整的歷史紀錄 |
| `**/*_test.go` | `go build` 本來就不編測試檔，放進去沒用；排除後，**只改測試不會讓 image 重 build** |
| `*.exe` | Windows 上在 `app/` 跑 `go build` 會產生 `shorty.exe`，不該送進 Linux image |
| `/shorty` | Linux／macOS 上 `go build` 產生的是沒有副檔名的 `shorty`，`*.exe` 抓不到它，要另外寫。開頭的 `/` 只是寫給人看的：`.dockerignore` 的規則**本來就一律從 context 根目錄比對**，`/shorty` 和 `shorty` 完全一樣（這點跟 `.gitignore` 不同） |
| `*.md` | 文件不影響執行，改文件不該讓快取失效 |
| `Dockerfile`、`.dockerignore` | builder 讀 Dockerfile 不受 `.dockerignore` 影響；排除它們是為了**改 Dockerfile 不會讓 `COPY . .` 那層失效**，也不會把 build 設定帶進 image |
| `.env`、`.env.*` | 本機的環境變數檔常常有密碼。秘密不能進 image（第 14 章） |

注意 `*.exe`、`*.md` 只比對 `app/` 根目錄。如果以後在子目錄放了 `.md`，要改成 `**/*.md` 才會排除。

### `ARG` vs `ENV`

| | `ARG` | `ENV` |
|------|------|------|
| 什麼時候存在 | **只有 build 期間** | build 期間 + **container 執行時** |
| 怎麼設定 | `--build-arg NAME=value`，Dockerfile 可給預設值 | Dockerfile 寫死；執行時可被 `docker run -e` 蓋掉 |
| 作用範圍 | 宣告它的那個 stage，從宣告處開始 | 宣告它的 stage，以及以它為底的 image |
| 典型用途 | 版本號、要裝的工具版本、build 選項 | 程式執行時要讀的設定（`PORT`、`LOG_LEVEL`） |
| 能放秘密嗎 | **不能**，`docker history` 看得到 | **不能**，`docker inspect` 看得到 |

要讓 build 時的參數在執行時也看得到，可以接力：

```dockerfile
ARG VERSION=dev
ENV APP_VERSION=$VERSION      # 變成 container 裡的環境變數
```

shorty 不需要這樣做，因為版本會直接編進執行檔裡。

ARG 的兩個細節：

- **寫在第一個 `FROM` 之前的 `ARG` 只能用在 `FROM` 那一行**（例如 `FROM golang:${GO_VERSION}-alpine`）。stage 裡面要用，得再寫一次 `ARG GO_VERSION`（不用給值，會沿用）。
- **快取**：`ARG` 這行本身不會讓快取失效，但它**之後的每一個 `RUN`** 都會把它當環境變數，值一變，那些 `RUN` 就全部重做。所以 `ARG VERSION` 要放得越後面越好 —— 放在 `RUN go mod download` 之前的話，每換一次版本號就要重新下載依賴。

### `-ldflags -X`：連結時改寫字串變數

`app/main.go` 裡有：

```go
// version 在 build 時用 -ldflags "-X main.version=..." 注入
var version = "dev"
```

```bash
go build -ldflags "-X main.version=v0.1.0" .
```

Go 的 linker 會把 `main` 套件的 `version` 變數改成 `"v0.1.0"`。規則：

- 格式是 `-X <import path>.<變數名>=<值>`。在 `main` 套件寫 `main.version`；在其他套件要寫完整路徑，例如 `-X github.com/ChecheHuang/docker-tutorials/app/internal/server.Foo=...`。
- 只能用在 **`string` 型別的 package 層級變數**，不能是 `const`。
- **名字打錯不會報錯**，只是靜靜地沒效果。一定要實際驗證。

## shorty 在這章的演進

完整檔案在 [`Dockerfile`](./Dockerfile)：

```dockerfile
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X main.version=${VERSION}" \
      -o /out/shorty .

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/shorty /shorty
EXPOSE 8080
ENTRYPOINT ["/shorty"]
```

跟第 09 章的差異：

```diff
 COPY . .
-RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/shorty .
+
+ARG VERSION=dev
+RUN CGO_ENABLED=0 go build -trimpath \
+      -ldflags "-s -w -X main.version=${VERSION}" \
+      -o /out/shorty .
```

- `ARG VERSION=dev` 放在 build stage、而且在依賴層**之後**。沒給 `--build-arg` 時就是 `dev`，跟程式碼裡的預設值一致。
- runtime stage 不需要 `VERSION`，版本已經編進執行檔裡了。
- `.dockerignore` 沒有改，本章是把它「看懂」。

**image 大小（範例）**：~23 MB，跟第 09 章一樣。這章改的是**可追蹤性**，不是大小。

## 動手做

以下指令都在 repo 根目錄執行。

### 1. 看看 context 裡到底送了什麼

用 `-f-` 從 stdin 餵一份臨時 Dockerfile，把 context 整包 `COPY` 進 alpine 再列出來：

```bash
docker build -t ctx-check -f- app <<'EOF'
FROM alpine:3.22
COPY . /ctx
CMD ["find", "/ctx", "-type", "f"]
EOF

docker run --rm ctx-check
```

範例輸出：

```
/ctx/go.mod
/ctx/go.sum
/ctx/main.go
/ctx/internal/cache/cache.go
/ctx/internal/server/server.go
/ctx/internal/store/postgres.go
/ctx/internal/store/store.go
```

沒有 `Dockerfile`、`.dockerignore`、`server_test.go` —— 都被 `app/.dockerignore` 擋掉了。

再放一個假的 `.env` 和一個不在清單裡的檔案試試：

```bash
echo 'POSTGRES_PASSWORD=supersecret' > app/.env
echo 'hello' > app/notes.txt
docker build -t ctx-check -f- app <<'EOF'
FROM alpine:3.22
COPY . /ctx
CMD ["find", "/ctx", "-type", "f"]
EOF
docker run --rm ctx-check | grep -E "env|notes"
rm app/.env app/notes.txt
```

範例輸出：

```
/ctx/notes.txt
```

`.env` 被擋住了，`notes.txt` 沒有 —— `.dockerignore` 是黑名單，沒寫到的都會進去。

### 2. 不帶版本 build

```bash
docker build -f tutorials/10-build-context-args/Dockerfile -t shorty:ch10 app
docker run --rm -d -p 8080:8080 --name s shorty:ch10
curl localhost:8080/version
docker stop s
```

範例輸出：

```
{"version":"dev"}
```

### 3. 帶版本 build

```bash
docker build -f tutorials/10-build-context-args/Dockerfile \
  --build-arg VERSION=v0.1.0 -t shorty:v0.1.0 app

docker run --rm -d -p 8080:8080 --name s shorty:v0.1.0
curl localhost:8080/version
docker logs s
docker stop s
```

範例輸出：

```
{"version":"v0.1.0"}
{"time":"...","level":"WARN","msg":"DATABASE_URL not set, using in-memory store"}
{"time":"...","level":"WARN","msg":"REDIS_URL not set, cache disabled"}
{"time":"...","level":"INFO","msg":"listening","addr":":8080","version":"v0.1.0"}
...
```

啟動 log 也印出了版本。

### 4. 用 git 資訊當版本

```bash
docker build -f tutorials/10-build-context-args/Dockerfile \
  --build-arg VERSION="$(git describe --tags --always --dirty)" -t shorty:ch10 app
```

沒有 tag 時 `git describe --always` 會給 commit 短 hash（例如 `3f2a9c1`），有未 commit 的修改會加上 `-dirty`。tag 策略、OCI label 是第 22 章的主題。

### 5. 觀察 ARG 與快取

換一個版本號再 build：

```bash
docker build -f tutorials/10-build-context-args/Dockerfile \
  --build-arg VERSION=v0.1.1 -t shorty:v0.1.1 app
```

範例輸出：

```
 => CACHED [build 2/6] WORKDIR /src
 => CACHED [build 3/6] COPY go.mod go.sum ./
 => CACHED [build 4/6] RUN go mod download
 => CACHED [build 5/6] COPY . .
 => [build 6/6] RUN CGO_ENABLED=0 go build -trimpath ...
 => [stage-1 2/2] COPY --from=build /out/shorty /shorty
```

依賴層和 `COPY . .` 都沿用快取，只有用到 `VERSION` 的 `go build` 重做；執行檔內容變了，runtime stage 的 `COPY --from` 也跟著重做。`stage-1` 是沒取名字的最後一個 stage 的預設名稱。

## 對照 Node／TypeScript

Node 專案的 `.dockerignore` 通常長這樣（第 08 章的 [`node/.dockerignore`](../08-multi-stage/node/.dockerignore) 是精簡版）：

```
node_modules
dist
coverage
.git
.env
.env.*
npm-debug.log*
Dockerfile
.dockerignore
```

`node_modules` 是**最重要的一行**：本機的 `node_modules` 可能有幾百 MB，裡面的 native module 還是為 macOS／Windows 編的。image 裡應該用 `npm ci` 重新裝。

版本注入的對應做法：

| Go | Node／TypeScript |
|------|------|
| `-ldflags "-X main.version=$VERSION"` 編進執行檔 | `ARG VERSION` + `ENV APP_VERSION=$VERSION`，程式讀 `process.env.APP_VERSION` |
| | 或用 bundler 在 build 時替換常數：esbuild `--define:__VERSION__='"v0.1.0"'`、Vite `define` |
| | 或直接讀 `package.json` 的 `version`（但它通常不等於 git 版本） |

`ARG` 的作用範圍、快取規則在 Node 的 Dockerfile 裡完全一樣：`ARG VERSION` 放在 `RUN npm ci` 之前，每換一次版本就重裝所有依賴。

## 常見陷阱

1. **`.dockerignore` 放錯位置**：它要在 **context 根目錄**。shorty 的 context 是 `app/`，放在 repo 根目錄的 `.dockerignore` 對它完全沒作用。
2. **`*.md` 以為會排除所有層**：只排除根目錄的，子目錄要用 `**/*.md`。
3. **排除了 build 需要的檔案**：例如手滑寫了 `*.sum`，`COPY go.mod go.sum ./` 會報 `"/go.sum": not found`。
4. **`ARG` 寫在 `FROM` 之前，卻在 stage 裡用**：值是空的，`-X main.version=` 會把版本設成空字串，`/version` 回 `{"version":""}`。要在 stage 裡再宣告一次 `ARG VERSION`。
5. **`-X main.Version`（大寫）**：變數叫 `version`，名字對不上，linker 不會報錯，版本仍然是 `dev`。
6. **`-ldflags` 引號**：多個 flag 要包在同一組引號裡：`-ldflags "-s -w -X main.version=${VERSION}"`。拆開寫 `-ldflags -s -w ...` 會被解析成別的參數。
7. **用 `ARG` 或 `ENV` 傳 token**：`--build-arg NPM_TOKEN=...` 會留在 build 紀錄裡。私有依賴要用 BuildKit 的 secret mount（第 11 章），執行期秘密見第 14 章。
8. **`COPY ../something`**：context 外的檔案拿不到。要嘛把 context 往上提一層，要嘛把檔案搬進 context。

## 練習

1. 照「動手做」第 1 步列出 context 內容，確認 `server_test.go` 不在裡面。
2. 在 `.gitignore` 裡寫 `shorty` 會排除**任何一層**叫 `shorty` 的檔案，但在 `.dockerignore` 裡只排除 context 根目錄那一個。如果想在 `.dockerignore` 排除所有層的 `shorty`，要怎麼寫？
3. 把 Dockerfile 裡的 `ARG VERSION=dev` 移到 `RUN go mod download` 之前，換兩個不同的 `VERSION` build，觀察依賴層是否還被快取。做完改回來。
4. 故意把 `-X main.version` 打成 `-X main.Version`，build 後 `curl /version`，確認 linker 真的沒有報錯。
5. 在 runtime stage 加上 `ARG VERSION` 和 `ENV APP_VERSION=$VERSION`，build 後用 `docker inspect --format '{{.Config.Env}}' shorty:ch10` 看環境變數。

## 下一章

到這裡，shorty 的 Dockerfile 已經很接近最終版了，但每次重 build，Go 的編譯快取都會被丟掉；需要私有依賴時，token 也沒有安全的傳法。[第 11 章 buildkit](../11-buildkit) 介紹 `# syntax=`、`RUN --mount=type=cache`、`--mount=type=secret` 和 `docker buildx`。
