package portscan

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestMasscanExplicitRoutingReachesProcess(t *testing.T) {
	dir := t.TempDir()
	record := filepath.Join(dir, "args")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$MASSCAN_TEST_ARGS\"\n"
	if err := os.WriteFile(filepath.Join(dir, "masscan"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("MASSCAN_TEST_ARGS", record)
	var cfg MasscanConfig
	if err := json.Unmarshal([]byte(`{"Ports":"80","Rate":100,"Adapter":"eth7","AdapterIP":"192.0.2.2","RouterIP":"192.0.2.1","RouterMAC":"02:00:00:00:00:01"}`), &cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := NewMasscanScanner(cfg).Scan(context.Background(), []string{"198.51.100.10"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Fields(string(data))
	for _, pair := range [][]string{{"--interface", "eth7"}, {"--source-ip", "192.0.2.2"}, {"--router-ip", "192.0.2.1"}, {"--router-mac", "02:00:00:00:00:01"}} {
		found := false
		for i := 0; i+1 < len(args); i++ {
			if reflect.DeepEqual(args[i:i+2], pair) {
				found = true
			}
		}
		if !found {
			t.Errorf("missing routing arguments %v in %v", pair, args)
		}
	}
}

func TestMasscanRouteReferenceFindsInterfaceAndGateway(t *testing.T) {
	dir := t.TempDir()
	record := filepath.Join(dir, "args")
	t.Setenv("TEST_IP_CALLS", filepath.Join(dir, "ip-calls"))
	ipScript := `#!/bin/sh
printf '%s\n' "$*" >> "$TEST_IP_CALLS"
case "$*" in
 "-j -4 route get 198.51.100.10") printf '%s' '[{"dev":"eth9","gateway":"192.0.2.1","prefsrc":"192.0.2.9"}]' ;;
 "-j -4 neigh show to 192.0.2.1 dev eth9") printf '%s' '[{"dst":"192.0.2.1","lladdr":"02:00:00:00:00:09","state":["REACHABLE"]}]' ;;
 *) exit 2 ;;
esac
`
	for name, body := range map[string]string{"ip": ipScript, "masscan": `#!/bin/sh
printf '%s\n' "$@" > "$MASSCAN_TEST_ARGS"
`} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("MASSCAN_TEST_ARGS", record)
	var cfg MasscanConfig
	if err := json.Unmarshal([]byte(`{"Ports":"80","Rate":100,"RouteTarget":"198.51.100.10"}`), &cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := NewMasscanScanner(cfg).Scan(context.Background(), []string{"198.51.100.10", "198.51.100.11"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	calls, err := os.ReadFile(filepath.Join(dir, "ip-calls"))
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(calls), "\n"); n != 2 {
		t.Fatalf("expected one route and one neighbour lookup, got %d", n)
	}
	for _, want := range []string{"--interface\neth9", "--source-ip\n192.0.2.9", "--router-ip\n192.0.2.1", "--router-mac\n02:00:00:00:00:09"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("missing %q in %s", want, data)
		}
	}
}

func TestMasscanRoutingValidation(t *testing.T) {
	fakeIP(t, `[{"dev":"eth9","gateway":"192.0.2.1"}]`, `[]`)
	for _, cfg := range []MasscanConfig{
		{Adapter: "eth0;id"}, {Adapter: "-bad"}, {AdapterIP: "hostname"},
		{RouterIP: "::1"}, {RouterIP: "--help"}, {RouterMAC: "invalid"},
		{RouterMAC: "00:00:00:00:00:00"}, {RouterMAC: "ff:ff:ff:ff:ff:ff"},
		{RouterMAC: "01:00:00:00:00:01"}, {RouterMAC: "02:00:00:00:00:00:00:01"},
		{RouteTarget: "example.com"}, {RouteTarget: "198.51.100.0/24"},
	} {
		if _, err := PrepareMasscanRouting(context.Background(), cfg); err == nil {
			t.Errorf("accepted invalid routing %+v", cfg)
		}
	}
}

func TestMasscanNeighbourSafety(t *testing.T) {
	cases := []struct {
		name, route, neigh, wantIP, wantMAC string
		bad                                 bool
	}{
		{"gateway", `[{"dev":"eth9","gateway":"192.0.2.1"}]`, `[{"dst":"192.0.2.1","lladdr":"02:00:00:00:00:09","state":["REACHABLE"]}]`, "192.0.2.1", "02:00:00:00:00:09", false},
		{"on link", `[{"dev":"eth9"}]`, `[]`, "", "", true},
		{"cold", `[{"dev":"eth9","gateway":"192.0.2.1"}]`, `[]`, "192.0.2.1", "", false},
		{"failed", `[{"dev":"eth9","gateway":"192.0.2.1"}]`, `[{"dst":"192.0.2.1","lladdr":"02:00:00:00:00:09","state":["FAILED"]}]`, "192.0.2.1", "", false},
		{"wrong device", `[{"dev":"eth9","gateway":"192.0.2.1"}]`, `[{"dst":"192.0.2.1","dev":"eth8","lladdr":"02:00:00:00:00:09","state":["REACHABLE"]}]`, "192.0.2.1", "", false},
		{"wrong gateway", `[{"dev":"eth9","gateway":"192.0.2.1"}]`, `[{"dst":"192.0.2.2","lladdr":"02:00:00:00:00:09","state":["REACHABLE"]}]`, "192.0.2.1", "", false},
		{"invalid mac", `[{"dev":"eth9","gateway":"192.0.2.1"}]`, `[{"dst":"192.0.2.1","lladdr":"00:00:00:00:00:00","state":["REACHABLE"]}]`, "192.0.2.1", "", false},
		{"incomplete", `[{"dev":"eth9","gateway":"192.0.2.1"}]`, `[{"dst":"192.0.2.1","lladdr":"02:00:00:00:00:09","state":["INCOMPLETE"]}]`, "192.0.2.1", "", false},
		{"bad route", `not-json`, `[]`, "", "", true},
		{"no route", `[]`, `[]`, "", "", true},
		{"blackhole", `[{"dev":"eth9","type":"blackhole"}]`, `[]`, "", "", true},
		{"invalid gateway", `[{"dev":"eth9","gateway":"invalid"}]`, `[]`, "", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fakeIP(t, tc.route, tc.neigh)
			got, err := PrepareMasscanRouting(context.Background(), MasscanConfig{RouteTarget: "198.51.100.10"})
			if (err != nil) != tc.bad {
				t.Fatalf("error=%v, expected failure=%v", err, tc.bad)
			}
			if !tc.bad && (got.Adapter != "eth9" || got.RouterIP != tc.wantIP || got.RouterMAC != tc.wantMAC) {
				t.Fatalf("unexpected routing %+v", got)
			}
		})
	}
}

func fakeIP(t *testing.T, route, neigh string) {
	t.Helper()
	d := t.TempDir()
	script := `#!/bin/sh
case "$*" in
 *"route get"*) printf '%s' "$TEST_ROUTE" ;;
 *"neigh show"*) printf '%s' "$TEST_NEIGH" ;;
 *) exit 2 ;;
esac
`
	if err := os.WriteFile(filepath.Join(d, "ip"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", d+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TEST_ROUTE", route)
	t.Setenv("TEST_NEIGH", neigh)
}

func TestMasscanRouteExplicitOverridesAndCancellation(t *testing.T) {
	fakeIP(t, `[{"dev":"eth9","gateway":"192.0.2.1","prefsrc":"192.0.2.9"}]`, `[]`)
	cfg := MasscanConfig{RouteTarget: "198.51.100.10", Adapter: "eth9", AdapterIP: "192.0.2.99", RouterIP: "192.0.2.2", RouterMAC: "02:00:00:00:00:02"}
	got, err := PrepareMasscanRouting(context.Background(), cfg)
	if err != nil || got != cfg {
		t.Fatalf("explicit routing changed: %+v, %v", got, err)
	}
	cfg.Adapter = "eth8"
	if _, err := PrepareMasscanRouting(context.Background(), cfg); err == nil {
		t.Fatal("interface mismatch accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := PrepareMasscanRouting(ctx, MasscanConfig{}); err == nil {
		t.Fatal("cancelled context ignored")
	}
}

func TestHybridPassesGatewaySettingsToMasscan(t *testing.T) {
	d := t.TempDir()
	record := filepath.Join(d, "args")
	if err := os.WriteFile(filepath.Join(d, "masscan"), []byte(`#!/bin/sh
printf '%s\n' "$@" > "$MASSCAN_TEST_ARGS"
`), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", d+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("MASSCAN_TEST_ARGS", record)
	var cfg HybridConfig
	if err := json.Unmarshal([]byte(`{"Ports":"80","Rate":100,"RouterIP":"192.0.2.1","RouterMAC":"02:00:00:00:00:01","RouteTarget":"198.51.100.10"}`), &cfg); err != nil {
		t.Fatal(err)
	}
	fakeIP(t, `[{"dev":"eth9","gateway":"192.0.2.1"}]`, `[]`)
	if _, err := NewHybridScanner(cfg).Scan(context.Background(), []string{"198.51.100.10"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--interface\neth9", "--router-ip\n192.0.2.1", "--router-mac\n02:00:00:00:00:01"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("missing %q in %s", want, data)
		}
	}
}

func TestMasscanRoutingDefaultNeedsNoIPTool(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	cfg := MasscanConfig{Adapter: "eth0", RouterIP: "192.0.2.1"}
	got, err := PrepareMasscanRouting(context.Background(), cfg)
	if err != nil || got != cfg {
		t.Fatalf("default unexpectedly performed lookup: %+v %v", got, err)
	}
	cfg.RouteTarget = "198.51.100.10"
	if _, err := PrepareMasscanRouting(context.Background(), cfg); err == nil {
		t.Fatal("missing iproute2 was ignored")
	}
}

func TestMasscanRoutingDeadline(t *testing.T) {
	d := t.TempDir()
	if err := os.WriteFile(filepath.Join(d, "ip"), []byte("#!/bin/sh\nexec /bin/sleep 10\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", d)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := PrepareMasscanRouting(ctx, MasscanConfig{RouteTarget: "198.51.100.10"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline not preserved: %v", err)
	}
}
