# 11. BuildKit：cache mount 與 secret mount

> 範圍：`# syntax=`、`RUN --mount=type=cache`、`--mount=type=secret`（私有依賴）、`docker buildx`、`--progress=plain`、`docker builder prune`

> 🧸 想先用故事直覺理解這章？看 [STORY.md](./STORY.md)（小學生都懂的比喻版）。

## 為什麼需要這個

第 10 章結束時，shorty 的 Dockerfile 已經是 multi-stage + distroless + `ARG VERSION`，image 只有二十多 MB。但 build 還有兩個問題：

1. **layer 快取是「全有或全無」**。`go.mod` 加了一個依賴，`RUN go mod download` 這層就整層失效，**所有**模組重新下載；改一行程式碼，`go build` 那層失效，**所有** package 重新編譯。在自己電腦上 `go build` 只要兩秒，到了 Docker 裡卻要一分鐘。
2. **私有依賴需要 token**。公司內部的 Go module 放在私有 GitHub repo，`go mod download` 要帳密。用 `ARG GITHUB_TOKEN` 傳進去？第 14 章會示範：build arg 會留在 `docker history` 裡，等於把 token 印在 image 上送出去。

BuildKit（Docker Engine 23.0 起的預設 builder）用 `RUN --mount` 解決這兩件事：

- `--mount=type=cache`：給 `RUN` 一個**跨 build 保留**的資料夾，不進 layer。
- `--mount=type=secret`：把秘密**只在這一個 `RUN` 期間**掛進去，不進 layer、不進 history。

## 核心概念

### BuildKit 是什麼

```
docker build ...                 你下的指令（其實是 docker buildx build 的別名）
      │
      ▼
 buildx（CLI plugin）            負責解析參數、選 builder
      │
      ▼
 BuildKit（builder）             真正執行 build：平行跑 stage、管理快取
      │   └─ frontend：dockerfile:1   ← 由 `# syntax=` 決定版本，負責「看懂」Dockerfile
      ▼
 image（存進 docker 的 image store）
```

- **舊 builder（legacy）**：一行一行照順序跑，沒有 `--mount`。Docker 23.0 之後 Linux 上預設就是 BuildKit，不需要再設 `DOCKER_BUILDKIT=1`。
- **`# syntax=docker/dockerfile:1`**：寫在 Dockerfile **第一行**，叫 BuildKit 先去拉 `docker/dockerfile:1` 這個前端 image 來解析 Dockerfile。`:1` 代表「1.x 最新穩定版」，新語法（例如 secret 的 `env=`）不用等 Docker Engine 升級就能用。

### cache mount vs layer cache

```
                   layer cache（第 07 章）            cache mount（本章）
存在哪裡           image 的一層                       builder 的快取區，不進 image
什麼時候失效       這一行或前面任何一行變了           不會因為指令變而失效，只有 prune 才清
失效時             整層重做（全部重下載／重編）       資料夾還在，工具自己做增量
會不會讓 image 變大 會（如果留在最後的 stage）        不會
```

兩個搭配使用：layer 快取命中時，連 `RUN` 都不用跑；layer 快取沒命中時，cache mount 讓 `go` 只做「真的變了的那部分」。

Go 需要快取的兩個資料夾（`golang` image 以 root 執行，`GOPATH=/go`）：

| 路徑 | 是什麼 | 對照 |
|------|--------|------|
| `/go/pkg/mod` | 下載的 module 原始碼（`GOMODCACHE`） | `~/.npm` |
| `/root/.cache/go-build` | 編譯產物快取（`GOCACHE`） | `tsc --incremental` 的 `.tsbuildinfo` |

### secret mount

```
docker build --secret id=netrc,src=$HOME/.netrc ...
                         │
                         ▼
RUN --mount=type=secret,id=netrc,target=/root/.netrc  go mod download
        │
        ├─ 這個 RUN 執行時：/root/.netrc 存在（tmpfs，不寫進 layer）
        └─ RUN 結束：檔案消失；layer、docker history、image 裡都找不到
```

- 不寫 `target` 時，預設掛在 `/run/secrets/<id>`。
- `env=NAME`（dockerfile 1.10+）：直接變成這個 `RUN` 的環境變數。
- `required=true`：沒給 `--secret` 就讓 build 失敗，避免默默用空 token 繼續然後在奇怪的地方報錯。
- **secret 的內容不參與快取計算**：換了 token，這層照樣命中快取。這通常是你要的（token 換了，依賴並沒變）。

### buildx 與 builder

`docker buildx` 是 BuildKit 的 CLI。`docker build` 現在就是 `docker buildx build` 的別名，用的是預設 builder（driver = `docker`，直接用 dockerd 內建的 BuildKit）。

另外可以建 `docker-container` driver 的 builder：BuildKit 跑在一個獨立 container 裡，支援更多功能（多平台輸出、把快取匯出到 registry／GitHub Actions）。這些是第 23、24 章的事，本章只需要知道怎麼看、怎麼切。

## shorty 在這章

[`Dockerfile`](./Dockerfile) 跟第 10 章比只多兩件事：

```dockerfile
# syntax=docker/dockerfile:1                           ← 1. 指定前端

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \            ← 2. module 快取
    go mod download

COPY . .
ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod \            ← 2. 編譯時也要掛同一個
    --mount=type=cache,target=/root/.cache/go-build \  ← 2. 編譯快取
    CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/shorty .
```

另外兩份示範檔：

| 檔案 | 用途 |
|------|------|
| [`Dockerfile.secret-demo`](./Dockerfile.secret-demo) | 可以直接跑，證明 secret 在 build 時讀得到、image 裡找不到 |
| [`Dockerfile.private-modules`](./Dockerfile.private-modules) | 私有 Go module 的**樣板**（shorty 沒有私有依賴，換成你的 org 才能用） |

最終版 `app/Dockerfile` 的 cache mount 寫法跟這章一樣，只是再加上 `--platform=$BUILDPLATFORM`（第 23 章）和 non-root（第 21 章）。

## 動手做

以下指令都從 repo 根目錄執行。

### 1. 確認 BuildKit 與 buildx

```bash
docker buildx version
docker buildx ls
```

範例輸出：

```
github.com/docker/buildx v0.29.1 ...

NAME/NODE           DRIVER/ENDPOINT     STATUS    BUILDKIT   PLATFORMS
default*            docker
 \_ default          \_ default         running   v0.25.1    linux/amd64, linux/amd64/v2, ...
desktop-linux       docker
 \_ desktop-linux    \_ desktop-linux   running   v0.25.1    linux/amd64, linux/arm64, ...
```

`*` 是目前使用中的 builder。Docker Desktop 會多一個 `desktop-linux`。版本號依你的安裝而異。

### 2. 第一次 build，用 `--progress=plain` 看細節

預設的 `auto` 輸出會把完成的步驟折疊起來；`plain` 會把每一步的完整輸出都印出來，除錯時必備。

```bash
docker build -f tutorials/11-buildkit/Dockerfile --progress=plain -t shorty:ch11 app
```

範例輸出（節錄）：

```
#1 [internal] load build definition from Dockerfile
#2 resolve image config for docker-image://docker.io/docker/dockerfile:1
#3 docker-image://docker.io/docker/dockerfile:1@sha256:...
...
#9 [build 4/6] RUN --mount=type=cache,target=/go/pkg/mod     go mod download
#9 DONE 8.4s
...
#11 [build 6/6] RUN --mount=type=cache,target=/go/pkg/mod  --mount=type=cache,target=/root/.cache/go-build ...
#11 DONE 21.7s
```

注意 `#2`、`#3`：這就是 `# syntax=` 在拉前端。

### 3. 改一行程式碼再 build

隨便改一下 `app/internal/server/server.go` 的一行註解（改完記得改回來），再 build：

```bash
docker build -f tutorials/11-buildkit/Dockerfile --progress=plain -t shorty:ch11 app
```

範例輸出：

```
#9 [build 4/6] RUN --mount=type=cache,target=/go/pkg/mod     go mod download
#9 CACHED
...
#11 [build 6/6] RUN --mount=type=cache,target=/go/pkg/mod ...
#11 DONE 3.2s
```

`COPY . .` 失效，`go build` 那層必須重跑——但因為 `/root/.cache/go-build` 還在，只有 `server` package 和 `main` 重編，pgx、go-redis 都不用。21 秒變 3 秒。

想體驗「依賴變了」的情況：在 `app/` 下 `go get` 一個小套件，build 一次——你會看到 `go mod download` 重跑，但只下載新的那一個。（試完記得把 `app/go.mod`、`app/go.sum` 還原，`app/` 是各章共用的。）

### 4. secret mount：build 讀得到，image 找不到

```bash
export DEMO_TOKEN=s3cr3t-value
docker build -f tutorials/11-buildkit/Dockerfile.secret-demo \
  --secret id=demo_token,env=DEMO_TOKEN \
  --progress=plain --no-cache -t secret-demo app
```

範例輸出（節錄）：

```
#6 [2/3] RUN --mount=type=secret,id=demo_token,required=true     echo "build 時讀得到 secret，長度 = ..."
#6 0.214 build 時讀得到 secret，長度 = 12 bytes
#7 [3/3] RUN --mount=type=secret,id=demo_token,env=DEMO_TOKEN     test -n "$DEMO_TOKEN" && echo "env 形式也讀得到"
#7 0.198 env 形式也讀得到
```

檢查 image：

```bash
docker history --no-trunc secret-demo | grep -c s3cr3t-value   # 0
docker run --rm secret-demo
```

範例輸出：

```
0
ls: /run/secrets: No such file or directory
DEMO_TOKEN=<empty>
```

`--secret` 的來源有兩種：`src=<檔案路徑>` 或 `env=<環境變數名>`。不給 `--secret` 試試看，`required=true` 會讓 build 直接失敗。

### 5. 私有 Go module（樣板說明）

[`Dockerfile.private-modules`](./Dockerfile.private-modules) 的重點：

```dockerfile
RUN apk add --no-cache git                  # golang:alpine 沒有 git，直接抓 VCS 需要它
ENV GOPRIVATE=github.com/your-org/*         # 不走 proxy.golang.org、不查 sum.golang.org
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=secret,id=netrc,target=/root/.netrc,required=true \
    go mod download
```

`.netrc`（放在 build context **以外**，例如 `$HOME/.netrc`）：

```
machine github.com login x-access-token password ghp_xxxxxxxxxxxx
```

```bash
docker build -f tutorials/11-buildkit/Dockerfile.private-modules \
  --secret id=netrc,src=$HOME/.netrc -t shorty:private app
```

為什麼是 `.netrc`？`GOPRIVATE` 的模組 `go` 會直接用 `git` 走 https 去抓，而 `git`（透過 curl）會讀 `~/.netrc` 拿帳密；如果公司用私有 GOPROXY（Athens、Artifactory），`go` 對 proxy 的 https 請求也會讀 `.netrc`。一份檔案兩種情況都能用。

**不要**用 `git config url."https://${TOKEN}@github.com/".insteadOf ...` 這種寫法：`git config --global` 會把 token 寫進 `/root/.gitconfig`，那就留在 layer 裡了。

### 6. buildx：建立獨立 builder（看看就好）

```bash
docker buildx create --name ch11 --driver docker-container
docker buildx inspect ch11 --bootstrap
docker buildx ls
```

用它 build 時要加 `--load`，結果才會回到 `docker images` 看得到的 image store：

```bash
docker buildx build --builder ch11 --load \
  -f tutorials/11-buildkit/Dockerfile -t shorty:ch11 app
```

不同 builder 的快取是**分開的**——在 `ch11` 第一次 build 又是全部重下載。玩完刪掉：

```bash
docker buildx rm ch11
```

### 7. 看快取用了多少、清掉

```bash
docker buildx du          # 列出 build cache 每一筆
docker system df          # 總覽，看 "Build Cache" 那一行
```

範例輸出（`docker system df`）：

```
TYPE            TOTAL     ACTIVE    SIZE      RECLAIMABLE
Images          12        3         1.42GB    1.1GB (77%)
Containers      3         1         2.1kB     0B (0%)
Local Volumes   2         1         48.3MB    0B (0%)
Build Cache     41        0         612MB     612MB
```

清除：

```bash
docker builder prune                       # 清掉沒被引用的快取（會問你 y/N）
docker builder prune --filter until=24h    # 只清 24 小時以前的
docker builder prune -a                    # 全清（包含 cache mount），下次 build 從零開始
```

`docker builder prune` 是 `docker buildx prune` 的別名，作用在目前的 builder 上。

## 對照 Node／TypeScript

| Go + BuildKit | Node |
|---------------|------|
| `--mount=type=cache,target=/go/pkg/mod` | `--mount=type=cache,target=/root/.npm`，再跑 `npm ci` |
| `--mount=type=cache,target=/root/.cache/go-build` | 沒有直接對應；最接近的是 `tsc --incremental` 或 Next.js 的 `.next/cache` |
| `GOPRIVATE` + `.netrc` | 私有 registry 的 `.npmrc`（`//npm.pkg.github.com/:_authToken=...`） |
| `--mount=type=secret,id=netrc,target=/root/.netrc` | `--mount=type=secret,id=npmrc,target=/root/.npmrc` |

Node 版的典型寫法：

```dockerfile
# syntax=docker/dockerfile:1
FROM node:22-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN --mount=type=cache,target=/root/.npm \
    --mount=type=secret,id=npmrc,target=/root/.npmrc \
    npm ci
```

`docker build --secret id=npmrc,src=$HOME/.npmrc ...`。最常見的錯誤是 `COPY .npmrc .` 再 `RUN rm .npmrc`——刪掉也沒用，前一層裡還在。

## 常見陷阱

- **忘了第一行 `# syntax=`**：新版 Docker 內建的前端通常也看得懂 `--mount`，但有些語法（例如 secret 的 `env=`）需要較新的前端。寫上 `# syntax=docker/dockerfile:1` 才能確保大家用同一個版本。它必須是**檔案第一行**，前面不能有其他指令（註解也要在它之後）。
- **只在 `go mod download` 掛 `/go/pkg/mod`，`go build` 沒掛**：cache mount 的內容**不在 layer 裡**，下一個 `RUN` 看不到。結果 `go build` 發現模組快取是空的，默默把所有依賴再下載一次——build 會成功，只是快取白做了。每個要用模組的 `RUN` 都要掛同一個 target。
- **以為 cache mount 會被 CI 帶走**：`--cache-to type=gha` 或 `type=registry` 匯出的是 **layer 快取**，cache mount 的內容不會跟著走。CI 的每台新機器 cache mount 都是空的——第 24 章會處理。
- **非 root 使用者的 GOCACHE 路徑不同**：如果 build stage 用 `USER` 切成別的使用者，`GOCACHE` 會是 `/home/<user>/.cache/go-build`，`target` 要跟著改。可以 `RUN go env GOCACHE GOMODCACHE` 印出來確認。
- **平行 build 搶同一個快取**：預設 `sharing=shared`，多個 build 可以同時讀寫。Go 的快取本身能處理並行；apt 之類的工具不行，要用 `sharing=locked`。
- **secret 放在 build context 裡**：`src=./token.txt` 如果在 `app/` 底下，雖然 secret mount 沒問題，但 `COPY . .` 會把它一起複製進 image。秘密檔放 context 外面，或至少寫進 `.dockerignore`（第 10 章）。
- **secret 被印出來**：`RUN --mount=type=secret,... cat /run/secrets/token` 會把內容印在 `--progress=plain` 的輸出和 CI log 裡。只拿來用，不要 echo。
- **Windows／Docker Desktop**：
  - PowerShell 沒有 `export`，改用 `$env:DEMO_TOKEN = "s3cr3t-value"`；路徑可寫 `--secret id=netrc,src=$HOME\.netrc`。本系列建議用 Git Bash 或 WSL 跑指令。
  - Git Bash 的 `$HOME` 是 `/c/Users/<你>` 這種 POSIX 路徑，原生 Windows 版的 `docker.exe` 不一定認得。`src=` 給檔案路徑時改寫成 `src="$(cygpath -w ~/.netrc)"` 最保險；用 `env=` 來源就沒有這個問題。
  - `.netrc` 用 Windows 記事本存可能變成 CRLF 換行，git 一般能處理，但若認證怪怪的先確認換行字元。
  - Docker Desktop 的 build cache 存在它的 Linux VM 磁碟裡；`docker builder prune -a` 清完空間，VM 的虛擬磁碟檔不一定會馬上縮小。
- **`docker builder prune -a` 太順手**：會連 cache mount 一起清，下一次 build 又是從零下載。平常用 `--filter until=...` 就好。

## 練習

1. 用 `--progress=plain` 各 build 一次第 10 章版本（如果你有）與本章版本，改一行程式碼後再各 build 一次，比較 `go build` 那一步的秒數。
2. 把本章 `Dockerfile` 中 `go build` 那個 `RUN` 的 `--mount=type=cache,target=/go/pkg/mod` 拿掉，build 看看會發生什麼？從輸出解釋原因。
3. 用 `--secret id=demo_token,src=<某個檔案>` 改成檔案來源再跑一次 secret-demo，並用 `docker history --no-trunc` 確認 image 裡沒有內容。
4. 跑 `docker buildx du --verbose`，找出哪一筆是 `/go/pkg/mod` 的 cache mount（看 `Type` 和 `Description`）。
5. 思考題：為什麼 secret 的內容不參與快取計算是合理的？舉一個「反而需要它重跑」的情境，該怎麼處理？（提示：`--no-cache-filter` 或改一個 `ARG`）

## 下一章

image 好了，接下來是「怎麼跑才對」。第 12 章 [networking](../12-networking/) 把 shorty、Postgres、Redis 放進同一個自訂 network，用 container 名字互相連線，並搞清楚 `-p` 和 container 裡的 `localhost` 到底指誰。
