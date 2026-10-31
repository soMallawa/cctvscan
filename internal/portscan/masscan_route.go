package portscan

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"os/exec"
	"regexp"
	"time"
)

// PrepareMasscanRouting resolves an explicitly selected route reference using
// Linux iproute2. It reads kernel state only: no ARP, ping, or scan is sent.
// All targets in a scan must use the selected route; split mixed-route batches.
func PrepareMasscanRouting(ctx context.Context, cfg MasscanConfig) (MasscanConfig, error) {
	if err := ctx.Err(); err != nil {
		return cfg, err
	}
	if err := validateMasscanRouting(cfg); err != nil {
		return cfg, err
	}
	if cfg.RouteTarget == "" {
		return cfg, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	args := []string{"-j", "-4", "route", "get", cfg.RouteTarget}
	if cfg.Adapter != "" {
		args = append(args, "oif", cfg.Adapter)
	}
	if cfg.AdapterIP != "" {
		args = append(args, "from", cfg.AdapterIP)
	}
	data, err := exec.CommandContext(ctx, "ip", args...).Output()
	if err != nil {
		if ctx.Err() != nil {
			return cfg, fmt.Errorf("masscan route lookup: %w", ctx.Err())
		}
		return cfg, fmt.Errorf("masscan route lookup failed (requires Linux iproute2): %w", err)
	}
	var routes []struct {
		Dev     string `json:"dev"`
		Gateway string `json:"gateway"`
		PrefSrc string `json:"prefsrc"`
		Type    string `json:"type"`
	}
	if err = json.Unmarshal(data, &routes); err != nil {
		return cfg, fmt.Errorf("decode route: %w", err)
	}
	if len(routes) != 1 || routes[0].Dev == "" || (routes[0].Type != "" && routes[0].Type != "unicast") {
		return cfg, fmt.Errorf("route reference must resolve to one unicast interface")
	}
	r := routes[0]
	if cfg.Adapter != "" && cfg.Adapter != r.Dev {
		return cfg, fmt.Errorf("route interface conflicts with adapter override")
	}
	cfg.Adapter = r.Dev
	if cfg.AdapterIP == "" {
		cfg.AdapterIP = r.PrefSrc
	}
	if cfg.RouterIP == "" {
		cfg.RouterIP = r.Gateway
	}
	if err := validateMasscanRouting(cfg); err != nil {
		return cfg, fmt.Errorf("invalid resolved route: %w", err)
	}
	// Masscan normally ARPs an interface gateway, not every target. Without
	// a next-hop override, an on-link route cannot be resolved safely here.
	if cfg.RouterIP == "" && cfg.RouterMAC == "" {
		return cfg, fmt.Errorf("on-link route has no gateway: supply an explicit --router-ip or --router-mac appropriate for this batch")
	}
	if cfg.RouterIP == "" || cfg.RouterMAC != "" {
		return cfg, nil
	}
	data, err = exec.CommandContext(ctx, "ip", "-j", "-4", "neigh", "show", "to", cfg.RouterIP, "dev", cfg.Adapter).Output()
	if err != nil {
		if ctx.Err() != nil {
			return cfg, ctx.Err()
		}
		return cfg, nil // Masscan resolves a cold/unavailable neighbour cache itself.
	}
	var neighbours []struct {
		Dst   string   `json:"dst"`
		Dev   string   `json:"dev"`
		MAC   string   `json:"lladdr"`
		State []string `json:"state"`
	}
	if err = json.Unmarshal(data, &neighbours); err != nil {
		return cfg, nil
	}
	for _, n := range neighbours {
		if n.Dst != cfg.RouterIP || (n.Dev != "" && n.Dev != cfg.Adapter) || !validRouterMAC(n.MAC) {
			continue
		}
		bad := false
		for _, state := range n.State {
			if state == "FAILED" || state == "INCOMPLETE" {
				bad = true
			}
		}
		if !bad {
			cfg.RouterMAC = n.MAC
			break
		}
	}
	return cfg, nil
}

var adapterName = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.:-]{0,14}$`)

func validateMasscanRouting(cfg MasscanConfig) error {
	if cfg.Adapter != "" && !adapterName.MatchString(cfg.Adapter) {
		return fmt.Errorf("invalid interface name %q", cfg.Adapter)
	}
	for _, field := range []struct{ name, value string }{{"source IP", cfg.AdapterIP}, {"router IP", cfg.RouterIP}, {"route target", cfg.RouteTarget}} {
		if field.value == "" {
			continue
		}
		ip, err := netip.ParseAddr(field.value)
		if err != nil || !ip.Is4() || ip.IsUnspecified() || ip.IsMulticast() || field.value == "255.255.255.255" {
			return fmt.Errorf("%s must be a single unicast IPv4 address", field.name)
		}
	}
	if cfg.RouterMAC != "" && !validRouterMAC(cfg.RouterMAC) {
		return fmt.Errorf("router MAC must be a nonzero unicast Ethernet MAC (6 bytes)")
	}
	return nil
}

func validRouterMAC(value string) bool {
	mac, err := net.ParseMAC(value)
	if err != nil || len(mac) != 6 || mac[0]&1 != 0 {
		return false
	}
	for _, b := range mac {
		if b != 0 {
			return true
		}
	}
	return false
}
