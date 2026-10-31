package httptransport

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestBudgetPacingCancellationReleasesSlots(t *testing.T) {
	b := &requestBudget{all: make(chan struct{}, 1), perHost: 1, hosts: make(map[string]*hostSlots), interval: time.Hour, next: time.Now().Add(time.Hour)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := b.acquire(ctx, "fixture.invalid"); done <- err }()
	deadline := time.Now().Add(time.Second)
	for len(b.all) != 1 {
		if time.Now().After(deadline) {
			t.Fatal("pacing waiter never acquired global slot")
		}
		time.Sleep(time.Millisecond)
	}
	b.mu.Lock()
	host := b.hosts["fixture.invalid"]
	held := host != nil && len(host.slots) == 1
	b.mu.Unlock()
	if !held {
		t.Fatal("pacing did not occur after host admission")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("pacing cancellation stalled")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.all) != 0 || len(host.slots) != 0 || len(b.hosts) != 0 {
		t.Fatalf("canceled pacing leaked slots: global=%d host=%d refs=%d", len(b.all), len(host.slots), len(b.hosts))
	}
}
