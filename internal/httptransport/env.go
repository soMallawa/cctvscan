package httptransport

import (
	"errors"
	"os"
)

// ConfigureFromEnv reads a proxy without exposing it in process arguments.
// An explicitly selected variable must be nonempty. With no selection, an
// absent or empty CCTVSCAN_HTTP_PROXY leaves existing routing unchanged.
func ConfigureFromEnv(name string) error {
	explicit := name != ""
	if !explicit {
		name = "CCTVSCAN_HTTP_PROXY"
	}
	raw := os.Getenv(name)
	if explicit && raw == "" {
		return errors.New("selected HTTP proxy environment variable is missing or empty")
	}
	return Configure(raw)
}
