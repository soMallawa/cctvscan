//go:build integration

package store

import (
	"context"
	"testing"
	"time"
)

// Failure to publish the event must roll back BOTH the app state and River's
// cancellation. The existing global publication guard provides a real DB fault
// boundary without schema changes or a mocked transaction.
func TestCancellationEventFailureRollsBack(t *testing.T) {
	s, _ := fixture(t)
	ctx := context.Background()
	j, err := s.CreateJob(ctx, input())
	if err != nil {
		t.Fatal(err)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(739250033)`); err != nil {
		t.Fatal(err)
	}
	blocked, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
	defer cancel()
	if _, err = s.CancelQueued(blocked, j.ID); err == nil {
		t.Fatal("cancellation committed while its event was blocked")
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetJob(ctx, j.ID)
	if err != nil || got.State != "queued" || got.CancelRequested || got.FinishedAt != nil {
		t.Fatalf("state escaped rollback: %+v %v", got, err)
	}
	var state string
	if err = s.pool.QueryRow(ctx, `SELECT state FROM river_job WHERE args->>'job_id'=$1`, j.ID).Scan(&state); err != nil || state != "available" {
		t.Fatalf("River cancellation escaped rollback: %s %v", state, err)
	}
	events, err := s.GetEvents(ctx, j.ID, 0, 100)
	if err != nil || len(events) != 1 {
		t.Fatalf("event escaped rollback: %+v %v", events, err)
	}
}
