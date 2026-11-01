package runner

import (
	"context"
	"errors"
	"github.com/postfix/cctvscan/internal/controlplane/config"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func script(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "fixture-scanner")
	if e := os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0700); e != nil {
		t.Fatal(e)
	}
	return p
}
func TestProcessIsolatesSecretsAndArguments(t *testing.T) {
	t.Setenv("HOST_PROVIDER_SECRET", "not-for-scanner")
	dir := t.TempDir()
	bin := script(t, `printf '%s\n' "$@" > args.txt
 env > environment.txt
 cp "$HOME/targets.txt" scope.txt
`)
	c := config.DefaultConfig()
	c.Targets = []string{"127.0.0.1"}
	c.ProxyVersionID = "owned-proxy"
	if e := (Process{Binary: bin, Directory: dir, ProxyURL: "http://fixture-user:fixture-password@127.0.0.1:19090"}).Run(context.Background(), c); e != nil {
		t.Fatal(e)
	}
	args, _ := os.ReadFile(filepath.Join(dir, "args.txt"))
	environment, _ := os.ReadFile(filepath.Join(dir, "environment.txt"))
	scope, _ := os.ReadFile(filepath.Join(dir, "scope.txt"))
	if !strings.Contains(string(args), "--unprivileged") || !strings.Contains(string(args), "--http-proxy-env") {
		t.Fatal("missing explicit scanner policy arguments")
	}
	if strings.Contains(string(args), "fixture-password") || strings.Contains(string(environment), "not-for-scanner") {
		t.Fatal("secret crossed process boundary")
	}
	if !strings.Contains(string(environment), "CCTVSCAN_JOB_HTTP_PROXY=http://fixture-user:fixture-password@127.0.0.1:19090") {
		t.Fatal("selected proxy not injected")
	}
	if string(scope) != "127.0.0.1\n" {
		t.Fatal("scope changed")
	}
}
func TestProcessCancellationStopsChildGroup(t *testing.T) {
	dir := t.TempDir()
	bin := script(t, `sleep 60 &
 kid=$!
 trap 'kill "$kid" 2>/dev/null; wait "$kid"; exit 0' TERM
 echo "$kid" > child.pid
 wait "$kid"
`)
	c := config.DefaultConfig()
	c.Targets = []string{"127.0.0.1"}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- (Process{Binary: bin, Directory: dir}).Run(ctx, c) }()
	var pid int
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		b, _ := os.ReadFile(filepath.Join(dir, "child.pid"))
		pid, _ = strconv.Atoi(strings.TrimSpace(string(b)))
		if pid > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pid == 0 {
		cancel()
		<-done
		t.Fatal("fixture did not start")
	}
	cancel()
	select {
	case e := <-done:
		if !errors.Is(e, context.Canceled) {
			t.Fatalf("cancellation lost: %v", e)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("scanner did not stop")
	}
	if e := syscall.Kill(pid, 0); e == nil {
		t.Fatal("grandchild survived cancellation")
	}
}
