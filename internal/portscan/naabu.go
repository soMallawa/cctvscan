package portscan

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/projectdiscovery/goflags"
	"github.com/projectdiscovery/naabu/v2/pkg/result"
	"github.com/projectdiscovery/naabu/v2/pkg/runner"
)

// NaabuConfig holds configuration for naabu scanning
type NaabuConfig struct {
	Ports        string
	Rate         int
	Retry        int
	Wait         int
	Adapter      string
	AdapterIP    string
	ExtraArgs    []string
	Debug        bool
	Privileged   bool // Force privileged mode (SYN scan)
	Unprivileged bool // Force unprivileged mode (CONNECT scan)
	Timeout      time.Duration
	Threads      int
}

// NaabuScanner uses naabu for port verification and localhost scanning
type NaabuScanner struct {
	run func(context.Context, NaabuConfig, []string) (map[string][]int, error)
	cfg NaabuConfig
}

// NewNaabuScanner creates a new naabu scanner instance
func NewNaabuScanner(cfg NaabuConfig) *NaabuScanner {
	return &NaabuScanner{cfg: cfg}
}

// Scan performs naabu scanning for the given targets
func (s *NaabuScanner) Scan(ctx context.Context, targets []string) (map[string][]int, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.run != nil {
		return s.run(ctx, s.cfg, targets)
	}
	if s.cfg.Privileged && s.cfg.Unprivileged {
		return nil, fmt.Errorf("conflicting scan modes")
	}
	if len(targets) == 0 {
		return map[string][]int{}, nil
	}

	// Determine scan type based on configuration and privileges
	scanType := s.determineScanType()

	if s.cfg.Debug {
		log.Printf("DEBUG: Using naabu scan type: %s", scanType)
	}

	// Set default values if not configured
	timeout := s.cfg.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	threads := s.cfg.Threads
	if threads == 0 {
		threads = 25 // Default thread count
	}

	options := &runner.Options{
		Host:         goflags.StringSlice(targets),
		DisableStdin: true, DisableUpdateCheck: true, NoColor: true,
		Ports:      s.cfg.Ports,
		Rate:       s.cfg.Rate,
		Retries:    s.cfg.Retry + 1,
		ScanType:   scanType,
		SourceIP:   s.cfg.AdapterIP,
		Interface:  s.cfg.Adapter,
		Silent:     !s.cfg.Debug,
		Verbose:    s.cfg.Debug,
		Debug:      s.cfg.Debug,
		Timeout:    timeout,
		Threads:    threads,
		WarmUpTime: s.cfg.Wait,
	}

	if s.cfg.Debug {
		log.Printf("DEBUG: Naabu options: Host=%v, Ports=%s, Rate=%d", options.Host, options.Ports, options.Rate)
	}

	// Collect results
	results := make(map[string][]int)
	var mu sync.Mutex

	// Set up callback to collect results
	// Use receive events: pinned Naabu's final OnResult path can omit exact
	// host:port targets when its bare-IP hostname lookup is absent.
	options.OnReceive = func(hostResult *result.HostResult) {
		if hostResult.IP != "" && len(hostResult.Ports) > 0 {
			mu.Lock()
			for _, port := range hostResult.Ports {
				results[hostResult.IP] = append(results[hostResult.IP], port.Port)
			}
			mu.Unlock()
		}
	}

	// Create and run naabu runner
	naabuRunner, err := runner.NewRunner(options)
	if err != nil {
		return nil, fmt.Errorf("failed to create naabu runner: %w", err)
	}

	// Ensure proper cleanup
	defer func() {
		if closeErr := naabuRunner.Close(); closeErr != nil && s.cfg.Debug {
			log.Printf("DEBUG: Error closing naabu runner: %v", closeErr)
		}
	}()

	// Execute the scan
	if err := naabuRunner.RunEnumeration(ctx); err != nil {
		return nil, fmt.Errorf("naabu scan failed: %w", err)
	}

	if s.cfg.Debug {
		log.Printf("DEBUG: Naabu discovered %d hosts with ports", len(results))
	}

	if err := ctx.Err(); err != nil {
		return results, err
	}
	return results, nil
}

// determineScanType determines the appropriate scan type based on configuration and privileges
func (s *NaabuScanner) determineScanType() string {
	// If explicitly configured, use that
	if s.cfg.Privileged {
		return runner.SynScan
	}
	if s.cfg.Unprivileged {
		return runner.ConnectScan
	}

	// Auto-detect based on privileges
	// Check if running as root (Unix) or with admin privileges (Windows)
	if s.isPrivileged() {
		return runner.SynScan
	}

	return runner.ConnectScan
}

// isPrivileged checks if the process has sufficient privileges for SYN scans
func (s *NaabuScanner) isPrivileged() bool {
	// On Unix systems, check if running as root
	if os.Geteuid() == 0 {
		return true
	}

	// Additional checks could be added here for Windows or other platforms
	// For now, default to false if not root on Unix
	return false
}

// VerifyPorts verifies discovered ports using naabu
func (s *NaabuScanner) VerifyPorts(ctx context.Context, discoveredPorts map[string][]int) (map[string][]int, error) {
	if len(discoveredPorts) == 0 {
		return discoveredPorts, nil
	}

	// Group identical port sets. Pinned Naabu's host:port input changes meaning
	// with a global Ports value, while an empty Ports value enables defaults.
	// Bare hosts plus their exact nonempty port set avoid both scope expansions.
	groups := map[string][]string{}
	for host, ports := range discoveredPorts {
		set := map[int]bool{}
		for _, p := range ports {
			set[p] = true
		}
		ps := make([]int, 0, len(set))
		for p := range set {
			ps = append(ps, p)
		}
		sort.Ints(ps)
		if len(ps) > 0 {
			spec := buildPortString(ps)
			groups[spec] = append(groups[spec], host)
		}
	}
	specs := make([]string, 0, len(groups))
	for spec := range groups {
		specs = append(specs, spec)
	}
	sort.Strings(specs)
	out := map[string][]int{}
	for _, spec := range specs {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		hosts := groups[spec]
		sort.Strings(hosts)
		cfg := s.cfg
		cfg.Ports = spec
		cfg.Privileged = false
		cfg.Unprivileged = true
		scanner := NewNaabuScanner(cfg)
		scanner.run = s.run
		r, err := scanner.Scan(ctx, hosts)
		if err != nil {
			return out, err
		}
		for h, ps := range r {
			out[h] = append(out[h], ps...)
		}
	}
	return out, nil
}

// buildPortString converts a slice of ports to naabu-compatible string
func buildPortString(ports []int) string {
	if len(ports) == 0 {
		return ""
	}

	// Pre-allocate with estimated capacity for better performance
	parts := make([]string, 0, len(ports))
	for _, port := range ports {
		parts = append(parts, strconv.Itoa(port)) // Use strconv.Itoa instead of fmt.Sprintf
	}

	return strings.Join(parts, ",")
}

// ValidateNaabuInstallation checks if naabu is installed and accessible
func ValidateNaabuInstallation() error {
	// Try to create a naabu runner to validate installation
	options := &runner.Options{
		Host:       goflags.StringSlice([]string{"127.0.0.1"}),
		Ports:      "80",
		Rate:       100,
		Silent:     true,
		ScanType:   runner.ConnectScan, // Use CONNECT for validation to avoid privilege issues
		Timeout:    5 * time.Second,
		Threads:    1,
		WarmUpTime: 0,
	}

	r, err := runner.NewRunner(options)
	if err != nil {
		return fmt.Errorf("naabu not available: %w", err)
	}

	defer r.Close()
	log.Printf("Naabu installation validated")
	return nil
}
