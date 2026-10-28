# syntax=docker/dockerfile:1
#
# 第 22 章：在第 21 章的 non-root 版上加 OCI 標準 label
#
#   docker build -f tutorials/22-tags-registry/Dockerfile \
#     --build-arg VERSION=1.2.0 \
#     --build-arg REVISION=$(git rev-parse HEAD) \
#     --build-arg CREATED=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
#     -t shorty:1.2.0 app
#
#   docker image inspect --format '{{json .Config.Labels}}' shorty:1.2.0
#
# app/Dockerfile 只寫了「不會變」的 source / title；會變的 version / revision / created
# 在 CI 交給 docker/metadata-action 產生（第 24 章），這份示範手動版。

# ── build stage ─────────────────────────────────────────────
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
FROM gcr.io/distroless/static-debian12:nonroot

# ARG 的作用範圍只到所在的 stage 結束：runtime stage 要用就得再宣告一次
ARG VERSION=dev
ARG REVISION=unknown
ARG CREATED=unknown

# org.opencontainers.image.* 是 OCI 規範的標準 key，GHCR / Docker Hub / Trivy 都看得懂
# source 指向 GitHub repo：GHCR 會用它把 package 自動連到 repo 頁面
LABEL org.opencontainers.image.source="https://github.com/ChecheHuang/docker-tutorials" \
      org.opencontainers.image.title="shorty" \
      org.opencontainers.image.description="shorty URL shortener (docker-tutorials)" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${REVISION}" \
      org.opencontainers.image.created="${CREATED}"

COPY --from=build /out/shorty /shorty

USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/shorty"]
