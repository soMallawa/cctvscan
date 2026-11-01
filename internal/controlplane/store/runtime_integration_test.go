//go:build integration

package store

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/postfix/cctvscan/internal/controlplane/config"
	"net/url"
	"os"
	"testing"
)

func runtimeFixture(t *testing.T) *Store {
	t.Helper()
	ctx := context.Background()
	dsn := os.Getenv("CCTVSCAN_TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("explicit disposable DB required")
	}
	conn, e := pgx.Connect(ctx, dsn)
	if e != nil {
		t.Fatal("fixture connection unavailable")
	}
	name := "cctvscan_runtime_" + uuid.NewString()[:8]
	ident := pgx.Identifier{name}.Sanitize()
	if _, e = conn.Exec(ctx, "CREATE DATABASE "+ident); e != nil {
		conn.Close(ctx)
		t.Fatal("isolated runtime database could not be created")
	}
	u, e := url.Parse(dsn)
	if e != nil {
		t.Fatal("fixture DSN invalid")
	}
	u.Path = "/" + name
	s, e := New(ctx, u.String())
	if e != nil {
		t.Fatal("isolated runtime database unavailable")
	}
	t.Cleanup(func() { s.Close(); conn.Exec(ctx, "DROP DATABASE "+ident+" WITH (FORCE)"); conn.Close(ctx) })
	if e = s.Migrate(ctx); e != nil {
		t.Fatal(e)
	}
	return s
}
func TestTightenedPolicyRejectsQueuedSnapshotDurably(t *testing.T) {
	s := runtimeFixture(t)
	ctx := context.Background()
	rid := uuid.NewString()
	if e := s.RegisterRunner(ctx, rid); e != nil {
		t.Fatal(e)
	}
	in := input()
	in.Config.Targets = []string{"127.0.0.1", "127.0.0.2"}
	j, e := s.CreateJob(ctx, in)
	if e != nil {
		t.Fatal(e)
	}
	policy := config.DefaultPolicy()
	policy.MaxTargets = 1
	raw, _ := json.Marshal(policy)
	if _, e = s.pool.Exec(ctx, `UPDATE settings SET policy=$1 WHERE singleton`, raw); e != nil {
		t.Fatal(e)
	}
	if _, e = s.BeginAttempt(ctx, j.ID, rid); !errors.Is(e, ErrInvalidInput) {
		t.Fatal("current policy was ignored", e)
	}
	saved, e := s.GetJob(ctx, j.ID)
	if e != nil || saved.State != "failed" || saved.Attempt != 0 {
		t.Fatal("rejected queued job left without a terminal outcome", e)
	}
}
func TestLostRunnerIsOrphanedWithoutRetry(t *testing.T) {
	s := runtimeFixture(t)
	ctx := context.Background()
	rid := uuid.NewString()
	if e := s.RegisterRunner(ctx, rid); e != nil {
		t.Fatal(e)
	}
	j, e := s.CreateJob(ctx, input())
	if e != nil {
		t.Fatal(e)
	}
	a, e := s.BeginAttempt(ctx, j.ID, rid)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.pool.Exec(ctx, `UPDATE runners SET heartbeat_at=now()-interval '1 minute' WHERE id=$1`, rid); e != nil {
		t.Fatal(e)
	}
	if e = s.RecoverOrphans(ctx); e != nil {
		t.Fatal(e)
	}
	saved, e := s.GetJob(ctx, j.ID)
	if e != nil || saved.State != "orphaned" {
		t.Fatal("lost execution was not recorded as uncertain", e)
	}
	if _, e = s.BeginAttempt(ctx, j.ID, rid); !errors.Is(e, ErrFenced) {
		t.Fatal("lost execution automatically restarted", e)
	}
	if e = s.FinishAttempt(ctx, a, "completed"); !errors.Is(e, ErrFenced) {
		t.Fatal("lost worker overwrote orphan outcome", e)
	}
}
func TestAttemptReservationAndFencing(t *testing.T) {
	s := runtimeFixture(t)
	ctx := context.Background()
	rid := uuid.NewString()
	if e := s.RegisterRunner(ctx, rid); e != nil {
		t.Fatal(e)
	}
	j, e := s.CreateJob(ctx, input())
	if e != nil {
		t.Fatal(e)
	}
	a, e := s.BeginAttempt(ctx, j.ID, rid)
	if e != nil {
		t.Fatal("cannot claim persisted job", e)
	}
	if a.Job.State != "running" || a.ID == "" || a.Token == "" {
		t.Fatal("attempt did not acquire durable execution identity")
	}
	if _, e = s.BeginAttempt(ctx, j.ID, rid); !errors.Is(e, ErrFenced) {
		t.Fatal("duplicate claim accepted", e)
	}
	second, e := s.CreateJob(ctx, input())
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.BeginAttempt(ctx, second.ID, rid); !errors.Is(e, ErrCapacity) {
		t.Fatal("global execution budget ignored", e)
	}
	forged := a
	forged.Token = uuid.NewString()
	if e = s.FinishAttempt(ctx, forged, "completed"); !errors.Is(e, ErrFenced) {
		t.Fatal("stale write accepted", e)
	}
	if e = s.Phase(ctx, a, "scanning"); e != nil {
		t.Fatal(e)
	}
	progress, e := s.GetJob(ctx, j.ID)
	if e != nil || progress.Phase != "scanning" {
		t.Fatal("phase did not persist", e)
	}
	data := []ResultInput{{Target: "127.0.0.1", Brand: "Fixture", State: "inconclusive", Data: []byte(`{"CredentialState":"inconclusive"}`), Camera: true, Streams: 1, Inconclusive: true}}
	if e = s.SaveResults(ctx, a, data); e != nil {
		t.Fatal(e)
	}
	if e = s.SaveResults(ctx, a, data); e != nil {
		t.Fatal(e)
	}
	progress, e = s.GetJob(ctx, j.ID)
	if e != nil || progress.ResultCount != 1 || progress.CameraCount != 1 || progress.StreamCount != 1 || progress.InconclusiveCount != 1 {
		t.Fatal("result import is not idempotent", e)
	}
	if e = s.SaveResults(ctx, forged, data); !errors.Is(e, ErrFenced) {
		t.Fatal("stale import accepted", e)
	}
	if _, e = s.CancelQueued(ctx, j.ID); e != nil {
		t.Fatal(e)
	}
	if e = s.FinishAttempt(ctx, a, "completed"); e != nil {
		t.Fatal(e)
	}
	done, e := s.GetJob(ctx, j.ID)
	if e != nil {
		t.Fatal(e)
	}
	if done.State != "canceled" {
		t.Fatal("cancellation lost at completion", done.State)
	}
	if e = s.FinishAttempt(ctx, a, "completed"); !errors.Is(e, ErrFenced) {
		t.Fatal("terminal attempt rewritten", e)
	}
	if _, e = s.BeginAttempt(ctx, second.ID, rid); e != nil {
		t.Fatal("capacity was not released", e)
	}
}
