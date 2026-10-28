# 13. 資料保存：volume 與 bind mount

> 範圍：container 檔案系統是暫時的、named volume vs bind mount vs tmpfs、Postgres 資料保存、備份與還原 volume、Windows bind mount 效能

> 🧸 想先用故事直覺理解這章？看 [STORY.md](./STORY.md)（小學生都懂的比喻版）。

## 為什麼需要這個

第 12 章最後的那個實驗：`docker rm -f -v db` 再重建，所有短網址都不見了。

這不是 bug，是 container 的設計：**container 的檔案系統是用完即丟的**。image 是唯讀的 layer，container 啟動時在最上面疊一層薄薄的「可寫層」；你在 container 裡寫的任何檔案都在這層，**`docker rm` 的時候跟著一起刪掉**。

這對 shorty 本身是好事——它是無狀態的，砍掉重建毫無損失。但 Postgres 的資料、使用者上傳的檔案，必須放在 container **外面**。這章就是在講「外面」有哪幾種。

## 核心概念

### 可寫層：stop 不會丟，rm 才會丟

```
┌─────────────────────────────┐
│  可寫層（container 專屬）    │  ← docker rm 時消失
├─────────────────────────────┤
│  image layer N（唯讀）       │
│  ...                        │  ← 所有用這個 image 的 container 共用
│  image layer 1（唯讀）       │
└─────────────────────────────┘
```

- `docker stop` → `docker start`：可寫層還在，資料還在。
- `docker rm`：可寫層刪掉。之後用同一個 image `docker run`，是一個全新的、乾淨的 container。
- 升級 image（`postgres:17.5` → `17.6`）一定要建新 container，所以「資料放可寫層」等於「每次升級都丟資料」。

另外可寫層用 copy-on-write 的 storage driver，大量寫入的效能也比較差。資料庫本來就不該寫在這裡。

### 三種掛載

```
            host                               container
┌──────────────────────────────┐        ┌────────────────────────────┐
│ Docker 管理的區域              │        │                            │
│   /var/lib/docker/volumes/     │        │                            │
│     shorty-db/_data  ──────────┼─named──┼─▶ /var/lib/postgresql/data │
│                                │ volume │                            │
│ 你自己的資料夾                  │        │                            │
│   ./tutorials/13-volumes/site ─┼─bind───┼─▶ /usr/share/nginx/html    │
│                                │ mount  │                            │
│ 記憶體（RAM）  ─────────────────┼─tmpfs──┼─▶ /tmp                     │
└──────────────────────────────┘        └────────────────────────────┘
```

| | named volume | bind mount | tmpfs |
|---|---|---|---|
| 資料在哪 | Docker 管理的目錄 | host 上你指定的路徑 | 記憶體 |
| 誰建立 | `docker volume create` 或第一次 `run` 時自動建 | 你自己 | Docker，container 停止即消失 |
| 跨平台 | 一致 | 路徑、權限、效能依 OS 而異 | Linux container 才有 |
| 典型用途 | **資料庫**、需要保存的狀態 | **開發時掛原始碼**、設定檔 | 暫存檔、不想落地的東西 |
| 備份 | 要透過一個 container 打包 | 直接複製 host 上的資料夾 | 不需要 |

### `-v` 與 `--mount`

同一件事兩種寫法：

```bash
-v shorty-db:/var/lib/postgresql/data                       # named volume
--mount type=volume,src=shorty-db,dst=/var/lib/postgresql/data

-v "$(pwd)/site:/usr/share/nginx/html:ro"                   # bind mount（來源是路徑）
--mount type=bind,src="$(pwd)/site",dst=/usr/share/nginx/html,readonly

--tmpfs /tmp:rw,size=64m                                    # tmpfs
--mount type=tmpfs,dst=/tmp,tmpfs-size=67108864
```

差別：`-v` 用「來源長得像不像路徑」決定是 volume 還是 bind mount；bind mount 的來源不存在時，`-v` 會**默默幫你建一個空資料夾**（擁有者是 root），`--mount` 則直接報錯。腳本裡建議用 `--mount`，錯誤比較早浮現。

### 匿名 volume：你以為沒用 volume，其實有

`postgres` image 的 Dockerfile 裡有一行 `VOLUME /var/lib/postgresql/data`。意思是：**你沒掛任何東西到這裡時，Docker 自動建一個隨機名字的匿名 volume**。

所以第 12 章其實每次都有 volume——只是名字是一串 hex，`docker rm` 不加 `-v` 時它會被留下來變成孤兒；加了 `-v`（或用 `--rm` 跑）就跟著刪除。重建 container 時 Docker 也不會自動接回舊的匿名 volume，所以資料「看起來」不見了。

**結論：要保存的資料，一律用有名字的 volume。**

### named volume 的「預先填入」

掛一個**空的** named volume 到 image 裡已經有檔案的路徑時，Docker 會先把 image 裡那個路徑的內容複製進 volume。bind mount 不會這樣——bind mount 會直接**蓋住**那個路徑，image 原本的檔案看不到。

## shorty 在這章

shorty 本身不需要 volume（資料都在 Postgres，快取在 Redis）。這章只改 Postgres：

```diff
 docker run -d --name db --network shorty-net \
   -e POSTGRES_USER=shorty -e POSTGRES_PASSWORD=shorty -e POSTGRES_DB=shorty \
+  -v shorty-db:/var/lib/postgresql/data \
   postgres:17-alpine
```

根目錄 `compose.yaml` 的 `db-data:/var/lib/postgresql/data` 就是同一件事（第 18 章）。

本章附兩個腳本：

| 檔案 | 用途 |
|------|------|
| [`backup.sh`](./backup.sh) | 把 volume 打包成 `.tgz`（透過 stdout 串流，Windows 也不用煩惱路徑） |
| [`restore.sh`](./restore.sh) | 把 `.tgz` 還原到一個新的（空的）volume |

## 動手做

以下指令都從 repo 根目錄執行。如果你用 Git Bash，先執行 `export MSYS_NO_PATHCONV=1`（原因見常見陷阱）。

### 1. 可寫層是暫時的

```bash
docker run --name scratchpad alpine:3.22 sh -c 'echo hello > /note.txt'
docker diff scratchpad                         # container 已經停了，但可寫層還在
docker rm scratchpad
docker run --rm alpine:3.22 cat /note.txt      # 新 container：沒有這個檔
```

範例輸出：

```
A /note.txt
cat: can't open '/note.txt': No such file or directory
```

`docker diff` 列出可寫層相對於 image 的變化（`A` 新增、`C` 修改、`D` 刪除）。container 停止時它還在；`rm` 之後就沒了。

### 2. 找出第 12 章留下的匿名 volume

```bash
docker volume ls
```

範例輸出：

```
DRIVER    VOLUME NAME
local     3f9c1a7e0b2d4c...e81
local     a04b7d2c9e1f6a...19c
```

這些就是沒加 `-v` 刪 db 時留下的孤兒。清掉（`prune` 預設只清**匿名**的、沒人用的 volume）：

```bash
docker volume prune
```

### 3. 建 named volume，跑 Postgres

延續第 12 章的 network（還沒建的話先 `docker network create shorty-net`）：

```bash
docker volume create shorty-db

docker run -d --name db --network shorty-net \
  -e POSTGRES_USER=shorty -e POSTGRES_PASSWORD=shorty -e POSTGRES_DB=shorty \
  -v shorty-db:/var/lib/postgresql/data \
  postgres:17-alpine

docker run -d --name cache --network shorty-net redis:7-alpine

docker run -d --name shorty --network shorty-net -p 8080:8080 \
  -e BASE_URL=http://localhost:8080 \
  -e "DATABASE_URL=postgres://shorty:shorty@db:5432/shorty?sslmode=disable" \
  -e REDIS_URL=redis://cache:6379/0 \
  shorty

curl -XPOST localhost:8080/api/links -d '{"url":"https://go.dev"}'
```

範例輸出：

```
{"code":"Q7mZp2k","short_url":"http://localhost:8080/Q7mZp2k","url":"https://go.dev"}
```

看看 volume 的資訊：

```bash
docker volume inspect shorty-db
```

範例輸出：

```json
[
    {
        "CreatedAt": "2026-10-28T08:50:12Z",
        "Driver": "local",
        "Labels": null,
        "Mountpoint": "/var/lib/docker/volumes/shorty-db/_data",
        "Name": "shorty-db",
        "Options": null,
        "Scope": "local"
    }
]
```

在 Docker Desktop 上，`Mountpoint` 這個路徑在 Linux VM 裡面，不是在你的 C 槽。

### 4. 砍掉 db，資料還在

```bash
docker rm -f db
docker run -d --name db --network shorty-net \
  -e POSTGRES_USER=shorty -e POSTGRES_PASSWORD=shorty -e POSTGRES_DB=shorty \
  -v shorty-db:/var/lib/postgresql/data \
  postgres:17-alpine

docker logs db 2>&1 | grep -i "skipping initialization"
curl localhost:8080/api/links/Q7mZp2k      # 換成你的 code
```

範例輸出：

```
PostgreSQL Database directory appears to contain a database; Skipping initialization
{"code":"Q7mZp2k","url":"https://go.dev","hits":0,"created_at":"2026-10-28T08:50:30.12Z"}
```

`Skipping initialization`：Postgres 看到 volume 裡已經有資料庫，直接沿用。shorty 不用重啟——pgx 的 connection pool 會丟掉斷掉的連線、重新連（db 重建後 IP 可能變了，但名字 `db` 沒變）。剛重建的那一兩秒內打 API 可能拿到 500，之後就恢復。

### 5. 備份方法 A：`pg_dump`（資料庫首選）

資料庫的備份優先用它自己的工具。邏輯備份不需要停機，也能還原到不同版本的 Postgres：

```bash
mkdir -p tutorials/13-volumes/backup
docker exec db pg_dump -U shorty -d shorty > tutorials/13-volumes/backup/shorty.sql
```

還原（到任何一個空的 shorty 資料庫）：

```bash
docker exec -i db psql -U shorty -d shorty < tutorials/13-volumes/backup/shorty.sql
```

注意還原用 `-i`（要讀 stdin），但**不要**加 `-t`，否則 Windows 上的重導向會出問題。

### 6. 備份方法 B：直接打包整個 volume（通用）

任何 volume 都能用這招（不只是資料庫）。原理是：開一個臨時 alpine container，把 volume 以唯讀掛進去，用 `tar` 打包輸出到 stdout。

**資料庫 volume 打包前要先停掉 DB**，不然可能打包到寫一半的檔案：

```bash
docker stop db
bash tutorials/13-volumes/backup.sh shorty-db tutorials/13-volumes/backup/shorty-db.tgz
docker start db
```

範例輸出：

```
已備份 shorty-db → tutorials/13-volumes/backup/shorty-db.tgz（6512384 bytes）
```

`backup.sh` 的核心只有一行：

```bash
docker run --rm -v shorty-db:/volume:ro alpine:3.22 tar czf - -C /volume . > shorty-db.tgz
```

官方文件常見的寫法是再 bind mount 一個 host 資料夾當輸出位置（`-v "$(pwd)/backup:/backup"`），效果一樣；用 stdout 串流的好處是不用處理 Windows 路徑。

### 7. 還原到新 volume，驗證備份真的能用

**沒驗證過的備份不算備份。** 還原到一個新 volume，再開一個臨時 Postgres 讀它：

```bash
bash tutorials/13-volumes/restore.sh tutorials/13-volumes/backup/shorty-db.tgz shorty-db-restored

docker run -d --name db-check \
  -e POSTGRES_PASSWORD=whatever \
  -v shorty-db-restored:/var/lib/postgresql/data \
  postgres:17-alpine

docker exec db-check psql -U shorty -d shorty -c 'select code, url from links'
```

（等個兩三秒讓它啟動。`POSTGRES_PASSWORD` 在已有資料的 volume 上不會被使用，但 image 啟動時仍要求要有。）

範例輸出：

```
  code   |      url
---------+----------------
 Q7mZp2k | https://go.dev
(1 row)
```

清掉驗證用的東西：

```bash
docker rm -f db-check
docker volume rm shorty-db-restored
```

### 8. bind mount：host 改檔，container 立刻看到

```bash
docker run -d --name site -p 8082:80 \
  --mount type=bind,src="$(pwd)/tutorials/13-volumes/site",dst=/usr/share/nginx/html,readonly \
  nginx:alpine
curl localhost:8082
```

用編輯器改 `tutorials/13-volumes/site/index.html`，再 `curl` 一次——內容立刻變了，不用重 build、不用重啟。這是第 19 章「開發時掛原始碼 + 熱重載」的基礎。

```bash
docker rm -f site
```

### 9. tmpfs：放在記憶體、停止即消失

```bash
docker run --rm --tmpfs /scratch:rw,size=64m alpine:3.22 sh -c 'df -h /scratch; echo temp > /scratch/x; ls /scratch'
```

範例輸出：

```
Filesystem                Size      Used Available Use% Mounted on
tmpfs                    64.0M         0     64.0M   0% /scratch
x
```

第 21 章把 shorty 設成 `read_only` 時，需要寫暫存檔的地方就是用 tmpfs 補。

### 10. 收拾

```bash
docker rm -f shorty cache db
docker volume ls                 # shorty-db 還在——這正是重點
docker volume rm shorty-db       # 真的不要資料時才刪
```

## 對照 Node／TypeScript

| 情境 | Node 開發者熟悉的 | Docker 裡 |
|------|------------------|-----------|
| 本機 Postgres 的資料 | Postgres.app／Homebrew 存在 `~/Library/...` | named volume `shorty-db` |
| 上傳檔案 | `multer` 存到 `./uploads` | 掛 named volume 到 `/app/uploads`；上線多半改用 S3 之類的物件儲存 |
| 開發時改 code 即時生效 | `nodemon`、`tsx watch` | bind mount 原始碼 + 同樣的 watcher（第 19 章） |
| `node_modules` | 在專案資料夾裡 | bind mount 專案時，常再掛一個 named volume 到 `/app/node_modules` 把它「遮住」，避免 host（Windows／macOS）編的原生模組跑進 Linux container |
| 暫存檔 | `os.tmpdir()` | tmpfs |

最後那個 `node_modules` 技巧利用的正是「named volume 會預先填入 image 內容、而且蓋在 bind mount 上面」的特性。

## 常見陷阱

- **改了 `POSTGRES_PASSWORD` 卻登不進去**：`POSTGRES_USER`／`POSTGRES_PASSWORD`／`POSTGRES_DB` **只在 volume 是空的、第一次初始化時**生效。之後改環境變數完全沒用，資料庫裡的密碼還是舊的。要嘛用 `ALTER USER` 改，要嘛（開發環境）刪掉 volume 重來。
- **`docker volume prune` 沒清掉 named volume**：Docker Engine 23 起，`prune` 預設只清匿名 volume；要連沒人用的 named volume 一起清，加 `-a`。「沒人用」的意思是沒有被任何 container 引用——停止中的 container 也算有在用，所以 `docker stop db` 後 prune 不會清掉 `shorty-db`；但 `docker rm db` 之後再 `prune -a`，資料就沒了。
- **用 `docker rm -v` 刪掉了 named volume？** 不會。`-v` 只刪**匿名** volume，named volume 永遠要你自己 `docker volume rm`。第 18 章的 `docker compose down -v` 則會刪掉 compose 檔裡宣告的 named volume——那才是要小心的。
- **bind mount 權限問題**：Linux 上 bind mount 的檔案保留 host 的 uid/gid。container 裡用非 root 使用者（例如第 21 章的 shorty 是 uid 65532、postgres 是 uid 70）寫入 host 資料夾時，常常 `permission denied`。資料庫請用 named volume。
- **Postgres 18 的路徑改了**：本系列用 `postgres:17-alpine`，資料目錄是 `/var/lib/postgresql/data`。`postgres:18` 起 image 的 `VOLUME` 改成 `/var/lib/postgresql`、`PGDATA` 預設變成帶版本號的子目錄。升級時照抄舊的掛載路徑，會變成資料寫進匿名 volume——升級前先看官方 image 的說明。
- **直接 tar 正在跑的資料庫**：備份檔可能是壞的。要嘛先 `docker stop`，要嘛用 `pg_dump`。
- **跨大版本還原 volume**：用 Postgres 17 的 volume 打包，還原後給 Postgres 18 跑是起不來的（資料檔格式不同）。跨版本要用 `pg_dump` 或 `pg_upgrade`。
- **Windows／Docker Desktop**：
  - **Git Bash 的路徑改寫**：Git Bash 會把看起來像 POSIX 路徑的參數（例如 `-v shorty-db:/volume` 裡的 `/volume`）改寫成 `C:/Program Files/Git/volume`，出現莫名其妙的錯誤。執行 `export MSYS_NO_PATHCONV=1` 關掉這個行為；本章腳本已經內建。bind mount 的 host 路徑在 Git Bash 下可以用 `"$(pwd -W)/..."` 拿到 `C:/...` 形式。
  - **bind mount 效能**：Docker Desktop（WSL2 後端）的 container 跑在 Linux VM 裡。掛 `C:\` 底下的資料夾要經過跨 OS 的檔案共享，大量小檔案的讀寫（`go build`、`npm install`、`node_modules`）會**慢上好幾倍**。解法：把專案放在 WSL 的檔案系統裡（例如 Ubuntu 的 `~/work/...`，Windows 端看是 `\\wsl$\Ubuntu\home\...`），從 WSL 裡下指令；或讓重 I/O 的資料夾用 named volume。
  - **檔案變更通知**：從 `C:\` bind mount 進去的檔案，修改時 container 裡的 inotify 不一定收得到，熱重載工具可能沒反應（第 19 章會再談）。專案放 WSL 檔案系統裡就沒這問題。
  - **不要把資料庫目錄 bind mount 到 `C:\`**：權限、`fsync` 行為都跟 Linux 不同，Postgres 可能起不來或很慢。一律 named volume。
  - named volume 存在 Docker Desktop 的 VM 虛擬磁碟裡；在 Windows 檔案總管找不到是正常的，要拿出來就用本章的備份方法。

## 練習

1. 不建 volume、直接 `-v my-new-vol:/var/lib/postgresql/data` 跑 Postgres，再 `docker volume ls`——volume 是什麼時候被建立的？
2. 用 `-v ./not-exist:/data`（`-v` 寫法）和 `--mount type=bind,src=./not-exist,dst=/data`（`--mount` 寫法）各跑一次 alpine，比較行為差異，並看 host 上多了什麼。
3. 把 shorty 的資料用 `pg_dump` 備份，刪掉 `shorty-db`，建一個全新的 db，再用 `psql` 還原，確認短網址的 `hits` 都回來了。
4. 寫一個指令，把 `shorty-db` 整個複製成 `shorty-db-copy`（提示：一個 container 同時掛兩個 volume，`cp -a`）。
5. Windows 使用者：把 `app/` 放在 `C:\` 和放在 WSL 檔案系統各 bind mount 進 `golang:1.26-alpine`，跑 `go build ./...` 比較時間。

## 下一章

到目前為止，密碼 `shorty` 都直接寫在指令裡，`docker inspect` 一下就看得到。第 14 章 [config-secrets](../14-config-secrets/) 整理 shorty 的所有設定、改用 `--env-file`，並說明為什麼秘密不能 bake 進 image、以及用檔案掛載秘密的做法。
