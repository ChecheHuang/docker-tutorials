# 14. 設定與機密

> 範圍：12-factor config、`-e`／`--env-file`、不要把秘密 bake 進 image（`docker history` 看得到）、Docker secrets 檔案掛載（`/run/secrets`、`_FILE` 慣例）、shorty 的環境變數表

> 🧸 想先用故事直覺理解這章？看 [STORY.md](./STORY.md)（小學生都懂的比喻版）。

## 為什麼需要這個

同一個 shorty image，要在你的筆電、CI、staging、production 上跑。四個地方的 DB 位址、密碼、log 等級都不一樣。

有兩種做法：

1. 每個環境 build 一個 image（`shorty:staging`、`shorty:prod`），設定寫在 image 裡。
2. **只 build 一個 image**，設定在啟動時從外面給。

第 1 種看起來直覺，但問題很多：staging 測過的 image 跟上 production 的不是同一個；改一個 log 等級要重 build；更糟的是密碼跟著 image 推上 registry，任何能 `docker pull` 的人都拿得到。

業界的共識是第 2 種，也就是 [The Twelve-Factor App](https://12factor.net/config) 的第三條：**設定存在環境裡，不存在程式碼（或 image）裡**。shorty 從第一天就照這個規則寫：所有設定都是環境變數。

## 核心概念

### 設定的三個來源與優先順序

```
             優先順序低 ────────────────────────────▶ 高
Dockerfile 的 ENV  ─▶  --env-file 檔案  ─▶  -e KEY=VALUE
（image 裡的預設值）    （一批設定）          （單一覆寫）
```

- `ENV` 寫在 Dockerfile，適合放**不敏感的預設值**（例如 `ENV PORT=8080`）。
- `docker run` 給的值會蓋掉 image 的 `ENV`。
- 同一個 key 不要在 `--env-file` 和 `-e` 都給，看設定的人會搞不清楚哪個生效。

### `-e` 的三種寫法

```bash
-e LOG_LEVEL=debug        # 直接給值
-e LOG_LEVEL              # 不寫值：把「host 目前的」LOG_LEVEL 傳進去（host 沒設就不傳）
--env-file shorty.env     # 從檔案讀一整批
```

第二種很好用：秘密不會出現在指令本身（shell history、`ps` 看不到），但它還是會變成 container 的環境變數。

### 「bake 進 image」= 送給所有人

```
docker build --build-arg DB_PASSWORD=hunter2 ...
                     │
                     ▼
image ─┬─ config：ENV API_TOKEN=...            ← docker image inspect 看得到
       ├─ history：RUN |1 DB_PASSWORD=hunter2  ← docker history 看得到
       └─ layer 3：/tmp/app.conf                ← 下一層 rm 了，這一層還在
                     │
                     ▼ docker push
registry ─▶ 任何能 pull 的人、CI 快取、你的離職同事
```

image 一旦推出去，**就當作秘密已經外洩**。正確做法：

| 時機 | 秘密怎麼給 |
|------|-----------|
| build 時需要（私有依賴） | `RUN --mount=type=secret`（第 11 章） |
| 執行時需要 | 啟動 container 時給：環境變數，或**掛載成檔案** |

### 環境變數也不是完美的

環境變數比 bake 進 image 好太多，但它還是會出現在：

- `docker inspect <container>`（能用 docker 的人都看得到）
- 程式的 crash report、debug 頁面、不小心 `log.Info("config", "env", os.Environ())`
- 子行程（環境變數會被繼承）

所以更進一步的做法是**把秘密掛成檔案**，程式啟動時讀檔。

### 檔案掛載的秘密：`/run/secrets` 與 `_FILE` 慣例

```
host：secrets/db_password（權限收緊、不進 git）
          │  bind mount，唯讀
          ▼
container：/run/secrets/db_password
          │
          ▼
程式讀 POSTGRES_PASSWORD_FILE=/run/secrets/db_password 指向的檔案
```

- **`/run/secrets/<name>`** 是 Docker Swarm 的 `docker secret`、以及 Compose 的 `secrets:` 預設的掛載位置。手動 `docker run` 時我們用 bind mount 模擬同樣的路徑，之後換成 compose 時程式不用改。
- **`_FILE` 慣例**：很多官方 image（postgres、mysql、mariadb……）的 entrypoint 支援「`XXX` 或 `XXX_FILE` 二選一」：`POSTGRES_PASSWORD_FILE=/run/secrets/db_password` 會讀那個檔案的內容當密碼。環境變數裡只有**路徑**，沒有密碼本身。

### shorty 的環境變數表

| 變數 | 預設值 | 說明 | 敏感？ |
|------|--------|------|-------|
| `PORT` | `8080` | container 內監聽的 port。改了要記得改 `-p` 右邊 | 否 |
| `BASE_URL` | `http://localhost:$PORT` | 回給使用者的短網址前綴，要寫**外部**看得到的網址 | 否 |
| `DATABASE_URL` | （空 → in-memory） | Postgres DSN，含帳密 | **是** |
| `REDIS_URL` | （空 → 不快取） | Redis URL；有密碼時寫成 `redis://:pass@cache:6379/0` | 有密碼時是 |
| `LOG_LEVEL` | `info` | `debug`／`info`／`warn`／`error`，不認得的值退回 `info` | 否 |
| `GOMEMLIMIT` | （無） | Go runtime 的變數，不是 shorty 的；第 15 章 | 否 |

shorty 目前只認 `DATABASE_URL` 這個環境變數，**不支援 `DATABASE_URL_FILE`**。所以本章對 Postgres 用檔案秘密，對 shorty 仍用 `--env-file`——並在練習裡思考如果要支援 `_FILE`，該改哪裡。

## shorty 在這章

本章目錄的檔案：

| 檔案 | 用途 |
|------|------|
| [`shorty.env.example`](./shorty.env.example) | 設定範本，進 git；複製成 `shorty.env` 再填密碼 |
| [`Dockerfile.bad-secrets`](./Dockerfile.bad-secrets) | 反面教材：ARG／ENV／`RUN rm` 都藏不住秘密 |
| [`.gitignore`](./.gitignore) | 讓 `shorty.env` 和 `secrets/` 不會被 commit |

另外，`app/.dockerignore` 早在第 10 章就排除了 `.env`、`.env.*`，避免 `COPY . .` 把本機設定帶進 image。

## 動手做

以下指令都從 repo 根目錄執行。Git Bash 使用者先 `export MSYS_NO_PATHCONV=1`（見第 13 章）。

### 1. 看 image 預設的環境變數，用 `-e` 覆寫

```bash
docker build -t shorty app
docker image inspect shorty --format '{{json .Config.Env}}'
docker run --rm -e LOG_LEVEL=debug -e PORT=9000 -p 9000:9000 shorty
```

範例輸出：

```
["PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin","SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt"]
{"time":"...","level":"WARN","msg":"DATABASE_URL not set, using in-memory store"}
{"time":"...","level":"WARN","msg":"REDIS_URL not set, cache disabled"}
{"time":"...","level":"INFO","msg":"listening","addr":":9000","version":"dev"}
```

image 裡沒有任何 shorty 的設定——這正是我們要的。`Ctrl+C` 結束（shorty 收到 SIGINT 會 graceful shutdown）。

### 2. 反面教材：bake 進 image 的秘密

```bash
docker build -f tutorials/14-config-secrets/Dockerfile.bad-secrets \
  --build-arg DB_PASSWORD=hunter2 -t bad-secrets app
```

範例輸出（BuildKit 自己就在警告你）：

```
 2 warnings found (use docker --debug to expand):
 - SecretsUsedInArgOrEnv: Do not use ARG or ENV instructions for sensitive data (ARG "DB_PASSWORD") (line 16)
 - SecretsUsedInArgOrEnv: Do not use ARG or ENV instructions for sensitive data (ENV "API_TOKEN") (line 19)
```

假裝你是一個只拿到 image 的陌生人：

```bash
docker history --no-trunc --format '{{.CreatedBy}}' bad-secrets
docker image inspect bad-secrets --format '{{json .Config.Env}}'
```

範例輸出：

```
CMD ["sh" "-c" "echo API_TOKEN=$API_TOKEN"]
RUN ... /bin/sh -c rm /tmp/app.conf # buildkit
RUN |1 DB_PASSWORD=hunter2 /bin/sh -c echo "password=${DB_PASSWORD}" > /tmp/app.conf # buildkit
ENV API_TOKEN=tok_live_51HxYzEXAMPLE
...
["PATH=...","API_TOKEN=tok_live_51HxYzEXAMPLE"]
```

三種藏法全部破功：

- `ARG`：只要有 `RUN` 用到它，值就被記在 history 的 `|1 DB_PASSWORD=hunter2`。
- `ENV`：存在 image config，`inspect` 一行就看到，而且每個 container 都繼承。
- `RUN rm`：檔案在前一層 layer 裡還在。`docker save bad-secrets -o bad.tar` 解開來，翻到那一層就找得到 `/tmp/app.conf`。

清掉：`docker rmi bad-secrets`。

### 3. 準備網路、Redis 與一個檔案形式的 DB 密碼

```bash
docker network create shorty-net 2>/dev/null || true
docker run -d --name cache --network shorty-net redis:7-alpine

mkdir -p tutorials/14-config-secrets/secrets
openssl rand -hex 16 | tr -d '\n' > tutorials/14-config-secrets/secrets/db_password
chmod 644 tutorials/14-config-secrets/secrets/db_password
```

- `tr -d '\n'`：密碼檔尾巴不要有換行（很多程式會把換行當成密碼的一部分；postgres 的 entrypoint 會去掉，但別的程式不一定）。
- `chmod 644`：postgres 的 entrypoint 是以 uid 70（postgres 使用者）讀這個檔案，Linux 上 host 檔案權限太緊會讀不到。上線時用 Compose／Swarm 的 secrets 可以指定 uid 與權限，不用這樣放寬。
- 沒有 `openssl`？Git Bash 有內建；或用 `head -c 16 /dev/urandom | od -An -tx1 | tr -d ' \n'`。

### 4. Postgres 讀檔案秘密

```bash
docker run -d --name db --network shorty-net \
  -e POSTGRES_USER=shorty -e POSTGRES_DB=shorty \
  -e POSTGRES_PASSWORD_FILE=/run/secrets/db_password \
  --mount type=bind,src="$(pwd)/tutorials/14-config-secrets/secrets/db_password",dst=/run/secrets/db_password,readonly \
  -v shorty-db-ch14:/var/lib/postgresql/data \
  postgres:17-alpine
```

這裡刻意用**新的** volume `shorty-db-ch14`：第 13 章的 `shorty-db` 早已用密碼 `shorty` 初始化過，`POSTGRES_PASSWORD(_FILE)` 只有第一次初始化才生效。

確認密碼沒出現在環境變數裡：

```bash
docker inspect db --format '{{range .Config.Env}}{{println .}}{{end}}' | grep POSTGRES
```

範例輸出：

```
POSTGRES_USER=shorty
POSTGRES_DB=shorty
POSTGRES_PASSWORD_FILE=/run/secrets/db_password
```

只有路徑，沒有密碼。

### 5. shorty 用 `--env-file`

```bash
cp tutorials/14-config-secrets/shorty.env.example tutorials/14-config-secrets/shorty.env
PW=$(cat tutorials/14-config-secrets/secrets/db_password)
sed -i "s/CHANGE_ME/$PW/" tutorials/14-config-secrets/shorty.env
unset PW

docker run -d --name shorty --network shorty-net -p 8080:8080 \
  --env-file tutorials/14-config-secrets/shorty.env \
  shorty

docker logs shorty
curl localhost:8080/readyz
```

範例輸出：

```
{"time":"...","level":"INFO","msg":"connected to postgres"}
{"time":"...","level":"INFO","msg":"connected to redis"}
{"time":"...","level":"INFO","msg":"listening","addr":":8080","version":"dev"}
{"cache":"ok","store":"ok"}
```

（macOS 的 `sed -i` 要寫成 `sed -i ''`。產生的密碼是 hex，不含 `/`，所以 `sed` 不會出錯。）

`shorty.env` 已被本目錄的 `.gitignore` 排除。確認一下：

```bash
git status --short tutorials/14-config-secrets/   # 不該出現 shorty.env 和 secrets/
```

### 6. 環境變數看得到什麼

```bash
docker inspect shorty --format '{{range .Config.Env}}{{println .}}{{end}}' | grep DATABASE_URL
```

範例輸出：

```
DATABASE_URL=postgres://shorty:3f9a...c21e@db:5432/shorty?sslmode=disable
```

shorty 的 DB 密碼仍然是明文環境變數——這是 shorty 目前的限制（不支援 `_FILE`）。比 bake 進 image 好得多（不會跟著 image 外流），但能操作 docker 的人都看得到。

### 7. 只想臨時改一個設定

```bash
docker rm -f shorty
docker run -d --name shorty --network shorty-net -p 8080:8080 \
  --env-file tutorials/14-config-secrets/shorty.env \
  -e LOG_LEVEL=debug \
  shorty
```

設定是建立 container 時決定的，**改設定 = 重建 container**（image 不用動）。這是「設定與程式分離」最實際的好處。

### 8. 收拾

```bash
docker rm -f shorty db cache
docker volume rm shorty-db-ch14
docker network rm shorty-net
```

## 對照 Node／TypeScript

| Node 世界 | Docker 世界 |
|-----------|-------------|
| `dotenv` 讀 `.env` | `docker run --env-file`（但格式比 dotenv 陽春，見陷阱） |
| `process.env.DATABASE_URL` | Go 的 `os.Getenv("DATABASE_URL")`，一樣的東西 |
| `.env.example` 進 git、`.env` 不進 | `shorty.env.example` 進 git、`shorty.env` 不進 |
| Next.js 的 `NEXT_PUBLIC_*` 在 **build 時**就被寫死進 bundle | 同樣的陷阱：前端 bundle 裡的值等於 bake 進 image，**絕對不要放秘密**；要在 runtime 換值得另外設計（例如由後端提供 `/config`） |
| `fs.readFileSync(process.env.DB_PASSWORD_FILE)` | `_FILE` 慣例，程式自己讀檔 |
| `zod` 驗證 `process.env` | 啟動時檢查設定、缺了就直接 exit（shorty 的做法是給預設值 + 警告 log） |

**Node 開發者最常踩的雷**：在 Dockerfile 寫 `COPY . .`，而專案根目錄有 `.env`。如果 `.dockerignore` 沒排除它，你的本機密碼就進 image 了。

## 常見陷阱

- **`--env-file` 不是 dotenv**：`docker run --env-file` 的解析很陽春：
  - 值裡的引號**不會**被去掉：`LOG_LEVEL="debug"` 的值是 `"debug"`（含引號）。
  - 不支援 `${VAR}` 展開、不支援 `export KEY=...`、不支援行尾註解（`KEY=val # 註解` 的值包含 `# 註解`）。
  - Compose 的 `.env`（第 18 章）規則不一樣、支援比較多。兩者不要混用同一個檔案。
- **CRLF 換行**：在 Windows 用記事本或沒設定好的編輯器存 `shorty.env`，每行尾巴會多一個 `\r`。症狀是莫名其妙的錯誤，例如 `sslmode=disable\r` 不被接受、`LOG_LEVEL` 永遠是 info。VS Code 右下角把 `CRLF` 切成 `LF`；或在 repo 的 `.gitattributes` 對 `*.env*` 設 `eol=lf`。
- **`BASE_URL` 寫成 container 內的位址**：`BASE_URL=http://shorty:8080` 會讓 API 回傳使用者瀏覽器打不開的網址。它是給「外面的人」看的。
- **改 `POSTGRES_PASSWORD` 沒效**：只有空 volume 第一次初始化時讀（第 13 章）。
- **密碼裡有特殊字元**：放進 URL（`DATABASE_URL`）時，`@`、`:`、`/`、`#`、`?` 要 percent-encode（`@` → `%40`）。產生密碼時用 hex 或 base64url 最省事。
- **以為 `-e SECRET` 比較安全就結束了**：它避免了 shell history，但值仍然是 container 的環境變數，`docker inspect` 看得到。
- **秘密進了 log**：shorty 不會印出 `DATABASE_URL`，但 pgx 連線失敗的錯誤訊息有時會帶 host 資訊。自己寫程式時，記錄設定前先遮蔽敏感欄位。
- **Windows／Docker Desktop**：
  - PowerShell 設環境變數是 `$env:LOG_LEVEL = "debug"`，之後 `docker run -e LOG_LEVEL` 才傳得進去。
  - bind mount 單一檔案（`secrets/db_password`）在 Docker Desktop 上可行，但如果你之後在 host 用「另存新檔」的方式改這個檔案，container 裡可能還看到舊內容（編輯器換掉了檔案的 inode）。改秘密後重建 container 最穩。
  - Git Bash 下 `src="$(pwd)/..."` 會是 `/c/work/...`；如果 docker 抱怨路徑，改用 `"$(pwd -W)/..."`。

## 練習

1. 把 `shorty.env` 裡的 `LOG_LEVEL=info` 改成 `LOG_LEVEL="debug"`（加引號），重建 shorty，看 log 等級是 debug 還是 info？為什麼？
2. 在 host `export LOG_LEVEL=warn`，然後用 `-e LOG_LEVEL`（不寫值）啟動 shorty，確認 `request` 這種 info 等級的 log 不見了。
3. 把 `Dockerfile.bad-secrets` 改成用第 11 章的 `RUN --mount=type=secret`，重新 build，再用 `docker history --no-trunc` 確認 `hunter2` 不見了。
4. 設計題：如果要讓 shorty 支援 `DATABASE_URL_FILE`，`main.go` 的哪個函式要改？寫出你會怎麼改（只要 5～10 行 Go）。想想：兩個都有設時誰優先？檔案尾巴的換行怎麼處理？
5. 用 `docker save bad-secrets -o bad.tar`，解開 tar，找出含有 `/tmp/app.conf` 的那一層。（提示：看 `index.json` 或 `manifest.json`，layer 都是 tar 檔。）

## 下一章

設定搞定了，但 shorty 現在可以吃光整台機器的記憶體與 CPU，掛了也不會自己起來。第 15 章 [resources-restart](../15-resources-restart/) 加上 `--memory`／`--cpus` 限制、看 OOM killed（exit 137）長什麼樣子、設定 Go 的 `GOMEMLIMIT`，以及 restart policy。
