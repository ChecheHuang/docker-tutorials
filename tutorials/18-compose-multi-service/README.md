# 18. Compose 多服務：App + Postgres + Redis

> 範圍：多個 service、compose 預設網路與 service 名稱 DNS、`depends_on` 與 `condition: service_healthy`、service 的 `healthcheck`（`CMD` vs `CMD-SHELL`）、named volume、`.env` 變數替換（`${VAR:-default}`）、`.env.example`、`up --wait`、`down -v`

> 🧸 想先用故事直覺理解這章？看 [STORY.md](./STORY.md)（小學生都懂的比喻版）。

## 為什麼需要這個

第 17 章的 shorty 是 in-memory，重啟就忘光。要讓它變成真正的服務，得接上 Postgres（存短網址）和 Redis（快取轉址）。第 12、13 章我們手動做過：

```bash
docker network create shorty-net
docker volume create shorty-db
docker run -d --name db --network shorty-net -v shorty-db:/var/lib/postgresql/data \
  -e POSTGRES_USER=shorty -e POSTGRES_PASSWORD=shorty -e POSTGRES_DB=shorty postgres:17-alpine
docker run -d --name cache --network shorty-net redis:7-alpine
# ……然後等 DB 好了，才能跑 shorty
docker run -d --name shorty --network shorty-net -p 8080:8080 \
  -e DATABASE_URL=postgres://shorty:shorty@db:5432/shorty?sslmode=disable \
  -e REDIS_URL=redis://cache:6379/0 shorty
```

痛點不只是長，而是**順序**：shorty 啟動時 Postgres 可能還在初始化，Redis 也可能還沒 listen。手動跑時你會「等一下再跑」，自動化時誰來等？

這章把三個服務寫進同一個 `compose.yaml`，讓 Compose 負責網路、volume、啟動順序與「等它真的好了」。

## 核心概念

### 預設網路與 service 名稱 DNS

Compose 會為每個 project 自動建立一個 `<project>_default` bridge 網路，所有 service 都接上去，而且**service 名稱就是 DNS 名稱**。所以：

```yaml
DATABASE_URL: postgres://shorty:...@db:5432/shorty
REDIS_URL: redis://cache:6379/0
```

`db`、`cache` 就是下面兩個 service 的名稱。這就是第 12 章「自訂 network 的內建 DNS」，只是 Compose 幫你建好了。

`db` 和 `cache` **沒有寫 `ports`**：它們只需要被 compose 網路裡的 shorty 連到，不需要對 host 開放。少開一個 port，就少一個被掃描的機會。

### depends_on：三種等待條件

```yaml
depends_on:
  db:
    condition: service_healthy
```

| condition | 等到什麼時候 | 適合 |
|------|------|------|
| `service_started`（短寫法 `depends_on: [db]` 的預設） | container **啟動**了 | 對方啟動很快、或你的程式自己會重試 |
| `service_healthy` | 對方的 **healthcheck 通過** | DB、cache 這類要初始化的服務 |
| `service_completed_successfully` | 對方**跑完並 exit 0** | 一次性任務，例如 migration |

「container 啟動了」和「Postgres 能接受連線了」差很多：postgres image 第一次啟動要 `initdb`、建使用者、建資料庫，可能要好幾秒。`service_started` 只保證前者。

`depends_on` 還有兩個選項：`restart: true`（被依賴的服務被 Compose 重啟時，連帶重啟自己）、`required: false`（對方不存在或沒啟動也不報錯）。本章用不到，知道有就好。

> 注意：`depends_on` 只管 **Compose 啟動時的順序**。服務跑起來之後 DB 掛了、重開了，`depends_on` 不會幫你做任何事——那是程式自己要處理的（重連、`/readyz` 回 503）。

### 雙保險：depends_on + 程式內重試

shorty 的 `main.go` 裡有一段：

```go
// compose 剛起來時 DB 可能還沒好，重試幾次（第 18 章）
for i := 1; i <= 10; i++ {
    pg, err := store.NewPostgres(ctx, dsn)
    ...
}
```

為什麼有了 `service_healthy` 還要重試？因為**上線環境不一定有 Compose**。K8s 沒有 `depends_on`，pod 啟動順序不保證。程式自己會重試，才是可攜的；`depends_on` 是讓本機開發更順。

反過來看 Redis：`openCache` **沒有重試**，連不上就直接 `fatal` 結束。所以 `cache` 的 `condition: service_healthy` 在這裡不是錦上添花，而是必要。

### service 的 healthcheck

```yaml
healthcheck:
  test: ["CMD-SHELL", "pg_isready -U shorty -d shorty"]
  interval: 5s
  timeout: 3s
  retries: 10
```

跟第 16 章的 Dockerfile `HEALTHCHECK` 是同一個機制，只是寫在 compose 檔裡（compose 的設定會覆蓋 image 裡的）。`test` 的寫法：

| 寫法 | 意思 |
|------|------|
| `["CMD", "redis-cli", "ping"]` | 直接執行，不經過 shell（exec form） |
| `["CMD-SHELL", "pg_isready -U shorty"]` | 交給 container 裡的 `/bin/sh -c`，可以用 `&&`、`||`、環境變數 |
| `"pg_isready -U shorty"`（字串） | 等同 `CMD-SHELL` |
| `["NONE"]` 或 `disable: true` | 關掉 image 內建的 healthcheck |

`pg_isready` 是 postgres image 內建的工具，專門回答「現在能連嗎」；`redis-cli ping` 回 `PONG` 就 exit 0。兩個 image 都是 alpine 版，有 shell，所以不會遇到第 16 章 distroless 的問題。

另外有 `start_period` 和 `start_interval`，用法同第 16 章。

### named volume

```yaml
services:
  db:
    volumes:
      - db-data:/var/lib/postgresql/data
volumes:
  db-data:
```

頂層 `volumes:` **宣告**，service 底下**使用**。兩邊都要寫，少了頂層宣告會報錯（第 20 章的除錯練習就有這題）。實際的 volume 名稱是 `<project>_db-data`。

| 指令 | volume 怎麼樣 |
|------|------|
| `docker compose down` | **保留**，下次 up 資料還在 |
| `docker compose down -v` | **刪除**，DB 從頭初始化 |

> postgres 18 之後的官方 image 把資料目錄改到 `/var/lib/postgresql/18/docker`，建議的掛載點變成 `/var/lib/postgresql`。本系列用 `postgres:17-alpine`，掛 `/var/lib/postgresql/data` 是對的；升級大版本時要一起改，而且大版本之間的資料目錄不能直接沿用（要 `pg_dump`／`pg_upgrade`）。

### .env 與變數替換

```yaml
POSTGRES_PASSWORD: ${POSTGRES_PASSWORD:-shorty}
```

Compose 讀檔時，會從兩個地方找 `POSTGRES_PASSWORD`：

1. 你下指令那個 shell 的環境變數（優先）
2. **project directory 裡的 `.env` 檔**（預設就是 compose 檔所在的資料夾）

都沒有，就用 `:-` 後面的預設值 `shorty`。常用語法：

| 語法 | 沒設時 | 設成空字串時 |
|------|------|------|
| `${VAR}` | 空字串 + 警告 | 空字串 |
| `${VAR:-default}` | `default` | `default` |
| `${VAR-default}` | `default` | 空字串 |
| `${VAR:?請設定 VAR}` | **報錯停止** | 報錯停止 |
| `$$` | 一個字面上的 `$`（不替換） | |

慣例是：`.env.example` 進版控（列出有哪些變數、給安全的範例值），`.env` 自己複製一份、填真的值、**不進版控**（本 repo 的 `.gitignore` 已經有 `.env`）。

`.env` 檔 ≠ container 的環境變數。`.env` 只用來替換 compose 檔裡的 `${...}`；要傳進 container，得在 `environment:` 引用，或用 `env_file:`。秘密管理的完整討論在第 14 章，image 安全在第 21 章。

### 結果就是根目錄的 compose.yaml

把本章的檔案和根目錄 `compose.yaml` 擺在一起比，差別只有三處：

| | 本章 | 根目錄（最終版） |
|------|------|------|
| `build.context` | `../../app` | `./app` |
| `restart: unless-stopped`、`deploy.resources.limits.memory` | 沒有 | 有（第 15 章的內容） |
| `read_only: true`、`security_opt: [no-new-privileges:true]` | 沒有 | 有（第 21 章） |

## shorty 在這章的演進

| 項目 | 第 17 章 | 第 18 章 |
|------|------|------|
| service | `app` | `app` + `db`（postgres:17-alpine）+ `cache`（redis:7-alpine） |
| 資料 | in-memory，重啟就沒 | Postgres + named volume `db-data` |
| 快取 | 無（`REDIS_URL` 沒設） | Redis |
| 啟動順序 | — | `depends_on` + `condition: service_healthy` |
| 設定 | `${LOG_LEVEL:-info}` | 加上 `.env`／`.env.example`：`POSTGRES_PASSWORD`、`VERSION` |
| image tag | `shorty:ch17` | `shorty:${VERSION:-dev}`，跟根目錄一致 |

## 動手做

以下指令都從 repo 根目錄執行。先設好 `COMPOSE_FILE` 省得每次打 `-f`：

```bash
export COMPOSE_FILE=tutorials/18-compose-multi-service/compose.yaml
```

### 1. 準備 .env（選用）

```bash
cp tutorials/18-compose-multi-service/.env.example tutorials/18-compose-multi-service/.env
```

不複製也能跑，所有變數都有 `:-` 預設值。複製了就會用 `.env` 裡的 `change-me` 當密碼。確認替換結果：

```bash
docker compose config | grep -E 'POSTGRES_PASSWORD|DATABASE_URL'
```

範例輸出：

```
      DATABASE_URL: postgres://shorty:change-me@db:5432/shorty?sslmode=disable
      POSTGRES_PASSWORD: change-me
```

### 2. 啟動並等到全部 healthy

```bash
docker compose up -d --build --wait
```

範例輸出（節錄）：

```
 ✔ app  Built
[+] Running 5/5
 ✔ Network 18-compose-multi-service_default    Created
 ✔ Volume "18-compose-multi-service_db-data"   Created
 ✔ Container 18-compose-multi-service-cache-1  Healthy
 ✔ Container 18-compose-multi-service-db-1     Healthy
 ✔ Container 18-compose-multi-service-app-1    Started
```

`--wait` 會等所有 service 都 running／healthy 才回到提示字元，很適合寫在腳本或 CI 裡。shorty 沒有 healthcheck，所以它「running」就算數。

觀察啟動順序：`app` 一定在 `db`、`cache` 變 healthy **之後**才 `Started`。

### 3. 看狀態

```bash
docker compose ps
```

範例輸出：

```
NAME                               IMAGE                COMMAND                  SERVICE   CREATED          STATUS                    PORTS
18-compose-multi-service-app-1     shorty:dev           "/shorty"                app       20 seconds ago   Up 8 seconds              0.0.0.0:8080->8080/tcp
18-compose-multi-service-cache-1   redis:7-alpine       "docker-entrypoint.s…"   cache     20 seconds ago   Up 19 seconds (healthy)   6379/tcp
18-compose-multi-service-db-1      postgres:17-alpine   "docker-entrypoint.s…"   db        20 seconds ago   Up 19 seconds (healthy)   5432/tcp
```

`db`、`cache` 的 PORTS 欄只有 `5432/tcp`、`6379/tcp`，沒有 `0.0.0.0:...->`，代表沒對 host 開放。

### 4. 試用並看 log

```bash
curl -XPOST localhost:8080/api/links -d '{"url":"https://go.dev"}'
curl localhost:8080/readyz
docker compose logs app
```

範例輸出：

```
{"code":"k3Hx9Qa","short_url":"http://localhost:8080/k3Hx9Qa","url":"https://go.dev"}
{"cache":"ok","store":"ok"}
app-1  | {"time":"2026-10-28T01:00:08.20Z","level":"INFO","msg":"connected to postgres"}
app-1  | {"time":"2026-10-28T01:00:08.21Z","level":"INFO","msg":"connected to redis"}
app-1  | {"time":"2026-10-28T01:00:08.21Z","level":"INFO","msg":"listening","addr":":8080","version":"dev"}
```

`connected to postgres` 出現在第一次嘗試就成功——因為 `service_healthy` 已經幫我們等過了。

### 5. 驗證資料活過 down

```bash
docker compose down
docker volume ls --filter name=db-data
docker compose up -d --wait
curl -i localhost:8080/k3Hx9Qa      # 換成你自己的 code
```

範例輸出：

```
DRIVER    VOLUME NAME
local     18-compose-multi-service_db-data

HTTP/1.1 302 Found
Location: https://go.dev
```

`down` 刪了 container 和網路，但 volume 還在，短網址還能轉址。

### 6. 拿掉 condition 看看會怎樣

把 `app` 的 `depends_on` 暫時改成短寫法：

```yaml
depends_on: [db, cache]
```

然後清掉資料重來（讓 Postgres 需要完整初始化）：

```bash
docker compose down -v
docker compose up -d
docker compose logs app
```

範例輸出：

```
app-1  | {"time":"...","level":"WARN","msg":"postgres not ready, retrying","attempt":1,"err":"ping postgres: failed to connect to `user=shorty database=shorty`: ... dial tcp ...:5432: connect: connection refused"}
app-1  | {"time":"...","level":"WARN","msg":"postgres not ready, retrying","attempt":2,"err":"..."}
app-1  | {"time":"...","level":"INFO","msg":"connected to postgres"}
```

shorty 的重試迴圈救了它。但如果 Redis 剛好慢了一步，你會看到 `fatal` 然後 container 直接 `Exited (1)`——沒有 restart policy 的話就停在那裡。實驗完記得改回 `condition: service_healthy`。

### 7. 進 DB 看資料

```bash
docker compose exec db psql -U shorty -d shorty -c '\dt'
docker compose exec cache redis-cli keys '*'
```

`exec` 在已經在跑的 container 裡執行指令（第 05 章），對應到 service 名稱，不用查 container ID。第 20 章會整理更多除錯指令。

### 8. 清掉

```bash
docker compose down -v     # 連 volume 一起刪；只想停掉就不要加 -v
unset COMPOSE_FILE
```

## 對照 Node／TypeScript

| Node 世界 | Compose 世界 |
|------|------|
| `dotenv` 在程式啟動時讀 `.env` 進 `process.env` | Compose 在**讀 YAML 時**用 `.env` 替換 `${...}`，程式本身不讀 `.env` |
| `.env.example` 進 git、`.env` 進 `.gitignore` | 完全一樣的慣例 |
| Prisma／TypeORM 連線字串 `postgresql://user:pass@localhost:5432/db` | 在 Compose 裡主機名稱是 service 名稱 `db`，不是 `localhost` |
| `wait-on tcp:5432`、`wait-for-it.sh` 等 DB | `depends_on` + `condition: service_healthy`（不用額外腳本） |
| `prisma migrate deploy` 在啟動前跑 | 可以做成一個 `migrate` service，app 用 `condition: service_completed_successfully` 等它 |
| 很多 Node 專案只把 DB 放進 Compose，Node 在 host 跑 | 這時 Node 要連 `localhost:5432`，db 就**必須**加 `ports: ["5432:5432"]` |

Node 開發者最常卡的是最後一行：在 host 跑 `npm run dev` 時連 `localhost`，把 app 也搬進 Compose 後要改成 service 名稱。

## 常見陷阱

1. **`depends_on` 短寫法以為會等 DB 好**：它只等 container 啟動。要等健康就用 `condition: service_healthy`，而且對方**一定要有 healthcheck**，否則 Compose 會報錯。
2. **在 container 裡連 `localhost`**：`localhost` 是 container 自己（第 12 章）。要連別的 service，用 service 名稱。
3. **改了 `.env` 的 `POSTGRES_PASSWORD`，DB 卻還是舊密碼**：postgres image 只在**資料目錄是空的**時候用這些變數初始化。volume 已經有資料就不會再動。shorty 會連不上（`password authentication failed`），詳見第 20 章。
4. **`.env` 放錯位置**：它要在 project directory（compose 檔旁邊）。用 `-f tutorials/18-.../compose.yaml` 從 repo 根目錄執行時，讀的是 `tutorials/18-compose-multi-service/.env`，不是根目錄的 `.env`。不確定就用 `docker compose config` 看替換結果，或明確指定 `--env-file`。
5. **healthcheck 裡想用 container 的環境變數**：`pg_isready -U $POSTGRES_USER` 會被 Compose 在讀檔時替換成 host 的值（多半是空的）。要留給 container 的 shell 展開，得寫 `$$POSTGRES_USER`。
6. **密碼有特殊字元**：`DATABASE_URL` 是 URL，密碼裡的 `@`、`:`、`/`、`#` 要 URL-encode，不然解析會錯。
7. **把 DB 的 port 開到 host 卻用弱密碼**：`ports: ["5432:5432"]` 等於對整個網路開放（預設綁 `0.0.0.0`）。真的要開，至少寫 `"127.0.0.1:5432:5432"`。
8. **`down -v` 手滑**：它會刪掉 named volume，本機 DB 資料全沒。習慣上只在「確定要從頭來」時才加。

## 練習

1. 在 `.env` 把 `LOG_LEVEL` 改成 `debug`，不用 `--build` 重跑 `up -d`，哪些 container 被重建了？為什麼 `db`、`cache` 沒動？
2. 用 `${POSTGRES_PASSWORD:?請在 .env 設定 POSTGRES_PASSWORD}` 取代預設值，刪掉 `.env` 後跑 `docker compose config`，看看錯誤訊息。
3. 為 `db` 加上 `ports: ["127.0.0.1:5432:5432"]`，從 host 用 `psql` 或 GUI 工具連進去（第 19 章會改用 profiles 起 pgAdmin）。
4. 設計一個 `migrate` service（不用真的實作 migration，`command` 先用 `echo migrated` 代替），讓 `app` 用 `condition: service_completed_successfully` 等它。
5. 執行 `diff tutorials/18-compose-multi-service/compose.yaml compose.yaml`，確認差異跟本章「結果就是根目錄的 compose.yaml」那張表一致。

## 下一章

現在的 compose 很適合「跑起來用」，但開發時每改一行 Go 就要 `up --build` 重 build image，很慢。[第 19 章 compose-dev-workflow](../19-compose-dev-workflow/) 用 `compose.override.yaml` + air 熱重載、Compose Watch、profiles 和多個 `-f` 檔，把同一份 compose 變成好用的開發環境。
