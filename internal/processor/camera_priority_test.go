package processor

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestPossibleEvidenceOutranksUnrecognisedEndpoint(t *testing.T) {
	var candidatePort int
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, port, _ := net.SplitHostPort(r.Host)
		p, _ := strconv.Atoi(port)
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if p == candidatePort {
			_, _ = w.Write([]byte("<title>Web Client</title>"))
		} else {
			_, _ = w.Write([]byte("<title>API service</title>"))
		}
	})
	a := httptest.NewServer(handler)
	defer a.Close()
	b := httptest.NewServer(handler)
	defer b.Close()
	_, ap, _ := net.SplitHostPort(a.Listener.Addr().String())
	p1, _ := strconv.Atoi(ap)
	_, bp, _ := net.SplitHostPort(b.Listener.Addr().String())
	p2, _ := strconv.Atoi(bp)
	candidatePort = max(p1, p2)
	p := NewOptimizedProcessor(false, "", t.TempDir())
	r := p.processHost(context.Background(), "127.0.0.1", []int{p1, p2})
	if r.CameraDetection.State != "possible" || r.CameraDetection.PageTitle != "Web Client" {
		t.Fatalf("weaker first endpoint hid a review candidate: %+v", r.CameraDetection)
	}
}
