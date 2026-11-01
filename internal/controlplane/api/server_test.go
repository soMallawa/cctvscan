package api

import (
	"github.com/postfix/cctvscan/internal/controlplane/auth"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOriginAndStrictJSON(t *testing.T) {
	s, e := New(Options{Auth: auth.Config{Mode: "local"}, PublicOrigin: "http://localhost:14904"})
	if e != nil {
		t.Fatal(e)
	}
	for _, v := range []struct {
		origin, body string
		status       int
	}{{"http://evil.example", "{}", 403}, {"", "{}", 403}, {"http://localhost:14904", `{"unknown":true}`, 400}, {"http://localhost:14904", `{} {}`, 400}, {"http://localhost:14904", `{"config":` + strings.Repeat(" ", 1048577) + `{}}`, 413}} {
		r := httptest.NewRequest("POST", "http://localhost:14904/api/jobs/preview", strings.NewReader(v.body))
		r.RemoteAddr = "127.0.0.1:1234"
		r.Header.Set("Origin", v.origin)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != v.status {
			t.Errorf("origin %q status=%d want=%d body=%s", v.origin, w.Code, v.status, w.Body.String())
		}
	}
}
func TestDefaultAuthFailsClosed(t *testing.T) {
	if _, e := New(Options{PublicOrigin: "https://console.example.com"}); e == nil {
		t.Fatal("unconfigured Access boundary accepted")
	}
}
func TestNoDatabaseNotReady(t *testing.T) {
	s, e := New(Options{Auth: auth.Config{Mode: "local"}, PublicOrigin: "http://localhost:14904"})
	if e != nil {
		t.Fatal(e)
	}
	r := httptest.NewRequest("GET", "http://localhost:14904/api/health", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 503 || !strings.Contains(w.Body.String(), `"auth_mode":"local"`) {
		t.Fatalf("readiness: %d %s", w.Code, w.Body.String())
	}
}
