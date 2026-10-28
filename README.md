# Docker Tutorials

從零開始學 Docker，一路走到把服務上線。共 **25 章**、分 **5 個 Part**，全系列用同一個 Go 短網址服務 **shorty** 從第一個 Dockerfile 一路演進到 CI 自動發佈多平台 image。

> 完整章節規劃與設計理由見 [ROADMAP.md](./ROADMAP.md)。
>
> 🧸 想先用故事直覺理解？每章都附 `STORY.md` 故事版（小學生都懂的比喻版）。
>
> 從 Node／TypeScript 來：每章 README 都有「對照 Node／TypeScript」段落。
>
> 🪟 用 Windows 學：先看 [WINDOWS.md](./WINDOWS.md)，設定好 WSL2 再開始。

## 快速開始

```bash
# 只跑 shorty（in-memory，不需要 DB）
docker build -t shorty app
docker run --rm -p 8080:8080 shorty

# 完整版：shorty + Postgres + Redis
docker compose up -d --build
curl -XPOST localhost:8080/api/links -d '{"url":"https://go.dev"}'
```

> Windows：上面的 `curl` 指令請在 Git Bash 或 WSL 執行。PowerShell 裡 `curl` 可能是 `Invoke-WebRequest` 的別名，JSON 的引號也會被吃掉，要用 `curl.exe` 並改寫引號（第 02 章有說明）。

沒有 Docker 也能先跑程式本身：`cd app && go run .`

## 目錄

### Part 1 — 基礎概念（01–05）
| # | 章節 | 主題 |
|---|------|------|
| 01 | [why-containers](./tutorials/01-why-containers) | 為什麼要用 container，跟 VM 差在哪 |
| 02 | [install-first-container](./tutorials/02-install-first-container) | 安裝與第一個 container |
| 03 | [image-container-registry](./tutorials/03-image-container-registry) | image、container、registry 的關係 |
| 04 | [cli-lifecycle](./tutorials/04-cli-lifecycle) | 常用指令與 container 生命週期 |
| 05 | [exec-logs-inspect](./tutorials/05-exec-logs-inspect) | 進到 container 裡：exec、logs、inspect |

### Part 2 — 寫 Dockerfile（06–11）
| # | 章節 | 主題 |
|---|------|------|
| 06 | [dockerfile-basics](./tutorials/06-dockerfile-basics) | Dockerfile 基本指令，shorty 第一版 |
| 07 | [layers-cache](./tutorials/07-layers-cache) | layer 與 build 快取 |
| 08 | [multi-stage](./tutorials/08-multi-stage) | multi-stage build（Go／Rust／Node 對照） |
| 09 | [base-images](./tutorials/09-base-images) | base image 怎麼選：alpine／distroless／scratch |
| 10 | [build-context-args](./tutorials/10-build-context-args) | build context、`.dockerignore`、ARG／ENV |
| 11 | [buildkit](./tutorials/11-buildkit) | BuildKit：cache mount、secret mount |

### Part 3 — 執行期（12–16）
| # | 章節 | 主題 |
|---|------|------|
| 12 | [networking](./tutorials/12-networking) | 網路：bridge、port、DNS |
| 13 | [volumes](./tutorials/13-volumes) | 資料保存：volume 與 bind mount |
| 14 | [config-secrets](./tutorials/14-config-secrets) | 設定與機密 |
| 15 | [resources-restart](./tutorials/15-resources-restart) | 資源限制與 restart policy |
| 16 | [healthcheck-shutdown](./tutorials/16-healthcheck-shutdown) | healthcheck 與 graceful shutdown |

### Part 4 — Compose（17–20）
| # | 章節 | 主題 |
|---|------|------|
| 17 | [compose-basics](./tutorials/17-compose-basics) | compose 基礎 |
| 18 | [compose-multi-service](./tutorials/18-compose-multi-service) | 多服務：App + Postgres + Redis |
| 19 | [compose-dev-workflow](./tutorials/19-compose-dev-workflow) | 開發流程：熱重載、override、profiles |
| 20 | [compose-debugging](./tutorials/20-compose-debugging) | compose 除錯技巧 |

### Part 5 — 上線（21–25）
| # | 章節 | 主題 |
|---|------|------|
| 21 | [image-security](./tutorials/21-image-security) | image 安全：non-root、掃描、精簡 |
| 22 | [tags-registry](./tutorials/22-tags-registry) | tag 策略與 registry |
| 23 | [multi-platform](./tutorials/23-multi-platform) | 多平台 build（amd64／arm64，Go／Rust／Node 對照） |
| 24 | [github-actions](./tutorials/24-github-actions) | GitHub Actions 自動 build、push |
| 25 | [incident-drills](./tutorials/25-incident-drills) | 事故演練：OOM、磁碟、時區、DNS |

## 實戰專案：shorty

| 路徑 | 內容 |
|------|------|
| [`app/`](./app) | Go 短網址服務原始碼（各章不改它，只改「怎麼包」） |
| [`app/Dockerfile`](./app/Dockerfile) | 最終版 Dockerfile（第 06–11、16、21–23 章演進的結果） |
| [`compose.yaml`](./compose.yaml) | 最終版 compose（第 17–20 章演進的結果） |
| `tutorials/NN-*/` | 各章的中間版本 Dockerfile／compose |

## 快速指令

```bash
./run list           # 列出章節
./run readme 08      # 看第 08 章 README
./run story 08       # 看第 08 章故事版
./run app            # 不用 Docker，直接 go run shorty
./run test           # 跑 shorty 的測試
./run up / down      # docker compose up -d --build / down
```
