# syntax=docker/dockerfile:1
#
# 第 16 章：在 distroless image 裡加上 HEALTHCHECK。
#
# distroless 沒有 shell、沒有 curl，所以另外編一個迷你 probe 程式放進去。
# probe 的原始碼在 tutorials/16-healthcheck-shutdown/probe/，不在 build context（app/）裡，
# 所以用 BuildKit 的「具名 build context」把它送進來（從 repo 根目錄執行）：
#
#   docker build \
#     --build-context probe=tutorials/16-healthcheck-shutdown/probe \
#     -f tutorials/16-healthcheck-shutdown/Dockerfile \
#     -t shorty:ch16 app
#
# 少了 --build-context probe=...，下面的 COPY --from=probe 會被當成 image 名稱，
# 去 Docker Hub 找 docker.io/library/probe，然後失敗。

# ── build stage：shorty 本體（跟第 11 章一樣）──────────────
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

# ── probe stage：迷你 healthcheck 程式 ──────────────────────
# stage 名字刻意叫 probe-build，不要跟具名 context「probe」撞名
FROM golang:1.26-alpine AS probe-build
WORKDIR /probe
# --from=probe 指的是 --build-context probe=... 送進來的那個資料夾
COPY --from=probe . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/probe .

# ── runtime stage ───────────────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build       /out/shorty /shorty
COPY --from=probe-build /out/probe  /probe
USER nonroot:nonroot
EXPOSE 8080

# exec form（JSON 陣列）：不經過 shell，distroless 本來就沒有 shell
#   --interval       每隔多久檢查一次
#   --timeout        單次檢查超過多久算失敗
#   --start-period   啟動寬限期：這段時間內失敗不計入 retries
#   --start-interval 寬限期內的檢查間隔（Docker Engine 25+）
#   --retries        連續失敗幾次才標成 unhealthy
# 打的是 /healthz（liveness），不是 /readyz —— 理由見 README「核心概念」
HEALTHCHECK --interval=10s --timeout=3s --start-period=10s --start-interval=1s --retries=3 \
    CMD ["/probe"]

# exec form：shorty 直接當 PID 1，docker stop 的 SIGTERM 直接送到它手上
ENTRYPOINT ["/shorty"]
