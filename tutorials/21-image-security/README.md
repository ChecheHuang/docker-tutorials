# 21. Image 安全：non-root、唯讀、掃描與精簡

> 範圍：non-root `USER`、`read_only` + tmpfs、`no-new-privileges`、`--cap-drop ALL`、Trivy／Docker Scout 掃描、pin base image digest、精簡 image；結果就是 `app/Dockerfile`

> 🧸 想先用故事直覺理解這章？看 [STORY.md](./STORY.md)（小學生都懂的比喻版）。

## 為什麼需要這個

到第 20 章為止，shorty 已經「能跑、跑得好」。但上線前要換一個角度問：**如果有一天 shorty 被打穿了，攻擊者能做什麼？**

container 不是 VM，它跟 host 共用同一顆 kernel（第 01 章）。所以 container 的安全靠的是「**層層減少攻擊者手上的東西**」：

| 攻擊者想要 | 我們拿掉 |
|------|------|
| root 權限 | `USER nonroot` |
| 寫入後門、改設定 | `read_only: true` |
| 用 setuid 程式提權 | `no-new-privileges` |
| 改網路、掛載檔案系統、改檔案擁有者 | `--cap-drop ALL` |
| shell、curl、套件管理器 | distroless／scratch 精簡 image |
| 已知漏洞的套件 | Trivy／Docker Scout 掃描 |
| 偷換 base image | pin digest |

沒有任何一項是萬靈丹，但疊起來之後，「打穿一個 bug」就很難變成「拿下整台主機」。

## 核心概念

### 1. non-root USER

Docker 預設用 image 裡的 root（uid 0）跑 process。container 的 root 雖然被 namespace 隔離，但**它跟 host 的 root 是同一個 uid**（除非開了 user namespace remap）。只要再配一個 kernel 或 runtime 漏洞，就有機會逃逸。

```dockerfile
FROM gcr.io/distroless/static-debian12:nonroot
USER nonroot:nonroot     # uid/gid 65532
```

- distroless 的 `:nonroot` 變體已經在 `/etc/passwd` 準備好 `nonroot` 使用者。
- alpine／debian 要自己建：`RUN adduser -D -u 10001 app` 然後 `USER app`。
- 寫數字（`USER 65532:65532`）比寫名字更保險：Kubernetes 的 `runAsNonRoot` 只能驗證數字 uid。

non-root 不能 bind 1024 以下的 port？**在 Docker 裡通常可以**：Docker 20.10 起預設把 container 的 `net.ipv4.ip_unprivileged_port_start` 設成 0。不過 shorty 聽 8080，本來就沒這個問題。

### 2. read_only + tmpfs

`--read-only` 讓 container 的 root filesystem 唯讀。攻擊者沒辦法把惡意程式寫進 `/usr/bin`、也沒辦法改 `/etc`。

真的需要寫的路徑（`/tmp`、cache 目錄）用 tmpfs 補：

```bash
docker run --read-only --tmpfs /tmp:size=16m shorty:ch21
```

shorty 只寫 stdout、資料放 Postgres，所以完全不需要可寫的檔案系統，這也是「log 寫 stdout」（第 05 章）的另一個好處。

### 3. no-new-privileges

Linux 的 setuid 程式（例如 `sudo`、`passwd`）執行時會「變成檔案擁有者」。`no-new-privileges` 告訴 kernel：**這個 process 和它的子孫，永遠不能比現在拿到更多權限**。

```bash
docker run --security-opt no-new-privileges:true shorty:ch21
```

distroless 本來就沒有 setuid 程式，但這是零成本的保險，永遠開著。

### 4. Linux capabilities 與 `--cap-drop ALL`

root 的權力被 kernel 拆成幾十個 capability（`CAP_CHOWN`、`CAP_NET_RAW`、`CAP_SYS_ADMIN`…）。Docker 預設已經只給一小組（約 14 個），但其中像 `NET_RAW`（可以送偽造封包）對 shorty 完全沒用。

```bash
docker run --cap-drop ALL shorty:ch21                         # 全丟
docker run --cap-drop ALL --cap-add NET_BIND_SERVICE nginx    # 需要什麼再加回來
```

原則：**先全丟，缺什麼再一個一個加**，跟防火牆「預設拒絕」一樣。

### 5. 精簡 image

image 裡每多一個套件，就多一份 CVE 清單要追。比較第 06 章和現在：

| image | 內容 | 大小（約） | 有 shell |
|------|------|------|------|
| `golang:1.26`（第 06 章） | Debian + Go 工具鏈 + git | ~900MB | 有 |
| `alpine` | musl + busybox + apk | ~8MB + app | 有 |
| `distroless/static-debian12:nonroot` | CA 憑證、tzdata、passwd | ~2MB + app | **沒有** |
| `scratch` | 什麼都沒有 | 只有 app | 沒有 |

沒有 shell 也代表：攻擊者就算能執行任意指令，也找不到 `sh`、`curl`、`wget` 下載第二階段工具。

### 6. 掃描：Trivy 與 Docker Scout

兩個工具都是「讀 image 裡的套件清單（OS 套件 + Go module／npm／cargo 依賴）→ 對照漏洞資料庫」：

| | Trivy | Docker Scout |
|------|------|------|
| 出品 | Aqua Security，開源 | Docker 官方，Docker Desktop 內建 |
| 怎麼跑 | CLI 或 `aquasec/trivy` image | `docker scout` CLI plugin |
| 額外功能 | 掃 Dockerfile／compose 設定錯誤（`trivy config`）、IaC、secret | base image 升級建議（`recommendations`） |
| CI | `aquasecurity/trivy-action`（第 24 章） | `docker/scout-action` |

掃描結果不是「零 CVE 才能上線」，而是：**HIGH／CRITICAL、而且有修正版的，要處理**。

### 7. pin base image digest

`golang:1.26-alpine` 是一個會移動的 tag：Go 出 1.26.3、alpine 出安全更新，它都會指向新的 image。digest（`sha256:...`）則是內容的雜湊，**永遠只指向同一份 bytes**。

```dockerfile
FROM gcr.io/distroless/static-debian12:nonroot@sha256:<digest>
```

tag 和 digest 同時寫：tag 給人看、digest 給機器驗證。代價是你要主動更新 digest，通常交給 Dependabot／Renovate 開 PR（它們看得懂 `FROM ...@sha256:`）。

## shorty 在這章

本章目錄：

| 檔案 | 用途 |
|------|------|
| `Dockerfile.bad` | 反面教材：`golang:latest`、root、秘密寫在 ENV、多裝 curl、shell form |
| `Dockerfile` | 第 11 章版本 + `:nonroot` + `USER nonroot:nonroot` |
| `compose.hardened.yaml` | runtime 全套：`user`、`read_only`、`tmpfs`、`cap_drop: [ALL]`、`no-new-privileges`、`pids_limit` |

對照最終版：

- **`app/Dockerfile`**：build stage 用 `golang:1.26-alpine`（編譯器只留在 build stage）、runtime 是 `distroless/static-debian12:nonroot`、`USER nonroot:nonroot`、exec form `ENTRYPOINT`、`CGO_ENABLED=0` 靜態連結（才放得進沒有 libc 的 image）。
- **根目錄 `compose.yaml`**：`read_only: true`、`security_opt: [no-new-privileges:true]`、`cap_drop: [ALL]`、`memory: 128M`。本章 `compose.hardened.yaml` 再示範 `tmpfs`、`pids_limit` 等更完整的設定。

## 動手做

### 1. 先 build 好壞兩版，比大小

```bash
docker build -f tutorials/21-image-security/Dockerfile.bad -t shorty:bad app
docker build -f tutorials/21-image-security/Dockerfile -t shorty:ch21 app
docker images shorty
```

範例輸出：

```
REPOSITORY   TAG    IMAGE ID       CREATED          SIZE
shorty       ch21   3f1c0a9b2d7e   10 seconds ago   23.2MB
shorty       bad    9a8b7c6d5e4f   1 minute ago     1.82GB
```

### 2. 秘密真的看得到

```bash
docker history --no-trunc shorty:bad | grep DATABASE_URL
docker image inspect --format '{{.Config.User}}' shorty:bad    # 空字串 = root
docker image inspect --format '{{.Config.User}}' shorty:ch21   # nonroot:nonroot
```

### 3. 掃描：Trivy

不用安裝，直接跑 Trivy 的 image（掛 Docker socket 讓它讀本機 image）：

```bash
docker run --rm -v /var/run/docker.sock:/var/run/docker.sock \
  aquasec/trivy:0.76.1 image --severity HIGH,CRITICAL --ignore-unfixed shorty:bad

docker run --rm -v /var/run/docker.sock:/var/run/docker.sock \
  aquasec/trivy:0.76.1 image --severity HIGH,CRITICAL --ignore-unfixed shorty:ch21
```

範例輸出（`shorty:ch21`）：

```
shorty:ch21 (debian 12.12)
==========================
Total: 0 (HIGH: 0, CRITICAL: 0)

shorty (gobinary)
=================
Total: 0 (HIGH: 0, CRITICAL: 0)
```

`shorty:bad` 則會列出一長串 Debian 套件（curl、openssl、perl…）的 CVE。CI 裡加 `--exit-code 1`，有 HIGH／CRITICAL 就讓 job 失敗。

Trivy 也能掃 Dockerfile 本身的設定問題：

```bash
docker run --rm -v "$PWD":/work -w /work aquasec/trivy:0.76.1 \
  config tutorials/21-image-security/
```

範例輸出（節錄）：會指出 `Dockerfile.bad` 用了 `:latest`、沒有 `USER`（以 root 執行）、`apt-get` 沒加 `--no-install-recommends` 等。

> Windows PowerShell 把 `"$PWD"` 換成 `${PWD}`；Git Bash 遇到路徑被改寫時加 `MSYS_NO_PATHCONV=1`。

### 4. 掃描：Docker Scout

Docker Desktop 已內建，需先 `docker login`：

```bash
docker scout quickview shorty:ch21
docker scout cves --only-severity critical,high shorty:bad
docker scout recommendations shorty:bad      # 建議換哪個 base image
```

範例輸出（`quickview`）：

```
  Target     │  shorty:ch21  │    0C     0H     0M     0L
    digest   │  3f1c0a9b2d7e │
  Base image │  distroless/static-debian12:nonroot │ 0C 0H 0M 0L
```

### 5. 驗證 runtime 限制

用 alpine 感受每個開關（distroless 沒有 shell，沒辦法 exec 進去試）：

```bash
docker run --rm --read-only alpine touch /hello
# touch: /hello: Read-only file system

docker run --rm --read-only --tmpfs /tmp alpine touch /tmp/hello && echo ok
# ok

docker run --rm --cap-drop ALL alpine chown nobody /etc/hostname
# chown: /etc/hostname: Operation not permitted

docker run --rm --user 65532 alpine id
# uid=65532 gid=0(root)
```

然後跑強化版 shorty，確認設定都生效：

```bash
docker compose -f tutorials/21-image-security/compose.hardened.yaml up -d --build
curl localhost:8080/healthz
docker inspect --format 'ro={{.HostConfig.ReadonlyRootfs}} capdrop={{.HostConfig.CapDrop}} sec={{.HostConfig.SecurityOpt}} user={{.Config.User}}' \
  $(docker compose -f tutorials/21-image-security/compose.hardened.yaml ps -q app)
docker compose -f tutorials/21-image-security/compose.hardened.yaml down
```

範例輸出：

```
{"status":"ok"}
ro=true capdrop=[ALL] sec=[no-new-privileges:true] user=65532:65532
```

### 6. 查 digest 並 pin 住

```bash
docker buildx imagetools inspect gcr.io/distroless/static-debian12:nonroot
```

範例輸出（節錄）：

```
Name:      gcr.io/distroless/static-debian12:nonroot
MediaType: application/vnd.oci.image.index.v1+json
Digest:    sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
```

把 `Digest:` 那一行（多平台的 index digest）貼進 `FROM`：

```dockerfile
FROM gcr.io/distroless/static-debian12:nonroot@sha256:0123456789abcdef...
```

> 上面的 digest 只是格式示範，請用你自己查到的值。要 pin 的是 **index（manifest list）的 digest**，不是某一個平台的 digest，否則第 23 章的多平台 build 會壞。

## 對照 Node／TypeScript

| 概念 | Node 世界 | Docker 世界 |
|------|------|------|
| 依賴漏洞掃描 | `npm audit` | `trivy image`／`docker scout cves`（連 OS 套件一起掃） |
| 鎖版本 | `package-lock.json` 的 `integrity` | `FROM ...@sha256:` |
| 自動升級 PR | Dependabot（npm） | Dependabot（docker ecosystem）／Renovate |
| 不用 root | `node` image 內建 `node` 使用者 → `USER node` | `USER nonroot:nonroot` |

Node 特有的坑：

- `node:24` 完整版有 Python、gcc、git，掃出來 CVE 很多；runtime 用 `node:24-slim` 或 `gcr.io/distroless/nodejs24-debian12`。
- `USER node` 要放在 `COPY` 之後並配 `COPY --chown=node:node`，否則 app 要寫 cache 時會 permission denied。
- `npm audit` 只看 JS 依賴；base image 裡的 openssl 漏洞它看不到，所以兩種掃描都要。

## 常見陷阱

1. **`USER` 放錯位置**：`USER` 之後的 `RUN` 也用那個使用者執行，太早寫會讓 `apt-get install` 失敗。慣例是放在最後、`ENTRYPOINT` 前面。
2. **read_only 後 app 啟動就壞**：有些程式會寫 `/tmp`、`~/.cache`、pid 檔。先看錯誤訊息是哪個路徑，再針對那個路徑掛 tmpfs，不要整個關掉 read_only。
3. **以為 ENV 清掉就安全**：`Dockerfile.bad` 示範了，每一層 metadata 都在 `docker history` 裡。秘密只能 runtime 給（第 14 章）或用 `--mount=type=secret`（第 11 章）。
4. **只掃一次**：今天零 CVE，下個月可能新公布十個。掃描要放進 CI（第 24 章），並定期重 build 拿到 base image 的修補。
5. **pin 了 digest 就不更新**：pin 是為了「可重現 + 可審查」，不是「永遠不動」。沒有自動更新流程的 pin，只會讓漏洞永久凍結在 image 裡。
6. **掃描工具本身也是供應鏈**：2026 年 3 月 `aquasecurity/trivy-action` 曾發生 tag 被竄改的事件。CI 裡的第三方 action 最好 pin 到 commit SHA，Trivy image 也指定確切版本，不要用 `latest`。
7. **distroless 沒有 shell 不好 debug**：用 `:debug` 變體（多了 busybox）臨時排查，或 `docker run --rm -it --pid container:<name> --network container:<name> busybox` 掛進同一個 namespace，不要為了 debug 把正式 image 加回 shell。

## 練習

1. 用 Trivy 和 Docker Scout 各掃一次 `shorty:bad`，比較兩者回報的 CRITICAL 數量是否一致，想想為什麼可能不同（資料庫來源、是否忽略無修正版本）。
2. 把 `compose.hardened.yaml` 的 `tmpfs` 拿掉，shorty 還能正常啟動嗎？為什麼？
3. 寫一個 `Dockerfile.alpine`：runtime 用 `alpine`，自己 `adduser` 建立 uid 10001 的使用者並切過去，比較它跟 distroless 版的掃描結果和大小。
4. 查出 `golang:1.26-alpine` 的 index digest，把本章 `Dockerfile` 的兩個 `FROM` 都 pin 住，build 一次確認可以跑。
5. 試試 `docker run --rm --cap-drop ALL nginx`，看 nginx 為什麼起不來，再找出最少要 `--cap-add` 哪幾個 capability。

## 下一章

image 夠安全了，接下來要決定它叫什麼名字、放在哪裡：第 22 章 [tags-registry](../22-tags-registry) 講 tag 策略、digest、OCI label，以及把 shorty 推上 Docker Hub／GHCR。
