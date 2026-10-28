# shorty 第四版 Dockerfile（第 09 章）—— 選定 distroless/static 當 runtime base
#
#   docker build -f tutorials/09-base-images/Dockerfile -t shorty:ch09 app
#
# 其他候選版本放在同目錄：Dockerfile.debian-slim / Dockerfile.alpine / Dockerfile.scratch

# ── build stage：四個版本都一樣，只換下面的 runtime base ─────────
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# CGO_ENABLED=0 → 純 Go 靜態執行檔，不依賴 libc，放進哪種 base 都能跑
# -trimpath 拿掉本機路徑；-s -w 拿掉符號表與除錯資訊（執行檔小約 30%）
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/shorty .

# ── runtime：distroless/static ─────────────────────────────
# 只有 CA 憑證、tzdata、/etc/passwd，沒有 shell、沒有套件管理器
FROM gcr.io/distroless/static-debian12
COPY --from=build /out/shorty /shorty
EXPOSE 8080
ENTRYPOINT ["/shorty"]
