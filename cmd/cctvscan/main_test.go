package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRoutingCLIExitsWithoutScanning(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "cctvscan")
	if out, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	for name, body := range map[string]string{
		"ip": `#!/bin/sh
case "$*" in
 *"route get"*) printf '%s' '[{"dev":"eth9","gateway":"192.0.2.1"}]' ;;
 *"neigh show"*) printf '%s' '[{"dst":"192.0.2.1","lladdr":"02:00:00:00:00:09","state":["REACHABLE"]}]' ;;
 *) exit 2 ;;
esac
`,
		"masscan": "#!/bin/sh\ntouch \"$TEST_SCANNER_MARKER\"\nexit 91\n",
		"naabu":   "#!/bin/sh\ntouch \"$TEST_SCANNER_MARKER\"\nexit 91\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	marker := filepath.Join(dir, "scanner-started")
	t.Setenv("TEST_SCANNER_MARKER", marker)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	cases := []struct {
		name string
		args []string
		want string
		fail bool
	}{
		{"route check", []string{"--masscan-route-check", "--masscan-route-target", "198.51.100.10"}, `"Adapter": "eth9"`, false},
		{"explicit check", []string{"--masscan-route-check", "--interface", "eth7", "--router-ip", "192.0.2.1", "--router-mac", "02:00:00:00:00:07"}, `"RouterMAC": "02:00:00:00:00:07"`, false},
		{"alias conflict", []string{"--masscan-route-check", "--interface", "eth7", "--adapter", "eth8"}, "conflict", true},
		{"invalid gateway", []string{"--masscan-route-check", "--router-ip", "bad"}, "IPv4", true},
		{"interfaces", []string{"--list-interfaces"}, `"MAC":`, false},
		{"diagnostic ignores scan target", []string{"--masscan-route-check", "--masscan-route-target", "198.51.100.10", "198.51.100.11"}, `"Adapter": "eth9"`, false},
		{"diagnostic exits before target expansion", []string{"--masscan-route-check", "--interface", "eth7", "not-an-ip"}, `"Adapter": "eth7"`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := exec.Command(binary, tc.args...).CombinedOutput()
			if (err != nil) != tc.fail || !strings.Contains(string(out), tc.want) || strings.Contains(string(out), "Scanning ") {
				t.Fatalf("error=%v output=%s, wanted %q", err, out, tc.want)
			}
		})
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("diagnostic command started scanner")
	}
}
