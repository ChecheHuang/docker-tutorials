# 12. 網路：bridge、port、DNS

> 範圍：bridge network、`-p` 是 NAT、自訂 network 的內建 DNS（用 container 名字互連）、`host` 模式、`localhost` 在 container 裡指誰、`docker network create` + 手動跑 postgres + redis + shorty

> 🧸 想先用故事直覺理解這章？看 [STORY.md](./STORY.md)（小學生都懂的比喻版）。

## 為什麼需要這個

到第 11 章為止，shorty 都是一個人跑：`docker run -p 8080:8080 shorty`，沒設 `DATABASE_URL`，資料存在記憶體裡。真正上線時它要連 Postgres 存資料、連 Redis 做快取。

第一次把 DB 也放進 container 的人，幾乎都會寫出這行：

```bash
docker run -e DATABASE_URL=postgres://shorty:shorty@localhost:5432/shorty shorty
```

然後看到 `connection refused`。因為 **container 裡的 `localhost` 是 container 自己**，不是你的電腦，也不是 Postgres 那個 container。

這章要搞懂三件事：

1. container 之間怎麼找到彼此（答案：自訂 network + 用名字）。
2. 外面（你的瀏覽器、curl）怎麼連到 container（答案：`-p`，它是 NAT）。
3. container 怎麼連回你的電腦（答案：`host.docker.internal`）。

## 核心概念

### 每個 container 都有自己的網路堆疊

```
你的電腦（Linux host，或 Docker Desktop 的 Linux VM）
┌──────────────────────────────────────────────────────────┐
│  eth0: 192.168.1.20        lo: 127.0.0.1（host 自己的）    │
│                                                          │
│   bridge: shorty-net（172.18.0.1）  ← 一台虛擬交換器      │
│     ├── db      172.18.0.2   lo: 127.0.0.1（db 自己的）   │
│     ├── cache   172.18.0.3   lo: 127.0.0.1（cache 自己的）│
│     └── shorty  172.18.0.4   lo: 127.0.0.1（shorty 自己的）│
│                     ▲                                    │
│   -p 8080:8080 ─────┘  host:8080 ──NAT──▶ 172.18.0.4:8080  │
└──────────────────────────────────────────────────────────┘
```

每個 container 有自己的 network namespace：自己的網卡、IP、路由表、**自己的 `127.0.0.1`**。所以在 shorty 裡連 `localhost:5432`，敲的是 shorty 自己的門，裡面當然沒有 Postgres。

### 四種 network driver（本章用到的）

| network | 怎麼來 | 特性 |
|---------|--------|------|
| `bridge`（預設那個） | 安裝 Docker 就有，沒指定 `--network` 就接這個 | 有 IP、能上網，**沒有名字解析** |
| 自訂 bridge | `docker network create shorty-net` | 有 IP、能上網，**內建 DNS：用 container 名字互連** |
| `host` | 內建 | 不隔離，直接用 host 的網路；`-p` 無效 |
| `none` | 內建 | 只有 loopback，完全斷網 |

**重點：預設的 `bridge` 沒有 DNS，自訂的才有。** 這是歷史包袱（舊的做法是 `--link`，已經不建議使用）。所以實務上一律自己建 network。

### 內建 DNS

接在自訂 network 上的 container，`/etc/resolv.conf` 會指向 `127.0.0.11`——Docker 內建的 DNS server。它會回答：

- **container 名字**（`--name db` → `db`）
- **network alias**（`--network-alias postgres`）
- 其他名字轉給 host 的 DNS（所以 `go.dev` 照樣查得到）

### `-p` 是 NAT

```
-p 8080:8080            host 所有介面的 8080 → container 的 8080
-p 127.0.0.1:8080:8080  只有 host 本機能連（比較安全）
-p 9000:8080            host 的 9000 → container 的 8080
-P                      把 Dockerfile 裡 EXPOSE 的 port 各配一個隨機 host port
```

Docker 用 iptables／nftables 規則（加上 `docker-proxy`）把打到 host port 的封包轉進 container。幾個推論：

- **`EXPOSE` 不會發布 port**，它只是文件（加上給 `-P` 用）。沒寫 `-p` 外面就連不到。
- **container 之間互連不需要 `-p`**。shorty 連 `db:5432` 走的是 bridge 內部，Postgres 不用對外開 port。
- 程式必須監聽 `0.0.0.0`（或 `:8080`），**不能只聽 `127.0.0.1`**——NAT 轉進來的封包是從 bridge 網卡進來的，不是 loopback。shorty 用 `":" + port`，所以沒問題。

### container 連回 host

| 環境 | 怎麼連 host |
|------|-------------|
| Docker Desktop（Windows／macOS） | `host.docker.internal`，內建 |
| Linux Docker Engine | 加 `--add-host=host.docker.internal:host-gateway` |

## shorty 在這章

shorty 的程式不用改，只要給對環境變數：

| 變數 | 值 | 為什麼 |
|------|----|--------|
| `DATABASE_URL` | `postgres://shorty:shorty@db:5432/shorty?sslmode=disable` | `db` 是 Postgres container 的名字 |
| `REDIS_URL` | `redis://cache:6379/0` | `cache` 是 Redis container 的名字 |
| `BASE_URL` | `http://localhost:8080` | 這是**給使用者看的**網址，所以是 host 的 port |

名字刻意取成 `db` 和 `cache`，跟根目錄 `compose.yaml` 的 service 名一樣——第 17 章換成 compose 時，連線字串一個字都不用改。

本章附一個 [`setup.sh`](./setup.sh)，把下面「動手做 4」包成 `up`／`check`／`down` 三個指令。

## 動手做

以下指令都從 repo 根目錄執行。Windows 請用 Git Bash 或 WSL（理由見「常見陷阱」）。

### 1. 看看有哪些 network

```bash
docker network ls
```

範例輸出：

```
NETWORK ID     NAME      DRIVER    SCOPE
3c1f0e2a9b11   bridge    bridge    local
8d2b7e6f4c02   host      host      local
f1a9c3d5e703   none      null      local
```

### 2. 證明「預設 bridge 沒有 DNS」

```bash
docker run -d --name db -e POSTGRES_USER=shorty -e POSTGRES_PASSWORD=shorty -e POSTGRES_DB=shorty postgres:17-alpine
docker run --rm busybox:1.37 nslookup db
```

範例輸出：

```
Server:		192.168.65.7
Address:	192.168.65.7:53

** server can't find db: NXDOMAIN
```

兩個都在預設 `bridge` 上，彼此有 IP 但叫不出名字。清掉：

```bash
docker rm -f -v db
```

（`-v` 是順便刪掉 postgres image 自動建立的匿名 volume，第 13 章會解釋。）

### 3. `localhost` 指的是誰

```bash
docker run -d --name web -p 8081:80 nginx:alpine
docker exec web wget -qO- http://localhost:80 | head -4    # container 裡的 localhost = nginx 自己 → 成功
docker run --rm busybox:1.37 wget -qO- -T 2 http://localhost:8081   # 另一個 container 的 localhost → 失敗
```

範例輸出（第二個）：

```
wget: can't connect to remote host (127.0.0.1): Connection refused
```

那個 busybox 的 `localhost` 是 busybox 自己，它身上沒有 8081。但在**你的電腦**上 `curl localhost:8081` 是通的——因為 `-p` 開的是 host 的 port。

```bash
docker rm -f web
```

### 4. 正式來：自訂 network + 三個 container

```bash
docker network create shorty-net

docker run -d --name db --network shorty-net \
  -e POSTGRES_USER=shorty -e POSTGRES_PASSWORD=shorty -e POSTGRES_DB=shorty \
  postgres:17-alpine

docker run -d --name cache --network shorty-net redis:7-alpine

docker build -t shorty app

docker run -d --name shorty --network shorty-net -p 8080:8080 \
  -e BASE_URL=http://localhost:8080 \
  -e "DATABASE_URL=postgres://shorty:shorty@db:5432/shorty?sslmode=disable" \
  -e REDIS_URL=redis://cache:6379/0 \
  shorty
```

`DATABASE_URL` 整段加引號：裡面的 `?` 在 bash／zsh 是萬用字元。

注意 **db 和 cache 都沒有 `-p`**——只有 shorty 需要被外面連到。

看 shorty 的 log：

```bash
docker logs shorty
```

範例輸出：

```
{"time":"2026-10-28T08:40:01.12Z","level":"WARN","msg":"postgres not ready, retrying","attempt":1,"err":"ping postgres: ... connect: connection refused"}
{"time":"2026-10-28T08:40:02.15Z","level":"INFO","msg":"connected to postgres"}
{"time":"2026-10-28T08:40:02.16Z","level":"INFO","msg":"connected to redis"}
{"time":"2026-10-28T08:40:02.16Z","level":"INFO","msg":"listening","addr":":8080","version":"dev"}
```

Postgres 第一次啟動要初始化資料庫，比 shorty 慢個一兩秒；shorty 內建重試 10 次所以撐得過去。（「等對方真的好了再啟動」是第 16、18 章 healthcheck 的事。）

### 5. 從 host 打 API

```bash
curl localhost:8080/readyz
curl -XPOST localhost:8080/api/links -d '{"url":"https://go.dev"}'
curl -i localhost:8080/<上一步拿到的 code>
```

範例輸出：

```
{"cache":"ok","store":"ok"}
{"code":"aB3xK9p","short_url":"http://localhost:8080/aB3xK9p","url":"https://go.dev"}
HTTP/1.1 302 Found
Location: https://go.dev
...
```

確認資料真的進了 Postgres、快取真的進了 Redis：

```bash
docker exec db psql -U shorty -c 'select code, url, hits from links'
docker exec cache redis-cli keys 'link:*'
```

範例輸出：

```
  code   |      url       | hits
---------+----------------+------
 aB3xK9p | https://go.dev |    1
(1 row)

1) "link:aB3xK9p"
```

### 6. 在 network 裡面看 DNS

shorty 是 distroless，沒有 shell 也沒有 `nslookup`。借一個 busybox 接到同一個 network：

```bash
docker run --rm --network shorty-net busybox:1.37 sh -c 'cat /etc/resolv.conf; nslookup db; wget -qO- http://shorty:8080/healthz'
```

範例輸出：

```
nameserver 127.0.0.11
options ndots:0

Server:		127.0.0.11
Address:	127.0.0.11:53

Non-authoritative answer:
Name:	db
Address: 172.18.0.2

{"status":"ok"}
```

看整個 network 上有誰：

```bash
docker network inspect shorty-net --format '{{range .Containers}}{{.Name}} {{.IPv4Address}}{{"\n"}}{{end}}'
docker port shorty
```

範例輸出：

```
db 172.18.0.2/16
cache 172.18.0.3/16
shorty 172.18.0.4/16

8080/tcp -> 0.0.0.0:8080
8080/tcp -> [::]:8080
```

### 7. 動態接上／拔掉 network、別名

```bash
docker run -d --name tmp busybox:1.37 sleep 600       # 在預設 bridge 上
docker network connect --alias pg-client shorty-net tmp
docker exec tmp nslookup db                            # 現在查得到了
docker network disconnect shorty-net tmp
docker rm -f tmp
```

一個 container 可以同時接好幾個 network（例如前端 network + 後端 network），這就是隔離「誰可以連 DB」的基本手法。

### 8. `host` 模式（Linux 才有意義）

```bash
docker run --rm --network host nginx:alpine
```

在 Linux 上，nginx 直接用 host 的 port 80，不需要 `-p`（加了也會被忽略）。效能最好、但完全沒隔離，而且 port 會跟 host 上的程式撞。在 Docker Desktop 上，「host」預設是那台 Linux VM，不是你的 Windows／macOS（見常見陷阱）。

### 9. 收拾

```bash
docker rm -f -v shorty cache db
docker network rm shorty-net
```

或直接 `bash tutorials/12-networking/setup.sh down`。

## 對照 Node／TypeScript

| 情境 | Node 世界的寫法 | 在 Docker 裡 |
|------|-----------------|--------------|
| 連 DB | `.env` 寫 `DATABASE_URL=postgres://...@localhost:5432/...` | 改成 container 名字：`@db:5432` |
| 啟動 server | `app.listen(3000)` | 預設就聽所有介面，OK |
| 開發伺服器 | `vite`、`next dev` 預設只聽 `localhost` | 要 `vite --host 0.0.0.0`、`next dev -H 0.0.0.0`，不然 `-p` 連不進來 |
| 前端打 API | 瀏覽器 `fetch('http://api:8080')` | **不行**：瀏覽器跑在 host，不認得 container 名字；要用 `localhost:8080` 或反向代理 |
| 打 host 上的服務 | `localhost:3000` | `host.docker.internal:3000` |

最後一點特別容易混淆：**container 名字只有在 Docker network 裡面才查得到**。SSR（Next.js server 端）在 container 裡可以用 `http://api:8080`；同一個頁面的瀏覽器端 JavaScript 不行。

## 常見陷阱

- **`connection refused` 而且你寫了 `localhost`**：99% 是這章的主題。在 container 裡，`localhost` 是自己。改成對方的 container 名字，並確認兩邊在同一個自訂 network。
- **`no such host` / `NXDOMAIN`**：名字打錯，或其中一方在預設 `bridge` 上（沒 DNS），或兩邊在不同 network。`docker inspect <name> --format '{{json .NetworkSettings.Networks}}'` 看它接了哪些 network。
- **`-p` 了還是連不到**：程式只聽 `127.0.0.1`。`docker exec` 進去（或用 busybox 接同一 network）連得到、host 連不到，就是這個。
- **`Bind for 0.0.0.0:8080 failed: port is already allocated`**：host 上已經有東西佔 8080（另一個 container、或本機跑著 `go run`）。改 `-p 18080:8080`，或 `docker ps` 找出兇手。第 25 章有完整排查流程。
- **把 DB port 也 `-p` 出去**：`-p 5432:5432` 會讓整個區網都能連你的 Postgres（而且 Docker 的 NAT 規則會繞過部分 host 防火牆設定，例如 Linux 上的 ufw）。需要從 host 用 GUI 工具連時，用 `-p 127.0.0.1:5432:5432`。
- **Windows／Docker Desktop**：
  - Docker Desktop 的 container 跑在一台 Linux VM（WSL2 或 Hyper-V）裡；`-p` 會被 Desktop 再轉到 Windows 的 `localhost`，所以 `curl localhost:8080` 照樣通。
  - `host.docker.internal` 內建可用，指向你的 Windows 主機。在 container 裡要連 Windows 上跑的服務（例如本機的 Postgres）就用它。
  - `--network host` 在 Docker Desktop 上預設指的是 **VM 的網路**，不是 Windows 的。Docker Desktop 4.34 起可以在 Settings → Resources → Network 開啟 host networking 支援，但仍有限制；這系列不依賴它。
  - Windows PowerShell 5.1 的 `curl` 是 `Invoke-WebRequest` 的別名，`-XPOST -d` 不能用；請用 `curl.exe`，或直接用 Git Bash。PowerShell 裡 JSON 的引號跳脫也很麻煩，這也是建議用 Git Bash 的原因。
  - 公司 VPN 有時會跟 Docker 預設的子網（`172.17.0.0/16` 等）衝突，症狀是連上 VPN 後 container 上不了網。可以 `docker network create --subnet 10.123.0.0/24 shorty-net` 指定別的網段。
- **改了 network 設定但 container 沒變**：`--network`、`-p` 都是建立 container 時決定的（`network connect` 例外）。要改 `-p` 只能 `rm` 掉重建。

## 練習

1. 把 shorty 的 `DATABASE_URL` 故意寫成 `@localhost:5432`，跑起來看 log，數一數它重試幾次之後放棄，以及 container 的最終狀態（`docker ps -a`）。
2. 不用 `--name db`，改用 `--network-alias postgres` 讓 Postgres 在 network 裡叫 `postgres`，調整 shorty 的連線字串讓它連得上。
3. 建兩個 network：`front` 與 `back`。shorty 同時接兩個，db 只接 `back`，再跑一個 busybox 只接 `front`。驗證 busybox 連得到 shorty、但查不到 db。
4. 用 `-p 127.0.0.1:8080:8080` 重跑 shorty，再用 `docker port shorty` 比較輸出差異。
5. 在 host 上開一個 `python -m http.server 9999`（或任何本機服務），從 container 裡用 `host.docker.internal:9999` 連它。Linux 使用者記得加 `--add-host=host.docker.internal:host-gateway`。

## 下一章

試試看：`docker rm -f -v db`，再重建一次 db——剛剛建的短網址全不見了。第 13 章 [volumes](../13-volumes/) 用 named volume 讓 Postgres 的資料活得比 container 久，並示範怎麼備份、還原一個 volume。
