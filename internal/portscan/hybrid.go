package portscan

import (
	"context"
	"fmt"
	"log"
	"net/netip"
	"sort"
	"time"
)

type HybridConfig struct {
	Ports                                                string
	Rate, Retry, Wait                                    int
	Adapter, AdapterIP, RouterIP, RouterMAC, RouteTarget string
	ExtraArgs                                            []string
	Debug, Privileged, Unprivileged                      bool
	Threads                                              int
}
type HybridScanner struct {
	cfg      HybridConfig
	discover func(context.Context, string, []string) (map[string][]int, error)
	verify   func(context.Context, map[string][]int) (map[string][]int, error)
}

func NewHybridScanner(cfg HybridConfig) *HybridScanner {
	s := &HybridScanner{cfg: cfg}
	s.discover = s.discoverDefault
	verifyCfg := s.naabuConfig()
	verifyCfg.Rate = max(1, cfg.Rate/2)
	verifyCfg.Privileged = false
	verifyCfg.Unprivileged = true
	s.verify = NewNaabuScanner(verifyCfg).VerifyPorts
	return s
}
func (s *HybridScanner) naabuConfig() NaabuConfig {
	return NaabuConfig{Ports: s.cfg.Ports, Rate: s.cfg.Rate, Retry: s.cfg.Retry, Wait: s.cfg.Wait, Adapter: s.cfg.Adapter, AdapterIP: s.cfg.AdapterIP, Debug: s.cfg.Debug, Privileged: s.cfg.Privileged, Unprivileged: s.cfg.Unprivileged, Threads: s.cfg.Threads, Timeout: 1500 * time.Millisecond}
}
func (s *HybridScanner) discoverDefault(ctx context.Context, backend string, targets []string) (map[string][]int, error) {
	if backend == "naabu" {
		cfg := s.naabuConfig()
		if s.hasLocalhostTargets(targets) {
			cfg.Privileged = false
			cfg.Unprivileged = true
		}
		return NewNaabuScanner(cfg).Scan(ctx, targets)
	}
	return NewMasscanScanner(MasscanConfig{Ports: s.cfg.Ports, Rate: s.cfg.Rate, Retry: s.cfg.Retry, Wait: s.cfg.Wait, Adapter: s.cfg.Adapter, AdapterIP: s.cfg.AdapterIP, RouterIP: s.cfg.RouterIP, RouterMAC: s.cfg.RouterMAC, RouteTarget: s.cfg.RouteTarget, Debug: s.cfg.Debug}).Scan(ctx, targets)
}
func (s *HybridScanner) Scan(ctx context.Context, targets []string) (map[string][]int, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.cfg.Privileged && s.cfg.Unprivileged {
		return nil, fmt.Errorf("privileged and unprivileged modes conflict")
	}
	discovered := map[string][]int{}
	groups := map[string][]string{}
	for _, t := range targets {
		backend := "masscan"
		if s.cfg.Unprivileged || s.hasLocalhostTargets([]string{t}) {
			backend = "naabu"
		}
		groups[backend] = append(groups[backend], t)
	}
	for _, backend := range []string{"naabu", "masscan"} {
		if len(groups[backend]) == 0 {
			continue
		}
		found, err := s.discover(ctx, backend, groups[backend])
		if err != nil {
			return nil, fmt.Errorf("%s discovery failed: %w", backend, err)
		}
		for host, ports := range found {
			discovered[host] = append(discovered[host], ports...)
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(discovered) == 0 {
		return discovered, nil
	}
	verified, err := s.verify(ctx, discovered)
	if err != nil {
		return discovered, fmt.Errorf("verification incomplete (discovery is unverified): %w", err)
	}
	for h, ps := range verified {
		set := map[int]bool{}
		for _, p := range ps {
			set[p] = true
		}
		ps = ps[:0]
		for p := range set {
			ps = append(ps, p)
		}
		sort.Ints(ps)
		verified[h] = ps
	}
	return verified, ctx.Err()
}
func (s *HybridScanner) hasLocalhostTargets(targets []string) bool {
	for _, t := range targets {
		if t == "localhost" {
			return true
		}
		if p, e := netip.ParsePrefix(t); e == nil {
			if p.Addr().IsLoopback() {
				return true
			}
		} else if a, e := netip.ParseAddr(t); e == nil && a.IsLoopback() {
			return true
		}
	}
	return false
}

// ValidateInstallation checks if both masscan and naabu are available
func ValidateInstallation() error {
	// Validate masscan
	if err := ValidateMasscanInstallation(); err != nil {
		log.Printf("WARNING: Masscan validation failed: %v", err)
	}

	// Validate naabu
	if err := ValidateNaabuInstallation(); err != nil {
		log.Printf("WARNING: Naabu validation failed: %v", err)
	}

	return nil
}
