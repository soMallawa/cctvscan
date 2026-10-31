package processor

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
)

func TestCanceledHostsHaveExplicitOutcome(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := NewOptimizedProcessor(false, "", t.TempDir())
	r := p.ProcessHosts(ctx, map[string][]int{"fixture.invalid": {80}})
	if len(r) != 1 || !errors.Is(r[0].Error, context.Canceled) {
		t.Fatalf("cancellation lost: %+v", r)
	}
}
func TestNeutralEndpointDoesNotTriggerRepeatedDiscovery(t *testing.T) {
	var hits atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1); io.WriteString(w, "public brochure") }))
	defer s.Close()
	host, port, _ := net.SplitHostPort(s.Listener.Addr().String())
	pn, _ := strconv.Atoi(port)
	p := NewOptimizedProcessor(false, "", t.TempDir())
	p.ProcessHosts(context.Background(), map[string][]int{host: {pn}})
	if n := hits.Load(); n != 1 {
		t.Fatalf("neutral endpoint requested %d times instead of once", n)
	}
}
