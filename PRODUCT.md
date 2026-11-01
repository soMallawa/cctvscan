# CCTVScan Operations Console

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

A private single-operator console for Manthila Mallawa, managing authorized camera/network assessment jobs and evidence. The user confirmed this audience/access model on the current planning round. Team roles and a separate application login are not required.

## Product Purpose

Turn the existing Go CCTVScan CLI into a durable, inspectable operations application: configure scope and proxy policy, queue/run jobs, monitor actual execution, investigate results, and manage runners/settings without losing work on page refresh or service restart.

## Operating Context

Quasar frontend, durable backend/database, independent runners, live job activity and statistics, and an all-Docker deployment are explicit requirements. The intended private access layer is Cloudflare Zero Trust; no redundant app login. Verify Access identity at the application boundary rather than trusting arbitrary identity headers. Local integration must work without modifying existing tunnels or exposing the app publicly.

## Capabilities and Constraints

The existing scanner controls discovery/probing/HTTP proxying, credential-check opt-ins, HTTP image captures and ONVIF opt-ins. Preserve the repaired CLI as an independent tool. All applicable controls must have a typed console setting, safe diagnostic operation or clearly documented server-owned equivalent; arbitrary shell arguments and arbitrary server filesystem paths are not a console feature.

Use isolated scanner processes because proxy settings and HTTP budgets are process-global in the current engine. Preserve compact CIDRs and explicit scope; never silently sample. Persist jobs, attempts, events, searchable results, configuration versions and audit history in a strong database. PostgreSQL and a PostgreSQL-backed queue are architecture recommendations, not claims of existing functionality.

Keep discovered URLs, limited media evidence and playable video distinct. Vendor-associated CVEs are research leads, not confirmed device vulnerabilities. HTTP proxying is not OS-wide egress isolation: raw discovery/TCP discovery, RTSP sockets, ONVIF UDP and dependency-private/media networking are separate.

Live monitoring refers to job progress, event replay and runner health. Continuous camera-video playback is an unresolved additional feature that would require a media gateway; do not imply the existing CLI provides it. Do not auto-fetch camera media from the browser and bypass worker routing.

## Brand Commitments

Name: CCTVScan. Prefer plain operational language and an original, polished interface rather than generic admin templates. Quasar is the required frontend framework. The visual world is not chosen yet.

## Evidence on Hand

Published repair baseline: `f25a841b7d0b33978d45a6b9077fb05cb623f3c2`. Existing scanner source, regression tests, audit closure and safe localhost CLI fixtures are real. No real camera fleet, historical job telemetry or production metrics have been supplied; synthetic demonstrations must be labeled and cannot masquerade as actual scan history.

## Product Principles

1. Durable state, not browser-memory jobs.
2. Explain what actually happened, including partial/failed/canceled/unknown outcomes.
3. Isolation and secret handling are part of the product, not hidden operational assumptions.
4. Preserve complete requested scope and explicit opt-ins.
5. Make the common workflow fast while keeping all advanced controls inspectable.

## Accessibility and Inclusion

Keyboard-operable controls, meaningful focus, non-color-only status, reduced-motion support, readable tabular data and usable mobile views are the implementation quality baseline.
