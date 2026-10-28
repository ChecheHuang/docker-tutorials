# shorty 第三版 Dockerfile（第 08 章）—— multi-stage：在大 image 編譯，只把執行檔搬進小 image
#
#   docker build -f tutorials/08-multi-stage/Dockerfile -t shorty:ch08 app
#   docker run --rm -p 8080:8080 shorty:ch08

# ── build stage：有 Go 工具鏈、原始碼、module cache ──────────
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# CGO_ENABLED=0：編出不依賴 C 函式庫的靜態執行檔（為什麼重要見第 09 章）
RUN CGO_ENABLED=0 go build -o /out/shorty .

# ── runtime stage：乾淨的 alpine，只放一個執行檔 ─────────────
FROM alpine:3.22
COPY --from=build /out/shorty /usr/local/bin/shorty
EXPOSE 8080
CMD ["shorty"]
