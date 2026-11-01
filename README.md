# CCTVScan

A Go CLI for authorized IPv4 device discovery, camera-oriented fingerprinting and media endpoint inspection. Uses Masscan for external SYN discovery and Naabu for CONNECT discovery/verification. **An open port, vendor hint, protocol response, candidate URL, media header and playable stream are different kinds of evidence.** CCTVScan does not claim playback verification or exploitability.

## Private operations console

The Quasar web application, Go API, PostgreSQL-backed queue and isolated scanner worker are included. Start the loopback-only lab with:

```sh
export CCTVSCAN_UID="$(id -u)" CCTVSCAN_GID="$(id -g)"
export CCTVSCAN_AUTH_MODE=local
export CCTVSCAN_PUBLIC_ORIGIN=http://127.0.0.1:14910
# Run once for a NEW installation; never replace an existing encryption key.
python3 deploy/init-secrets.py
docker compose --parallel 1 --env-file /dev/null build
docker compose --env-file /dev/null up -d --wait
```

Open `http://127.0.0.1:14910`. This is a local lab, not public authentication. For remote access, configure and verify Cloudflare Access before exposing a tunnel. See [console operations](docs/console-operations.md) for secret preservation, resource limits, backup/restore, tests and deployment boundaries. See [delivery scope](docs/console-delivery.md) for verified capabilities and explicit limitations. The standalone CLI remains independent.

## Build a fresh binary

Requirements: Go 1.25+, C compiler and libpcap development headers/libraries. External SYN discovery also needs Masscan installed before running; CONNECT discovery does not need the Masscan executable. Install system dependencies during setup, never during a scan.

```sh
# Debian/Ubuntu setup, if missing
sudo apt-get install build-essential libpcap-dev masscan
make build
./bin/cctvscan --help
```

The obsolete committed executable was removed. `make build` or `mkdir -p bin && go build -trimpath -o bin/cctvscan ./cmd/cctvscan` creates the current executable. Generated executables, results and snapshots are not source artifacts. Naabu is a pinned Go dependency, not a separate executable to download at scan time.

## Inspect the scope without scanning

```sh
./bin/cctvscan --plan --ports 80,443,554 192.0.2.0/24
./bin/cctvscan --plan --ports 0-65535 --unprivileged 192.0.2.1
./bin/cctvscan --plan --comprehensive 192.0.2.1
```

Examples use documentation-only addresses; replace them with your authorized scope. `--plan`, `--list-interfaces` and `--masscan-route-check` do not scan. No target is silently sampled.

- Default preset: **79 unique ports**.
- Comprehensive preset: **685 unique ports**, not the previously advertised 1000+.
- `--ports` explicitly overrides the preset, including `0-65535`; it conflicts with `--comprehensive` rather than being ignored.
- IPv4 CIDRs remain compact through parsing and are passed natively to backends. IPv6 is explicitly unsupported, not silently expanded.
- `--max-targets` defaults to 1,048,576 requested addresses. Overlapping ranges count toward a conservative upper bound. Increase it deliberately or use `0` to disable this guard. Oversized inputs are rejected, never truncated.

## Run an authorized scan

```sh
# CONNECT discovery; no Masscan executable or root required
./bin/cctvscan --unprivileged --ports 80,443,554 --output results 192.0.2.1

# External SYN discovery via Masscan, then exact-port CONNECT verification
sudo ./bin/cctvscan --rate 1000 --output results 192.0.2.0/24
```

Keep scan rates appropriate for your network. `--unprivileged` never invokes Masscan. Mixed loopback/external target entries are partitioned rather than changing an entire batch's backend. Verification groups hosts with identical discovered port sets: it never scans a union cross-product or implicitly expands to Naabu's top-port preset. A verification error remains an error; unverified discovery is not promoted to verified results.

## Optional operations are explicit

Normal discovery **does not test credentials, send ONVIF discovery packets, save snapshots or launch detached media work**.

- `--check-credentials --creds FILE`: explicitly enable the existing Basic-auth check using your supplied file. Only explicit Basic challenges are tested. The coordinator is bounded, stops on success/cancellation and joins all work before returning. Results distinguish `not_tested`, `no_match`, `found`, `inconclusive` and `canceled`. Values are not printed or serialized. Digest/form authentication is not implemented.
- `--capture`: explicitly save validated JPEG/PNG or first-part MJPEG images from media-evidenced HTTP endpoints. Files are bounded to 256 KiB input and 8 megapixels, re-encoded as JPEG, mode 0600, with hashed filenames. Capture is joined before reporting; a failure is not reported as a saved image. This is not RTSP capture or video recording.
- `--onvif`: explicitly send a unicast UDP WS-Discovery probe. Only namespace-valid, correlated NetworkVideoTransmitter ProbeMatches count as detection. An arbitrary UDP reply is not ONVIF proof.

Credential files use one `user:password` pair per line; blank lines and `#` comments are ignored. File changes are read afresh rather than hidden behind a stale global credential cache. A file over 1 MiB or malformed entry is rejected, not silently truncated.

## Bounded work and deadlines

| Control | Default | Meaning |
|---|---:|---|
| `--rate` | 1000 | Discovery packet/connection rate; verification is reduced once |
| `--retry` | 3 | Extra attempts; forwarded to Masscan/Naabu |
| `--wait` | 1 | Late-reply/warm-up seconds; forwarded to both backends |
| `--threads` | 25 | Naabu worker bound |
| `--host-workers` | 5 | Fixed post-scan worker pool |
| `--http-requests` | 5 | Shared concurrent first-party HTTP requests, held through body completion |
| `--http-per-host` | 2 | HTTP request slots per destination hostname, across ports |
| `--http-rps` | 10 | Shared HTTP request pacing, including redirects; 0 disables pacing |
| `--host-timeout` | 30s | Per-host post-scan wall-clock budget |
| `--timeout` | 30m | Overall context, including target parsing |

Raw RTSP/UDP sockets honor cancellation and deadlines. HTTP bodies, images, workers, caches and expansion are bounded. The pinned Naabu dependency may finish an in-progress dial/warm-up before returning after cancellation; it is not hard real-time process isolation. External scanner/protocol sockets are not governed by the HTTP request budget.

## Results and evidence

Every executed scan publishes a unique atomic `scan-*.json` under `--output` (directory default `.`), even for a canceled/inconclusive scan once discovery has started. Files are private (0600); successive runs do not intentionally replace one another. Failed validation does not start a scan. If the output directory cannot be written, discovery is not started.

- `State`: `complete`, `inconclusive`, `failed` or `canceled`.
- `VerifiedPorts`: confirmed discovery/verification output.
- `UnverifiedDiscovery`: only retained separately when verification cannot complete.
- Per-host and per-endpoint statuses retain failed requests instead of turning transport failures into empty negative findings.
- HTTP metadata covers every supplied HTTP-like port. Root metadata is reused for classification; neutral hosts do not trigger repeated deep path discovery.
- RTSP 401/403/404 counts as a protocol response, with status/auth metadata—not a working feed.
- Protocol path candidates have state `candidate`, status 0 and `Verified=false`.
- HTTP media responses have state `media_header` and payload/header evidence. `Verified=true` means this limited media evidence, **not** playback, decoding the whole video, camera identity or authorization.
- Camera/vendor classification is heuristic and is labeled as a hint/likely camera. Generic HTML, generic URL path names and short ambiguous vendor substrings are not sufficient vendor evidence.
- CVEs are a deliberately small source-verified vendor association list, with no placeholders. These are research leads, never confirmed device vulnerabilities. See [CVE sources](internal/cvedb/SOURCES.md).

Exit 0 means the executed pipeline completed without recorded stage errors, not that every host is a camera or every advisory applies. Exit 1 means a failed/inconclusive/canceled run or configuration/runtime error; Go's flag parser uses exit 2 for malformed/unknown flags. Read the saved state and stage errors, not just an empty list. Original error objects, body snippets and credential values are excluded from JSON.

## Explicit application-wide HTTP/HTTPS proxy

```sh
# Set this securely outside shell history; do not put a real credential in argv.
export CCTVSCAN_HTTP_PROXY='http://USER:PASSWORD@PROXY_HOST:PORT'
./bin/cctvscan --unprivileged --ports 80,443 --output results 192.0.2.1

# Select a different secret-bearing environment variable
./bin/cctvscan --http-proxy-env PRIVATE_PROXY --plan 192.0.2.1
```

All first-party HTTP/HTTPS methods and redirects use explicitly configured policy, including helper/enrichment clients. An explicit proxy ignores `NO_PROXY`, rejects malformed configuration and does not fall back to direct HTTP. HTTPS proxy certificates/hostnames remain verified independently of permissive target TLS for self-signed cameras. Proxy errors are sanitized. A plain forward-proxy HTTP response cannot always be attributed conclusively to a target.

**Not OS-level egress isolation:** Masscan/Naabu raw/TCP discovery, RTSP, UDP and any independently launched subprocess networking remain separate. Dependency-private transports are not guaranteed by first-party factories; optional dependency stdin/update paths are disabled in the scanner wrapper. Target TLS intentionally accepts self-signed/untrusted certificates and must not be interpreted as authenticated target identity. No live commercial proxy account was tested.

## Masscan routing controls

`--adapter` / `--interface` (aliases), `--adapter-ip`, `--router-ip`, `--router-mac` and `--masscan-route-target` are opt-in. Router MAC means the **next-hop** MAC, not the adapter's own MAC. No machine-specific adapter, gateway or MAC is baked in.

```sh
./bin/cctvscan --list-interfaces
./bin/cctvscan --masscan-route-check --masscan-route-target 192.0.2.1
./bin/cctvscan --masscan-route-check --adapter eth0 --router-ip 192.0.2.254
```

Use a route reference only for a single-route batch. Explicit overrides are validated and lookup runs once per batch. Diagnostics do not enable scanning.

## Development and verification

```sh
make build
make test
make vet
# Optional installed tools
staticcheck ./...
govulncheck ./...
```

`make test` runs the race suite. Regression fixtures use local servers, in-memory callbacks and fake scanner processes; no real camera credentials are required. Do not run every historical benchmark blindly: some upstream benchmark functions contain network target examples. Run the pure CVE/compact-target benchmarks explicitly as documented in [AUDIT-FIXES.md](AUDIT-FIXES.md).

See [USAGE.md](USAGE.md) for the operational contract and [AUDIT-FIXES.md](AUDIT-FIXES.md) for the finding-to-fix/test map, validation evidence and remaining boundaries.
