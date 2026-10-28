# 07. Layer 與 build 快取

> 範圍：image 是一疊唯讀 layer、哪些指令會產生 layer、`docker history`、build cache 的命中與失效規則、指令順序（不常變的放前面）、依賴層拆分（`COPY go.mod go.sum` + `go mod download`）、`--no-cache`／`--pull`

> 🧸 想先用故事直覺理解這章？看 [STORY.md](./STORY.md)（小學生都懂的比喻版）。

## 為什麼需要這個

第 06 章的 shorty 能跑了。現在試著改一行 `main.go` 再 build：`go build` 又從頭把 pgx、go-redis 全部下載一次。依賴根本沒變，卻每次都要等。

在自己電腦上多等 20 秒還能忍；在 CI 上一天跑一百次、或依賴多到要下載幾分鐘的專案（Node 的 `node_modules`、Rust 的 crate），就是真金白銀的時間。

原因出在 Dockerfile 的**指令順序**。只要理解 image 是「一層一層疊起來」的、Docker 怎麼判斷「這層可以沿用」，改兩行 Dockerfile 就能讓「只改程式碼」的 build 跳過依賴下載。

## 核心概念

### image 是一疊唯讀的 layer

Dockerfile 裡每個改動檔案系統的指令（`RUN`、`COPY`、`ADD`）都會產生一個新的 **layer**，記錄「跟上一層比，多了／改了／刪了哪些檔案」。其他指令（`WORKDIR`、`ENV`、`EXPOSE`、`CMD`）只改 image 的設定（metadata），大小是 0。

```
┌──────────────────────────────┐  CMD ["shorty"]         (metadata)
├──────────────────────────────┤  RUN go build           +215 MB
├──────────────────────────────┤  COPY . .               +21 KB
├──────────────────────────────┤  RUN go mod download    +45 MB
├──────────────────────────────┤  COPY go.mod go.sum     +4 KB
├──────────────────────────────┤  WORKDIR /src
├══════════════════════════════┤
│  golang:1.26 的 layer 們      │  ~850 MB
└──────────────────────────────┘
          （大小為範例）
```

container 跑起來時，Docker 在最上面再疊一層可寫的薄層；底下的 layer 全部唯讀、而且可以被多個 image 共用 —— 十個以 `golang:1.26` 為底的 image，硬碟上只存一份 base。

**layer 只會往上加**：在後面的 `RUN rm -rf /go/pkg/mod` 不會讓 image 變小，檔案還躺在下面那層，只是被「標記刪除」。想真的變小要換做法（第 08 章 multi-stage）。

### build cache：沒變就沿用

`docker build` 時，builder 一步一步往下走，每一步先問：「我以前做過**一模一樣**的這一步嗎？」做過就直接拿出結果（輸出裡顯示 `CACHED`），沒做過才真的執行。

「一模一樣」的判斷規則：

| 指令 | 什麼情況會失效 |
|------|------|
| 任何指令 | **上一層失效了** —— 一層 miss，之後全部重做 |
| `RUN` | 指令**字串**變了（連多一個空白都算）。注意：只比字串，不比執行結果 |
| `COPY`／`ADD` | 被複製的檔案**內容**（或權限等屬性）變了。只是 `touch` 改修改時間不會失效 |
| `ARG`／`ENV` | 用到的值變了（第 10 章） |
| `FROM` | 本機已有這個 tag 就沿用，不會自動檢查遠端有沒有更新；要更新用 `--pull` |

第一條是關鍵：**快取是一條鏈，斷在哪裡，後面就全部重來**。

### 所以：不常變的放前面

第 06 章的順序是：

```dockerfile
COPY . .                  # ← 任何一個 .go 檔變了，這層就失效
RUN go build ...          # ← 跟著重做：重新下載依賴 + 重新編譯
```

依賴下載被綁在「原始碼有沒有變」這個條件上，而原始碼幾乎每次 build 都會變。

解法是把「依賴清單」和「原始碼」拆開：

```dockerfile
COPY go.mod go.sum ./     # ① 只有改依賴時才變
RUN go mod download       # ② 跟著 ① 走：依賴沒變就 CACHED
COPY . .                  # ③ 原始碼變了從這裡開始重做
RUN go build ...
```

`go.mod`／`go.sum` 只有在 `go get`、升級套件時才會變，所以 ② 大部分時候都直接沿用。

### `docker history`：看 image 有哪些 layer

```bash
docker history shorty:ch07
```

每一行是一層，最上面是最新的。`SIZE` 那欄告訴你哪一步最胖，`CREATED BY` 告訴你是哪個指令做的。`docker history` 看得到每個 `RUN` 的完整指令 —— 這也是為什麼**秘密不能寫在 Dockerfile 裡**（第 14 章）。

## shorty 在這章的演進

完整檔案在 [`Dockerfile`](./Dockerfile)：

```dockerfile
FROM golang:1.26
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o /usr/local/bin/shorty .

EXPOSE 8080
CMD ["shorty"]
```

跟第 06 章的差異：

```diff
 FROM golang:1.26
 WORKDIR /src
+
+COPY go.mod go.sum ./
+RUN go mod download
+
 COPY . .
 RUN go build -o /usr/local/bin/shorty .
```

**image 大小（範例）**：

| image | 大小 | 說明 |
|------|------|------|
| `shorty:ch06` | ~1.7 GB | |
| `shorty:ch07` | ~1.7 GB | 幾乎一樣 —— 這章改的是 **build 速度**，不是大小 |

依賴從 `RUN go build` 那層搬到了 `RUN go mod download` 那層，總量沒變。瘦身是下一章的事。

## 動手做

以下指令都在 repo 根目錄執行。

### 1. 兩版都先 build 一次

```bash
docker build -f tutorials/06-dockerfile-basics/Dockerfile -t shorty:ch06 app
docker build -f tutorials/07-layers-cache/Dockerfile      -t shorty:ch07 app
```

### 2. 什麼都不改，再 build 一次

```bash
docker build -f tutorials/07-layers-cache/Dockerfile -t shorty:ch07 app
```

範例輸出 —— 每一步都 `CACHED`，一秒內結束：

```
 => CACHED [2/6] WORKDIR /src
 => CACHED [3/6] COPY go.mod go.sum ./
 => CACHED [4/6] RUN go mod download
 => CACHED [5/6] COPY . .
 => CACHED [6/6] RUN go build -o /usr/local/bin/shorty .
```

### 3. 模擬「改了程式碼」

加一個不影響功能的 Go 檔（`app/` 的程式碼各章都不改，做完記得刪掉）：

```bash
echo 'package main' > app/cachebust.go
```

再 build 兩版：

```bash
docker build -f tutorials/06-dockerfile-basics/Dockerfile -t shorty:ch06 app
docker build -f tutorials/07-layers-cache/Dockerfile      -t shorty:ch07 app
```

範例輸出（ch06）—— `COPY . .` 失效，`go build` 得重新下載所有依賴：

```
 => CACHED [2/4] WORKDIR /src
 => [3/4] COPY . .                                                  0.1s
 => [4/4] RUN go build -o /usr/local/bin/shorty .                  21.4s
```

範例輸出（ch07）—— 依賴層還在，只重新編譯：

```
 => CACHED [2/6] WORKDIR /src
 => CACHED [3/6] COPY go.mod go.sum ./
 => CACHED [4/6] RUN go mod download
 => [5/6] COPY . .                                                  0.1s
 => [6/6] RUN go build -o /usr/local/bin/shorty .                   9.7s
```

ch07 省掉的就是下載時間。網路越慢、依賴越多，差距越大。

收尾：

```bash
rm app/cachebust.go
```

> 為什麼 ch07 的 `go build` 還要 9 秒？因為 Go 的編譯快取（`/root/.cache/go-build`）也在被重做的那層裡，跟著一起作廢了。第 11 章的 `RUN --mount=type=cache` 會把它留下來。

### 4. 看 layer

```bash
docker history shorty:ch07
```

範例輸出：

```
IMAGE          CREATED         CREATED BY                                      SIZE      COMMENT
5d1e0b7c9a42   1 minute ago    CMD ["shorty"]                                  0B        buildkit.dockerfile.v0
<missing>      1 minute ago    EXPOSE map[8080/tcp:{}]                         0B        buildkit.dockerfile.v0
<missing>      1 minute ago    RUN /bin/sh -c go build -o /usr/local/bin/sh…   168MB     buildkit.dockerfile.v0
<missing>      1 minute ago    COPY . . # buildkit                             21.5kB    buildkit.dockerfile.v0
<missing>      5 minutes ago   RUN /bin/sh -c go mod download # buildkit       47.3MB    buildkit.dockerfile.v0
<missing>      5 minutes ago   COPY go.mod go.sum ./ # buildkit                3.92kB    buildkit.dockerfile.v0
<missing>      5 minutes ago   WORKDIR /src                                    0B        buildkit.dockerfile.v0
<missing>      2 weeks ago     ...（以下是 golang:1.26 的 layer）
```

- `<missing>` 不是壞掉，只是那些中間層沒有自己的 image ID。
- 注意 `CREATED` 欄：依賴層是 5 分鐘前做的（被沿用），上面幾層是剛剛重做的。
- 加 `--no-trunc` 看完整指令。

### 5. 強制不用快取

```bash
docker build --no-cache -f tutorials/07-layers-cache/Dockerfile -t shorty:ch07 app   # 每步都重做
docker build --pull     -f tutorials/07-layers-cache/Dockerfile -t shorty:ch07 app   # 先檢查 golang:1.26 有沒有新版
```

## 對照 Node／TypeScript

Node 的版本更經典，因為 `npm ci` 動輒一兩分鐘：

```dockerfile
FROM node:24-slim
WORKDIR /app
COPY package.json package-lock.json ./   # ① 依賴清單
RUN npm ci                               # ② 依賴沒變就 CACHED
COPY . .                                 # ③ 原始碼
RUN npm run build
CMD ["node", "dist/server.js"]
```

| Go | Node |
|------|------|
| `go.mod` + `go.sum` | `package.json` + `package-lock.json` |
| `go mod download` | `npm ci` |
| 依賴在 `/go/pkg/mod` | 依賴在 `./node_modules` |

兩個 Node 特有的重點：

- **用 `npm ci` 不要用 `npm install`**：`npm ci` 嚴格照 lock 檔裝、不會改 lock 檔，結果可重現；lock 跟 `package.json` 對不上時直接報錯，而不是偷偷更新。
- **只 `COPY package.json` 不 `COPY` lock 檔** 是常見錯誤：快取照樣運作，但每次裝的版本可能不一樣。
- **本機的 `node_modules` 會被 `COPY . .` 蓋過去**：不只是快取問題，還會把 macOS／Windows 編的 native module 帶進 Linux。第 10 章用 `.dockerignore` 擋掉。

## 常見陷阱

1. **`COPY . .` 放在依賴下載之前**：本章要解決的問題本身。任何檔案一變，依賴就重抓。
2. **`RUN apt-get update` 單獨一行**：

   ```dockerfile
   RUN apt-get update                  # ← 被快取住，永遠是第一次 build 那天的套件清單
   RUN apt-get install -y curl         # ← 某天加了新套件，用的卻是過期清單，可能 404
   ```

   要寫在同一個 `RUN` 裡：`RUN apt-get update && apt-get install -y ...`。原因就是「`RUN` 只比字串不比結果」。同理，`RUN curl https://.../latest.tar.gz` 也會被快取住，不會自己抓新版。
3. **以為刪檔案能讓 image 變小**：`RUN go build ... ` 之後再 `RUN rm -rf /go/pkg/mod`，下面那層還在，大小不變。要嘛在同一個 `RUN` 裡刪，要嘛用 multi-stage（第 08 章）。
4. **`.git` 跟著進 context**：每次 commit 都改到 `.git/` 裡的檔案，`COPY . .` 永遠失效。shorty 的 build context 是 `app/`，而且 `app/.dockerignore` 已經排除 `.git`（第 10 章）。
5. **CI 上每次都沒快取**：CI runner 通常是乾淨的機器，本機的快取帶不過去。要用 registry cache 或 GitHub Actions cache（第 24 章）。
6. **改註解會不會失效？** Dockerfile 裡的 `#` 註解不是指令，改了不影響快取；但 `RUN` 指令裡多一個空白、換一行反斜線位置，字串就不同了，會失效。

## 練習

1. 照「動手做」第 3 步，實際量一下 ch06 和 ch07 在「改了程式碼」時的 build 時間差。
2. 改 `app/go.mod`（例如在檔尾加一行空行）再 build ch07，觀察從哪一步開始失效。做完改回來。
3. 用 `docker history --no-trunc shorty:ch07` 找出最大的一層是哪個指令，想想為什麼它這麼大。
4. 在 ch07 的 Dockerfile 最後加一行 `RUN rm -rf /go/pkg/mod /root/.cache/go-build`，build 後比較 `docker images` 的大小。有變小嗎？為什麼？
5. 用 `touch app/main.go`（只改修改時間、不改內容）再 build，看 `COPY . .` 有沒有失效。

## 下一章

build 變快了，但 image 還是 1.1GB，裡面躺著 Go 工具鏈、原始碼、module cache。[第 08 章 multi-stage](../08-multi-stage) 用兩個 `FROM` 把「編譯的地方」和「執行的地方」分開，image 直接降到幾十 MB，並對照 Rust 和 Node 的做法。
