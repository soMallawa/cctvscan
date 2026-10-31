package portscan

import (
	"github.com/projectdiscovery/naabu/v2/pkg/runner"
	"testing"
)

func TestNaabuModeConstants(t *testing.T) {
	if s := NewNaabuScanner(NaabuConfig{Unprivileged: true}).determineScanType(); s != runner.ConnectScan {
		t.Fatalf("want %s got %s", runner.ConnectScan, s)
	}
	if s := NewNaabuScanner(NaabuConfig{Privileged: true}).determineScanType(); s != runner.SynScan {
		t.Fatalf("want %s got %s", runner.SynScan, s)
	}
}
func TestExplicitAllPortsPreserved(t *testing.T) {
	s := NewMasscanScanner(MasscanConfig{Ports: "0-65535"})
	if p := s.getPortsToScan(); p != "0-65535" {
		t.Fatalf("explicit range changed: %s", p)
	}
}
