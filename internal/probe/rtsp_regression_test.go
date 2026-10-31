package probe

import (
	"context"
	"io"
	"net"
	"strconv"
	"sync/atomic"
	"testing"
)

func TestAuditRTSPHonorsCanceledContext(t *testing.T) {
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer l.Close()
	var hits atomic.Int32
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			c, e := l.Accept()
			if e != nil {
				return
			}
			hits.Add(1)
			io.WriteString(c, "RTSP/1.0 200 OK\r\nServer: fixture\r\n\r\n")
			c.Close()
		}
	}()
	_, p, _ := net.SplitHostPort(l.Addr().String())
	port, _ := strconv.Atoi(p)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ProbeRTSP(ctx, "127.0.0.1", []int{port})
	l.Close()
	<-done
	if hits.Load() > 0 {
		t.Errorf("already canceled RTSP probe still opened %d TCP connection", hits.Load())
	}
}
func TestAuditRTSPStatusVsAvailability(t *testing.T) {
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer l.Close()
	go func() {
		c, e := l.Accept()
		if e != nil {
			return
		}
		defer c.Close()
		io.WriteString(c, "RTSP/1.0 401 Unauthorized\r\nServer: fixture\r\n\r\n")
	}()
	_, p, _ := net.SplitHostPort(l.Addr().String())
	port, _ := strconv.Atoi(p)
	r := ProbeRTSP(context.Background(), "127.0.0.1", []int{port})
	if !r.Any {
		t.Error("RTSP 401 responder is discarded, not represented as RTSP present/auth required")
	}
}
