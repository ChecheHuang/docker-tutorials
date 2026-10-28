# 06. Dockerfile 基本指令：shorty 第一版

> 範圍：Dockerfile 是什麼、`FROM`／`WORKDIR`／`COPY`／`RUN`／`CMD`／`EXPOSE`、`docker build -f ... -t ... <context>`、build time vs run time、shorty 第一版 image

> 🧸 想先用故事直覺理解這章？看 [STORY.md](./STORY.md)（小學生都懂的比喻版）。

## 為什麼需要這個

Part 1 我們跑的都是別人包好的 image：`nginx`、`alpine`、`postgres`。但你真正要上線的是**自己的程式**，Docker Hub 上不會有人幫你包 shorty。

你當然可以 `docker run -it golang:1.26 bash`，進去手動 `git clone`、`go build`，再 `docker commit` 存成 image。問題是：

- 下次要重做一遍，沒人記得當初打了哪些指令
- 同事做出來的 image 跟你的不一樣
- CI 沒辦法「手動進去打指令」

**Dockerfile 就是把「怎麼做出這個 image」寫成一份文字檔**：一行一個步驟、從上到下執行、可以進版控、誰來跑結果都一樣。這章先寫出 shorty 最直白的第一版 —— 能跑，但很胖，後面四章再一步一步改進。

## 核心概念

### Dockerfile 的六個基本指令

| 指令 | 什麼時候作用 | 做什麼 |
|------|------|------|
| `FROM <image>` | build | 以某個 image 當起點，每份 Dockerfile 都從它開始 |
| `WORKDIR <path>` | build + run | 切換（必要時建立）工作目錄，之後的指令與 container 啟動時都在這裡 |
| `COPY <src> <dest>` | build | 把 **build context** 裡的檔案複製進 image |
| `RUN <command>` | build | 在 build 過程中執行指令（編譯、安裝套件），結果存進 image |
| `CMD ["exe", "arg"]` | run | container 啟動時**預設**要執行的指令，`docker run` 後面接指令可以蓋掉它 |
| `EXPOSE <port>` | 文件 | 宣告服務聽哪個 port；**不會**真的對外開放，要靠 `docker run -p` |

最容易搞混的是 `RUN` 跟 `CMD`：

```
docker build  ──►  FROM → WORKDIR → COPY → RUN   （做 image，只跑一次）
docker run    ──►  CMD                          （每次起 container 都跑）
```

`RUN go build` 是「做菜」，`CMD ["shorty"]` 是「上菜時要做的第一件事」。

### build context：`docker build` 最後那個參數

```bash
docker build -f tutorials/06-dockerfile-basics/Dockerfile -t shorty:ch06 app
#            └─ 用哪份 Dockerfile ─────────────────────┘ └─ 名字:tag ┘ └─ context
```

- **`app`**（最後的位置參數）是 **build context**：Docker 會把這個資料夾的內容交給 builder，`COPY` 的來源路徑都是相對於它。
- **`-f`** 指定 Dockerfile 的位置。Dockerfile 不一定要放在 context 裡面 —— 這就是本系列「每章一份 Dockerfile、但 context 都是 `app/`」能成立的原因。
- **`-t`** 給 image 取名字和 tag。沒給的話只會得到一個 `<none>` 的 image ID。

所以 Dockerfile 裡的 `COPY . .` 意思是「把 context（`app/`）整包複製到 image 裡目前的 `WORKDIR`」。context 的細節（為什麼要小、`.dockerignore` 怎麼寫）留到第 10 章。

### exec form vs shell form（先記一句）

```dockerfile
CMD ["shorty"]        # exec form：直接執行 shorty  ← 本系列一律用這種
CMD shorty            # shell form：實際上是 /bin/sh -c "shorty"
```

兩種都跑得起來，但 shell form 會讓 `sh` 變成 PID 1，`docker stop` 送的 SIGTERM 到不了 shorty。為什麼、怎麼影響 graceful shutdown，第 16 章細講。現在先養成寫 JSON 陣列的習慣。

## shorty 在這章的演進

這是第一版，完整檔案在 [`Dockerfile`](./Dockerfile)：

```dockerfile
FROM golang:1.26
WORKDIR /src
COPY . .
RUN go build -o /usr/local/bin/shorty .
EXPOSE 8080
CMD ["shorty"]
```

逐行看：

1. **`FROM golang:1.26`**：官方 Go image，底下是完整的 Debian，加上 Go 1.26 工具鏈、git、gcc。要跟 `app/go.mod` 的 `go 1.26.1` 對得上（tag `1.26` 會追最新的 1.26.x patch）。
2. **`WORKDIR /src`**：之後都在 `/src` 做事。不存在會自動建立，不用 `RUN mkdir`。
3. **`COPY . .`**：把 `app/` 整包放進 `/src`。
4. **`RUN go build -o /usr/local/bin/shorty .`**：在 image 裡編譯。因為還沒下載依賴，`go build` 會順便把 pgx、go-redis 抓下來。執行檔放進 `/usr/local/bin`（已經在 `PATH` 裡），所以 `CMD` 寫 `shorty` 就找得到。
5. **`EXPOSE 8080`**：shorty 預設聽 `PORT=8080`，寫在這裡給人看、也給工具看。
6. **`CMD ["shorty"]`**：起 container 就執行 shorty。

跟上一章比：Part 1 用的都是別人的 image，這是我們**第一份自己的 Dockerfile**。

**image 大小（範例）**：

| image | 大小 |
|------|------|
| `golang:1.26`（base） | ~850 MB |
| `shorty:ch06` | **~1.1 GB** |

多出來的兩百多 MB 是原始碼、下載的 module、Go 的 build cache，再加上執行檔本身。真正上線需要的只有那個 ~18 MB 的執行檔，其他全是行李。這就是第 07、08 章要解決的問題。

## 動手做

以下指令都在 repo 根目錄執行。

### 1. build

```bash
docker build -f tutorials/06-dockerfile-basics/Dockerfile -t shorty:ch06 app
```

範例輸出（BuildKit 的格式，時間和 digest 每台不同）：

```
[+] Building 48.3s (9/9) FINISHED
 => [internal] load build definition from Dockerfile                0.0s
 => [internal] load metadata for docker.io/library/golang:1.26      1.2s
 => [internal] load .dockerignore                                   0.0s
 => [1/4] FROM docker.io/library/golang:1.26@sha256:...            25.1s
 => [internal] load build context                                   0.1s
 => => transferring context: 21.45kB                                0.0s
 => [2/4] WORKDIR /src                                              0.3s
 => [3/4] COPY . .                                                  0.1s
 => [4/4] RUN go build -o /usr/local/bin/shorty .                  19.8s
 => exporting to image                                              1.6s
 => => naming to docker.io/library/shorty:ch06                      0.0s
```

注意幾件事：

- `[1/4]`～`[4/4]` 對應 `FROM`、`WORKDIR`、`COPY`、`RUN`；`EXPOSE`、`CMD` 只改 metadata，不算步驟。
- `load .dockerignore`：`app/` 裡已經有一份 `.dockerignore`，它決定哪些檔案不送進 context，第 10 章解釋。
- `transferring context`：真的只送了 `app/` 的內容，二十幾 KB。

### 2. 看大小

```bash
docker images shorty
```

範例輸出：

```
REPOSITORY   TAG    IMAGE ID       CREATED          SIZE
shorty       ch06   3f2a9c1d8e7b   30 seconds ago   1.12GB
```

### 3. 跑起來

```bash
docker run --rm -p 8080:8080 --name shorty shorty:ch06
```

範例輸出（JSON log 寫到 stdout，第 05 章講過為什麼）：

```
{"time":"...","level":"WARN","msg":"DATABASE_URL not set, using in-memory store"}
{"time":"...","level":"WARN","msg":"REDIS_URL not set, cache disabled"}
{"time":"...","level":"INFO","msg":"listening","addr":":8080","version":"dev"}
```

開另一個終端機：

```bash
curl -XPOST localhost:8080/api/links -d '{"url":"https://go.dev"}'
curl -i localhost:8080/<上一步拿到的 code>
curl localhost:8080/version
```

範例輸出：

```
{"code":"aB3xYz","short_url":"http://localhost:8080/aB3xYz","url":"https://go.dev"}
HTTP/1.1 302 Found
Location: https://go.dev
...
{"version":"dev"}
```

`version` 是 `dev`，因為我們還沒在 build 時注入版本 —— 第 10 章處理。

### 4. 看看 image 裡裝了什麼

```bash
docker run --rm shorty:ch06 ls /src          # 原始碼都在 image 裡
docker run --rm shorty:ch06 go version       # 整套 Go 工具鏈也在
docker run --rm shorty:ch06 which gcc        # 連 C 編譯器都在
```

`docker run <image> <指令>` 後面接的指令會**蓋掉 `CMD`**，所以這三個都不會啟動 shorty，只是借這個 image 跑一下別的指令。範例輸出：

```
go.mod
go.sum
internal
main.go
go version go1.26.x linux/amd64
/usr/bin/gcc
```

上線的 container 裡放著原始碼、編譯器、C 工具鏈 —— 又大、又多了一堆攻擊面。

按 `Ctrl+C` 停掉 shorty（或在另一個終端機 `docker stop shorty`）。

## 對照 Node／TypeScript

Node 開發者寫的第一份 Dockerfile 通常長這樣，問題一模一樣：

```dockerfile
FROM node:24
WORKDIR /app
COPY . .
RUN npm install
EXPOSE 8080
CMD ["npm", "start"]
```

| 這一行 | Go 版 | Node 版的坑 |
|------|------|------|
| `FROM` | `golang:1.26` ~850MB | `node:24` 也是完整 Debian，~1GB 上下 |
| `COPY . .` | 原始碼進 image | **連本機的 `node_modules` 都 COPY 進去**，在 macOS／Windows 裝的 native module 到 Linux 會壞（第 10 章用 `.dockerignore` 擋） |
| `RUN` | `go build` | `npm install` 會改 lock 檔；上線該用 `npm ci`（第 07 章） |
| `CMD` | 直接跑執行檔 | `npm start` 讓 npm 當 PID 1，訊號轉發有問題；建議 `CMD ["node", "server.js"]`（第 16 章） |

最大的差別在後面：Go 編完只剩一個執行檔，runtime 什麼都不需要；Node 永遠需要 `node` 本體。這會讓第 08、09 章兩邊的 image 大小差很多。

## 常見陷阱

1. **context 給錯**：在 repo 根目錄打 `docker build -f tutorials/06-dockerfile-basics/Dockerfile .`（最後是 `.` 不是 `app`），`COPY . .` 就會把整個 repo 複製進去，`/src` 底下沒有 `go.mod`，`go build` 報 `go: go.mod file not found in current directory or any parent directory`。
2. **以為 `EXPOSE` 會開 port**：沒有 `-p 8080:8080`，主機連不到。反過來，沒寫 `EXPOSE` 用 `-p` 一樣能通。
3. **把啟動指令寫成 `RUN`**：`RUN shorty` 會在 build 時啟動伺服器，然後 build 卡住永遠不結束。啟動要用 `CMD`。
4. **`CMD` 用 shell form**：`CMD shorty` 能跑，但 `docker stop` 要等 10 秒才被強制殺掉（第 16 章）。
5. **服務只聽 `127.0.0.1`**：在 container 裡聽 localhost，`-p` 轉進來的連線會被拒絕。shorty 聽 `:8080`（所有介面）所以沒事，自己寫服務時要注意（第 12 章）。
6. **忘了 `-t`**：`docker images` 一堆 `<none>`，之後 `docker image prune` 清掉。
7. **`FROM golang:latest`**：今天是 1.26，明天可能就變 1.27，build 結果不可重現。至少釘到 minor 版本；釘 digest 在第 21 章。

## 練習

1. 照「動手做」build 並跑起 `shorty:ch06`，用 `curl` 建一個短網址並確認 302 轉址。
2. 用 `docker run --rm shorty:ch06 du -sh /go/pkg/mod /root/.cache/go-build` 看 module cache（官方 image 的 `GOPATH` 是 `/go`）和 build cache 各佔多少空間。
3. 把 `CMD ["shorty"]` 改成 `CMD shorty` build 一份 `shorty:ch06-shell`，各自 `docker run -d` 後 `time docker stop`，比較停下來花多久。（做完記得改回來。）
4. 不改 Dockerfile，用 `docker run -e PORT=9090 -p 9090:9090 shorty:ch06` 讓 shorty 聽別的 port。想想看：`EXPOSE 8080` 這時候還對嗎？
5. 故意用錯 context（陷阱 1），看看錯誤訊息長什麼樣子。

## 下一章

改一行 `main.go` 再 build 一次，你會發現 `go build` 又重新下載了一次所有依賴。[第 07 章 layers-cache](../07-layers-cache) 解釋 image 是一層一層疊起來的、快取什麼時候會失效，並把依賴下載拆成獨立的一層。
