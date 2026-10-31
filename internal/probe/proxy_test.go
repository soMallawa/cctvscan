package probe

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/postfix/cctvscan/internal/httptransport"
)

func TestMetadataAndLoginRequestsUseConfiguredProxy(t *testing.T) {
	var direct, proxied atomic.Int32
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { direct.Add(1) }))
	defer origin.Close()
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxied.Add(1)
		w.Header().Set("Server", "fixture")
		w.Write([]byte("fixture"))
	}))
	defer proxy.Close()
	if err := httptransport.Configure(proxy.URL); err != nil {
		t.Fatal(err)
	}
	defer httptransport.Configure("")
	u, _ := url.Parse(origin.URL)
	host, port, _ := net.SplitHostPort(u.Host)
	p, _ := strconv.Atoi(port)
	meta := ProbeHTTPMeta(context.Background(), host, []int{p})
	pages := FindLoginPages(context.Background(), host, []int{p})
	optimized := OptimizedLoginPageFinder(context.Background(), host, []int{p})
	if meta.Server != "fixture" || len(pages) == 0 || len(optimized) == 0 || proxied.Load() == 0 || direct.Load() != 0 {
		t.Fatalf("meta=%+v pages=%d optimized=%d proxy=%d direct=%d", meta, len(pages), len(optimized), proxied.Load(), direct.Load())
	}
}
