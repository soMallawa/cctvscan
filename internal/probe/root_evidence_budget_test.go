package probe

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestRootEvidenceHasPerHostBudget(t *testing.T) {
	ports := []int{}
	for range 5 {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(strings.Repeat("x", 9000))) }))
		defer s.Close()
		_, port, _ := net.SplitHostPort(s.Listener.Addr().String())
		p, _ := strconv.Atoi(port)
		ports = append(ports, p)
	}
	m := ProbeHTTPMeta(context.Background(), "127.0.0.1", ports)
	total := 0
	for _, e := range m.Endpoints {
		if e.RootEvidence != nil {
			total += e.RootEvidence.CapturedBytes
		}
	}
	if total > 32768 {
		t.Fatalf("root evidence escaped host budget: %d bytes", total)
	}
	if len(m.Endpoints) != 5 || m.Endpoints[4].RootEvidence != nil {
		t.Fatal("extra response must still be probed without exceeding archive budget")
	}
}
