# 24. GitHub Actions：自動測試、掃描、build、push

> 範圍：`docker/setup-qemu-action`、`docker/setup-buildx-action`、`docker/login-action`（GHCR + `GITHUB_TOKEN`）、`docker/metadata-action` 產 tag、`docker/build-push-action` + GHA cache、多平台、Trivy 掃描、PR 只 build 不 push；成果是 `.github/workflows/docker.yml`

> 🧸 想先用故事直覺理解這章？看 [STORY.md](./STORY.md)（小學生都懂的比喻版）。

## 為什麼需要這個

第 21–23 章每一步都是手動的：

```bash
go test ./...
docker buildx build --platform linux/amd64,linux/arm64 --build-arg VERSION=1.2.0 \
  -t ghcr.io/chechehuang/shorty:1.2.0 -t ghcr.io/chechehuang/shorty:1.2 ... --push app
docker run --rm ... aquasec/trivy image ...
```

手動的問題：會忘記跑測試、tag 打錯、從有未 commit 修改的工作目錄 build、用自己電腦的 token 推、每個人 build 出來的不一樣。

CI 的目標：**只要 push 程式碼或打 git tag，同一套步驟每次都一模一樣地跑完**，而且 image 一定來自 repo 裡某一個 commit。

## 核心概念

### 1. 觸發事件決定行為

| 事件 | test | build | 掃描 | push | 產生的 tag |
|------|------|------|------|------|------|
| `pull_request` → main | ✅ | ✅ 兩個平台 | ✅ | ❌ | （`pr-12`，不推） |
| `push` 到 main／master | ✅ | ✅ | ✅ | ✅ | `main`、`sha-a1b2c3d` |
| `push` tag `v1.2.3` | ✅ | ✅ | ✅ | ✅ | `1.2.3`、`1.2`、`1`、`latest`、`sha-a1b2c3d` |

PR 一定要 build：確認 Dockerfile 沒壞、兩個平台都編得過。但**不能 push**：fork 來的 PR 拿不到寫入權限，而且未審查的程式碼不該變成 image。

### 2. 五個 Docker 官方 action

| action | 版本 | 做什麼 |
|------|------|------|
| `docker/setup-qemu-action` | v4 | 註冊 QEMU（第 23 章） |
| `docker/setup-buildx-action` | v4 | 建立 `docker-container` builder，支援多平台與 GHA cache |
| `docker/login-action` | v4 | `docker login`，這裡登入 `ghcr.io` |
| `docker/metadata-action` | v6 | 從 git ref／sha 算出 tag 與 OCI label（第 22 章的規則） |
| `docker/build-push-action` | v7 | 等於 `docker buildx build`，可以 `push`、`load`、cache |

這幾個 action 在 2026 年初都升了 major 版（改用 Node 24 runtime），GitHub 託管的 runner 不受影響；自架 runner 需要 Actions Runner v2.327.1 以上。

### 3. `GITHUB_TOKEN` 與 `permissions`

每個 workflow run，GitHub 會自動產生一個短效 token `secrets.GITHUB_TOKEN`，run 結束就失效。它能做什麼由 `permissions` 決定：

```yaml
permissions:
  contents: read     # workflow 層級：預設最小權限
jobs:
  docker:
    permissions:
      contents: read
      packages: write   # 只有這個 job 能寫 GHCR
```

不用自己建 PAT、不用存密碼，這是 GHCR 搭配 Actions 最大的好處。

### 4. GHA cache

`type=gha` 把 BuildKit 的 layer 快取存進 GitHub Actions 的 cache 服務：

```yaml
cache-from: type=gha
cache-to: type=gha,mode=max
```

- `mode=min`（預設）只存最終 image 的 layer；`mode=max` 連 build stage 的中間 layer（`go mod download` 那層）都存，multi-stage build 一定要 `max` 才有效。
- 每個 repo 的 Actions cache 有容量上限（預設 10GB），滿了會淘汰最舊的。
- 注意：`RUN --mount=type=cache` 的內容（Go module cache）**不會**被 `type=gha` 存下來，只有 layer 會。

## shorty 在這章

成果在 repo 根目錄：**`.github/workflows/docker.yml`**。兩個 job：

```
test ──needs──▶ docker
 go vet          compute image name
 go test         setup-qemu → setup-buildx → login（非 PR）
                 metadata → build amd64 + load → trivy
                 → build amd64+arm64（非 PR 才 push）→ summary
```

## 動手做：逐步讀 workflow

### 觸發與共用設定

```yaml
on:
  push:
    branches: [main, master]
    tags: ["v*.*.*"]
  pull_request:
    branches: [main, master]
  workflow_dispatch:
```

- tag 用 `v*.*.*` 過濾，只有像 `v1.2.3` 的 tag 才觸發，`vtest` 這種不會。
- `workflow_dispatch` 讓你能在 Actions 頁面手動按按鈕跑。

```yaml
concurrency:
  group: ${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: true
```

同一個 PR 連續 push 三次，只跑最後一次，省 runner 時間。

### job 1：test

```yaml
- uses: actions/checkout@v7
- uses: actions/setup-go@v7
  with:
    go-version-file: app/go.mod
    cache-dependency-path: app/go.sum
- run: go vet ./...
- run: go test ./...
```

- `defaults.run.working-directory: app`：`run:` 都在 `app/` 下執行，因為 `go.mod` 在那裡。但 `uses:` 的路徑參數不受影響，所以 `go-version-file` 要寫 `app/go.mod`。
- `go-version-file` 讓 CI 的 Go 版本永遠跟 `go.mod` 一致，不用在兩個地方改版本號。
- 這個 job 不碰 Docker：測試在原生環境跑最快。`docker` job 用 `needs: test` 等它通過。

### job 2：docker

**① image 名稱轉小寫**

```yaml
- name: Compute image name
  run: echo "IMAGE=ghcr.io/${GITHUB_REPOSITORY_OWNER,,}/shorty" >> "$GITHUB_ENV"
```

`${VAR,,}` 是 bash 的「轉小寫」語法。寫進 `$GITHUB_ENV` 之後，後面的 step 都能用 `${{ env.IMAGE }}`。第 22 章提過：GHCR 名稱有大寫會被拒絕。

**② QEMU 與 buildx**

```yaml
- uses: docker/setup-qemu-action@v4
- uses: docker/setup-buildx-action@v4
```

`app/Dockerfile` 是交叉編譯，runtime stage 只有 `COPY`，其實用不到 QEMU；留著是保險，哪天 Dockerfile 在目標平台執行 `RUN` 也不會壞。buildx 那步則是必要的：預設的 `docker` driver 不支援 `type=gha` cache。

**③ 登入（只有非 PR）**

```yaml
- name: Login to GHCR
  if: github.event_name != 'pull_request'
  uses: docker/login-action@v4
  with:
    registry: ghcr.io
    username: ${{ github.actor }}
    password: ${{ secrets.GITHUB_TOKEN }}
```

**④ metadata：tag 與 label**

```yaml
- id: meta
  uses: docker/metadata-action@v6
  with:
    images: ${{ env.IMAGE }}
    tags: |
      type=semver,pattern={{version}}
      type=semver,pattern={{major}}.{{minor}}
      type=semver,pattern={{major}},enable=${{ !startsWith(github.ref, 'refs/tags/v0.') }}
      type=ref,event=branch
      type=ref,event=pr
      type=sha
```

| 規則 | `push v1.2.3` | `push main` | PR #12 |
|------|------|------|------|
| `type=semver,pattern={{version}}` | `1.2.3` | — | — |
| `type=semver,pattern={{major}}.{{minor}}` | `1.2` | — | — |
| `type=semver,pattern={{major}}` | `1` | — | — |
| `type=ref,event=branch` | — | `main` | — |
| `type=ref,event=pr` | — | — | `pr-12` |
| `type=sha` | `sha-a1b2c3d` | `sha-a1b2c3d` | `sha-a1b2c3d` |
| （flavor `latest=auto`） | `latest` | — | — |

- semver 規則會自動去掉 git tag 的 `v`；`1.3.0-rc.1` 這種預發佈版不會產生 `1.3`、`1`、`latest`。
- `0.x` 版不打 `{{major}}`：`0` 這個 tag 沒什麼意義（0.x 的每個 minor 都可能不相容）。
- 它也會輸出 `labels`：`org.opencontainers.image.source`／`version`／`revision`／`created`…，第 22 章手動傳的 build arg 全部自動化。
- `steps.meta.outputs.version` 是「最主要的那個版本字串」（tag 時是 `1.2.3`、main 是 `main`、PR 是 `pr-12`），拿來當 `VERSION` build arg，`/version` 就會回傳它。

**⑤ build 一份給 Trivy 掃**

```yaml
- name: Build for scan (linux/amd64)
  uses: docker/build-push-action@v7
  with:
    context: ./app
    platforms: linux/amd64
    load: true
    tags: shorty:scan
    build-args: VERSION=${{ steps.meta.outputs.version }}
    cache-from: type=gha
```

多平台 image 沒辦法 `load` 進 runner 的 Docker（第 23 章），所以先單獨 build amd64 版載入本機給 Trivy 讀。`context: ./app` 對應「build context 一律是 `app/`」的慣例，`Dockerfile` 預設就找 `app/Dockerfile`。

**⑥ Trivy 掃描**

```yaml
- uses: aquasecurity/trivy-action@ed142fd0673e97e23eac54620cfb913e5ce36c25 # v0.36.0
  with:
    image-ref: shorty:scan
    format: table
    exit-code: "1"
    ignore-unfixed: true
    vuln-type: os,library
    severity: CRITICAL,HIGH
    version: v0.76.1
```

- `exit-code: "1"` + `severity`：有 HIGH／CRITICAL 就讓 step 失敗，後面的 push 就不會執行——**有已知重大漏洞的 image 推不上去**。
- `ignore-unfixed`：還沒有修正版的漏洞先不擋（擋了你也沒辦法修）。
- pin 到 commit SHA、`version` 指定 Trivy 版本：第 21 章提過，掃描工具本身也是供應鏈的一環。

**⑦ 多平台 build + push**

```yaml
- name: Build and push (linux/amd64, linux/arm64)
  id: build
  uses: docker/build-push-action@v7
  with:
    context: ./app
    platforms: linux/amd64,linux/arm64
    push: ${{ github.event_name != 'pull_request' }}
    tags: ${{ steps.meta.outputs.tags }}
    labels: ${{ steps.meta.outputs.labels }}
    annotations: ${{ steps.meta.outputs.annotations }}
    build-args: VERSION=${{ steps.meta.outputs.version }}
    cache-from: type=gha
    cache-to: type=gha,mode=max
```

- `push` 是一個運算式：PR 時是 `false`，只 build 驗證。
- amd64 剛剛在 ⑤ build 過，builder 裡有快取，這步主要花在 arm64（交叉編譯，也很快）。
- `annotations` 把同一組 metadata 也寫成 OCI annotation（預設寫在各平台的 manifest 上；想連 index 一起寫，可在 metadata step 設 env `DOCKER_METADATA_ANNOTATIONS_LEVELS: manifest,index`）。label 存在 image config 裡，annotation 存在 manifest 上，有些 registry 頁面只讀後者。
- `steps.build.outputs.digest` 是推上去的 index digest，最後的 Summary step 會把它印在 run 的摘要頁。

### 實際跑一次

```bash
git add .github/workflows/docker.yml
git commit -m "ci: build and push shorty"
git push origin main            # → :main、:sha-xxxxxxx

git tag v1.2.0
git push origin v1.2.0          # → :1.2.0、:1.2、:1、:latest
```

到 GitHub repo 的 **Actions** 分頁看 run；成功後在 **Packages** 看到 `shorty`。第一次推上去的 package 預設是 private，要公開可到 package settings 改 visibility。因為 image 有 `org.opencontainers.image.source` label，package 會自動連到這個 repo。

驗證：

```bash
docker buildx imagetools inspect ghcr.io/chechehuang/shorty:1.2.0
docker run --rm -d --name shorty-ci -p 8080:8080 ghcr.io/chechehuang/shorty:1.2.0
curl localhost:8080/version
docker stop shorty-ci
```

範例輸出：

```json
{"version":"1.2.0"}
```

## 對照 Node／TypeScript

| Node 專案的 CI | shorty 的 CI |
|------|------|
| `actions/setup-node` + `node-version-file: .nvmrc` | `actions/setup-go` + `go-version-file: app/go.mod` |
| `cache: npm` | `setup-go` 內建 module cache |
| `npm ci && npm test` | `go vet ./... && go test ./...` |
| `npm audit --audit-level=high` | Trivy `severity: CRITICAL,HIGH` |
| `npm publish`（tag 觸發） | `build-push-action` `push: true`（tag 觸發） |
| `NODE_AUTH_TOKEN: ${{ secrets.NPM_TOKEN }}` | `GITHUB_TOKEN` + `packages: write` |

Node 專案的 Docker 部分幾乎可以照抄這份 workflow，只要把 `test` job 換成 `setup-node` + `npm ci` + `npm test`，`context` 指向 Node 專案目錄。如果 image 裡有 native module，記得第 23 章說的：多平台 build 會走 QEMU，CI 時間會變長，可以考慮用 GitHub 的 arm64 runner 分開 build。

## 常見陷阱

1. **`denied: installation not allowed to Write organization package`** 或 403：忘了 `permissions: packages: write`；或 package 已經存在、但沒把這個 repo 加進 package 的 *Manage Actions access*。
2. **PR 的 run 失敗在 login**：fork PR 的 `GITHUB_TOKEN` 是唯讀的，所以 login step 要 `if: github.event_name != 'pull_request'`。
3. **cache 沒效果**：用了 `cache-to: type=gha` 卻沒加 `mode=max`，multi-stage 的 build stage 不會被存。或忘了 `setup-buildx-action`。
4. **`/version` 回 `dev`**：`build-args` 沒傳 `VERSION`；或傳了但 Dockerfile 的 `ARG VERSION` 寫錯 stage。
5. **tag 推了卻沒觸發**：tag 名稱不符合 `v*.*.*`（例如 `1.2.0` 沒有 `v`、或 `v1.2`）。
6. **Trivy 突然擋住 build**：base image 新公布了 CVE。通常重新 build（拿到 base 的修補版）就好；真的暫時無解，用 `.trivyignore` 列出 CVE ID 並寫明原因與期限，不要直接把 `exit-code` 改成 0。
7. **action 用浮動 tag**：`@v7` 會跟著 v7.x 自動更新。Docker 官方 action 通常可以接受；第三方 action（像 trivy-action）建議 pin commit SHA，交給 Dependabot（`package-ecosystem: github-actions`）更新。
8. **在 workflow 裡 `echo` 秘密**：GitHub 會遮蔽已知 secret，但經過編碼或拼接的值遮不到。不要印 token。

## 練習

1. 開一個 PR，確認 run 有 build 兩個平台、有跑 Trivy，但 Packages 裡沒有出現 `pr-N` 的 tag。
2. 推 `v1.2.0`、再推 `v1.2.1`，用 `imagetools inspect` 確認 `1.2`、`1`、`latest` 都移到 `1.2.1`。
3. 推 `v1.3.0-rc.1`，確認 `latest` 沒有被移動。
4. 把 Trivy 的 `severity` 改成 `CRITICAL,HIGH,MEDIUM`，看看 shorty 會不會被擋下來。
5. 在 `metadata-action` 加一條 `type=raw,value=edge,enable={{is_default_branch}}`，讓 main 的每次 build 也有一個 `edge` tag。
6. 加一個 `.github/dependabot.yml`，讓 Dependabot 同時更新 `github-actions`、`docker`（`app/Dockerfile` 的 base image）和 `gomod`。

## 下一章

image 會自動推上去了，但上線之後總會出事。第 25 章 [incident-drills](../25-incident-drills) 是事故演練：OOM 循環重啟、磁碟被塞滿、時區錯誤、DNS 失敗、port 衝突、殭屍行程——每一個都附重現步驟與排查流程。
