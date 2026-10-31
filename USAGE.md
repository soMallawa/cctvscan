# CCTVScan Usage Guide

This comprehensive guide covers all features and usage patterns for CCTVScan, an advanced IP camera security assessment tool.

## Table of Contents

1. [Quick Start](#quick-start)
2. [Command Line Options](#command-line-options)
3. [Scanning Modes](#scanning-modes)
4. [Advanced Features](#advanced-features)
5. [Output Interpretation](#output-interpretation)
6. [Examples](#examples)
7. [Troubleshooting](#troubleshooting)

## Application-Wide HTTP/HTTPS Proxy

This changes routing, not which existing features execute. It does not add a
separate scan mode, new HTTP methods to probes, new targets, or new credential
strategies. Use only for assets you are authorized to assess.

### Configuration

Use an environment variable so credentials do not appear in command arguments:

```bash
# Bash: interactive, hidden input; the proxy URL is not stored in shell history.
read -r -s -p 'HTTP(S) proxy URL: ' CCTVSCAN_HTTP_PROXY
printf '\n'
export CCTVSCAN_HTTP_PROXY

# Local diagnostic only: validates configuration, but sends no proxy request.
./cctvscan --http-proxy-env CCTVSCAN_HTTP_PROXY --list-interfaces

# For a normal authorized run, append --http-proxy-env CCTVSCAN_HTTP_PROXY
# to your existing invocation. Its non-HTTP stages still use direct networking.

unset CCTVSCAN_HTTP_PROXY
```

- Supported proxy URL schemes: `http` and `https`, with optional URL-encoded
  username/password and port. SOCKS URLs are rejected; they are not silently
  treated as direct connections. DataImpulse endpoint/protocol/port values must
  come from your account configuration, not from these illustrative examples.
- `--http-proxy-env NAME` selects another environment variable. Explicitly
  selecting a missing/empty variable is an error. This is recommended for
  unattended proxy-required runs so a missing secret cannot disable the policy.
- Without the flag, a nonempty `CCTVSCAN_HTTP_PROXY` enables the proxy. If that
  variable is absent/empty, original client routing is preserved. This does not
  globally override existing HTTP_PROXY/HTTPS_PROXY behavior of default clients.
- Malformed proxy configuration fails before scanning. No proxy URL is accepted
  on the command line. Environment values remain accessible to privileged users
  and process-inspection tools; use protected secret injection and never `set -x`.
- Configuration is startup-only, before creating clients/workers. Live proxy
  changes and previously constructed clients are not supported. Library callers
  must call `httptransport.Configure` before making clients or starting work.

### Coverage and failure behavior

- All nine first-party custom transports and geolocation use the shared factory.
  It clones existing timeouts, connection settings and target TLS configuration.
- HTTP method handling is unchanged: GET, HEAD, POST, PUT, PATCH, DELETE, OPTIONS,
  TRACE and extension methods are routed by the transport, not a method filter.
  This does **not** make probes automatically issue all those methods.
- Redirects and HTTPS CONNECT tunnels use the supplied proxy. Even localhost and
  entries in `NO_PROXY` are not bypassed. The proxy's exit network determines
  target reachability; a private address does not mean your local network.
- The Go default HTTP transport is also configured, covering default-client and
  nil-transport dependency requests. No proxy failure causes a direct retry.
- An HTTPS proxy always has its certificate/hostname verified, even if a camera
  client permits self-signed target certificates. Target TLS compatibility does
  not disable proxy authentication. Caller-provided CA roots can be used by
  library callers; normal CLI operation uses the system trust store.
- Transport errors are sanitized, without raw proxy URLs, credentials or remote
  status text. CONNECT rejection, forward HTTP 407 and other proxied request
  failures have separate error identities. Cancellation/timeout identity is kept.
  Original error chains are intentionally omitted because they can contain secrets.
- A plain HTTP 403/5xx is not proof of a proxy outage: it may be a target response.
  Other than 407, normal forward-proxy response statuses are retained. Response
  content is untrusted and is not a general-purpose secret-redaction boundary.
- Existing callers may suppress transport errors or return empty findings. This
  patch does not redesign result reporting: an empty result is not proof that a
  camera is absent or that authentication failed. Check provider restrictions
  and controlled connectivity before interpreting scan results.
- DataImpulse port permissions, bandwidth, concurrent connections and account
  permissions still apply. This feature does not bypass blocked destinations.

### Non-HTTP and dependency boundaries

Masscan/raw packets, Naabu TCP port discovery, direct RTSP sockets, ONVIF UDP
and ordinary DNS are unchanged. This is not VPN routing or process-wide network
isolation, and it does not disable the existing credential/media stages.

The current CLI only passes validated IP/CIDR targets into Naabu and does not
enable its passive lookup, ASN expansion, cloud upload or updater paths. Those
unexposed features have private HTTP transports in the pinned dependencies
(`retryablehttp`, `asnmap`, update/uncover helpers); replacing the Go default
transport does not control them. They must be wired explicitly before exposing
those features. Future custom HTTP clients, DoH settings, or media subprocesses
also require review; the first-party client coverage test guards current client
construction sites, not arbitrary third-party code.

### Local verification

```bash
go test -race ./internal/httptransport ./internal/probe ./cmd/cctvscan
```

The proxy tests use loopback fixtures and `.invalid` hostnames, not public
cameras or a paid proxy account. They exercise method forwarding, redirects,
CONNECT, auth isolation, proxy failure/no fallback, redaction, configuration,
TLS verification and representative real probe integration.

## Masscan Routing and Interface Selection

The fork supports explicit interface/gateway control and non-scanning diagnostics:

```bash
# Local interface names, MACs, IP addresses, MTUs and flags; exits without scanning.
./cctvscan --list-interfaces

# Resolve the route for a reference IPv4 address; exits without scanning.
# This example uses a documentation address, not a live scan target.
./cctvscan --masscan-route-check --masscan-route-target 192.0.2.10

# Validate explicit overrides without scanning (values below are examples).
./cctvscan --masscan-route-check --interface eth0 \
  --adapter-ip 192.0.2.2 --router-ip 192.0.2.1 \
  --router-mac 02:00:00:00:00:01
```

Routing flags can also be passed to a normal, authorized scan:

- `--adapter NAME` or `--interface NAME`: outgoing interface. Both may be supplied only with the same value. Explicit interface and `--adapter-ip` are passed to Masscan and Naabu.
- `--adapter-ip IPv4`: explicit source IPv4 address; overrides the route-selected source (`prefsrc`).
- `--router-ip IPv4`: Masscan's next-hop gateway. This is **not the destination IP** for a routed network.
- `--router-mac MAC`: Masscan's next-hop **Ethernet** MAC, not the local NIC's MAC. Must be a nonzero unicast 6-byte address.
- `--masscan-route-target IPv4`: opt-in Linux `iproute2` lookup for one explicit reference address. Fills the Masscan interface, preferred source IPv4, gateway and cached gateway MAC when available. Explicit values take precedence; conflicting interface results fail.
- `--masscan-route-check`: print routing configuration as JSON and exit before target expansion, discovery, verification or processing. Empty fields mean no override; this is not a reachability/privilege check.
- `--list-interfaces`: local interface inventory as JSON and exit. No scan target is required.

### Limits and operational notes

- Defaults remain unchanged: no route subprocess is run unless `--masscan-route-target` is set. No interface, gateway or MAC from the development host is hard-coded.
- Route discovery is **Masscan-only**, IPv4-only and requires Linux `ip -j`. It reads route/neighbor state with a shared four-second deadline; it does not ping, send ARP or warm a neighbor cache. Naabu retains its existing routing behavior unless explicitly configured with `--adapter` / `--adapter-ip`.
- A reference selects **one route for the whole Masscan batch**; it does not validate every target's route. Split targets using different gateways/interfaces into separate runs, especially with VPNs, policy routing or multiple NICs. Route lookup is not repeated for every endpoint.
- An empty/failed/incomplete neighbor entry leaves the MAC unset so Masscan can perform its normal resolution. Inspect the JSON before running if Masscan needs a manual MAC workaround. Cached entries may age; rerun diagnostics after network changes.
- An on-link reference without a gateway fails unless an explicit gateway or MAC is supplied. Masscan normally resolves an interface gateway, not each target individually; this resolver cannot safely infer one destination MAC for an on-link subnet. A manual next-hop override must be appropriate for the entire batch.
- Subprocesses use argument arrays, not a shell. Interface and address overrides are validated; route errors fail before launching Masscan.
- These controls do not add proxy support. Masscan sends raw traffic directly and requires suitable privileges/capabilities and an Ethernet-compatible interface. A VPN/TUN interface appearing in the inventory is not proof Masscan can use it. For containers, inspect the container's interfaces/routes, not the host's.
- `--rate` already applies to Masscan and Naabu; this change does not raise it. Existing `--retry` / `--wait` behavior is unchanged (the wrapper currently forwards them to Naabu only).
- Normal scan mode still runs the upstream processing pipeline; diagnostic flags are the non-scanning mode. This patch changes no credential, stream, capture or proxy logic.

## Quick Start

### Basic Usage

```bash
# Scan a single IP (default 79 ports)
sudo ./cctvscan 192.168.1.100

# Scan a network range
sudo ./cctvscan 192.168.1.0/24

# Scan targets from file
sudo ./cctvscan targets.txt

# Comprehensive scan (1000+ ports)
sudo ./cctvscan --comprehensive 192.168.1.100
```

### Prerequisites

```bash
# Install dependencies
sudo apt-get install masscan
go install github.com/projectdiscovery/naabu/v2/cmd/naabu@latest

# Set masscan capabilities
sudo setcap cap_net_raw+ep $(which masscan)

# Build the tool
go build -o cctvscan ./cmd/cctvscan
```

## Command Line Options

### Core Options

| Option | Description | Default | Example |
|--------|-------------|---------|---------|
| `--comprehensive` | Use 1000+ camera ports | false | `--comprehensive` |
| `--ports` | Custom port range | 0-65535 | `--ports 80,443,8080,554` |
| `--rate` | Packets per second | 1000 | `--rate 5000` |
| `--timeout` | Scan timeout | 30m | `--timeout 1h` |
| `--debug` | Debug mode | false | `--debug` |

### Scanning Options

| Option | Description | Default | Example |
|--------|-------------|---------|---------|
| `--privileged` | Force SYN scan | auto | `--privileged` |
| `--unprivileged` | Force CONNECT scan | auto | `--unprivileged` |
| `--retry` | Number of retries | 3 | `--retry 5` |
| `--wait` | Wait for late replies | 1 | `--wait 2` |
| `--threads` | Number of threads | 25 | `--threads 50` |

### Network Options

| Option | Description | Default | Example |
|--------|-------------|---------|---------|
| `--adapter` | Network adapter | auto | `--adapter eth0` |
| `--adapter-ip` | Source IP | auto | `--adapter-ip 192.168.1.10` |

### File Options

| Option | Description | Default | Example |
|--------|-------------|---------|---------|
| `--creds` | Credentials file | /etc/cctvscan/credentials.txt | `--creds mycreds.txt` |
| `--output` | Output directory | . | `--output /tmp/results` |

### Help Options

| Option | Description |
|--------|-------------|
| `--help` | Show help message |
| `--version` | Show version information |

## Scanning Modes

### 1. Default Mode (79 ports) - Fast Scanning

**Use Case**: Quick reconnaissance and initial assessment

```bash
# Basic scan
sudo ./cctvscan 192.168.1.100

# Network range
sudo ./cctvscan 192.168.1.0/24

# With debug output
sudo ./cctvscan --debug 192.168.1.100
```

**Ports Scanned**:
- Web ports: 80, 81, 82, 83, 84, 85, 86, 87, 88, 89, 443, 8080, 8443
- RTSP ports: 554, 8554, 10554, 1554, 2554, 3554, 4554, 5554, 6554, 7554, 9554
- RTMP ports: 1935, 1936, 1937, 1938, 1939
- ONVIF: 3702
- Miscellaneous: 37777, 5000, 7001, 8999, 9000-9002, 10000, 8181, 5001, 50000, 8880, 8889, 3001

### 2. Comprehensive Mode (1000+ ports) - Thorough Scanning

**Use Case**: Complete security assessment and penetration testing

```bash
# Comprehensive scan
sudo ./cctvscan --comprehensive 192.168.1.100

# With custom rate
sudo ./cctvscan --comprehensive --rate 2000 192.168.1.100

# With timeout
sudo ./cctvscan --comprehensive --timeout 2h 192.168.1.0/24
```

**Additional Ports**:
- Custom camera ports: 37777-37800
- ONVIF ports: 3702-3710
- High port ranges: 10000-65010 (increments of 10)
- MMS ports: 1755-1760
- Extended web ports: 5000-9010, 8888-9999
- VLC streaming: 8080-8190

### 3. Custom Port Mode - Targeted Scanning

**Use Case**: Specific port ranges or known services

```bash
# Specific ports
sudo ./cctvscan --ports 80,443,8080,554 192.168.1.100

# Port range
sudo ./cctvscan --ports 8000-9000 192.168.1.100

# Mixed ranges
sudo ./cctvscan --ports 80,443,8000-9000,554 192.168.1.100
```

## Advanced Features

### Enhanced Brand Detection

The tool automatically detects 15+ camera brands using multiple methods:

- **Server Headers**: Brand-specific server strings
- **Content Analysis**: Page titles, form fields, keywords
- **DVR/NVR Patterns**: Specialized detection for recording devices
- **Model Detection**: Specific model identification (e.g., CP Plus UVR-0401E1)

### Comprehensive Stream Detection

Multi-protocol stream detection supporting:

- **MJPEG Streams**: Real-time video streams
- **RTSP Streams**: Real-time streaming protocol
- **RTMP Streams**: Real-time messaging protocol
- **MMS Streams**: Microsoft media streaming
- **HLS Streams**: HTTP live streaming

### Geographic Intelligence

IP location lookup provides:

- **ISP Information**: Internet service provider details
- **Geographic Coordinates**: Latitude/longitude with Google Maps links
- **Location Details**: City, region, country, postal code
- **Timezone Information**: Local timezone data

### Search Integration

Direct integration with security search engines:

- **Shodan**: Device and service discovery
- **Censys**: Host and certificate analysis
- **Zoomeye**: Network device search
- **Google Dorking**: Advanced search queries for camera discovery

## Output Interpretation

### Basic Output

```
=== Processing 192.168.1.100 ===
Open ports: [80 443 8080 554]
HTTP ports: [80 443 8080]
RTSP ports: [554]
HTTP Server: Hikvision-Webs
Login pages: [http://192.168.1.100/ http://192.168.1.100/login]
```

### Enhanced Detection Output

```
🎥 Camera Detected: Hikvision
   Model: DS-2CD2143G0-I
   Server: Hikvision-Webs
   Indicators: [Server Header: Hikvision-Webs, Content Type: text/html]
   Auth Required: Basic
   Endpoints: [http://192.168.1.100/ http://192.168.1.100/login]

📺 Streams Found (3):
   RTSP: rtsp://192.168.1.100:554/Streaming/Channels/101
   HTTP-Video: http://192.168.1.100:8080/video/mjpg.cgi
   MJPEG: http://192.168.1.100:80/mjpg/video.mjpg

🛡️ CVEs Found:
   CVE-2021-36260: https://nvd.nist.gov/vuln/detail/CVE-2021-36260
   CVE-2017-7921: https://nvd.nist.gov/vuln/detail/CVE-2017-7921

🌍 Geographic Information:
   IP: 192.168.1.100
   ISP: Private Network
   Location: Local Network
   Coordinates: N/A (Private IP)

🔍 Search Integration:
   Shodan: https://www.shodan.io/search?query=192.168.1.100
   Censys: https://search.censys.io/hosts/192.168.1.100
   Zoomeye: https://www.zoomeye.org/searchResult?q=192.168.1.100
   Google Dork: https://www.google.com/search?q=site:192.168.1.100+inurl:view/view.shtml
```

## Examples

### Example 1: Basic Network Scan

```bash
# Scan local network
sudo ./cctvscan 192.168.1.0/24

# Output: Shows all cameras found in the network
```

### Example 2: Comprehensive Security Assessment

```bash
# Thorough scan with debug output
sudo ./cctvscan --comprehensive --debug 192.168.1.100

# Output: Detailed analysis with all detection features
```

### Example 3: Custom Port Range

```bash
# Scan specific ports
sudo ./cctvscan --ports 80,443,8080,554,1935 192.168.1.100

# Output: Focused scan on specific services
```

### Example 4: High-Speed Scanning

```bash
# Fast scan with high rate
sudo ./cctvscan --rate 10000 192.168.1.0/24

# Output: Quick network reconnaissance
```

### Example 5: Long-Running Scan

```bash
# Extended timeout for large networks
sudo ./cctvscan --comprehensive --timeout 4h 10.0.0.0/8

# Output: Comprehensive scan of large network
```

## Troubleshooting

### Common Issues

#### 1. Permission Denied

```bash
# Error: Permission denied for masscan
# Solution: Set capabilities
sudo setcap cap_net_raw+ep $(which masscan)
```

#### 2. Naabu Not Found

```bash
# Error: naabu: command not found
# Solution: Install naabu
go install github.com/projectdiscovery/naabu/v2/cmd/naabu@latest
```

#### 3. No Open Ports Found

```bash
# Issue: No ports detected
# Solutions:
# 1. Check if target is reachable
ping 192.168.1.100

# 2. Try unprivileged mode
./cctvscan --unprivileged 192.168.1.100

# 3. Check firewall settings
sudo ufw status
```

#### 4. Slow Scanning

```bash
# Issue: Scan is too slow
# Solutions:
# 1. Increase rate
./cctvscan --rate 5000 192.168.1.100

# 2. Use default mode instead of comprehensive
./cctvscan 192.168.1.100

# 3. Reduce timeout
./cctvscan --timeout 10m 192.168.1.100
```

#### 5. Memory Issues

```bash
# Issue: Out of memory
# Solutions:
# 1. Use default mode
./cctvscan 192.168.1.100

# 2. Reduce thread count
./cctvscan --threads 10 192.168.1.100

# 3. Scan smaller ranges
./cctvscan 192.168.1.0/28
```

### Debug Mode

Enable debug mode for detailed troubleshooting:

```bash
# Debug mode
sudo ./cctvscan --debug 192.168.1.100

# Output includes:
# - Detailed scan configuration
# - Port discovery process
# - Detection method details
# - Error messages
```

### Performance Tuning

#### For Fast Scanning

```bash
# High rate, default ports
sudo ./cctvscan --rate 10000 192.168.1.0/24
```

#### For Thorough Scanning

```bash
# Comprehensive mode, extended timeout
sudo ./cctvscan --comprehensive --timeout 2h 192.168.1.0/24
```

#### For Large Networks

```bash
# High threads, custom rate
sudo ./cctvscan --threads 100 --rate 5000 10.0.0.0/8
```

## Best Practices

### 1. Start with Default Mode

```bash
# Quick reconnaissance first
sudo ./cctvscan 192.168.1.0/24

# Then comprehensive for interesting targets
sudo ./cctvscan --comprehensive 192.168.1.100
```

### 2. Use Appropriate Timeouts

```bash
# Small networks: 30 minutes
sudo ./cctvscan --timeout 30m 192.168.1.0/24

# Large networks: 2+ hours
sudo ./cctvscan --comprehensive --timeout 4h 10.0.0.0/8
```

### 3. Monitor Resource Usage

```bash
# Check CPU and memory usage
top -p $(pgrep cctvscan)

# Adjust threads if needed
sudo ./cctvscan --threads 50 192.168.1.0/24
```

### 4. Save Results

```bash
# Save to file
sudo ./cctvscan 192.168.1.0/24 > results.txt

# Save with timestamp
sudo ./cctvscan 192.168.1.0/24 > "scan_$(date +%Y%m%d_%H%M%S).txt"
```

### 5. Use Debug Mode for Troubleshooting

```bash
# Debug mode for issues
sudo ./cctvscan --debug 192.168.1.100 2>&1 | tee debug.log
```

## Security Considerations

### 1. Legal Compliance

- Only scan systems you own or have explicit permission to test
- Check local laws and regulations
- Obtain written authorization before testing

### 2. Network Impact

- Use appropriate scan rates to avoid overwhelming networks
- Monitor network performance during scans
- Consider scanning during off-peak hours

### 3. Data Protection

- Secure scan results and logs
- Follow data protection regulations
- Implement proper access controls

### 4. Responsible Disclosure

- Report vulnerabilities to vendors
- Follow responsible disclosure practices
- Provide detailed technical information

## Support

For issues, questions, or contributions:

1. Check this usage guide
2. Review the README.md
3. Check GitHub issues
4. Submit new issues with detailed information

## License

MIT License - See LICENSE file for details.
