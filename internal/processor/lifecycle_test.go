package processor

import (
	"context"
	"fmt"
	"net"
	"testing"
)

func TestRTSPTransportFailureNotComplete(t *testing.T) {
	var l net.Listener
	var port int
	for _, p := range []int{1554, 2554, 3554, 4554, 5554} {
		v, e := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if e == nil {
			l = v
			port = p
			break
		}
	}
	if l == nil {
		t.Fatal("cannot allocate local RTSP fixture")
	}
	defer l.Close()
	go func() {
		c, e := l.Accept()
		if e == nil {
			c.Close()
		}
	}()
	p := NewOptimizedProcessor(false, "", t.TempDir())
	r := p.ProcessHosts(context.Background(), map[string][]int{"127.0.0.1": {port}})
	if len(r) != 1 || r[0].Error == nil || r[0].State != "inconclusive" {
		t.Fatalf("RTSP transport failure collapsed: %+v", r)
	}
}
