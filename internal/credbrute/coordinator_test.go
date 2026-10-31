package credbrute

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestCoordinatorWaitsAndJoins(t *testing.T) {
	jobs := make(chan checkJob, 1)
	jobs <- checkJob{credential: "fixture"}
	close(jobs)
	release := make(chan struct{})
	started := make(chan struct{})
	done := make(chan string, 1)
	var active atomic.Int32
	go func() {
		s, _ := firstSuccessful(context.Background(), jobs, 2, func(ctx context.Context, j checkJob) (bool, error) {
			active.Add(1)
			defer active.Add(-1)
			close(started)
			<-release
			return true, nil
		})
		done <- s
	}()
	<-started
	select {
	case <-done:
		t.Fatal("returned while work unfinished")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if s := <-done; s != "fixture" || active.Load() != 0 {
		t.Fatalf("result=%q active=%d", s, active.Load())
	}
}
