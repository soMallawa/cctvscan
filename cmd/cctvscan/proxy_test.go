package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestHTTPProxyCLI(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "cctvscan")
	if out, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	t.Setenv("CCTVSCAN_HTTP_PROXY", "http://secret-user:secret-password@bad:99999")
	t.Setenv("TEST_SELECTED_HTTP_PROXY", "http://127.0.0.1:12345")
	for _, tc := range []struct {
		name string
		args []string
		want string
		fail bool
	}{
		{"invalid default", nil, "invalid HTTP proxy configuration", true},
		{"selected overrides default", []string{"--http-proxy-env", "TEST_SELECTED_HTTP_PROXY", "--list-interfaces"}, "MAC", false},
		{"missing selection", []string{"--http-proxy-env", "MISSING_PROXY_FOR_TEST", "--list-interfaces"}, "missing or empty", true},
		{"help", []string{"--help"}, "http-proxy-env", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := exec.Command(binary, tc.args...).CombinedOutput()
			s := string(out)
			if (err != nil) != tc.fail || !strings.Contains(s, tc.want) || strings.Contains(s, "secret-") || strings.Contains(s, "Scanning ") {
				t.Fatalf("error=%v output=%s", err, s)
			}
		})
	}
}
