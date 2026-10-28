# 04. 常用指令與 container 生命週期

> 範圍：`run`／`create`／`start`／`stop`／`kill`／`rm`、container 狀態機、`--rm`、`-it`、`-e`、`--name`、`ps -a`、`system prune`

> 🧸 想先用故事直覺理解這章？看 [STORY.md](./STORY.md)（小學生都懂的比喻版）。

## 為什麼需要這個

到目前為止你只用過 `docker run`、`docker stop`、`docker rm`，照著打能動，但很多事情會讓你困惑：

- 為什麼 `docker ps` 看不到我剛剛跑的 container？
- `stop` 和 `kill` 差在哪？為什麼有時候 `stop` 要等 10 秒？
- 為什麼 `--name web` 第二次就報錯？
- 為什麼硬碟越來越滿？

這些答案都在 **container 的生命週期** 裡。搞懂狀態機，每個指令在做什麼就清楚了。

## 核心概念

### 1. Container 狀態機

```
                    docker create
     （不存在） ─────────────────────▶  created
         ▲                                │
         │                                │ docker start
         │                                ▼
         │        docker pause   ┌───────────────────┐
         │      ┌──────────────── │                   │
         │      ▼                 │      running      │◀──────────┐
         │   paused ────────────▶ │                   │           │
         │        docker unpause  └───────────────────┘           │
         │                                │                       │ docker start
         │                                │ 主 process 結束        │ docker restart
         │                                │ docker stop / kill    │
         │                                ▼                       │
         │         docker rm         ┌──────────┐                 │
         └───────────────────────────│  exited  │─────────────────┘
                                     └──────────┘
```

另外還有 `restarting`（restart policy 正在重啟它，第 15 章）、`removing`、`dead`（刪除失敗的殘骸，很少見）。

關鍵觀念：

- **container 的壽命 = 主 process（PID 1）的壽命。** 主 process 結束，container 就進入 `exited`。
- **`exited` 不是消失。** 停下來的 container 還佔著磁碟（可寫層還在），可以再 `start`，也可以 `docker logs` 看它死前說了什麼。要 `rm` 才真的刪除。
- **`docker run` = `create` + `start`**（沒有 `-d` 的話還會 attach 到它的輸出）。

### 2. `stop` vs `kill`

```
docker stop web                         docker kill web
     │                                       │
     ▼                                       ▼
送 SIGTERM（或 image 指定的 STOPSIGNAL）    直接送 SIGKILL
     │                                       │
     ├─ process 自己收尾、結束 ──▶ exited    └──▶ 立刻被 kernel 終止 ──▶ exited (137)
     │
     └─ 10 秒內沒結束 ──▶ 補一發 SIGKILL ──▶ exited (137)
```

| exit code | 常見意思 |
|-----------|---------|
| `0` | 正常結束 |
| `1`、`2`… | 程式自己回報錯誤 |
| `125` | `docker run` 本身失敗（參數錯、image 有問題） |
| `126`／`127` | 指令不能執行／找不到指令 |
| `137` | 128 + 9，被 SIGKILL（`kill`、`stop` 逾時、或 OOM，第 15 章） |
| `143` | 128 + 15，收到 SIGTERM 後結束 |

shorty 的 `main.go` 會攔 SIGTERM、在 8 秒內 graceful shutdown，就是為了趕在 `docker stop` 的 10 秒期限前收完尾。PID 1 與訊號的細節在第 16 章。

### 3. `docker run` 常用旗標

| 旗標 | 作用 |
|------|------|
| `-d` | 背景執行（detached） |
| `--name web` | 給 container 取名，之後指令都能用名字代替 ID；**名字不能重複** |
| `--rm` | container 結束後自動刪除，適合一次性的指令 |
| `-i` | 保持 stdin 開著（interactive），讓你能輸入 |
| `-t` | 配一個假終端機（TTY），才有提示字元、顏色、`Ctrl+C` |
| `-it` | 兩個合起來，互動式 shell 必備 |
| `-e KEY=value` | 設定環境變數；`-e KEY` 不給值，會從主機的同名變數帶進去 |
| `-p 8080:80` | port 對應（第 02 章，細節在第 12 章） |

指令格式：

```
docker run [旗標...] IMAGE [指令 參數...]
           └─ 旗標一定放在 image 前面 ─┘  └─ image 後面的東西會傳給 container ─┘
```

`docker run alpine --rm echo hi` 是錯的：`--rm` 被當成 container 要執行的指令了。

### 4. 新舊指令名稱

Docker 後來把指令整理成「管理指令」，舊的短指令都還能用：

| 短指令 | 管理指令 |
|--------|---------|
| `docker ps` | `docker container ls` |
| `docker rm` | `docker container rm` |
| `docker images` | `docker image ls` |
| `docker rmi` | `docker image rm` |

## 動手做

所有指令從 repo 根目錄執行。

### 步驟 1：`create` 和 `start` 分開做

```bash
docker create --name web -p 8080:80 nginx:1.27-alpine
docker ps
docker ps -a
```

範例輸出：

```
8c1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e
CONTAINER ID   IMAGE     COMMAND   CREATED   STATUS    PORTS     NAMES

CONTAINER ID   IMAGE               COMMAND                  CREATED         STATUS    PORTS     NAMES
8c1e2f3a4b5c   nginx:1.27-alpine   "/docker-entrypoint.…"   3 seconds ago   Created             web
```

`docker ps` 只列**執行中**的；`-a`（all）連停止、剛建立的都列。`Created` 狀態下 process 還沒啟動，port 也還沒綁。

```bash
docker start web
docker ps
```

範例輸出：

```
web
CONTAINER ID   IMAGE               COMMAND                  CREATED          STATUS         PORTS                                     NAMES
8c1e2f3a4b5c   nginx:1.27-alpine   "/docker-entrypoint.…"   20 seconds ago   Up 2 seconds   0.0.0.0:8080->80/tcp, [::]:8080->80/tcp   web
```

### 步驟 2：`stop`，再 `start` 回來

```bash
docker stop web
docker ps -a --filter name=web
```

範例輸出：

```
web
CONTAINER ID   IMAGE               COMMAND                  CREATED          STATUS                     PORTS     NAMES
8c1e2f3a4b5c   nginx:1.27-alpine   "/docker-entrypoint.…"   1 minute ago     Exited (0) 2 seconds ago             web
```

`Exited (0)`：nginx 收到停止訊號後乾淨地結束。container 還在，可以再啟動，而且是**同一個** container（ID 一樣、可寫層裡的變更還在）：

```bash
docker start web
```

### 步驟 3：`kill`

```bash
docker kill web
docker ps -a --filter name=web
```

範例輸出：

```
web
CONTAINER ID   IMAGE               ...   STATUS                       PORTS     NAMES
8c1e2f3a4b5c   nginx:1.27-alpine   ...   Exited (137) 1 second ago              web
```

`137` = 128 + 9（SIGKILL）。process 沒有機會收尾，正在處理的請求會直接斷掉。

### 步驟 4：為什麼 `stop` 有時候要等 10 秒

```bash
docker run -d --name sleeper alpine sleep 300
time docker stop sleeper
```

範例輸出：

```
sleeper

real    0m10.4s
```

`sleep` 是 PID 1，又沒有處理 SIGTERM，所以 Docker 等滿 10 秒才補 SIGKILL。`docker ps -a` 會看到它是 `Exited (137)`。這就是為什麼你的服務**一定要處理 SIGTERM**——第 16 章會用 shorty 示範正確做法，以及 `--init`。

> **PowerShell**：用 `Measure-Command { docker stop sleeper }`。

### 步驟 5：`rm`

```bash
docker rm sleeper
docker start web
docker rm web
```

範例輸出：

```
sleeper
web
Error response from daemon: cannot remove container "web": container is running: stop the container before removing or force remove
```

執行中的 container 不能直接刪。兩個做法：

```bash
docker stop web && docker rm web   # 好好收尾再刪
docker rm -f web                   # 強制：等於 kill + rm
```

> **PowerShell**：PowerShell 7 支援 `&&`；Windows PowerShell 5.1 不支援，請分兩行打。

### 步驟 6：`--rm` 一次性 container

```bash
docker run --rm alpine echo bye
docker ps -a --filter ancestor=alpine
```

`echo` 跑完 container 就自動刪了，`ps -a` 找不到它。不加 `--rm` 的話，每跑一次就留下一個 `Exited` 的 container 佔空間。

### 步驟 7：`-it` 互動式 shell

```bash
docker run -it --rm alpine sh
```

範例畫面：

```
/ # cat /etc/alpine-release
3.22.2
/ # whoami
root
/ # exit
```

`exit` 結束了 `sh`（主 process），container 就結束，又因為 `--rm` 被刪掉。

試試少一個旗標會怎樣：

| 指令 | 結果 |
|------|------|
| `docker run --rm alpine sh` | 沒有 stdin，`sh` 讀到 EOF 立刻結束，什麼都沒發生 |
| `docker run -i --rm alpine sh` | 能輸入指令、有輸出，但沒有 `/ #` 提示字元 |
| `docker run -it --rm alpine sh` | 完整的互動 shell |

**離開但不停止**：在 `-it` 的 container 裡按 `Ctrl+P` 再按 `Ctrl+Q`，可以 detach，container 繼續跑；之後用 `docker attach <名字>` 接回去。（VS Code 終端機會攔 `Ctrl+P`，請在一般終端機試。）要「另外開一個 shell 進去」而不是接回主 process，用第 05 章的 `docker exec`。

> **Git Bash**：如果看到 `the input device is not a TTY`，在指令前加 `winpty`（`winpty docker run -it --rm alpine sh`），或改用 PowerShell。

### 步驟 8：`-e` 環境變數，以及引號陷阱

```bash
docker run --rm -e GREETING=hello alpine sh -c 'echo $GREETING from $HOSTNAME'
```

範例輸出：

```
hello from 4d5e6f7a8b9c
```

`$GREETING` 要在 **container 裡**展開，所以用**單引號**包起來，不讓主機的 shell 先動手。改成雙引號試試：

```bash
docker run --rm -e GREETING=hello alpine sh -c "echo $GREETING from $HOSTNAME"
```

Git Bash 會先把 `$GREETING` 換成主機上的值（空的），`$HOSTNAME` 換成**你電腦**的名字。**PowerShell 也一樣**：雙引號裡的 `$GREETING` 會被當成 PowerShell 變數展開。結論：**要傳給 container 的 `$` 一律用單引號**，這條在 Git Bash 和 PowerShell 都成立。

一個真實的例子——postgres 沒給密碼就不肯啟動：

```bash
docker run -d --name pg postgres:17-alpine
docker ps -a --filter name=pg
docker logs pg
```

範例輸出：

```
CONTAINER ID   IMAGE                STATUS                     NAMES
2b3c4d5e6f7a   postgres:17-alpine   Exited (1) 2 seconds ago   pg

Error: Database is uninitialized and superuser password is not specified.
       You must specify POSTGRES_PASSWORD to a non-empty value for the
       superuser. For example, "-e POSTGRES_PASSWORD=password" on "docker run".
...
```

主 process 一啟動就以 exit code 1 結束，container 也就 `Exited (1)`。（`docker logs` 是第 05 章的主角，這裡先借用。）補上環境變數：

```bash
docker rm pg
docker run -d --name pg -e POSTGRES_PASSWORD=secret postgres:17-alpine
docker ps --filter name=pg
```

這次 STATUS 是 `Up ...`。（把密碼直接寫在指令上不是好習慣，第 14 章會講 `--env-file` 和 secrets。）

### 步驟 9：`--name` 不能重複

```bash
docker run -d --name pg -e POSTGRES_PASSWORD=secret postgres:17-alpine
```

範例輸出：

```
docker: Error response from daemon: Conflict. The container name "/pg" is already in use by container "2b3c4d5e6f7a...". You have to remove (or rename) that container to be able to reuse that name.
```

就算舊的 `pg` 已經 `Exited`，名字也還被它佔著。`docker rm -f pg` 之後才能重用。

### 步驟 10：大掃除

先看看累積了多少東西：

```bash
docker ps -a
```

清掉所有已停止的 container：

```bash
docker container prune
```

一次清更多：

```bash
docker system prune
```

範例輸出：

```
WARNING! This will remove:
  - all stopped containers
  - all networks not used by at least one container
  - all dangling images
  - unused build cache

Are you sure you want to continue? [y/N] y
Deleted Containers:
2b3c4d5e6f7a...

Total reclaimed space: 12.3MB
```

| 指令 | 額外刪除 |
|------|---------|
| `docker system prune` | 停止的 container、沒用到的 network、dangling image、build cache |
| `docker system prune -a` | 再加上**所有沒被 container 使用的 image**（下次要重新 pull） |
| `docker system prune --volumes` | 再加上沒被使用的 **volume**——資料庫資料可能就在裡面，小心（第 13 章） |

看磁碟到底被誰吃掉，用 `docker system df`（第 25 章的磁碟演練會細講）。

收拾本章：

```bash
docker rm -f pg
```

## 對照 Node／TypeScript

| Node 世界 | Docker 世界 |
|-----------|------------|
| `pm2 start app.js --name api` | `docker run -d --name api ...` |
| `pm2 stop api` / `pm2 restart api` | `docker stop api` / `docker restart api` |
| `pm2 delete api` | `docker rm api` |
| `pm2 list` | `docker ps -a` |
| `npx some-cli`（跑完就走，不留痕跡） | `docker run --rm ...` |
| `node`（進入 REPL） | `docker run -it --rm node:22-alpine node` |
| `process.env.GREETING` | `-e GREETING=hello` |
| `process.on('SIGTERM', ...)` 收尾 | `docker stop` 送 SIGTERM，10 秒內要結束 |
| `kill -9 <pid>` | `docker kill` |
| `process.exit(1)` | `Exited (1)` |

Node 開發者特別要注意：用 `npm start` 當 container 的主 process 時，`npm` 不一定會把 SIGTERM 轉給你的 `node`，結果每次 `docker stop` 都要等 10 秒被強殺。第 16 章會講為什麼該直接跑 `node server.js`。

## 常見陷阱

1. **`docker ps` 看不到 container**：它已經結束了。用 `docker ps -a`，看 STATUS 的 exit code。
2. **旗標放在 image 後面**：`docker run nginx -d` 的 `-d` 會被傳給 nginx 當參數。旗標一律放在 image 名稱前面。
3. **一直 `docker run` 卻不加 `--rm`**：每次都留下一個 exited container，`docker ps -a` 一長串，名字還會撞。
4. **把 `stop` 當成刪除**：stop 後 container 還在，port 雖然釋放了，名字和磁碟還佔著。
5. **以為重新 `docker run` 會保留上次的變更**：`run` 永遠建立**新的** container。要接著用同一個，是 `docker start`。
6. **雙引號裡的 `$`**：Git Bash 和 PowerShell 都會在主機端先展開，傳進 container 的是空字串或你電腦的值。用單引號。
7. **PowerShell 的換行**：bash 用 `\` 斷行，PowerShell 用反引號 `` ` ``。從文件複製多行指令到 PowerShell 前要改。
8. **`system prune -a --volumes` 隨手亂打**：會刪掉本機所有沒在用的 image 和 volume，包括你的開發資料庫。

## 練習

1. 用 `docker create` + `docker start` 跑一個 `redis:7-alpine`，名字叫 `cache`，然後用 `docker ps -a` 觀察每一步的 STATUS。
2. 分別用 `docker stop` 和 `docker kill` 停掉 `cache`，比較 exit code 與花費時間。
3. 跑 `docker run --rm -e NAME=docker alpine sh -c 'echo hi $NAME'`，再把單引號改成雙引號跑一次，分別在 Git Bash 和 PowerShell 試，解釋輸出差異。
4. 跑 `docker run -it --name box alpine sh`，用 `Ctrl+P` `Ctrl+Q` detach，再 `docker attach box` 接回去，最後 `exit`。`box` 現在是什麼狀態？
5. 故意跑 `docker run alpine nosuchcommand`，看 exit code 是多少，對照上面的表格。
6. 用 `docker system prune` 清理，記下 reclaimed 的空間。

## 下一章

→ [05. 進到 container 裡：exec、logs、inspect](../05-exec-logs-inspect)：container 跑起來之後，怎麼看它在做什麼、進去它裡面、查它的設定。
