package api

import (
	"encoding/json"
	"github.com/postfix/cctvscan/internal/controlplane/auth"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVendorCredentialCommentsAreValidatedWithoutDiscardingHeaders(t *testing.T) {
	s, e := New(Options{Auth: auth.Config{Mode: "local"}, PublicOrigin: "http://localhost:14904"})
	if e != nil {
		t.Fatal(e)
	}
	body, _ := json.Marshal(map[string]string{"name": "Fixture vendor set", "content": "# Hikvision\nfixture-user:fixture-pass\n# Generic\nfixture-other:fixture-value\n"})
	r := httptest.NewRequest("POST", "http://localhost:14904/api/credential-sets", strings.NewReader(string(body)))
	r.RemoteAddr = "127.0.0.1:1234"
	r.Header.Set("Origin", "http://localhost:14904")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 503 || !strings.Contains(w.Body.String(), "secret encryption") {
		t.Fatalf("valid vendor headers were rejected before encryption: HTTP %d", w.Code)
	}
}
