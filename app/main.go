// shorty — 貫穿整套 Docker 教學的短網址服務。
//
// 所有設定都從環境變數來（第 14 章）：
//
//	PORT          監聽 port，預設 8080
//	BASE_URL      回傳短網址的前綴，預設 http://localhost:$PORT
//	DATABASE_URL  Postgres DSN；沒設就用 in-memory（重啟即消失）
//	REDIS_URL     Redis URL；沒設就不快取
//	LOG_LEVEL     debug / info / warn / error，預設 info
//
// 收到 SIGTERM / SIGINT 會 graceful shutdown（第 16 章）。
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ChecheHuang/docker-tutorials/app/internal/cache"
	"github.com/ChecheHuang/docker-tutorials/app/internal/server"
	"github.com/ChecheHuang/docker-tutorials/app/internal/store"
)

// version 在 build 時用 -ldflags "-X main.version=..." 注入（第 10 章；第 22、24 章拿來對應 image tag）。
var version = "dev"

func main() {
	log := newLogger(os.Getenv("LOG_LEVEL"))
	if err := run(log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	port := getenv("PORT", "8080")
	baseURL := getenv("BASE_URL", "http://localhost:"+port)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	st, err := openStore(ctx, log)
	if err != nil {
		return err
	}
	defer st.Close()

	ca, err := openCache(ctx, log)
	if err != nil {
		return err
	}
	defer ca.Close()

	srv := &http.Server{
		Addr: ":" + port,
		Handler: (&server.Server{
			Store: st, Cache: ca, BaseURL: baseURL, Version: version, Log: log,
		}).Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", srv.Addr, "version", version)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		log.Info("shutdown signal received, draining")
	}

	// docker stop 預設給 10 秒才 SIGKILL，這裡留 8 秒收尾
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	log.Info("bye")
	return nil
}

func openStore(ctx context.Context, log *slog.Logger) (store.Store, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Warn("DATABASE_URL not set, using in-memory store")
		return store.NewMemory(), nil
	}
	// compose 剛起來時 DB 可能還沒好，重試幾次（第 18 章）
	var lastErr error
	for i := 1; i <= 10; i++ {
		pg, err := store.NewPostgres(ctx, dsn)
		if err == nil {
			log.Info("connected to postgres")
			return pg, nil
		}
		lastErr = err
		log.Warn("postgres not ready, retrying", "attempt", i, "err", err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return nil, lastErr
}

func openCache(ctx context.Context, log *slog.Logger) (cache.Cache, error) {
	url := os.Getenv("REDIS_URL")
	if url == "" {
		log.Warn("REDIS_URL not set, cache disabled")
		return cache.Noop{}, nil
	}
	r, err := cache.NewRedis(ctx, url)
	if err != nil {
		return nil, err
	}
	log.Info("connected to redis")
	return r, nil
}

// 輸出 JSON 到 stdout：container 的 log 慣例是寫 stdout，交給 docker logs 收（第 05 章）
func newLogger(level string) *slog.Logger {
	var lv slog.Level
	if err := lv.UnmarshalText([]byte(level)); err != nil {
		lv = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lv}))
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
