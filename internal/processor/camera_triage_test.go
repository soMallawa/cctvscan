package processor

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func TestPossibleCameraReachesBoundedHTTPMediaDiscovery(t *testing.T) {
	var mediaRequests atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<title>Web Client</title>"))
			return
		}
		mediaRequests.Add(1)
		http.NotFound(w, r)
	}))
	defer s.Close()
	_, port, _ := net.SplitHostPort(s.Listener.Addr().String())
	pnum, _ := strconv.Atoi(port)
	p := NewOptimizedProcessor(false, "", t.TempDir())
	p.Options.HostTimeout = 5 * time.Second
	result := p.processHost(context.Background(), "127.0.0.1", []int{pnum})
	if mediaRequests.Load() == 0 {
		t.Fatal("generic Web Client candidate was gated out of deeper HTTP media checks")
	}
	if result.CameraDetection.State != "possible" || result.CameraDetection.IsCamera {
		t.Fatalf("unverified candidate was promoted: %+v", result.CameraDetection)
	}
}

func TestCanceledHostHasUnknownCameraDecision(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := NewOptimizedProcessor(false, "", t.TempDir())
	r := p.processHost(ctx, "127.0.0.1", []int{8080})
	if r.CameraDetection.State != "unknown" || r.CameraDetection.EvidenceVersion != 1 {
		t.Fatalf("failed probe is not explicitly unknown: %+v", r.CameraDetection)
	}
}
