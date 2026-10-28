# syntax=docker/dockerfile:1
#
# 第 11 章：BuildKit 版 —— 在第 10 章的 multi-stage 上加 cache mount
#
#   docker build -f tutorials/11-buildkit/Dockerfile -t shorty:ch11 app
#   docker build -f tutorials/11-buildkit/Dockerfile --build-arg VERSION=v0.11.0 -t shorty:ch11 app
#   docker build -f tutorials/11-buildkit/Dockerfile --progress=plain -t shorty:ch11 app
#
# 跟第 10 章比，只多了兩件事：
#   1. 第一行的 `# syntax=`：指定 Dockerfile 前端版本，才能用 RUN --mount
#   2. RUN --mount=type=cache：Go 的 module 快取與編譯快取跨 build 保留
#
# 還沒加的（後面章節）：non-root USER（21）、LABEL（22）、--platform=$BUILDPLATFORM（23）

# ── build stage ─────────────────────────────────────────────
FROM golang:1.26-alpine AS build
WORKDIR /src

# 依賴層：go.mod / go.sum 沒變，這層直接用 layer 快取（第 07 章）
# 就算這層失效（例如加了一個依賴），cache mount 裡的舊模組還在，只下載新的那個
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

ARG VERSION=dev
# 編譯時也要掛同一個 /go/pkg/mod：cache mount 的內容「不會」留在 layer 裡，
# 少掛這一行，go build 會找不到剛剛下載的模組
# /root/.cache/go-build 是 Go 的編譯快取（GOCACHE 預設位置）：只改一個檔案，其他 package 不用重編
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/shorty .

# ── runtime stage ───────────────────────────────────────────
FROM gcr.io/distroless/static-debian12
COPY --from=build /out/shorty /shorty
EXPOSE 8080
ENTRYPOINT ["/shorty"]
