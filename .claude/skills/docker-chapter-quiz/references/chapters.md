# Docker Tutorials 章節列表與核心考點

## Part 1 — 基礎概念

| 編號 | 目錄名稱 | 主題 | 核心考點 |
|------|---------|------|---------|
| 01 | why-containers | 為什麼要用 container | container vs VM、共用 kernel、CLI → dockerd → containerd → runc |
| 02 | install-first-container | 安裝與第一個 container | Docker Desktop／WSL2、`run hello-world`、`-d -p 8080:80 nginx`、Git Bash 路徑轉換 |
| 03 | image-container-registry | image、container、registry | 唯讀模板 vs 執行實例、registry／repository／tag／digest、`pull`／`images`／`rmi` |
| 04 | cli-lifecycle | 指令與生命週期 | `create`／`start`／`stop`／`kill`／`rm`、狀態機、`--rm`、`-it`、`ps -a`、`system prune` |
| 05 | exec-logs-inspect | exec、logs、inspect | `exec -it`、`logs -f`、`inspect --format`、`stats`、`cp`、log 寫 stdout |

## Part 2 — 寫 Dockerfile

| 編號 | 目錄名稱 | 主題 | 核心考點 |
|------|---------|------|---------|
| 06 | dockerfile-basics | Dockerfile 基本指令 | `FROM`／`WORKDIR`／`COPY`／`RUN`／`CMD`／`EXPOSE`、單階段 image 為什麼很大 |
| 07 | layers-cache | layer 與快取 | 先 COPY go.mod 再 COPY 原始碼、快取失效往下傳、`docker history` |
| 08 | multi-stage | multi-stage build | build vs runtime stage、`COPY --from`、Go／Rust（cargo-chef）／Node（`npm ci`）對照 |
| 09 | base-images | base image 選擇 | alpine（musl）／debian-slim／distroless／scratch、`CGO_ENABLED=0`、CA 憑證、時區 |
| 10 | build-context-args | build context 與 ARG | context 是什麼、`.dockerignore`、`ARG` vs `ENV`、`-ldflags -X main.version` |
| 11 | buildkit | BuildKit | `# syntax=`、`--mount=type=cache`、`--mount=type=secret`、`buildx`、`--progress=plain` |

## Part 3 — 執行期

| 編號 | 目錄名稱 | 主題 | 核心考點 |
|------|---------|------|---------|
| 12 | networking | 網路 | bridge、`-p` 是 NAT、自訂 network 的 DNS、預設 bridge 沒有名稱解析、container 裡的 `localhost` |
| 13 | volumes | 資料保存 | named volume vs bind mount vs tmpfs、Postgres 資料保存、volume 備份還原 |
| 14 | config-secrets | 設定與機密 | 12-factor、`-e`／`--env-file`、秘密不能 bake 進 image、`_FILE` 慣例 |
| 15 | resources-restart | 資源與重啟 | `--memory`／`--cpus`、exit 137 與 OOMKilled、`GOMEMLIMIT`、restart policy |
| 16 | healthcheck-shutdown | healthcheck 與關機 | `HEALTHCHECK`、`/healthz` vs `/readyz`、distroless 沒 curl、PID 1 與 SIGTERM、exec vs shell form、`--init` |

## Part 4 — Compose

| 編號 | 目錄名稱 | 主題 | 核心考點 |
|------|---------|------|---------|
| 17 | compose-basics | compose 基礎 | services／ports／environment、`up`／`down`／`ps`／`logs`、project name |
| 18 | compose-multi-service | 多服務 | `depends_on` + `condition: service_healthy`、named volume、`.env` 變數替換與位置 |
| 19 | compose-dev-workflow | 開發流程 | `compose.override.yaml` 自動合併、air 熱重載、`develop.watch`、profiles、多個 `-f` |
| 20 | compose-debugging | compose 除錯 | `compose config`、`run --rm`、`events`、port 被佔、DB 沒好、舊 volume 的密碼、忘了 `--build` |

## Part 5 — 上線

| 編號 | 目錄名稱 | 主題 | 核心考點 |
|------|---------|------|---------|
| 21 | image-security | image 安全 | non-root `USER`、`read_only`、`cap_drop: [ALL]`、`no-new-privileges`、Trivy／Scout、digest pin |
| 22 | tags-registry | tag 與 registry | semver + git sha、不要只靠 `latest`、digest、OCI label、GHCR 名稱要小寫 |
| 23 | multi-platform | 多平台 build | amd64 vs arm64、QEMU vs 交叉編譯、`$BUILDPLATFORM`／`$TARGETARCH`、manifest list、`--push` vs `--load` |
| 24 | github-actions | GitHub Actions | setup-buildx、metadata-action、build-push-action、GHA cache、PR 只 build 不 push、action pin commit |
| 25 | incident-drills | 事故演練 | OOM 重啟循環、磁碟被 image／log 塞滿、時區、container 內 DNS、port 衝突、殭屍行程 |
