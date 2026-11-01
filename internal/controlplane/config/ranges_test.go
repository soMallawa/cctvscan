package config

import (
	"context"
	"github.com/postfix/cctvscan/internal/targets"
	"reflect"
	"testing"
)

func TestInclusiveIPv4Ranges(t *testing.T) {
	for _, input := range []string{"192.0.2.253-192.0.3.2", "192.0.2.253 - 192.0.3.2"} {
		c := validConfig()
		c.Targets = []string{input}
		c.MaxTargets = 6
		preview, err := Validate(context.Background(), c, DefaultPolicy())
		if err != nil {
			t.Fatal(err)
		}
		got, err := targets.Expand(preview.NormalizedTargets)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"192.0.2.253", "192.0.2.254", "192.0.2.255", "192.0.3.0", "192.0.3.1", "192.0.3.2"}
		if !reflect.DeepEqual(got, want) || preview.RequestedAddressUpperBound != uint64(len(want)) {
			t.Fatalf("scope mismatch: %+v, expanded=%v", preview, got)
		}
		c.MaxTargets = 5
		if _, err = Validate(context.Background(), c, DefaultPolicy()); err == nil {
			t.Fatal("range bypassed address ceiling")
		}
	}
}

func TestIPv4RangeBoundaries(t *testing.T) {
	for _, input := range []string{"192.0.2.2-192.0.2.1", "192.0.2.1-", "-192.0.2.1", "192.0.2.1-192.0.2.2-192.0.2.3", "::1-::2", "192.0.2.1-example.com"} {
		c := validConfig()
		c.Targets = []string{input}
		if _, err := Validate(context.Background(), c, DefaultPolicy()); err == nil {
			t.Fatalf("accepted invalid range %q", input)
		}
	}
	c := validConfig()
	c.Targets = []string{"255.255.255.255-255.255.255.255"}
	p, err := Validate(context.Background(), c, DefaultPolicy())
	if err != nil || p.RequestedAddressUpperBound != 1 {
		t.Fatalf("last IPv4 address: %+v %v", p, err)
	}
	c.Targets = []string{"0.0.0.0-255.255.255.255"}
	c.MaxTargets = uint64(1) << 32
	policy := DefaultPolicy()
	policy.MaxTargets = c.MaxTargets
	p, err = Validate(context.Background(), c, policy)
	if err != nil || p.RequestedAddressUpperBound != c.MaxTargets || !reflect.DeepEqual(p.NormalizedTargets, []string{"0.0.0.0/0"}) {
		t.Fatalf("full IPv4 range must stay compact: %+v %v", p, err)
	}
}
