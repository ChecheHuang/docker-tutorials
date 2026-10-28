// Package cache 快取 code → url 的對應，讓轉址不用每次查 DB。
//
// 沒設 REDIS_URL 時用 Noop（永遠 miss），服務照樣能跑。
package cache

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

var ErrMiss = errors.New("cache miss")

type Cache interface {
	Get(ctx context.Context, code string) (string, error)
	Set(ctx context.Context, code, url string) error
	Ping(ctx context.Context) error
	Close()
}

// ── Noop ──────────────────────────────────────────────────

type Noop struct{}

func (Noop) Get(context.Context, string) (string, error) { return "", ErrMiss }
func (Noop) Set(context.Context, string, string) error   { return nil }
func (Noop) Ping(context.Context) error                  { return nil }
func (Noop) Close()                                      {}

// ── Redis ─────────────────────────────────────────────────

type Redis struct {
	client *redis.Client
	ttl    time.Duration
}

func NewRedis(ctx context.Context, url string) (*Redis, error) {
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, err
	}
	c := redis.NewClient(opt)
	if err := c.Ping(ctx).Err(); err != nil {
		_ = c.Close()
		return nil, err
	}
	return &Redis{client: c, ttl: time.Hour}, nil
}

func (r *Redis) Get(ctx context.Context, code string) (string, error) {
	v, err := r.client.Get(ctx, "link:"+code).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrMiss
	}
	return v, err
}

func (r *Redis) Set(ctx context.Context, code, url string) error {
	return r.client.Set(ctx, "link:"+code, url, r.ttl).Err()
}

func (r *Redis) Ping(ctx context.Context) error { return r.client.Ping(ctx).Err() }
func (r *Redis) Close()                         { _ = r.client.Close() }
