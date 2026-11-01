package api

import (
	"encoding/json"
	"github.com/postfix/cctvscan/internal/controlplane/auth"
	"net/http/httptest"
	"testing"
)

func TestCatalogueIsReachableOfflineThroughProtectedAPI(t *testing.T) {
	s, e := New(Options{Auth: auth.Config{Mode: "local"}, PublicOrigin: "http://localhost:14904"})
	if e != nil {
		t.Fatal(e)
	}
	for _, path := range []string{"/api/catalogue/meta", "/api/catalogue/models?search=RLC-823A&limit=10", "/api/catalogue/models/reolink-rlc-823a", "/api/catalogue/profiles?brand=hikvision"} {
		r := httptest.NewRequest("GET", "http://localhost:14904"+path, nil)
		r.RemoteAddr = "127.0.0.1:5000"
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != 200 || !json.Valid(w.Body.Bytes()) {
			t.Fatalf("%s: status %d", path, w.Code)
		}
	}
	r := httptest.NewRequest("GET", "http://localhost:14904/api/catalogue/meta", nil)
	r.RemoteAddr = "203.0.113.1:5000"
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code < 400 {
		t.Fatal("catalogue escaped authentication boundary")
	}
}
