# 05. 進到 container 裡：exec、logs、inspect

> 範圍：`exec -it sh`、`logs -f`、`inspect` + `--format`、`stats`、`cp`、`top`；為什麼 container 的 log 要寫 stdout

> 🧸 想先用故事直覺理解這章？看 [STORY.md](./STORY.md)（小學生都懂的比喻版）。

## 為什麼需要這個

Container 跑起來之後，它就像一個關起來的盒子。出問題時你會想知道：

- 它**說了什麼**？→ `docker logs`
- 我能不能**進去看看**？→ `docker exec`
- 它的**設定**是什麼？IP 多少？環境變數有哪些？→ `docker inspect`
- 它**吃了多少資源**？裡面**有哪些 process**？→ `docker stats`、`docker top`
- 我想**拿一個檔案出來／放一個檔案進去**→ `docker cp`

這六個指令是日常除錯的基本功。後面每一章（尤其第 20 章 compose 除錯、第 25 章事故演練）都會反覆用到。

## 核心概念

### 1. 六個指令各看哪裡

```
                        container "web"
             ┌────────────────────────────────────┐
 docker exec │  PID 1: nginx master  ──stdout──┐  │
 ───────────▶│  PID 7: nginx worker  ──stderr──┤  │──▶ docker logs
 （多開一個   │  PID 30: sh  ← exec 進來的        │  │    （Docker 收集 PID 1 的
  process）  │                                 │  │     stdout / stderr）
             │  檔案系統 /etc/nginx/...  ◀──────┼──┼──▶ docker cp
             └─────────────────────────────────┼──┘
                   ▲                ▲          │
       docker inspect（設定、狀態、網路） docker stats / top（資源、process）
```

### 2. `exec` 是「多開一個 process」，不是「登入」

`docker exec -it web sh` 在**已經在跑的** container 裡，另外啟動一個 `sh` process，和主 process 共用同一個檔案系統、網路、namespace。

- container 必須是 `running`，停止的 container 不能 exec。
- 你 `exit` 只是結束那個 `sh`，**主 process 不受影響**，container 繼續跑。
- 對照第 04 章的 `docker attach`：attach 是接到**主 process** 的輸入輸出，在裡面按 `Ctrl+C` 可能把整個 container 停掉。日常要「進去看看」，用 `exec`。
- image 裡要**有**那個指令才能 exec。shorty 最終版用的 `distroless/static` 連 `sh` 都沒有，`docker exec -it ... sh` 會失敗——這是刻意的安全設計，第 16、21 章會講怎麼替代。

### 3. 為什麼 log 要寫 stdout

Docker 會自動收集每個 container **主 process 的 stdout 和 stderr**，`docker logs` 看的就是這個。

```
程式寫 stdout/stderr ──▶ Docker logging driver ──▶ docker logs / compose logs / 雲端 log 系統
程式寫 /var/log/app.log ──▶ 留在 container 可寫層 ──▶ docker logs 看不到；container 刪掉就沒了
```

寫檔案的壞處：

| 問題 | 說明 |
|------|------|
| `docker logs` 看不到 | 要 exec 進去 `cat`，distroless 還沒有 `cat` |
| 跟著 container 消失 | 一 `rm` 就沒了，正好是最需要看 log 的時候 |
| 撐爆可寫層 | 沒有 rotation，磁碟慢慢被吃光 |
| 平台接不起來 | compose、Kubernetes、雲端 log 服務都預設收 stdout |

所以慣例是：**程式只管把 log 寫到 stdout，收集、保存、輪替交給平台**（這也是 12-factor 的「logs 是事件流」）。官方 nginx image 甚至把 `/var/log/nginx/access.log` 做成指向 `/dev/stdout` 的 symlink。shorty 也是這樣：`main.go` 的 `newLogger` 用 `slog.NewJSONHandler(os.Stdout, ...)` 把 JSON log 寫到 stdout。log 檔案太大、要設 rotation 是第 25 章的演練。

### 4. `inspect --format` 是 Go template

`docker inspect` 輸出一大包 JSON。`--format`（或 `-f`）用 Go template 只取你要的欄位：

| 寫法 | 意思 |
|------|------|
| `{{.State.Status}}` | 取欄位 |
| `{{json .Config.Env}}` | 把欄位輸出成 JSON |
| `{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}` | 走訪 map／陣列 |
| `{{.Name}} {{.State.ExitCode}}` | 一次取多個，中間的文字原樣輸出 |

## 動手做

所有指令從 repo 根目錄執行。先準備兩個 container：

```bash
docker run -d --name web -p 8080:80 nginx:1.27-alpine
docker run -d --name pg -e POSTGRES_PASSWORD=secret postgres:17-alpine
```

> **Git Bash 使用者先看**：Git Bash 會把「以 `/` 開頭的參數」自動改寫成 Windows 路徑，例如 `/usr/share/nginx/html` 變成 `C:/Program Files/Git/usr/share/nginx/html`，送進 container 當然找不到。本章有很多 container 內的路徑，請先執行 `export MSYS_NO_PATHCONV=1`（只影響目前這個視窗），或改用 PowerShell。

### 步驟 1：`exec` 跑單一指令

```bash
docker exec web nginx -v
docker exec web ls /usr/share/nginx/html
```

範例輸出：

```
nginx version: nginx/1.27.5
50x.html
index.html
```

### 步驟 2：`exec -it` 進去逛

```bash
docker exec -it web sh
```

範例畫面：

```
/ # ps
PID   USER     TIME  COMMAND
    1 root      0:00 nginx: master process nginx -g daemon off;
   30 nginx     0:00 nginx: worker process
   31 nginx     0:00 nginx: worker process
   38 root      0:00 sh
   44 root      0:00 ps
/ # cat /etc/nginx/conf.d/default.conf | head -5
server {
    listen       80;
    listen  [::]:80;
    server_name  localhost;

/ # exit
```

注意 PID 1 是 nginx，你的 `sh` 是另外一個 process。`exit` 之後：

```bash
docker ps --filter name=web
```

STATUS 還是 `Up`——container 沒有因為你離開而停下來。

nginx 是 alpine 版所以用 `sh`；debian 系的 image（例如 `nginx:latest`）通常也有 `bash`。不確定就先試 `sh`。

進資料庫也是同一招：

```bash
docker exec -it pg psql -U postgres
```

範例畫面：

```
psql (17.6)
Type "help" for help.

postgres=# select 1 + 1 as answer;
 answer
--------
      2
(1 row)

postgres=# \q
```

不需要互動時，拿掉 `-it` 直接下指令：

```bash
docker exec pg psql -U postgres -c 'select version();'
```

`exec` 也接受 `-e`（多帶環境變數）、`-u`（換使用者）、`-w`（換工作目錄）等旗標，一樣放在 container 名字前面。

### 步驟 3：`logs`

先產生幾筆請求：

```bash
# Git Bash / macOS / Linux
for i in 1 2 3; do curl -s http://localhost:8080 > /dev/null; done
```

```powershell
# PowerShell
1..3 | ForEach-Object { curl.exe -s -o NUL http://localhost:8080 }
```

```bash
docker logs web
```

範例輸出（節錄）：

```
/docker-entrypoint.sh: Configuration complete; ready for start up
2026/10/28 08:10:02 [notice] 1#1: nginx/1.27.5
2026/10/28 08:10:02 [notice] 1#1: start worker processes
172.17.0.1 - - [28/Oct/2026:08:15:42 +0000] "GET / HTTP/1.1" 200 615 "-" "curl/8.9.1" "-"
172.17.0.1 - - [28/Oct/2026:08:15:42 +0000] "GET / HTTP/1.1" 200 615 "-" "curl/8.9.1" "-"
172.17.0.1 - - [28/Oct/2026:08:15:43 +0000] "GET / HTTP/1.1" 200 615 "-" "curl/8.9.1" "-"
```

常用旗標：

```bash
docker logs --tail 2 web          # 只看最後 2 行
docker logs --since 5m web        # 最近 5 分鐘
docker logs -t web                # 每行前面加上 Docker 記錄的時間戳
docker logs -f web                # 持續追蹤（follow），新的 log 會一直印出來
```

`-f` 的時候去瀏覽器重新整理 <http://localhost:8080>，會看到新的一行冒出來。按 `Ctrl+C` 只是**停止追蹤**，不會停掉 container。

看看 nginx 為什麼能把 log 交給 Docker：

```bash
docker exec web ls -l /var/log/nginx
```

範例輸出：

```
lrwxrwxrwx    1 root     root            11 Oct  7 18:00 access.log -> /dev/stdout
lrwxrwxrwx    1 root     root            11 Oct  7 18:00 error.log -> /dev/stderr
```

### 步驟 4：寫檔案 vs 寫 stdout

用兩個小 container 親眼比較：

```bash
docker run -d --name to-file alpine sh -c 'while true; do date >> /tmp/app.log; sleep 1; done'
docker run -d --name to-stdout alpine sh -c 'while true; do date; sleep 1; done'
```

過幾秒：

```bash
docker logs to-file
docker logs --tail 3 to-stdout
```

範例輸出：

```
Wed Oct 28 08:20:11 UTC 2026
Wed Oct 28 08:20:12 UTC 2026
Wed Oct 28 08:20:13 UTC 2026
```

`to-file` 什麼都沒印——log 被關在 container 裡的 `/tmp/app.log`，只能 `docker exec to-file tail /tmp/app.log` 才看得到，container 一刪就沒了。收拾：

```bash
docker rm -f to-file to-stdout
```

### 步驟 5：`inspect` + `--format`

整包 JSON（很長，先看一眼就好）：

```bash
docker inspect web
```

取特定欄位：

```bash
docker inspect web --format '{{.State.Status}}'
docker inspect web --format '{{.Config.Image}} started at {{.State.StartedAt}}'
docker inspect web --format '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}'
docker inspect web --format '{{json .NetworkSettings.Ports}}'
docker inspect pg --format '{{json .Config.Env}}'
```

範例輸出：

```
running
nginx:1.27-alpine started at 2026-10-28T08:10:01.912345678Z
172.17.0.2
{"80/tcp":[{"HostIp":"0.0.0.0","HostPort":"8080"},{"HostIp":"::","HostPort":"8080"}]}
["POSTGRES_PASSWORD=secret","PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin","LANG=en_US.utf8","PG_MAJOR=17","PG_VERSION=17.6",...,"PGDATA=/var/lib/postgresql/data"]
```

最後一行說明兩件事：

1. 你用 `-e` 傳的值和 image 自帶的 `ENV` 合在一起了。
2. **密碼明碼就躺在 `inspect` 裡**，任何能跑 `docker inspect` 的人都看得到。第 14 章會講怎麼處理 secrets。

`172.17.0.2` 是 container 在預設 bridge network 上的 IP。不要在程式裡寫死它，第 12 章會用 container 名字互連。

port 對應有專門的捷徑：

```bash
docker port web
```

範例輸出：

```
80/tcp -> 0.0.0.0:8080
80/tcp -> [::]:8080
```

> **PowerShell 的另一種做法**：把 JSON 轉成物件再取欄位，不用記 Go template：
>
> ```powershell
> (docker inspect web | ConvertFrom-Json).State.Status
> ```
>
> bash 有裝 `jq` 的話也可以 `docker inspect web | jq '.[0].State.Status'`。

> **引號**：`--format` 的模板用單引號，Git Bash 和 PowerShell 都沒問題；`cmd.exe` 要改雙引號。

### 步驟 6：`stats` 和 `top`

```bash
docker stats --no-stream
```

範例輸出：

```
CONTAINER ID   NAME   CPU %   MEM USAGE / LIMIT     MEM %   NET I/O           BLOCK I/O        PIDS
8c1e2f3a4b5c   web    0.00%   9.1MiB / 7.6GiB       0.12%   5.2kB / 4.8kB     0B / 4.1kB       9
2b3c4d5e6f7a   pg     0.02%   38.5MiB / 7.6GiB      0.49%   1.1kB / 0B        0B / 49.2MB      6
```

- 拿掉 `--no-stream` 就是即時更新的畫面（像 `top`），`Ctrl+C` 離開。
- `LIMIT` 目前是整台 Docker 主機（Docker Desktop 的 VM）的記憶體，因為還沒設限制。第 15 章會用 `--memory` 限制它。
- 只想看幾欄：`docker stats --no-stream --format 'table {{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}'`

```bash
docker top web
```

範例輸出：

```
UID      PID      PPID     C    STIME   TTY   TIME       CMD
root     23101    23078    0    08:10   ?     00:00:00   nginx: master process nginx -g daemon off;
101      23150    23101    0    08:10   ?     00:00:00   nginx: worker process
101      23151    23101    0    08:10   ?     00:00:00   nginx: worker process
```

和步驟 2 在 container 裡看到的 `ps` 比較：裡面 nginx 是 PID 1，從外面看卻是 PID 23101——同一個 process，在不同 PID namespace 裡有不同編號（第 01 章）。`docker top` 的好處是不需要 container 裡有 `ps`，distroless 也能用。

### 步驟 7：`cp`

從 container 拿檔案出來：

```bash
docker cp web:/etc/nginx/conf.d/default.conf tutorials/05-exec-logs-inspect/default.conf
```

範例輸出：

```
Successfully copied 3.07kB to tutorials/05-exec-logs-inspect/default.conf
```

把本章附的 `index.html` 放進去：

```bash
docker cp tutorials/05-exec-logs-inspect/index.html web:/usr/share/nginx/html/index.html
```

重新整理 <http://localhost:8080>，首頁變成「這頁是用 docker cp 放進來的」。

但這個改動只存在 `web` 的可寫層（第 03 章）：

```bash
docker rm -f web
docker run -d --name web -p 8080:80 nginx:1.27-alpine
```

再重新整理，又變回 Welcome to nginx!。`docker cp` 適合**除錯時臨時**拿放檔案；要正式把檔案放進 image 是 Dockerfile 的 `COPY`（第 06 章），開發時要同步檔案是 bind mount（第 13 章）。`docker cp` 對停止的 container 也能用，適合把「死掉的 container」裡的檔案救出來。

### 收拾

```bash
docker rm -f web pg
rm tutorials/05-exec-logs-inspect/default.conf
```

> **PowerShell**：第二行改成 `Remove-Item tutorials/05-exec-logs-inspect/default.conf`。

## 對照 Node／TypeScript

| Node 世界 | Docker 世界 |
|-----------|------------|
| `ssh` 到伺服器，在裡面下指令 | `docker exec -it <name> sh` |
| `pm2 logs api` / `pm2 logs api --lines 50` | `docker logs -f api` / `docker logs --tail 50 api` |
| `console.log` / `pino()` 預設寫 stdout | 正是 Docker 要的 |
| `winston` 的 `File` transport 寫 `logs/app.log` | 在 container 裡是反模式，改用 Console transport |
| `pm2 monit` / `top` | `docker stats` / `docker top` |
| `pm2 show api`（看設定、路徑、環境變數） | `docker inspect api` |
| `scp` 抓伺服器上的檔案 | `docker cp` |
| `node --inspect` + Chrome DevTools | 要 `-p 9229:9229` 把 debug port 開出來（第 12 章講 port） |

Node 專案最常見的錯：在 Express 加了一個寫檔案的 logger，container 裡 log 一直長，`docker logs` 卻空空的。**在 container 裡，`console.log` 就是最好的 log transport**；要結構化就用 pino 輸出 JSON 到 stdout，跟 shorty 的 `slog.NewJSONHandler(os.Stdout, ...)` 是同一個概念。

## 常見陷阱

1. **exec 進停止的 container**：`Error response from daemon: container ... is not running`。先 `docker logs` 看它為什麼死，或 `docker start` 再 exec。
2. **image 裡沒有 shell**：`exec: "sh": executable file not found in $PATH`。distroless、scratch 的 image 都這樣（shorty 的最終版也是）。用 `docker logs`、`docker inspect`、`docker top`、`docker cp` 這些「從外面看」的工具；Docker Desktop 也提供 `docker debug` 指令附加除錯工具（第 16、21 章會提）。
3. **以為 `Ctrl+C` 會停掉 `logs -f` 追蹤的 container**：不會，只是停止追蹤。但 `docker attach` 時的 `Ctrl+C` 是送給主 process 的，可能真的停掉 container。
4. **Git Bash 改寫路徑**：`docker exec web ls /etc` 變成去找 `C:/Program Files/Git/etc`。用 `MSYS_NO_PATHCONV=1`、在路徑前多加一個斜線（`//etc`），或用 PowerShell。
5. **在 container 裡用 exec 修 bug**：`exec` 進去改設定、`apk add` 裝工具，能暫時救火，但下次重建就沒了，而且環境跟 image 不一致。修好後一定要回去改 Dockerfile。
6. **`inspect` 裡看得到密碼**：`-e` 傳的密碼、`.Config.Env` 一覽無遺。不要以為「沒寫在 Dockerfile 就安全」。
7. **`docker logs` 越來越慢、磁碟越來越滿**：預設的 `json-file` logging driver 沒有大小上限。第 25 章會設 `max-size`／`max-file`。
8. **`--format` 模板打錯欄位名**：會出現 `template: :1:8: executing "" at <.State.Stauts>: map has no entry for key "Stauts"`。欄位名大小寫要跟 `docker inspect` 的 JSON 完全一樣。

## 練習

1. 跑一個 `redis:7-alpine`（名字 `cache`），用 `docker exec -it cache redis-cli` 進去執行 `SET hello world`、`GET hello`，再用不帶 `-it` 的方式 `docker exec cache redis-cli GET hello` 拿到同樣的值。
2. 用 `docker inspect` + `--format` 一行印出 `cache` 的：名字、狀態、IP、image。（提示：`{{.Name}}` 前面會有個 `/`。）
3. 開兩個終端機：一個跑 `docker logs -f web`，另一個用 `curl` 打 `http://localhost:8080/nope`，觀察 404 的 log 出現在哪裡。用 `docker logs web 2>/dev/null`（PowerShell：`2>$null`）只看 stdout，access log 和 error log 分別在哪一邊？
4. 用 `docker stats` 觀察 `pg`，同時 `docker exec pg psql -U postgres -c 'select count(*) from generate_series(1, 5000000);'`，看 CPU 欄的變化。
5. 用 `docker cp` 把 `pg` 的 `/var/lib/postgresql/data/postgresql.conf` 拷到主機，找出 `max_connections` 的預設值。
6. 收拾所有練習用的 container。

## 下一章

→ [06. Dockerfile 基本指令，shorty 第一版](../06-dockerfile-basics)：不再只用別人的 image——開始把 shorty 包成自己的 image。
