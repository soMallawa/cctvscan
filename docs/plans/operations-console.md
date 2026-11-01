# CCTVScan Operations Console Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Status:** Access/audience confirmed: private single operator behind Cloudflare Zero Trust, no extra app login. Implementation has started on `feat/operations-console` with test-first configuration/preview contracts. Visual direction is being selected. No deployment, commit or push authorized for this feature.

**Goal:** Build a complete Quasar operations interface around the existing scanner: durable jobs, reliable isolated execution, live monitoring, searchable evidence, statistics, proxy/settings management and reproducible Docker deployment.

**Architecture:** A Quasar/Vue frontend talks to a Go control-plane API. PostgreSQL stores configuration, immutable job inputs, attempts, durable events, results and audit history; a PostgreSQL-backed queue dispatches work to independent runner services. Every attempt launches a separate scanner process so the scanner's process-global HTTP proxy/limiter configuration cannot leak between jobs.

**Tech Stack:** Quasar + Vue 3 + TypeScript + Pinia + Vue Router; Go HTTP API; PostgreSQL with pgx and migrations; River PostgreSQL-backed queue subject to pinned-version/toolchain compatibility verification; SSE with durable event replay; Docker Compose with separately hardened web/API/runner/database services. No Redis requirement merely to keep jobs alive.

## Confirmed Requirements and Open Decisions

Confirmed: preserve the existing Go scanner; use Quasar; provide jobs, runners, live monitoring, statistics, a durable database, Docker packaging and full applicable scanner/proxy settings. Existing evidence/privacy fixes and default opt-ins must survive unchanged. Build and verification use owned local fixtures, not public scans or live credentials.

Recommended first deployment: a private, single-workspace console, with identity supplied by verified Cloudflare Access where exposed; no redundant application login. This is a recommendation, not a newly confirmed user requirement. Decide whether multiple operators/roles or self-contained application login are required before defining authorization and data ownership. Visual direction remains undecided; do not silently choose a generic Quasar admin template.

Live monitoring initially means execution progress, runner health and job events. Displaying continuous RTSP video requires a separate authenticated media gateway; it is not a capability of the current scanner or a plain browser video element. Existing validated HTTP capture artifacts are in scope. Do not auto-load discovered URLs directly from the browser and bypass worker proxy policy.

## 1. Product Surfaces

1. **Overview:** actual active/queued/finished jobs; current runner capacity; recent failures; useful time-series based on stored observations, not invented demo metrics.
2. **Jobs:** paginated/searchable list; state, phase, age, scope, configuration, proxy version, runner and outcome; bulk actions bounded and individually authorized.
3. **New Scan:** target text/file import, exact scope preview, port presets/custom ports, engine/mode, limits/timeouts, proxy selection and explicit optional credential/ONVIF/capture switches. Review effective configuration before queueing.
4. **Job Detail:** persistent state/phase timeline, measured counters, event feed, structured errors, cancellation, retry-as-new-attempt, downloads and full configuration snapshot. No fake percentage during unknown-size discovery.
5. **Results:** host/port/brand/protocol/state filters; endpoint evidence, media candidates versus header evidence, research-lead CVEs and captured-image evidence. Never relabel candidates as playable streams or CVE leads as confirmed vulnerabilities.
6. **Runners:** heartbeats, online/offline/draining state, configured capacity, measured CPU/RSS/disk, capabilities, interfaces and route diagnostics from the selected runner rather than the API container.
7. **Proxy Profiles:** HTTP/HTTPS scheme, endpoint, credentials, immutable version, assignment, worker-side test, sanitized errors and clearly displayed protocol coverage.
8. **Profiles and Schedules:** reusable scan settings, optional timezone-aware schedules, priority, concurrency/resource limits, retention and audit history. A schedule creates immutable jobs, not mutable live runs.
9. **Settings:** access mode, worker budgets, retention, maintenance and database/backup health. Never return database or encryption keys to the browser.

Every surface needs loading/empty/error/partial/reconnecting states, keyboard navigation and useful mobile behavior. Build a deliberate token system and surface brief after the product/access decision; do not start with a stock card-grid dashboard.

## 2. Complete Scanner-Control Coverage

Source of truth: `cmd/cctvscan/main.go` at published repair commit `f25a841`.

| Existing Control | Console Contract |
| --- | --- |
| Positional targets and target-file input | Typed IP/CIDR inputs and bounded file upload. Server creates a job-private normalized target file; clients cannot provide host filesystem paths. |
| `--plan`, `--max-targets` | Preview returns compact normalized scope, address upper bound and unique port count without scanning. Preserve full requested scope; reject explicitly, never silently sample. Server policy can impose a stricter maximum. |
| Default / `--comprehensive` / `--ports` | Mutually exclusive preset/custom controls; preserve full port range including zero when explicitly requested. Counts come from backend validation, not duplicated frontend guesses. |
| `--rate`, `--retry`, `--wait`, `--threads` | Validated numeric fields with units, defaults and policy ceilings. Show configured rate separately from measured throughput. |
| `--privileged`, `--unprivileged` | Explicit mode choice tied to runner capabilities; normal deployment defaults to CONNECT. Reject conflicting/unsupported modes before dispatch. |
| `--adapter`, `--interface`, `--adapter-ip` | One canonical adapter control; source address selected/validated against runner capabilities. Alias compatibility remains in CLI. |
| `--router-ip`, `--router-mac`, `--masscan-route-target` | Advanced routing section with next-hop terminology; per-runner diagnostics and explicit single-route-batch caveat. No machine-specific defaults. |
| `--masscan-route-check`, `--list-interfaces` | Separate read-only runner diagnostics, with timeouts and no public-target scan as a side effect. |
| `--timeout`, `--host-timeout`, `--host-workers` | Overall job budget, host budget and host worker count; policy validation also applies to API clients. |
| `--http-requests`, `--http-per-host`, `--http-rps` | Per-attempt HTTP limits; reserve capacity against worker/installation limits. Account for aggregate limits across concurrent attempts rather than describing each process's limits as global across the fleet. |
| `--http-proxy-env` | Select a versioned secret profile. Runner injects a dedicated environment value into that child only. Browser/API cannot select arbitrary host environment variables or see the URL secret. |
| `--check-credentials`, `--creds` | Explicit default-off setting and secret-set reference. Runner materializes a private temporary file; never arbitrary host paths, plaintext exports or automatic real-account QA. |
| `--capture` | Explicit default-off bounded image capture; protected artifact viewer/download and retention. Not continuous video recording. |
| `--onvif` | Explicit default-off UDP discovery with direct-network scope warning. |
| `--output` | Server-owned per-attempt output root, never an arbitrary user path. Results and artifact manifests imported durably. |
| `--debug` | Bounded, sanitized diagnostic events with secret-redaction tests. Never dump environment/credentials. |
| `--help` | Contextual field help plus CLI compatibility documentation; not an executable arbitrary-argument field. |

## 3. Durable Data Model

Use SQL migrations and relational constraints, not an unstructured JSON-only job store. Proposed tables:

- `workspaces`, `actors`, optional `memberships`: determined by confirmed access model.
- `scan_profiles`, `profile_versions`: validated reusable defaults and immutable snapshots.
- `proxy_profiles`, `proxy_versions`, `secret_versions`: encrypted write-only secrets, ownership and rotation metadata.
- `jobs`: immutable requested/effective scope and settings, requester, idempotency key, priority, scheduled time, queue reference and lifecycle timestamps.
- `job_attempts`: attempt identity, runner, execution token, phase, lease/heartbeat, child identity, cancellation request, exit code, normalized outcome and versioned engine provenance.
- `runners`, `runner_capabilities`: heartbeat, capacity, supported modes, diagnostic metadata and maintenance state.
- `job_events`: monotonically identified durable events with bounded payloads and retention. Index `(job_id, event_id)` for replay.
- `host_results`, `port_results`, `endpoint_results`: typed searchable columns with per-attempt uniqueness; bounded JSONB holds versioned scanner-specific details.
- `artifacts`: opaque ID, owned relative storage key, MIME, size, content hash, attempt and retention; never trust a user-supplied filesystem path.
- `schedules`, `schedule_firings`: timezone and missed-run policy; uniqueness on `(schedule_id, scheduled_for)` prevents duplicate scheduled jobs.
- `audit_events`: actor/action/object/time and sanitized change metadata, not secret values.

Index queue eligibility, job state/time, runner heartbeat, host/port lookup and per-attempt result identity. Paginate large lists. Do not store a million-address CIDR as a million-row expanded input list. Use bounded connection pools, explicit transactions, migration locking, backup/restore verification and documented retention. JSON/CSV exports are streamed and spreadsheet-formula safe.

## 4. Job State and Execution Contract

Lifecycle: `queued -> claimed -> running -> succeeded | inconclusive | failed | canceled | timed_out | orphaned`. Cancellation is a durable request; an active attempt is `cancel_requested` until its process tree has actually stopped. Keep lifecycle distinct from discovery/probing/report-import phases and from per-host evidence outcomes.

- Durable enqueue and job creation are transactional. Caller idempotency keys prevent duplicate clicks/reconnects from enqueueing duplicate jobs.
- Use a mature PostgreSQL queue if toolchain/API verification passes; do not silently replace it with an in-memory goroutine queue. Verify dispatch, lease recovery and priority/concurrency behavior against real PostgreSQL.
- Use parameterized SQL and argument arrays (`exec.CommandContext` plus explicit process-group lifecycle), never shell-concatenated targets/flags.
- One scanner child per attempt prevents global HTTP proxy/limiter cross-talk. Child environment is allowlisted; no inherited provider/host secrets.
- Record engine version, profile version, proxy version and normalized effective arguments without secrets. Existing CLI remains usable independently.
- Cancellation/deadline sends graceful termination to the process group, waits a bounded grace, then kills the group if needed. Collect/reap child processes and import trustworthy partial results without marking them complete.
- Database/heartbeat failure causes the runner to stop claiming and fail closed on active leases. Execution tokens fence stale database writes.
- Fencing does **not** guarantee exactly-once network effects. A lost/unknown running attempt becomes `orphaned`, not silently auto-rescanned. Infrastructure retries before execution are bounded; after execution starts, retries require an explicit new attempt and show prior uncertainty.
- Queue pause stops dispatch; runner drain finishes existing work. Do not advertise true mid-scan pause/resume without durable engine checkpoints. Cancel/retry is not resume.
- Schedule dispatcher uses database coordination and unique firing keys. Default missed-run policy skips missed periods and records them rather than replaying a backlog of scans.

## 5. Live Monitoring and Statistics

The existing CLI emits human text and a final JSON report, not a structured progress stream. Add an optional versioned NDJSON event interface and observer hooks without breaking current stdout/report behavior. Do not infer exact progress by scraping terminal messages.

Events: attempt started, phase started/completed, discovery evidence, verification outcome, host processing completed, structured stage error, bounded metrics observation, artifact ready and terminal outcome. Each carries schema version, attempt identity, sequence, timestamp and bounded sanitized data.

Runner imports events in bounded batches; persist critical transitions/results before publishing. SSE replays persisted event IDs using `Last-Event-ID`; database notification is a wake-up optimization, never the sole event store. Bound subscriber buffers and log history; reconnect/replay or resynchronize rather than blocking the scanner or losing authoritative state.

Show measured host completions, verified ports, evidence categories, failures, elapsed time and queue age. Discovery has an indeterminate progress display until the denominator is known; the requested address upper bound is not a measured completion denominator. Separate configured packet rate from measured HTTP starts/latency and runner CPU/RSS. Display stale heartbeats, unknown ETA and disconnected monitoring explicitly. Deduplicate result/event imports across reconnect/retry.

## 6. Proxy and Secret Guarantees

- Preserve all-method application HTTP/HTTPS proxy routing, redirects, no direct fallback, explicit-proxy precedence over `NO_PROXY`, and independent strict HTTPS-proxy TLS verification.
- Keep proxy selection per attempt; rotating a profile creates a new version and never silently changes a running job's route. Define retention/revocation for old versions still referenced by jobs.
- HTTP(S) proxy coverage excludes raw discovery packets, TCP connect discovery, RTSP sockets, UDP ONVIF and dependency-private/media networking. Show this in setup, review and detail screens. If an operator requires all egress proxied, reject incompatible modes instead of implying HTTP settings provide OS isolation.
- Encrypt stored proxy/credential secrets with authenticated encryption; master key comes from a mounted Docker secret or protected external key source, never the database/image/Git. Record key version; document rotation and recovery.
- Secret fields are write-only. Editing non-secret fields must not erase a retained password; explicit clear/replace actions are separate. Redact URL userinfo, tokens and configured secrets in logs/events/results/exports/errors.
- Proxy tests execute on a chosen runner and against a fixed deployment-approved test endpoint, with bounded timeout and separate proxy/target evidence. No arbitrary API-side URL-fetch test endpoint. QA uses owned local proxy fixtures only.
- Captures are served through the authenticated artifact API, not by loading discovered URLs in the user's browser. No untrusted HTML rendering or auto-followed media URLs.

## 7. Docker and Access Design

`compose.yaml`: web reverse proxy/static Quasar build, API, CONNECT runner and PostgreSQL; persistent database/artifact volumes and backup procedure. Database has no published host port. Publish only the web entry on `127.0.0.1` by default. SSE proxy buffering disabled with appropriate idle timeout.

Non-root default services, dropped capabilities, no Docker socket mount, read-only roots where practical, writable job tmpfs/owned artifact volume, CPU/memory/PID limits, bounded logs, health/readiness checks, restart/drain behavior and pinned dependency/image versions. No scan-time package installation; images contain required scanner/runtime dependencies. Migrations run as an explicit controlled service/command with a distinct database role.

An explicit Linux raw-runner override/profile provides only justified network capabilities and routing mode when required. Do not grant the entire stack `privileged: true`; test real routing separately from raw-socket readiness. Host-network choices are documented exceptions with different isolation properties. Normal local QA uses CONNECT and fixture services.

Access decision is pending. For Cloudflare Access deployment, verify JWT signature/issuer/audience server-side, not merely an email header; protect SSE, mutation routes, artifact downloads and worker registration. For a self-contained/team deployment, implement the selected authentication/roles consistently. A no-auth dev mode must be explicit and loopback-only; never accidentally expose it through a tunnel. Apply body limits, origin/CSRF rules, authorization on every job/profile/artifact, and audit all mutations.

## 8. Build Sequence and Concrete File Boundaries

Each numbered unit is a build stage with smaller test-first changes; finish and verify one vertical slice before expanding it. No commits, pushes or public deployment without separate authorization.

### Stage A: Product, Contracts and Design

Files: `PRODUCT.md`, `DESIGN.md`, `.ui-craft/brief.md`, `docs/console-api.md`, `docs/console-settings-coverage.md`, `web/`.

1. Confirm audience/access model and establish product truth. Agree the operations-screen direction; persist tokens and state contracts before components.
2. Define typed create/preview/status/cancel/proxy/profile/event/result APIs and error codes, plus settings coverage mapped to the current CLI flag set.
3. Add schema-validation tests and a coverage test that detects future scanner controls missing from the console mapping. Shared server validation remains authoritative.

### Stage B: Database and Queue Foundation

Files: `internal/controlplane/store/`, `internal/controlplane/migrations/`, `internal/controlplane/jobs/`, `cmd/cctvscan-server/main.go`, `cmd/cctvscan-worker/main.go`, `qa/console/`.

1. Failing integration test: migration into a disposable PostgreSQL instance and transactional job enqueue/idempotency. Implement migrations/store and get GREEN.
2. Failing tests: duplicate claims, queue restart persistence, invalid lifecycle transitions and stale execution-token writes. Add queue adapter/state machine and get GREEN.
3. Failing tests: ownership/access rejection, private secret storage and write-only API responses. Implement only the confirmed authorization model and get GREEN.
4. Add priority, capacity reservations, runner registration/heartbeat and draining tests before implementation.

### Stage C: Safe Runner and Engine Events

Files: `internal/controlplane/runner/`, `internal/controlplane/secrets/`, `internal/events/`, `cmd/cctvscan/main.go`, `internal/processor/optimized.go`, `internal/httptransport/budget.go`, `internal/portscan/`.

1. Fake executable regression: hostile target text never invokes a shell, per-attempt paths/env are isolated, output is bounded and secret values are absent. Implement runner adapter.
2. Cancellation regression: fixture child and grandchild both stop/reap; durable cancellation survives API restart. Implement process-group shutdown and timeout handling.
3. Observer/event regression: real local host outcomes produce ordered versioned events without altering existing CLI results. Add opt-in events and hooks.
4. Crash/DB-loss regression: orphan detection, stale-write fencing and no automatic duplicate network execution. Implement recovery policy.
5. Two concurrent fake jobs with different proxy secrets must never cross-contaminate; malformed/dead proxy must never trigger direct fallback.

### Stage D: First Complete Quasar Slice

Files: `web/package.json`, `web/quasar.config.ts`, `web/src/layouts/ConsoleLayout.vue`, `web/src/pages/JobsPage.vue`, `web/src/pages/NewScanPage.vue`, `web/src/pages/JobDetailPage.vue`, `web/src/stores/`, `web/src/services/`.

1. Browser test RED: open empty Jobs, preview owned scope, submit a fixture job, see persisted detail and real events. Build the actual Quasar shell and API integration, not a disconnected mock dashboard.
2. Add live reconnect/replay, cancel confirmation/pending/terminal states, API errors, empty history and mobile layout tests, then implement.
3. Verify through the actual Compose web origin, not just direct API/Vite URLs.

### Stage E: Complete Operations Coverage

Files: `web/src/pages/ResultsPage.vue`, `RunnersPage.vue`, `ProxiesPage.vue`, `ProfilesPage.vue`, `SchedulesPage.vue`, `SettingsPage.vue`, `OverviewPage.vue`; corresponding `internal/controlplane/` handlers/services.

1. Add searchable paginated result/evidence views, protected capture/download and JSON/CSV export tests before implementation.
2. Add versioned proxy CRUD/test, secret replace/retain/remove and profile snapshot tests before UI and handlers.
3. Add runner capability/route diagnostics, measured statistics, retention and audit views backed by real database records.
4. Add timezone/idempotent schedule-firing and missed-run tests, then scheduling UI/service.
5. Close every settings-coverage row and every disabled/unavailable action with a truthful explanation; no unimplemented buttons.

### Stage F: Docker, Failure Tests and Release

Files: `Dockerfile`, `web/Dockerfile`, `compose.yaml`, `compose.raw.yaml`, `.dockerignore`, `deploy/`, `qa/console/`, `README.md`, `docs/console-operations.md`.

1. Build/runtime smoke tests use installed image code and normal service commands, not source mounts that hide packaging defects.
2. Recreate API/worker/database containers and verify persisted jobs, original evidence bytes, SSE replay and no accidental rerun.
3. Verify read-only roots, runtime UID, capabilities, loopback bindings, readiness, aggregate resource caps and backup restoration into fresh volumes.
4. Run full original scanner matrix plus backend, frontend and browser tests; request independent backend/security and visual review. Fix concrete blockers and rerun the complete affected matrix.
5. Deliver complete source, Compose/config templates, operations/backup/access documentation and verified local URL. Commit/push/deploy remain separately approved steps.

## 9. Required Verification Commands and Acceptance

Keep original scanner gates: bounded `go test ./...`, `go test -race ./...`, `go vet ./...`, Staticcheck and fresh CLI build/integration. Extend Makefile with explicit `test-console`, `test-console-integration` and `test-console-e2e` targets. Frontend scripts: `npm ci`, `npm run typecheck`, `npm run test:unit`, `npm run build`, `npm run test:e2e`. Container acceptance starts with `docker compose config --quiet` and an isolated disposable Compose project; use an empty env file and explicit fixture-only configuration so ambient proxy/provider settings cannot cause real calls.

Release blockers: duplicate active attempts, lost cancellation, unfinished subprocesses, stale writes, lost events after reconnect/restart, secret values in any response/log/export, proxy fallback/cross-talk, false stream/CVE/progress claims, incomplete CLI settings coverage, unprotected mutation/artifact routes, broken mobile/keyboard behavior, inability to restore database and artifacts, or a failed scanner regression matrix.

Verification data is synthetic/owned and labeled as such. Do not perform public scans, real credential attempts or operational camera capture to prove the console works.

## 10. Current Host Constraints

Planning inspection found approximately 785 MiB available RAM, no swap and 7.9 GiB free disk. This is evidence about this host now, not a recommended deployment size or a permanent capability limit. Avoid simultaneous frontend/Go/Docker builds; use bounded serial units, small isolated fixture services and monitor available resources before starting each heavy stage. Do not prune unrelated images/volumes or alter other services to make room. If safe headroom is insufficient, report the blocker and select a separate build/runtime host rather than risking the Hermes gateway.

## References Inspected

- Existing CLI and processor/report source in this repository, including process-global HTTP settings and final-only JSON output.
- https://quasar.dev/introduction-to-quasar (retrieved successfully during planning).
- https://www.postgresql.org/docs/current/explicit-locking.html (retrieved successfully during planning).
- River documentation at https://riverqueue.com/docs returned HTTP 403 in the planning environment. Verify queue APIs, licensing and a compatible pinned release through the official repository/documentation before committing to integration; do not invent API signatures.
