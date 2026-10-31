package httptransport_test

import (
	"context"
	"github.com/postfix/cctvscan/internal/httptransport"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestBudgetCountsRedirectRequests(t *testing.T) {
	httptransport.ConfigureLimits(5, 2, 10)
	defer httptransport.ConfigureLimits(5, 2, 0)
	var mu sync.Mutex
	var seen []time.Time
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, time.Now())
		mu.Unlock()
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/done", http.StatusFound)
		}
	}))
	defer s.Close()
	req, _ := http.NewRequestWithContext(context.Background(), "GET", s.URL+"/start", nil)
	resp, e := httptransport.Do(s.Client(), req)
	if e != nil {
		t.Fatal(e)
	}
	resp.Body.Close()
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 2 || seen[1].Sub(seen[0]) < 75*time.Millisecond {
		t.Fatalf("redirect bypassed rate budget: %v", seen)
	}
}
