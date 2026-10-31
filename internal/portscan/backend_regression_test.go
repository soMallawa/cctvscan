package portscan

import (
	"context"
	"errors"
	"net"
	"reflect"
	"sort"
	"strconv"
	"testing"
	"time"
)

func TestNaabuExactPairLocalhost(t *testing.T) {
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer l.Close()
	go func() {
		for {
			c, e := l.Accept()
			if e != nil {
				return
			}
			c.Close()
		}
	}()
	_, p, _ := net.SplitHostPort(l.Addr().String())
	port, _ := strconv.Atoi(p)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s := NewNaabuScanner(NaabuConfig{Rate: 100, Unprivileged: true, Threads: 1, Timeout: 500 * time.Millisecond})
	r, e := s.VerifyPorts(ctx, map[string][]int{"127.0.0.1": {port}})
	if e != nil || len(r["127.0.0.1"]) != 1 || r["127.0.0.1"][0] != port {
		t.Fatalf("open pair disappeared: %v %v", r, e)
	}
}
func TestUnprivilegedNeverUsesMasscan(t *testing.T) {
	s := NewHybridScanner(HybridConfig{Unprivileged: true, Rate: 20})
	s.discover = func(ctx context.Context, backend string, targets []string) (map[string][]int, error) {
		if backend != "naabu" {
			t.Errorf("unprivileged routed to %s", backend)
		}
		return map[string][]int{}, nil
	}
	if _, err := s.Scan(context.Background(), []string{"198.51.100.10"}); err != nil {
		t.Fatal(err)
	}
}
func TestVerificationErrorsAreNotSuccess(t *testing.T) {
	s := NewHybridScanner(HybridConfig{Rate: 20})
	s.discover = func(context.Context, string, []string) (map[string][]int, error) {
		return map[string][]int{"198.51.100.10": {80}}, nil
	}
	s.verify = func(context.Context, map[string][]int) (map[string][]int, error) {
		return nil, errors.New("fixture failure")
	}
	if _, e := s.Scan(context.Background(), []string{"198.51.100.10"}); e == nil {
		t.Fatal("verification failure silently accepted")
	}
}
func TestVerificationExactPairsAndSingleRateReduction(t *testing.T) {
	s := NewNaabuScanner(NaabuConfig{Rate: 100})
	var observed []string
	s.run = func(ctx context.Context, cfg NaabuConfig, targets []string) (map[string][]int, error) {
		if cfg.Ports == "" {
			t.Error("empty port override enables unwanted default-port scans")
		}
		for _, h := range targets {
			observed = append(observed, h+":"+cfg.Ports)
		}
		if cfg.Rate != 100 {
			t.Errorf("rate halved again: %d", cfg.Rate)
		}
		return map[string][]int{}, nil
	}
	_, e := s.VerifyPorts(context.Background(), map[string][]int{"198.51.100.10": {80}, "198.51.100.11": {443}})
	sort.Strings(observed)
	if !reflect.DeepEqual(observed, []string{"198.51.100.10:80", "198.51.100.11:443"}) {
		t.Errorf("verification scope widened: %v", observed)
	}
	if e != nil {
		t.Fatal(e)
	}
}
