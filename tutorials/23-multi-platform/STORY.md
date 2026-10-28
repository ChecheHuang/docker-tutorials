# 故事版：兩種插座的玩具工廠 🔌

> 這篇是**寫給小朋友也看得懂**的故事版。用一個「玩具工廠要把玩具賣到兩個插座不一樣的國家」的故事，幫你「感覺」出 amd64 與 arm64、`exec format error`、manifest list、buildx builder、QEMU 模擬、交叉編譯（`$BUILDPLATFORM`／`$TARGETARCH`）、`--push` 與 `--load` 的限制，以及 Go／Rust／Node 各自的做法。故事裡不講程式名詞，名詞都放在 `>` 小框框和最後的對照表裡。先享受故事，別急著背。

---

## 插頭插不進去

小芸家開了一間**會唱歌的機器狗**工廠。機器狗在國內大受歡迎，於是她們把一整批寄到海對面的「圓孔國」。

一週後，圓孔國的小朋友打電話來哭：「機器狗的插頭是**扁的**，我們家的插座是**圓的**，插不進去！」

機器狗本身一點問題也沒有，歌也很好聽。問題只是：**插頭跟插座的形狀不合**。

> 這就是 **CPU 架構不合**：image 裡的執行檔是某一種 CPU（amd64 或 arm64）的機器碼。在 Apple Silicon Mac 上 build 的 arm64 image，丟到 x86 伺服器就會出現 `exec format error`。container 共用主機的 kernel，也共用主機的 CPU。

---

## 一個盒子，兩種插頭

小芸想到辦法：每個盒子裡放**兩隻**機器狗，一隻扁插頭、一隻圓插頭，盒子外面貼一張**目錄卡**：

「扁孔國的朋友請拿左邊那隻；圓孔國的朋友請拿右邊那隻。」

各國的店員收到盒子，看一眼目錄卡，自動把對的那隻拿出來。客人只要說「我要機器狗 3 號」，完全不用知道插頭的事。

> 這就是 **manifest list（image index）**：一個 tag 底下有一張目錄，列出 `linux/amd64`、`linux/arm64` 各自的 image。`docker pull` 會自動挑本機平台那一份。`docker buildx imagetools inspect` 可以看到這張目錄卡。

---

## 新的生產線

原本的工廠只有一條生產線，只會做扁插頭。小芸的爸爸在旁邊加蓋了一間**新式生產線**，這條線可以一次接好幾張訂單、同時做好幾種版本。

> 這就是 **buildx builder**：`docker buildx create --name multi --driver docker-container --use` 建立一個能做多平台的 BuildKit。`docker buildx ls` 看看有哪些生產線、各自會做哪些平台。

---

## 辦法一：請一位會模仿的翻譯員

圓孔國版的機器狗要怎麼做？第一個辦法很簡單：

工廠請了一位**翻譯員**，他會假裝自己是圓孔國的工人。每一個步驟——鎖螺絲、焊電線、錄歌——圓孔國的說明書寫什麼，他就**一句一句翻譯**給扁孔國的工人做。

好處是：說明書一個字都不用改。

壞處是：**超級慢**。每做一步都要先翻譯，本來一小時做完的機器狗，現在要做一整天。

> 這就是 **QEMU 模擬**：`docker run --privileged --rm tonistiigi/binfmt --install all` 讓 Linux 看得懂另一種 CPU 的程式，交給 QEMU 一條一條翻譯執行（Docker Desktop 已內建）。Dockerfile 不用改，但所有 `RUN` 都在模擬器裡跑，編譯會慢 5–20 倍。本章的 `Dockerfile.qemu` 就是這種做法。

---

## 辦法二：用自己的工具，直接做出另一種插頭

小芸的哥哥阿哲是工廠最厲害的師傅。他說：「何必翻譯？我用**我們自己的機器**，只要換一個模具，就能直接壓出圓插頭。」

他在工作檯上貼兩張紙：

- 「**我在哪裡做**」：扁孔國工廠
- 「**要做給誰**」：圓孔國

阿哲用的是自己最熟的工具、最快的速度，只是最後一步換了模具。做好的圓插頭，再裝進**圓孔國的外殼**裡。整個過程跟做扁插頭一樣快。

> 這就是 **交叉編譯**：`FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build` 讓編譯器跑在 build 機器的原生架構（「我在哪裡做」），`ARG TARGETARCH` 告訴它要輸出哪個架構（「要做給誰」），然後 `GOOS=$TARGETOS GOARCH=$TARGETARCH go build`。runtime stage 沒寫 `--platform`，所以裝進的是目標平台的 distroless（圓孔國的外殼）。

---

## 三位師傅，三種難度

工廠裡還有兩位師傅，大家換模具的難度不一樣：

- **阿哲（做金屬狗）**：只要換一個模具，按個鈕就好。最輕鬆。
- **小鐵（做精密機械狗）**：除了換模具，還得去買**圓孔國專用的螺絲和焊料**，裝好才能開工。準備比較麻煩，但做起來還是很快。
- **小諾（做布偶狗）**：布偶狗本身根本沒有插頭！只有肚子裡的**小音箱**才有插頭。如果音箱是現成的、哪國都能用，那就直接縫進去；如果音箱要現場焊，那就得請翻譯員到圓孔國的規格下慢慢焊。

> 這就是 **Go／Rust／Node 的差別**：Go（`CGO_ENABLED=0`）只要設 `GOARCH`；Rust 要 `rustup target add` 加上目標架構的 linker 與 libc（本章 `Dockerfile.rust` 用 Debian 的 cross gcc，也可以用 cargo-zigbuild）；Node 是直譯語言，JS 檔不分架構，但 native module（bcrypt、sharp）要在目標平台安裝，所以 `Dockerfile.node` 只在依賴都是純 JS 時才在原生架構跑 `npm ci`。

---

## 倉庫放得下，家裡的櫃子放不下

雙插頭盒子做好了，要放去哪裡？

送到**大倉庫**完全沒問題：大倉庫有專門放「一盒兩隻」的架子，目錄卡也會一起保存。

可是小芸想先搬一盒回**家裡的舊櫃子**看看——舊櫃子每一格只能放一隻狗，放不下「一盒兩隻」。她只能挑一隻帶回家，或者換一個**新款的櫃子**，新櫃子就有放目錄盒的格子。

> 這就是 **`--push` vs `--load`**：多平台 image `--push` 到 registry 沒問題；`--load` 進本機的 classic image store 會報 `docker exporter does not currently support exporting manifest lists`，只能單一平台 `--load`。換成 **containerd image store**（新款櫃子）就可以載入多平台。

---

## 回到現實：故事 ↔ Docker 對照表

| 故事裡的東西 | 真正的 Docker 概念 |
|------|------|
| 扁插頭／圓插頭 | **amd64／arm64** |
| 插不進去 | `exec format error` |
| 一盒兩隻 + 目錄卡 | **manifest list（image index）** |
| 店員看目錄卡自動挑 | `docker pull` 依平台選 manifest |
| 新式生產線 | **buildx builder**（`docker-container` driver） |
| 一句一句翻譯的翻譯員 | **QEMU 模擬**（`tonistiigi/binfmt`） |
| 用自己的工具換模具 | **交叉編譯** |
| 「我在哪裡做」 | `$BUILDPLATFORM` |
| 「要做給誰」 | `$TARGETPLATFORM`／`$TARGETOS`／`$TARGETARCH` |
| 圓孔國的外殼 | runtime stage 的目標平台 base image |
| 阿哲（金屬狗） | Go：`GOARCH=arm64` |
| 小鐵（精密機械狗） | Rust：target triple + cross linker |
| 小諾（布偶狗）和小音箱 | Node：JS 不分架構，native module 要注意 |
| 大倉庫 | registry（`--push`） |
| 每格只放一隻的舊櫃子 | classic image store（`--load` 只能單一平台） |
| 新款櫃子 | containerd image store |

---

## 一句話記住

> **同一隻機器狗要賣到兩種插座的國家：把兩種插頭裝進同一盒、貼上目錄卡（manifest list）；做圓插頭時別請翻譯員慢慢翻（QEMU），用自己的工具直接換模具（交叉編譯）。**
