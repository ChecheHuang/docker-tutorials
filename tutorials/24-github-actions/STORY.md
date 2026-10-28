# 故事版：營養午餐的中央廚房 🍱

> 這篇是**寫給小朋友也看得懂**的故事版。用一個「全市學校的營養午餐由一間自動化中央廚房做好、送出去」的故事，幫你「感覺」出 GitHub Actions workflow、觸發事件、`needs`、`GITHUB_TOKEN` 與 `permissions`、setup-qemu／setup-buildx、login、metadata-action 產 tag、build-push-action、GHA cache、Trivy 掃描、PR 只 build 不 push 是什麼。故事裡不講程式名詞，名詞都放在 `>` 小框框和最後的對照表裡。先享受故事，別急著背。

---

## 以前：王伯伯一個人顧廚房

全市的營養午餐，以前都由王伯伯一個人在廚房做。他做得很好吃，可是：

有時候忘了先試味道；有時候便當盒上的日期貼錯；有一次還用了**自己家裡的**鑰匙去開學校倉庫，結果鑰匙弄丟，整個倉庫要換鎖。

每天做出來的便當，味道也都跟王伯伯當天的心情有關。

市政府決定蓋一間**自動化中央廚房**：只要菜單（食譜）一改，或是要出新的節慶特餐，廚房就照著**牆上貼的流程表**，一步一步自動做完。每天做出來的都一模一樣。

> 這就是 **CI/CD 與 GitHub Actions**：`.github/workflows/docker.yml` 就是牆上的流程表。每次 push 或開 PR，同一套步驟自動跑，image 一定來自 repo 裡某一個 commit，不會因為「誰的電腦」而不一樣。

---

## 三種情況，三種做法

中央廚房會遇到三種情況：

1. **有人提議改菜單**：「我覺得咖哩可以少放一點辣。」廚房會**照新食譜做一份試吃**，確認做得出來、味道沒問題——但**不會送到學校**。
2. **菜單正式改了**：廚房照新食譜做，送到學校，便當盒上貼「**每日餐**」和「**食譜第 a1b2c3d 版**」。
3. **節慶特餐**：校長宣布「**第 1 季第 2 號特餐**」，廚房做好後，便當盒上貼「1.2 號」「1 季」「最新特餐」好幾張標籤。

> 這就是 **觸發事件**：`pull_request` 只 build 不 push；`push` 到 `main` 推 `:main` 和 `:sha-xxxxxxx`；push `v1.2.0` 這種 git tag 推 `:1.2.0`、`:1.2`、`:1`、`:latest`。

---

## 第一關：先檢查食材，沒過就不開火

流程表的第一步，不是開火煮飯，而是**檢查食材**：青菜新不新鮮、肉有沒有壞、份量對不對。

這一步在**旁邊的小桌子**上做就好，不用動到大鍋爐，幾分鐘就完成。只要有一樣不合格，後面的大鍋爐**根本不會點火**。

> 這就是 **`test` job** 和 **`needs: test`**：`actions/setup-go` 用 `go.mod` 指定的版本跑 `go vet` 和 `go test ./...`，不碰 Docker。測試沒過，`docker` job 就不會開始。

---

## 一張只用一天的通行證

以前王伯伯用自己的鑰匙開倉庫。現在，中央廚房**每天早上**會從市政府拿到一張**當日通行證**，晚上就自動失效。

而且通行證上寫清楚能做什麼：「可以**看**食譜」「可以**放便當進**學校倉庫」。其他的門，一扇都開不了。

但如果是「**有人提議改菜單**」的試吃情況，廚房**不拿**倉庫通行證——試吃的便當本來就不該送去學校。

> 這就是 **`GITHUB_TOKEN` 與 `permissions`**：每個 run 自動產生、結束就失效。workflow 預設 `contents: read`，只有 `docker` job 加上 `packages: write`。`docker/login-action` 用它登入 `ghcr.io`，而且加了 `if: github.event_name != 'pull_request'`——PR 不登入、也不能 push。

---

## 準備好兩種便當盒

全市的學校有兩種餐車：舊式餐車只放得下**方形**便當盒，新式餐車只放得下**圓形**的。所以每份午餐都要做兩種盒子。

廚房的機器手臂已經會**直接把飯菜裝進圓盒**，不用另外請人模仿圓盒餐車的做法。不過廚房還是請了一位**翻譯員**待命，以防哪天食譜裡有一步非得用圓盒專用工具做不可。

> 這就是 **`docker/setup-buildx-action`**（能做多平台的生產線）和 **`docker/setup-qemu-action`**（待命的翻譯員）。shorty 的 Dockerfile 是交叉編譯，`platforms: linux/amd64,linux/arm64` 一次做出兩種，QEMU 只是保險。

---

## 自動貼標籤機

以前便當盒的標籤是王伯伯手寫的，常常寫錯。

現在廚房有一台**自動貼標籤機**：它看今天是「試吃」「每日餐」還是「節慶特餐」，自動決定要貼哪幾張標籤；還會自動印一張**營養成分貼紙**：哪間廚房做的、哪一版食譜、幾點做好的。

節慶特餐是試作版（像「1.3 號試作」）的時候，機器也知道**不能**把「最新特餐」那張標籤貼上去。

> 這就是 **`docker/metadata-action`**：`type=semver`、`type=ref,event=branch`、`type=ref,event=pr`、`type=sha` 這些規則自動算出 tag；同時產生 `org.opencontainers.image.*` label（營養成分貼紙）。`steps.meta.outputs.version` 當成 `VERSION` 傳進 build，`/version` 就會回傳它。

---

## 營養師的抽檢

便當做好、還沒送出前，廚房會先拿**一盒方形的**給營養師檢查。

營養師手上有一份全國公布的「**有問題食材清單**」。只要便當裡有**嚴重**的問題食材、而且**市場上已經買得到替代品**，她就會按下紅色按鈕：「這批不准出門！」整條送貨線馬上停下來。

> 這就是 **Trivy 掃描**：先用 `build-push-action` 的 `load: true` build 一份 amd64 image 載入 runner（多平台 image 沒辦法 load），再用 `aquasecurity/trivy-action` 掃。`severity: CRITICAL,HIGH` + `exit-code: "1"` + `ignore-unfixed: true`，有已知可修的重大漏洞，後面的 push 就不會執行。

---

## 冷凍備料庫

切菜、熬高湯最花時間。中央廚房在隔壁蓋了一間**冷凍備料庫**：今天切好的菜、熬好的湯底，做完就存進去。明天只要食譜那幾步沒改，直接拿出來用，十分鐘就能開始裝盒。

而且廚房會把**所有中間步驟**的半成品都存起來，不只存最後的便當——不然明天還是要從熬湯開始。

> 這就是 **GHA cache**：`cache-from: type=gha` 和 `cache-to: type=gha,mode=max`。`mode=max` 連 build stage 的中間 layer 都存，multi-stage build 才有效。剛剛為了給營養師檢查做的那盒方形便當，也讓最後的正式製作可以直接沿用。

---

## 最後：裝箱、送出、記下指紋

全部通過之後，最後一步才是**真正的製作和送貨**：兩種盒子一起做好，貼上所有標籤，送到學校倉庫。

送出去之後，廚房會在當天的**工作日誌**上寫下這批便當的指紋號碼。以後哪間學校反應問題，對照日誌就知道是哪一批。

但如果今天只是「有人提議改菜單」的試吃，這一步只做、不送。

> 這就是 **`docker/build-push-action`**：`context: ./app`、`platforms: linux/amd64,linux/arm64`、`push: ${{ github.event_name != 'pull_request' }}`、`tags`／`labels` 來自 metadata。最後的 Summary step 把 `steps.build.outputs.digest` 寫進 run 的摘要頁。

---

## 回到現實：故事 ↔ GitHub Actions 對照表

| 故事裡的東西 | 真正的概念 |
|------|------|
| 自動化中央廚房 | **GitHub Actions** |
| 牆上的流程表 | `.github/workflows/docker.yml` |
| 提議改菜單 → 試吃不送 | `pull_request`：build，不 push |
| 菜單正式改了 | `push` 到 `main` |
| 節慶特餐 | push git tag `v1.2.0` |
| 先檢查食材 | **`test` job**（`go vet`、`go test`） |
| 沒過就不開火 | **`needs: test`** |
| 當日通行證 | **`GITHUB_TOKEN`** |
| 通行證上寫能做什麼 | **`permissions: packages: write`** |
| 試吃不拿通行證 | `if: github.event_name != 'pull_request'` |
| 方形／圓形便當盒 | `linux/amd64`／`linux/arm64` |
| 會裝兩種盒子的生產線 | **`docker/setup-buildx-action`** |
| 待命的翻譯員 | **`docker/setup-qemu-action`** |
| 自動貼標籤機 | **`docker/metadata-action`** |
| 營養成分貼紙 | OCI label |
| 營養師抽檢 | **Trivy**（`aquasecurity/trivy-action`） |
| 冷凍備料庫 | **GHA cache**（`type=gha,mode=max`） |
| 真正製作和送貨 | **`docker/build-push-action`** |
| 工作日誌上的指紋 | image digest、`$GITHUB_STEP_SUMMARY` |

---

## 一句話記住

> **CI 是一間照流程表自動做午餐的中央廚房：先檢查食材（test），拿當日通行證（GITHUB_TOKEN），自動貼標籤（metadata），營養師抽檢（Trivy），兩種便當盒一起做（多平台）——試吃只做不送，正式的才送進倉庫（GHCR）。**
