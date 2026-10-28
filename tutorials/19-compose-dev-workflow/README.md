# 19. Compose 開發流程

> 範圍：`compose.override.yaml` 自動合併與合併規則、開發用 `Dockerfile.dev`、bind mount + air 熱重載、Compose Watch（`develop.watch`：`sync`／`rebuild`／`sync+restart`）、profiles（`--profile tools` 起 pgAdmin）、多個 `-f` 檔切換環境

> 🧸 想先用故事直覺理解這章？看 [STORY.md](./STORY.md)（小學生都懂的比喻版）。

## 為什麼需要這個

第 18 章的 compose 已經能一鍵跑起整套服務，但它是「上線的樣子」：每改一行 Go，都要

```bash
docker compose up -d --build
```

等 BuildKit 重跑 build stage、產出新 image、換掉 container。就算有 cache mount，也要十幾秒，而且每次都要切回終端機打指令。

另一方面，我們不想為了開發另外維護一份完全不同的 compose——那樣兩份會越長越不一樣，最後「開發環境能跑、上線不能跑」。

這章的目標：**同一份基礎 compose，用疊加的方式切換成開發模式**，並且存檔就自動重載。

## 核心概念

### compose.override.yaml：自動合併

在同一個目錄、**不加 `-f`** 執行 `docker compose`，Compose 會依序讀：

1. `compose.yaml`
2. `compose.override.yaml`（如果存在）

並把第二個**疊**在第一個上面。慣例是：`compose.yaml` 寫上線也成立的設定，`compose.override.yaml` 寫只有本機開發才要的東西（bind mount、debug log、開發用 image）。

> **一旦用了 `-f`，override 就不會自動讀。** 這是本章的指令都要 `cd` 到章節目錄、不從 repo 根目錄 `-f` 的原因，也是後面「多環境檔」能運作的原因。

### 合併規則

| 欄位類型 | 例子 | 怎麼合併 |
|------|------|------|
| 單一值 | `image`、`command`、`build.dockerfile` | 後面的**覆蓋**前面的 |
| mapping | `environment`、`labels` | 依 key 合併，同 key 後面的覆蓋 |
| `volumes`、`devices` | | 依 **container 內的路徑**合併 |
| `ports`、`expose`、`dns` 這類 sequence | | **合併**（不會覆蓋） |

`ports` 會合併這點很容易踩：override 裡再寫一次 `"8080:8080"`，等於宣告兩次。所以本章的 `compose.override.yaml` 不碰 `ports`。要整個換掉某個欄位，可以用 `!override` 或 `!reset` 這兩個 YAML tag（例如 `ports: !reset []`）。

不確定合併結果時，永遠先看：

```bash
docker compose config
```

### 開發用 image：Dockerfile.dev

上線的 `app/Dockerfile` 是 distroless，只有一個編好的執行檔，裡面沒有 Go、沒有 shell，不可能在 container 裡重編。開發時需要的是另一種 image：

| | `app/Dockerfile`（上線） | `Dockerfile.dev`（開發） |
|------|------|------|
| base | `gcr.io/distroless/static-debian12:nonroot` | `golang:1.26-alpine` |
| 原始碼 | build 時 COPY，編完丟掉 | **不 COPY**，執行時 bind mount |
| 執行 | `/shorty` | `air`：監看 → 重編 → 重啟 |
| 大小 | 約 20MB | 數百 MB |

`Dockerfile.dev` 放在章節目錄，build context 仍是 `app/`。compose 裡的 `dockerfile:` 是**相對於 context** 的路徑，所以寫成 `../tutorials/19-compose-dev-workflow/Dockerfile.dev`。

### air 熱重載

[air](https://github.com/air-verse/air) 是 Go 版的 nodemon：

```
存檔 → air 偵測到 .go 變更 → go build -o /tmp/air/shorty . → 送 SIGINT 給舊的 shorty → 啟動新的
```

本章的 `.air.toml` 幾個重點：

- `tmp_dir = "/tmp/air"`：編譯產物放 container 內，不寫回 bind mount，host 的 `app/` 不會多出 `tmp/`。
- `send_interrupt = true` + `kill_delay = "2s"`：先 SIGINT 讓 shorty 走第 16 章的 graceful shutdown。
- `poll = true`：Docker Desktop（Windows、部分 macOS 設定）的 bind mount 常收不到檔案變更事件（inotify），改用輪詢。Linux 原生 Docker 可改回 `false`。
- 設定檔放章節目錄，用 bind mount 唯讀掛到 `/etc/air/air.toml`，遵守「`app/` 不改」的約定。

另外兩個 named volume `go-mod`、`go-build` 是 Go 的模組與編譯快取：container 重建後不用重新下載依賴、重新編譯整個標準庫。

### Compose Watch：develop.watch

Docker Compose 2.22 起內建 **Compose Watch**，由 Compose **在 host 上**監看檔案，不依賴 container 裡的工具，也沒有 bind mount 的檔案事件問題：

```yaml
services:
  app:
    develop:
      watch:
        - action: rebuild
          path: ../../app
          ignore:
            - "**/*_test.go"
```

| action | 做什麼 | 適合 |
|------|------|------|
| `sync` | 把變更的檔案複製到 container 的 `target` 路徑 | 直譯語言、或 container 裡有熱重載工具 |
| `rebuild` | 重新 build image，換掉 container | 編譯型語言、改了依賴 |
| `sync+restart` | 複製檔案後重啟 container | 改設定檔 |

shorty 的上線 image 是 distroless 的單一執行檔，所以 `compose.watch.yaml` 只用 `rebuild`。如果想讓 watch 搭配 air（不用 bind mount），可以這樣寫：

```yaml
services:
  app:
    develop:
      watch:
        - action: sync
          path: ../../app
          target: /src
        - action: rebuild
          path: ../../app/go.mod   # 依賴變了才重 build image
```

`sync` 需要 container 裡有基本的檔案工具、使用者對 `target` 有寫入權限，所以只能用在 `Dockerfile.dev` 這種 image，不能用在 distroless。另外，同一個路徑**不要**同時 bind mount 又 sync。

| | bind mount + air | Compose Watch `rebuild` |
|------|------|------|
| 改完到生效 | 1–3 秒（只重編） | 十幾秒（重 build image） |
| 跑的是什麼 | 開發 image | **跟上線一樣的 image** |
| Windows 檔案事件 | 要開 polling | 沒問題（host 端監看） |
| 需要 | `Dockerfile.dev`、`.air.toml` | 只要加 `develop.watch` |

兩者不是互斥的：日常改程式用 air，送出 PR 前用 watch 或 `-f compose.yaml` 跑一次上線 image 確認。

### profiles：預設不啟動的服務

```yaml
pgadmin:
  image: dpage/pgadmin4:9
  profiles: [tools]
```

有 `profiles` 的 service **預設不啟動**。要用時：

```bash
docker compose --profile tools up -d    # 或 COMPOSE_PROFILES=tools docker compose up -d
docker compose up -d pgadmin            # 直接指名 service，也會啟動它
```

適合放 pgAdmin、Adminer（`adminer:6`）、跑 migration 或壓測的一次性工具：大家都需要偶爾用，但不想每次 `up` 都跑。沒有 `profiles` 的 service 永遠會啟動。

### 多個 -f：切換環境

```bash
docker compose -f compose.yaml -f compose.prod.yaml up -d
```

後面的檔案疊在前面的上面，規則跟 override 一樣。本章準備了：

| 指令 | 讀哪些檔 | 用途 |
|------|------|------|
| `docker compose up` | `compose.yaml` + `compose.override.yaml` | 日常開發（air） |
| `docker compose -f compose.yaml up` | 只有 `compose.yaml` | 跟第 18 章一樣 |
| `docker compose -f compose.yaml -f compose.watch.yaml up --watch` | 基礎 + watch | 用上線 image 開發 |
| `docker compose -f compose.yaml -f compose.prod.yaml up` | 基礎 + prod | 類上線：restart、記憶體限制、`LOG_LEVEL=warn` |

也可以用 `COMPOSE_FILE=compose.yaml:compose.prod.yaml`（Windows 用 `;` 分隔）省掉 `-f`。另一種做法是頂層 `include:` 引入其他 compose 檔，適合把大型專案拆成多個子專案，本系列不展開。

## shorty 在這章的演進

| 項目 | 第 18 章 | 第 19 章 |
|------|------|------|
| 基礎 compose | app + db + cache | 相同，再加 `pgadmin`（`profiles: [tools]`） |
| 開發模式 | 每次 `up --build` | `compose.override.yaml`：`Dockerfile.dev` + air + bind mount |
| 另一種開發模式 | — | `compose.watch.yaml`：`develop.watch` + `rebuild` |
| 類上線 | — | `compose.prod.yaml`：加回第 15 章的 restart 與 memory limit |
| `app/` | 不變 | 不變（air 設定放章節目錄） |

本章檔案：

```
tutorials/19-compose-dev-workflow/
├── compose.yaml            # 基礎（= 第 18 章 + pgadmin）
├── compose.override.yaml   # 開發：air + bind mount（自動合併）
├── compose.watch.yaml      # Compose Watch 版
├── compose.prod.yaml       # 類上線
├── Dockerfile.dev          # golang + air
└── .air.toml               # air 設定
```

## 動手做

**本章例外：以下指令都在章節目錄執行**（override 自動合併需要這樣）。

```bash
cd tutorials/19-compose-dev-workflow
```

### 1. 先看合併結果

```bash
docker compose config --services
docker compose config | sed -n '/^  app:/,/^  cache:/p'
```

範例輸出（節錄）：

```
app
cache
db
```

```yaml
  app:
    build:
      context: C:\work\alphacore\tutorials\docker-tutorials\app
      dockerfile: ../tutorials/19-compose-dev-workflow/Dockerfile.dev
      args:
        VERSION: dev
    environment:
      LOG_LEVEL: debug
      ...
    image: shorty:dev-air
    volumes:
      - type: bind
        source: C:\work\alphacore\tutorials\docker-tutorials\app
        target: /src
      ...
```

`--services` 沒列出 `pgadmin`（profile 沒啟用）。`LOG_LEVEL` 已經被 override 蓋成 `debug`，`image` 也換了。

### 2. 用 air 啟動開發環境

```bash
docker compose up -d --build
docker compose logs -f app
```

範例輸出：

```
app-1  |   __    _   ___
app-1  |  / /\  | | | |_)
app-1  | /_/--\ |_| |_| \_ v1.67.4, built with Go go1.26.x
app-1  |
app-1  | watching .
app-1  | watching internal
app-1  | watching internal/cache
app-1  | watching internal/server
app-1  | watching internal/store
app-1  | building...
app-1  | running...
app-1  | {"time":"...","level":"INFO","msg":"connected to postgres"}
app-1  | {"time":"...","level":"INFO","msg":"connected to redis"}
app-1  | {"time":"...","level":"INFO","msg":"listening","addr":":8080","version":"dev"}
```

### 3. 改程式，看它自動重載

在 host 上（另一個視窗、你的編輯器）改 `app/internal/server/server.go` 的 `healthz`，例如把 `"ok"` 改成 `"ok!"`，存檔。log 會出現：

範例輸出：

```
app-1  | internal/server/server.go has changed
app-1  | building...
app-1  | {"time":"...","level":"INFO","msg":"shutdown signal received, draining"}
app-1  | {"time":"...","level":"INFO","msg":"bye"}
app-1  | running...
app-1  | {"time":"...","level":"INFO","msg":"listening","addr":":8080","version":"dev"}
```

```bash
curl localhost:8080/healthz
```

範例輸出：

```
{"status":"ok!"}
```

注意 `shutdown signal received` → `bye`：air 送的 SIGINT 讓 shorty 走了 graceful shutdown。**實驗完記得把 `server.go` 改回來**——本系列約定 `app/` 不改。

### 4. 開 pgAdmin

```bash
docker compose --profile tools up -d
docker compose --profile tools ps
```

瀏覽器開 <http://localhost:5050>，用 `dev@example.com`／`shorty` 登入，新增 server：Host 填 **`db`**（service 名稱，不是 localhost）、使用者 `shorty`、密碼 `shorty`。pgAdmin 跟 db 在同一個 compose 網路，所以用 service 名稱連。

停掉時也要帶 profile，否則 `down` 不會處理 pgadmin：

```bash
docker compose --profile tools down
```

### 5. 換成 Compose Watch

```bash
docker compose -f compose.yaml -f compose.watch.yaml up --build --watch
```

範例輸出（節錄，改了一個 .go 檔之後）：

```
         ⦿ Watch enabled
...
         ⦿ Rebuilding service(s) ["app"] after changes were detected...
 ✔ app  Built
         ⦿ service(s) ["app"] successfully built
app-1  | {"time":"...","level":"INFO","msg":"listening","addr":":8080","version":"dev"}
```

這次跑的是**上線的 distroless image**（`image: shorty:dev`），每次變更都完整 rebuild。按 Ctrl+C 結束 watch（container 也會停）。

### 6. 類上線模式

```bash
docker compose -f compose.yaml -f compose.prod.yaml config | grep -E 'restart|memory|LOG_LEVEL'
docker compose -f compose.yaml -f compose.prod.yaml up -d --build
```

範例輸出：

```
      LOG_LEVEL: warn
    restart: unless-stopped
          memory: "134217728"
    restart: unless-stopped
    restart: unless-stopped
```

（`config` 會把 `128M` 正規化成 bytes。）

### 7. 收工

```bash
docker compose --profile tools down
cd ../..
```

`go-mod`、`go-build` 兩個快取 volume 留著，下次開發更快；要清掉就加 `-v`（會連 DB 資料一起清）。

## 對照 Node／TypeScript

| Node 世界 | Go + Compose 世界 |
|------|------|
| `nodemon`／`tsx watch`／`node --watch` | **air** |
| `nodemon.json` | `.air.toml` |
| `docker-compose.yml` + `docker-compose.override.yml` 把 `./src` 掛進 container、跑 `npm run dev` | 完全一樣的模式 |
| 經典問題：bind mount 蓋掉 container 裡的 `node_modules` → 加一個匿名 volume `- /app/node_modules` | Go 沒有這個問題：依賴在 `/go/pkg/mod`，不在原始碼目錄裡 |
| `CHOKIDAR_USEPOLLING=true`（Windows bind mount 收不到事件） | air 的 `poll = true` |
| Vite／Next.js dev server 的 HMR | Go 沒有 HMR，air 是「重編 + 重啟」，但 Go 編譯快，體感差不多 |
| `develop.watch` + `action: sync` 同步 `./src`，`rebuild` 監看 `package.json` | Docker 官方範例就是 Node；Go 因為要編譯，多半用 `rebuild` |

Node 專案用 Compose Watch 的典型寫法：`sync` 同步原始碼（dev server 自己熱重載），`rebuild` 監看 `package.json`（依賴變了才重 build image）。這跟上面「sync + air」的寫法是同一個思路。

## 常見陷阱

1. **從 repo 根目錄 `-f` 執行，以為 override 有生效**：用了 `-f` 就不會自動讀 `compose.override.yaml`。先 `docker compose config` 確認。
2. **override 裡又寫一次 ports**：`ports` 是合併不是覆蓋，會變成重複宣告。要換掉用 `!override`。
3. **`dockerfile:` 路徑以為是相對於 compose 檔**：它是相對於 `build.context`。
4. **改了檔案 air 沒反應**：Docker Desktop 的 bind mount 檔案事件不可靠，確認 `poll = true`；或改用 Compose Watch。
5. **air 編譯產物寫進 bind mount**：預設 `tmp_dir = "tmp"` 會在 host 的 `app/` 生出 `tmp/`，在 Linux 上還是 root 擁有、刪不掉。本章改放 `/tmp/air`。
6. **開發 image 被拿去上線**：`Dockerfile.dev` 有完整工具鏈、跑 root、數百 MB。用不同的 image 名稱（`shorty:dev-air`）避免混淆，CI 只 build `app/Dockerfile`（第 24 章）。
7. **`down` 沒帶 `--profile`**：依 Compose 版本不同，profile 裡的 service 不一定會被一起停掉、刪掉。啟動時帶了什麼 `--profile`，停止時也帶上，最不會出錯。
8. **bind mount 和 `sync` 同時用在同一個路徑**：兩邊互相覆蓋，行為難以預測。二選一。
9. **Windows 上 bind mount 很慢**：repo 放在 `C:\` 走的是跨檔案系統掛載（第 13 章）。放在 WSL2 的 Linux 檔案系統（例如 `~/work`）裡會快很多，也不需要 polling。

## 練習

1. 在 `compose.override.yaml` 裡把 db 的 port 開到 `127.0.0.1:5432`，讓 host 上的 GUI 工具連得到；確認 `-f compose.yaml` 模式下不會開。
2. 把 pgAdmin 換成 Adminer（`adminer:6`，container port 8080，用 `ADMINER_DEFAULT_SERVER: db` 預設連 db），一樣放在 `tools` profile。
3. 用 `ports: !override` 在 override 裡把 app 改成 `"9090:8080"`，用 `docker compose config` 驗證只剩一組 port。
4. 寫一個 `compose.ci.yaml`：app 不開 port、`LOG_LEVEL=debug`，搭配 `up --wait` 和 `curl` 做冒煙測試。指令要怎麼下？
5. 改用上面 README 裡「`sync` + air」的 watch 寫法（拿掉 bind mount），比較跟 bind mount 版的重載速度。

## 下一章

檔案一多、疊一多，就會開始出現「明明有設，為什麼沒生效」的問題。[第 20 章 compose-debugging](../20-compose-debugging/) 整理 `config`、`logs`、`exec`、`run --rm`、`events` 等除錯工具，附一份常見錯誤對照表，最後給你一個故意寫壞的 compose 檔來練習。
