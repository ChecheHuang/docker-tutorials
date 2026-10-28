// Package store 定義短網址的儲存介面，提供 in-memory 與 Postgres 兩種實作。
//
// 沒設 DATABASE_URL 時用 Memory —— 第 06 章只有一個 container 也能跑；
// 第 18 章接上 compose 的 Postgres 後改用 Postgres。
package store

import (
	"context"
	"errors"
	"sync"
	"time"
)

var ErrNotFound = errors.New("link not found")

type Link struct {
	Code      string    `json:"code"`
	URL       string    `json:"url"`
	Hits      int64     `json:"hits"`
	CreatedAt time.Time `json:"created_at"`
}

type Store interface {
	Create(ctx context.Context, code, url string) (Link, error)
	Get(ctx context.Context, code string) (Link, error)
	IncrHits(ctx context.Context, code string) error
	Ping(ctx context.Context) error
	Close()
}

// ── Memory ────────────────────────────────────────────────

type Memory struct {
	mu    sync.RWMutex
	links map[string]Link
}

func NewMemory() *Memory {
	return &Memory{links: make(map[string]Link)}
}

func (m *Memory) Create(_ context.Context, code, url string) (Link, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	l := Link{Code: code, URL: url, CreatedAt: time.Now().UTC()}
	m.links[code] = l
	return l, nil
}

func (m *Memory) Get(_ context.Context, code string) (Link, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	l, ok := m.links[code]
	if !ok {
		return Link{}, ErrNotFound
	}
	return l, nil
}

func (m *Memory) IncrHits(_ context.Context, code string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, ok := m.links[code]
	if !ok {
		return ErrNotFound
	}
	l.Hits++
	m.links[code] = l
	return nil
}

func (m *Memory) Ping(context.Context) error { return nil }
func (m *Memory) Close()                     {}
