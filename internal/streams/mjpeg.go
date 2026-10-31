package streams

import (
	"context"
	"errors"
	"github.com/postfix/cctvscan/internal/httptransport"
	"path/filepath"
	"time"
)

// TryMJPEG is an explicitly invoked legacy capture helper. It is synchronous,
// validates the media and reports failures; normal host discovery never calls it.
func TryMJPEG(ctx context.Context, host string, ports []int, dir string) error {
	rs, e := DetectStreamsDetailed(ctx, host, ports)
	var errs []error
	errs = append(errs, e)
	for _, r := range rs {
		if r.Verified && (r.Type == StreamMJPEG || r.Type == StreamSnapshot) {
			_, e := SaveSnapshot(ctx, httptransport.Client(3*time.Second), r, filepath.Clean(dir))
			errs = append(errs, e)
		}
	}
	return errors.Join(errs...)
}
