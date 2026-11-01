package store

import "testing"

func TestValidState(t *testing.T) {
	for _, state := range []string{"", "queued", "running", "canceling", "completed", "inconclusive", "failed", "canceled", "timed_out", "orphaned"} {
		if !validState(state) {
			t.Errorf("valid lifecycle filter %q rejected", state)
		}
	}
	for _, state := range []string{"succeeded", "cancelled", "interrupted", "unknown", "COMPLETED", " queued", "timed-out"} {
		if validState(state) {
			t.Errorf("invalid lifecycle filter %q accepted", state)
		}
	}
}
