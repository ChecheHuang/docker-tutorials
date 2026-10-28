# Docker Tutorials Roadmap

本文件說明 25 章的設計理由、學習目標，以及貫穿全系列的實戰專案 **shorty** 怎麼一章一章演進。對應索引見 [README.md](./README.md)。

## 設計原則

1. **入門到上線**：從「container 是什麼」講到「CI 自動 build 多平台 image 推上 registry」，終點是能獨立把一個後端服務容器化並上線，銜接 go-tutorials 第 38 章／rust-tutorials 第 48 章的 Kubernetes。
2. **一個專案從頭長到尾**：Part 1 先用現成 image（nginx、alpine、postgres）建立直覺；從第 06 章起，每章都在 `app/`（shorty 短網址服務）上加一件事，最後長成 `app/Dockerfile` + `compose.yaml` 的上線版本。
3. **Go 為主，重點章三語對照**：Go 編成單一靜態執行檔，最適合示範 multi-stage 與 scratch／distroless。第 08、09、23 章另附 Rust 與 Node 版本，比較 image 大小與建置策略。
4. **每章都能動手**：章節目錄裡的 Dockerfile／compose／腳本都是可以直接跑的；README 的「動手做」一律從 repo 根目錄下指令。
5. **故事版 + 對照 TypeScript**：每章附 `STORY.md`（小學生都懂的比喻版），README 附「對照 Node／TypeScript 開發者」段落，例如 `node_modules` 為什麼不要 COPY、`npm ci` 和 layer 快取的關係。

## 實戰專案：shorty

`app/` 是一個 Go 短網址服務（只用標準庫 `net/http` + pgx + go-redis）：

| 端點 | 用途 |
|------|------|
| `POST /api/links` `{"url": "..."}` | 建立短網址 |
| `GET /{code}` | 302 轉址，hits +1 |
| `GET /api/links/{code}` | 查統計 |
| `GET /healthz` | liveness |
| `GET /readyz` | readiness（檢查 DB／Redis） |
| `GET /version` | 回傳 build 時注入的版本 |

設定全部走環境變數：`PORT`、`BASE_URL`、`DATABASE_URL`（沒設 → in-memory）、`REDIS_URL`（沒設 → 不快取）、`LOG_LEVEL`。收到 SIGTERM 會 graceful shutdown，log 以 JSON 寫到 stdout。

**檔案慣例**：

- `app/` 的 Go 程式碼是固定的，各章不改它，只改「怎麼包」。
- 各章的中間版本放在自己的目錄：`tutorials/NN-*/Dockerfile`、`tutorials/NN-*/compose.yaml`。
- build context 一律是 `app/`：`docker build -f tutorials/08-multi-stage/Dockerfile -t shorty:ch08 app`。
- 章節 compose 檔放在章節目錄，用 `context: ../../app` 指回原始碼。
- 最終版是 `app/Dockerfile` 和根目錄的 `compose.yaml`。

## 章節詳述

### Part 1 — 基礎概念（01–05）

用現成 image 建立直覺，還不碰 shorty。

| # | 章節 | 重點 |
|---|------|------|
| 01 | why-containers | 「在我電腦可以跑」問題、container vs VM、kernel 共用、Docker 架構（CLI → dockerd → containerd → runc）只講輪廓 |
| 02 | install-first-container | Docker Desktop／Engine 安裝、`docker run hello-world`、`docker run -d -p 8080:80 nginx`、Windows／WSL2 與 macOS 的差異 |
| 03 | image-container-registry | image 是唯讀模板、container 是執行中的實例、registry／repository／tag／digest、`pull`／`images`／`rmi` |
| 04 | cli-lifecycle | `run`／`create`／`start`／`stop`／`kill`／`rm`、container 狀態機、`--rm`、`-it`、`-e`、`--name`、`ps -a`、`system prune` |
| 05 | exec-logs-inspect | `exec -it sh`、`logs -f`、`inspect` + `--format`、`stats`、`cp`、`top`；為什麼 container 的 log 要寫 stdout |

### Part 2 — 寫 Dockerfile（06–11）

從最笨的 Dockerfile 開始，一章改進一件事。

| # | 章節 | shorty 演進 |
|---|------|------|
| 06 | dockerfile-basics | 第一版：`FROM golang`、`COPY . .`、`RUN go build`、`CMD`，能跑但 image 近 1GB。`FROM`／`WORKDIR`／`COPY`／`RUN`／`CMD`／`EXPOSE` |
| 07 | layers-cache | 改成先 `COPY go.mod go.sum` + `go mod download` 再 `COPY . .`；`docker history`、快取失效規則、指令順序 |
| 08 | multi-stage | build stage + runtime stage，image 從 ~1GB 降到 ~20MB；**附 Rust（cargo-chef）與 Node（`npm ci` + prune）版本對照** |
| 09 | base-images | alpine（musl）／debian-slim／distroless／scratch 比較、`CGO_ENABLED=0`、CA 憑證與時區；**附 Rust 與 Node 各自適合的 base image** |
| 10 | build-context-args | build context 是什麼、`.dockerignore`、`ARG` vs `ENV`、`--build-arg VERSION` + `-ldflags -X main.version` |
| 11 | buildkit | `# syntax=` 、`RUN --mount=type=cache`、`--mount=type=secret`（私有依賴）、`docker buildx`、`--progress=plain` |

### Part 3 — 執行期（12–16）

shorty 的 image 好了，接下來是「怎麼跑才對」。

| # | 章節 | 重點 |
|---|------|------|
| 12 | networking | bridge network、`-p` 是 NAT、自訂 network 的內建 DNS（用 container 名字互連）、`host` 模式、`localhost` 在 container 裡指誰；手動 `docker network create` + 跑 postgres + shorty |
| 13 | volumes | container 檔案系統是暫時的、named volume vs bind mount vs tmpfs、Postgres 資料保存、備份 volume、Windows bind mount 效能 |
| 14 | config-secrets | 12-factor、`-e`／`--env-file`、不要把秘密 bake 進 image（`docker history` 看得到）、Docker secrets 檔案掛載、shorty 的環境變數表 |
| 15 | resources-restart | `--memory`／`--cpus`、OOM killed（exit 137）、Go 的 `GOMEMLIMIT`、restart policy（`no`／`on-failure`／`unless-stopped`／`always`） |
| 16 | healthcheck-shutdown | `HEALTHCHECK`、liveness vs readiness（`/healthz` vs `/readyz`）、distroless 沒有 curl 怎麼辦、PID 1 與訊號、exec form vs shell form、`docker stop` 的 10 秒、`--init`／tini、shorty 的 graceful shutdown |

### Part 4 — Compose（17–20）

| # | 章節 | shorty 演進 |
|---|------|------|
| 17 | compose-basics | 把第 12–16 章的一長串 `docker run` 變成 `compose.yaml`：services、ports、environment、`up`／`down`／`ps`／`logs` |
| 18 | compose-multi-service | 加上 Postgres + Redis、`depends_on` + `condition: service_healthy`、named volume、`.env` 變數替換；結果就是根目錄 `compose.yaml` |
| 19 | compose-dev-workflow | `compose.override.yaml`、bind mount + 熱重載（air）、`develop.watch`、profiles（`--profile tools` 起 pgAdmin）、多環境檔 `-f` |
| 20 | compose-debugging | `config` 看合併結果、`logs`、`exec`、`run --rm`、`events`、常見錯誤（port 被佔、DB 還沒好、volume 權限、改了 Dockerfile 沒 `--build`） |

### Part 5 — 上線（21–25）

| # | 章節 | 重點 |
|---|------|------|
| 21 | image-security | non-root `USER`、`read_only`、`no-new-privileges`、cap drop、用 Trivy／Docker Scout 掃描、pin base image digest；結果就是 `app/Dockerfile` |
| 22 | tags-registry | tag 策略（semver、git sha、不要只靠 `latest`）、digest、OCI label、Docker Hub／GHCR、`login`／`push`／`pull`、`/version` 驗證 |
| 23 | multi-platform | amd64 vs arm64（Apple Silicon、Graviton）、`buildx --platform`、QEMU vs 交叉編譯、`$BUILDPLATFORM`／`$TARGETARCH`、manifest list；**附 Rust 與 Node 的多平台做法** |
| 24 | github-actions | `docker/setup-buildx-action`、`metadata-action` 產 tag、`build-push-action` + GHA cache、推到 GHCR、PR 只 build 不 push；成果是 `.github/workflows/docker.yml` |
| 25 | incident-drills | 事故演練：OOM 循環重啟、磁碟被 image／log 塞滿（`system df`、log rotation）、時區錯誤、container 內 DNS 失敗、port 衝突、殭屍行程；每個都附重現步驟與排查流程 |

## 與其他系列的關係

- **go-tutorials 第 23 章 Docker**：那章是「部落格 API 的 Dockerfile 逐行解析」，本系列是完整版。
- **rust-tutorials 第 46 章 Docker**：Rust 專屬的 cargo-chef、musl 靜態連結，本系列第 08、09 章會引用。
- **K8s**：本系列結束後接 go-tutorials 第 38 章或 rust-tutorials 第 48 章。
