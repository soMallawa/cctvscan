package processor

import (
	"context"
	"sync"
	"testing"
)

func TestObserverReportsEveryFinalHostWithoutChangingResults(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := NewOptimizedProcessor(false, "", t.TempDir())
	var mu sync.Mutex
	seen := map[string]HostResult{}
	p.Options.Observer = func(r HostResult) { mu.Lock(); defer mu.Unlock(); seen[r.Host] = r }
	out := p.ProcessHosts(ctx, map[string][]int{"127.0.0.1": {8080}, "127.0.0.2": {8081}})
	if len(out) != 2 || len(seen) != 2 {
		t.Fatalf("results=%d observations=%d", len(out), len(seen))
	}
	for _, r := range out {
		if r.State != "canceled" || seen[r.Host].State != r.State {
			t.Fatal("observer changed the outcome")
		}
	}
}
