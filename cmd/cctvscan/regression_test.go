package main

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNonScanningPlanHonorsExplicitControls(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "cctvscan")
	if b, e := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); e != nil {
		t.Fatalf("build %v %s", e, b)
	}
	b, e := exec.Command(bin, "--plan", "--ports", "0-65535", "--unprivileged", "--rate", "4", "--wait", "7", "--retry", "2", "198.18.0.0/16").CombinedOutput()
	if e != nil {
		t.Fatalf("plan: %v %s", e, b)
	}
	var p struct {
		Targets    []string
		ScanConfig struct {
			Ports             string
			Wait, Retry, Rate int
			Unprivileged      bool
		}
	}
	if e = json.Unmarshal(b, &p); e != nil {
		t.Fatal(e)
	}
	if len(p.Targets) != 1 || p.ScanConfig.Ports != "0-65535" || p.ScanConfig.Wait != 7 || p.ScanConfig.Retry != 2 || p.ScanConfig.Rate != 4 || !p.ScanConfig.Unprivileged {
		t.Fatalf("controls changed: %s", b)
	}
	for _, args := range [][]string{{"--plan", "--rate", "0", "198.18.0.1"}, {"--plan", "--ports", "70000", "198.18.0.1"}, {"--plan", "--timeout", "0s", "198.18.0.1"}, {"--plan", "--privileged", "--unprivileged", "198.18.0.1"}} {
		b, e = exec.Command(bin, args...).CombinedOutput()
		if e == nil || strings.Contains(string(b), "Scanning ") {
			t.Fatalf("invalid input accepted %v: %s", args, b)
		}
	}
}
