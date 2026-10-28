#!/usr/bin/env bash
# 第 15 章：故意讓一個 container 被 OOM killed，看 exit code 與 OOMKilled 旗標
#
#   bash tutorials/15-resources-restart/oom-demo.sh          # 限制 64MiB，嘗試配置 256MiB
#   bash tutorials/15-resources-restart/oom-demo.sh 32m 64   # 自訂：限制 32m，配置 64MiB
#
# 用 python 當 PID 1：它就是吃記憶體的那個行程，被 kernel 殺掉時 container 直接以 137 結束。
set -euo pipefail

LIMIT="${1:-64m}"
ALLOC_MIB="${2:-256}"
NAME=hog

docker rm -f "$NAME" >/dev/null 2>&1 || true

echo "限制 --memory=$LIMIT（關掉 swap），嘗試配置 ${ALLOC_MIB}MiB ..."
# b'x' * N 會真的寫入每一個 byte，記憶體一定會被實際使用（不像 bytearray 可能只是保留位址）
docker run --name "$NAME" --memory "$LIMIT" --memory-swap "$LIMIT" python:3.13-alpine \
  python -c "x = b'x' * (${ALLOC_MIB} * 1024 * 1024); print('居然活下來了', len(x))" || true

docker inspect "$NAME" --format 'Status={{.State.Status}} ExitCode={{.State.ExitCode}} OOMKilled={{.State.OOMKilled}}'
docker rm "$NAME" >/dev/null
