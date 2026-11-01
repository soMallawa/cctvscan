# CCTVScan Private Operations Console

Single-operator console; no application login. Cloudflare Access JWT verification is the production boundary. Nothing publishes a host interface other than loopback. API and nginx share a network namespace so API loopback authentication is usable in an **explicit local-only lab**, without trusting arbitrary forwarded identity headers.

## Build and start

Prerequisites: Docker with Compose v2+, Python 3, Linux. Reserve at least 2 GiB available memory and adequate disk for normal source builds. Build serially on constrained hosts. Never prune other projects to make space.

```sh
export CCTVSCAN_UID="$(id -u)" CCTVSCAN_GID="$(id -g)"
python3 deploy/init-secrets.py
export CCTVSCAN_ACCESS_ISSUER=https://YOUR-TEAM.cloudflareaccess.com
export CCTVSCAN_ACCESS_AUDIENCE=YOUR-APPLICATION-AUD
export CCTVSCAN_PUBLIC_ORIGIN=https://YOUR-PRIVATE-HOST
# Set your protected Access application and tunnel separately, pointing to port 14910.
docker compose --parallel 1 --env-file /dev/null build
docker compose --env-file /dev/null up -d --wait
```

Default origin is `http://127.0.0.1:14910`. Replace the illustrative issuer/audience/origin above with your actual values. Empty Access settings fail closed. Do not expose a tunnel without its Access application/policy. Tunnel/DNS changes are separate operations, not performed by Compose. A production JWT/edge login must be verified against the real Access team; local QA is not that proof.

For an owned, loopback-only lab, explicitly set `CCTVSCAN_AUTH_MODE=local` and `CCTVSCAN_PUBLIC_ORIGIN=http://127.0.0.1:14910`. Do not use this mode for a public origin. The API stays on literal loopback inside its namespace. The published web port is also loopback-only. No secondary password or invented login is required.

Secrets are 32 raw key bytes, a database password, and a private database URL under `deploy/secrets` (or `CCTVSCAN_SECRETS_DIR`). Never regenerate an existing key: saved proxy and credential ciphertext require the original. Files must stay owner-only and owned by the configured runtime UID. No secret values belong in Git, browser storage, URLs, service argv or reports. The one-shot init service owns only the work volume and has no network; steady-state API/worker/web are non-root with dropped capabilities and read-only roots. PostgreSQL uses password-authenticated Unix sockets, including local connections, and has no published port.

## Runtime boundaries

- Default worker has one execution slot, bounded memory/PIDs and CONNECT discovery. A job has an immutable configuration and a fenced attempt. HTTP proxy policy is first-party HTTP/HTTPS only, never a promise of OS-wide egress containment.
- Credential checks, ONVIF and capture are explicit opt-ins. A candidate URL is not verified media; HTTP success is not proof of a camera.
- Versioned native observations travel on a private close-on-exec pipe. Sanitized host outcomes and counts are committed before child exit. Final report reconciliation is deduplicated; it must not count hosts twice.
- Cancel is pending until the worker confirms the child process group has stopped. Missing runners become orphaned, not automatically retried. Retry is an explicit action because repeating network work can have effects.
- Raw/SYN/Masscan operation is not enabled in this stack. It needs a separately reviewed network/capability deployment and real routing validation. Do not add `privileged: true` or mistake container raw-socket capability for verified packet round trips.
- `retention_days` is currently persisted configuration, **not an automatic purge promise**. Retain backups and perform deliberate offline cleanup only; no background deletion is silently enabled.

## Health and resources

`docker compose ps` shows container health; `/api/health` and the Runners screen show authenticated database/worker state. API's container probe accepts 401 in Access mode solely as listener liveness: it does not manufacture a JWT or establish live DB readiness. The database has its own readiness probe. Worker health is its persisted heartbeat; a running container alone is insufficient.

Defaults: API 96 MiB, worker 512 MiB, DB 128 MiB, web 32 MiB plus a 24 MiB one-shot initializer. No swap, bounded CPU/PIDs. Change `CCTVSCAN_{API,WORKER,DB,WEB}_MEMORY` deliberately; an OOM is a failed/uncertain attempt, never a successful empty scan. Docker limits bound worker plus scanner/helper children in the same container. Serialize builds/tests and runtime acceptance on small hosts.

## Backup

Choose a private new destination outside the source checkout. Python invokes Compose with `--env-file /dev/null` unless `CCTVSCAN_COMPOSE_ENV_FILE` is set. Preserve the same environment, UID/GID and `COMPOSE_PROJECT_NAME` used to start the stack.

```sh
bash deploy/backup.sh /secure/path/new-backup
python3 deploy/backup_restore.py verify /secure/path/new-backup
```

Backup stops API and worker while dumping PostgreSQL and archiving exact private job bytes and key files, then starts those same services in a finally block. Schedule a maintenance window; an active attempt interrupted by stopping is uncertain/orphaned and will not silently replay. Backups contain sensitive evidence and credentials/key material: keep them owner-only, encrypt off-host copies, restrict access and test restoration. SHA256SUMS detects accidental corruption, not a hostile operator who rewrites the entire backup.

## Restore into a fresh project only

Do not overwrite the original volumes. Set a new project, new empty secret directory and unused loopback port/origin. The script rejects existing target containers and nonempty key directories.

```sh
export COMPOSE_PROJECT_NAME=cctvscan-restored
export CCTVSCAN_SECRETS_DIR=/secure/path/new-restored-secrets
export CCTVSCAN_PORT=14911
export CCTVSCAN_PUBLIC_ORIGIN=http://127.0.0.1:14911
bash deploy/restore.sh /secure/path/new-backup
```

Restore verifies hashes/archive paths, imports into fresh volumes and brings up **only DB/API/web**. The worker remains stopped: inspect queued jobs and enabled schedules before explicitly starting it. This prevents an unreviewed restored queue from initiating scans. Keep the original project until you compare jobs, event replay, result counts, private image hashes and decryption of a saved fixture profile. Do not copy original private paths or encryption keys into a public release.

## Upgrade and recovery

Use a backup/maintenance window, build a new immutable image, then recreate API, worker and web together. They share an API network namespace: recreating API requires recreating web too. Applied migration checksums are immutable. Do not edit previously applied SQL. Verify `/api/health`, runner heartbeat, event replay, historical evidence and a fixture-only new job after upgrades. Inspect orphaned attempts; retry only deliberately. Restarts never authorize new public scans.

## Capacity-constrained installed-image acceptance

`deploy/compose.prebuilt.yaml` builds runtime images from freshly tested local Linux binaries and `web/dist`. Build those with the pinned Go toolchain/libpcap and locked Node dependencies first. This verifies installed runtime code, not an unexecuted multistage source build. Record that distinction in release receipts. The production Dockerfiles also provide normal source builds.

## Checks

```sh
python3 qa/console/test_packaging.py
make test-console
make test-console-integration
make test-console-e2e
```

The integration target requires an explicit disposable `CCTVSCAN_TEST_DATABASE_URL`; never point it at production. `scanner_e2e` tests require a freshly built scanner and only create owned localhost fixtures. Browser QA must use the installed web origin; intercepted browser responses are not backend journey evidence. No commit/push/public deployment is implied by these commands.
