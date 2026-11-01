//go:build integration

package api

import (
	"bufio"
	"context"
	"encoding/json"
	"github.com/postfix/cctvscan/internal/controlplane/config"
	"github.com/postfix/cctvscan/internal/controlplane/store"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSSEGlobalReplay(t *testing.T) {
	s := apiFixture(t)
	j, e := s.options.Store.CreateJob(context.Background(), store.CreateJobInput{Name: "sse", Config: func() config.ScanConfig { c := config.DefaultConfig(); c.Targets = []string{"192.0.2.1"}; return c }(), Policy: config.DefaultPolicy(), Priority: 2})
	if e != nil {
		t.Fatal(e)
	}
	a, e := s.options.Store.AddEvent(context.Background(), j.ID, "log", json.RawMessage(`{"message":"safe first"}`))
	if e != nil {
		t.Fatal(e)
	}
	b, e := s.options.Store.AddEvent(context.Background(), j.ID, "log", json.RawMessage(`{"message":"safe second"}`))
	if e != nil {
		t.Fatal(e)
	}
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+"/api/events?after="+strconv.FormatInt(a.Seq, 10), nil)
	res, e := http.DefaultClient.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("SSE status=%d", res.StatusCode)
	}
	scanner := bufio.NewScanner(res.Body)
	found := false
	for scanner.Scan() {
		line := scanner.Text()
		if line == "id: "+strconv.FormatInt(b.Seq, 10) {
			found = true
		}
		if strings.HasPrefix(line, "data: ") {
			if !found || !strings.Contains(line, "safe second") || !strings.Contains(line, `"at":`) {
				t.Fatalf("bad replay %s", line)
			}
			break
		}
	}
	if !found {
		t.Fatal("missing replay event")
	}
	for _, path := range []string{"/api/events?after=-1", "/api/events?after=NaN", "/api/jobs/not-an-id/events"} {
		w := request(t, s, "GET", path, nil)
		if w.Code != 400 {
			t.Fatal("invalid cursor/id", w.Code)
		}
	}
}
