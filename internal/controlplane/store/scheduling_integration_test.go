//go:build integration

package store

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"sync"
	"testing"
	"time"
)

func dueSchedule(t *testing.T, s *Store, at time.Time) string {
	t.Helper()
	v, e := s.SaveResource(context.Background(), "schedules", ResourceInput{ID: uuid.NewString(), Name: "owned recurring fixture", Config: input().Config, Cron: "* * * * *", Timezone: "UTC", Enabled: true, NextRunAt: &at}, false)
	if e != nil {
		t.Fatal(e)
	}
	var obj struct{ ID string }
	if e = json.Unmarshal(v, &obj); e != nil {
		t.Fatal(e)
	}
	return obj.ID
}
func TestScheduleOccurrenceAtomicDedupAndOverlap(t *testing.T) {
	s := runtimeFixture(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 31, 12, 0, 10, 0, time.UTC)
	due := now.Truncate(time.Minute)
	id := dueSchedule(t, s, due)
	var wg sync.WaitGroup
	errors := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := s.DispatchSchedules(ctx, now); errors <- e }()
	}
	wg.Wait()
	close(errors)
	for e := range errors {
		if e != nil {
			t.Fatal(e)
		}
	}
	n, e := s.CountJobs(ctx, ListJobsInput{})
	if e != nil || n != 1 {
		t.Fatal("occurrence was not atomically queued exactly once", n, e)
	}
	var occurrences int
	if e = s.pool.QueryRow(ctx, `SELECT count(*) FROM schedule_occurrences WHERE schedule_id=$1`, id).Scan(&occurrences); e != nil || occurrences != 1 {
		t.Fatal("occurrence ledger mismatch", e)
	}
	if _, e = s.DispatchSchedules(ctx, now.Add(time.Minute)); e != nil {
		t.Fatal(e)
	}
	n, e = s.CountJobs(ctx, ListJobsInput{})
	if e != nil || n != 1 {
		t.Fatal("overlap skip ignored", n, e)
	}
	var state string
	if e = s.pool.QueryRow(ctx, `SELECT state FROM schedule_occurrences WHERE schedule_id=$1 ORDER BY scheduled_at DESC LIMIT 1`, id).Scan(&state); e != nil || state != "overlap_skipped" {
		t.Fatal("skipped overlap not recorded", state, e)
	}
}
func TestScheduleMisfireDoesNotCreateBacklog(t *testing.T) {
	s := runtimeFixture(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 31, 12, 0, 10, 0, time.UTC)
	id := dueSchedule(t, s, now.Add(-time.Hour))
	if _, e := s.DispatchSchedules(ctx, now); e != nil {
		t.Fatal(e)
	}
	n, e := s.CountJobs(ctx, ListJobsInput{})
	if e != nil || n != 0 {
		t.Fatal("missed interval replayed without consent", n, e)
	}
	var next time.Time
	if e = s.pool.QueryRow(ctx, `SELECT next_run_at FROM schedules WHERE id=$1`, id).Scan(&next); e != nil || !next.After(now) {
		t.Fatal("misfire did not advance beyond now", e)
	}
}
