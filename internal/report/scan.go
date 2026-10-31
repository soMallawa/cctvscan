package report

import (
	"encoding/json"
	"github.com/postfix/cctvscan/internal/processor"
	"os"
	"path/filepath"
	"strings"
)

type ScanReport struct {
	SchemaVersion              int
	State                      string
	Targets                    []string
	RequestedAddressUpperBound uint64
	Ports                      string
	VerifiedPorts              map[string][]int
	UnverifiedDiscovery        map[string][]int `json:",omitempty"`
	Results                    []processor.HostResult
	Errors                     []string
}

// WriteScan publishes only complete JSON bytes under a unique name, mode 0600.
func WriteScan(dir string, r ScanReport) (string, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(dir, ".scan-*.tmp")
	if err != nil {
		return "", err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err = enc.Encode(r); err != nil {
		f.Close()
		return "", err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return "", err
	}
	if err = f.Close(); err != nil {
		return "", err
	}
	name := filepath.Join(dir, strings.TrimSuffix(strings.TrimPrefix(filepath.Base(tmp), "."), ".tmp")+".json")
	if err = os.Rename(tmp, name); err != nil {
		return "", err
	}
	return name, nil
}
