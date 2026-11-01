//go:build integration

package api

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/postfix/cctvscan/internal/controlplane/auth"
	"github.com/postfix/cctvscan/internal/controlplane/config"
	"github.com/postfix/cctvscan/internal/controlplane/secrets"
	"github.com/postfix/cctvscan/internal/controlplane/store"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func apiFixture(t *testing.T) *Server {
	t.Helper()
	dsn := os.Getenv("CCTVSCAN_TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("explicit test DB required")
	}
	st, e := store.New(context.Background(), dsn)
	if e != nil {
		t.Fatal("test DB unavailable")
	}
	t.Cleanup(st.Close)
	if e = st.Migrate(context.Background()); e != nil {
		t.Fatal(e)
	}
	box, _ := secrets.New(make([]byte, 32))
	s, e := New(Options{Store: st, Secrets: box, Auth: auth.Config{Mode: "local"}, PublicOrigin: "http://localhost:14904"})
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func request(t *testing.T, s *Server, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw := []byte{}
	if body != nil {
		raw, _ = json.Marshal(body)
	}
	r := httptest.NewRequest(method, "http://localhost:14904"+path, strings.NewReader(string(raw)))
	r.RemoteAddr = "127.0.0.1:5000"
	r.Header.Set("Origin", "http://localhost:14904")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}
func TestJobLifecycleHTTP(t *testing.T) {
	s := apiFixture(t)
	c := config.DefaultConfig()
	c.Targets = []string{"192.0.2.1"}
	key := uuid.NewString()
	body := map[string]any{"name": "http-" + key, "config": c, "priority": 2, "idempotency_key": key}
	w := request(t, s, "POST", "/api/jobs", body)
	if w.Code != 201 {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	var j store.JobDTO
	if e := json.Unmarshal(w.Body.Bytes(), &j); e != nil {
		t.Fatal(e)
	}
	w = request(t, s, "POST", "/api/jobs", body)
	if w.Code != 201 || !strings.Contains(w.Body.String(), j.ID) {
		t.Fatal("idempotency", w.Code, w.Body.String())
	}
	body["name"] = "conflict"
	if w = request(t, s, "POST", "/api/jobs", body); w.Code != 409 {
		t.Fatal("conflict", w.Code, w.Body.String())
	}
	if w = request(t, s, "DELETE", "/api/jobs/"+j.ID, nil); w.Code != 409 {
		t.Fatal("live delete", w.Code, w.Body.String())
	}
	if w = request(t, s, "POST", "/api/jobs/"+j.ID+"/cancel", nil); w.Code != 200 || !strings.Contains(w.Body.String(), `"state":"canceled"`) {
		t.Fatal("cancel", w.Code, w.Body.String())
	}
	if w = request(t, s, "GET", "/api/jobs?search=http-"+key, nil); w.Code != 200 || !strings.Contains(w.Body.String(), `"total":1`) {
		t.Fatal("list", w.Code, w.Body.String())
	}
	if w = request(t, s, "GET", "/api/jobs/"+j.ID+"/results", nil); w.Code != 200 || !strings.Contains(w.Body.String(), `"items":[]`) {
		t.Fatal("results", w.Code, w.Body.String())
	}
	if w = request(t, s, "DELETE", "/api/jobs/"+j.ID, nil); w.Code != 204 {
		t.Fatal("terminal delete", w.Code, w.Body.String())
	}
}
