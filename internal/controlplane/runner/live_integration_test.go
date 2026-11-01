//go:build integration

package runner

import (
	"context"
	"github.com/google/uuid"
	"github.com/postfix/cctvscan/internal/controlplane/config"
	"github.com/postfix/cctvscan/internal/controlplane/store"
	"github.com/riverqueue/river"
	"strings"
	"testing"
	"time"
)

func TestNativeHostResultIsDurableWhileScannerRuns(t *testing.T) {
	st := workerFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	c := config.DefaultConfig()
	c.Targets = []string{"127.0.0.1"}
	j, e := st.CreateJob(ctx, store.CreateJobInput{Name: "live owned fixture", Config: c, Policy: config.DefaultPolicy(), Priority: 2})
	if e != nil {
		t.Fatal(e)
	}
	rid := uuid.NewString()
	if e = st.RegisterRunner(ctx, rid); e != nil {
		t.Fatal(e)
	}
	binary := script(t, `printf '%s\n' '{"schema_version":1,"seq":"1","kind":"phase","data":{"phase":"host_processing"}}' '{"schema_version":1,"seq":"2","kind":"host_result","data":{"Host":"127.0.0.1","Ports":[8080],"Brand":"Fixture","State":"complete","CredentialState":"not_tested","password":"private-fixture-secret","CameraDetection":{"IsCamera":false}}}' >&3
sleep 30
`)
	w := &Worker{Store: st, Binary: binary, Root: t.TempDir(), RunnerID: rid}
	done := make(chan error, 1)
	go func() { done <- w.Work(ctx, &river.Job[store.ScanJobArgs]{Args: store.ScanJobArgs{JobID: j.ID}}) }()
	t.Cleanup(func() {
		cancel()
		select {
		case e := <-done:
			if e != nil {
				t.Error(e)
			}
		case <-time.After(4 * time.Second):
			t.Error("fixture worker did not stop")
		}
	})
	deadline := time.After(2 * time.Second)
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-deadline:
			t.Fatal("host result was not durable while scanner was active")
		case <-tick.C:
			saved, e := st.GetJob(ctx, j.ID)
			if e != nil {
				t.Fatal(e)
			}
			if saved.ResultCount == 1 {
				if saved.State != "running" || saved.Phase != "host_processing" {
					t.Fatalf("live outcome misrepresented: %s/%s", saved.State, saved.Phase)
				}
				rows, n, e := st.Results(ctx, j.ID, "", "", 10, 0)
				if e != nil || n != 1 || strings.Contains(string(rows[0]), "private-fixture-secret") {
					t.Fatal("live evidence not sanitized/durable")
				}
				return
			}
		}
	}
}
