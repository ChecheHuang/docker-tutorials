# syntax=docker/dockerfile:1
#
# shorty 的最終版 Dockerfile —— 整套教學一路演進到這裡。
# 各章的中間版本放在 tutorials/NN-*/Dockerfile，這份是「上線用」的結果。
#
#   docker build -t shorty app
#   docker build --build-arg VERSION=v1.2.3 -t shorty:v1.2.3 app
#   docker buildx build --platform linux/amd64,linux/arm64 -t <registry>/shorty --push app   # 第 23 章
#   （多平台 image 要 --push 到 registry；除非開了 containerd image store，否則無法 --load 回本機）

# ── build stage ─────────────────────────────────────────────
# --platform=$BUILDPLATFORM：編譯器跑在 build 機器的原生架構，靠 Go 交叉編譯產出目標架構（第 23 章）
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
WORKDIR /src

# 先只複製 go.mod / go.sum 下載依賴：原始碼改了，這層快取還在（第 07 章）
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
# CGO_ENABLED=0 → 靜態連結，才能放進沒有 libc 的 distroless/static（第 09 章）
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/shorty .

# ── runtime stage ───────────────────────────────────────────
# distroless/static：只有 CA 憑證、tzdata、/etc/passwd，沒有 shell（第 09、21 章）
FROM gcr.io/distroless/static-debian12:nonroot

# OCI 標準 label，registry 頁面會顯示（第 22 章）
LABEL org.opencontainers.image.source="https://github.com/ChecheHuang/docker-tutorials" \
      org.opencontainers.image.title="shorty"

COPY --from=build /out/shorty /shorty

# nonroot 使用者（uid 65532），不要用 root 跑服務（第 21 章）
USER nonroot:nonroot
EXPOSE 8080

# exec form：shorty 直接當 PID 1，才收得到 docker stop 送的 SIGTERM（第 16 章）
ENTRYPOINT ["/shorty"]
