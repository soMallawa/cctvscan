package httptransport

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type admissionTransport func(*http.Request) (*http.Response, error)

func (f admissionTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestBudgetQueuedAdmissionSpacing(t *testing.T) {
	for _, limit := range []string{"host", "global"} {
		t.Run(limit, func(t *testing.T) {
			global, host := 6, 3
			if limit == "global" {
				global, host = 3, 1
			}
			if err := ConfigureLimits(global, host, 25); err != nil {
				t.Fatal(err)
			}
			defer ConfigureLimits(5, 2, 0)
			starts := make(chan time.Time, 6)
			client := &http.Client{Transport: admissionTransport(func(r *http.Request) (*http.Response, error) {
				starts <- time.Now()
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("held")), Request: r}, nil
			})}
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			defer cancel()
			request := func(n int) (*http.Response, error) {
				hostname := "same.invalid"
				if limit == "global" {
					hostname = fmt.Sprintf("host%d.invalid", n)
				}
				req, _ := http.NewRequestWithContext(ctx, "GET", "http://"+hostname, nil)
				return Do(client, req)
			}
			var held []*http.Response
			for i := 0; i < 3; i++ {
				resp, err := request(i)
				if err != nil {
					t.Fatal(err)
				}
				held = append(held, resp)
				<-starts
			}
			defer func() {
				for _, resp := range held {
					resp.Body.Close()
				}
			}()
			done := make(chan error, 3)
			for i := 3; i < 6; i++ {
				go func(n int) {
					resp, err := request(n)
					if err == nil {
						resp.Body.Close()
					}
					done <- err
				}(i)
			}
			// All callers must reach the concurrency wait. In the old implementation
			// refs are acquired only after pacing deadlines, so this also waits past them.
			deadline := time.Now().Add(time.Second)
			for {
				budgets.RLock()
				b := budgets.b
				budgets.RUnlock()
				b.mu.Lock()
				refs := 0
				for _, h := range b.hosts {
					refs += h.refs
				}
				b.mu.Unlock()
				if refs == 6 {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("callers did not queue behind held body slots")
				}
				time.Sleep(time.Millisecond)
			}
			time.Sleep(80 * time.Millisecond)
			select {
			case <-starts:
				t.Fatal("body-lifetime cap bypassed")
			default:
			}
			for _, resp := range held {
				resp.Body.Close()
			}
			var previous time.Time
			for i := 0; i < 3; i++ {
				select {
				case start := <-starts:
					if !previous.IsZero() && start.Sub(previous) < 30*time.Millisecond {
						t.Errorf("queued actual RoundTrip starts only %v apart; want paced admission", start.Sub(previous))
					}
					previous = start
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
			}
			for i := 0; i < 3; i++ {
				if err := <-done; err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
