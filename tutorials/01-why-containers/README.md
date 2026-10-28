# 01. 為什麼需要 container

> 範圍：「在我電腦可以跑」問題、container vs VM、kernel 共用、Docker 架構（CLI → dockerd → containerd → runc）輪廓

> 🧸 想先用故事直覺理解這章？看 [STORY.md](./STORY.md)（小學生都懂的比喻版）。

## 為什麼需要這個

每個寫過後端的人都講過這句話：

```
「在我電腦可以跑啊！」
```

你的程式在自己電腦好好的，丟到同事電腦、CI、正式機就壞掉。原因幾乎都不是程式碼本身，而是**程式碼以外的東西**不一樣：

| 差異 | 例子 |
|------|------|
| 語言 runtime 版本 | 你用 Go 1.26 / Node 22，伺服器裝的是 Go 1.21 / Node 18 |
| 系統函式庫 | 少了 `libssl`、`ca-certificates`，HTTPS 連不出去 |
| 作業系統 | 你在 Windows／macOS 開發，正式機是 Linux；路徑分隔、大小寫敏感不同 |
| 外部服務 | 你本機的 Postgres 是 16 版，正式機是 13 版 |
| 設定 | 環境變數、時區、語系沒設 |

傳統解法是寫一份「安裝說明」：先裝 X 版、再裝 Y、記得設 Z……然後每一台機器都照著做一遍，祈禱沒人漏步驟。

**Container 的想法是：不要給對方安裝說明，直接給對方「已經裝好的環境」。** 程式、runtime、系統函式庫、設定檔一起打包成一個 image，丟到任何裝了 Docker 的 Linux 機器上，跑起來都一樣。

這一章只講觀念和輪廓，建立「container 到底是什麼」的直覺。第 02 章才開始安裝和跑第一個 container。

## 核心概念

### 1. Container 其實就是「被隔離的 process」

這是最重要、也最常被誤解的一點：**container 不是一台小電腦，它是一個（或一群）普通的 Linux process**，只是 kernel 幫它戴上了眼罩和手銬：

```
                 一台 Linux 主機
┌──────────────────────────────────────────────────┐
│  process A   process B   ┌─────────────────────┐ │
│  (sshd)      (dockerd)   │ container: nginx    │ │
│                          │  看到的檔案：自己的  │ │
│                          │  看到的 PID：1 開始  │ │
│                          │  能用的記憶體：限額  │ │
│                          └─────────────────────┘ │
├──────────────────────────────────────────────────┤
│              同一個 Linux kernel                  │
└──────────────────────────────────────────────────┘
```

Kernel 提供的兩大功能：

| 功能 | 做什麼 | 白話 |
|------|--------|------|
| **namespaces** | 隔離「看得到什麼」：PID、網路、掛載點、hostname、使用者 | 眼罩：只看得到自己的東西 |
| **cgroups** | 限制「能用多少」：CPU、記憶體、I/O、process 數 | 手銬：最多只能吃這麼多資源 |

再加上一個獨立的根檔案系統（來自 image），process 就以為自己在一台乾淨的機器上。

### 2. Container vs VM

```
       虛擬機（VM）                         Container
┌─────────┬─────────┬─────────┐   ┌─────────┬─────────┬─────────┐
│  App A  │  App B  │  App C  │   │  App A  │  App B  │  App C  │
├─────────┼─────────┼─────────┤   ├─────────┼─────────┼─────────┤
│ Libs    │ Libs    │ Libs    │   │ Libs    │ Libs    │ Libs    │
├─────────┼─────────┼─────────┤   └─────────┴─────────┴─────────┘
│Guest OS │Guest OS │Guest OS │   ┌─────────────────────────────┐
│(kernel) │(kernel) │(kernel) │   │   Container runtime (Docker)│
├─────────┴─────────┴─────────┤   ├─────────────────────────────┤
│        Hypervisor           │   │   Host OS（共用一個 kernel） │
├─────────────────────────────┤   ├─────────────────────────────┤
│         硬體                │   │          硬體               │
└─────────────────────────────┘   └─────────────────────────────┘
```

| | VM | Container |
|---|---|---|
| 隔離單位 | 整台虛擬電腦（含自己的 kernel） | process（共用主機 kernel） |
| 啟動時間 | 數十秒到分鐘（要開機） | 毫秒到秒（只是起一個 process） |
| 大小 | GB 等級（整個 OS） | MB 等級（只要 app 需要的檔案） |
| 隔離強度 | 強（硬體虛擬化） | 較弱（共用 kernel，kernel 漏洞會影響全部） |
| 能跑不同 OS？ | 可以（Linux 上跑 Windows VM） | 不行，Linux container 需要 Linux kernel |

兩者不是互斥的：雲端上的 container 通常就跑在 VM 裡。

### 3. Kernel 共用帶來的結果

因為 container 共用主機的 kernel：

- **Linux container 需要 Linux kernel。** 在 Windows／macOS 上，Docker Desktop 其實偷偷開了一台輕量 Linux VM（Windows 用 WSL2，macOS 用 Apple Virtualization framework），container 跑在那台 VM 的 kernel 上。第 02 章會細講。
- **image 裡沒有 kernel。** `alpine` image 只有約 3–4 MB（壓縮後），因為它只放了 `/bin`、`/etc`、`/lib` 這些使用者空間的檔案。
- **「Ubuntu container」不是真的 Ubuntu。** 它只是有 Ubuntu 的檔案和套件管理器，kernel 還是主機那顆。

### 4. Docker 架構輪廓

你打的 `docker run` 只是一個 client。真正做事的是背後一串元件：

```
  你                                    Docker 主機（Linux，或 Docker Desktop 的 VM）
┌──────────┐   REST API          ┌──────────┐   gRPC   ┌────────────┐        ┌──────┐
│ docker   │ ───────────────────▶│ dockerd  │ ───────▶ │ containerd │ ─────▶ │ runc │
│  (CLI)   │ unix socket /       │ (daemon) │          │            │  shim  │      │
└──────────┘ Windows named pipe  └──────────┘          └────────────┘        └──┬───┘
                                   │ image 管理、           │ container          │ 呼叫 kernel
                                   │ 網路、volume、          │ 生命週期、          │ 建 namespaces
                                   │ build（BuildKit）       │ 拉 image            │ + cgroups
                                                                                 ▼
                                                                         你的 process（PID 1）
```

| 元件 | 角色 | 記一句 |
|------|------|--------|
| `docker` CLI | 把你的指令翻成 API 請求 | 櫃台點餐單 |
| `dockerd` | Docker daemon，管 image、網路、volume、build | 店長 |
| `containerd` | 管 container 的生命週期與 image 儲存（CNCF 專案，Kubernetes 也用它） | 廚房主管 |
| `runc` | 依 OCI 規格真正建出 container，建完就退場 | 實際開火的廚師 |

只要記住輪廓就好：**CLI 和 daemon 是分開的**，所以 CLI 可以連到遠端的 Docker 主機；**containerd／runc 是業界標準**，所以 Docker build 出來的 image，Kubernetes 也能跑（都遵守 OCI 規格）。

### 5. 那 shorty 呢？

本系列的實戰專案 `app/`（shorty 短網址服務）從第 06 章才開始包成 image。它是 Go 寫的、編成單一靜態執行檔，最後的 image 只放這一個檔案（見 `app/Dockerfile` 的 `FROM gcr.io/distroless/static-debian12:nonroot`），大約 10 MB 出頭——這正是「container 只放 app 需要的東西」最好的示範。Part 1 我們先用現成的 image 建立直覺。

## 動手做

> 還沒裝 Docker？先看 [第 02 章](../02-install-first-container) 裝好再回來。這幾個實驗只是讓你**親眼看到**上面講的觀念，指令細節後面幾章會一一解釋。所有指令都從 repo 根目錄執行。

### 步驟 1：CLI 和 daemon 是兩個東西

```bash
docker version
```

範例輸出（版本號依你安裝的而定）：

```
Client:
 Version:           28.5.1
 API version:       1.51
 Go version:        go1.24.8
 OS/Arch:           windows/amd64
 Context:           desktop-linux

Server: Docker Desktop 4.48.0 (207573)
 Engine:
  Version:          28.5.1
  API version:      1.51 (minimum version 1.24)
  OS/Arch:          linux/amd64
 containerd:
  Version:          1.7.27
 runc:
  Version:          1.2.5
 docker-init:
  Version:          0.19.0
```

注意兩件事：

- **Client 是 `windows/amd64`，Server 是 `linux/amd64`。** 在 Windows 上，你的 CLI 是 Windows 程式，daemon 跑在 Docker Desktop 的 Linux VM 裡。
- Server 底下列出了 `containerd`、`runc`——就是上面架構圖那一串。

如果只看到 Client、Server 那段是錯誤訊息（`Cannot connect to the Docker daemon` 或 `error during connect`），代表 daemon 沒起來：打開 Docker Desktop 等它變綠燈。

### 步驟 2：container 共用主機的 kernel

```bash
docker run --rm alpine uname -r
docker run --rm ubuntu uname -r
```

範例輸出（Windows + WSL2）：

```
6.6.87.2-microsoft-standard-WSL2
6.6.87.2-microsoft-standard-WSL2
```

兩個不同「發行版」的 container，回報的 kernel 版本**一模一樣**，因為它們都是同一個 kernel 上的 process。macOS 上你會看到類似 `6.10.14-linuxkit`（Docker Desktop VM 的 kernel）；在原生 Linux 上則是你主機的 kernel 版本。

但它們的「使用者空間」不同：

```bash
docker run --rm alpine cat /etc/os-release
```

> **Git Bash**：Git Bash 會把以 `/` 開頭的參數改寫成 Windows 路徑（`/etc/os-release` → `C:/Program Files/Git/etc/os-release`），container 裡當然找不到。先執行 `export MSYS_NO_PATHCONV=1`，或改用 PowerShell。

範例輸出：

```
NAME="Alpine Linux"
ID=alpine
VERSION_ID=3.22.2
PRETTY_NAME="Alpine Linux v3.22"
...
```

### 步驟 3：container 裡的 process 以為自己是 PID 1

```bash
docker run --rm alpine ps
```

範例輸出：

```
PID   USER     TIME  COMMAND
    1 root      0:00 ps
```

整台機器上明明有幾百個 process，container 裡卻只看得到自己，而且自己是 PID 1——這就是 PID namespace 的效果。（PID 1 有特殊責任，例如處理訊號，第 16 章會細講。）

### 步驟 4：感受啟動速度

```bash
time docker run --rm alpine echo hello
```

範例輸出（image 已經下載過的情況）：

```
hello

real    0m0.6s
```

不到一秒就「開機、執行、關機」完畢，因為根本沒有開機這回事——只是起一個 process。

> **PowerShell**：沒有 `time`，改用 `Measure-Command { docker run --rm alpine echo hello }`，看 `TotalMilliseconds`。

### 步驟 5（選做）：看看 CLI 連到哪裡

```bash
docker context ls
```

範例輸出（Windows Docker Desktop）：

```
NAME              DESCRIPTION                               DOCKER ENDPOINT                             ERROR
default           Current DOCKER_HOST based configuration   npipe:////./pipe/docker_engine
desktop-linux *   Docker Desktop                            npipe:////./pipe/dockerDesktopLinuxEngine
```

`npipe://` 是 Windows named pipe；Linux／macOS 上會看到 `unix:///var/run/docker.sock` 這類 unix socket。這就是架構圖裡 CLI → dockerd 那條線。

## 對照 Node／TypeScript

| Node 世界 | Docker 世界 | 差別 |
|-----------|------------|------|
| `.nvmrc` / `engines` 鎖 Node 版本 | image 裡直接放好 Node | `.nvmrc` 只是「請你裝這版」，image 是「已經裝好了」 |
| `package-lock.json` 鎖 npm 套件 | image 連 OS 套件（`libssl`、`ca-certificates`）一起鎖 | lock 檔管不到 `apt install` 的東西 |
| `node_modules` 裡有 native addon（`bcrypt`、`sharp`），Mac 上 `npm install` 完丟到 Linux 就爆 | 在 image 裡（Linux）安裝，跑也在 Linux | 「build 環境＝執行環境」 |
| `pm2` 管 process | Docker 管 container（也是 process） | container 多了隔離與資源限制 |
| Vercel／Netlify 幫你處理 runtime | 自己決定 runtime 長什麼樣 | 自由度高，責任也高 |

一個 Node 開發者最容易有感的例子：你在 macOS 上 `npm install sharp`，下載的是 darwin-arm64 的 binary；把整包 `node_modules` 複製到 Linux 伺服器，`sharp` 直接載入失敗。Container 的做法是**在 Linux image 裡** `npm ci`，從根本消除這類差異（第 06、08 章會示範 `node_modules` 為什麼不要從主機 COPY 進去）。

## 常見陷阱

1. **把 container 當 VM 用**：在一個 container 裡塞 sshd + cron + app + DB，還想 `ssh` 進去改設定。Container 的慣例是「一個 container 一個主要 process」，要改就重新 build image。
2. **以為 container 和 VM 一樣安全**：共用 kernel 代表隔離比 VM 弱。不要用 root 跑服務、不要隨便加 `--privileged`（第 21 章會講 image 安全）。
3. **以為 Docker 能讓 Linux 程式在 Windows 原生跑**：Docker Desktop 是在 VM 裡跑 Linux kernel。真正的 Windows container（跑 Windows 程式）是另一回事，本系列不談。
4. **把「Docker」和「container」畫上等號**：Docker 是一套工具；container 是 Linux kernel 功能 + OCI 標準。Podman、containerd（`nerdctl`）、Kubernetes 都能跑同一個 image。
5. **在 Apple Silicon／ARM 機器上忽略 CPU 架構**：kernel 共用，CPU 架構也共用——amd64 的 image 在 arm64 主機上要靠模擬才能跑，慢而且可能出錯。第 23 章會講多平台 image。

## 練習

1. 用自己的話解釋：為什麼 `docker run --rm ubuntu uname -r` 顯示的不是 Ubuntu 的 kernel？
2. 列出你目前專案中，**不在** `package.json`／`go.mod` 裡、但程式跑起來需要的東西（提示：OS、CA 憑證、時區、外部服務、環境變數）。這些就是 image 要幫你打包的。
3. 跑 `docker run --rm alpine ps` 和 `docker run --rm alpine hostname`，說明你看到的 PID 與 hostname 為什麼和主機不同。
4. 畫出 `docker run alpine echo hi` 從你按下 Enter 到看到 `hi` 經過的元件（CLI → dockerd → containerd → runc → process），每一個寫一句它做了什麼。
5. 想一想：有哪些情況你會選 VM 而不是 container？（提示：要跑不同 OS、需要更強隔離、需要自訂 kernel 模組）

## 下一章

→ [02. 安裝與第一個 container](../02-install-first-container)：裝好 Docker Desktop，跑 `hello-world` 和 nginx。
