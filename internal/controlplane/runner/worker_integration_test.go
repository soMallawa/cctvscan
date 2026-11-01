//go:build integration

package runner

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/postfix/cctvscan/internal/controlplane/config"
	"github.com/postfix/cctvscan/internal/controlplane/store"
	"github.com/riverqueue/river"
	"net/url"
	"os"
	"testing"
	"time"
)

func workerFixture(t *testing.T) *store.Store {
	t.Helper()
	ctx := context.Background()
	dsn := os.Getenv("CCTVSCAN_TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("explicit disposable database required")
	}
	conn, e := pgx.Connect(ctx, dsn)
	if e != nil {
		t.Fatal("fixture unavailable")
	}
	name := "cctvscan_worker_" + uuid.NewString()[:8]
	ident := pgx.Identifier{name}.Sanitize()
	if _, e = conn.Exec(ctx, "CREATE DATABASE "+ident); e != nil {
		t.Fatal("cannot create isolated worker database")
	}
	u, _ := url.Parse(dsn)
	u.Path = "/" + name
	s, e := store.New(ctx, u.String())
	if e != nil {
		t.Fatal("cannot open isolated worker database")
	}
	t.Cleanup(func() { s.Close(); conn.Exec(ctx, "DROP DATABASE "+ident+" WITH (FORCE)"); conn.Close(ctx) })
	if e = s.Migrate(ctx); e != nil {
		t.Fatal(e)
	}
	return s
}
func TestQueueExecutesPersistedFixtureJob(t *testing.T) {
	st := workerFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c := config.DefaultConfig()
	c.Targets = []string{"127.0.0.1"}
	j, e := st.CreateJob(ctx, store.CreateJobInput{Name: "queued fixture", Config: c, Policy: config.DefaultPolicy(), Priority: 2})
	if e != nil {
		t.Fatal(e)
	}
	binary := script(t, `printf '%s' '{"SchemaVersion":1,"State":"complete","Results":[]}' > scan-queue.json
`)
	w := &Worker{Store: st, Binary: binary, Root: t.TempDir(), RunnerID: uuid.NewString()}
	exited := make(chan error, 1)
	go func() { exited <- w.Run(ctx) }()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case e := <-exited:
			t.Fatal("queue worker exited before completion", e)
		case <-ctx.Done():
			t.Fatal("queued job never completed")
		case <-ticker.C:
			saved, e := st.GetJob(ctx, j.ID)
			if e != nil {
				t.Fatal(e)
			}
			if saved.State == "completed" {
				cancel()
				select {
				case e := <-exited:
					if e != nil {
						t.Fatal(e)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("worker did not shut down")
				}
				return
			}
		}
	}
}
func TestWorkerImportsFixtureReportDurably(t *testing.T) {
	st := workerFixture(t)
	ctx := context.Background()
	c := config.DefaultConfig()
	c.Targets = []string{"127.0.0.1"}
	j, e := st.CreateJob(ctx, store.CreateJobInput{Name: "owned loopback fixture", Config: c, Policy: config.DefaultPolicy(), Priority: 2})
	if e != nil {
		t.Fatal(e)
	}
	rid := uuid.NewString()
	if e = st.RegisterRunner(ctx, rid); e != nil {
		t.Fatal(e)
	}
	binary := script(t, `printf '%s' '{"SchemaVersion":1,"State":"complete","Results":[{"Host":"127.0.0.1","Ports":[8080],"Brand":"Fixture","State":"complete","CredentialState":"not_tested","password":"fixture-secret","CameraDetection":{"IsCamera":false},"StreamResults":[{"URL":"rtsp://fixture-user:fixture-secret@127.0.0.1/candidate?token=fixture-secret","State":"candidate","Verified":false}]}]}' > scan-fixture.json
`)
	w := &Worker{Store: st, Binary: binary, Root: t.TempDir(), RunnerID: rid}
	if e = w.Work(ctx, &river.Job[store.ScanJobArgs]{Args: store.ScanJobArgs{JobID: j.ID}}); e != nil {
		t.Fatal(e)
	}
	done, e := st.GetJob(ctx, j.ID)
	if e != nil {
		t.Fatal(e)
	}
	if done.State != "completed" || done.ResultCount != 1 || done.CameraCount != 0 || done.StreamCount != 0 {
		t.Fatalf("wrong durable outcome: state=%s hosts=%d cameras=%d streams=%d", done.State, done.ResultCount, done.CameraCount, done.StreamCount)
	}
	rows, total, e := st.Results(ctx, j.ID, "", "", 50, 0)
	if e != nil || total != 1 {
		t.Fatal("no durable evidence", e)
	}
	var obj map[string]any
	if e = json.Unmarshal(rows[0], &obj); e != nil {
		t.Fatal(e)
	}
	data := obj["result"].(map[string]any)
	if _, ok := data["password"]; ok {
		t.Fatal("worker persisted plaintext secret")
	}
	stream := data["StreamResults"].([]any)[0].(map[string]any)
	if stream["URL"] != "rtsp://127.0.0.1/candidate" {
		t.Fatal("worker retained URL secret")
	}
	if e = w.Work(ctx, &river.Job[store.ScanJobArgs]{Args: store.ScanJobArgs{JobID: j.ID}}); e != nil {
		t.Fatal("completed replay should acknowledge without scanning", e)
	}
}
