# End-to-End Audit Repairs

- Base: `38a7d50290deb7d3277e0304e316f59b19c6278a`
- Repair Branch: `fix/end-to-end-audit`

Scope: the 19 findings and eight optimization opportunities in the 2026-10-31 audit. Changes are local and intentionally uncommitted/unpushed pending owner approval.

## Finding Closure Matrix

| ID | Implemented correction | Regression / evidence |
|---|---|---|
| F01 | Generic HTML/JSON/XML/binary MIME types are not camera or stream evidence. Shared deterministic vendor classifier; HTTP media needs a recognizable payload/header. Neutral hosts stop after root metadata rather than broad redundant path requests. | `TestAuditHTMLNotCamera`, `TestNeutralHTMLNotStream`, `TestNeutralEndpointDoesNotTriggerRepeatedDiscovery`; valid generated JPEG and local CLI fixtures. |
| F02 | RTSP/RTMP/MMS paths remain `candidate`, status 0, `Verified=false`. HTTP `media_header` evidence is a separate state. No playback claims; canceled detection makes no successful stream entries. | `TestCandidatesAreNotSuccessfulStreams`; console uses candidate/media evidence labels. |
| F03 | Replaced early-return/goroutine-per-credential logic with a joined fixed-worker coordinator; cancel on success; fresh validated file reads; explicit typed states; credential checks opt-in. | `TestCoordinatorWaitsAndJoins` uses blocked in-memory callbacks only, including cancellation. No live authentication was tested. |
| F04 | Both classifiers use deterministic vendor-specific evidence before generic recorder words. Removed ambiguous `nvr`, `sd`, `bb`, `security systems`, `snc` and bare `uvr` vendor triggers. | `TestExplicitVendorBeatsGenericRecorder` covers Dahua NVR, SDK/BBS documentation, Foscam security systems, CP Plus and short ambiguous identifiers. |
| F05 | Removed unverified catalog entries, placeholders and cross-vendor records. Five remaining published identifiers were checked against CVE Program CNA records; sources committed alongside the data. Output says vendor association/research lead, not a vulnerability verdict. | `TestCuratedIdentifiers`; `internal/cvedb/SOURCES.md`; authoritative records saved with repair evidence. |
| F06 | Empty default sentinel distinguishes a preset from explicit ports. Explicit `0-65535` is preserved; port syntax is validated; `--ports` conflicts with `--comprehensive`; plans and JSON expose effective scope. | `TestExplicitAllPortsPreserved`, `TestNonScanningPlanHonorsExplicitControls`; actual fake-Masscan argv receipt. |
| F07 | `--unprivileged` selects CONNECT/Naabu, never Masscan. Pinned `runner.SynScan`/`runner.ConnectScan` constants; contradictory modes rejected. Mixed target entries are partitioned for backend choice. Dependency stdin/update checks disabled. | `TestNaabuModeConstants`, `TestUnprivilegedNeverUsesMasscan`; real localhost CLI CONNECT discovery. |
| F08 | HTTP endpoint/state/error metadata and populated host stage errors replace empty negative findings. Raw RTSP transport failures also mark the host incomplete. CLI writes a durable report and exits nonzero for incomplete stages; failed verification is never promoted. Credentials/original error objects/snippets are excluded from JSON. | `TestAuditProxyFailureNotEmptyResult`, `TestRTSPTransportFailureNotComplete`, `TestVerificationErrorsAreNotSuccess`, `TestCredentialValuesNotSerialized`; dead-proxy real CLI fixture exits 1, reports `inconclusive`, zero direct HTTP. |
| F09 | Fixed host workers check context before post-scan work; RTSP/UDP use DialContext, context-bounded deadlines and cancel-triggered socket closure. Overall context begins before target parsing. | `TestAuditRTSPHonorsCanceledContext`, `TestCanceledHostsHaveExplicitOutcome`, canceled expansion checks; scanner dependency shutdown boundary documented below. |
| F10 | Shared HTTP concurrency/per-host/rate controls cover all first-party methods, redirect requests and response-body lifetime. Host/path/credential task creation is bounded instead of creating a goroutine for every target/path/credential. | `TestAuditConcurrencyBound` repairs the old peak-100 reproduction; `TestBudgetCountsRedirectRequests`; worker/coordinator review. |
| F11 | Native compact IPv4 CIDRs, context-aware normalization/walking, a safe materialization limit for legacy expansion, explicit address-family/resource errors, no wrap-around loop or silent sampling. CLI exposes an address upper-bound guard. | `TestCompactRangesAndCanceledExpansion`, `TestZeroPrefixExpansionFailsBoundedly`, existing exact `/30` test; pure compact-range benchmarks. |
| F12 | Parsed safe host plus URL digest filenames, bounded image decoding/re-encoding, atomic private files, and returned write/capture errors. No raw URL slash becomes a directory or arbitrary HTML becomes `.jpg`. | `TestSnapshotSafeFilenameAndImage`; `TestCaptureOptInAndJoined` decodes the actual synthetic saved file at return. |
| F13 | Removed detached legacy capture from normal host processing. Capture is opt-in, synchronous/joined and stores a path only after success. Legacy helper delegates to the same checked snapshot path. | `TestCaptureOptInAndJoined` verifies both disabled and enabled behavior using a generated JPEG and local server. |
| F14 | Parse RTSP response status/auth headers independently of stream success. 401/403/404 remain protocol evidence, not working-feed claims. | `TestAuditRTSPStatusVsAvailability`; result/console status fields. |
| F15 | Successful public HTML is not a login page. GET inspects auth challenges/password forms directly, so HEAD-405 servers do not hide protected endpoints. | `TestAuditPublicPageNotLogin`, `TestAuditHeadUnsupportedFallback`; proxy login fixture now supplies an actual password form. |
| F16 | Retain metadata for all supplied HTTP-like ports; classify from reused per-port bodies and RTSP server identity; retain the per-endpoint detections. | `TestAuditMetadataNotFirstPortOnly`; RTSP branding follows the same classifier, with real RTSP header passed to it by processor. |
| F17 | Wait/retry reach both backends, Naabu uses its actual option defaults, retry zero still means one initial attempt, and verification rate is reduced once. Exact-port verification groups identical port sets rather than cross-multiplying hosts/ports. | `TestVerificationExactPairsAndSingleRateReduction`, `TestNaabuExactPairLocalhost`; fake-Masscan wait/retry/rate argv receipt. |
| F18 | Valid ONVIF NetworkVideoTransmitter namespace, fresh message ID, correlation and response parsing. Arbitrary UDP/error strings no longer become positive ONVIF results. ONVIF is explicit opt-in. | `TestONVIFRequiresCorrelatedProbeMatch` covers valid fixture, malformed/unrelated content and wrong correlation. |
| F19 | Replaced unordered overlapping login/vendor map matching with one deterministic classifier and a generic login fallback. | `TestLoginDetectionDeterministic` repeats the overlapping input 5,000 times; original login-system test now passes without exclusions. |

## Optimization Closure

| ID | Implemented result |
|---|---|
| O1 | CVE lookup calls the curated brand map directly; it no longer ranges across unrelated body cache entries. |
| O2 | Shared reusable HTTP client/transport pool; connection reuse and bounded idle/live connections instead of per-helper connection churn. Explicit proxy policy resets the pool at startup configuration. |
| O3 | Verification groups identical discovered port sets; no union cross-product. A real pinned-Naabu host:port integration exposed implicit default-port behavior, so that path was abandoned and regression-tested before the final fixture passed. |
| O4 | Reuse root metadata for classification; neutral fixtures make exactly one metadata request. Remove automatic duplicate path/capture work; deep HTTP media paths run only after camera/protocol evidence. |
| O5 | Fixed host workers and shared request budgets replace per-host/per-credential goroutine floods and nested path fan-out. Configurable queue/admission/rate boundaries remain independent from raw scanner packet rates. |
| O6 | Compact native target ranges; bounded legacy expansion; a digest-keyed body fingerprint cache capped at 1,024 entries; no indefinite full-response metadata or credential-file cache. |
| O7 | Atomic private versioned JSON result publication, visible incomplete states/nonzero exits, corrected 79/685 port counts and timing claims, fresh build instructions, Makefile and removal of the stale committed executable. |
| O8 | Reused clients and precompiled classification/software patterns after the larger architectural changes. No blind claim that more parallelism produces higher valid-result throughput. |

## Measured Local Results

Pure microbenchmarks run through `python3 qa/benchmark.py` in a disposable source copy, without scanning:

- CVE lookup after 1,000/10,000 distinct body inputs: **84–91 ns/op**, 48 B/op, two allocations. Actual retained body cache sizes were 1,000 and 1,024. The audit's former 10,000-entry linear lookup was approximately 53 microseconds. This is a lookup-only measurement, **not an end-to-end scan speedup**; the curated list is also smaller now.
- Compact `/16` normalization: **320 B/op**, approximately 2.7 microseconds. Compact `/0`: **304 B/op**, approximately 2.8 microseconds. These numbers measure retaining a range, not enumeration/scanning of its addresses.
- Neutral local CLI endpoint: **one HTTP request**, no false camera/stream, credential state `not_tested`.
- Explicit local proxy: **one proxy request, zero direct HTTP requests**. Raw CONNECT discovery/verification stays separate, by design.
- The same proxy shut down: **exit 1**, saved `inconclusive` report, zero direct HTTP fallback and no synthetic proxy credential in logs/JSON.
- Fake Masscan process receives explicit `-p 0-65535 --rate 4 --wait 7 --retries 2` unchanged and makes no network scan.

## Reproduce

```sh
make build
make test
make vet
python3 qa/verify_cli.py
python3 qa/benchmark.py
```

`qa/verify_cli.py` accepts `CCTVSCAN_TEST_BIN` to select a prebuilt current binary. It uses owned localhost HTTP/proxy fixtures and an isolated fake Masscan executable, never public targets for an actual scan. Some historical upstream benchmarks contain network target examples: do not run them indiscriminately. Development setup must already have Go/C/libpcap; no scan-time installation occurs.

## Post-Review Regression Refinements

An independent full changed-source review identified three additional closure defects. A separate fixer repaired them with observed behavioral REDs followed by focused GREENs (repeated five times):

- `TestCredentialDiscoveryFailurePreservesRootEvidence` and `TestLoginDiscoveryDetailedPartialEvidence`: requested discovery reports request/read/cancellation failures, preserves root/partial URL evidence and uses a sanitized incomplete credential-stage result. The fixture has an empty/comment-only credential file and sends no Authorization.
- `TestBudgetQueuedAdmissionSpacing` (host/global saturation) and `TestBudgetPacingCancellationReleasesSlots`: pacing happens after concurrency admission; delayed queued callers cannot bunch together using expired reservations; canceled waits release slots.
- `TestMixedCaseMultipartBoundaryDiscoveryAndCapture`: preserve multipart boundary parameter case through detection and capture using a generated JPEG.

The first independent review had no other blocking proxy/TLS, coordinator-join, exact verification-scope or credential-serialization finding. The focused independent re-review returned **PASS**, with no remaining blockers, security concerns or suggestions in its targeted scope. Its source-traced verdict is included in the final evidence bundle.

## Final Verification

- **76 top-level tests passed** across 12 tested packages; **111 test/subtest pass events**, **zero failures and zero skips**.
- Complete `go test -race ./... -count=1`: **PASS**, including the former fingerprint flake (not excluded).
- `go vet ./...`: **PASS**.
- Staticcheck v0.8.2: **PASS**.
- Fresh actual `make build` to `bin/cctvscan`: **PASS**.
- Real CLI direct/proxy/dead-proxy/fake-Masscan fixtures: **PASS**.
- The Go/module source SHA-256 stayed unchanged across the final matrix: `0c53395c4b6e9c1afdbc624ad4205a8f65550100745882a68cbe7c64e652b161`.

Final matrix and independent reviewer verdict are recorded in the delivered repair evidence. Validation includes the complete test/race suites without skipping the former fingerprint flake, vet, Staticcheck, a fresh build and real CLI integration. Staticcheck v0.8.2 requires its own Go 1.26.10 toolchain; the application build/test toolchain is Go 1.25.1 on Linux amd64.

## Boundaries And Remaining Maintenance

- No public network scan, real camera, real account credential check, authentication bypass or live commercial proxy account test. The coordinator is tested with in-memory callbacks. Snapshot tests use generated images only.
- Media-header evidence is not continuous playback or complete video validation. Vendor/CVE association is not proof of product identity, affected firmware or exploitability. Self-signed target TLS intentionally does not authenticate target identity.
- Explicit proxy guarantees cover application first-party HTTP/HTTPS, not raw scanner/RTSP/UDP sockets, dependency-private clients or process-wide egress. Plain HTTP forward-proxy replies have attribution limitations. Existing proxy authentication, redirect, CONNECT and independent HTTPS-proxy certificate regressions remain part of the full suite.
- Pinned Naabu can finish an in-progress dial or configured warm-up after cancellation. Application probes close on context cancellation, but there is no hard real-time guarantee for third-party scanner internals. SYN/privileged operation was not exercised against a network; routing/argv are covered by local fake processes and existing diagnostics tests.
- The dependency vulnerability backlog mentioned before this audit is not claimed remediated by these application fixes. No unreviewed dependency major upgrades or exploitability conclusions were substituted for the 19 source findings. A dependency-upgrade task needs its own compatibility/reachability verification.
- Baseline Semgrep TLS-minimum warnings were not treated as confirmed TLS vulnerabilities; permissive target certificate policy is intentional and distinct from strict HTTPS-proxy identity verification.
- This is a source/build handoff, not a published release. No commit or push has been performed.
