//go:build integration

package store

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/postfix/cctvscan/internal/controlplane/config"
	"os"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T) (*Store, string) {
	t.Helper()
	dsn := os.Getenv("CCTVSCAN_TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("CCTVSCAN_TEST_DATABASE_URL must explicitly name a disposable test database")
	}
	s, err := New(context.Background(), dsn)
	if err != nil {
		t.Fatal("database connection failed (details withheld)")
	}
	t.Cleanup(s.Close)
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return s, dsn
}
func input() CreateJobInput {
	c := config.DefaultConfig()
	c.Targets = []string{"192.0.2.1"}
	return CreateJobInput{Name: "fixture", Config: c, Policy: config.DefaultPolicy(), Priority: 2}
}

func TestConcurrentMigrations(t *testing.T) {
 s,_:=fixture(t)
 ctx,cancel:=context.WithTimeout(context.Background(),2*time.Second);defer cancel()
 // Hold the application lock until all pool slots are occupied by waiters.
 conn,err:=s.pool.Acquire(ctx);if err!=nil {t.Fatal(err)}
 if _,err=conn.Exec(ctx,`SELECT pg_advisory_lock(739250011)`);err!=nil {t.Fatal(err)}
 done:=make(chan error,5)
 for i:=0;i<5;i++ {go func(){done<-s.Migrate(ctx)}()}
 deadline:=time.NewTimer(200*time.Millisecond);<-deadline.C
 if _,err=conn.Exec(ctx,`SELECT pg_advisory_unlock(739250011)`);err!=nil {t.Fatal(err)}
 conn.Release()
 for i:=0;i<5;i++ {if err:=<-done;err!=nil {t.Fatal("concurrent migration failed",err)}}
}

func TestDurableCreate(t *testing.T) {
	ctx := context.Background()
	s, dsn := fixture(t)
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	in := input()
	in.IdempotencyKey = "test-" + time.Now().Format("20060102150405.000000000")
	j, err := s.CreateJob(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.CreateJob(ctx, in)
	if err != nil || again.ID != j.ID {
		t.Fatal("dedup failed", err)
	}
	in.Name = "different"
	_, err = s.CreateJob(ctx, in)
	var conflict *ConflictError
	if !errors.As(err, &conflict) {
		t.Fatal("expected typed conflict", err)
	}
	var count int
	if err = s.pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE args->>'job_id'=$1`, j.ID).Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicate queue job", err, count)
	}
	var attempts int
	if err = s.pool.QueryRow(ctx, `SELECT max_attempts FROM river_job WHERE args->>'job_id'=$1`, j.ID).Scan(&attempts); err != nil || attempts != 1 {
		t.Fatal("river automatic retry enabled", err)
	}
	got, err := s.GetJob(ctx, j.ID)
	if err != nil || got.Config.Targets[0] != "192.0.2.1" {
		t.Fatal("config roundtrip", err)
	}
	e, err := s.AddEvent(ctx, j.ID, "created", json.RawMessage(`{"safe":true}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(e)
	if !strings.Contains(string(raw), `"seq":"`) {
		t.Fatal("sequence not string JSON")
	}
	s.Close()
	s, err = New(ctx, dsn)
	if err != nil {
		t.Fatal("reopen failed")
	}
	defer s.Close()
	if _, err = s.GetJob(ctx, j.ID); err != nil {
		t.Fatal(err)
	}
	events, err := s.GetEvents(ctx, j.ID, 0, 50)
	if err != nil || len(events) != 2 || events[1].Seq != e.Seq {
		t.Fatal("events not persisted", err)
	}
	jobs, err := s.ListJobs(ctx, ListJobsInput{Search: "no-match-for-fixture-12345"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(jobs)
	if string(raw) != "[]" {
		t.Fatal("empty list must be []")
	}
	cancelled, err := s.CancelQueued(ctx, j.ID)
	if err != nil || cancelled.State != "canceled" || cancelled.FinishedAt == nil {
		t.Fatal("cancel failed", err)
	}
	var state string
	if err = s.pool.QueryRow(ctx, `SELECT state FROM river_job WHERE args->>'job_id'=$1`, j.ID).Scan(&state); err != nil || state != "cancelled" {
		t.Fatal("queue not cancelled", state, err)
	}
}
