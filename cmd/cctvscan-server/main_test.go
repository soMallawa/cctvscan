package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRuntimeConfig(t *testing.T) {
	dir := t.TempDir()
	key := filepath.Join(dir, "key")
	dsn := filepath.Join(dir, "dsn")
	if err := os.WriteFile(key, []byte(strings.Repeat("k", 32)), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dsn, []byte("postgres://fixture@127.0.0.1:14902/fixture\n"), 0600); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"CCTVSCAN_SECRET_KEY_FILE": key, "DATABASE_URL_FILE": dsn, "CCTVSCAN_PUBLIC_ORIGIN": "http://127.0.0.1:14903", "CCTVSCAN_AUTH_MODE": "local", "CCTVSCAN_LISTEN_ADDR": "127.0.0.1:14904"}
	get := func(k string) string { return env[k] }
	cfg, err := loadConfig(get)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.listen != "127.0.0.1:14904" || len(cfg.key) != 32 || strings.Contains(cfg.dsn, "\n") {
		t.Fatal("invalid loaded config")
	}
	for _, tc := range []struct{ k, v string }{{"CCTVSCAN_AUTH_MODE", ""}, {"CCTVSCAN_LISTEN_ADDR", "0.0.0.0:14904"}, {"CCTVSCAN_LISTEN_ADDR", "localhost:14904"}, {"CCTVSCAN_PUBLIC_ORIGIN", ""}, {"CCTVSCAN_PUBLIC_ORIGIN", "file:///tmp"}, {"CCTVSCAN_LOCAL_TRUST_CIDR", "10.0.0.0/8"}} {
		old := env[tc.k]
		env[tc.k] = tc.v
		if _, err := loadConfig(get); err == nil {
			t.Errorf("accepted unsafe %s=%q", tc.k, tc.v)
		}
		env[tc.k] = old
	}
	if err := os.Chmod(key, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig(get); err == nil {
		t.Fatal("accepted public key file")
	}
	os.Chmod(key, 0600)
	link := filepath.Join(dir, "link")
	os.Symlink(key, link)
	env["CCTVSCAN_SECRET_KEY_FILE"] = link
	if _, err := loadConfig(get); err == nil {
		t.Fatal("accepted symlink")
	}
}

func TestHTTPBoundsAndShutdownContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv := newHTTPServer(ctx, "127.0.0.1:0", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	if srv.WriteTimeout != 0 || srv.ReadTimeout <= 0 || srv.ReadHeaderTimeout <= 0 || srv.MaxHeaderBytes > 65536 || srv.IdleTimeout <= 0 {
		t.Fatal("unsafe HTTP bounds")
	}
	base := srv.BaseContext(nil)
	cancel()
	if base.Err() == nil {
		t.Fatal("shutdown does not cancel request base context")
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/jobs", strings.NewReader(strings.Repeat("x", (1<<20)+1)))
	srv.Handler.ServeHTTP(w, r)
	if w.Code != 204 {
		t.Fatal(w.Code)
	}
}
