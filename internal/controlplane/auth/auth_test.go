package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLocalIgnoresForgedHeaders(t *testing.T) {
	b, e := New(Config{Mode: "local"})
	if e != nil {
		t.Fatal(e)
	}
	h := b.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	for _, addr := range []string{"198.51.100.1:4000", "10.0.0.1:4000"} {
		r := httptest.NewRequest("GET", "http://localhost/", nil)
		r.RemoteAddr = addr
		r.Header.Set("X-Forwarded-For", "127.0.0.1")
		r.Header.Set("Cf-Access-Authenticated-User-Email", "operator@example.com")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 401 {
			t.Errorf("untrusted %s status=%d want 401", addr, w.Code)
		}
	}
}
func TestAccessFailsClosed(t *testing.T) {
	if _, e := New(Config{}); e == nil {
		t.Fatal("missing access configuration accepted")
	}
}
func TestLocalTrustOnlyPrivateSubset(t *testing.T) {
	for _, s := range []string{"0.0.0.0/0", "172.0.0.0/8", "127.0.0.0/8", "192.0.2.0/24"} {
		if _, e := New(Config{Mode: "local", TrustCIDR: s}); e == nil {
			t.Errorf("accepted %s", s)
		}
	}
}
