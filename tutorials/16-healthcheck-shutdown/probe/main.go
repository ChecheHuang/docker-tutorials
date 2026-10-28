// probe 是給 distroless image 用的迷你 healthcheck 程式（第 16 章）。
//
// distroless 沒有 shell、沒有 curl / wget，HEALTHCHECK 沒東西可以跑，
// 所以我們自己編一個只做一件事的靜態執行檔：打一個 HTTP GET，
// 2xx 就 exit 0（healthy），其他情況 exit 1（unhealthy）。
//
//	probe                                  # 預設打 http://127.0.0.1:$PORT/healthz
//	probe http://127.0.0.1:8080/readyz     # 指定網址
//
// 只用標準庫，CGO_ENABLED=0 加 -s -w 編出來約 5–6MB。
package main

import (
	"fmt"
	"net/http"
	"os"
	"time"
)

func main() {
	url := "http://127.0.0.1:" + getenv("PORT", "8080") + "/healthz"
	if len(os.Args) > 1 {
		url = os.Args[1]
	}

	// timeout 要比 HEALTHCHECK --timeout 短，自己先放棄，才看得到錯誤訊息
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		fmt.Fprintln(os.Stderr, "probe:", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		fmt.Fprintln(os.Stderr, "probe:", url, "returned", resp.Status)
		os.Exit(1) // HEALTHCHECK 只認 0 = healthy、1 = unhealthy（2 保留不要用）
	}
	fmt.Println("probe:", url, resp.Status)
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
