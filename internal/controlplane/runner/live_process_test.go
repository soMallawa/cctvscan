//go:build linux

package runner

import (
	"context"
	"encoding/json"
	"github.com/postfix/cctvscan/internal/controlplane/config"
	"testing"
	"time"
)

func TestProcessDeliversObservationBeforeScannerExit(t *testing.T) {
	binary := script(t, `printf '%s\n' '{"schema_version":1,"seq":"1","kind":"phase","data":{"phase":"host_processing"}}' >&3
sleep 30
`)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	seen := make(chan json.RawMessage, 1)
	done := make(chan error, 1)
	p := Process{Binary: binary, Directory: t.TempDir(), OnEvent: func(_ context.Context, raw json.RawMessage) error { seen <- raw; return nil }}
	c := config.DefaultConfig()
	c.Targets = []string{"127.0.0.1"}
	go func() { done <- p.Run(ctx, c) }()
	select {
	case raw := <-seen:
		var e struct {
			Kind string `json:"kind"`
		}
		if json.Unmarshal(raw, &e) != nil || e.Kind != "phase" {
			t.Fatal("invalid live event")
		}
	case e := <-done:
		t.Fatalf("scanner exited before delivering live observation: %v", e)
	case <-time.After(time.Second):
		t.Fatal("live observation was withheld until exit")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("observer prevented bounded cancellation")
	}
}
