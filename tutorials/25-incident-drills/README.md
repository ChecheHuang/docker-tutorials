# 25. 事故演練

> 範圍：OOM 循環重啟（exit 137、`OOMKilled`）、磁碟被 image／build cache／log 塞滿（`system df`、prune、json-file log rotation、`daemon.json`）、時區錯誤（tzdata、`time/tzdata`、`TZ`）、container 內 DNS 失敗（預設 bridge、公司 DNS／VPN）、port 衝突、殭屍行程（PID 1 不收屍、`--init`）

> 🧸 想先用故事直覺理解這章？看 [STORY.md](./STORY.md)（小學生都懂的比喻版）。

## 為什麼需要這個

前 24 章教的是「怎麼做對」。這一章反過來：**故意把它弄壞，練習怎麼找出原因**。

真正的事故發生時，你不會有時間翻文件。每一個演練都照同樣的格式：

- **症狀**：你在監控、log、使用者回報裡看到什麼
- **重現步驟**：在本機安全地重現一次
- **排查流程**：從症狀一路追到原因的指令
- **修正**：當下怎麼止血
- **預防**：怎麼讓它不再發生

## 核心概念：排查的共同套路

不管哪種事故，前三個指令幾乎都一樣：

```bash
docker ps -a                     # 狀態：Up？Restarting？Exited (137)？
docker logs --tail 50 <name>     # 它死前說了什麼
docker inspect <name>            # State、RestartCount、HostConfig、Mounts、Networks
```

再依照症狀往下：資源看 `docker stats`／`docker events`，磁碟看 `docker system df`，網路用 `--network container:<name>` 借一個有工具的 container 進去看。

## shorty 在這章

本章目錄的每個子目錄是一個演練，全部從 repo 根目錄執行：

| 目錄 | 演練 |
|------|------|
| `oom/compose.yaml` | 64MB 上限 + 吃記憶體的 process，`on-failure:3` 重啟 |
| `disk/compose.yaml`、`disk/daemon.json` | 有／沒有 log rotation 的兩個吵鬧 container；daemon 層級設定範例 |
| `tz/Dockerfile.scratch` | shorty 放進 scratch，`TZ` 失效；`GOTAGS=timetzdata` 修正 |
| `dns/compose.broken.yaml` | `network_mode: bridge` 讓 shorty 找不到 `db` |
| `zombie/compose.yaml` | 有／沒有 `init: true` 的殭屍行程對照 |

port 衝突只需要兩行指令，直接寫在下面。

---

## 演練 1：OOM 循環重啟

**症狀**：服務每隔幾秒到幾分鐘就斷一下；`docker ps` 的 STATUS 一直是 `Up 3 seconds`、`Restarting (137)`；log 沒有任何錯誤訊息就突然中斷。

**重現步驟**

```bash
docker compose -f tutorials/25-incident-drills/oom/compose.yaml up -d
docker compose -f tutorials/25-incident-drills/oom/compose.yaml ps -a
```

範例輸出：

```
NAME        IMAGE         COMMAND                  SERVICE   STATUS
oom-hog-1   alpine:3.22   "sh -c 'echo 'starti…"   hog       Exited (137) 2 seconds ago
```

**排查流程**

```bash
docker inspect --format 'oom={{.State.OOMKilled}} exit={{.State.ExitCode}} restarts={{.RestartCount}}' oom-hog-1
# oom=true exit=137 restarts=3

docker events --filter event=oom --since 10m     # 另開一個 terminal 看即時事件
docker stats --no-stream                         # MEM USAGE / LIMIT 是不是貼著上限
```

- exit 137 = 128 + 9，代表被 **SIGKILL** 殺掉。原因可能是 OOM，也可能是 `docker kill`、或 `docker stop` 等太久。`OOMKilled=true` 才能確定是記憶體。
- kernel 的 OOM killer 不會通知 process，所以 app 的 log 裡**不會**有錯誤，只會突然中斷。
- 循環重啟期間 `State` 會被下一次啟動覆蓋，用 `docker events` 或在 restart 次數用完後再 inspect 比較準。

**修正**：先把 limit 調高止血（`docker update --memory 256m --memory-swap 256m <name>` 可以不重建 container），再找出是真的需要那麼多，還是 memory leak。

**預防**

- Go 服務設 `GOMEMLIMIT`（約 limit 的 80–90%，例如 limit `128M` 配 `GOMEMLIMIT=110MiB`），讓 GC 在接近上限前更積極回收（第 15 章）。
- `/healthz` + 監控 restart 次數，循環重啟要告警，不要靠使用者發現。
- `restart: unless-stopped` 會讓 OOM 看起來像「偶爾閃一下」；把 `RestartCount` 納入監控。

---

## 演練 2：磁碟被 image／build cache／log 塞滿

**症狀**：`docker build` 失敗 `no space left on device`；Postgres 寫不進去；Docker Desktop 的虛擬磁碟變得超大；主機 `/var/lib/docker` 佔滿整顆碟。

**重現步驟**

```bash
docker compose -f tutorials/25-incident-drills/disk/compose.yaml up -d
# 等一兩分鐘
```

**排查流程**

```bash
docker system df
```

範例輸出：

```
TYPE            TOTAL     ACTIVE    SIZE      RECLAIMABLE
Images          38        6         14.2GB    11.8GB (83%)
Containers      9         3         120MB     80MB (66%)
Local Volumes   12        4         3.1GB     2.2GB (70%)
Build Cache     214       0         9.6GB     9.6GB
```

`docker system df -v` 看每一項的細節。注意 **log 不算在這張表裡**，要另外查：

```bash
docker inspect --format '{{.LogPath}}' disk-noisy-1 disk-rotated-1

# Linux 主機：直接看檔案大小
sudo du -h $(docker inspect --format '{{.LogPath}}' disk-noisy-1)

# Docker Desktop：log 在 VM 裡，借一個特權 container 進 VM 看
docker run --rm --privileged --pid=host debian:bookworm-slim \
  nsenter -t 1 -m -- sh -c 'du -sh /var/lib/docker/containers/*/*-json.log'
```

`noisy` 的 log 會一直長大，`rotated` 停在約 3MB。

**修正**（由安全到危險）

```bash
docker builder prune --filter until=72h     # 3 天前的 build cache
docker image prune                          # dangling image（<none>:<none>）
docker image prune -a --filter until=168h   # 一週沒用到的 image（會刪掉沒有 container 在用的）
docker container prune                      # 已停止的 container
docker volume prune                         # ⚠️ 沒被使用的 volume，資料會消失，先確認
docker compose -f tutorials/25-incident-drills/disk/compose.yaml down
```

已經失控的 log 檔：重建 container（log 跟著刪）最乾淨；不能停機時才 `truncate -s 0 <LogPath>`（Linux）。

**預防**

- **daemon 層級**設預設 log rotation（Linux：`/etc/docker/daemon.json`；Docker Desktop：Settings → Docker Engine），範例見 `disk/daemon.json`：

  ```json
  { "log-driver": "json-file", "log-opts": { "max-size": "10m", "max-file": "3" } }
  ```

  改完要重啟 daemon，而且**只對之後新建的 container 生效**，既有的要重建。
- 或在 compose 裡每個 service 寫 `logging:`（`disk/compose.yaml` 的 `rotated`）。也可以改用 `local` log driver，它預設就會 rotate。
- build cache 設 GC 上限（`daemon.json` 的 `builder.gc`），CI runner 定期 `docker system prune`。

---

## 演練 3：時區錯誤

**症狀**：log 時間差 8 小時；「每天凌晨 0 點」的排程在早上 8 點才跑；明明設了 `TZ=Asia/Taipei` 也沒用。

**重現步驟**

```bash
docker build -f tutorials/25-incident-drills/tz/Dockerfile.scratch -t shorty:scratch app
docker run --rm -e TZ=Asia/Taipei shorty:scratch
```

範例輸出（Ctrl+C 結束）：

```json
{"time":"2026-10-28T01:00:00.123Z","level":"INFO","msg":"listening","addr":":8080","version":"dev"}
```

時間結尾是 `Z`（UTC），`TZ` 被默默忽略。

**排查流程**

1. 確認 `TZ` 有傳進去：`docker inspect --format '{{json .Config.Env}}' <name>`。
2. 確認 image 裡有時區資料庫 `/usr/share/zoneinfo`：
   - `scratch`：**沒有**。
   - `alpine`：預設**沒有**，要 `apk add --no-cache tzdata`。
   - `debian`／`ubuntu`：有。
   - `gcr.io/distroless/static`（shorty 的 `app/Dockerfile`）：**有**，`TZ` 直接生效。
3. Go 的行為：`TZ` 指定的時區載入失敗時，**不報錯、直接用 UTC**。這就是為什麼很難發現。

對照 distroless 版：

```bash
docker run --rm -e TZ=Asia/Taipei shorty
# {"time":"2026-10-28T09:00:00.123+08:00", ...}
```

**修正**：image 沒有 tzdata 時，把時區資料編進執行檔：

```bash
docker build -f tutorials/25-incident-drills/tz/Dockerfile.scratch \
  --build-arg GOTAGS=timetzdata -t shorty:scratch-tz app
docker run --rm -e TZ=Asia/Taipei shorty:scratch-tz
# {"time":"2026-10-28T09:00:00.123+08:00", ...}
```

`-tags timetzdata` 等於在程式裡 `import _ "time/tzdata"`，執行檔約多 450KB。

**預防**

- **伺服器、log、資料庫一律存 UTC**，只有顯示給使用者時才轉當地時間。`TZ` 只是讓 log 好讀，不要讓商業邏輯依賴它。
- 需要固定時區的邏輯（「台北時間每天 0 點結算」）在程式裡明確 `time.LoadLocation("Asia/Taipei")`，並搭配 `time/tzdata` 確保一定載入得到。
- Linux 上偶爾看到 `-v /etc/localtime:/etc/localtime:ro`，它只在 Linux 主機有效，Docker Desktop 上不可靠；`TZ` 環境變數比較一致。
- Postgres 有自己的時區設定（`timezone` 參數），跟 container 的 `TZ` 是兩回事。

---

## 演練 4：container 內 DNS 失敗

**症狀**：`lookup db on 127.0.0.11:53: no such host`、`Temporary failure in name resolution`；container 連不到其他 container，或連不到外網／公司內網的服務。

**重現步驟**

```bash
docker compose -f tutorials/25-incident-drills/dns/compose.broken.yaml up -d --build
docker compose -f tutorials/25-incident-drills/dns/compose.broken.yaml logs app
```

範例輸出（節錄）：

```
{"level":"WARN","msg":"postgres not ready, retrying","attempt":1,"err":"failed to connect to `user=shorty database=shorty`: hostname resolving error: lookup db on 192.168.65.7:53: no such host"}
```

**排查流程**

distroless 沒有 `nslookup`，借一個 busybox 共用 shorty 的 network namespace：

```bash
APP=$(docker compose -f tutorials/25-incident-drills/dns/compose.broken.yaml ps -aq app)
# app 重試 10 次後就 exit 了，--network container:$APP 會失敗（container 不在跑）。
# 改在同一個預設 bridge 上測，這正是 app 所在的網路：
docker run --rm busybox nslookup db
docker run --rm busybox cat /etc/resolv.conf
docker inspect --format '{{json .NetworkSettings.Networks}}' $APP
```

判讀：

| 看到 | 代表 |
|------|------|
| `nameserver 127.0.0.11` | 在自訂 network，用 Docker 內建 DNS，container 名字解析得到 |
| 其他 nameserver（host 的 DNS） | 在**預設 bridge**，**沒有** container 名字解析 |
| `nslookup go.dev` 也失敗 | 連外 DNS 有問題：公司 DNS、VPN、防火牆 |

本例 `Networks` 只有 `bridge`：`network_mode: bridge` 把它放到預設 bridge 了。

**修正**：刪掉兩行 `network_mode: bridge`，讓 compose 用專案自己的 network（第 12、17 章）。`docker run` 的話，先 `docker network create` 再 `--network` 加入。

連外 DNS 失敗（常見於公司網路或 VPN）：

```bash
docker run --rm --dns 10.0.0.53 busybox nslookup intranet.example.com   # 單次指定
```

長期改 `daemon.json`（Docker Desktop：Settings → Docker Engine）：

```json
{ "dns": ["10.0.0.53", "1.1.1.1"] }
```

compose 的 service 也可以寫 `dns:`。Docker Desktop 開了 VPN 後抓不到公司 DNS，通常重啟 Docker Desktop 會重新讀取 host 的 DNS 設定。

**預防**：永遠用自訂 network；連線字串用 service 名稱而不是 IP；公司環境把 DNS 設定寫進 `daemon.json` 並放進團隊的環境建置文件。

---

## 演練 5：port 衝突

**症狀**：`docker run` 或 `compose up` 直接失敗。

**重現步驟**

```bash
docker run -d --name squatter -p 8080:80 nginx
docker run --rm -p 8080:8080 shorty
```

範例輸出（三種常見訊息）：

```
# 被另一個 container 佔用
Bind for 0.0.0.0:8080 failed: port is already allocated

# 被 host 上的程式佔用（例如 go run . 還開著）
listen tcp4 0.0.0.0:8080: bind: address already in use

# Windows：port 落在 Hyper-V／WSL 保留範圍
ports are not available: exposing port TCP 0.0.0.0:8080 -> 127.0.0.1:0: listen tcp 0.0.0.0:8080: bind: An attempt was made to access a socket in a way forbidden by its access permissions.
```

**排查流程**

```bash
docker ps --filter publish=8080                   # 是不是 container 佔的
```

不是 container 的話，看 host：

| OS | 指令 |
|------|------|
| Linux | `sudo ss -ltnp 'sport = :8080'` |
| macOS | `lsof -nP -iTCP:8080 -sTCP:LISTEN` |
| Windows | `netstat -ano \| findstr :8080`，再 `tasklist /FI "PID eq <PID>"` |
| Windows 保留範圍 | `netsh interface ipv4 show excludedportrange protocol=tcp` |

**修正**：停掉佔用者（`docker rm -f squatter`），或換 host port：`-p 18080:8080`——左邊 host port 隨便換，右邊 container port 不用動。

**預防**：compose 的 port 用變數 `"${APP_PORT:-8080}:8080"`，每個人可以在 `.env` 改；只需要本機存取的服務綁 `127.0.0.1:8080:8080`，順便不對外暴露。

---

## 演練 6：殭屍行程（PID 1 不收屍）

**症狀**：`ps` 看到一堆 `<defunct>` 或狀態 `Z` 的行程；久了 PID 用完（`fork: Resource temporarily unavailable`，配合 `pids_limit` 更快出現）；`docker stop` 每次都要等滿 10 秒。

**重現步驟**

```bash
docker compose -f tutorials/25-incident-drills/zombie/compose.yaml up -d
docker exec zombie-no-init-1 ps -o pid,ppid,stat,comm
docker exec zombie-with-init-1 ps -o pid,ppid,stat,comm
```

> 注意：這裡用 `docker exec ... ps`，不用 `docker top`。`docker top` 在目前的 Docker Engine 上不會列出 `<defunct>` 行程（已實測），`exec` 看得到 `Z` 狀態。

範例輸出（節錄）：

```
# no-init
PID   PPID  STAT  COMMAND
    1     0 S     sleep
    8     1 Z     sleep
   10     1 Z     sleep
   12     1 Z     sleep
   14     1 Z     sleep
   16     1 Z     sleep
   17     0 R     ps

# with-init
PID   PPID  STAT  COMMAND
    1     0 S     docker-init
    7     1 S     sleep
   18     0 R     ps
```

**排查流程**

1. 看殭屍的 PPID：全部指向 container 的 PID 1。
2. 看 PID 1 是誰：`docker inspect --format '{{.Path}} {{.Args}}' <name>`。如果是 shell script、`sleep`、或一般不懂得 `wait()` 的程式，就是它。
3. Linux 規則：孤兒行程會被過繼給 PID 1，**收屍（wait）是 PID 1 的責任**。一般主機上 PID 1 是 systemd，但 container 裡 PID 1 是你的程式。

**修正**：加 `--init`（compose：`init: true`），Docker 會用 tini 當 PID 1，負責收屍和轉發訊號。

**預防**

- 會 fork 子行程的 image（shell script 當 entrypoint、呼叫外部指令、Chrome／Puppeteer、某些 Python／Node 工具）一律加 `init: true`，或在 Dockerfile 用 `tini` 當 `ENTRYPOINT`。
- shorty 是 Go 單一 process、不 fork，而且 `ENTRYPOINT ["/shorty"]` exec form 自己處理 SIGTERM（第 16 章），所以 `app/Dockerfile` 不需要 init。
- 別忘了 `docker stop` 要等 10 秒這個副作用：不處理 SIGTERM 的 PID 1 會一直等到被 SIGKILL，`--init` 也能順便改善。

---

## 對照 Node／TypeScript

| 事故 | Node 的版本 |
|------|------|
| OOM | V8 heap 上限與 container limit 不一致：設 `NODE_OPTIONS=--max-old-space-size=<MB>`（約 limit 的 75%），角色同 Go 的 `GOMEMLIMIT` |
| 磁碟 | `console.log` 狂印 debug 訊息塞滿 json-file；`npm` cache 留在 image 裡 |
| 時區 | `node:*-alpine` 沒有 tzdata；Node 的 `Intl` 內建 ICU 時區資料，但 `new Date().toString()` 吃 `TZ` |
| DNS | `node:*-alpine` 用 musl 的 resolver，舊版 musl（1.2.4 以前）DNS 回應太大時不會改用 TCP 重查，某些內網網域會解析失敗；換 `-slim`（glibc）通常就好 |
| 殭屍 | `npm start` 當 PID 1：npm 不轉發 SIGTERM、也不收屍。用 `CMD ["node", "server.js"]` + `init: true` |

## 常見陷阱

1. **只看 exit code 不看 `OOMKilled`**：137 不一定是 OOM。
2. **`docker system prune -a --volumes` 當日常清理**：會刪掉 DB 的 volume。volume 要單獨、確認過再刪。
3. **改了 `daemon.json` 卻沒效果**：沒重啟 daemon，或既有 container 沒重建。JSON 格式錯誤會讓 daemon 起不來，改之前先備份。
4. **在 distroless 裡找 shell 排查**：用 `--network container:`、`--pid container:` 借工具 container，或用 `:debug` 變體。
5. **時區問題用 `TZ` 修程式邏輯**：換台機器、換個 base image 就壞。存 UTC，顯示時再轉。
6. **把 port 衝突「修」成 `--network host`**：只是把問題藏起來，還失去隔離。換 host port 就好。

## 練習

1. 把 `oom/compose.yaml` 的 `restart` 改成 `unless-stopped`，用 `watch docker ps` 觀察循環重啟，再用 `docker events --filter event=oom` 抓到它。
2. 用根目錄的 `compose.yaml` 跑 shorty，故意把 `memory` 改成 `16M`，看 shorty 能不能起來；再加 `GOMEMLIMIT` 觀察差異。
3. 在你的 Docker Desktop／Engine 設好 `daemon.json` 的 log rotation，重建 `disk-noisy-1` 確認它也被限制了。
4. 不用 `timetzdata`，改成在 `Dockerfile.scratch` 裡從 build stage `COPY` `/usr/share/zoneinfo`（提示：alpine build stage 要先 `apk add tzdata`）。
5. 修好 `dns/compose.broken.yaml`，用 `docker run --rm --network container:<app> busybox cat /etc/resolv.conf` 確認 nameserver 變成 `127.0.0.11`。
6. 寫一個自己的「事故卡」：挑一個你在工作上遇過的 container 問題，照 症狀／重現／排查／修正／預防 五段寫下來。

## 結業：接下來去哪裡

恭喜走完 25 章。shorty 從第 06 章那個近 1GB、用 root 跑的 image，變成現在 non-root、唯讀、多平台、CI 自動掃描發佈的上線版本，而且你練過它壞掉時怎麼救。

單機 Docker 與 compose 的極限在於：一台機器掛了服務就掛了，擴充、滾動更新、自動修復都要自己處理。這正是 Kubernetes 要解決的問題。本系列學到的 image、healthcheck（liveness／readiness）、資源限制、non-root、`read_only`、log 寫 stdout，在 K8s 都有一對一的對應欄位。

下一步，到兄弟 repo 接著學（這兩個是同層的其他 repo，不是本 repo 的路徑）：

- Go 路線：`go-tutorials/tutorials/38-kubernetes`（從本 repo 根目錄看是 `../go-tutorials/tutorials/38-kubernetes`）
- Rust 路線：`rust-tutorials/tutorials/48-kubernetes`（從本 repo 根目錄看是 `../rust-tutorials/tutorials/48-kubernetes`）

把第 24 章推上 GHCR 的 `ghcr.io/<你>/shorty:1.2.0` 拿去當第一個 Deployment 的 image 吧。
