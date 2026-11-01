# Camera Detection, Evidence And Automatic Connection Checks

## Camera Decisions

New results distinguish **Supported**, **Possible** and **Unknown**. Supported means corroborating camera/vendor evidence was observed; it does not prove a physical camera, exact model or playable stream. Vendor names, generic Web Client titles, embedded web-server banners and isolated DVR/NVR keywords do not independently establish a camera. Failed probes remain Unknown, not verified non-cameras.

Possible root-page candidates can reach bounded HTTP media checks on their already-discovered candidate HTTP ports. Request budgets, per-host deadlines, selected-proxy routing and no-direct-fallback behavior remain in force. This upgrade does not itself start a scan. Older records are not retroactively relabelled or given invented response bodies.

## Offline Model Reference

The embedded CC0 catalogue is pinned to `7616a0a0944522159c3daad4a50988b740cb4d65` of `ch-bas/cctv-camera-database`. It contains 29,582 models, 235 brands and 420 documented connection references. Model candidates must be tied to retained device evidence; a catalogue entry is a reference, not a verified identification. Search and specifications operate without runtime dataset downloads. Catalogue paths are not permission to enable additional protocols or scan additional targets. No restricted iSpy database is copied.

## Existing Vendor-Specific Credentials

The credential resource preserves comment-based vendor sections in its encrypted content. Matching vendor entries are selected before explicitly generic entries. Other vendors' sections are not tried against a known vendor; unknown vendors receive only generic entries. Unrecognized annotated sections are excluded rather than silently converted into generic credentials. Flat legacy sets remain generic.

The installed vendor-aware default resource is imported from the existing `mycreds.txt`, not a new manually-entered list. New scan forms select it automatically when credential testing is permitted. Existing saved profiles/schedules referencing the previous default can be migrated without rewriting historical jobs. Scan-level choices still apply; no per-device credential or profile entry is required.

Credential values never belong in ordinary result JSON, observations, browser state or logs. The runner decrypts the selected set only into its private job workspace. Authentication acceptance is retained separately from media and frame outcomes.

## Authentication And Media Outcomes

HTTP Basic and supported HTTP Digest challenges are handled automatically. HTML/JavaScript login forms and vendor-specific challenge/session APIs are not implemented. Unsupported methods are reported, not bypassed. Credential checks run only when enabled and preserve the existing request policy.

- `found`: the challenged HTTP endpoint accepted a selected pair; not proof of a camera stream.
- `no_match`: attempts completed without acceptance or a reported transport failure.
- `not_tested`: no usable challenge/credential set; not a wrong-password verdict.
- `inconclusive` / `canceled`: transport, proxy or processing-budget failure; not confirmed rejection.

Normal camera scans also record bounded automatic connection outcomes: authentication accepted, media responded, and an actual frame decoded are separate facts. HTTP snapshot and live MJPEG checks validate image bytes, not HTTP 200 or MIME alone. RTSP checks negotiate supported authentication and require an actual decoded frame for frame verification. At most six deduplicated automatic connection candidates are considered per host; this is not exhaustive path coverage.

Only already-discovered, selected HTTP/HTTPS/RTSP ports are eligible. HTTP-only scans do not silently add RTSP or ONVIF. The selected proxy applies to HTTP/HTTPS checks with no direct fallback; RTSP is separate and is not sent through an HTTP proxy. Scan-level capture permission controls persistence of private image artifacts. Results show automatic evidence instead of a required manual connection form.

## Retained Root Responses

Only unauthenticated root GET response bodies are archived: at most **8,192 bytes per response** and **32,768 bytes per host record**. Original bytes are base64 encoded with SHA-256, capture time, byte count, capture limit, truncation flag and the selected WWW-Authenticate challenge. Initial/final URLs have URL secrets removed. Metadata includes status, server and content type.

Cookies, Authorization headers and credential-submission bodies are not archived. Root content is untrusted evidence protected by console authentication. The browser renders escaped text or downloads plaintext; it does not execute returned HTML or fetch its linked resources. The API accepts only bounded, integrity-valid archives. Generic raw-body and secret-field redaction remains enabled. Full responses, every header, JavaScript assets and multi-step authenticated sessions are not recorded.

An older result's missing body cannot be recovered from its extracted title. Slow proxy routes and exhausted deadlines remain inconclusive. Raising concurrency or trying more credentials does not add unsupported authentication methods.

## Verification And Interpretation

Acceptance uses owned loopback/private-network camera fixtures and ordinary web-service negative controls, never public devices. It exercises real image decoding, an authenticated RTSP video fixture, encrypted credential delivery, installed-worker processing, private artifacts and desktop/mobile browser rendering. Fixture success does not establish broad physical-camera accuracy or support for every catalogue model.

Review Possible/Unknown separately; inspect retained evidence and separate authentication, media and frame outcomes. Resolve proxy/request failures before interpreting negative findings. Existing historical jobs remain unchanged.
