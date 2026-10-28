# syntax=docker/dockerfile:1
#
# 第 21 章：安全強化版 —— 在第 11 章的 BuildKit 版上，把 runtime 改成 non-root
#
#   docker build -f tutorials/21-image-security/Dockerfile -t shorty:ch21 app
#   docker image inspect --format '{{.Config.User}}' shorty:ch21     # → nonroot:nonroot
#
# 跟第 11 章比，改了兩件事：
#   1. runtime base 從 distroless/static-debian12 換成 :nonroot 變體
#   2. 明確寫 USER nonroot:nonroot（不靠 base image 的預設，讀 Dockerfile 的人一眼看得到）
#
# 正式環境再加一步：把 base image 釘在 digest（見 README「pin base image」）
#   FROM gcr.io/distroless/static-debian12:nonroot@sha256:<用 imagetools inspect 查到的 digest>
#
# 還沒加的（後面章節）：OCI LABEL（22）、--platform=$BUILDPLATFORM（23）

# ── build stage ─────────────────────────────────────────────
# build stage 不會出現在最終 image，裡面有編譯器、git 都沒關係
FROM golang:1.26-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/shorty .

# ── runtime stage ───────────────────────────────────────────
# distroless/static：沒有 shell、沒有套件管理器、沒有 curl —— 攻擊者進來也沒工具可用
# :nonroot 變體的 /etc/passwd 裡有 nonroot（uid/gid 65532）
FROM gcr.io/distroless/static-debian12:nonroot

# 執行檔屬於 root、權限 755：nonroot 能執行、不能改寫自己的執行檔
COPY --from=build /out/shorty /shorty

USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/shorty"]
