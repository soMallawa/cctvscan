# CCTVScan Console Design System

Status: Live Dispatch, composition B (board plus inspector), chosen under the user's explicit delegation: “Choose the strongest layout yourself and proceed with the full build.” Product/access truth is confirmed in PRODUCT.md. Production implementation is authorized. Publication remains a separate approval.

## Operating Scene and Direction

A private operator checks queues, failures and evidence on a desktop during ordinary work, then checks status on a phone. Light neutral navigation/settings preserve readability; a bounded graphite execution board creates focused monitoring without turning every form into a dark neon security dashboard.

Live Dispatch borrows the legible fixed columns, persistent changes and segmented run document of a dispatch board, not decorative travel imagery. Job identity stays stable as state changes. Real names, scope, phase, worker and elapsed time are first-class. No oversized hero metrics, gauges, split-flap animation, barcodes, fake telemetry or boarding-pass ornament.

## Reference and Concept Provenance

Explored directions: network dispatch board; broadcast patch panel; service-manual workspace; railway occupancy board; logic-analyzer timeline; technical flight strips; metrology worksheet. Seed `adabfb38` assigned the service-manual workspace. The fused boarding/gate-board challenger improves both operator identification and product clarity for queue/runner monitoring, so Live Dispatch is the proposed primary direction; service-manual workspace remains an alternate. Craft reference: https://impeccable.style/worlds/cards/vernacular-ephemera-boarding-pass-and-gate-board.webp . Do not copy its uppercase-heavy specimen typography or literal travel objects.

## Token Spine

Provisional primitives: paper #F5F6F7, surface #FFFFFF, inset #ECEFF1, ink #141A1D, muted #56616B, divider #DCE1E4, graphite #171E23, graphite-raised #222B32, graphite-text #F6F8F9, graphite-muted #B1BCC5, signal yellow #F4CE39, signal ink #201C0B. Functional success/warning/error colors are reserved for status, always accompanied by text/icon.

Semantic layer: page/surface/subtle/monitor backgrounds; primary/secondary/inverse text; action/action-foreground; border/focus/error/success. Component layer: navigation current item, form controls, job row, event line, phase strip and primary button. Quasar brand variables map to these semantics rather than introducing a separate default Material blue.

Use a readable humanist/system sans family for interface text, tabular numerals throughout operational values and monospace only for IP/port/code measurements. Body 14-16px; page headings 28-34px with restrained negative tracking. Sentence-case labels. Do not turn all navigation into uppercase monospace.

Spacing scale: 4,8,12,16,24,32,48. Input radius 6px; content panels 12px; dialogs 14px. Desktop navigation about 224px, compact below 1024px and drawer below 768px. Table rows 44-48px. Touch targets at least 44px.

## Surface Contracts

Jobs/workspace is dominant, with a small factual summary strip rather than an equal metric-card grid. Primary action is New scan. Selection opens a clear detail pane; full job detail remains a linkable route. Job monitor presents real phase events, indeterminate discovery where the denominator is unknown, and clearly labeled measured versus configured rates.

New scan uses a proper work surface with progressive sections: Scope, Discovery, HTTP and proxy, Optional inspection, Runner and Review. Advanced settings remain accessible and searchable; no arbitrary CLI textbox. Proxy coverage caveat is visible before enqueueing.

Results favor an evidence table and an inspector over camera thumbnail tiles. Images load only through protected artifacts. Runner health, proxy configuration, schedules and settings share the same visual grammar. Empty, loading, error, partial and reconnecting states have distinct truthful copy.

## Interaction and Accessibility

Visible keyboard focus, semantic tables/forms/buttons, labeled actions and no color-only status. UI updates preserve selected rows and user scroll position. Under 200ms opacity/color transitions; no periodic blinking or decorative live pulse. Reduced-motion preference disables nonessential transitions. Mobile hides lower-priority columns but retains identity, state and actions; detailed data is available in the inspector, not discarded.

## Composition Probes

Three forthcoming synthetic design comps must keep this system fixed and vary composition: queue-first full board; split board plus inspector; timeline-led current run with subordinate queue. They are design references, not screenshots of a functioning app. Never seed fictional metrics into the real application.
