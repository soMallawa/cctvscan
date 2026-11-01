# Console API Contract v1

Single operator; same-origin `/api`. API JSON is snake_case. Versioned persisted scan configuration is the existing `internal/controlplane/config.ScanConfig`. Defaults and policy come from the backend, not copied UI constants. Browser code never receives proxy/credential plaintext or filesystem paths. No built-in login. Cloudflare Access JWT validation is mandatory in access mode; explicit loopback development mode is separate.

## Resource Envelopes

Lists return `{items: [], total: number}`; empty lists are arrays, not null. Errors return `{error: "safe actionable message", field?: "field_name"}` with appropriate 400/401/404/409/413/500/503 status. Never forward raw subprocess/SQL/transport errors to the browser. All mutation JSON uses strict decoding, body limits, unknown-field rejection, same-origin validation and server-side validation. IDs are opaque UUIDs or validated reference IDs. No arbitrary SQL, commands, environment names or paths.

### Job

`{id, name, state, phase, config, priority, created_at, started_at, finished_at, error, result_count, camera_count, stream_count, inconclusive_count, cancel_requested, attempt, retry_of}`. Timestamps are RFC3339 UTC; unavailable timestamps are null. Config is an immutable ScanConfig snapshot. State: queued/running/canceling/completed/inconclusive/failed/canceled/timed_out/orphaned. Unknown runs are orphaned, not successful or automatically retried. Attempt/claim ownership is tracked in the DB. The scope upper bound is not a unique-IP count. Terminal job results are imported atomically with the terminal outcome. A valid report is required for completion.

### Event

`{seq: "decimal-string", job_id, kind, at, phase, message, data}`. SSE `id` is the same sequence. Kind is state/phase/metrics/log/artifact/runner. Safe messages only. Event IDs are durable PostgreSQL sequences, not per-process counters. JSON big IDs use strings. Replay after `Last-Event-ID` or `?after=`; bounded batches, heartbeat comments and slow-consumer limits. Phase progress may be unknown; no invented percentages/ETAs. The CLI needs real observer events for phase/metrics; do not manufacture live counters by estimating time.

### Runner

`{id, name, state, hostname, capabilities: ["connect"], slots, active_jobs, last_seen_at, rss_bytes, cpu_percent, scanner_version}`. Null measured values mean unavailable, not zero. Raw capability is opt-in. Heartbeat liveness does not claim the network or proxy is healthy.

### Proxy Profile

Read: `{id, version_id, name, scheme, host, port, authentication_present, created_at, last_test_at, last_test_ok, last_test_message}`. No full authenticated URL or username/password. Create/update input: `{name, url}`; write-only URL. Updates create immutable versions. Jobs pin version_id. No secret in localStorage, history, logs or exports. Diagnostic test only uses an explicitly configured test destination, per-profile transport, strict proxy TLS and no direct fallback. No global API transport mutation.

### Scan Profile, Schedule, Credential Set

Profile: `{id, name, config, created_at, updated_at}`; persisted config replaces only safe defaults, not runner-managed paths. Schedule: `{id, name, cron, timezone, config, enabled, next_run_at, last_run_at, created_at}`. Credential set reads: `{id, name, entry_count, created_at}`; create input `{name, content}` is write-only and encrypted. Credential testing stays opt-in; selecting a set does not silently enable it.

## Routes

- `GET /api/health`: `{ok, database, queue, version, auth_mode}`. Readiness is a real DB query; non-ready returns 503.
- `GET /api/bootstrap`: `{defaults: ScanConfig, policy: Policy, auth_mode, version, proxy_test_configured}`.
- `POST /api/jobs/preview`: `{config: ScanConfig}` -> existing `config.Preview`; same validation is repeated on create. Reference existence and worker capability are checked server-side.
- `GET /api/jobs?state=&search=&limit=&offset=` -> Job list. Bounded pagination; default 50/max 200.
- `POST /api/jobs`: `{name, config, priority, scheduled_at?, idempotency_key}` -> Job, 201. Same key/payload returns original job; conflicting reuse returns 409. Insert job and River queue item in one PostgreSQL transaction. No automatic retries after execution begins.
- `GET /api/jobs/{id}` -> Job.
- `POST /api/jobs/{id}/cancel`: durable queued cancellation or running cancellation request -> Job. Running job becomes canceled only after joined process cleanup. Repeated cancellation is idempotent. No mutation via GET.
- `POST /api/jobs/{id}/retry`: `{idempotency_key}` -> newly queued Job with `retry_of`, 201. Prior history remains intact. Only terminal jobs, no automatic rerun of an unknown network side effect.
- `DELETE /api/jobs/{id}`: terminal-only removal, audited; 204. Never delete live work to fake cancellation.
- `GET /api/jobs/{id}/events`: replayable SSE for one job.
- `GET /api/events`: replayable SSE across the single private workspace, including jobs and runner activity. UI maintains one global connection, bounded to 200 displayed recent events.
- `GET /api/jobs/{id}/results?limit=&offset=` -> results list; existing scanner HostResult JSON, safely redacted, wrapped as `{id, job_id, target, brand, state, result}`. `result` preserves truthful scanner evidence and research-lead distinctions.
- `GET /api/results?search=&job_id=&state=&limit=&offset=` -> same cross-job result list.
- `GET /api/jobs/{id}/export`: downloadable redacted JSON report. No internal paths, proxy/credential values or unsafe URL userinfo/query secrets.
- `GET /api/artifacts/{id}`: authenticated captured-image/report retrieval through server-owned artifact mapping. No path input or browser-to-camera fetch.
- `GET /api/overview?days=7`: `{counts: {queued,running,completed,inconclusive,failed,canceled,timed_out,orphaned}, series:[{day,jobs,completed,failed}], result_count, camera_count, stream_count, recent_jobs: Job[], runners: Runner[]}`. Values and series come from PostgreSQL and real runner measurements, no seed data.
- `GET /api/runners` -> Runner list.
- `GET /api/proxies` -> Proxy list.
- `POST /api/proxies`: write-only profile input -> redacted Proxy, 201.
- `PUT /api/proxies/{id}`: new version -> redacted Proxy.
- `POST /api/proxies/{id}/test` -> `{ok, message, elapsed_ms}` and persisted safe diagnostic status. Return actionable configuration error when no diagnostic endpoint is configured; do not perform an unrequested public connectivity check.
- `DELETE /api/proxies/{id}`: reject active/reference-invalidating deletion, 204 when safe. Referenced immutable versions remain available to queued/running jobs.
- `GET/POST /api/profiles`, `PUT/DELETE /api/profiles/{id}`: list/create/update/delete safe scan profiles.
- `GET/POST /api/schedules`, `PUT/DELETE /api/schedules/{id}`: list/create/update/delete schedules. Cron/timezone validated; next_run computed. Transactional due-run dispatch, unique occurrence key and explicit skip-missed policy prevent restart catch-up storms.
- `GET/POST /api/credential-sets`, `DELETE /api/credential-sets/{id}`: metadata-only reads, encrypted write-only import and reference-safe removal.
- `GET /api/settings`: `{policy, retention_days, auth_mode, public_origin, max_concurrent_jobs, version}`. Sensitive auth/secrets never returned.
- `PUT /api/settings`: `{policy, retention_days, max_concurrent_jobs}`. Validated, audited persistence. Cannot switch authentication or reveal key/DB configuration. Live execution slots must honor settings before acceptance; do not claim an effective change if a runner restart is required. Worker defaults one slot; concurrency cannot silently oversubscribe server-owned aggregate resource budgets.

## Runtime and Persistence Invariants

- PostgreSQL 17, pgx, River version compatible with available Go 1.25.1 (v0.40.0 probe confirmed go 1.25.0; latest v0.46.1 requires 1.26.0). Pin coherent River/driver versions and exercise actual migrations/transaction rollback. Never substitute in-memory jobs when PostgreSQL is down.
- Worker launches `bin/cctvscan` as a child process group using argv arrays, job-local proxy env and private inputs/output directories. Credentials never go in argv. Bounded output, joined cancellation/escalation, bounded real deadline, fenced writes and worker-loss reconciliation. No Docker socket/privileged container.
- Shared HTTP proxy settings must not mutate the API process default transport. Versioned encrypted profile resolution happens inside the worker. Connection/discovery/raw/RTSP/UDP/media boundaries remain visible.
- On unknown execution loss, mark orphaned and require explicit operator retry. PostgreSQL fencing prevents stale writes, not exactly-once network side effects.
- Credentials/capture/ONVIF disabled by default. Raw runners need explicitly granted capabilities. No process pause represented as resumable scan.
- Secret encryption uses authenticated encryption and record/type-bound AAD, a Docker-mounted key outside DB, no plaintext storage. Dumps exclude key; restore exercise includes separately secured key and artifacts.
- Worker samples and observer events are measured facts. Unknown/stale values remain unknown/stale. Persisted result and event bodies are size-bounded and sanitized, never raw terminal text assumed safe.
- QA DBs, fake scanner processes and owned localhost/container fixtures only. Integration tests use an explicit integration build tag; authoritative release gate runs those tests with a disposable real PostgreSQL fixture and reports their counts separately. No hidden skips as claimed coverage.
