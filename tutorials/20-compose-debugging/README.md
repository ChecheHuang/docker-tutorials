# 20. Compose 除錯

> 範圍：`docker compose config`（看合併與替換結果）、`logs`、`exec`、`run --rm`、`events`、`ps --format`、`top`、除錯 distroless（netshoot sidecar）、常見錯誤對照表（port 被佔、DB 還沒好、volume 權限、忘了 `--build`、舊 volume 留著舊密碼、network 名稱衝突、Windows CRLF）、故意寫壞的 compose 練習

> 🧸 想先用故事直覺理解這章？看 [STORY.md](./STORY.md)（小學生都懂的比喻版）。

## 為什麼需要這個

到第 19 章，我們的 compose 已經有：基礎檔 + override + 多個 `-f`、`.env` 替換、profiles、healthcheck、depends_on。功能越多，「我以為的設定」和「Compose 實際用的設定」就越容易不一樣：

- `.env` 放錯位置，密碼其實用的是預設值。
- override 沒被讀到，跑的不是開發 image。
- 改了 Dockerfile，跑的卻是舊 image。
- shorty 啟動就 exit 1，log 一閃而過。

除錯的原則只有一個：**不要猜，去看**。這章把「去看」的工具整理成一套固定流程。

## 核心概念

### 除錯流程

```
1. config   → Compose 讀到的設定，跟我以為的一樣嗎？
2. ps -a    → 每個 service 現在是什麼狀態？exit code 多少？health 呢？
3. logs     → 掛掉的那個說了什麼？
4. events   → 剛剛發生了什麼事（重啟、health 變化、被 kill）？
5. exec/run → 進到網路裡面，親手連連看
```

大部分問題在第 1～3 步就會現形。

### 1. `docker compose config`：看 Compose 真正讀到什麼

```bash
docker compose -f <file> config               # 完整合併 + 變數替換後的結果
docker compose -f <file> config -q            # 只驗證，有錯才印
docker compose -f <file> config --services    # 列出 service（只含已啟用的 profile）
docker compose -f <file> config --volumes     # 列出 volume
docker compose -f <file> config --profiles    # 列出所有 profile
docker compose -f <file> config --no-interpolate   # 不做 ${...} 替換，看原始寫法
docker compose -f <file> config --format json      # JSON，方便丟給 jq
```

`config` 會告訴你：

- **哪些檔被合併了**（override 有沒有生效）
- **`${...}` 被換成什麼**（`.env` 有沒有讀到）；沒設的變數會出現 `The "X" variable is not set. Defaulting to a blank string.` 警告
- **相對路徑被展開成哪個絕對路徑**
- **YAML 語法、型別錯誤**：例如 list 裡混了 mapping、volume 沒宣告

**改完 compose 檔，先跑 `config` 再跑 `up`**，是最划算的習慣。

### 2. `ps`：狀態、exit code、health

```bash
docker compose ps -a                       # -a：連已經停掉的也列出來
docker compose ps --status exited          # 只看已結束的
docker compose ps --format "table {{.Service}}\t{{.State}}\t{{.Health}}\t{{.Status}}"
docker compose ps --format json            # 每個 container 一筆 JSON
```

範例輸出（`--format table ...`）：

```
SERVICE   STATE     HEALTH    STATUS
app       exited              Exited (1) 5 seconds ago
cache     running   healthy   Up 30 seconds (healthy)
db        running   healthy   Up 30 seconds (healthy)
```

exit code 的意思（第 04、15、16 章）：

| exit code | 常見原因 |
|------|------|
| 0 | 正常結束（shorty 收到 SIGTERM 後 graceful shutdown） |
| 1 | 程式自己報錯結束（shorty 的 `fatal`） |
| 137 | 被 SIGKILL：OOM（第 15 章）或 stop 超時（第 16 章） |
| 143 | 被 SIGTERM 結束、程式沒攔截 |

### 3. `logs`：看它說了什麼

```bash
docker compose logs app                 # 只看 app
docker compose logs -f --tail 50 app    # 最後 50 行，持續跟
docker compose logs --since 5m -t       # 最近 5 分鐘，加時間戳
docker compose logs --no-log-prefix app | jq .   # 拿掉 "app-1 |" 前綴，shorty 的 JSON log 直接丟給 jq
```

container **已經結束**也能看 log（只要還沒被 `down` 刪掉），這是 `logs` 比「前景跑 up」好用的地方。

### 4. `events`：剛剛發生了什麼

```bash
docker compose events           # 即時事件流：create、start、die、health_status、kill...
docker compose events --json
```

範例輸出：

```
2026-10-28 09:10:02.120000 container create 1a2b... (image=shorty:ch20, name=20-compose-debugging-app-1)
2026-10-28 09:10:02.480000 container start 1a2b... (image=shorty:ch20, name=20-compose-debugging-app-1)
2026-10-28 09:10:12.910000 container die 1a2b... (exitCode=1, image=shorty:ch20, name=20-compose-debugging-app-1)
```

適合抓「一直重啟」或「health 在 healthy／unhealthy 之間跳」這種一閃而過的問題。開一個視窗跑 `events`，另一個視窗重現問題。

### 5. `top`：container 裡跑了哪些 process

```bash
docker compose top
```

範例輸出：

```
20-compose-debugging-app-1
UID     PID     PPID    C   STIME   TTY   TIME       CMD
65532   51230   51208   0   09:10   ?     00:00:00   /shorty

20-compose-debugging-db-1
UID     PID     PPID    C   STIME   TTY   TIME       CMD
70      51010   50988   0   09:10   ?     00:00:00   postgres
70      51101   51010   0   09:10   ?     00:00:00   postgres: checkpointer
...
```

看得出 shorty 是 PID 1 的直接 process（exec form，第 16 章）、以 uid 65532（nonroot）執行。

### 6. `exec` 與 `run --rm`：進到網路裡親手試

```bash
# exec：在「正在跑」的 container 裡執行
docker compose exec db psql -U shorty -d shorty -c 'select count(*) from links'
docker compose exec cache redis-cli ping

# run --rm：用某個 service 的設定「另外」起一個一次性 container，跑完就刪
docker compose run --rm --no-deps cache redis-cli -h cache ping
docker compose run --rm --no-deps db psql "postgres://shorty:shorty@db:5432/shorty" -c '\dt'
```

| | `exec` | `run --rm` |
|------|------|------|
| container | 用已經在跑的那個 | 新建一個，結束就刪 |
| service 沒在跑時 | 不能用 | 可以用 |
| `ports` | — | 預設**不**發布（要 `--service-ports`），避免跟本尊搶 port |
| depends_on | — | 預設會一起啟動依賴，加 `--no-deps` 就不會 |
| 典型用途 | 進 DB 查資料 | 跑 migration、一次性腳本、用 service 的映像當工具 |

### 7. distroless 怎麼 exec？

```bash
docker compose exec app sh
```

範例輸出：

```
OCI runtime exec failed: exec failed: unable to start container process: exec: "sh": executable file not found in $PATH
```

shorty 的 image 沒有 shell（第 09、16 章），這是刻意的。要從 app 的角度看網路，就**另外帶一個工具箱 container 接到同一個網路**：

```bash
docker run --rm -it --network 20-compose-debugging_default nicolaka/netshoot
# 進去之後：
curl -s app:8080/readyz
dig db
nc -zv cache 6379
```

`nicolaka/netshoot` 內建 curl、dig、nc、tcpdump 等網路工具。網路名稱用 `docker network ls` 或 `docker compose config` 最後幾行查。進階一點可以用 `--network container:<app 的 container 名稱>` 共用 app 的 network namespace，連 `localhost` 都會指向 shorty。

## shorty 在這章的演進

shorty 本身和 compose 都不再改動。這章新增的是：

| 檔案 | 內容 |
|------|------|
| `tutorials/20-compose-debugging/broken.compose.yaml` | 故意寫壞的 compose，藏了 6 個 bug，用本章的工具一個一個抓出來 |

修好後的結果，等同第 18 章的 `compose.yaml`。

## 動手做

以下指令都從 repo 根目錄執行。

```bash
cp tutorials/20-compose-debugging/broken.compose.yaml tutorials/20-compose-debugging/fixed.compose.yaml
export COMPOSE_FILE=tutorials/20-compose-debugging/fixed.compose.yaml
```

之後都改 `fixed.compose.yaml`，`broken.compose.yaml` 留著對照。**先別看最後的解答**，照下面的流程走。

### 第一關：config

```bash
docker compose config -q
```

範例輸出：

```
WARN[0000] The "POSTGRES_USER" variable is not set. Defaulting to a blank string.
WARN[0000] The "POSTGRES_DB" variable is not set. Defaulting to a blank string.
validating ...fixed.compose.yaml: services.app.environment.[3]: unexpected type map[string]interface {}
```

這裡有**兩個**問題：一個警告、一個錯誤。錯誤會讓 Compose 直接拒絕執行；警告不會，但更危險，因為很容易被忽略。修好後再跑一次 `config -q`，會冒出下一個錯誤。重複到 `config -q` 沒有任何輸出為止。

### 第二關：up

```bash
docker compose up -d --build
```

如果 `up` 本身就失敗了，錯誤訊息會直接印出來。仔細看是**哪個 container** 的問題。

### 第三關：logs 與 ps

```bash
docker compose ps -a
docker compose logs app
```

shorty 如果 `Exited (1)`，log 裡會有 `postgres not ready, retrying` 重試 10 次的紀錄，最後一筆是 `fatal`。仔細看錯誤裡的**連線位址**。

### 第四關：curl

```bash
curl -i localhost:8080/readyz
```

如果所有 container 都是 running，curl 卻得到 `Empty reply from server` 或 `Connection reset by peer`，用 `docker compose ps` 看 PORTS 欄，再對照 shorty 的 log 裡 `listening` 那行的 `addr`。

### 收工

```bash
docker compose down -v
unset COMPOSE_FILE
```

## 常見錯誤對照表

| 症狀／錯誤訊息（大致） | 原因 | 怎麼查 | 怎麼修 |
|------|------|------|------|
| `Bind for 0.0.0.0:8080 failed: port is already allocated` | host 的 8080 被別的 container（例如上一章的 project 沒 down）或程式佔走 | `docker ps --filter publish=8080`；非 Docker 程式用 `lsof -i :8080`（macOS／Linux）、`netstat -ano \| findstr :8080`（Windows） | `down` 掉舊 project，或改 `ports` 左邊的 host port |
| Windows：`ports are not available ... An attempt was made to access a socket in a way forbidden by its access permissions` | 該 port 落在 Windows／Hyper-V 保留的範圍 | `netsh interface ipv4 show excludedportrange protocol=tcp` | 換一個不在保留範圍的 port |
| shorty log：`connect: connection refused`，然後 `fatal` | DB／Redis 還沒準備好，或主機名稱寫成 `localhost` | `logs app` 看連線位址；`config` 看 `depends_on` | 用 service 名稱（`db`）；`depends_on` 加 `condition: service_healthy` |
| `dependency failed to start: container ... has no healthcheck configured` | `condition: service_healthy` 的對象沒有 healthcheck | `config` 看該 service 有沒有 `healthcheck` | 補上 healthcheck，或改成 `service_started` |
| `dependency failed to start: container ... is unhealthy` | 對方的 healthcheck 一直失敗 | `docker inspect --format '{{json .State.Health}}' <container>` 看 `Output` | 修 healthcheck 指令或對方的設定 |
| 改了程式碼／Dockerfile，行為完全沒變；`/version` 還是舊的 | `up` 沒加 `--build`，用了已存在的舊 image | `docker compose images` 看 image 建立時間 | `up -d --build`，或 `docker compose build` 後再 up |
| shorty log：`password authentication failed for user "shorty"` | `db-data` volume 是用**舊密碼**初始化的，之後改 `.env` 不會生效 | `docker volume inspect <project>_db-data` 看建立時間 | 本機資料不重要：`down -v` 再 up；要保留：`docker compose exec db psql -U shorty -c "ALTER USER shorty PASSWORD '新密碼'"` |
| `permission denied` 寫入 volume／bind mount | container 內的使用者（shorty 是 uid 65532、postgres 是 uid 70）對掛載點沒有寫入權限 | `docker compose exec <svc> id`、`ls -ln` 看擁有者 | named volume：在 image 裡先建目錄並 `chown`（volume 第一次掛載會沿用 image 裡的擁有者）；bind mount：調整 host 目錄權限，或設 `user:` |
| `network X was found but has incorrect label com.docker.compose.network set to ""` | compose 想用的網路名稱，已經被手動 `docker network create` 建過（例如第 12 章的 `shorty-net`） | `docker network ls`、`docker network inspect X` | 刪掉舊網路；或在 compose 宣告 `external: true` 直接沿用 |
| 兩個專案互相把對方的 container 換掉或刪掉 | 兩個資料夾同名（都叫 `app/`），project name 一樣 | `docker compose ls` | 頂層寫 `name:`，或 `-p` 指定不同 project name |
| `Pool overlaps with other one on this address space` | 自訂 network 的 subnet 跟現有網路（或 VPN）重疊 | `docker network inspect` 看 subnet | 換 subnet，或清掉不用的網路 `docker network prune` |
| `exec /entrypoint.sh: no such file or directory`（檔案明明在） | Windows 上存檔成 CRLF，第一行變成 `#!/bin/sh\r`，找不到 `/bin/sh\r` 這個直譯器 | `file entrypoint.sh` 顯示 `with CRLF line terminators`；或 `cat -A` 看到 `^M` | `.gitattributes` 加 `*.sh text eol=lf`；編輯器改存 LF；或 `dos2unix` |
| `Found orphan containers ([...]) for this project` | compose 檔刪了某個 service，舊 container 還在 | `docker compose ps -a` | `up --remove-orphans` 或 `down --remove-orphans` |
| `no configuration file provided: not found` | 目前目錄沒有 `compose.yaml`，也沒 `-f` | `ls` | `cd` 到正確目錄、加 `-f`，或設 `COMPOSE_FILE` |
| override 好像沒生效 | 用了 `-f`，所以 `compose.override.yaml` 不會自動讀（第 19 章） | `config` 看結果 | 明確 `-f compose.yaml -f compose.override.yaml` |
| `.env` 的值沒被用到 | `.env` 不在 project directory（第 18 章） | `config` 看替換結果 | 搬到 compose 檔旁邊，或 `--env-file` |

shorty 的 app 沒有 entrypoint script，但只要你的專案有 `.sh` 給 container 執行，CRLF 遲早會遇到——在 Windows 上 clone 專案時 Git 的 `core.autocrlf=true` 就會悄悄轉換。

## 對照 Node／TypeScript

| Node 世界 | Compose 除錯 |
|------|------|
| `console.log(process.env)` 確認環境變數 | `docker compose config` 看替換結果；`docker compose exec <svc> env`（有 shell 的 image） |
| `node --inspect` + Chrome DevTools | Go 對應是 Delve（`dlv`），要在開發 image 裡跑並開 port；本系列不展開 |
| `npx kill-port 3000` | 先 `docker ps --filter publish=8080` 看是不是 container 佔的 |
| `EADDRINUSE` | `port is already allocated` |
| `ECONNREFUSED 127.0.0.1:5432` | 一模一樣的錯——在 container 裡 `localhost` 是自己 |
| `npm run migrate` | `docker compose run --rm <svc> <migration 指令>` |
| `node:alpine` image 可以 `docker exec -it ... sh` | distroless 不行，要帶 netshoot 之類的工具 container |

Node 開發者最熟悉的那句 `ECONNREFUSED 127.0.0.1:5432`，在 Compose 世界十之八九就是主機名稱寫成 `localhost`。

## 常見陷阱

1. **跳過 `config` 直接 `up`**：很多錯誤（型別、未宣告的 volume、沒設的變數）`config` 一秒就告訴你。
2. **忽略 `variable is not set` 警告**：它不會擋住執行，但代表某處被替換成空字串。
3. **前景 `up` 看 log，按 Ctrl+C 後 log 就沒了**：用 `up -d` + `logs -f`，container 結束後 log 仍查得到。
4. **用 `down` 清掉了證據**：`down` 會刪除 container 和它的 log。除錯時先 `ps -a`、`logs`，再 down。
5. **`run` 以為會發布 port**：`docker compose run` 預設不發布 `ports`，要加 `--service-ports`。
6. **在 distroless 裡找 shell**：找不到是正常的。用 netshoot 或改用開發 image（第 19 章）。
7. **以為 `down` 會清掉舊密碼**：不加 `-v`，volume 和裡面的舊密碼都還在。

## 練習

1. 照「動手做」的四關，把 `fixed.compose.yaml` 修到 `curl localhost:8080/readyz` 回 `{"cache":"ok","store":"ok"}`。
2. 修好之後，把 `POSTGRES_PASSWORD` 改成別的值（compose 裡兩處都改），**不加 `-v`** 地 `down` 再 `up`，重現 `password authentication failed`，然後用 `ALTER USER` 修好它。
3. 開兩個視窗：一個跑 `docker compose events`，另一個跑 `docker compose kill cache`，觀察事件順序，以及 shorty 的 `/readyz` 變成什麼。
4. 用 netshoot 接上 compose 網路，`dig db` 和 `dig app` 各回什麼 IP？跟 `docker network inspect 20-compose-debugging_default` 列出的 container IP 對得上嗎？
5. 在 Windows 上建一個 CRLF 的 `hello.sh`，用 `docker run --rm -v ${PWD}:/w alpine /w/hello.sh` 重現 `no such file or directory`，再用 `.gitattributes` 修好。

## 練習解答

`broken.compose.yaml` 的 6 個 bug，依「被發現的順序」排列：

| # | 哪裡 | 錯在哪 | 怎麼發現 | 修正 |
|------|------|------|------|------|
| 1 | `app.environment` | list 寫法裡混了一個 mapping：`- LOG_LEVEL: debug` | `config`：`services.app.environment.[3]: unexpected type ...` | 統一成 `- LOG_LEVEL=debug`（或整段改成 mapping 寫法） |
| 2 | `db.healthcheck.test` | `$POSTGRES_USER`、`$POSTGRES_DB` 被 Compose 在**讀檔時**用 host 的環境變數替換（多半是空的） | `config`：`The "POSTGRES_USER" variable is not set` 警告 | 寫成 `$$POSTGRES_USER`、`$$POSTGRES_DB`，留給 container 裡的 shell 展開；或直接寫死 `shorty` |
| 3 | `db.volumes` | 用了 `db-data`，但沒有頂層 `volumes:` 宣告 | `config`：`service "db" refers to undefined volume db-data: invalid compose project` | 檔案最後加上 `volumes:` / `  db-data:` |
| 4 | `app.depends_on.cache` | `condition: service_healthy`，但 `cache` 沒有 healthcheck | `up`：`dependency failed to start: container ...-cache-1 has no healthcheck configured`（措辭依版本略有不同） | 幫 `cache` 加 `healthcheck: test: ["CMD", "redis-cli", "ping"]` |
| 5 | `app.environment.DATABASE_URL` | 主機寫成 `localhost`，在 container 裡指的是 shorty 自己 | `logs app`：重試 10 次 `dial tcp 127.0.0.1:5432: connect: connection refused` 後 `fatal`；`ps -a` 看到 `Exited (1)` | 改成 `@db:5432` |
| 6 | `app.ports` | `"8080:80"`，但 shorty 在 container 裡聽的是 8080 | 全部 running，`curl` 卻 `Empty reply from server`；log 裡 `listening` 的 `addr` 是 `:8080` | 改成 `"8080:8080"` |

補充：

- **bug 2** 有點陰險：`pg_isready` 只檢查「能不能接受連線」，不驗證帳號，所以就算 `-U` 後面是空的，healthcheck 也**剛好會過**。這種「運氣好才正常」的 bug 最容易留到上線，所以 `variable is not set` 的警告一定要當錯誤處理。
- **bug 5** 被 shorty 的重試迴圈拖慢了大約 10 秒才現形。重試是好事，但除錯時要有耐心，或用 `logs -f` 盯著看。
- **bug 6** 也可以反過來修：設 `PORT: "80"` 讓 shorty 聽 80——但 nonroot 使用者在某些環境不能綁 1024 以下的 port（第 21 章），所以改 `ports` 比較好。

修好的關鍵片段：

```yaml
services:
  app:
    ports:
      - "8080:8080"                                                            # bug 6
    environment:
      - BASE_URL=http://localhost:8080
      - DATABASE_URL=postgres://shorty:shorty@db:5432/shorty?sslmode=disable   # bug 5
      - REDIS_URL=redis://cache:6379/0
      - LOG_LEVEL=debug                                                        # bug 1
  db:
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U $$POSTGRES_USER -d $$POSTGRES_DB"]    # bug 2
  cache:
    image: redis:7-alpine
    healthcheck:                                                               # bug 4
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 3s
      retries: 10

volumes:                                                                       # bug 3
  db-data:
```

修完後跟第 18 章的 `compose.yaml` 比對，除了密碼寫死、`environment` 用 list 寫法、image tag 不同之外，結構應該一樣。

## 下一章

Part 4 到這裡結束：shorty 已經能用 Compose 跑起完整的一套，也能開發、能除錯。[第 21 章 image-security](../21-image-security/) 進入 Part 5「上線」，回頭檢查 image 本身：non-root、`read_only`、`no-new-privileges`、cap drop、用 Trivy／Docker Scout 掃描漏洞——做完就是最終版的 `app/Dockerfile` 和根目錄 `compose.yaml` 裡那幾行安全設定。
