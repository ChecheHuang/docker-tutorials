# 17. Compose 基礎

> 範圍：為什麼要 Compose、`compose.yaml` 結構、services、build／image、ports、environment、變數替換入門、`up`／`down`／`ps`／`logs`／`build`、project name、`docker run` 參數對照

> 🧸 想先用故事直覺理解這章？看 [STORY.md](./STORY.md)（小學生都懂的比喻版）。

## 為什麼需要這個

回頭看第 12–16 章，跑一次 shorty 要打的指令越來越長：

```bash
docker build -t shorty:ch16 app
docker run -d --name shorty \
  -p 8080:8080 \
  -e BASE_URL=http://localhost:8080 \
  -e LOG_LEVEL=info \
  --stop-timeout 10 \
  shorty:ch16
```

這還只是 shorty 一個 container。問題是：

- **指令只存在你的終端機歷史裡**。新同事 clone 專案後，要從 README 或 Slack 裡拼湊出這串參數。
- **改一個參數要重打一整串**，還要先 `docker stop`、`docker rm` 舊的。
- **沒有「一整套」的概念**。DB、Redis、shorty 是一起的，但 Docker 只看到三個不相干的 container。

Docker Compose 的做法是：**把「要跑哪些 container、各自怎麼設定」寫成一個 YAML 檔**，進版控，然後用 `docker compose up` 讓 Docker 去「讓現實符合這份描述」。

> Compose v2 是 Docker CLI 的子指令 `docker compose`（中間空格）。舊的 Python 版 `docker-compose`（中間連字號）已經停止維護，本系列一律用 v2。

## 核心概念

### compose.yaml 的骨架

```yaml
services:        # 必填：要跑哪些 container
  app: { ... }
volumes:         # 選填：named volume（第 18 章）
networks:        # 選填：自訂網路
```

- 檔名預設是 `compose.yaml`（也接受 `compose.yml`、舊的 `docker-compose.yaml`／`.yml`）。
- **不要寫 `version:`**。它是 Compose 檔案格式 v2／v3 時代的遺物，現在的 Compose Specification 不需要，寫了只會出現 `the attribute 'version' is obsolete` 警告。

### 一個 service ≈ 一條 `docker run`

| `docker run` 參數 | `compose.yaml` |
|------|------|
| `docker build -t shorty:ch17 app` | `build: { context: ../../app }` + `image: shorty:ch17` |
| `--name shorty` | service 名稱 `app`（container 名稱由 Compose 產生） |
| `-p 8080:8080` | `ports: ["8080:8080"]` |
| `-e KEY=VALUE` | `environment: { KEY: VALUE }` |
| `--env-file .env.app` | `env_file: .env.app` |
| `-v data:/data` | `volumes: ["data:/data"]`（第 18 章） |
| `--network shorty-net` | 自動建立 `<project>_default` 網路 |
| `--restart unless-stopped` | `restart: unless-stopped` |
| `--memory 128m` | `deploy.resources.limits.memory: 128M` |
| `--stop-timeout 10` | `stop_grace_period: 10s` |
| `--init` | `init: true` |
| `--health-cmd ...` | `healthcheck: { test: [...] }` |

第 15 章的 restart policy 和記憶體限制，在 Compose 裡也是一行 key；本章為了讓檔案保持最小，先不放，最終版 `compose.yaml` 裡會看到它們。

### build 和 image 一起寫

```yaml
build:
  context: ../../app
image: shorty:ch17
```

- 只有 `build`：Compose 自己命名 image（`<project>-app`）。
- 只有 `image`：直接用（或去 registry pull）現成的 image，例如 `postgres:17-alpine`。
- **兩個都寫**：build 完用 `image` 的名字標記。之後要推上 registry（第 22 章）就很方便。

### 相對路徑是相對於 compose 檔

`context: ../../app` 是從 `tutorials/17-compose-basics/` 往上兩層找 `app/`，**跟你在哪個目錄下指令無關**。這也是本系列「章節 compose 檔放章節目錄，用 `../../app` 指回原始碼」的原因。

### project name

Compose 用 **project name** 把一組 container、network、volume 綁在一起，並當作名稱前綴：

| 資源 | 命名規則 | 本章範例 |
|------|------|------|
| container | `<project>-<service>-<編號>` | `17-compose-basics-app-1` |
| network | `<project>_default` | `17-compose-basics_default` |
| volume | `<project>_<volume>` | （第 18 章） |

project name 的決定順序（前面的優先）：

1. `docker compose -p <name>`
2. 環境變數 `COMPOSE_PROJECT_NAME`
3. compose 檔裡的頂層 `name:`
4. **第一個 compose 檔所在資料夾的名稱**（本章：`17-compose-basics`）

project name 只能有小寫字母、數字、`-`、`_`，且要以字母或數字開頭。兩個不同資料夾都叫 `app/`、各自有 `compose.yaml`，它們的 project name 都是 `app`，會互相踩到——第 20 章會再談。

### 變數替換入門

```yaml
LOG_LEVEL: ${LOG_LEVEL:-info}
```

Compose 在讀檔時，會把 `${VAR}` 換成**你下指令那個 shell 的環境變數**（或 `.env` 檔，第 18 章）。`:-info` 是「沒設或空字串就用 info」。所以：

```bash
LOG_LEVEL=debug docker compose -f tutorials/17-compose-basics/compose.yaml up -d
```

就能不改檔案切換 log 等級。注意這是**讀 YAML 時**做的文字替換，跟 container 裡的環境變數是兩回事。

### 宣告式：up 是「讓現實符合描述」

`docker compose up` 不是「開一個新 container」，而是比對 compose 檔和現況：

- 沒有 container → 建立
- 設定沒變 → 什麼都不做（`Running`）
- 設定變了（例如改了 `environment`）→ **砍掉重建**那個 container（`Recreated`）

所以改完 compose 檔，再跑一次 `up -d` 就好，不用手動 stop／rm。

## shorty 在這章的演進

| 項目 | 第 16 章 | 第 17 章 |
|------|------|------|
| 怎麼跑 | `docker build` + `docker run` 一長串 | `docker compose up -d --build` |
| 設定在哪 | 終端機歷史 | `tutorials/17-compose-basics/compose.yaml`（進版控） |
| Dockerfile | 章節版 | 直接用最終版 `app/Dockerfile`（build context `../../app`） |
| 資料 | in-memory | 仍是 in-memory（第 18 章接 Postgres + Redis） |

## 動手做

以下指令都從 repo 根目錄執行。每次都打 `-f` 很長，可以先設環境變數：

```bash
export COMPOSE_FILE=tutorials/17-compose-basics/compose.yaml   # PowerShell: $env:COMPOSE_FILE="..."
```

設了之後，下面的 `-f ...` 都可以省略。為了清楚，以下仍寫完整。

### 1. 先看 Compose 怎麼理解這個檔

```bash
docker compose -f tutorials/17-compose-basics/compose.yaml config
```

範例輸出（節錄）：

```yaml
name: 17-compose-basics
services:
  app:
    build:
      context: C:\work\alphacore\tutorials\docker-tutorials\app
      dockerfile: Dockerfile
    environment:
      BASE_URL: http://localhost:8080
      LOG_LEVEL: info
    image: shorty:ch17
    networks:
      default: null
    ports:
      - mode: ingress
        target: 8080
        published: "8080"
        protocol: tcp
    stop_grace_period: 10s
networks:
  default:
    name: 17-compose-basics_default
```

注意相對路徑已經被展開成絕對路徑、`${LOG_LEVEL:-info}` 變成 `info`、project name 是資料夾名稱。`config` 是 Compose 最好用的除錯指令，第 20 章會深入。

### 2. build 並啟動

```bash
docker compose -f tutorials/17-compose-basics/compose.yaml up -d --build
```

範例輸出（節錄）：

```
[+] Building 9.8s (17/17) FINISHED
 => [build 1/6] FROM docker.io/library/golang:1.26-alpine
 ...
 ✔ app  Built
[+] Running 2/2
 ✔ Network 17-compose-basics_default  Created
 ✔ Container 17-compose-basics-app-1  Started
```

- `-d`：背景執行（detached）。不加的話，log 會黏在前景，Ctrl+C 會停掉所有服務。
- `--build`：啟動前先 build。**沒加的話，image 已存在就直接用舊的**——改了程式碼卻沒效果，是 Compose 新手第一名的問題。

### 3. 看狀態

```bash
docker compose -f tutorials/17-compose-basics/compose.yaml ps
```

範例輸出：

```
NAME                      IMAGE         COMMAND     SERVICE   CREATED          STATUS          PORTS
17-compose-basics-app-1   shorty:ch17   "/shorty"   app       15 seconds ago   Up 14 seconds   0.0.0.0:8080->8080/tcp
```

`docker compose ps` 只列**這個 project** 的 container；`docker ps` 列全部。想看系統上有哪些 Compose project：

```bash
docker compose ls
```

範例輸出：

```
NAME                STATUS       CONFIG FILES
17-compose-basics   running(1)   C:\work\alphacore\tutorials\docker-tutorials\tutorials\17-compose-basics\compose.yaml
```

### 4. 試用並看 log

```bash
curl -XPOST localhost:8080/api/links -d '{"url":"https://go.dev"}'
docker compose -f tutorials/17-compose-basics/compose.yaml logs -f app
```

範例輸出：

```
app-1  | {"time":"2026-10-28T00:50:01.10Z","level":"WARN","msg":"DATABASE_URL not set, using in-memory store"}
app-1  | {"time":"2026-10-28T00:50:01.10Z","level":"WARN","msg":"REDIS_URL not set, cache disabled"}
app-1  | {"time":"2026-10-28T00:50:01.10Z","level":"INFO","msg":"listening","addr":":8080","version":"dev"}
app-1  | {"time":"2026-10-28T00:50:09.77Z","level":"INFO","msg":"request","method":"POST","path":"/api/links","dur_ms":0}
```

每行前面的 `app-1  |` 是 service 名稱 + 編號。多個服務時，log 會交錯顯示，靠這個前綴分辨。常用參數：`--tail 50`、`--since 10m`、`-t`（加時間戳）、`--no-log-prefix`。

### 5. 改設定，再 up 一次

```bash
LOG_LEVEL=debug docker compose -f tutorials/17-compose-basics/compose.yaml up -d
```

範例輸出：

```
[+] Running 1/1
 ✔ Container 17-compose-basics-app-1  Started
```

環境變數變了，Compose 會重建 container（看 `ps` 的 CREATED 時間會變新）。in-memory 的短網址也跟著消失了——這正是第 18 章要接 Postgres 的原因。

### 6. 只 build、不啟動

```bash
docker compose -f tutorials/17-compose-basics/compose.yaml build
docker compose -f tutorials/17-compose-basics/compose.yaml build --no-cache   # 完全不用快取
docker compose -f tutorials/17-compose-basics/compose.yaml images
```

### 7. 停止與清除

```bash
docker compose -f tutorials/17-compose-basics/compose.yaml stop    # 只停，container 留著
docker compose -f tutorials/17-compose-basics/compose.yaml start   # 再啟動
docker compose -f tutorials/17-compose-basics/compose.yaml down    # 停止並刪除 container 與 network
```

範例輸出（`down`）：

```
[+] Running 2/2
 ✔ Container 17-compose-basics-app-1  Removed
 ✔ Network 17-compose-basics_default  Removed
```

`down` 預設**不刪 image、不刪 named volume**。要連 build 出來的 image 一起刪：`down --rmi local`；連 volume 一起刪：`down -v`（第 18 章會很重要）。`stop`／`down` 都會照第 16 章的流程送 SIGTERM，等 `stop_grace_period` 再 SIGKILL。

### 8. 換個 project name 跑第二套

```bash
docker compose -f tutorials/17-compose-basics/compose.yaml -p shorty-b up -d
```

會失敗，因為 8080 已經被第一套佔走了（範例輸出：`Bind for 0.0.0.0:8080 failed: port is already allocated`）。先 `down` 第一套，或在練習裡改 port。

## 對照 Node／TypeScript

| Node 世界 | Compose 世界 |
|------|------|
| `package.json` 的 `scripts` 把長指令存起來 | `compose.yaml` 把 `docker run` 參數存起來 |
| `npm run dev` | `docker compose up` |
| `.env` + `dotenv` | Compose 的變數替換、`env_file`（第 18 章） |
| 專案根目錄放 `docker-compose.yml` 跑 Postgres、Redis，Node 本身在 host 上 `npm run dev` | 很常見的混合模式；本系列則是連 app 也放進 Compose |
| `concurrently "npm:api" "npm:worker"` 同時跑多個 process | 多個 service，log 用前綴分辨 |

很多 Node 專案的 `docker-compose.yml` 開頭還留著 `version: "3.8"`——那是舊格式的遺物，現在可以刪掉。

## 常見陷阱

1. **改了程式碼，`up -d` 沒有 `--build`**：image 已存在，Compose 不會自己重 build。養成 `up -d --build` 的習慣。
2. **`ports` 沒加引號**：YAML 1.1 會把 `22:22` 這種「冒號分隔、每段都小於 60」的數字當成六十進位（`22:22` → 1342）。`8080:8080` 剛好不會中，但一律寫成字串最安全。
3. **相對路徑以為是相對於目前目錄**：`context`、`env_file`、bind mount 都是相對於 compose 檔所在的資料夾（精確地說是 project directory，預設就是第一個 compose 檔的資料夾）。
4. **寫了 `container_name`**：寫死名稱後就不能用 `-p` 跑第二套，也不能 `--scale`。除非真的需要，不要寫。
5. **以為 `down` 會刪資料**：`down` 不刪 named volume；`down -v` 才會。反過來，忘記加 `-v` 而以為資料庫已經是乾淨的，也是常見的困惑（第 20 章）。
6. **前景 `up` 時按 Ctrl+C**：會停掉所有 service（但不刪除）。要背景跑記得 `-d`。
7. **`${VAR}` 以為是 container 裡的變數**：它是讀檔時用 host 的環境變數替換的。要在 container 裡展開，得寫 `$$VAR`（第 18 章的 healthcheck 會用到）。

## 練習

1. 把 `ports` 改成 `"9090:8080"`，`up -d` 後用 `curl localhost:9090/healthz` 驗證。`BASE_URL` 要不要跟著改？為什麼？
2. 在 `compose.yaml` 加上頂層 `name: shorty-dev`，再 `docker compose ... ps`，container 名稱變成什麼？舊的 `17-compose-basics-app-1` 跑去哪了？（提示：`docker ps`，以及 `--remove-orphans` 救不了它，因為它屬於另一個 project）
3. 用 `docker compose ... up -d --scale app=2` 試試看，會發生什麼錯？要怎麼改 `ports` 才能跑兩份？
4. 把第 16 章的 `HEALTHCHECK` 搬進 compose：在 `app` 底下加 `healthcheck`，`test` 要寫什麼？（提示：最終版 image 裡沒有 probe，要改用第 16 章的 Dockerfile，需要 `build.additional_contexts`）
5. 用 `docker compose ... logs --since 1m -t` 和 `docker logs 17-compose-basics-app-1` 比較輸出差異。

## 下一章

現在 compose 裡只有 shorty 一個服務，短網址一重啟就消失。[第 18 章 compose-multi-service](../18-compose-multi-service/) 加上 Postgres 和 Redis，用 `depends_on` + healthcheck 控制啟動順序、用 named volume 保存資料、用 `.env` 管理密碼，做出來的就是根目錄的 `compose.yaml`。
