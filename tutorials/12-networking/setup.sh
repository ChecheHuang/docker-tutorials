#!/usr/bin/env bash
# 第 12 章：手動把 shorty + Postgres + Redis 接在同一個自訂 network 上
#
#   bash tutorials/12-networking/setup.sh up      # 建 network、起三個 container
#   bash tutorials/12-networking/setup.sh check   # 從同一個 network 裡測 DNS 與連線
#   bash tutorials/12-networking/setup.sh down    # 全部移除
#
# 從 repo 根目錄執行。第 17 章會把這整份換成 compose.yaml。
set -euo pipefail

NET=shorty-net

up() {
  docker network create "$NET" >/dev/null 2>&1 || true

  docker run -d --name db --network "$NET" \
    -e POSTGRES_USER=shorty \
    -e POSTGRES_PASSWORD=shorty \
    -e POSTGRES_DB=shorty \
    postgres:17-alpine

  docker run -d --name cache --network "$NET" redis:7-alpine

  docker build -t shorty app

  # shorty 自己會重試連 Postgres 10 次，DB 晚幾秒好也沒關係
  docker run -d --name shorty --network "$NET" -p 8080:8080 \
    -e BASE_URL=http://localhost:8080 \
    -e "DATABASE_URL=postgres://shorty:shorty@db:5432/shorty?sslmode=disable" \
    -e REDIS_URL=redis://cache:6379/0 \
    shorty

  echo "等 shorty 起來後：curl localhost:8080/readyz"
}

check() {
  # shorty 是 distroless，沒有 shell；借一個 busybox 進同一個 network 測
  docker run --rm --network "$NET" busybox:1.37 sh -c '
    for h in db cache shorty; do nslookup "$h" 2>/dev/null | grep -A1 "Name:" ; done
    echo "--- GET http://shorty:8080/readyz"
    wget -qO- http://shorty:8080/readyz; echo
  '
}

down() {
  # -v：順便刪掉 postgres image 自帶的匿名 volume（第 13 章會改用 named volume）
  docker rm -f -v shorty cache db 2>/dev/null || true
  docker network rm "$NET" 2>/dev/null || true
}

case "${1:-}" in
  up) up ;;
  check) check ;;
  down) down ;;
  *) echo "用法：$0 up|check|down" >&2; exit 1 ;;
esac
