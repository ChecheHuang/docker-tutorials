#!/usr/bin/env bash
# 第 22 章：手動發佈 shorty 到 registry（第 24 章會改成 CI 自動做）
#
#   ./tutorials/22-tags-registry/release.sh 1.2.0                    # 推到 GHCR（預設）
#   IMAGE=docker.io/<你的帳號>/shorty ./tutorials/22-tags-registry/release.sh 1.2.0
#
# 事前準備：
#   GHCR      echo "$GHCR_TOKEN" | docker login ghcr.io -u <GitHub 帳號> --password-stdin
#   Docker Hub docker login
#
# 一次打四個 tag：1.2.0、1.2、1、sha-<7 碼>，只有正式版才更新 latest。

set -euo pipefail
cd "$(dirname "$0")/../.."

VERSION="${1:?用法: release.sh <semver，例如 1.2.0>}"
VERSION="${VERSION#v}"   # 允許傳 v1.2.0，tag 一律不帶 v

if ! [[ "$VERSION" =~ ^([0-9]+)\.([0-9]+)\.([0-9]+)(-[0-9A-Za-z.-]+)?$ ]]; then
  echo "不是 semver：$VERSION" >&2
  exit 1
fi
MAJOR="${BASH_REMATCH[1]}"
MINOR="${BASH_REMATCH[2]}"
PRERELEASE="${BASH_REMATCH[4]}"

# GHCR 的 image 名稱必須全小寫，GitHub 帳號有大寫也要轉
OWNER="$(git remote get-url origin 2>/dev/null | sed -E 's#.*[:/]([^/]+)/[^/]+(\.git)?$#\1#' || true)"
IMAGE="${IMAGE:-ghcr.io/${OWNER:-chechehuang}/shorty}"
IMAGE="$(echo "$IMAGE" | tr '[:upper:]' '[:lower:]')"

REVISION="$(git rev-parse HEAD)"
SHORT_SHA="$(git rev-parse --short=7 HEAD)"
CREATED="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

if [ -n "$(git status --porcelain)" ]; then
  echo "工作目錄有未 commit 的修改，revision label 會對不上程式碼，先 commit 再發佈" >&2
  exit 1
fi

TAGS=(-t "$IMAGE:$VERSION" -t "$IMAGE:sha-$SHORT_SHA")
if [ -z "$PRERELEASE" ]; then
  # 1.2.0-rc.1 這種預發佈版不應該搶走 1.2 / 1 / latest
  TAGS+=(-t "$IMAGE:$MAJOR.$MINOR" -t "$IMAGE:$MAJOR" -t "$IMAGE:latest")
fi

echo "==> build $IMAGE ($VERSION, $SHORT_SHA)"
docker build -f tutorials/22-tags-registry/Dockerfile \
  --build-arg VERSION="$VERSION" \
  --build-arg REVISION="$REVISION" \
  --build-arg CREATED="$CREATED" \
  "${TAGS[@]}" app

echo "==> push"
docker push --all-tags "$IMAGE"

echo "==> 推上去的 digest"
docker buildx imagetools inspect "$IMAGE:$VERSION"
