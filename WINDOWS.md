# 在 Windows 上學這套教學

整套 25 章都可以在 Windows 10 22H2 以上／Windows 11（含 Home 版）完成。關鍵只有兩件事：**Docker 跑在 WSL2 上**，以及**把 repo 放在 WSL2 的檔案系統裡**。照這份設定好，後面各章就跟在 Linux 上學一樣。

> 安裝的基本步驟與 macOS／Linux 的做法見[第 02 章](./tutorials/02-install-first-container/README.md)。這份只講 Windows 特有的選擇與坑。

## 1. 選一種裝法

| | A. Docker Desktop | B. WSL2 裡直接裝 Docker Engine |
|---|---|---|
| 安裝 | 下載安裝檔，最簡單 | 在 Ubuntu（WSL2）裡用 apt 安裝 |
| 圖形介面 | 有（看 container、image、volume、設定） | 沒有，全部用指令 |
| 授權 | 個人、教育、小型企業（員工少於 250 人**且**年營收少於 1,000 萬美元）免費；**超過就要付費訂閱** | 開源，免費 |
| 跟正式環境的差距 | 多一層 Desktop 自己的 VM 與網路轉發 | 就是 Linux 上的 Docker，跟伺服器一樣 |
| 適合 | 想快速開始、需要 GUI | 公司電腦授權有疑慮、想跟正式環境一致 |

**兩種擇一，不要同時裝**：兩邊都會搶 `docker` 指令和 socket，很容易搞不清楚現在連的是哪一個 Engine。

**用公司電腦學的話，先確認公司規模**。不確定是否符合免費條件時，選 B 最保險。

## 2. 安裝 WSL2（兩種裝法都需要）

以**系統管理員**開 PowerShell：

```powershell
wsl --install          # 預設會裝 Ubuntu；第一次裝完要重開機
wsl --update
wsl --status           # 確認「預設版本：2」
```

重開機後從開始功能表打開 **Ubuntu**，設定 Linux 使用者名稱和密碼。

## 3A. 裝 Docker Desktop

1. 從 <https://www.docker.com/products/docker-desktop/> 下載安裝，勾選 **Use WSL 2 instead of Hyper-V**。
2. 開 Docker Desktop，等狀態顯示 **Engine running**。
3. Settings → Resources → **WSL integration**，打開 Ubuntu。之後在 Ubuntu 裡下 `docker` 就會連到 Desktop 的 Engine。

## 3B. 在 WSL2 的 Ubuntu 裡裝 Docker Engine

以下都在 **Ubuntu 終端機**裡執行，步驟依官方文件 <https://docs.docker.com/engine/install/ubuntu/>：

```bash
# 加入 Docker 官方 apt 套件庫
sudo apt-get update
sudo apt-get install -y ca-certificates curl
sudo install -m 0755 -d /etc/apt/keyrings
sudo curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
sudo chmod a+r /etc/apt/keyrings/docker.asc
echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] \
  https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo "$VERSION_CODENAME") stable" \
  | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null

# 安裝 Engine + buildx + compose
sudo apt-get update
sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin

# 不用 sudo 就能跑 docker（要關掉終端機重開才生效）
sudo usermod -aG docker $USER
```

確認 WSL 有開 systemd，Docker 才會自動啟動。`wsl --install` 裝的新版 Ubuntu 預設就有開，檢查一下：

```bash
cat /etc/wsl.conf
# 應該看到：
# [boot]
# systemd=true
```

沒有的話自己加上這兩行，然後在 **PowerShell** 執行 `wsl --shutdown`，再重開 Ubuntu。最後確認：

```bash
docker version            # Client 和 Server 都要有版本號
docker compose version
docker buildx version
docker run --rm hello-world
```

## 4. 把 repo clone 到 WSL2 裡

**不要**在 `C:\work\...`（Windows 檔案系統）上跑這套教學，在 Ubuntu 裡重新 clone 一份：

```bash
mkdir -p ~/tutorials && cd ~/tutorials
git clone https://github.com/ChecheHuang/docker-tutorials.git
cd docker-tutorials
./run list
```

為什麼：

| 問題 | 放在 `C:\` | 放在 WSL2（`~/`） |
|------|-----------|------------------|
| bind mount 速度（第 13、19 章） | 跨檔案系統，明顯變慢 | 原生速度 |
| 熱重載（第 19 章 air、Compose Watch） | 常常收不到檔案變更通知，改了不會重編 | 正常 |
| 換行字元 | Git for Windows 可能把 `run`、`*.sh` 轉成 CRLF，執行時出現 `bad interpreter` | 一律 LF |
| Git Bash 路徑改寫 | `/etc/...` 被改成 `C:/Program Files/Git/etc/...` | 沒有這個問題 |

**編輯器**：VS Code 裝 **WSL** 擴充套件，在 Ubuntu 的 repo 目錄下執行 `code .`，VS Code 就會直接編輯 WSL2 裡的檔案。

**從 Windows 檔案總管看檔案**：網址列輸入 `\\wsl.localhost\Ubuntu\home\<你的使用者名稱>\tutorials`。

**瀏覽器**：container 用 `-p 8080:8080` 開的 port，在 Windows 瀏覽器打 `http://localhost:8080` 就連得到（WSL2 預設會把 localhost 轉發給 Windows）。

**跑 shorty 的測試**（`./run test`、`./run app`）需要 Go。只學 Docker 的話可以不裝，因為 Dockerfile 用的是 `golang` image；想裝的話依 <https://go.dev/doc/install> 在 Ubuntu 裡安裝 Linux 版。

## 5. 限制 WSL2 的記憶體與 CPU

WSL2 預設最多可以吃掉 Windows 一半（或更多）的記憶體。在 Windows 的 `%UserProfile%\.wslconfig`（例如 `C:\Users\<你>\.wslconfig`）寫入：

```ini
[wsl2]
memory=8GB
processors=4
```

然後在 PowerShell 執行 `wsl --shutdown`，重開 Ubuntu 生效。

> 這個上限是**整個 WSL2（包含所有 container）**能用的記憶體。第 15 章幫單一 container 設的 `--memory` 是在這個上限裡面再切一塊。

## 6. 各章在 Windows 上的差異

照上面設定好（Docker 在 WSL2、repo 在 `~/`）之後，大部分章節沒有差別。要留意的只有這些：

| 章節 | 差異 |
|------|------|
| 02 安裝 | 用這份文件的 3A 或 3B |
| 12 網路 | `--network host`：B 裝法跟 Linux 一樣；A 裝法要 Docker Desktop 4.34 以後的版本，並在 Settings 開啟 host networking |
| 13 volume、19 開發流程 | repo 放在 `~/` 就沒問題；放在 `C:\` 會慢，熱重載也可能失效 |
| 15 資源限制 | 整體上限受 `.wslconfig` 控制，OOM 示範的數字可能跟書上的範例輸出不同 |
| 23 多平台 | 兩種裝法都能 build arm64（靠 QEMU 模擬），速度很慢是正常的；B 裝法要先照第 23 章註冊 QEMU |
| 25 事故演練 | Docker 的資料存在 WSL2 的虛擬硬碟（`.vhdx`）裡，刪 image 後虛擬硬碟**不會自動縮小**（新版 WSL 可用 `wsl --manage Ubuntu --set-sparse true` 讓它自動回收）。`daemon.json`：A 裝法在 Settings → Docker Engine 改，B 裝法改 `/etc/docker/daemon.json` 後 `sudo systemctl restart docker` |

## 7. 常見問題

**`docker: command not found`（在 Ubuntu 裡）**
A 裝法：Docker Desktop 沒開，或 WSL integration 沒打開 Ubuntu。B 裝法：沒裝好，或還沒重開終端機讓 `docker` 群組生效。

**`permission denied while trying to connect to the Docker daemon socket`**
B 裝法的 `usermod -aG docker` 還沒生效。關掉所有 Ubuntu 視窗、在 PowerShell `wsl --shutdown` 後重開。

**`Cannot connect to the Docker daemon`**
B 裝法：Engine 沒啟動。`sudo systemctl status docker` 看狀態，確認 `/etc/wsl.conf` 有 `systemd=true`。

**`./run: /bin/bash^M: bad interpreter`**
檔案被轉成 CRLF 了，通常是在 Windows 那邊 clone 的。照第 4 節在 WSL2 裡重新 clone。

**公司 VPN 開著時 container 連不到外網或 DNS 失敗**
VPN 常常會改掉 WSL2 的網路或 DNS 設定，第 25 章的 DNS 事故演練有排查流程。
