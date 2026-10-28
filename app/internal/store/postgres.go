package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Postgres 用 pgx connection pool。啟動時自動建表，教學用不另外拉 migration 工具。
type Postgres struct {
	pool *pgxpool.Pool
}

const schema = `
CREATE TABLE IF NOT EXISTS links (
    code       TEXT PRIMARY KEY,
    url        TEXT NOT NULL,
    hits       BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
)`

func NewPostgres(ctx context.Context, dsn string) (*Postgres, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("pgxpool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	if _, err := pool.Exec(ctx, schema); err != nil {
		pool.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Postgres{pool: pool}, nil
}

func (p *Postgres) Create(ctx context.Context, code, url string) (Link, error) {
	var l Link
	err := p.pool.QueryRow(ctx,
		`INSERT INTO links (code, url) VALUES ($1, $2)
		 RETURNING code, url, hits, created_at`, code, url,
	).Scan(&l.Code, &l.URL, &l.Hits, &l.CreatedAt)
	return l, err
}

func (p *Postgres) Get(ctx context.Context, code string) (Link, error) {
	var l Link
	err := p.pool.QueryRow(ctx,
		`SELECT code, url, hits, created_at FROM links WHERE code = $1`, code,
	).Scan(&l.Code, &l.URL, &l.Hits, &l.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Link{}, ErrNotFound
	}
	return l, err
}

func (p *Postgres) IncrHits(ctx context.Context, code string) error {
	tag, err := p.pool.Exec(ctx, `UPDATE links SET hits = hits + 1 WHERE code = $1`, code)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *Postgres) Ping(ctx context.Context) error { return p.pool.Ping(ctx) }
func (p *Postgres) Close()                         { p.pool.Close() }
