// Package server 是 HTTP 層：建立短網址、轉址、查統計，加上 liveness / readiness。
package server

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/ChecheHuang/docker-tutorials/app/internal/cache"
	"github.com/ChecheHuang/docker-tutorials/app/internal/store"
)

type Server struct {
	Store   store.Store
	Cache   cache.Cache
	BaseURL string // 回給使用者的短網址前綴，例如 http://localhost:8080
	Version string
	Log     *slog.Logger
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/links", s.createLink)
	mux.HandleFunc("GET /api/links/{code}", s.getLink)
	mux.HandleFunc("GET /healthz", s.healthz) // liveness：process 活著就好（第 16 章）
	mux.HandleFunc("GET /readyz", s.readyz)   // readiness：DB / Redis 都連得上才算好
	mux.HandleFunc("GET /version", s.version) // 第 22 章：確認跑的是哪個 image
	mux.HandleFunc("GET /{code}", s.redirect)
	return s.logRequests(mux)
}

func (s *Server) createLink(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if u, err := url.ParseRequestURI(req.URL); err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		writeError(w, http.StatusBadRequest, "url must be http(s)")
		return
	}

	code := newCode()
	link, err := s.Store.Create(r.Context(), code, req.URL)
	if err != nil {
		s.Log.Error("create link", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"code":      link.Code,
		"url":       link.URL,
		"short_url": s.BaseURL + "/" + link.Code,
	})
}

func (s *Server) getLink(w http.ResponseWriter, r *http.Request) {
	link, err := s.Store.Get(r.Context(), r.PathValue("code"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		s.Log.Error("get link", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, link)
}

func (s *Server) redirect(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	ctx := r.Context()

	target, err := s.Cache.Get(ctx, code)
	if err != nil {
		link, err := s.Store.Get(ctx, code)
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			s.Log.Error("redirect lookup", "err", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		target = link.URL
		if err := s.Cache.Set(ctx, code, target); err != nil {
			s.Log.Warn("cache set", "err", err) // 快取失敗不影響轉址
		}
	}

	if err := s.Store.IncrHits(ctx, code); err != nil {
		s.Log.Warn("incr hits", "err", err)
	}
	http.Redirect(w, r, target, http.StatusFound)
}

func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	checks := map[string]string{"store": "ok", "cache": "ok"}
	status := http.StatusOK
	if err := s.Store.Ping(ctx); err != nil {
		checks["store"] = err.Error()
		status = http.StatusServiceUnavailable
	}
	if err := s.Cache.Ping(ctx); err != nil {
		checks["cache"] = err.Error()
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, checks)
}

func (s *Server) version(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"version": s.Version})
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		s.Log.Info("request", "method", r.Method, "path", r.URL.Path, "dur_ms", time.Since(start).Milliseconds())
	})
}

const alphabet = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func newCode() string {
	b := make([]byte, 7)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
