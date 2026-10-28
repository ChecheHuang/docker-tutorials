# 15. 資源限制與 restart policy

> 範圍：`--memory`／`--memory-swap`／`--cpus`、`docker stats`、OOM killed（exit 137）、Go 的 `GOMEMLIMIT`、restart policy（`no`／`on-failure`／`unless-stopped`／`always`）、`docker update`

> 🧸 想先用故事直覺理解這章？看 [STORY.md](./STORY.md)（小學生都懂的比喻版）。

## 為什麼需要這個

到上一章為止，我們跑的每個 container 都**沒有任何上限**：它可以用光整台機器的記憶體和 CPU。一台機器上同時跑 shorty、Postgres、Redis，只要其中一個有 memory leak，大家一起陪葬——而且是 kernel 隨便挑一個殺掉，不一定殺到兇手。

另一個問題：shorty 掛了（例如 DB 連不上、重試 10 次後放棄），它就停在 `Exited (1)`，直到有人發現、手動 `docker start`。

這章做兩件事：

1. **限制資源**：每個 container 有自己的記憶體與 CPU 預算，超過就只影響它自己。
2. **自動重啟**：用 restart policy 讓 Docker 在 container 掛掉時把它拉起來。

## 核心概念

### 限制是 cgroup 做的

```
host 的記憶體 8 GiB
├── shorty   cgroup  memory.max = 128 MiB   ← 超過：kernel 在這個 cgroup 裡挑行程殺掉
├── db       cgroup  memory.max = 512 MiB
├── cache    cgroup  memory.max = 64 MiB
└── 其他 host 上的程式
```

`--memory` 等旗標會被 Docker 寫進 Linux cgroup 的設定。**這是硬上限**：程式要不到記憶體時不會收到「記憶體不足」的錯誤，而是被 kernel 的 OOM killer 直接 `SIGKILL`。

### 常用旗標

| 旗標 | 意思 | 範例 |
|------|------|------|
| `--memory`（`-m`） | 記憶體上限 | `--memory 128m` |
| `--memory-swap` | 記憶體 **+ swap** 的總上限；設成跟 `--memory` 一樣 = 不准用 swap | `--memory-swap 128m` |
| `--memory-reservation` | 軟性下限，機器記憶體緊張時才生效 | `--memory-reservation 64m` |
| `--cpus` | 可用的 CPU 數量（可以是小數） | `--cpus 0.5` = 每 100ms 最多用 50ms CPU 時間 |
| `--cpu-shares` | CPU 搶不過來時的相對權重（預設 1024），不是上限 | `--cpu-shares 512` |
| `--cpuset-cpus` | 只能跑在哪幾顆 CPU | `--cpuset-cpus 0,1` |
| `--pids-limit` | 最多幾個行程／執行緒，防 fork bomb | `--pids-limit 100` |

**只設 `--memory` 不設 `--memory-swap` 的陷阱**：此時預設可以再用等量的 swap（總共 2 倍）。Docker Desktop 的 VM 預設有 swap，結果就是「限制 128m，實際吃到 256m 還沒被殺，只是變超慢」。所以本章一律兩個一起設。

### OOM killed 與 exit 137

```
exit code = 128 + 訊號編號
          = 128 + 9（SIGKILL）
          = 137
```

看到 137 代表 container 的主行程被 `SIGKILL` 殺掉。可能原因有三個：

| 原因 | 怎麼分辨 |
|------|---------|
| OOM killed | `docker inspect --format '{{.State.OOMKilled}}'` 是 `true` |
| 有人 `docker kill` | `OOMKilled` 是 `false` |
| `docker stop` 等 10 秒程式還不結束，被強制殺掉 | `OOMKilled` 是 `false`；第 16 章詳談 |

### Go 與記憶體上限：`GOMEMLIMIT`

Go 的 GC 預設看的是 `GOGC=100`：「heap 長到上次 GC 後存活量的 2 倍就收一次」。它**不知道** container 只有 128 MiB。

```
沒設 GOMEMLIMIT：
  存活 70 MiB → GC 目標 140 MiB → 還沒到就超過 128 MiB → OOM killed
  （其實有一半是垃圾，GC 一跑就能回收，可惜來不及）

設 GOMEMLIMIT=100MiB：
  Go runtime 管理的記憶體接近 100 MiB 時，GC 會更積極地回收
  → 留 28 MiB 給 runtime 以外的開銷（執行檔本身、cgo、kernel 帳上的東西）
```

- 經驗法則：設成 `--memory` 的 **80～90%**。單位是 `B`、`KiB`、`MiB`、`GiB`（注意是 **`MiB`**，不是 `MB` 或 `m`）。
- 它是**軟限制**：存活的資料真的超過上限時，GC 再努力也沒用，照樣 OOM。它解決的是「垃圾來不及收」，不是 memory leak。
- 環境變數設在 container 上（`-e GOMEMLIMIT=100MiB`），image 不用改；這就是第 14 章「設定從外面給」的好處。
- CPU 方面：Go 1.25 起，Linux 上的 runtime 會依 cgroup 的 CPU 上限自動調整 `GOMAXPROCS`，shorty 用 Go 1.26 所以 `--cpus` 不用額外設定。舊版 Go 常見的做法是用 `go.uber.org/automaxprocs`。

### restart policy

| policy | container 自己結束時 | 你 `docker stop` 之後 | Docker daemon 重啟（或 Docker Desktop 重開）時 |
|--------|---------------------|----------------------|---------------------------------------------|
| `no`（預設） | 不重啟 | — | 不啟動 |
| `on-failure[:N]` | exit code 非 0 才重啟，最多 N 次 | 不重啟 | 之前是在跑的才啟動 |
| `unless-stopped` | 一律重啟 | 不重啟 | **你手動停掉的不啟動**，其他都啟動 |
| `always` | 一律重啟 | 不重啟（但…） | **連你手動停掉的也會啟動** |

- 重啟有**退避**：等待時間從 100ms 開始每次加倍（有上限）；container 成功跑超過 10 秒後，退避時間會重設。所以「一啟動就掛」的 container 不會把 CPU 吃滿，但會一直 `Restarting`。
- **`docker stop` 永遠有效**：restart policy 不會跟你搶，手動停的就是停了（`always` 只是 daemon 重啟時會再把它叫起來）。
- 服務類 container（shorty、db、cache）一般用 **`unless-stopped`**；跑完就該結束的工作用 `on-failure:N`。
- `--rm` 和 `--restart`（`no` 以外）不能同時用：一個說「結束就刪掉」、一個說「結束就重啟」。

## shorty 在這章

把 shorty 的啟動指令加上：

```bash
--memory 128m --memory-swap 128m \
--cpus 0.5 \
--pids-limit 100 \
-e GOMEMLIMIT=100MiB \
--restart unless-stopped
```

128 MiB 對 shorty 綽綽有餘（閒置時不到 10 MiB），這裡是在畫一條「再多就是出事了」的線。根目錄 `compose.yaml` 的 `deploy.resources.limits.memory: 128M` 和 `restart: unless-stopped` 就是同一件事（第 17、18 章）。

本章附 [`oom-demo.sh`](./oom-demo.sh)：用 `python:3.13-alpine` 在 64 MiB 的限制下硬要 256 MiB，穩定重現 OOM killed。

## 動手做

以下指令都從 repo 根目錄執行。先把第 12～14 章的環境準備好（只需要 network、db、cache）：

```bash
docker network create shorty-net 2>/dev/null || true
docker run -d --name db --network shorty-net --restart unless-stopped \
  --memory 512m --memory-swap 512m \
  -e POSTGRES_USER=shorty -e POSTGRES_PASSWORD=shorty -e POSTGRES_DB=shorty \
  -v shorty-db:/var/lib/postgresql/data postgres:17-alpine
docker run -d --name cache --network shorty-net --restart unless-stopped \
  --memory 64m --memory-swap 64m redis:7-alpine
docker build -t shorty app
```

### 1. 帶著限制啟動 shorty

```bash
docker run -d --name shorty --network shorty-net -p 8080:8080 \
  --memory 128m --memory-swap 128m --cpus 0.5 --pids-limit 100 \
  -e GOMEMLIMIT=100MiB \
  --restart unless-stopped \
  -e BASE_URL=http://localhost:8080 \
  -e "DATABASE_URL=postgres://shorty:shorty@db:5432/shorty?sslmode=disable" \
  -e REDIS_URL=redis://cache:6379/0 \
  shorty
```

### 2. `docker stats` 看實際用量

```bash
docker stats --no-stream
```

範例輸出：

```
CONTAINER ID   NAME     CPU %     MEM USAGE / LIMIT   MEM %     NET I/O           BLOCK I/O     PIDS
4c1e9a7b2d10   shorty   0.00%     5.8MiB / 128MiB     4.53%     3.1kB / 2.4kB     0B / 0B       7
8a2f0c3d9e11   cache    0.35%     3.9MiB / 64MiB      6.09%     2.8kB / 1.1kB     0B / 0B       6
b7d3e1f0a412   db       0.01%     38.2MiB / 512MiB    7.46%     4.2kB / 3.0kB     0B / 1.2MB    7
```

`LIMIT` 欄位是你設的上限；沒設的 container 會顯示整台機器（或 Docker Desktop VM）的記憶體。拿掉 `--no-stream` 就是即時更新（`Ctrl+C` 離開）。

用 `inspect` 確認設定（單位是 bytes、以及 CPU 的十億分之一）：

```bash
docker inspect shorty --format 'Memory={{.HostConfig.Memory}} NanoCpus={{.HostConfig.NanoCpus}} Restart={{.HostConfig.RestartPolicy.Name}}'
```

範例輸出：

```
Memory=134217728 NanoCpus=500000000 Restart=unless-stopped
```

### 3. 重現 OOM killed（exit 137）

```bash
bash tutorials/15-resources-restart/oom-demo.sh
```

範例輸出：

```
限制 --memory=64m（關掉 swap），嘗試配置 256MiB ...
Status=exited ExitCode=137 OOMKilled=true
```

腳本的核心只有一行，你也可以手動跑：

```bash
docker run --name hog --memory 64m --memory-swap 64m python:3.13-alpine \
  python -c "x = b'x' * (256 * 1024 * 1024)"
docker ps -a --filter name=hog      # STATUS: Exited (137)
docker inspect hog --format '{{.State.OOMKilled}}'
docker rm hog
```

注意 python 沒有機會印出任何錯誤訊息——`SIGKILL` 無法被攔截。**這就是 OOM 難查的原因：程式的 log 裡什麼都沒有**，只能看 exit code 和 `OOMKilled`。

想即時看到 OOM 事件，另開一個終端機：

```bash
docker events --filter event=oom
```

把 `oom-demo.sh` 的參數改成 `bash tutorials/15-resources-restart/oom-demo.sh 64m 32` 再跑一次——配置量小於上限，就會印出「居然活下來了」。

### 4. restart policy：`on-failure:3`

```bash
docker run -d --name flaky --restart on-failure:3 alpine:3.22 \
  sh -c 'echo "start $(date +%T)"; sleep 1; exit 1'
```

等十秒左右：

```bash
docker logs flaky
docker inspect flaky --format 'RestartCount={{.RestartCount}} Status={{.State.Status}} ExitCode={{.State.ExitCode}}'
```

範例輸出：

```
start 09:01:10
start 09:01:11
start 09:01:13
start 09:01:16
RestartCount=3 Status=exited ExitCode=1
```

第一次 + 重啟 3 次 = 4 行，間隔越來越長（退避）。把 `exit 1` 改成 `exit 0` 再試：`on-failure` 對正常結束不會重啟。

### 5. shorty 連不上 DB 時會怎樣

```bash
docker stop db
docker restart shorty
docker logs -f shorty
```

shorty 會重試 10 次、約 10 秒後以 exit 1 結束，接著 `unless-stopped` 把它重新拉起來，又重試 10 次……用 `docker ps` 會看到 `Restarting (1) ...` 或 `Up 3 seconds`。這時把 DB 開回來：

```bash
docker start db
```

下一輪 shorty 就連上了。這種「靠重啟撐過依賴還沒好」是最基本的韌性；更好的做法（healthcheck、`depends_on: condition: service_healthy`）在第 16、18 章。

### 6. OOM + restart 的組合：重啟迴圈

```bash
docker run -d --name hog-loop --restart always --memory 64m --memory-swap 64m \
  python:3.13-alpine python -c "x = b'x' * (256 * 1024 * 1024)"
docker ps --filter name=hog-loop
docker inspect hog-loop --format 'RestartCount={{.RestartCount}}'
```

範例輸出：

```
CONTAINER ID   IMAGE                COMMAND                  CREATED          STATUS                          PORTS     NAMES
9e0d1c2b3a4f   python:3.13-alpine   "python -c 'x = b'x'…"   20 seconds ago   Restarting (137) 3 seconds ago            hog-loop
RestartCount=6
```

restart policy 不會修好 OOM，只會讓它**一直重來**。線上看到 `Restarting (137)` 就是這個情況——第 25 章的事故演練會從這裡開始排查。

```bash
docker rm -f hog-loop
```

### 7. `docker update`：不重建就改限制

```bash
docker update --memory 256m --memory-swap 256m --cpus 1 shorty
docker update --restart on-failure:5 flaky
docker inspect shorty --format 'Memory={{.HostConfig.Memory}} NanoCpus={{.HostConfig.NanoCpus}}'
```

範例輸出：

```
shorty
flaky
Memory=268435456 NanoCpus=1000000000
```

`docker update` 只能改**資源限制和 restart policy**。環境變數（包括 `GOMEMLIMIT`）、port、network、volume 都不行，要改只能重建 container。所以調高 `--memory` 時記得：shorty 的 `GOMEMLIMIT` 還是 100MiB，要一起調就得重建。

### 8. 收拾

```bash
docker rm -f shorty flaky db cache
docker network rm shorty-net
```

（`shorty-db` volume 留著，第 16 章還會用到；不要的話 `docker volume rm shorty-db`。）

## 對照 Node／TypeScript

| Go | Node |
|----|------|
| `GOMEMLIMIT=100MiB` | `NODE_OPTIONS=--max-old-space-size=100`（單位 MB，只管 V8 heap） |
| 超過上限 → 被 kernel OOM kill，exit 137，沒有 log | V8 heap 先到上限 → `FATAL ERROR: Reached heap limit` 自己崩潰（有 log）；container 上限先到 → 一樣 exit 137 |
| Go 1.25+ 依 `--cpus` 自動設 `GOMAXPROCS` | Node 主執行緒本來就單線程；`os.cpus().length` 回報的是主機的 CPU 數（`os.availableParallelism()` 依 Node／libuv 版本不同，也不一定反映 `--cpus`），用它決定 cluster／worker 數量時要自己把 `--cpus` 算進去 |
| restart policy | 在 container 裡**不要**再套 `pm2`／`forever` 做重啟；讓 Docker（或 K8s）負責。一個 container 一個行程 |

重點：Node 的 `--max-old-space-size` 要設得比 container 上限**小**，留空間給 Buffer、原生模組和 V8 以外的記憶體——跟 `GOMEMLIMIT` 留 10～20% 是同樣的道理。

## 常見陷阱

- **只設 `--memory` 沒設 `--memory-swap`**：可以偷用 swap，該被殺的不被殺，只是變得超慢。測試 OOM 行為時尤其要設。
- **以為 137 一定是 OOM**：先看 `OOMKilled`。`docker stop` 超時也是 137。
- **OOM 了但 `ExitCode` 不是 137**：如果被殺的是**子行程**而不是 PID 1，主行程可能自己用別的 code 結束。例如常見的壓力測試 image `polinux/stress`：它 fork 出 worker 吃記憶體，被殺的是 worker，`stress` 本身會印出 `got signal 9` 後以 exit 1 結束。shorty 是單一行程，被殺就是 137。
- **`GOMEMLIMIT` 單位寫錯**：`GOMEMLIMIT=100m` 或 `100MB` 是無效值，Go runtime 會在啟動時報錯。要寫 `100MiB`。
- **`GOMEMLIMIT` 設得跟 `--memory` 一樣**：沒有留空間給 Go runtime 以外的記憶體，等於沒設。
- **`--cpus 0.5` 讓延遲變高**：CPU 限制是「每 100ms 最多跑 50ms」，用完了就被暫停（throttling）到下一個週期。尖峰時 p99 延遲會明顯上升。`docker stats` 的 CPU% 卡在 50% 就是在被限流。
- **restart policy 掩蓋問題**：`docker ps` 看到 `Up 2 seconds` 以為沒事，其實它每 10 秒掛一次。習慣看 `RestartCount`。
- **`docker update --memory` 報錯 `Memory limit should be smaller than already set memoryswap limit`**：之前設過 `--memory-swap`，調高 `--memory` 時要同時調 `--memory-swap`。
- **Windows／Docker Desktop**：
  - 所有 container 的上限加起來，不能超過 Docker Desktop 那台 VM 本身的記憶體。WSL2 後端由 `%UserProfile%\.wslconfig` 的 `[wsl2]` 區塊 `memory=` 決定（預設約為 Windows 記憶體的一半）；Hyper-V 後端在 Settings → Resources 調整。
  - `docker stats` 在沒設上限的 container 上顯示的 `LIMIT` 是 VM 的記憶體，不是你電腦的。
  - `unless-stopped`／`always` 的「daemon 重啟」在 Docker Desktop 上就是「Docker Desktop 重開」。勾了「登入 Windows 時啟動 Docker Desktop」的話，開機後這些 container 會自己起來——有時你會疑惑「這個 container 為什麼又在跑」，就是這個原因。
- **Linux（舊環境）**：使用 cgroup v1 的舊版發行版，`--memory-swap` 可能出現 `Your kernel does not support swap limit capabilities` 警告，需要開核心參數。現代發行版預設是 cgroup v2，沒有這個問題。

## 練習

1. 用 `oom-demo.sh` 找出 `python:3.13-alpine` 在 `--memory 64m` 下大約能配置多少 MiB 才會被殺（二分搜尋）。為什麼不是剛好 64？
2. 拿掉 `--memory-swap`，只留 `--memory 64m` 跑 `oom-demo.sh` 的核心指令，結果有什麼不同？（在沒有 swap 的 Linux 機器上可能沒差別，想想為什麼。）
3. 把 shorty 設成 `--cpus 0.1`，用 `for i in $(seq 200); do curl -s -o /dev/null localhost:8080/healthz; done` 打一輪，同時看 `docker stats` 的 CPU%。再改回 `--cpus 1` 比較花費時間。
4. 建一個 `--restart always` 的 container 和一個 `--restart unless-stopped` 的 container，兩個都 `docker stop`，然後重開 Docker Desktop（或 Linux 上 `sudo systemctl restart docker`）。哪個自己起來了？
5. 用 `docker update` 把 `flaky` 從 `on-failure:3` 改成 `no`，再 `docker start flaky`，確認它不會再重啟。

## 下一章

restart policy 只看「行程還在不在」。但 shorty 可能行程活著、卻連不上 DB（`/readyz` 回 503）；`docker stop` 時它又需要時間把進行中的請求處理完。第 16 章 [healthcheck-shutdown](../16-healthcheck-shutdown/) 處理 `HEALTHCHECK`、liveness vs readiness、PID 1 與訊號，以及 shorty 的 graceful shutdown。
