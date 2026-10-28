# 故事版：小偵探與沒開門的點心舖 🔍

> 這篇是**寫給小朋友也看得懂**的故事版。用一個「小偵探調查點心舖為什麼沒開門」的故事，幫你「感覺」出 `docker compose config`、`ps`、`logs`、`events`、`exec`、`run --rm`、帶工具箱的 netshoot，以及 port 被佔、舊 volume 留著舊密碼、Windows CRLF 這些常見錯誤是什麼。故事裡不講程式名詞，名詞都放在 `>` 小框框和最後的對照表裡。先享受故事，別急著背。

---

## 點心舖今天沒開

商店街有一家點心舖，平常早上八點準時開門。店裡有三個人：**櫃台的阿福**、**廚房的蒸籠婆婆**、**冰櫃的小冰**。

今天已經九點了，鐵門還是關著。老闆急得團團轉：「我明明把工作分配表寫得好好的啊！」

商店街的小偵探米米被請來調查。她背著小包包，第一句話就說：

「**不要猜。我們一個一個去看。**」

> 這就是**除錯的原則**：不要猜，去看。Compose 的除錯有固定順序：`config` → `ps` → `logs` → `events` → `exec`／`run`。

---

## 第一條線索：真正貼在牆上的那張表

老闆說：「分配表我記得很清楚，阿福八點開門、婆婆七點開始蒸包子……」

米米搖搖頭：「我不要聽你**記得**什麼。我要看**真正貼在牆上**的那張。」

她走到後門，看到牆上的分配表，上面還疊了一張便條紙。兩張疊在一起看，寫的是：

> 阿福：八點開門。暗號：＿＿＿（空白）
> 婆婆：帳本放在「帳本抽屜」。

米米指著空白的地方：「暗號沒填。」再指著另一行：「這家店**根本沒有叫『帳本抽屜』的抽屜**。」

老闆愣住了：「我以為……我以為有寫……」

> 這就是 **`docker compose config`**：它印出 Compose **真正讀到的**設定——檔案合併後、`${...}` 替換後的結果。沒設的變數會出現 `variable is not set` 警告，用了卻沒宣告的 volume 會直接報錯 `refers to undefined volume`。改完檔案先跑 `config`，是最划算的習慣。

---

## 第二條線索：點名板

米米走進店裡，看了看點名板：

| 誰 | 現在怎樣 |
|------|------|
| 櫃台阿福 | **已經回家了**（留下紙條：「1」） |
| 蒸籠婆婆 | 在廚房，綠燈亮 |
| 冰櫃小冰 | 在冰櫃前，燈**沒有顏色** |

「阿福回家了，紙條上寫『1』，代表他是**自己決定**離開的，不是被趕走的。」米米說。

> 這就是 **`docker compose ps -a`**：`-a` 連已經結束的 container 也列出來。exit code `1` 是程式自己報錯結束，`137` 是被強制 kill（OOM 或 stop 超時）。`HEALTH` 欄可以看到誰是 healthy、誰根本沒有 healthcheck。

---

## 第三條線索：阿福的日記

阿福有寫日記的習慣，每件事都會記下來。他雖然回家了，但日記本**還留在櫃台上**。

米米翻開來：

> 7:50 我要去廚房找婆婆拿包子。
> 7:50 我喊「**我自己**！包子好了沒？」……沒人回答。
> 7:51 再喊一次「我自己！」……還是沒人回答。
> ……（重複了十次）
> 8:00 算了，我回家了。

米米嘆了一口氣：「他在喊**自己的名字**，當然沒人回答。要喊『**蒸籠婆婆**』啊。」

> 這就是 **`docker compose logs app`**：container 結束了，log 還查得到（只要還沒 `down`）。shorty 的 log 顯示重試 10 次連 `127.0.0.1:5432` 都 `connection refused`——`DATABASE_URL` 寫成 `localhost`，在 container 裡指的是自己。要寫 service 名稱 `db`。

### 為什麼不能先「收店」？

老闆說：「那我先把店收一收，明天重來？」

米米趕緊攔住他：「不行！收店會把阿福的日記一起丟掉，**證據就沒了**。先看完再收。」

> 這就是**先蒐證再 down**：`docker compose down` 會刪除 container 和它的 log。除錯時先 `ps -a`、`logs`，再 down。

---

## 第四條線索：監視器

米米還想知道：「事情是**照什麼順序**發生的？」

商店街的監視器記錄了每一個人進門、出門的時間：

> 7:45 蒸籠婆婆 進門
> 7:45 冰櫃小冰 進門
> 7:46 蒸籠婆婆 綠燈亮
> 7:50 櫃台阿福 進門
> 8:00 櫃台阿福 離開（紙條：1）

> 這就是 **`docker compose events`**：即時顯示 create、start、die、health_status 等事件。適合抓一閃而過、一直重啟的問題。

---

## 走進廚房親口問

米米走進廚房，直接問蒸籠婆婆：「婆婆，妳的包子好了嗎？」

「好了啊，一直都好了！」

米米又想問冰櫃小冰，可是她想：「小冰正在忙，我不要打擾她。」於是她**請了一位替身**，穿上跟小冰一樣的制服、拿一樣的工具，去冰櫃前面試試看冰櫃能不能打開。替身試完就走了，什麼也沒留下。

> 這就是 **`exec`** 和 **`run --rm`**：`docker compose exec db psql ...` 在**正在跑**的 container 裡執行指令；`docker compose run --rm cache redis-cli -h cache ping` 用同一個 service 的設定**另外**起一個一次性 container，跑完就刪，而且預設不搶 port。

### 櫃台裡什麼工具都沒有

米米想坐到阿福的櫃台，從櫃台的角度看看能不能叫到廚房。可是櫃台**超級簡單**：只有一張椅子，沒有電話、沒有對講機，連一支筆都沒有。

「沒關係，」米米打開她的小包包，「**我自己帶了工具箱**。」她拿出望遠鏡、對講機、一本電話簿，坐在櫃台旁邊，一個一個試：叫得到廚房嗎？叫得到冰櫃嗎？

> 這就是**除錯 distroless**：shorty 的 image 沒有 shell，`docker compose exec app sh` 會失敗。帶一個工具箱 container 接到同一個網路：`docker run --rm -it --network <project>_default nicolaka/netshoot`，裡面有 curl、dig、nc。

---

## 其他偵探筆記

米米辦過很多案子，她的筆記本裡還記著這些：

**停車位被佔了**：有一次點心舖的貨車停不進店門口，因為昨天**隔壁店的車沒開走**。

> `port is already allocated`：host 的 port 被別的 container（多半是上一章的 project 沒 down）佔走了。

**換了鑰匙，鎖沒換**：老闆換了一把新鑰匙，可是鎖還是第一天裝的那個舊鎖，新鑰匙打不開。

> 舊 volume 留著舊密碼：`POSTGRES_PASSWORD` 只在第一次初始化有效，改了 `.env` 會得到 `password authentication failed`。要嘛 `down -v` 重來，要嘛 `ALTER USER` 換鎖。

**看不見的怪字**：有一張從外地寄來的工作單，上面寫「去**廚房**」，可是阿福怎麼找都找不到廚房。米米用放大鏡一看，「廚房」後面多了一個**看不見的小符號**，所以阿福在找一個叫「廚房＋小符號」的地方。

> Windows **CRLF**：腳本第一行變成 `#!/bin/sh\r`，系統找不到 `/bin/sh\r`，錯誤訊息是讓人一頭霧水的 `no such file or directory`。用 `.gitattributes` 的 `*.sh text eol=lf` 解決。

**忘了換新菜單**：老闆印了新菜單，但沒說要換，店員就繼續用架上的舊菜單。

> 這就是**忘了 `--build`**：改了程式碼卻沒 `up --build`，跑的是舊 image。

---

## 米米的挑戰信

案子辦完，米米留了一封信給老闆的小孩：

「我把一張**故意寫錯六個地方**的工作分配表放在信封裡。你照我的方法——先看牆上的表、再看點名板、再翻日記、最後親自去試——把六個錯都找出來吧！」

> 這就是 **`broken.compose.yaml`**：藏了 6 個 bug，有些 `config` 就抓得到，有些要 `up` 之後看 log 才會現形。答案在 README 的「練習解答」。

---

## 回到現實：故事 ↔ Docker 對照表

| 故事裡的東西 | 真正的 Docker 概念 |
|------|------|
| 「不要猜，去看」 | 除錯原則：`config` → `ps` → `logs` → `events` → `exec`／`run` |
| 真正貼在牆上的表（疊了便條紙） | **`docker compose config`**（合併 + 替換後的結果） |
| 暗號空白 | `variable is not set. Defaulting to a blank string.` |
| 沒有那個抽屜 | `refers to undefined volume` |
| 點名板 | **`docker compose ps -a`** |
| 紙條上的「1」 | **exit code 1**（程式自己結束） |
| 燈沒有顏色 | 沒有 healthcheck |
| 留在櫃台上的日記 | **`docker compose logs`**（container 結束仍可查） |
| 喊「我自己」 | `DATABASE_URL` 寫成 **`localhost`** |
| 收店會把日記丟掉 | **`down`** 會刪掉 container 和 log |
| 監視器紀錄 | **`docker compose events`** |
| 走進廚房親口問 | **`docker compose exec`** |
| 穿同樣制服的替身，試完就走 | **`docker compose run --rm`** |
| 什麼都沒有的櫃台 | **distroless**（沒有 shell） |
| 米米自己的工具箱 | **netshoot** 工具 container |
| 隔壁的車沒開走 | **`port is already allocated`** |
| 新鑰匙、舊鎖 | 舊 volume 留著**舊密碼** |
| 看不見的小符號 | Windows **CRLF**（`\r`） |
| 架上的舊菜單 | 忘了 **`--build`** |
| 寫錯六個地方的分配表 | **`broken.compose.yaml`** |

---

## 一句話記住

> **出事了不要猜：先看牆上真正的表（config），再看點名板（ps），翻日記（logs），查監視器（events），最後親自走進去問（exec），櫃台沒工具就自己帶工具箱（netshoot）。**
