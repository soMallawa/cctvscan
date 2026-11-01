package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/postfix/cctvscan/internal/report"
	"log"
	"math"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/postfix/cctvscan/internal/events"
	"github.com/postfix/cctvscan/internal/httptransport"
	"github.com/postfix/cctvscan/internal/portscan"
	"github.com/postfix/cctvscan/internal/processor"
	"github.com/postfix/cctvscan/internal/targets"
)

var (
	planFlag             = flag.Bool("plan", false, "Validate inputs and print a compact JSON plan without scanning")
	eventFDFlag          = flag.Int("event-fd", 0, "Optional private pipe descriptor for versioned scanner observations")
	maxTargetsFlag       = flag.Uint64("max-targets", 1<<20, "Maximum requested address upper bound; 0 explicitly disables the scope guard")
	hostWorkersFlag      = flag.Int("host-workers", 5, "Concurrent host workers (1-256)")
	hostTimeoutFlag      = flag.Duration("host-timeout", 30*time.Second, "Per-host wall-clock processing budget")
	httpRequestsFlag     = flag.Int("http-requests", 5, "Application-wide simultaneous HTTP requests (1-128)")
	httpPerHostFlag      = flag.Int("http-per-host", 2, "Simultaneous HTTP requests per destination host")
	httpRPSFlag          = flag.Float64("http-rps", 10, "Application-wide HTTP requests per second, including redirects; 0 disables pacing")
	checkCredentialsFlag = flag.Bool("check-credentials", false, "Explicitly enable checks using the supplied credential file")
	captureFlag          = flag.Bool("capture", false, "Explicitly save bounded validated HTTP snapshot images")
	onvifFlag            = flag.Bool("onvif", false, "Explicitly enable UDP ONVIF discovery")
	httpProxyEnvFlag     = flag.String("http-proxy-env", "", "Environment variable containing explicit HTTP(S) proxy URL (default: CCTVSCAN_HTTP_PROXY); HTTP(S) only, not raw/RTSP/UDP")
	portsFlag            = flag.String("ports", "", "Port range to scan (e.g., '80,443,8000-9000')")
	rateFlag             = flag.Int("rate", 1000, "Packets per second rate for Masscan and Naabu")
	retryFlag            = flag.Int("retry", 3, "Number of retries for port scanning")
	waitFlag             = flag.Int("wait", 1, "Seconds to wait for late replies")
	adapterFlag          = flag.String("adapter", "", "Network adapter name for Masscan and Naabu")
	adapterIPFlag        = flag.String("adapter-ip", "", "Source IPv4 address for Masscan and Naabu")
	interfaceFlag        = flag.String("interface", "", "Alias for --adapter (conflicting values are rejected)")
	routerIPFlag         = flag.String("router-ip", "", "Masscan next-hop IPv4 address")
	routerMACFlag        = flag.String("router-mac", "", "Masscan next-hop Ethernet MAC, not the adapter MAC")
	routeTargetFlag      = flag.String("masscan-route-target", "", "IPv4 reference for Linux route/neighbor lookup; use only for a single-route batch")
	routeCheckFlag       = flag.Bool("masscan-route-check", false, "Print Masscan routing configuration as JSON and exit without scanning")
	listInterfacesFlag   = flag.Bool("list-interfaces", false, "List local interfaces as JSON and exit without scanning")
	timeoutFlag          = flag.String("timeout", "30m", "Overall scan timeout (e.g., '30m', '1h')")
	credsFlag            = flag.String("creds", "/etc/cctvscan/credentials.txt", "Credentials file for brute force")
	outputFlag           = flag.String("output", ".", "Output directory for results")
	debugFlag            = flag.Bool("debug", false, "Enable debug mode with verbose output")
	privilegedFlag       = flag.Bool("privileged", false, "Force privileged mode (SYN scan)")
	unprivilegedFlag     = flag.Bool("unprivileged", false, "Force unprivileged mode (CONNECT scan)")
	threadsFlag          = flag.Int("threads", 25, "Number of threads for naabu")
	comprehensiveFlag    = flag.Bool("comprehensive", false, "Use the comprehensive unique-port preset instead of the default preset")
	helpFlag             = flag.Bool("help", false, "Show help message")
)

func main() {
	flag.Parse()

	if *helpFlag {
		printHelp()
		return
	}
	if err := httptransport.ConfigureFromEnv(*httpProxyEnvFlag); err != nil {
		log.Fatal(err)
	}
	if *listInterfacesFlag {
		interfaces, err := net.Interfaces()
		if err != nil {
			log.Fatalf("List interfaces: %v", err)
		}
		type interfaceInfo struct {
			Name      string
			MAC       string
			MTU       int
			Flags     string
			Addresses []string
		}
		info := make([]interfaceInfo, 0, len(interfaces))
		for _, iface := range interfaces {
			addrs, err := iface.Addrs()
			if err != nil {
				log.Fatalf("Interface %s addresses: %v", iface.Name, err)
			}
			addresses := make([]string, 0, len(addrs))
			for _, addr := range addrs {
				addresses = append(addresses, addr.String())
			}
			info = append(info, interfaceInfo{iface.Name, iface.HardwareAddr.String(), iface.MTU, iface.Flags.String(), addresses})
		}
		if err := printJSON(info); err != nil {
			log.Fatal(err)
		}
		return
	}
	if *interfaceFlag != "" {
		if *adapterFlag != "" && *adapterFlag != *interfaceFlag {
			log.Fatal("--interface and --adapter conflict")
		}
		*adapterFlag = *interfaceFlag
	}
	if *routeCheckFlag {
		cfg, err := portscan.PrepareMasscanRouting(context.Background(), portscan.MasscanConfig{
			Adapter: *adapterFlag, AdapterIP: *adapterIPFlag, RouterIP: *routerIPFlag, RouterMAC: *routerMACFlag, RouteTarget: *routeTargetFlag,
		})
		if err != nil {
			log.Fatalf("Route check failed: %v", err)
		}
		if err := printJSON(cfg); err != nil {
			log.Fatal(err)
		}
		return
	}
	if len(flag.Args()) == 0 {
		printHelp()
		os.Exit(0)
	}

	timeout, err := time.ParseDuration(*timeoutFlag)
	if err != nil || timeout <= 0 {
		log.Fatal("timeout must be a positive duration")
	}
	if *rateFlag < 1 || *threadsFlag < 1 || *threadsFlag > 256 || *retryFlag < 0 || *waitFlag < 0 || *hostWorkersFlag < 1 || *hostWorkersFlag > 256 || *hostTimeoutFlag <= 0 {
		log.Fatal("invalid numeric scan controls")
	}
	if *privilegedFlag && *unprivilegedFlag {
		log.Fatal("privileged and unprivileged modes conflict")
	}
	if *httpRequestsFlag > 128 || *httpPerHostFlag > *httpRequestsFlag || math.IsNaN(*httpRPSFlag) || math.IsInf(*httpRPSFlag, 0) {
		log.Fatal("invalid HTTP budget")
	}
	if err = httptransport.ConfigureLimits(*httpRequestsFlag, *httpPerHostFlag, *httpRPSFlag); err != nil {
		log.Fatal(err)
	}
	base, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(base, timeout)
	defer cancel()
	targetList, err := targets.Normalize(ctx, flag.Args())
	if err != nil {
		log.Fatalf("Error parsing targets: %v", err)
	}
	if len(targetList) == 0 {
		log.Fatal("No valid targets found")
	}
	addressCount := targets.Count(targetList)
	if *maxTargetsFlag > 0 && addressCount > *maxTargetsFlag {
		log.Fatal("requested address upper bound exceeds --max-targets; no sampling was performed")
	}
	portsToScan := *portsFlag
	if portsToScan != "" && *comprehensiveFlag {
		log.Fatal("--ports and --comprehensive conflict")
	}
	if portsToScan == "" {
		if *comprehensiveFlag {
			portsToScan = portscan.GetComprehensiveCCTVPorts()
		} else {
			portsToScan = portscan.GetCCTVPorts()
		}
	}
	count, err := portscan.PortCount(portsToScan)
	if err != nil {
		log.Fatal(err)
	}
	cfg := portscan.HybridConfig{Ports: portsToScan, Rate: *rateFlag, Retry: *retryFlag, Wait: *waitFlag, Adapter: *adapterFlag, AdapterIP: *adapterIPFlag, RouterIP: *routerIPFlag, RouterMAC: *routerMACFlag, RouteTarget: *routeTargetFlag, Debug: *debugFlag, Privileged: *privilegedFlag, Unprivileged: *unprivilegedFlag, Threads: *threadsFlag}
	opts := processor.Options{Workers: *hostWorkersFlag, HostTimeout: *hostTimeoutFlag, CheckCredentials: *checkCredentialsFlag, Capture: *captureFlag, ONVIF: *onvifFlag}
	if *planFlag {
		if err = printJSON(struct {
			Targets                    []string
			RequestedAddressUpperBound uint64
			UniquePorts                int
			ScanConfig                 portscan.HybridConfig
			Processing                 processor.Options
		}{targetList, addressCount, count, cfg, opts}); err != nil {
			log.Fatal(err)
		}
		return
	}
	var observer *events.Writer
	if *eventFDFlag != 0 {
		var closeObserver func()
		observer, closeObserver, err = events.OpenFD(*eventFDFlag)
		if err != nil {
			log.Fatal("private event descriptor unavailable")
		}
		defer closeObserver()
		opts.Observer = func(result processor.HostResult) { _ = observer.Emit("host_result", result) }
	}
	// Fail locally before discovery if the output directory is not writable.
	if err = os.MkdirAll(*outputFlag, 0700); err != nil {
		log.Fatal("output directory unavailable")
	}
	f, err := os.CreateTemp(*outputFlag, ".write-check-*")
	if err != nil {
		log.Fatal("output directory is not writable")
	}
	f.Close()
	os.Remove(f.Name())
	fmt.Printf("Scanning %d compact target entries (address upper bound %d), %d unique ports\n", len(targetList), addressCount, count)
	rep := report.ScanReport{SchemaVersion: 1, State: "complete", Targets: targetList, RequestedAddressUpperBound: addressCount, Ports: portsToScan, Results: []processor.HostResult{}}
	if observer != nil {
		_ = observer.Emit("phase", map[string]string{"phase": "port_discovery"})
	}
	results, scanErr := portscan.NewHybridScanner(cfg).Scan(ctx, targetList)
	if scanErr != nil {
		rep.State = "failed"
		rep.UnverifiedDiscovery = results
		rep.Errors = append(rep.Errors, "discovery or verification incomplete; results are NOT verified")
	} else {
		rep.VerifiedPorts = results
		engine := processor.NewOptimizedProcessor(*debugFlag, *credsFlag, *outputFlag)
		engine.Options = opts
		if observer != nil {
			_ = observer.Emit("phase", map[string]string{"phase": "host_processing"})
		}
		rep.Results = engine.ProcessHosts(ctx, results)
		engine.PrintResults(rep.Results)
		for _, r := range rep.Results {
			if r.Error != nil {
				rep.State = "inconclusive"
			}
		}
	}
	if ctx.Err() != nil {
		rep.State = "canceled"
		rep.Errors = append(rep.Errors, ctx.Err().Error())
	}
	path, err := report.WriteScan(*outputFlag, rep)
	if err != nil {
		log.Fatal("failed to save results")
	}
	fmt.Printf("Results saved: %s; state=%s\n", path, rep.State)
	if rep.State != "complete" {
		os.Exit(1)
	}
}

func printJSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func printHelp() {
	fmt.Printf("Usage: %s [OPTIONS] <target> [target2 ...]\n", os.Args[0])
	fmt.Println("\nTargets can be: IP addresses, CIDR ranges, or files containing targets")
	fmt.Println("\nOptions:")
	flag.VisitAll(func(f *flag.Flag) {
		fmt.Printf("  -%-12s %s (default: %v)\n", f.Name, f.Usage, f.DefValue)
	})
	fmt.Println("\nExamples:")
	fmt.Printf("  %s 192.168.1.100                    # Basic scan (79 ports)\n", os.Args[0])
	fmt.Printf("  %s -comprehensive 192.168.1.100     # Comprehensive unique-port preset\n", os.Args[0])
	fmt.Printf("  %s -ports 80,443,8080,554 192.168.1.100  # Custom ports\n", os.Args[0])
	fmt.Printf("  %s -rate 5000 192.168.1.0/24        # High-speed network scan\n", os.Args[0])
	fmt.Printf("  %s -check-credentials -creds mycreds.txt targets.txt  # Explicit credential checks\n", os.Args[0])
	fmt.Printf("  %s -unprivileged 192.168.1.100      # No root required\n", os.Args[0])
	fmt.Printf("  %s -timeout 2h 10.0.0.0/8           # Large network with timeout\n", os.Args[0])
	fmt.Println("\nCredentials file format (user:pass per line):")
	fmt.Println("  admin:admin")
	fmt.Println("  admin:12345")
	fmt.Println("  root:root")
}
