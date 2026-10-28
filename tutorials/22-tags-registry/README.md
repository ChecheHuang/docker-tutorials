# 22. Tag 策略與 Registry

> 範圍：tag 策略（semver、git sha、不要只靠 `latest`）、digest pinning、OCI label（`org.opencontainers.image.*`）、Docker Hub vs GHCR、`docker login`／`tag`／`push`／`pull`、`docker buildx imagetools inspect`、用 `/version` 驗證

> 🧸 想先用故事直覺理解這章？看 [STORY.md](./STORY.md)（小學生都懂的比喻版）。

## 為什麼需要這個

到目前為止，shorty 的 image 都叫 `shorty:dev`、`shorty:ch21`，而且只存在你自己的電腦裡。要上線，得回答三個問題：

1. **放哪裡？** 正式機、同事、CI 都要拿得到 → 推到 registry。
2. **叫什麼名字？** 出事時要能說「現在線上跑的是 1.4.2，commit `a1b2c3d`」→ tag 策略。
3. **怎麼確定拿到的就是那一份？** tag 可以被覆蓋 → digest。

只靠 `latest` 的團隊，最常遇到的事故是：「兩台機器都說自己跑 `latest`，行為卻不一樣」——因為一台是上週 pull 的，一台是今天 pull 的。

## 核心概念

### 1. image 的完整名字

```
ghcr.io/chechehuang/shorty:1.2.0@sha256:9f86d0...
└──┬──┘ └────┬────┘ └─┬──┘ └─┬─┘ └──────┬──────┘
registry  namespace  repo   tag      digest
```

- 省略 registry 時預設是 Docker Hub（`docker.io`），省略 namespace 時是官方 image（`library/`）。所以 `nginx` 其實是 `docker.io/library/nginx:latest`。
- 同時寫 tag 和 digest 時，**以 digest 為準**，tag 只是給人看的。

### 2. tag 是「可移動的標籤」，digest 是「內容的指紋」

| | tag | digest |
|------|------|------|
| 長相 | `1.2.0`、`latest`、`sha-a1b2c3d` | `sha256:9f86d0...`（64 個 hex） |
| 會不會變 | 可以被重新 push 覆蓋 | 內容的雜湊，永遠不變 |
| 誰決定 | 你 | 由內容算出來 |
| 用途 | 人類溝通、部署設定 | 稽核、可重現、防止偷換 |

### 3. tag 策略：一次打好幾個

一個 image 可以同時有很多 tag，它們指向同一個 digest。shorty 每次發佈 `1.2.0` 時打：

| tag | 會不會被覆蓋 | 給誰用 |
|------|------|------|
| `1.2.0` | 不該被覆蓋（不可變） | 正式部署、回滾 |
| `1.2` | 每次 patch 版都會移動 | 「我要 1.2 系列的最新修補」 |
| `1` | 每次 minor／patch 都移動 | 寬鬆追版本 |
| `sha-a1b2c3d` | 不可變 | 追到 commit、每個 main build 都有 |
| `latest` | 每次正式版都移動 | 本機試玩、文件範例；**不要用在正式部署** |

原則：

- **部署設定寫不可變的 tag**（`1.2.0` 或 `sha-...`），最好再加 digest。
- `latest` 不是「最新」的意思，只是「沒寫 tag 時的預設名字」。誰最後 push 了 `latest`，它就指向誰。
- 預發佈版（`1.3.0-rc.1`）不要更新 `1.3`、`1`、`latest`。
- semver 的 tag 慣例不帶 `v`（git tag 是 `v1.2.0`，image tag 是 `1.2.0`），第 24 章的 metadata-action 會自動處理。

### 4. OCI label：讓 image 自我介紹

OCI image spec 定義了一組標準 label key，registry、掃描工具、Kubernetes 儀表板都看得懂：

| label | 例子 | 說明 |
|------|------|------|
| `org.opencontainers.image.source` | `https://github.com/ChecheHuang/docker-tutorials` | 原始碼位置；GHCR 用它把 package 連到 repo |
| `org.opencontainers.image.version` | `1.2.0` | 版本 |
| `org.opencontainers.image.revision` | `a1b2c3d4...`（完整 sha） | 哪個 commit |
| `org.opencontainers.image.created` | `2026-10-28T08:00:00Z` | RFC 3339 時間 |
| `org.opencontainers.image.title` | `shorty` | 名稱 |
| `org.opencontainers.image.description` | `shorty URL shortener` | 說明 |
| `org.opencontainers.image.licenses` | `MIT` | SPDX 授權 |

label 是 metadata，不會增加 image 大小，也不會被 tag 覆蓋影響。

### 5. 兩個版本號，要對得上

shorty 有兩個地方記版本：

1. **image 外面**：tag 和 `org.opencontainers.image.version` label。
2. **執行檔裡面**：build 時用 `-ldflags "-X main.version=${VERSION}"` 注入（第 10 章），`GET /version` 會回傳。

兩者都從同一個 `VERSION` build arg 來，所以部署後打一下 `/version` 就能確認「線上跑的真的是我推的那一版」。

### 6. Docker Hub vs GHCR

| | Docker Hub | GHCR（GitHub Container Registry） |
|------|------|------|
| 名稱 | `docker.io/<帳號>/shorty`（可簡寫 `<帳號>/shorty`） | `ghcr.io/<owner>/shorty` |
| 登入 | `docker login`（帳號 + Personal Access Token） | `docker login ghcr.io`（GitHub 帳號 + PAT，CI 裡用 `GITHUB_TOKEN`） |
| 權限 | Docker Hub 自己的 org／team | 跟 GitHub repo／org 權限整合 |
| 免費額度 | 公開 repo 免費；pull 有次數限制，未登入更低 | 公開 package 免費 |
| 適合 | 公開發行、給全世界 `docker pull` | 程式碼在 GitHub、配合 Actions（第 24 章） |

shorty 用 GHCR：程式碼在 GitHub，CI 用內建的 `GITHUB_TOKEN` 就能推，不用額外管密碼。

## shorty 在這章

| 檔案 | 用途 |
|------|------|
| `Dockerfile` | 第 21 章版本 + 完整 OCI label（`VERSION`／`REVISION`／`CREATED` 由 build arg 傳入） |
| `release.sh` | 手動發佈：檢查 semver、打 `1.2.0`／`1.2`／`1`／`sha-xxxxxxx`／`latest`、push、印出 digest |

`app/Dockerfile` 只寫了不會變的 `source` 與 `title` label；會隨每次 build 改變的 `version`／`revision`／`created`，在第 24 章由 `docker/metadata-action` 產生，透過 `build-push-action` 的 `labels` 加上去。

注意 `Dockerfile` 裡 runtime stage **重新宣告了 `ARG VERSION`**：`ARG` 只在宣告它的 stage 內有效，build stage 的 `VERSION` 到了 runtime stage 就看不到了。

## 動手做

### 1. build 一個有版本的 image

```bash
docker build -f tutorials/22-tags-registry/Dockerfile \
  --build-arg VERSION=1.2.0 \
  --build-arg REVISION=$(git rev-parse HEAD) \
  --build-arg CREATED=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
  -t shorty:1.2.0 app

docker image inspect --format '{{json .Config.Labels}}' shorty:1.2.0
```

範例輸出：

```json
{"org.opencontainers.image.created":"2026-10-28T08:00:00Z","org.opencontainers.image.description":"shorty URL shortener (docker-tutorials)","org.opencontainers.image.revision":"a1b2c3d4e5f60718293a4b5c6d7e8f9012345678","org.opencontainers.image.source":"https://github.com/ChecheHuang/docker-tutorials","org.opencontainers.image.title":"shorty","org.opencontainers.image.version":"1.2.0"}
```

### 2. 用 /version 驗證

```bash
docker run --rm -d --name shorty-122 -p 8080:8080 shorty:1.2.0
curl localhost:8080/version
docker stop shorty-122
```

範例輸出：

```json
{"version":"1.2.0"}
```

### 3. 登入 registry

GHCR 需要一個 GitHub Personal Access Token（classic，勾 `write:packages`；或 fine-grained token 給 packages 權限）：

```bash
echo "$GHCR_TOKEN" | docker login ghcr.io -u <你的 GitHub 帳號> --password-stdin
```

範例輸出：

```
Login Succeeded
```

Docker Hub：

```bash
docker login        # 照提示用瀏覽器或輸入帳號 + access token
```

> 用 `--password-stdin`，不要用 `-p`：`-p` 會留在 shell history 和 `ps` 裡。

### 4. tag 與 push

`docker tag` 不會複製任何資料，只是替同一個 image 多貼一張標籤：

```bash
IMAGE=ghcr.io/<你的帳號小寫>/shorty
docker tag shorty:1.2.0 $IMAGE:1.2.0
docker tag shorty:1.2.0 $IMAGE:sha-$(git rev-parse --short=7 HEAD)
docker push $IMAGE:1.2.0
docker push $IMAGE:sha-$(git rev-parse --short=7 HEAD)
```

範例輸出：

```
The push refers to repository [ghcr.io/chechehuang/shorty]
5f70bf18a086: Pushed
b2a1c4e3f5d6: Pushed
1.2.0: digest: sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08 size: 1234
```

或是一次做完：`./tutorials/22-tags-registry/release.sh 1.2.0`。

推完到 GitHub 個人頁的 **Packages** 分頁就看得到 shorty。新 package 預設是 private，要公開可在 package settings 改 visibility。

### 5. 看 registry 上的內容：imagetools inspect

`docker buildx imagetools inspect` 直接查 registry，不用先 pull：

```bash
docker buildx imagetools inspect $IMAGE:1.2.0
docker buildx imagetools inspect --format '{{json .Manifest}}' $IMAGE:1.2.0
```

範例輸出：

```
Name:      ghcr.io/chechehuang/shorty:1.2.0
MediaType: application/vnd.docker.distribution.manifest.v2+json
Digest:    sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08
```

第 23 章做完多平台 build 後，這裡會變成一份 manifest list，列出 `linux/amd64`、`linux/arm64` 各自的 digest。

### 6. 用 digest pull 和 run

```bash
docker rmi $IMAGE:1.2.0 shorty:1.2.0
docker pull $IMAGE@sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08
docker images --digests $IMAGE
```

範例輸出：

```
REPOSITORY                   TAG     DIGEST                                                                    IMAGE ID       SIZE
ghcr.io/chechehuang/shorty   <none>  sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08   3f1c0a9b2d7e   12.4MB
```

> 上面的 digest 都是示範值，換成你自己 push 出來的那一個。

在 compose 裡也一樣可以寫 `image: ghcr.io/chechehuang/shorty:1.2.0@sha256:...`。

## 對照 Node／TypeScript

| 概念 | npm 世界 | Docker 世界 |
|------|------|------|
| 套件倉庫 | npmjs.com、GitHub Packages | Docker Hub、GHCR、ECR、GAR |
| 版本號 | `package.json` 的 `version` | image tag |
| 浮動版本 | `^1.2.0`、`~1.2.0` | `1`、`1.2` 這種移動 tag |
| `latest` | `npm install foo` 預設裝 `latest` dist-tag | 沒寫 tag 預設 `latest` |
| 鎖內容 | `package-lock.json` 的 `integrity` | digest |
| 登入 | `npm login`、`.npmrc` token | `docker login`、`~/.docker/config.json` |
| 發佈 | `npm publish` | `docker push` |

最大的差別：**npm 不允許覆蓋已發佈的版本號，registry 預設允許覆蓋 tag**。所以「`1.2.0` 不可變」在 Docker 世界是團隊紀律（或 registry 的 immutable tag 設定，例如 ECR），不是天生保證。

## 常見陷阱

1. **GHCR 名稱有大寫**：`ghcr.io/ChecheHuang/shorty` 會被拒絕（`repository name must be lowercase`）。owner 一律轉小寫。
2. **忘了 `docker tag` 就 push**：`docker push shorty:1.2.0` 會嘗試推到 `docker.io/library/shorty`，得到 `denied: requested access to the resource is denied`。push 前名字裡必須有你的 registry 和 namespace。
3. **只推 `latest`**：沒辦法回滾，也沒辦法知道線上是哪一版。
4. **重推同一個版本號**：修了 bug 想「偷偷」重推 `1.2.0`，已經 pull 過的機器不會更新，沒 pull 過的拿到新的，線上變成兩種版本混跑。修了就發 `1.2.1`。
5. **label 寫了 version，但 `/version` 回 `dev`**：只傳了 label 用的 `VERSION`，沒傳進 `go build` 的 `-ldflags`，或 runtime stage 忘了重新宣告 `ARG`。兩邊都從同一個 build arg 來最保險。
6. **工作目錄有未 commit 修改還 build**：`revision` label 對到的 commit 根本不是 image 裡的程式碼。`release.sh` 會擋這個。
7. **把 PAT 貼進 Dockerfile 或 compose**：token 只放在 `docker login` 和 CI secret 裡。
8. **Docker Hub pull 次數限制**：CI 大量 `docker pull` 未登入時容易撞到 `toomanyrequests`。CI 裡先 `docker login`，或把常用 base image 鏡像到自己的 registry。

## 練習

1. 用 `release.sh` 發佈 `1.2.0`，再發佈 `1.2.1`，用 `imagetools inspect` 確認 `1.2` 和 `1` 現在指向 `1.2.1` 的 digest，而 `1.2.0` 沒動。
2. 發佈 `1.3.0-rc.1`，確認 `latest` 沒有被更新。
3. 故意不傳 `--build-arg VERSION`，看 `/version` 和 label 各回什麼。
4. 把 `compose.yaml` 的 `app` 改成直接用 `image: ghcr.io/<你>/shorty:1.2.0@sha256:...`（拿掉 `build:`），`docker compose up -d` 後用 `curl localhost:8080/version` 驗證。
5. 同一個 image 同時推到 Docker Hub 和 GHCR，比較兩邊的 digest 是否相同，想想為什麼。

## 下一章

目前推上去的 image 只有你電腦的 CPU 架構。如果你用 Apple Silicon，推上去的是 arm64，丟到 x86 伺服器會跑不起來。第 23 章 [multi-platform](../23-multi-platform) 用 `buildx` 一次 build 出 amd64 和 arm64。
