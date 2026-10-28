# 驗證清單：在真的 Docker 上把整套跑一遍

這套教學是在**沒有 Docker 的機器上寫的**。CI（`.github/workflows/docker.yml`）已經證實 `app/Dockerfile` 能 build、多平台 build 能過、Trivy 掃描通過，但**各章自己的 Dockerfile、compose 檔和「動手做」指令都還沒在真的 Docker 上跑過**。這份清單就是要補上這一步。

> 給 Claude：使用者會在 WSL2 的 Ubuntu 裡開一個新的 Claude Code session，請你照這份清單驗證並修正。先讀 [README.md](./README.md)、[ROADMAP.md](./ROADMAP.md)、[WINDOWS.md](./WINDOWS.md) 了解整套的結構與慣例。

## 環境

- Windows 11 + WSL2 Ubuntu，Docker Engine 直接裝在 Ubuntu 裡（[WINDOWS.md](./WINDOWS.md) 的 3B 裝法），**不是** Docker Desktop。
- repo 在 `~/tutorials/docker-tutorials`（WSL2 的檔案系統），不要用 `/mnt/c/...` 那份。
- 開始前確認：`docker version`（Client、Server 都有）、`docker compose version`、`docker buildx version`、`docker run --rm hello-world` 不需要 `sudo`。
- Go 不一定有裝。`./run test` 需要 Go，但驗證 Docker 不需要。

## 不要做的事

- **不要在沒問過使用者的情況下**執行以下指令，它們會刪資料或影響整台機器：
  - `docker system prune -a`（會刪掉所有沒在用的 image）
  - `docker compose down -v`、`docker volume rm`（會刪 Postgres 資料）
  - 修改 `/etc/docker/daemon.json` 並重啟 Docker（第 25 章）
  - 第 25 章塞滿磁碟的演練
- 不要 push image 到 registry（第 22、24 章）。
- `sudo` 需要使用者輸入密碼，需要的話請使用者自己執行。

## 驗證順序

依序做，每一步記錄「通過／失敗＋錯誤訊息」。失敗就修，修完重跑那一步。

### 1. 最終版（最重要）

```bash
docker build -t shorty app
docker run --rm -p 8080:8080 shorty            # 另一個終端機：curl localhost:8080/healthz
docker compose up -d --build
docker compose ps                               # app、db、cache 都要 healthy／running
curl -XPOST localhost:8080/api/links -d '{"url":"https://go.dev"}'
curl -i localhost:8080/<上一步回傳的 code>     # 要 302
curl localhost:8080/readyz                      # store、cache 都是 ok
docker compose down
```

### 2. 各章的 Dockerfile 能不能 build

每章的 Dockerfile 都在章節目錄，build context 是 `app/`（Rust／Node 範例例外，見各章 README）。全部 build 一遍，並記下實際的 image 大小，跟 README 裡標「範例」的數字比對，差很多就更新 README。

**最可能出錯、優先檢查**：

| 章節 | 檢查什麼 |
|------|------|
| 08、09 | Node 的 image tag：`node:24-*`、`gcr.io/distroless/nodejs24-debian12` 是否存在 |
| 08、09 | Rust 的 cargo-chef 流程、`rust:1.95-*` tag、musl 靜態連結版本 |
| 11 | cache mount、secret mount 的範例（`Dockerfile.private-modules` 是範本，本來就不能 build，不用修） |
| 16 | `--build-context probe=...` + `COPY --from=probe` 的健康檢查程式 |
| 23 | 多平台 build：先依第 23 章註冊 QEMU、建 buildx builder；只 build 不 push |

### 3. 各章 compose 檔

```bash
docker compose -f tutorials/17-compose-basics/compose.yaml config      # 先看合併結果
docker compose -f tutorials/17-compose-basics/compose.yaml up -d --build
```

- 第 17、18 章：照 README 的「動手做」跑一遍。
- 第 19 章：**要先 `cd tutorials/19-compose-dev-workflow`**（`compose.override.yaml` 只有不加 `-f` 時才會自動合併）。驗證 air 熱重載：改 `app/internal/server/server.go`，確認 container 會自動重編，**驗證完把改動還原**。也驗證 `develop.watch` 和 `--profile tools`。
- 第 20 章：`broken.compose.yaml` 是故意寫壞的，確認 README 裡寫的錯誤訊息跟實際一致，不一樣就更新 README。

### 4. 各章「動手做」的指令

逐章照 README 的「動手做」段落執行，重點看：

- 指令能不能跑、flag 是否存在
- 實際輸出跟 README 的「範例輸出」是否大致相符（ID、時間、大小不同是正常的）
- 第 12–15 章的腳本：`tutorials/12-networking/setup.sh`、`13-volumes/backup.sh`、`restore.sh`、`15-resources-restart/oom-demo.sh`
- 第 15 章 OOM 示範要看到 exit code 137、`OOMKilled=true`
- 第 21 章：Trivy 用 `aquasec/trivy` image 跑；Docker Scout 需要 `docker login`，沒登入就跳過並註記
- 第 25 章：只跑**不會**刪資料、不會改 daemon 設定的演練（OOM、時區、DNS、port 衝突、殭屍行程），其他先問使用者

## 修正原則

- `app/` 的 Go 程式碼和 `app/Dockerfile`、根目錄 `compose.yaml` 是共用的最終版，改了要確認不會影響其他章節的說明。
- README 的「範例輸出」改成實際跑出來的結果時，ID、digest 這類每次都不同的值保留成範例即可。
- 修完一個段落就 commit 一次，commit message 用中文、說明修了哪一章的什麼問題。
- 全部做完後，在這份文件最下面補一段「驗證結果」：哪些通過、修了什麼、還有哪些沒驗證（例如需要登入的步驟）。

## 驗證結果

驗證日期 2026-10-28，環境：WSL2 Ubuntu、Docker Engine 29.9.2、Compose v5.6.1、buildx v0.38.1、amd64 主機（arm64 用 QEMU 模擬）。

**通過**

- **步驟 1 最終版**：`docker build`、`docker run`、`compose up --build`（db、cache healthy）、POST 回 201、GET 短網址回 302、`/readyz` 回 `{"cache":"ok","store":"ok"}`、`down`。
- **步驟 2 各章 Dockerfile**：第 06–11、14、16（需 `--build-context probe=...`）、19（`Dockerfile.dev`）、21、22、25（tz）全部 build 成功。第 11 章 `secret-demo` 的 secret 不在 `docker history` 裡；`private-modules` 依 VERIFY 預期需要 `.netrc`，不驗證。
- **步驟 3 compose**：第 17、18、19（預設、prod、watch）、20、21、25 的 `config` 皆通過。第 17、18 `up`／`down` 正常；第 18 `down`（不加 `-v`）後再 `up`，資料仍在。第 19 air 熱重載與 `compose watch` 的 rebuild 皆實測（測完已還原 `server.go`）；`--profile tools` 的 pgAdmin 回 302。第 20 broken 檔逐關重現，錯誤訊息與 README 一致。
- **步驟 4 動手做**：第 08 `--target build`、第 09 壞掉的 build 出現 `exec /shorty: no such file or directory`、第 10 build context 排除 `.env`、`VERSION` 注入、第 11 cache／builder／secret、第 12 `setup.sh up/check/down`、第 13 備份還原（非空 volume 會拒絕覆蓋）、第 14 密碼檔與 `--env-file`、第 15 OOM（exit 137、`OOMKilled=true`）、第 16 healthcheck 轉 healthy、`docker stop` 0.6 秒 exit 0；shell-form 等滿 10 秒後 exit 137。第 21 Trivy：最終版 0 HIGH／CRITICAL，`Dockerfile.bad` 有 1 個 HIGH；read-only、cap-drop、`--user` 範例行為符合 README。第 22 本機 build 與 OCI label 正確（未 push）。第 23 `binfmt` 註冊後，amd64 + arm64 的 cross、qemu、rust、node 多平台 build 皆通過，arm64 image 在 QEMU 下回 `{"status":"ok"}`。第 25 OOM、時區、DNS 失敗、port 衝突、殭屍行程皆重現。

**修正（已 commit）**

- 第 21 `compose.hardened.yaml`：`pids_limit` 與 `deploy.resources.limits` 併用時，compose v5.6 報 `can't set distinct values on 'pids_limit' and 'deploy.resources.limits.pids'`。改用 `mem_limit: 128M`，限制值不變。
- 第 25 README 演練 4：`docker run --network container:$APP` 在 app 退出後無法執行，改成在預設 bridge 上測（這也是 app 所在的網路）。
- 第 25 README 演練 6 與 `zombie/compose.yaml` 註解：此環境的 `docker top` 不列出 `<defunct>`，改用 `docker exec ... ps -o pid,ppid,stat,comm`。
- 範例 image 大小依實測更新：第 07、08、09、10、11、21 章。

**未驗證／需要確認**

- **沒有執行的破壞性指令**（VERIFY 規定要先問）：第 18、19、20 章的 `down -v`、第 14 章的 `docker volume rm shorty-db-ch14`、第 13 章的 `docker volume prune`、第 25 章 disk 演練與 `daemon.json` 演練。
- **第 21 章 Docker Scout**：未登入，略過。
- **第 22 章 `release.sh`**：未 push。只驗證了等價的本機 build 與 label。
- **第 24 章 GitHub Actions**：無法在本機執行。workflow 可解析，用到的 action 版本 tag／SHA 都存在。
- **第 15 章**：只跑了 OOM 腳本，其他段落未逐項跑。
- **第 06、07 章**：只驗證 build 指令，未比對 cache 耗時。
- **測試殘留**（未清除，需要你決定）：volumes `docker-tutorials_db-data`、`18-compose-multi-service_db-data`、`shorty-db`、`shorty-db-restored`、`shorty-db-ch14`、`19-compose-dev-workflow_*`；buildx builder `multi`（第 23 章建立）；兩個無名的 `shorty` container（`lucid_goldstine`、`serene_lewin`，沒有發布 port）。
