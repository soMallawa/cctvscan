# Console Delivery Scope

## User-operable core

- Quasar/Vue console: job dispatch, server-validated scope preview, explicit authorization, persistent job details, real event history and live SSE updates.
- PostgreSQL/River queue; isolated scanner subprocess per attempt; fenced writes, pending cancellation, explicit retry-as-new-job and uncertain/orphan recovery without automatic rescanning.
- Paginated host results and protected, integrity-checked stored images; JSON/CSV exports. Vendor/CVE leads and media-header evidence are not confirmed vulnerabilities or playback.
- Encrypted versioned proxy profiles, write-only credential imports, reusable scan profiles, timezone-aware schedules and policy settings.
- Runner heartbeat/capability/RSS view and statistics derived from saved records, not invented telemetry.
- Hardened Docker Compose with private sockets/volumes, loopback web binding, limits, health probes and separate backup/fresh-project restore commands.

## Explicit limits—not hidden completed features

- The installed stack uses TCP CONNECT. Raw/SYN/Masscan needs a separately validated privileged/routing deployment; no raw-runner overlay is supplied or claimed verified.
- No continuous RTSP playback gateway or video recording. Stored HTTP still images are supported.
- HTTP proxy policy is application HTTP/HTTPS, not OS-wide egress enforcement. TCP discovery, RTSP and ONVIF UDP remain separate.
- Proxy diagnostics currently run in the API process against a deployment-owned fixed endpoint. This is NOT proof of routing/connectivity from a selected worker. A runner-specific diagnostic dispatcher, interface inventory, drain/queue-pause controls, CPU/disk telemetry and audit-log browser are not included.
- The target editor accepts explicit IP/CIDR text. A browser target-file importer is not included; the standalone CLI retains file input. Lists are never silently sampled.
- `retention_days` is persisted policy only; there is no automatic deletion service. Backups and deliberate offline cleanup remain operator responsibilities.
- One execution slot is the supported installation capability. A running container alone is not a successful scan or healthy worker.
- Local acceptance uses owned ordinary HTTP and generated JPEG fixtures. It proves the integrated application path, not real-camera fleet coverage or public raw-packet routing.
- Cloudflare Access JWT validation is implemented and covered by local tests; no production Access team/tunnel login is claimed. The currently installed acceptance origin is loopback-only, without application login.
- The capacity-constrained images package freshly built Linux binaries and the production web bundle. A clean Docker multistage source build is documented but is a separate build-environment claim.

## Launch and preservation

Follow `console-operations.md`. Never regenerate the encryption key of an existing deployment. Fresh restore uses a distinct Compose project, volumes, secret directory and port. Its worker stays stopped until an operator reviews queued jobs and schedules.

The source release excludes credentials, local databases, private image evidence, secret-bearing backups, generated binaries, node_modules and historic scan output. The installed QA database contains explicitly labeled synthetic jobs, not operational camera results. No Git commit, push, public tunnel, DNS change or public scan is performed by the delivery workflow.

## Verification receipts

Release-specific test/build/browser/recovery outcomes are recorded in the separate verification report, with failures and unexecuted gates distinguished from passes. This document defines capability boundaries; it is not a blanket claim that every item in the original aspirational implementation plan was delivered.
