//go:build integration && scanner_e2e

package runner

import (
	"context"
	"github.com/postfix/cctvscan/internal/controlplane/config"
	"github.com/postfix/cctvscan/internal/controlplane/store"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// This gate uses exactly one explicitly bound localhost HTTP fixture. It does
// not enable credentials, ONVIF, capture, raw scanning, or external targets.
func TestRealScannerThroughDurableQueue(t *testing.T) {
	binary := os.Getenv("CCTVSCAN_TEST_SCANNER_BIN")
	if !filepath.IsAbs(binary) {
		t.Fatal("explicit freshly built scanner binary required")
	}
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("Owned ordinary web fixture; not a camera."))
	}))
	defer fixture.Close()
	u, _ := url.Parse(fixture.URL)
	st := workerFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
	c := config.DefaultConfig()
	c.Targets = []string{"127.0.0.1"}
	c.PortMode = "custom"
	c.Ports = u.Port()
	c.MaxTargets = 1
	c.Retry = 1
	c.Wait = 1
	c.Threads = 1
	c.HostWorkers = 1
	c.HostTimeoutSeconds = 5
	c.TimeoutSeconds = 25
	c.HTTPRPS = 0
	j, e := st.CreateJob(ctx, store.CreateJobInput{Name: "real scanner / owned loopback fixture", Config: c, Policy: config.DefaultPolicy(), Priority: 2})
	if e != nil {
		t.Fatal(e)
	}
	w := &Worker{Store: st, Binary: binary, Root: t.TempDir(), LibraryPath: os.Getenv("LD_LIBRARY_PATH")}
	exited := make(chan error, 1)
	go func() { exited <- w.Run(ctx) }()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case e := <-exited:
			t.Fatal("worker stopped before terminal record", e)
		case <-ctx.Done():
			t.Fatal("loopback job did not terminate")
		case <-ticker.C:
			saved, e := st.GetJob(ctx, j.ID)
			if e != nil {
				t.Fatal(e)
			}
			if !store.Terminal(saved.State) {
				continue
			}
			cancel()
			if e = <-exited; e != nil {
				t.Fatal(e)
			}
			if saved.State != "completed" || saved.ResultCount != 1 || saved.CameraCount != 0 || saved.StreamCount != 0 {
				t.Fatalf("unexpected fixture outcome: state=%s results=%d camera=%d media=%d", saved.State, saved.ResultCount, saved.CameraCount, saved.StreamCount)
			}
			items, total, e := st.Results(context.Background(), j.ID, "", "", 10, 0)
			if e != nil || total != 1 || len(items) != 1 {
				t.Fatal("real scanner output was not persisted", e)
			}
			t.Log("Real localhost scanner -> River -> fenced attempt -> sanitized durable result: PASS")
			return
		}
	}
}
