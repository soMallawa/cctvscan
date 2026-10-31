package targets

import (
	"context"
	"testing"
)

func TestCompactRangesAndCanceledExpansion(t *testing.T) {
	r, e := Normalize(context.Background(), []string{"198.18.0.0/16"})
	if e != nil || len(r) != 1 || r[0] != "198.18.0.0/16" {
		t.Fatalf("compact range: %v %v", r, e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = ExpandContext(ctx, []string{"198.18.0.0/16"}); e != context.Canceled {
		t.Fatalf("expected cancellation, got %v", e)
	}
}
func TestZeroPrefixExpansionFailsBoundedly(t *testing.T) {
	if _, e := ExpandContext(context.Background(), []string{"0.0.0.0/0"}); e == nil {
		t.Fatal("unbounded expansion permitted")
	}
}
