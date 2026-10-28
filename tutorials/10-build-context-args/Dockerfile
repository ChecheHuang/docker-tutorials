# shorty 第五版 Dockerfile（第 10 章）—— build 時注入版本號
#
#   docker build -f tutorials/10-build-context-args/Dockerfile \
#     --build-arg VERSION=v0.1.0 -t shorty:ch10 app
#   docker run --rm -p 8080:8080 shorty:ch10
#   curl localhost:8080/version      # {"version":"v0.1.0"}
#
# build context 是 app/，app/.dockerignore 決定哪些檔案「不」送進來。

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

# ARG 只活在 build 期間、而且只在宣告它的 stage 裡有效
# 放在 COPY . . 之後：VERSION 換了只會讓下面這一行重做，不會連依賴層一起失效
ARG VERSION=dev
# -X main.version=... 在連結時把 main 套件的 version 變數改成指定字串
RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X main.version=${VERSION}" \
      -o /out/shorty .

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/shorty /shorty
EXPOSE 8080
ENTRYPOINT ["/shorty"]
