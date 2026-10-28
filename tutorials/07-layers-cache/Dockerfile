# shorty 第二版 Dockerfile（第 07 章）—— 依賴層獨立出來，改程式碼不用重新下載依賴
#
#   docker build -f tutorials/07-layers-cache/Dockerfile -t shorty:ch07 app
#   docker history shorty:ch07

FROM golang:1.26
WORKDIR /src

# ① 先只複製「依賴清單」：go.mod / go.sum 很少變
COPY go.mod go.sum ./
# ② 下載依賴：只要上面兩個檔沒變，這層就直接吃快取
RUN go mod download

# ③ 再複製會常常變的原始碼：改 main.go 只會讓這行之後的 layer 重做
COPY . .
RUN go build -o /usr/local/bin/shorty .

EXPOSE 8080
CMD ["shorty"]
