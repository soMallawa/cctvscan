# CCTVScan Operational Usage

This document supersedes the earlier usage examples and guessed performance claims. See [README.md](README.md) for build/setup, all new controls, proxy semantics and result definitions.

## Safe sequence

1. Build the current source: `make build`.
2. Inspect interfaces/routing if using Masscan: `./bin/cctvscan --list-interfaces` and `--masscan-route-check` are non-scanning diagnostics.
3. Validate the exact authorized scope: `./bin/cctvscan --plan --ports 80,443,554 --unprivileged 192.0.2.1`.
4. Run with an explicit output directory and conservative rates.
5. Read the unique `scan-*.json` result, per-host/per-endpoint state and exit status. Empty findings are not automatically a successful negative scan.

Documentation addresses below are examples only, not instructions to scan public networks.

```sh
./bin/cctvscan --unprivileged --ports 80,443,554 --http-rps 5 --host-workers 3 --output results 192.0.2.1
./bin/cctvscan --plan --comprehensive 192.0.2.0/24
./bin/cctvscan --plan --ports 0-65535 --wait 2 --retry 1 192.0.2.1
```

The default and comprehensive presets contain **79 and 685 unique ports**, respectively. `--ports` and `--comprehensive` are mutually exclusive. A supplied full range remains a full range. Compact IPv4 CIDRs are not materialized into millions of strings. The 1,048,576-address upper-bound guard must be deliberately raised or disabled for larger scopes; this never samples them.

## Optional stages

Neither `--creds FILE` alone nor discovery of a login page enables authentication attempts. Add `--check-credentials` deliberately. The existing checker supports explicit HTTP Basic challenges only, not Digest/forms or bypass. Values are redacted from console/JSON. Completion states distinguish no test, no match, a verified result and an incomplete/canceled attempt.

Snapshots require `--capture`. They are validated, bounded images saved under `<output>/snapshots`; no background capture remains after the host result is reported. Raw RTSP candidates are not downloaded or treated as playable. ONVIF UDP discovery requires `--onvif`; normal TCP/HTTP discovery does not silently send it to every host.

## Explicit proxy

Load the proxy URL securely into `CCTVSCAN_HTTP_PROXY`, or use `--http-proxy-env NAME` to select another environment variable. Do not supply actual secrets in argv, issue reports or shell history. All first-party HTTP/HTTPS methods and redirect requests use this policy; `NO_PROXY` does not bypass it. Configuration/proxy failures do not fall back to direct HTTP. HTTPS proxy identity stays verified even though camera target TLS is permissive.

The raw discovery/verification TCP sockets, Masscan packets, RTSP and UDP are **not** HTTP proxy traffic. A commercial HTTP proxy is not a drop-in tunnel for the entire pipeline or OS-level egress enforcement. See the detailed limitations in README.

## Timing and resource controls

`--timeout` starts before target normalization; `--host-timeout` bounds each host's post-scan stages. `--host-workers`, `--http-requests`, `--http-per-host` and `--http-rps` are separate controls. Request slots cover response-body lifetime and redirects. `--rate`, `--threads`, `--wait` and `--retry` control the discovery/verification side, not HTTP concurrency. Verification lowers rate once, not twice.

Naabu is a pinned external library; an in-progress connect or its configured warm-up can form a shutdown tail after context cancellation. The application does not claim hard real-time cancellation of that dependency. Its optional stdin/update paths are disabled; no scanning-time installation is performed.

## Output contract

Reports contain a schema version, requested compact targets/address upper bound, selected port specification, verified ports, separately labeled unverified discovery on failure, and sorted per-host results. Credential values, original error objects and raw body snippets are excluded. Output uses private permissions and atomic publication under a unique filename. Capture errors and request/proxy failures remain visible as stage errors.

- `candidate`: generated protocol path only, no successful response or playback implied.
- `media_header`: payload/header media evidence, not continuous video/playback proof.
- RTSP `Any=true`: an RTSP response was parsed, even if 401/403/404; inspect status/auth fields.
- Vendor/CVE hints: heuristic identity and curated research leads, not confirmed vulnerabilities.
- `complete`: requested application work completed without recorded stage errors, not a blanket security verdict.
- `inconclusive`, `failed`, `canceled`: inspect the saved per-stage outcome; the process exits nonzero.

No real camera, account-specific commercial proxy, authentication bypass or public network scanning was used to verify the audit repairs. Local integration fixtures and a fake Masscan process cover the actual CLI path.
