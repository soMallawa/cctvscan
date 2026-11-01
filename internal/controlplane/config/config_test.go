package config

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/postfix/cctvscan/internal/portscan"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDefaults(t *testing.T) {
	want := ScanConfig{Targets: []string{}, PortMode: "camera", ScanMode: "connect", Rate: 1000, Retry: 3, Wait: 1, Threads: 25, TimeoutSeconds: 1800, HostTimeoutSeconds: 30, HostWorkers: 5, HTTPRequests: 5, HTTPPerHost: 2, HTTPRPS: 10, MaxTargets: 1 << 20}
	if got := DefaultConfig(); !reflect.DeepEqual(got, want) {
		t.Fatalf("defaults: %+v, want %+v", got, want)
	}
	p := DefaultPolicy()
	if p.AllowRaw || !p.AllowCapture || !p.AllowONVIF || !p.AllowCredentials {
		t.Fatalf("capabilities: %+v", p)
	}
}

func validConfig() ScanConfig { c := DefaultConfig(); c.Targets = []string{"127.0.0.1"}; return c }

func TestCompactTargets(t *testing.T) {
	c := validConfig()
	c.Targets = []string{" 127.0.0.99/24 ", "127.0.0.0/24", "127.0.0.1"}
	got, err := Validate(context.Background(), c, DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"127.0.0.0/24", "127.0.0.1"}
	if !reflect.DeepEqual(got.NormalizedTargets, want) || got.RequestedAddressUpperBound != 257 || !reflect.DeepEqual(got.Config.Targets, want) {
		t.Fatalf("compact preview: %+v", got)
	}
	c.Targets[0] = "changed"
	if got.Config.Targets[0] != want[0] {
		t.Fatal("preview aliases input")
	}
}

func TestTargetScopeAndSafeErrors(t *testing.T) {
	file := filepath.Join(t.TempDir(), "private-targets.txt")
	if err := os.WriteFile(file, []byte("127.0.0.1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{file, "/proc/self/environ", "missing-private-file", "::1", "::ffff:127.0.0.1", "::/0", "localhost", "127.0.0.1/33", "", "# comment"} {
		c := validConfig()
		c.Targets = []string{input}
		_, err := Validate(context.Background(), c, DefaultPolicy())
		if err == nil || !strings.Contains(err.Error(), "targets") {
			t.Fatalf("expected targets error for %q: %v", input, err)
		}
		if input != "" && strings.Contains(err.Error(), input) {
			t.Fatalf("input leaked: %v", err)
		}
	}
	c := validConfig()
	c.Targets = nil
	if _, err := Validate(context.Background(), c, DefaultPolicy()); err == nil {
		t.Fatal("empty scope accepted")
	}
	c.Targets = []string{"127.0.0.1/8"}
	if _, err := Validate(context.Background(), c, DefaultPolicy()); err == nil || !strings.Contains(err.Error(), "no sampling") {
		t.Fatalf("scope must be explicitly rejected: %v", err)
	}
	c.MaxTargets = 1 << 24
	p := DefaultPolicy()
	p.MaxTargets = 1 << 24
	got, err := Validate(context.Background(), c, p)
	if err != nil || !reflect.DeepEqual(got.NormalizedTargets, []string{"127.0.0.0/8"}) || got.RequestedAddressUpperBound != 1<<24 {
		t.Fatalf("whole CIDR: %+v %v", got, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = Validate(ctx, validConfig(), DefaultPolicy()); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}

func TestPortSelections(t *testing.T) {
	for _, tc := range []struct {
		mode, spec, effective string
		count                 int
	}{
		{"camera", "", portscan.GetCCTVPorts(), 0},
		{"comprehensive", "", portscan.GetComprehensiveCCTVPorts(), 0},
		{"custom", "0-65535", "0-65535", 65536},
		{"custom", "80,80,79-81", "80,80,79-81", 3},
	} {
		t.Run(tc.mode+tc.spec, func(t *testing.T) {
			c := validConfig()
			c.PortMode = tc.mode
			c.Ports = tc.spec
			got, err := Validate(context.Background(), c, DefaultPolicy())
			if err != nil {
				t.Fatal(err)
			}
			count := tc.count
			if count == 0 {
				count, _ = portscan.PortCount(tc.effective)
			}
			if got.UniquePorts != count || got.EffectivePorts != tc.effective {
				t.Fatalf("ports: %+v", got)
			}
			p := DefaultPolicy()
			p.MaxUniquePorts = count - 1
			if _, err = Validate(context.Background(), c, p); err == nil {
				t.Fatal("port ceiling ignored")
			}
		})
	}
	for _, tc := range []struct{ mode, spec string }{{"camera", "80"}, {"comprehensive", "80"}, {"custom", ""}, {"unknown", ""}, {"", ""}, {"custom", "65536"}, {"custom", "90-80"}, {"custom", "80,,443"}} {
		c := validConfig()
		c.PortMode = tc.mode
		c.Ports = tc.spec
		if _, err := Validate(context.Background(), c, DefaultPolicy()); err == nil {
			t.Fatalf("bad ports accepted: %+v", tc)
		}
	}
}

func TestNumericBounds(t *testing.T) {
	tests := []struct {
		field  string
		values []any
	}{
		{"Rate", []any{0, -1, 100001}}, {"Retry", []any{-1, 11}}, {"Wait", []any{-1, 61}},
		{"Threads", []any{0, -1, 257}}, {"HostWorkers", []any{0, -1, 257}},
		{"HTTPRequests", []any{0, -1, 129}}, {"HTTPPerHost", []any{0, -1, 6}},
		{"MaxTargets", []any{uint64(0), uint64(1<<20 + 1)}},
		{"TimeoutSeconds", []any{0.0, -1.0, 86401.0, math.NaN(), math.Inf(1), math.Inf(-1)}},
		{"HostTimeoutSeconds", []any{0.0, -1.0, 3601.0, math.NaN(), math.Inf(1)}},
		{"HTTPRPS", []any{-1.0, 10001.0, math.NaN(), math.Inf(1), math.Inf(-1)}},
	}
	for _, tc := range tests {
		for _, value := range tc.values {
			t.Run(tc.field, func(t *testing.T) {
				c := validConfig()
				reflect.ValueOf(&c).Elem().FieldByName(tc.field).Set(reflect.ValueOf(value))
				if _, err := Validate(context.Background(), c, DefaultPolicy()); err == nil {
					t.Fatalf("%s=%v accepted", tc.field, value)
				}
			})
		}
	}
	c := validConfig()
	c.Retry = 0
	c.Wait = 0
	c.TimeoutSeconds = 0.25
	c.HostTimeoutSeconds = 0.125
	if _, err := Validate(context.Background(), c, DefaultPolicy()); err != nil {
		t.Fatalf("valid zero retry/wait and fractional durations: %v", err)
	}
}

func TestDefaultPolicyPreservesUnpacedHTTP(t *testing.T) {
	c := validConfig()
	c.HTTPRPS = 0
	got, err := Validate(context.Background(), c, DefaultPolicy())
	if err != nil {
		t.Fatalf("default policy must allow disabled HTTP pacing: %v", err)
	}
	if got.Config.HTTPRPS != 0 {
		t.Fatalf("HTTP pacing changed: got %v, want 0", got.Config.HTTPRPS)
	}
}

func TestHTTPPacingPolicy(t *testing.T) {
	p := DefaultPolicy()
	if !p.AllowUnpacedHTTP {
		t.Fatal("default policy disables unpaced HTTP")
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if string(fields["allow_unpaced_http"]) != "true" {
		t.Fatalf("missing or incorrect allow_unpaced_http JSON capability: %s", data)
	}

	p.AllowUnpacedHTTP = false
	c := validConfig()
	c.HTTPRPS = 0
	c.ProxyVersionID = "private-proxy-reference"
	_, err = Validate(context.Background(), c, p)
	const want = "http_rps: unpaced HTTP is disabled by server policy; select a positive rate within the policy ceiling"
	if err == nil || err.Error() != want {
		t.Fatalf("want safe actionable policy error %q, got %v", want, err)
	}

	for _, allow := range []bool{false, true} {
		p.AllowUnpacedHTTP = allow
		p.MaxHTTPRPS = 2.5
		for _, rate := range []float64{0.125, 2.5, 2.75} {
			c.HTTPRPS = rate
			got, err := Validate(context.Background(), c, p)
			if rate > p.MaxHTTPRPS {
				if err == nil || !strings.Contains(err.Error(), "http_rps") {
					t.Fatalf("allow_unpaced_http=%t: rate %v exceeds ceiling: %v", allow, rate, err)
				}
			} else if err != nil || got.Config.HTTPRPS != rate {
				t.Fatalf("allow_unpaced_http=%t: positive rate %v changed or rejected: %+v %v", allow, rate, got, err)
			}
		}
	}
}

func TestEveryPolicyCeiling(t *testing.T) {
	// Lower each policy ceiling below a valid request; never clamp a request.
	for _, field := range []string{"MaxTargetEntries", "MaxUniquePorts", "MaxRate", "MaxRetry", "MaxWait", "MaxThreads", "MaxTimeoutSeconds", "MaxHostTimeoutSeconds", "MaxHostWorkers", "MaxHTTPRequests", "MaxHTTPPerHost", "MaxHTTPRPS", "MaxTargets"} {
		t.Run(field, func(t *testing.T) {
			p := DefaultPolicy()
			v := reflect.ValueOf(&p).Elem().FieldByName(field)
			v.Set(reflect.Zero(v.Type()))
			if _, err := Validate(context.Background(), validConfig(), p); err == nil {
				t.Fatalf("ignored %s", field)
			}
		})
	}
	for _, field := range []string{"MaxTimeoutSeconds", "MaxHostTimeoutSeconds", "MaxHTTPRPS"} {
		for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), -1} {
			p := DefaultPolicy()
			reflect.ValueOf(&p).Elem().FieldByName(field).SetFloat(value)
			if _, err := Validate(context.Background(), validConfig(), p); err == nil {
				t.Fatalf("invalid policy %s=%v accepted", field, value)
			}
		}
	}
}

func TestModesAndFeatureOptIns(t *testing.T) {
	for _, mode := range []string{"", "bogus", "CONNECT", "auto", "syn"} {
		c := validConfig()
		c.ScanMode = mode
		if _, err := Validate(context.Background(), c, DefaultPolicy()); err == nil {
			t.Fatalf("unsafe/invalid mode accepted: %s", mode)
		}
	}
	for _, mode := range []string{"connect", "auto", "syn"} {
		c := validConfig()
		c.ScanMode = mode
		p := DefaultPolicy()
		p.AllowRaw = true
		if _, err := Validate(context.Background(), c, p); err != nil {
			t.Fatalf("allowed mode %s: %v", mode, err)
		}
	}
	for _, tc := range []struct{ flag, gate string }{{"Capture", "AllowCapture"}, {"ONVIF", "AllowONVIF"}, {"CheckCredentials", "AllowCredentials"}} {
		t.Run(tc.flag, func(t *testing.T) {
			c := validConfig()
			c.CredentialSetID = "secret-set-1"
			reflect.ValueOf(&c).Elem().FieldByName(tc.flag).SetBool(true)
			if _, err := Validate(context.Background(), c, DefaultPolicy()); err != nil {
				t.Fatalf("explicit opt-in: %v", err)
			}
			p := DefaultPolicy()
			reflect.ValueOf(&p).Elem().FieldByName(tc.gate).SetBool(false)
			if _, err := Validate(context.Background(), c, p); err == nil {
				t.Fatal("feature gate ignored")
			}
		})
	}
	c := validConfig()
	c.CheckCredentials = true
	if _, err := Validate(context.Background(), c, DefaultPolicy()); err == nil || !strings.Contains(err.Error(), "credential_set_id") {
		t.Fatalf("credentials without reference: %v", err)
	}
	c = validConfig()
	c.ProxyVersionID = "proxy-version-1"
	c.CredentialSetID = "secret-set-1"
	got, err := Validate(context.Background(), c, DefaultPolicy())
	if err != nil || got.Config.CheckCredentials || got.Config.Capture || got.Config.ONVIF {
		t.Fatalf("reference implicitly enabled features: %+v %v", got, err)
	}
}

func TestRoutingAndReferences(t *testing.T) {
	c := validConfig()
	c.Adapter = "eth0"
	c.AdapterIP = "127.0.0.2"
	c.RouterIP = "127.0.0.1"
	c.RouterMAC = "02:00:00:00:00:01"
	c.RouteTarget = "127.0.0.3"
	if _, err := Validate(context.Background(), c, DefaultPolicy()); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ field, value string }{
		{"AdapterIP", "::1"}, {"AdapterIP", "127.0.0.1/24"}, {"RouterIP", "localhost"}, {"RouterIP", "::ffff:127.0.0.1"}, {"RouteTarget", "127.0.0.0/24"},
		{"RouterMAC", "bad-mac"}, {"RouterMAC", "00:11:22:33:44:55:66:77"}, {"Adapter", "/private/interface"}, {"Adapter", "eth0\nsecret"},
		{"ProxyVersionID", "http://user:password@proxy"}, {"CredentialSetID", "/private/credentials.txt"}, {"CredentialSetID", " "},
	} {
		t.Run(tc.field+tc.value, func(t *testing.T) {
			c := validConfig()
			reflect.ValueOf(&c).Elem().FieldByName(tc.field).SetString(tc.value)
			if _, err := Validate(context.Background(), c, DefaultPolicy()); err == nil {
				t.Fatal("invalid routing/reference accepted")
			} else if strings.TrimSpace(tc.value) != "" && strings.Contains(err.Error(), tc.value) {
				t.Fatalf("input leaked: %v", err)
			}
		})
	}
}

func TestDurationCannotOverflowRunner(t *testing.T) {
	for _, field := range []string{"TimeoutSeconds", "HostTimeoutSeconds"} {
		c := validConfig()
		p := DefaultPolicy()
		reflect.ValueOf(&c).Elem().FieldByName(field).SetFloat(1e20)
		reflect.ValueOf(&p).Elem().FieldByName("Max" + field).SetFloat(1e21)
		if _, err := Validate(context.Background(), c, p); err == nil {
			t.Fatalf("%s can overflow time.Duration", field)
		}
		reflect.ValueOf(&c).Elem().FieldByName(field).SetFloat(1e-12)
		if _, err := Validate(context.Background(), c, p); err == nil {
			t.Fatalf("%s becomes a zero time.Duration", field)
		}
	}
}

func TestTargetTextNeverReadsSameNamedFile(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("127.0.0.1", []byte("not-a-target\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Validate(context.Background(), validConfig(), DefaultPolicy()); err != nil {
		t.Fatalf("IPv4 text interpreted as file: %v", err)
	}
}

func TestJSONContract(t *testing.T) {
	got, err := Validate(context.Background(), validConfig(), DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var obj map[string]json.RawMessage
	if err = json.Unmarshal(data, &obj); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"normalized_targets", "requested_address_upper_bound", "unique_ports", "effective_ports", "config"} {
		if _, ok := obj[key]; !ok {
			t.Fatalf("missing preview JSON key %s", key)
		}
	}
	if len(obj) != 5 {
		t.Fatalf("unexpected preview fields: %s", data)
	}
	var conf map[string]json.RawMessage
	if err = json.Unmarshal(obj["config"], &conf); err != nil {
		t.Fatal(err)
	}
	keys := []string{"targets", "port_mode", "ports", "scan_mode", "rate", "retry", "wait", "threads", "adapter", "adapter_ip", "router_ip", "router_mac", "route_target", "timeout_seconds", "host_timeout_seconds", "host_workers", "http_requests", "http_per_host", "http_rps", "max_targets", "proxy_version_id", "credential_set_id", "check_credentials", "capture", "onvif", "debug"}
	if len(conf) != len(keys) {
		t.Fatalf("unexpected user config fields: %s", obj["config"])
	}
	for _, key := range keys {
		if _, ok := conf[key]; !ok {
			t.Fatalf("missing config JSON key %s", key)
		}
	}
}
