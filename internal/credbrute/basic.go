package credbrute

import (
	"context"
	"time"
)

// TryDefaultBasic retains the legacy API but uses the joined, bounded coordinator.
func TryDefaultBasic(ctx context.Context, host string, urls []string, file string, timeout time.Duration) string {
	r, _ := CheckCredentials(ctx, urls, file, timeout)
	return r.Found
}
