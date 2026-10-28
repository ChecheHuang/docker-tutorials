# shorty 第一版 Dockerfile（第 06 章）—— 能跑，但很胖（~1GB）
#
#   docker build -f tutorials/06-dockerfile-basics/Dockerfile -t shorty:ch06 app
#   docker run --rm -p 8080:8080 shorty:ch06
#
# build context 是 app/，所以下面的 COPY . . 指的是 app/ 底下的東西。

# 以官方 Go image 當底：裡面有完整的 Debian + Go 工具鏈
FROM golang:1.26

# 之後的 COPY / RUN / CMD 都以 /src 為工作目錄（不存在會自動建立）
WORKDIR /src

# 把 build context（app/）整包複製進 image 的 /src
COPY . .

# 在 image 裡編譯：下載依賴 + 編出執行檔
RUN go build -o /usr/local/bin/shorty .

# 文件性質：告訴使用者這個服務聽 8080（真正對外開放要靠 docker run -p）
EXPOSE 8080

# container 啟動時預設執行的指令（exec form，第 16 章會講為什麼）
CMD ["shorty"]
