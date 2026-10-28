#!/usr/bin/env bash
# 第 13 章：把 backup.sh 產生的 .tgz 還原到一個 volume
#
#   bash tutorials/13-volumes/restore.sh tutorials/13-volumes/backup/shorty-db.tgz shorty-db-restored
#
# 為了安全，目標 volume 必須是「不存在」或「空的」。
set -euo pipefail
export MSYS_NO_PATHCONV=1

IN="${1:?用法：$0 <備份檔.tgz> <volume>}"
VOL="${2:?用法：$0 <備份檔.tgz> <volume>}"

[ -f "$IN" ] || { echo "找不到 $IN" >&2; exit 1; }

docker volume create "$VOL" >/dev/null

# 目標 volume 有東西就停下來，不要蓋掉別人的資料
if [ -n "$(docker run --rm -v "$VOL":/volume alpine:3.22 ls -A /volume)" ]; then
  echo "volume $VOL 不是空的，拒絕覆蓋" >&2
  exit 1
fi

# 以 root 解壓，tar 會保留原本的檔案擁有者（postgres 的 uid 70）與權限
docker run --rm -i -v "$VOL":/volume alpine:3.22 \
  tar xzf - -C /volume < "$IN"

echo "已還原 $IN → $VOL"
