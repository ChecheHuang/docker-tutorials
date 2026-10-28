package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ChecheHuang/docker-tutorials/app/internal/cache"
	"github.com/ChecheHuang/docker-tutorials/app/internal/store"
)

func newTestServer() *httptest.Server {
	s := &Server{
		Store:   store.NewMemory(),
		Cache:   cache.Noop{},
		BaseURL: "http://short.test",
		Version: "test",
		Log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	return httptest.NewServer(s.Routes())
}

func TestCreateRedirectAndStats(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	res, err := http.Post(ts.URL+"/api/links", "application/json", strings.NewReader(`{"url":"https://example.com"}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d", res.StatusCode)
	}
	var created struct {
		Code     string `json:"code"`
		ShortURL string `json:"short_url"`
	}
	_ = json.NewDecoder(res.Body).Decode(&created)
	res.Body.Close()
	if !strings.HasPrefix(created.ShortURL, "http://short.test/") {
		t.Fatalf("short_url = %q", created.ShortURL)
	}

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err = client.Get(ts.URL + "/" + created.Code)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusFound || res.Header.Get("Location") != "https://example.com" {
		t.Fatalf("redirect = %d %q", res.StatusCode, res.Header.Get("Location"))
	}

	res, err = http.Get(ts.URL + "/api/links/" + created.Code)
	if err != nil {
		t.Fatal(err)
	}
	var link store.Link
	_ = json.NewDecoder(res.Body).Decode(&link)
	res.Body.Close()
	if link.Hits != 1 {
		t.Fatalf("hits = %d, want 1", link.Hits)
	}
}

func TestRejectsBadURL(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	res, err := http.Post(ts.URL+"/api/links", "application/json", strings.NewReader(`{"url":"ftp://x"}`))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", res.StatusCode)
	}
}

func TestHealthAndReady(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()
	for _, p := range []string{"/healthz", "/readyz", "/version"} {
		res, err := http.Get(ts.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("%s status = %d", p, res.StatusCode)
		}
	}
}
