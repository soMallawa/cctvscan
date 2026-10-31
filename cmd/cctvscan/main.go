package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"github.com/postfix/cctvscan/internal/portscan"
	"github.com/postfix/cctvscan/internal/processor"
	"github.com/postfix/cctvscan/internal/targets"
)

var (
	portsFlag          = flag.String("ports", "0-65535", "Port range to scan (e.g., '80,443,8000-9000')")
	rateFlag           = flag.Int("rate", 1000, "Packets per second rate for Masscan and Naabu")
	retryFlag          = flag.Int("retry", 3, "Number of retries for port scanning")
	waitFlag           = flag.Int("wait", 1, "Seconds to wait for late replies")
	adapterFlag        = flag.String("adapter", "", "Network adapter name for Masscan and Naabu")
	adapterIPFlag      = flag.String("adapter-ip", "", "Source IPv4 address for Masscan and Naabu")
	interfaceFlag      = flag.String("interface", "", "Alias for --adapter (conflicting values are rejected)")
	routerIPFlag       = flag.String("router-ip", "", "Masscan next-hop IPv4 address")
	routerMACFlag      = flag.String("router-mac", "", "Masscan next-hop Ethernet MAC, not the adapter MAC")
	routeTargetFlag    = flag.String("masscan-route-target", "", "IPv4 reference for Linux route/neighbor lookup; use only for a single-route batch")
	routeCheckFlag     = flag.Bool("masscan-route-check", false, "Print Masscan routing configuration as JSON and exit without scanning")
	listInterfacesFlag = flag.Bool("list-interfaces", false, "List local interfaces as JSON and exit without scanning")
	timeoutFlag        = flag.String("timeout", "30m", "Overall scan timeout (e.g., '30m', '1h')")
	credsFlag          = flag.String("creds", "/etc/cctvscan/credentials.txt", "Credentials file for brute force")
	outputFlag         = flag.String("output", ".", "Output directory for results")
	debugFlag          = flag.Bool("debug", false, "Enable debug mode with verbose output")
	privilegedFlag     = flag.Bool("privileged", false, "Force privileged mode (SYN scan)")
	unprivilegedFlag   = flag.Bool("unprivileged", false, "Force unprivileged mode (CONNECT scan)")
	threadsFlag        = flag.Int("threads", 25, "Number of threads for naabu")
	comprehensiveFlag  = flag.Bool("comprehensive", false, "Use comprehensive 1000+ camera port scan instead of default 79 ports")
	helpFlag           = flag.Bool("help", false, "Show help message")
)

func main() {
	flag.Parse()

	if *helpFlag {
		printHelp()
		return
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

	// Parse timeout duration
	timeout, err := time.ParseDuration(*timeoutFlag)
	if err != nil {
		log.Fatalf("Invalid timeout format: %v", err)
	}

	// Parse targets
	targetList, err := targets.Expand(flag.Args())
	if err != nil {
		log.Fatalf("Error parsing targets: %v", err)
	}

	if len(targetList) == 0 {
		log.Fatal("No valid targets found")
	}

	if *debugFlag {
		log.Printf("DEBUG: Scanning %d target(s): %v", len(targetList), targetList)
		log.Printf("DEBUG: Configuration - ports: %s, rate: %d, retry: %d, wait: %d, timeout: %v",
			*portsFlag, *rateFlag, *retryFlag, *waitFlag, timeout)
	}

	fmt.Printf("Scanning %d target(s)\n", len(targetList))

	// Configure naabu - use camera ports by default unless specified
	portsToScan := *portsFlag
	if portsToScan == "0-65535" {
		// Use camera-specific ports by default
		if *comprehensiveFlag {
			portsToScan = portscan.GetComprehensiveCCTVPorts()
			if *debugFlag {
				log.Printf("DEBUG: Using comprehensive camera-specific ports: %s", portsToScan)
			}
		} else {
			portsToScan = portscan.GetCCTVPorts()
			if *debugFlag {
				log.Printf("DEBUG: Using default camera-specific ports: %s", portsToScan)
			}
		}
	}

	cfg := portscan.HybridConfig{
		Ports:        portsToScan,
		Rate:         *rateFlag,
		Retry:        *retryFlag,
		Wait:         *waitFlag,
		Adapter:      *adapterFlag,
		AdapterIP:    *adapterIPFlag,
		RouterIP:     *routerIPFlag,
		RouterMAC:    *routerMACFlag,
		RouteTarget:  *routeTargetFlag,
		ExtraArgs:    []string{"--open-only"},
		Debug:        *debugFlag,
		Privileged:   *privilegedFlag,
		Unprivileged: *unprivilegedFlag,
		Threads:      *threadsFlag,
	}

	if *debugFlag {
		log.Printf("DEBUG: Scanner config: %+v", cfg)
	}

	scanner := portscan.NewHybridScanner(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Scan targets
	results, err := scanner.Scan(ctx, targetList)
	if err != nil {
		log.Fatalf("Scan failed: %v", err)
	}

	fmt.Printf("Found %d hosts with open ports\n", len(results))

	// Use optimized processor for concurrent processing
	processor := processor.NewOptimizedProcessor(*debugFlag, *credsFlag, *outputFlag)
	hostResults := processor.ProcessHosts(ctx, results)

	// Print results
	processor.PrintResults(hostResults)

	if *debugFlag {
		log.Printf("DEBUG: Scan completed successfully")
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
	fmt.Printf("  %s -comprehensive 192.168.1.100     # Thorough scan (1000+ ports)\n", os.Args[0])
	fmt.Printf("  %s -ports 80,443,8080,554 192.168.1.100  # Custom ports\n", os.Args[0])
	fmt.Printf("  %s -rate 5000 192.168.1.0/24        # High-speed network scan\n", os.Args[0])
	fmt.Printf("  %s -debug -creds mycreds.txt targets.txt  # Debug with custom creds\n", os.Args[0])
	fmt.Printf("  %s -unprivileged 192.168.1.100      # No root required\n", os.Args[0])
	fmt.Printf("  %s -timeout 2h 10.0.0.0/8           # Large network with timeout\n", os.Args[0])
	fmt.Println("\nCredentials file format (user:pass per line):")
	fmt.Println("  admin:admin")
	fmt.Println("  admin:12345")
	fmt.Println("  root:root")
}
