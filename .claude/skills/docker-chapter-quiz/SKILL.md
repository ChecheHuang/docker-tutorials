---
name: docker-chapter-quiz
description: Docker 教學章節互動測驗工具。針對 docker-tutorials 專案的 25 章，透過問答形式測試使用者對特定章節的掌握程度，答錯時立即講解直到完全理解。使用時機：使用者說「測驗」、「quiz」、「考考我」、「我想測試第 X 章」、「幫我出題」，或任何要測試 Docker 教學章節知識的場景。
---

# Docker Chapter Quiz

針對 `tutorials/` 下的 25 章 Docker 課程進行互動式知識測驗。

## 工具準備

使用前先載入 AskUserQuestion：
```
ToolSearch: select:AskUserQuestion
```

## 測驗流程

### 第一步：選擇模式

```
AskUserQuestion({
  questions: [
    {
      question: "請選擇測驗模式：",
      header: "測驗模式",
      multiSelect: false,
      options: [
        { label: "學習模式（推薦）", description: "答錯立即講解，確認理解後才繼續下一題，共 5 題" },
        { label: "考試模式", description: "全部 5 題做完後再統一講解錯誤，模擬實際考試" },
        { label: "快速測驗", description: "只出 3 題，快速確認核心概念" }
      ]
    }
  ]
})
```

### 第二步：選擇章節

先以 Part 縮小範圍（使用者也可直接透過 Other 輸入章節編號如「08」跳過此步）。AskUserQuestion 每題最多 4 個選項，所以 Part 分兩頁問：

```
AskUserQuestion({
  questions: [
    {
      question: "要測驗哪個 Part？也可直接輸入章節編號（如「08」）：",
      header: "選擇 Part",
      multiSelect: false,
      options: [
        { label: "Part 1 基礎概念 (01–05)", description: "container vs VM、安裝、image／container／registry、生命週期、exec／logs／inspect" },
        { label: "Part 2 寫 Dockerfile (06–11)", description: "基本指令、layer 快取、multi-stage、base image、build context／ARG、BuildKit" },
        { label: "Part 3 執行期 (12–16)", description: "網路、volume、設定與機密、資源限制與 restart、healthcheck 與 graceful shutdown" },
        { label: "Part 4–5 Compose 與上線 (17–25)", description: "compose 基礎／多服務／開發流程／除錯、image 安全、tag 與 registry、多平台、GitHub Actions、事故演練" }
      ]
    }
  ]
})
```

選了「Part 4–5」時，再問一次要 Part 4（17–20）還是 Part 5（21–25）。

選定 Part 後，再顯示該 Part 下的具體章節（每次最多 4 個，剩餘讓使用者用 Other 輸入編號）：

```
AskUserQuestion({
  questions: [
    {
      question: "寫 Dockerfile 系列，請選擇章節（或 Other 輸入其他編號）：",
      header: "選擇章節",
      multiSelect: false,
      options: [
        { label: "06 Dockerfile 基本指令", description: "FROM、WORKDIR、COPY、RUN、CMD、EXPOSE" },
        { label: "07 layer 與快取", description: "指令順序、快取失效規則、docker history" },
        { label: "08 multi-stage", description: "build stage vs runtime stage、Go／Rust／Node 對照" },
        { label: "09–11 其他章節", description: "請用 Other 輸入章節編號，如「09」" }
      ]
    }
  ]
})
```

詳細章節目錄與核心考點見 `references/chapters.md`。

### 第三步：讀取章節內容

選定章節（如 08）後讀取：
```
tutorials/08-multi-stage/README.md
tutorials/08-multi-stage/Dockerfile        （章節目錄裡的 Dockerfile／compose.yaml／腳本一併讀取）
```

路徑格式：`tutorials/{XX-目錄名稱}/README.md`，目錄名稱對照見 `references/chapters.md`。
涉及 shorty 的題目，可參考 `app/`、`app/Dockerfile`（最終版）與根目錄 `compose.yaml`（最終版）。

### 第四步：設計並出題

根據模式決定題數：學習 / 考試模式 5 題，快速測驗 3 題。

難度排序：概念題（第 1–2 題）→ 應用題（第 3–4 題）→ 陷阱或事故排查題（第 5 題）

題型可以混用：
- **概念題**：例如「image 和 container 的關係」「ARG 和 ENV 的差別」
- **Dockerfile／compose 閱讀題**：給一段檔案，問 image 大小、快取會不會失效、哪裡有安全問題
- **指令題**：給一個情境，問該下哪個指令或 flag
- **排查題**：給症狀（例如 exit code 137、`connection refused`、DB 還沒好），問最可能的原因

**一般選擇題**（options 必須是 `{ label, description }` 物件）：

```
AskUserQuestion({
  questions: [
    {
      question: "【第 1 題 / 共 5 題】

關於 image 和 container，以下哪個描述正確？",
      header: "第 1 題",
      multiSelect: false,
      options: [
        { label: "A", description: "image 是唯讀模板，container 是在 image 上加一層可寫層的執行實例" },
        { label: "B", description: "container 停止後，裡面寫入的檔案會自動存回 image" },
        { label: "C", description: "一個 image 同一時間只能跑一個 container" },
        { label: "D", description: "刪掉 container 會連帶刪掉 image" }
      ]
    }
  ]
})
```

**Dockerfile 閱讀題**（使用 `preview` 讓使用者邊看邊作答，只有單選題支援 preview）：

```
AskUserQuestion({
  questions: [
    {
      question: "【第 3 題 / 共 5 題】

只改了 main.go 一行，重新 build 時哪一步開始會重跑？",
      header: "第 3 題",
      multiSelect: false,
      options: [
        {
          label: "A",
          description: "從 COPY . . 開始重跑，go mod download 那層吃快取",
          preview: "FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /out/shorty ."
        },
        { label: "B", description: "從 FROM 開始全部重跑" },
        { label: "C", description: "從 RUN go mod download 開始重跑" },
        { label: "D", description: "完全不用重跑，全部吃快取" }
      ]
    }
  ]
})
```

### 第五步：判斷答案與回饋

**答對**：
- 讚美 + 1–2 句說明為何正確
- 學習模式：立即繼續下一題；考試模式：記錄，全部結束後統整

**答錯（學習模式）**：
- 說明答錯，給出正確答案與詳細解釋（含 Dockerfile／指令範例）
- 出等效題確認理解：
  ```
  AskUserQuestion({
    questions: [
      {
        question: "確認理解：{同概念換角度的題目}",
        header: "再確認",
        multiSelect: false,
        options: [{ label: "A", description: "..." }, ...]
      }
    ]
  })
  ```
- 再次答錯時：再解釋一次後直接繼續下一題（避免無限迴圈）

**答錯（考試模式）**：記錄錯誤，不立即說明，全部結束後統整講解。

### 第六步：章節總結

```
AskUserQuestion({
  questions: [
    {
      question: "本章測驗完成！\n得分：{X}/{Y}（學習模式以第一次答對計）\n\n{答錯概念列表（如有）}\n\n要繼續嗎？",
      header: "繼續測驗",
      multiSelect: false,
      options: [
        { label: "繼續選其他章節", description: "回到章節選擇" },
        { label: "重測本章", description: "換一批題目再測一次" },
        { label: "結束", description: "離開測驗" }
      ]
    }
  ]
})
```

考試模式在此處逐一講解錯題後再顯示上述選項。

## 題目設計原則

- 優先測該章節**最核心概念**，非邊緣案例
- 4 個選項的干擾選項要選**常見誤解**，提升鑑別度
- 題目貼近 `tutorials/` 與 shorty 實際的 Dockerfile、compose 與指令
- 有程式碼展示時，使用 `preview` 欄位讓使用者邊看邊選
