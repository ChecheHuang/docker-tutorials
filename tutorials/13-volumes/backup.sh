#!/usr/bin/env bash
# 第 13 章：把一個 docker volume 打包成 .tgz（透過 stdout 串流，不需要 bind mount）
#
#   bash tutorials/13-volumes/backup.sh shorty-db tutorials/13-volumes/backup/shorty-db.tgz
#
# 備份資料庫的 volume 之前，先 `docker stop db`，否則檔案可能停在寫到一半的狀態。
set -euo pipefail
export MSYS_NO_PATHCONV=1   # Git Bash：不要把 :/volume 這種參數改寫成 Windows 路徑

VOL="${1:?用法：$0 <volume> <輸出檔.tgz>}"
OUT="${2:?用法：$0 <volume> <輸出檔.tgz>}"

docker volume inspect "$VOL" >/dev/null   # 不存在就直接報錯，免得默默建一個空的
mkdir -p "$(dirname "$OUT")"

# :ro 唯讀掛載，備份程式不可能弄壞原始資料
docker run --rm -v "$VOL":/volume:ro alpine:3.22 \
  tar czf - -C /volume . > "$OUT"

echo "已備份 $VOL → $OUT（$(wc -c < "$OUT") bytes）"
