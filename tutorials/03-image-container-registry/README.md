# 03. image、container、registry 的關係

> 範圍：image 是唯讀模板、container 是執行中的實例、registry／repository／tag／digest、`pull`／`images`／`rmi`

> 🧸 想先用故事直覺理解這章？看 [STORY.md](./STORY.md)（小學生都懂的比喻版）。

## 為什麼需要這個

第 02 章你打了 `docker run nginx`，Docker 自己去「某個地方」下載了「某個東西」，然後跑起來。這章要把那三個名詞釐清：

- 下載下來的是 **image**
- 跑起來的是 **container**
- 下載的來源是 **registry**

很多 Docker 的困惑（「我改了 container 裡的檔案，怎麼重開就不見了？」「`latest` 怎麼跟昨天的不一樣？」「刪 image 為什麼說有人在用？」）都是因為這三者的關係沒搞清楚。

## 核心概念

### 1. Image vs Container

```
                 image（唯讀，像 class）
            ┌─────────────────────────────┐
            │ nginx:1.27-alpine           │
            │  layer 3: nginx 設定         │
            │  layer 2: nginx 執行檔       │
            │  layer 1: alpine 基本檔案    │
            └──────────────┬──────────────┘
          docker run       │        docker run
         ┌─────────────────┼─────────────────┐
         ▼                                   ▼
┌──────────────────┐               ┌──────────────────┐
│ container web-a  │               │ container web-b  │
│ ┌──────────────┐ │               │ ┌──────────────┐ │
│ │ 可寫層（自己的）│ │               │ │ 可寫層（自己的）│ │
│ └──────────────┘ │               │ └──────────────┘ │
│  + 共用的 image   │               │  + 共用的 image   │
│  + 執行中的 process│              │  + 執行中的 process│
└──────────────────┘               └──────────────────┘
```

| | image | container |
|---|---|---|
| 本質 | 一疊唯讀的檔案系統 layer + 設定（要跑什麼指令、環境變數、port） | image + 一層可寫層 + 一個（或一群）process |
| 可以改嗎 | 不行，image 永遠唯讀 | 可以寫檔，但寫在自己的可寫層 |
| 數量關係 | 一個 | 同一個 image 可以生出任意多個 |
| 刪掉會怎樣 | 本機少了模板，可以再 pull | 可寫層裡的變更跟著消失 |

**重點：container 裡的變更不會寫回 image。** 刪掉 container，變更就沒了。要保存資料用 volume（第 13 章），要改 image 就改 Dockerfile 重新 build（第 06 章）。

Layer 是 image 能共用、能快取的關鍵：`nginx:1.27-alpine` 和 `redis:7-alpine` 都建立在 alpine 上，相同的 layer 在本機只存一份。第 07 章會細講 layer 怎麼來、怎麼快取。

### 2. Image 的完整名字

你打的 `nginx` 其實是縮寫。完整的 image reference 長這樣：

```
docker.io / library / nginx : 1.27-alpine @ sha256:65645c7b...
└───┬───┘   └──┬──┘   └─┬─┘   └────┬────┘   └──────┬───────┘
 registry   namespace  repo       tag            digest
```

| 部分 | 意思 | 省略時的預設 |
|------|------|-------------|
| **registry** | 存放 image 的伺服器 | `docker.io`（Docker Hub） |
| **namespace** | 使用者或組織 | Docker Hub 官方 image 是 `library` |
| **repository** | 一個 image 的所有版本放在同一個 repository | （必填） |
| **tag** | 人取的版本名稱，**可以被移動** | `latest` |
| **digest** | image 內容的 SHA-256 雜湊，**內容不變它就不變** | （不指定就用 tag） |

所以這些寫法指的是同一個東西：

```
nginx
nginx:latest
docker.io/library/nginx:latest
```

其他 registry 的例子：

| image | registry |
|-------|----------|
| `postgres:17-alpine` | Docker Hub 官方 image |
| `ghcr.io/<owner>/shorty:v1.0.0` | GitHub Container Registry（第 22、24 章會推 shorty 上去） |
| `gcr.io/distroless/static-debian12:nonroot` | Google 的 registry——shorty 最終版 `app/Dockerfile` 的 runtime base image |
| `mcr.microsoft.com/dotnet/runtime:8.0` | Microsoft Container Registry |

### 3. Tag vs Digest

```
           repository: nginx
  tag（標籤，可移動）          digest（內容指紋，不可變）
  ┌──────────────┐
  │ latest       │──┐
  │ 1.27         │──┼──────▶ sha256:65645c7b…   ← 今天
  │ 1.27-alpine  │  │
  └──────────────┘  └ ─ ─ ─▶ sha256:9f1c2d3e…   ← 下個月 nginx 發新版，latest 就移過去了
```

- **tag 像便利貼**，誰都可以撕下來貼到別的 image 上。`latest` 只是一個預設名字，**不代表「最新」**，也不保證今天和明天是同一個東西。
- **digest 像指紋**，由內容算出來，同一個 digest 永遠指向同一份內容。要百分之百重現，就用 digest。

Tag 怎麼取、為什麼正式環境不要只靠 `latest`，是第 22 章的主題。

### 4. 一個 tag，多個平台

同一個 `nginx:1.27-alpine` 其實同時有 amd64、arm64 等版本。Registry 上的 tag 指向一份「清單」（image index），pull 時 Docker 自動挑符合你機器架構的那份。所以 Apple Silicon 和 Intel 打同一個指令都能用。細節在第 23 章。

## 動手做

所有指令從 repo 根目錄執行。

### 步驟 1：pull 幾個 image

```bash
docker pull alpine:3.22
```

範例輸出：

```
3.22: Pulling from library/alpine
2d35ebdb57d9: Pull complete
Digest: sha256:4b7ce07002c69e8f3d704a9c5d6fd3053be500b7f1c69fc0d80990c2ad8dd412
Status: Downloaded newer image for alpine:3.22
docker.io/library/alpine:3.22
```

- `2d35ebdb57d9: Pull complete`：每一行是一個 layer
- `Digest:`：這次拿到的內容指紋
- 最後一行：補完整的 image reference

再多拉幾個，後面會用到：

```bash
docker pull nginx:1.27-alpine
docker pull redis:7-alpine
docker pull postgres:17-alpine
```

拉 `redis:7-alpine` 時你可能看到某一行寫 `Already exists`——那是和之前 image 共用的 layer，不用再下載。

### 步驟 2：列出本機的 image

```bash
docker images
```

範例輸出：

```
REPOSITORY    TAG           IMAGE ID       CREATED        SIZE
postgres      17-alpine     a1b2c3d4e5f6   2 weeks ago    278MB
redis         7-alpine      b2c3d4e5f6a7   3 weeks ago    41.4MB
nginx         1.27-alpine   c3d4e5f6a7b8   6 months ago   48.2MB
alpine        3.22          d4e5f6a7b8c9   3 weeks ago    8.31MB
nginx         latest        e5f6a7b8c9d0   2 weeks ago    192MB
hello-world   latest        f6a7b8c9d0e1   5 months ago   10.1kB
```

> 不同版本的 CLI 與 image store 顯示的欄位、大小可能不同（例如較新版本會分成磁碟用量與內容大小），概念一樣。`docker images` 和 `docker image ls` 是同一個指令。

只看某個 repository：

```bash
docker image ls nginx
```

看 digest：

```bash
docker images --digests alpine
```

範例輸出：

```
REPOSITORY   TAG       DIGEST                                                                    IMAGE ID       CREATED       SIZE
alpine       3.22      sha256:4b7ce07002c69e8f3d704a9c5d6fd3053be500b7f1c69fc0d80990c2ad8dd412   d4e5f6a7b8c9   3 weeks ago   8.31MB
```

IMAGE ID 是本機用的識別碼；DIGEST 是 registry 上那份內容的指紋。比對兩台機器是不是跑同一個 image，看 digest 最準。

### 步驟 3：看 image 的基本資訊

```bash
docker image inspect alpine:3.22 --format '{{.Os}}/{{.Architecture}}'
docker image inspect nginx:1.27-alpine --format '{{json .Config.Cmd}}'
```

範例輸出：

```
linux/amd64
["nginx","-g","daemon off;"]
```

第一行：Docker 幫你挑了符合主機架構的版本（Apple Silicon 會是 `linux/arm64`）。第二行：image 裡記錄了「預設要跑什麼指令」——container 啟動時就是跑這個。`inspect` 和 `--format` 的完整用法在第 05 章。

> **引號**：`--format` 的模板在 Git Bash 和 PowerShell 都用單引號就好。如果你在 `cmd.exe`，要改成雙引號：`--format "{{.Os}}"`。

### 步驟 4：同一個 image，container 各自獨立

用同一個 alpine image 跑一個 container，在裡面寫一個檔案：

```bash
docker run --name note alpine sh -c 'echo hi > /hello.txt'
```

看這個 container 相對於 image 改了什麼：

```bash
docker diff note
```

範例輸出：

```
A /hello.txt
```

`A` 是 Added（還有 `C` Changed、`D` Deleted）。這個檔案只在 `note` 的可寫層裡。

從**同一個 image** 再開一個新 container 去找它：

```bash
docker run --rm alpine cat /hello.txt
```

> **Git Bash**：如果錯誤訊息裡出現 `C:/Program Files/Git/hello.txt`，是 Git Bash 把 `/hello.txt` 改寫成 Windows 路徑了。先 `export MSYS_NO_PATHCONV=1` 再跑（第 02 章常見陷阱有說明）。

範例輸出：

```
cat: can't open '/hello.txt': No such file or directory
```

image 沒被改到，新 container 是乾淨的。清掉 `note`：

```bash
docker rm note
```

### 步驟 5：tag 只是一個指標

```bash
docker tag alpine:3.22 my-alpine:test
docker image ls --filter reference='*alpine*'
```

範例輸出：

```
REPOSITORY   TAG       IMAGE ID       CREATED       SIZE
my-alpine    test      d4e5f6a7b8c9   3 weeks ago   8.31MB
alpine       3.22      d4e5f6a7b8c9   3 weeks ago   8.31MB
```

兩行的 IMAGE ID **一樣**——沒有複製任何東西，只是多了一張便利貼。（用 tag 管理自己 image 的版本是第 22 章的事。）

### 步驟 6：用 digest pull

把步驟 2 看到的 digest 貼上（你的會跟範例不同）：

```bash
docker pull alpine@sha256:4b7ce07002c69e8f3d704a9c5d6fd3053be500b7f1c69fc0d80990c2ad8dd412
```

不管 `alpine:3.22` 這個 tag 以後被移到哪，這個 digest 拉到的永遠是同一份內容。

### 步驟 7：刪除 image

先刪剛剛的便利貼：

```bash
docker rmi my-alpine:test
```

範例輸出：

```
Untagged: my-alpine:test
```

只有 `Untagged`，沒有 `Deleted`——因為 `alpine:3.22` 還指著同一個 image，內容不會被刪。

再試試刪掉一個「有 container 在用」的 image：

```bash
docker run -d --name web nginx:1.27-alpine
docker rmi nginx:1.27-alpine
```

範例輸出：

```
Error response from daemon: conflict: unable to delete nginx:1.27-alpine (must be forced) - container 1a2b3c4d5e6f is using its referenced image c3d4e5f6a7b8
```

就算 container 已經停了也一樣——container 是 image 生出來的，container 還在，image 就不能直接刪。先刪 container 再刪 image：

```bash
docker rm -f web
docker rmi nginx:1.27-alpine
```

範例輸出：

```
web
Untagged: nginx:1.27-alpine
Untagged: nginx@sha256:65645c7bb6a0661892a8b03b89d0743208a18dd2f3f17a54ef4b76fb8e2f2a10
Deleted: sha256:c3d4e5f6a7b8...
```

（`docker rm -f` 的意思第 04 章會講。）

## 對照 Node／TypeScript

| npm 世界 | Docker 世界 |
|----------|------------|
| npm registry（registry.npmjs.org） | Docker Hub（docker.io） |
| 私有 registry（GitHub Packages、Verdaccio） | GHCR、ECR、GCR、自架 registry |
| `@scope/package` | `namespace/repository` |
| `react@18.3.1`（版本號） | `nginx:1.27-alpine`（tag） |
| dist-tag `latest`（`npm publish` 時會移動） | tag `latest`（一樣會移動，一樣不可靠） |
| `package-lock.json` 裡的 `integrity: sha512-...` | digest `sha256:...` |
| `npm install` 下載到快取 | `docker pull` 下載到本機 image store |
| 發布出去的 package tarball（不能改，只能發新版） | image（唯讀，只能 build 新的） |
| 用那個 package 跑起來的 `node` process | container |

兩個最有感的對照：

- **`"react": "^18.0.0"` 每次 install 可能拿到不同版本**，所以要 lock 檔；Docker 的 `nginx:latest` 也一樣，要穩就用明確的 tag，要絕對可重現就用 digest。
- **一個 package 可以同時被很多專案 `require`**，各自的執行狀態互不影響；一個 image 也可以同時跑很多 container，各自的可寫層互不影響。

## 常見陷阱

1. **以為 `latest` 是「最新版」**：它只是沒寫 tag 時的預設名字。`docker pull nginx` 拿到的是 nginx 維護者把 `latest` 貼到的那個版本，而且本機已經有 `nginx:latest` 的話，`docker run nginx` **不會**自動去拉新的。
2. **在 container 裡改設定，以為改到 image**：`docker diff` 看得到的變更只存在那個 container。重建就沒了。
3. **刪不掉 image**：有 container（包括已停止的）在用它。`docker ps -a` 找出來，先 `docker rm`。
4. **`<none>:<none>` 的 image 越來越多**：tag 被移到新 image 後，舊的就變成沒名字的「dangling image」。用 `docker image prune` 清（第 04 章的 `system prune` 會一起清）。
5. **Docker Hub 的 pull 次數限制**：匿名 pull 有次數上限，CI 或公司共用 IP 很容易撞到 `toomanyrequests: You have reached your pull rate limit`。`docker login` 後額度較高。
6. **拼錯名字或 tag**：`pull access denied for ngnix, repository does not exist or may require 'docker login'`——多半只是打錯字，或 tag 不存在（`manifest for nginx:1.99 not found`）。
7. **Windows：刪了 image 磁碟空間沒變大**：Docker Desktop 的資料存在 WSL2 的虛擬磁碟檔（`.vhdx`）裡，檔案不一定會自動縮小。Docker Desktop 的 Troubleshoot／Resources 設定裡可以清理或調整磁碟；磁碟被塞滿的排查是第 25 章的演練。

## 練習

1. 寫出 `redis:7-alpine` 的完整 image reference（含 registry 和 namespace）。
2. 用 `docker images --digests` 找到 `postgres:17-alpine` 的 digest，用 digest 再 pull 一次，確認 IMAGE ID 相同。
3. 跑 `docker run --name t1 alpine sh -c 'mkdir /data && echo 1 > /data/a && rm /etc/motd'`，用 `docker diff t1` 看結果，說明每一行的 `A`／`C`／`D` 代表什麼。最後刪掉 `t1`。
4. 用 `docker tag` 幫 `nginx:latest` 多貼兩個名字，再一一 `docker rmi`，觀察什麼時候出現 `Deleted`。
5. 用 `docker image inspect --format '{{.Os}}/{{.Architecture}}'` 檢查你本機所有 image 的平台。

## 下一章

→ [04. 常用指令與 container 生命週期](../04-cli-lifecycle)：`create`、`start`、`stop`、`kill`、`rm`，以及 container 的狀態機。
