# 02. 安裝與第一個 container

> 範圍：Docker Desktop／Docker Engine 安裝、`docker run hello-world`、`docker run -d -p 8080:80 nginx`、Windows／WSL2 與 macOS 的差異

> 🧸 想先用故事直覺理解這章？看 [STORY.md](./STORY.md)（小學生都懂的比喻版）。

## 為什麼需要這個

第 01 章說 container 是「被隔離的 Linux process」。要跑它，你需要兩樣東西：

1. 一個 **Linux kernel**（container 共用它）
2. 一套 **Docker 工具**（CLI + daemon + containerd + runc）

在 Linux 上兩樣都很直接；在 Windows／macOS 上，第 1 樣得靠一台藏起來的 Linux VM。這章把環境裝好，然後跑兩個 container：一個印完訊息就結束的 `hello-world`，一個在背景持續服務的 nginx 網站。

## 核心概念

### 三種平台，三種長相

```
 Linux（原生）                 Windows + Docker Desktop           macOS + Docker Desktop
┌──────────────────┐        ┌───────────────────────────────┐   ┌──────────────────────────────┐
│ docker CLI       │        │ docker.exe (CLI)               │   │ docker (CLI)                 │
│     │ unix sock  │        │     │ named pipe               │   │     │ unix socket            │
│ dockerd          │        │ ┌───▼──────── WSL2 VM ───────┐ │   │ ┌───▼──── Linux VM ────────┐ │
│ containerd/runc  │        │ │ dockerd / containerd / runc│ │   │ │ dockerd / containerd     │ │
│ container        │        │ │ container                  │ │   │ │ container                │ │
├──────────────────┤        │ ├────────────────────────────┤ │   │ ├──────────────────────────┤ │
│ 主機 Linux kernel │        │ │ WSL2 Linux kernel          │ │   │ │ VM 的 Linux kernel        │ │
└──────────────────┘        │ └────────────────────────────┘ │   │ └──────────────────────────┘ │
                            │ Windows                        │   │ macOS（Apple Virtualization）│
                            └───────────────────────────────┘   └──────────────────────────────┘
```

| | Linux | Windows | macOS |
|---|---|---|---|
| 建議安裝 | Docker Engine（套件庫） | Docker Desktop（WSL2 後端） | Docker Desktop |
| Linux kernel 在哪 | 主機本身 | WSL2 的 VM | Docker Desktop 的 VM |
| `-p` 對外的 port | 主機直接 listen | Docker Desktop 幫你轉到 Windows 的 `localhost` | 同左，轉到 macOS 的 `localhost` |
| 效能注意 | 無 | 原始碼放 WSL 檔案系統比放 `C:\` 快很多（第 13 章） | bind mount 檔案 I/O 較慢（第 13 章） |

### `docker run` 做了什麼

```
docker run -d -p 8080:80 nginx
           │   │          │
           │   │          └─ image 名稱（沒寫 tag → nginx:latest）
           │   └─ 主機 8080 → container 80（第 12 章細講 port 與 NAT）
           └─ detached：丟到背景跑，印出 container ID 就回來

1. 本機有 nginx:latest 嗎？沒有 → 從 Docker Hub pull（第 03 章）
2. 用這個 image 建立一個新 container
3. 啟動它（nginx 開始 listen container 內的 80 port）
4. 設定 port 轉發：主機 8080 → container 80
```

`hello-world` 和 `nginx` 最大的差別：`hello-world` 的程式印完訊息就結束，所以 container 也跟著結束；nginx 是常駐的 server，會一直跑到你叫它停。**container 的壽命 = 裡面主 process 的壽命**（第 04 章會細講狀態機）。

## 動手做

所有指令從 repo 根目錄執行。

### 步驟 1：安裝

**Windows（作者的環境）**

1. 確認 Windows 10 22H2 以上或 Windows 11，BIOS 已開啟虛擬化（工作管理員 → 效能 → CPU →「虛擬化：已啟用」）。
2. 以系統管理員開 PowerShell，安裝／更新 WSL2：

   ```powershell
   wsl --install
   wsl --update
   wsl --status
   ```

3. 從 <https://www.docker.com/products/docker-desktop/> 下載 Docker Desktop，安裝時勾選 **Use WSL 2 instead of Hyper-V**。
4. 開 Docker Desktop，等左下角顯示 **Engine running**。
5. （選做）如果你平常在 WSL 的 Ubuntu 裡開發：Settings → Resources → WSL integration，打開你的 distro，Ubuntu 裡就能直接用 `docker`。

**macOS**

從同一個網址下載 Docker Desktop，注意選對 **Apple Silicon** 或 **Intel** 版本，拖進 Applications 後打開。

**Linux（Ubuntu 為例）**

依官方文件 <https://docs.docker.com/engine/install/ubuntu/> 加入 Docker 的 apt 套件庫後安裝：

```bash
sudo apt-get install docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
sudo usermod -aG docker $USER   # 讓自己不用 sudo 就能跑 docker；要重新登入才生效
```

> 開發機想快速裝，也可以用官方的 `curl -fsSL https://get.docker.com | sh`，但正式機請照文件走套件庫。加入 `docker` 群組等同給了 root 權限，只在自己的機器上這樣做。

> **授權**：Docker Desktop 對個人、教育、小型企業（員工少於 250 人且年營收少於 1,000 萬美元）免費；大公司需付費訂閱。Linux 上的 Docker Engine 是開源的，沒有這個限制。

### 步驟 2：確認安裝

```bash
docker version
docker compose version
```

範例輸出：

```
Client:
 Version:           28.5.1
 ...
 OS/Arch:           windows/amd64
 Context:           desktop-linux

Server: Docker Desktop 4.48.0 (207573)
 Engine:
  Version:          28.5.1
  ...
  OS/Arch:          linux/amd64

Docker Compose version v2.40.0
```

`Client` 和 `Server` 兩段都要有。`docker compose`（中間是空格，Compose v2）是 Docker CLI 的 plugin，第 17 章才用到，先確認它在。舊的 `docker-compose`（連字號，v1）已經停止維護，不要再用。

想看更多 daemon 資訊：

```bash
docker info
```

範例輸出（節錄）：

```
Server:
 Containers: 0
  Running: 0
  Paused: 0
  Stopped: 0
 Images: 0
 Server Version: 28.5.1
 Storage Driver: overlayfs
 ...
 Kernel Version: 6.6.87.2-microsoft-standard-WSL2
 Operating System: Docker Desktop
 OSType: linux
 Architecture: x86_64
 CPUs: 8
 Total Memory: 7.6GiB
```

`OSType: linux`——即使你在 Windows 上，daemon 也是跑在 Linux。

### 步驟 3：第一個 container — hello-world

```bash
docker run hello-world
```

範例輸出：

```
Unable to find image 'hello-world:latest' locally
latest: Pulling from library/hello-world
17eec7bbc9d7: Pull complete
Digest: sha256:54e66cc1dd1fcb1c3c58bd8017914dbed8701e2d8c74d9262e26bd9cc1642d31
Status: Downloaded newer image for hello-world:latest

Hello from Docker!
This message shows that your installation appears to be working correctly.

To generate this message, Docker took the following steps:
 1. The Docker client contacted the Docker daemon.
 2. The Docker daemon pulled the "hello-world" image from the Docker Hub.
    (amd64)
 3. The Docker daemon created a new container from that image which runs the
    executable that produces the output you are currently reading.
 4. The Docker daemon streamed that output to the Docker client, which sent it
    to your terminal.
...
```

這段訊息本身就是第 01 章架構圖的文字版：client → daemon → pull image → 建 container → 把輸出串流回來。

再跑一次：

```bash
docker run hello-world
```

這次不會有 `Pulling` 那幾行——image 已經在本機了，直接建一個**新的** container 來跑。

### 步驟 4：第一個常駐服務 — nginx

```bash
docker run -d -p 8080:80 --name web nginx
```

範例輸出：

```
Unable to find image 'nginx:latest' locally
latest: Pulling from library/nginx
...
Status: Downloaded newer image for nginx:latest
3f2a1b9c8d7e6f5a4b3c2d1e0f9a8b7c6d5e4f3a2b1c0d9e8f7a6b5c4d3e2f1a
```

最後那一長串就是 container ID。`--name web` 幫它取個好記的名字（第 04 章會細講）。

看看它在不在跑：

```bash
docker ps
```

範例輸出：

```
CONTAINER ID   IMAGE     COMMAND                  CREATED         STATUS         PORTS                                     NAMES
3f2a1b9c8d7e   nginx     "/docker-entrypoint.…"   5 seconds ago   Up 4 seconds   0.0.0.0:8080->80/tcp, [::]:8080->80/tcp   web
```

`PORTS` 欄的 `0.0.0.0:8080->80/tcp` 就是 `-p 8080:80` 的結果。

### 步驟 5：打開網站

瀏覽器開 <http://localhost:8080>，會看到 **Welcome to nginx!**。

用指令確認：

```bash
# Git Bash / macOS / Linux
curl -s http://localhost:8080 | grep title
```

```powershell
# PowerShell：用 curl.exe，避免 Windows PowerShell 5.1 把 curl 當成 Invoke-WebRequest 的別名
curl.exe -s http://localhost:8080 | Select-String title
```

範例輸出：

```
<title>Welcome to nginx!</title>
```

### 步驟 6：收拾

```bash
docker stop web
docker rm web
```

範例輸出：

```
web
web
```

`stop`、`rm` 的差別和整個生命週期是第 04 章的主題，這裡先照著打。

### 步驟 7（Windows 選做）：限制 WSL2 吃的記憶體

WSL2 的 VM 預設最多可用主機一半的記憶體。想限制它，在 `%UserProfile%\.wslconfig` 寫：

```ini
[wsl2]
memory=6GB
processors=4
```

然後 `wsl --shutdown`，再重開 Docker Desktop。（這是 VM 整體的上限；單一 container 的限制是第 15 章的 `--memory`。）

## 對照 Node／TypeScript

| Node 世界 | Docker 世界 |
|-----------|------------|
| 安裝 Node（nvm／官方安裝檔） | 安裝 Docker Desktop／Engine |
| `node --version` | `docker version`（多一段 Server，因為是 client／server 架構） |
| `npx create-vite` → 第一次會下載套件再執行 | `docker run nginx` → 第一次會 pull image 再執行 |
| `node script.js` 跑完就結束 | `docker run hello-world`：主 process 結束 → container 結束 |
| `node server.js` 常駐，`Ctrl+C` 才停 | `docker run -d nginx`：常駐在背景，`docker stop` 才停 |
| `app.listen(3000)`，瀏覽器開 `localhost:3000` | container 裡 listen 80，靠 `-p 8080:80` 才能從主機的 `localhost:8080` 連到 |

最後一列是 Node 開發者最常卡住的地方：在 container 裡 `listen(80)` 不代表你的主機有 80 port 在聽。**不加 `-p`，主機根本連不到它。**

## 常見陷阱

1. **`Cannot connect to the Docker daemon` / `error during connect`**：CLI 裝好了但 daemon 沒起來。Windows／macOS 打開 Docker Desktop 等它 Engine running；Linux 跑 `sudo systemctl enable --now docker`。
2. **Linux 上 `permission denied while trying to connect to the Docker daemon socket`**：還沒加入 `docker` 群組，或加了但沒重新登入（可先 `newgrp docker`）。
3. **port 被佔用**：`Bind for 0.0.0.0:8080 failed: port is already allocated` 表示 8080 已經有人在用（可能是你的 dev server，或另一個 container）。換一個：`-p 8081:80`。
4. **Windows 上明明沒人用的 port 也綁不了**：錯誤像 `ports are not available: ... bind: An attempt was made to access a socket in a way forbidden by its access permissions`。Hyper-V／WSL 會保留一段 port 範圍，用 `netsh interface ipv4 show excludedportrange protocol=tcp` 查，換一個不在範圍內的 port。
5. **PowerShell 的 `curl` 不是 curl**：Windows PowerShell 5.1 的 `curl` 是 `Invoke-WebRequest` 的別名，參數完全不同；一律打 `curl.exe`。（PowerShell 7 已拿掉這個別名，但打 `curl.exe` 兩邊都通。）
6. **Git Bash 的 `-it` 出現 `the input device is not a TTY`**：舊版 mintty 的問題，在指令前加 `winpty`，或改用 Windows Terminal + PowerShell。
7. **Git Bash 偷改路徑**：Git Bash（MSYS）會把以 `/` 開頭的參數當成 Unix 路徑，自動改寫成 Windows 路徑，例如 `docker run --rm alpine ls /etc` 實際送出的是 `C:/Program Files/Git/etc`。在 Git Bash 用 Docker 時先 `export MSYS_NO_PATHCONV=1`（可寫進 `~/.bashrc`），或在路徑前多加一個斜線（`//etc`）。PowerShell 沒有這個問題。
8. **Apple Silicon 上跑 amd64-only 的 image**：會看到 `The requested image's platform (linux/amd64) does not match the detected host platform (linux/arm64/v8)` 警告，能跑但走模擬、比較慢。第 23 章會講多平台。
9. **忘了 `-d`**：`docker run -p 8080:80 nginx` 會把 nginx 的 log 直接印在終端機、佔住它。按 `Ctrl+C` 會停掉 container；想要背景跑就加 `-d`。

## 練習

1. 跑 `docker run -d -p 8081:80 --name web2 nginx`，用瀏覽器開 <http://localhost:8081>。同時再開一個 `-p 8082:80 --name web3`，說明為什麼兩個 nginx 都 listen 80 卻不衝突。
2. 刻意再跑一次 `docker run -d -p 8081:80 nginx`，觀察錯誤訊息；把它改成能成功的版本。
3. 不加 `-d` 跑 `docker run -p 8080:80 nginx`，在瀏覽器重新整理幾次，看終端機印出什麼？按 `Ctrl+C` 後 `docker ps` 還看得到它嗎？
4. 執行 `docker info`，找出 `Kernel Version` 和 `OSType`，用第 01 章的觀念解釋為什麼在 Windows 上會是 Linux。
5. 收拾：用 `docker stop` + `docker rm` 把練習 1 的 container 清掉。

## 下一章

→ [03. image、container、registry 的關係](../03-image-container-registry)：剛剛 pull 下來的 `nginx` 到底是什麼？tag、digest 又是什麼？
