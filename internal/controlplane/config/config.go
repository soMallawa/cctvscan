// Package config defines the server-facing scan contract. Validation never scans,
// reads target files, resolves secret references, or changes requested budgets.
package config

import (
	"context"
	"fmt"
	"math"
	"net"
	"net/netip"
	"regexp"
	"sort"
	"strings"

	"github.com/postfix/cctvscan/internal/portscan"
	"github.com/postfix/cctvscan/internal/targets"
)

// ScanConfig contains operator controls only; filesystem paths and environment
// variable names remain server-owned. Reference IDs are opaque, not secrets.
type ScanConfig struct {
	Targets            []string `json:"targets"`
	PortMode           string   `json:"port_mode"`
	Ports              string   `json:"ports"`
	ScanMode           string   `json:"scan_mode"`
	Rate               int      `json:"rate"`
	Retry              int      `json:"retry"`
	Wait               int      `json:"wait"`
	Threads            int      `json:"threads"`
	Adapter            string   `json:"adapter"`
	AdapterIP          string   `json:"adapter_ip"`
	RouterIP           string   `json:"router_ip"`
	RouterMAC          string   `json:"router_mac"`
	RouteTarget        string   `json:"route_target"`
	TimeoutSeconds     float64  `json:"timeout_seconds"`
	HostTimeoutSeconds float64  `json:"host_timeout_seconds"`
	HostWorkers        int      `json:"host_workers"`
	HTTPRequests       int      `json:"http_requests"`
	HTTPPerHost        int      `json:"http_per_host"`
	HTTPRPS            float64  `json:"http_rps"`
	MaxTargets         uint64   `json:"max_targets"`
	ProxyVersionID     string   `json:"proxy_version_id"`
	CredentialSetID    string   `json:"credential_set_id"`
	CheckCredentials   bool     `json:"check_credentials"`
	Capture            bool     `json:"capture"`
	ONVIF              bool     `json:"onvif"`
	Debug              bool     `json:"debug"`
}

// Policy is explicit server configuration, not an operator-supplied override.
// Ceilings never disable guards: zero ceilings deny positive budgets.
// Unpaced HTTP requires AllowUnpacedHTTP. Use DefaultPolicy as a base.
type Policy struct {
	MaxTargets            uint64  `json:"max_targets"`
	MaxTargetEntries      int     `json:"max_target_entries"`
	MaxUniquePorts        int     `json:"max_unique_ports"`
	MaxRate               int     `json:"max_rate"`
	MaxRetry              int     `json:"max_retry"`
	MaxWait               int     `json:"max_wait"`
	MaxThreads            int     `json:"max_threads"`
	MaxTimeoutSeconds     float64 `json:"max_timeout_seconds"`
	MaxHostTimeoutSeconds float64 `json:"max_host_timeout_seconds"`
	MaxHostWorkers        int     `json:"max_host_workers"`
	MaxHTTPRequests       int     `json:"max_http_requests"`
	MaxHTTPPerHost        int     `json:"max_http_per_host"`
	MaxHTTPRPS            float64 `json:"max_http_rps"`
	AllowUnpacedHTTP      bool    `json:"allow_unpaced_http"`
	AllowRaw              bool    `json:"allow_raw"`
	AllowCapture          bool    `json:"allow_capture"`
	AllowONVIF            bool    `json:"allow_onvif"`
	AllowCredentials      bool    `json:"allow_credentials"`
}

// Preview is authoritative only for the supplied policy; reference existence and
// runner readiness must be checked by the future control plane at submission.
type Preview struct {
	NormalizedTargets          []string   `json:"normalized_targets"`
	RequestedAddressUpperBound uint64     `json:"requested_address_upper_bound"`
	UniquePorts                int        `json:"unique_ports"`
	EffectivePorts             string     `json:"effective_ports"`
	Config                     ScanConfig `json:"config"`
}

func DefaultConfig() ScanConfig {
	return ScanConfig{Targets: []string{}, PortMode: "camera", ScanMode: "connect", Rate: 1000, Retry: 3, Wait: 1, Threads: 25, TimeoutSeconds: 1800, HostTimeoutSeconds: 30, HostWorkers: 5, HTTPRequests: 5, HTTPPerHost: 2, HTTPRPS: 10, MaxTargets: 1 << 20}
}

func DefaultPolicy() Policy {
	return Policy{MaxTargets: 1 << 20, MaxTargetEntries: 4096, MaxUniquePorts: 65536, MaxRate: 100000, MaxRetry: 10, MaxWait: 60, MaxThreads: 256, MaxTimeoutSeconds: 86400, MaxHostTimeoutSeconds: 3600, MaxHostWorkers: 256, MaxHTTPRequests: 128, MaxHTTPPerHost: 128, MaxHTTPRPS: 10000, AllowUnpacedHTTP: true, AllowCapture: true, AllowONVIF: true, AllowCredentials: true}
}

var (
	referenceID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
	adapterName = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,15}$`)
)

// Validate returns a compact preview without I/O or silently filling defaults.
// Build requests from DefaultConfig and apply operator overrides before calling.
func Validate(ctx context.Context, c ScanConfig, p Policy) (Preview, error) {
	if err := ctx.Err(); err != nil {
		return Preview{}, err
	}
	switch c.ScanMode {
	case "connect":
	case "auto", "syn":
		if !p.AllowRaw {
			return Preview{}, fmt.Errorf("scan_mode: raw capability is unavailable; select connect")
		}
	default:
		return Preview{}, fmt.Errorf("scan_mode: select connect, auto, or syn")
	}
	for _, feature := range []struct {
		field            string
		enabled, allowed bool
	}{
		{"capture", c.Capture, p.AllowCapture}, {"onvif", c.ONVIF, p.AllowONVIF}, {"check_credentials", c.CheckCredentials, p.AllowCredentials},
	} {
		if feature.enabled && !feature.allowed {
			return Preview{}, fmt.Errorf("%s: disabled by server policy", feature.field)
		}
	}
	if c.CheckCredentials && c.CredentialSetID == "" {
		return Preview{}, fmt.Errorf("credential_set_id: required when check_credentials is enabled")
	}
	for _, reference := range []struct{ field, value string }{{"proxy_version_id", c.ProxyVersionID}, {"credential_set_id", c.CredentialSetID}} {
		if reference.value != "" && !referenceID.MatchString(reference.value) {
			return Preview{}, fmt.Errorf("%s: use an opaque reference ID of 1-128 letters, digits, underscores, or hyphens", reference.field)
		}
	}
	if c.Adapter != "" && !adapterName.MatchString(c.Adapter) {
		return Preview{}, fmt.Errorf("adapter: use a valid interface name (1-15 letters, digits, dots, underscores, colons, or hyphens)")
	}
	for _, address := range []struct{ field, value string }{{"adapter_ip", c.AdapterIP}, {"router_ip", c.RouterIP}, {"route_target", c.RouteTarget}} {
		if address.value == "" {
			continue
		}
		if addr, err := netip.ParseAddr(address.value); err != nil || !addr.Is4() {
			return Preview{}, fmt.Errorf("%s: provide a single IPv4 address", address.field)
		}
	}
	if c.RouterMAC != "" {
		if mac, err := net.ParseMAC(c.RouterMAC); err != nil || len(mac) != 6 {
			return Preview{}, fmt.Errorf("router_mac: provide a six-byte Ethernet MAC address")
		}
	}
	for _, limit := range []struct {
		field           string
		value, min, max int
	}{
		{"rate", c.Rate, 1, p.MaxRate}, {"retry", c.Retry, 0, p.MaxRetry}, {"wait", c.Wait, 0, p.MaxWait},
		{"threads", c.Threads, 1, min(p.MaxThreads, 256)}, {"host_workers", c.HostWorkers, 1, min(p.MaxHostWorkers, 256)},
		{"http_requests", c.HTTPRequests, 1, min(p.MaxHTTPRequests, 128)}, {"http_per_host", c.HTTPPerHost, 1, min(p.MaxHTTPPerHost, c.HTTPRequests)},
	} {
		if limit.value < limit.min || limit.value > limit.max {
			return Preview{}, fmt.Errorf("%s: must be between %d and %d (policy/scanner ceiling)", limit.field, limit.min, limit.max)
		}
	}
	for _, limit := range []struct {
		field      string
		value, max float64
	}{
		{"timeout_seconds", c.TimeoutSeconds, p.MaxTimeoutSeconds}, {"host_timeout_seconds", c.HostTimeoutSeconds, p.MaxHostTimeoutSeconds},
	} {
		if math.IsNaN(limit.value) || math.IsInf(limit.value, 0) || math.IsNaN(limit.max) || math.IsInf(limit.max, 0) || limit.value <= 0 || limit.value > limit.max {
			return Preview{}, fmt.Errorf("%s: must be finite, positive, and within a finite policy ceiling", limit.field)
		}
	}
	if math.IsNaN(c.HTTPRPS) || math.IsInf(c.HTTPRPS, 0) || c.HTTPRPS < 0 {
		return Preview{}, fmt.Errorf("http_rps: must be finite and nonnegative")
	}
	if c.HTTPRPS == 0 {
		if !p.AllowUnpacedHTTP {
			return Preview{}, fmt.Errorf("http_rps: unpaced HTTP is disabled by server policy; select a positive rate within the policy ceiling")
		}
	} else if math.IsNaN(p.MaxHTTPRPS) || math.IsInf(p.MaxHTTPRPS, 0) || p.MaxHTTPRPS <= 0 || c.HTTPRPS > p.MaxHTTPRPS {
		return Preview{}, fmt.Errorf("http_rps: must be within a finite, positive policy ceiling")
	}
	for _, duration := range []struct {
		field   string
		seconds float64
	}{{"timeout_seconds", c.TimeoutSeconds}, {"host_timeout_seconds", c.HostTimeoutSeconds}} {
		// Runners use signed nanosecond time.Duration; reject overflow and truncation
		// to zero even if a deployment raises its configured duration ceilings.
		if duration.seconds < 1e-9 || duration.seconds >= float64(math.MaxInt64)/1e9 {
			return Preview{}, fmt.Errorf("%s: must fit a positive nanosecond time.Duration", duration.field)
		}
	}
	if len(c.Targets) == 0 || len(c.Targets) > p.MaxTargetEntries {
		return Preview{}, fmt.Errorf("targets: provide 1 to %d compact IPv4 entries", p.MaxTargetEntries)
	}
	normalized := make([]string, 0, len(c.Targets))
	seen := map[string]bool{}
	for _, raw := range c.Targets {
		if err := ctx.Err(); err != nil {
			return Preview{}, err
		}
		raw = strings.TrimSpace(raw)
		var entries []string
		if prefix, err := netip.ParsePrefix(raw); err == nil && prefix.Addr().Is4() {
			entries = []string{prefix.Masked().String()}
		} else if addr, err := netip.ParseAddr(raw); err == nil && addr.Is4() {
			entries = []string{addr.String()}
		} else if strings.Contains(raw, "-") {
			var err error
			entries, err = compactRange(raw)
			if err != nil {
				return Preview{}, fmt.Errorf("targets: provide an ascending IPv4 range as IP_FROM-IP_TO")
			}
		} else {
			return Preview{}, fmt.Errorf("targets: use IPv4 addresses, CIDRs, or IP_FROM-IP_TO ranges; files and hostnames are not allowed")
		}
		for _, canonical := range entries {
			if !seen[canonical] {
				seen[canonical] = true
				normalized = append(normalized, canonical)
			}
		}
	}
	if len(normalized) > p.MaxTargetEntries {
		return Preview{}, fmt.Errorf("targets: normalized scope exceeds the compact-entry policy ceiling; no sampling was performed")
	}
	// Do not call targets.Normalize: even valid IPv4 text may name a local file.
	sort.Strings(normalized)
	count := targets.Count(normalized)
	if c.MaxTargets == 0 || c.MaxTargets > p.MaxTargets {
		return Preview{}, fmt.Errorf("max_targets: must be positive and at most policy ceiling %d", p.MaxTargets)
	}
	if count > c.MaxTargets {
		return Preview{}, fmt.Errorf("targets: requested address upper bound exceeds max_targets; no sampling was performed")
	}
	effective := c.Ports
	switch c.PortMode {
	case "camera", "comprehensive":
		if c.Ports != "" {
			return Preview{}, fmt.Errorf("ports: conflicts with port_mode preset; select custom to supply ports")
		}
		effective = portscan.GetCCTVPorts()
		if c.PortMode == "comprehensive" {
			effective = portscan.GetComprehensiveCCTVPorts()
		}
	case "custom":
	default:
		return Preview{}, fmt.Errorf("port_mode: select camera, comprehensive, or custom")
	}
	ports, err := portscan.PortCount(effective)
	if err != nil {
		return Preview{}, fmt.Errorf("ports: provide numeric ports or ascending ranges within 0-65535")
	}
	if ports > p.MaxUniquePorts {
		return Preview{}, fmt.Errorf("ports: unique count exceeds policy ceiling %d", p.MaxUniquePorts)
	}
	c.Targets = append([]string{}, normalized...)
	return Preview{NormalizedTargets: normalized, RequestedAddressUpperBound: count, UniquePorts: ports, EffectivePorts: effective, Config: c}, nil
}
