package probe

import (
	"context"
	"github.com/postfix/cctvscan/internal/httptransport"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func auditProxy(t *testing.T, h http.HandlerFunc) {
	t.Helper()
	s := httptest.NewServer(h)
	if e := httptransport.Configure(s.URL); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { httptransport.Configure(""); s.Close() })
}
func TestAuditConcurrencyBound(t *testing.T) {
	var active, peak atomic.Int32
	auditProxy(t, func(w http.ResponseWriter, r *http.Request) {
		a := active.Add(1)
		for {
			old := peak.Load()
			if a <= old || peak.CompareAndSwap(old, a) {
				break
			}
		}
		time.Sleep(40 * time.Millisecond)
		active.Add(-1)
		w.WriteHeader(404)
	})
	FindMJPEGPaths(context.Background(), "fixture.invalid", []int{80, 81, 82, 83, 84})
	t.Logf("5 port slots yielded peak simultaneous HTTP requests=%d; paths per port=%d", peak.Load(), len(MJPEGPaths))
	if peak.Load() > 5 {
		t.Errorf("port semaphore does not enforce global HTTP concurrency: peak=%d", peak.Load())
	}
}
