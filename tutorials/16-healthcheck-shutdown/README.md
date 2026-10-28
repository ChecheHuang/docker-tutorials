# 16. 健康檢查與優雅關機

> 範圍：`HEALTHCHECK`、liveness vs readiness（`/healthz` vs `/readyz`）、distroless 沒有 curl 怎麼辦、具名 build context、PID 1 與訊號、exec form vs shell form、`docker stop` 的 10 秒與 SIGKILL、`docker stop -t`、`--init`／tini、`stop_grace_period`、shorty 的 graceful shutdown

> 🧸 想先用故事直覺理解這章？看 [STORY.md](./STORY.md)（小學生都懂的比喻版）。

## 為什麼需要這個

到第 15 章為止，shorty 已經「跑得起來」，也有 restart policy 幫它掛掉後重開。但還有兩個問題沒解決：

1. **Docker 只知道 process 還在不在，不知道它還能不能用。** shorty 的 process 可能活著，但 HTTP server 卡死、連不上 DB——`docker ps` 照樣顯示 `Up 3 hours`。我們需要一個「定期戳一下，看它有沒有回應」的機制。
2. **關機的方式決定會不會掉請求。** `docker stop`、`docker compose down`、K8s 滾動更新，都會先送 SIGTERM 給 container。程式如果收不到、或收到不理，Docker 等 10 秒就直接 SIGKILL——正在處理的請求斷掉、DB 連線沒關、log 沒寫完。

這章把兩件事一起處理：**怎麼回報「我還好嗎」**，以及**怎麼好好地說再見**。

## 核心概念

### HEALTHCHECK 指令

```dockerfile
HEALTHCHECK --interval=10s --timeout=3s --start-period=10s --start-interval=1s --retries=3 \
    CMD ["/probe"]
```

Docker 會在 container **裡面**每隔 `interval` 執行一次 `CMD`，看 exit code：

| exit code | 意思 |
|------|------|
| `0` | healthy |
| `1` | unhealthy |
| `2` | 保留，不要用 |

| 參數 | 預設值 | 說明 |
|------|------|------|
| `--interval` | 30s | 兩次檢查之間隔多久 |
| `--timeout` | 30s | 單次檢查超過多久算失敗 |
| `--start-period` | 0s | 啟動寬限期，這段時間的失敗不計入 retries |
| `--start-interval` | 5s | 寬限期內的檢查間隔（Docker Engine 25 以上） |
| `--retries` | 3 | 連續失敗幾次才變 `unhealthy` |

health 狀態有三種：`starting` → `healthy` ／ `unhealthy`，會顯示在 `docker ps` 的 STATUS 欄。

**重點：單機 Docker 看到 `unhealthy` 不會幫你重啟 container。** restart policy（第 15 章）只看 process 有沒有結束。health 狀態的用途是：

- 給人看（`docker ps`、監控）
- 給 Compose 的 `depends_on: condition: service_healthy` 用（第 18 章）
- 給 Swarm 用（Swarm 會替換 unhealthy 的 task）

### liveness vs readiness

shorty 有兩個健康端點（`app/internal/server/server.go`）：

| 端點 | 問的問題 | shorty 的實作 | 失敗時該怎麼處理 |
|------|------|------|------|
| `/healthz`（liveness） | 「你這個 process 還活著嗎？」 | 永遠回 `200 {"status":"ok"}` | 重啟它 |
| `/readyz`（readiness） | 「你現在能接客嗎？」 | Ping DB 和 Redis，任一失敗回 `503` | 先別把流量導給它，但**不要**重啟 |

為什麼要分開？假設 Postgres 重開機 30 秒：

- 如果拿 `/readyz` 當 liveness，所有 shorty 會同時被判定「死了」而被重啟——重啟也救不了 DB，反而讓服務整個消失，DB 回來時還要一起冷啟動。
- 正確做法是：liveness 只看自己（`/healthz`），readiness 才看依賴（`/readyz`），由 load balancer 或 K8s 暫時把流量移開。

Docker 的 `HEALTHCHECK` 只有一個槽，沒有 liveness/readiness 之分，所以本章的 probe **預設打 `/healthz`**。K8s 則有 `livenessProbe`、`readinessProbe`、`startupProbe` 三個，直接用 HTTP 打 `/healthz`、`/readyz`，不需要 container 裡有任何工具。

### distroless 沒有 curl 怎麼辦

網路上最常見的寫法是：

```dockerfile
HEALTHCHECK CMD curl -f http://localhost:8080/healthz || exit 1
```

這在 shorty 的 image 上完全行不通：`gcr.io/distroless/static-debian12` 沒有 `curl`、沒有 `wget`、連 `/bin/sh` 都沒有（上面那行是 shell form，需要 sh）。有四條路：

| 做法 | 優點 | 缺點 |
|------|------|------|
| ① 換成有 curl 的 base image | 最簡單 | image 變大、攻擊面變大，違背第 09 章選 distroless 的初衷 |
| ② 自己編一個迷你 probe 程式放進去 | 不加 shell、不加套件，仍是靜態執行檔 | 多一個 stage、多 5MB 左右 |
| ③ 讓 shorty 本身支援 `shorty healthcheck` 子指令 | 不用多一個執行檔 | 要改 app 程式碼 |
| ④ 不在 image 裡做，交給 orchestrator | image 最乾淨 | 單機 `docker run` 看不到 health |

- 本章示範 **②**：`tutorials/16-healthcheck-shutdown/probe/` 是一個只用標準庫、約 40 行的 Go 程式，打一個 HTTP GET，2xx 就 `exit 0`。
- **③** 的想法是在 `main.go` 開頭判斷 `os.Args[1] == "healthcheck"`，就改成「打自己的 `/healthz` 然後結束」，`HEALTHCHECK CMD ["/shorty", "healthcheck"]`。很多 Go 服務這樣做，但本系列約定 `app/` 不改，所以只介紹不實作。
- 最終版 `app/Dockerfile` 選的是 **④**：上線時跑在 K8s 上，由 kubelet 從外面打 HTTP probe；本機用 Compose 時，也沒有其他服務需要等 shorty 變 healthy。根目錄 `compose.yaml` 那行註解「healthcheck 交給外部」就是這個意思。

### 具名 build context

probe 的原始碼放在章節目錄，不在 build context（`app/`）裡——`COPY` 拿不到 context 外面的檔案（第 10 章）。BuildKit 的解法是**額外**送一個具名的 context：

```bash
docker build --build-context probe=tutorials/16-healthcheck-shutdown/probe ...
```

Dockerfile 裡用 `COPY --from=probe . .` 取用，就像從另一個 stage 複製一樣。`--build-context` 的值也可以是 `docker-image://...` 或 Git URL，這裡用的是本機資料夾。

### PID 1 與訊號

container 裡第一個啟動的 process 是 **PID 1**，它有兩個特殊之處：

1. `docker stop` 的 SIGTERM **只送給 PID 1**。
2. Linux 對 PID 1 很「保護」：**沒有自己註冊 handler 的訊號會被忽略**。一般 process 收到 SIGTERM 的預設動作是結束，但 PID 1 不會。

shorty 用 `signal.NotifyContext` 註冊了 SIGINT／SIGTERM，所以當 PID 1 沒問題。問題出在**誰是 PID 1**。

### exec form vs shell form

```dockerfile
ENTRYPOINT ["/shorty"]           # exec form：PID 1 就是 /shorty
CMD /shorty                      # shell form：實際是 /bin/sh -c '/shorty'
```

shell form 會先起一個 `sh`，sh 變成 PID 1，shorty 是它的子行程。sh 不會把 SIGTERM 轉給孩子，自己又因為是 PID 1 而忽略訊號——結果就是**什麼都沒發生，直到 10 秒後 SIGKILL**。本章的 `Dockerfile.shell-form` 就是這個反例。

> 有些 shell 遇到 `sh -c '單一指令'` 會偷偷優化成 exec，所以反例刻意寫成 `/shorty; echo ...`——真實世界的 entrypoint script 常常就長這樣。需要 shell 的時候，最後一行用 `exec /shorty "$@"`，讓 shorty 取代 shell 成為 PID 1。

### docker stop 的流程

```
docker stop shorty
  │
  ├─ 送 STOPSIGNAL（預設 SIGTERM）給 PID 1
  ├─ 等待（預設 10 秒）
  └─ 還沒結束 → 送 SIGKILL（無法攔截）→ exit code 137
```

| 想調整的東西 | 指令／設定 |
|------|------|
| 這次多等一點 | `docker stop -t 30 shorty` |
| 這個 container 預設多等一點 | `docker run --stop-timeout 30 ...` |
| 換成別的訊號 | Dockerfile `STOPSIGNAL SIGINT` 或 `docker run --stop-signal SIGINT` |
| Compose | `stop_grace_period: 30s`、`stop_signal: SIGINT`（第 17 章起） |

shorty 在 `main.go` 裡的 shutdown timeout 是 **8 秒**，刻意比 Docker 的 10 秒短：自己先收完尾，不要被 SIGKILL 打斷。**app 的 timeout < 平台的 grace period** 是通則，K8s 的 `terminationGracePeriodSeconds`（預設 30 秒）也一樣。

### `--init` 與 tini

`docker run --init` 會在你的程式前面插一個極小的 init（Docker 內建的 `docker-init`，就是 tini），讓它當 PID 1：

- 幫忙**收殭屍行程**（zombie reaping）：如果你的程式會 fork 子行程（例如呼叫外部指令），子行程結束後要有人 `wait()` 它。
- 把訊號**轉給它的直接子行程**，讓「沒寫 signal handler 的程式」也能被 SIGTERM 正常結束。

shorty 不 fork、有自己的 handler，所以不需要 `--init`。也要注意：`--init` 只轉給直接子行程，**救不了 shell form**——它會轉給 sh，sh 結束，shorty 還是在最後被 kill 掉。治本仍然是 exec form。Compose 對應的是 `init: true`。

## shorty 在這章的演進

| 項目 | 第 15 章 | 第 16 章 |
|------|------|------|
| Dockerfile | `app/Dockerfile` 的前身 | `tutorials/16-healthcheck-shutdown/Dockerfile`：多一個 `probe-build` stage、`HEALTHCHECK CMD ["/probe"]` |
| 健康狀態 | `docker ps` 只看到 `Up` | `Up (healthy)` |
| 關機 | 依賴 exec form | 用反例 `Dockerfile.shell-form` 證明 exec form 的重要 |
| 最終版 | — | `app/Dockerfile` 不帶 HEALTHCHECK（選 ④），保留 exec form `ENTRYPOINT ["/shorty"]` |

本章檔案：

```
tutorials/16-healthcheck-shutdown/
├── Dockerfile              # distroless + probe + HEALTHCHECK
├── Dockerfile.shell-form   # 反例：shell form，SIGTERM 送不到
└── probe/
    ├── go.mod
    └── main.go             # 迷你 HTTP probe
```

## 動手做

以下指令都從 repo 根目錄執行。

### 1. build 帶 HEALTHCHECK 的 image

```bash
docker build \
  --build-context probe=tutorials/16-healthcheck-shutdown/probe \
  -f tutorials/16-healthcheck-shutdown/Dockerfile \
  -t shorty:ch16 app
```

### 2. 看 health 狀態變化

```bash
docker run -d --name shorty -p 8080:8080 shorty:ch16
docker ps --filter name=shorty
```

範例輸出（剛啟動）：

```
CONTAINER ID   IMAGE         COMMAND     CREATED        STATUS                            PORTS                    NAMES
3f2a9c1d7e44   shorty:ch16   "/shorty"   2 seconds ago  Up 1 second (health: starting)    0.0.0.0:8080->8080/tcp   shorty
```

幾秒後再看一次，STATUS 會變成 `Up 15 seconds (healthy)`。看詳細紀錄：

```bash
docker inspect --format '{{json .State.Health}}' shorty
```

範例輸出（排版過）：

```json
{
  "Status": "healthy",
  "FailingStreak": 0,
  "Log": [
    {
      "Start": "2026-10-28T08:40:11.52Z",
      "End": "2026-10-28T08:40:11.53Z",
      "ExitCode": 0,
      "Output": "probe: http://127.0.0.1:8080/healthz 200 OK\n"
    }
  ]
}
```

probe 印到 stdout／stderr 的內容會出現在 `Output`，這就是為什麼 probe 失敗時要把錯誤印出來。

### 3. 用 events 盯 health 變化

health 狀態一改變，Docker 就發一個 `health_status` 事件：

```bash
docker events --filter container=shorty --filter event=health_status
```

範例輸出：

```
2026-10-28T08:40:21.53+08:00 container health_status: healthy 3f2a9c1d7e44 (image=shorty:ch16, name=shorty)
```

### 4. 看 graceful shutdown 的 log

另開一個終端機跟著 log：

```bash
docker logs -f shorty
```

然後停止它，順便計時：

```bash
time docker stop shorty
docker inspect --format '{{.State.ExitCode}}' shorty
```

範例輸出（log，JSON 每行一筆）：

```
{"time":"2026-10-28T00:40:01.10Z","level":"WARN","msg":"DATABASE_URL not set, using in-memory store"}
{"time":"2026-10-28T00:40:01.10Z","level":"WARN","msg":"REDIS_URL not set, cache disabled"}
{"time":"2026-10-28T00:40:01.10Z","level":"INFO","msg":"listening","addr":":8080","version":"dev"}
{"time":"2026-10-28T00:40:11.53Z","level":"INFO","msg":"request","method":"GET","path":"/healthz","dur_ms":0}
{"time":"2026-10-28T00:41:30.02Z","level":"INFO","msg":"shutdown signal received, draining"}
{"time":"2026-10-28T00:41:30.02Z","level":"INFO","msg":"bye"}
```

範例輸出（計時與 exit code）：

```
real    0m0.4s
0
```

對照 `main.go`：

1. `signal.NotifyContext` 收到 SIGTERM → `ctx.Done()` → 印 `shutdown signal received, draining`
2. `srv.Shutdown(shutdownCtx)`：停止接新連線，等進行中的請求做完（最多 8 秒）
3. 印 `bye`，`run()` 回傳 nil，`defer` 依序關掉 Redis、Postgres 連線
4. exit 0。**不到一秒**就停好了，完全沒用到 10 秒寬限

注意每 10 秒會有一筆 `/healthz` 的 request log——healthcheck 也是請求。

### 5. 反例：shell form

```bash
docker rm shorty
docker build -f tutorials/16-healthcheck-shutdown/Dockerfile.shell-form -t shorty:ch16-shell app
docker run -d --name shorty-shell -p 8080:8080 shorty:ch16-shell
docker top shorty-shell
```

範例輸出（PID 是 host 上看到的編號）：

```
UID    PID     PPID    C   STIME   TTY   TIME       CMD
root   41201   41180   0   08:45   ?     00:00:00   /bin/sh -c /shorty; echo "shorty exited with code $?"
root   41230   41201   0   08:45   ?     00:00:00   /shorty
```

sh 在上、shorty 在下。現在停它：

```bash
time docker stop shorty-shell
docker inspect --format '{{.State.ExitCode}}' shorty-shell
docker logs shorty-shell | tail -n 2
```

範例輸出：

```
real    0m10.3s
137
{"time":"2026-10-28T00:45:02.40Z","level":"INFO","msg":"listening","addr":":8080","version":"dev"}
```

等了整整 10 秒，exit code 137（128 + 9 = SIGKILL），而且 log 裡**沒有** `shutdown signal received`，也沒有 `bye`、沒有 `shorty exited`。

### 6. 調整等待時間

```bash
docker start shorty-shell
time docker stop -t 3 shorty-shell     # 只等 3 秒就 SIGKILL
docker rm shorty-shell
```

`-t` 不會讓 shell form 變正確，只是讓你少等一點。在 Compose 裡對應的設定是：

```yaml
services:
  app:
    stop_grace_period: 15s   # docker compose stop / down 時等多久才 SIGKILL
```

## 對照 Node／TypeScript

| 主題 | Node | Go（shorty） |
|------|------|------|
| 收 SIGTERM | `process.on('SIGTERM', () => server.close(...))` | `signal.NotifyContext(ctx, syscall.SIGTERM)` |
| 優雅關閉 HTTP | `server.close()` 停止接新連線，等現有連線結束 | `srv.Shutdown(ctx)` |
| PID 1 問題 | **最常踩**：`CMD ["npm", "start"]` 讓 npm 當 PID 1，npm 轉送訊號的行為不可靠；官方建議直接 `CMD ["node", "server.js"]` | exec form 直接跑執行檔 |
| 沒寫 handler 時 | Node 當 PID 1 且沒註冊 SIGTERM，也會被忽略 → 卡 10 秒 | 同樣的規則 |
| init | 官方 node image 文件建議 `docker run --init` 或裝 tini | 不需要（不 fork、有 handler） |
| healthcheck | node image 有 node 可以 `CMD ["node", "healthcheck.js"]`，或 alpine 版用 `wget` | distroless 沒工具，自己編 probe |

Node 專案最常見的錯誤組合是 `CMD npm run start`（shell form + npm），等於兩層中間人；改成 exec form 直接跑 `node`，問題就消失大半。

## 常見陷阱

1. **把 `/readyz` 當 liveness**：DB 一抖，所有副本一起被判死、一起重啟，讓小事故變大事故。
2. **以為 unhealthy 會自動重啟**：單機 Docker 不會。要自動重啟得靠 Swarm、K8s，或第三方工具（例如 autoheal）。
3. **在 distroless 寫 `HEALTHCHECK CMD curl ...`**：build 會成功，執行時每次檢查都失敗（找不到 curl），container 永遠 unhealthy。
4. **probe 的 timeout 比 `--timeout` 長**：Docker 先砍掉 probe，`Output` 欄看不到真正的錯誤。probe 自己設 2 秒、HEALTHCHECK 給 3 秒。
5. **檢查太頻繁**：`--interval=1s` 會讓 log 被 `/healthz` 洗版，也白白吃 CPU。啟動期需要快，就用 `--start-interval`。
6. **shell form 的 ENTRYPOINT**：除了收不到訊號，`docker run image arg` 的參數也傳不進去（shell form 會忽略 CMD 與執行時參數）。
7. **app 的 shutdown timeout ≥ grace period**：app 還在收尾就被 SIGKILL。shorty 8 秒 < Docker 10 秒，K8s 預設 30 秒更寬鬆。
8. **忘了 `--build-context`**：`COPY --from=probe` 會被當成 image `docker.io/library/probe`，錯誤訊息是 pull access denied 之類，很容易看不懂。

## 練習

1. 把 probe 改成打 `/readyz`（提示：`HEALTHCHECK CMD ["/probe", "http://127.0.0.1:8080/readyz"]`），用 in-memory 模式跑起來，它會 healthy 嗎？為什麼？（看 `store.Memory.Ping` 和 `cache.Noop.Ping`）
2. 用 `docker run --health-interval=5s --health-retries=1 ...` 覆寫 image 裡的時間參數，觀察 `docker inspect` 的 `.Config.Healthcheck` 變化。再想想：為什麼 `--health-cmd "curl ..."` 在這個 image 上行不通？（提示：`--health-cmd` 的字串會交給 `/bin/sh -c` 執行）
3. 在 shell form 反例上試 `docker run --init`，`docker stop` 之後 log 裡會出現 `bye` 嗎？說明原因。
4. 把 `Dockerfile.shell-form` 的 CMD 改成 `CMD exec /shorty`，重新 build，驗證 `docker stop` 變快、exit code 變 0。
5. 不實作，只寫出虛擬碼：如果要做選項 ③（`shorty healthcheck` 子指令），`main.go` 的 `main()` 要怎麼改？要注意哪些事（提示：不要讀 `DATABASE_URL`、不要連 DB）？

## 下一章

到這裡，跑一套 shorty 要記一大串：`docker network create`、`docker volume create`、postgres 的 `docker run -e ...`、shorty 的 `docker run -p ... -e ... --restart ... --memory ...`。[第 17 章 compose-basics](../17-compose-basics/) 把這些全部寫進一個 `compose.yaml`，一行 `docker compose up` 搞定。
