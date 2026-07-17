---
phase: 06-public-donation-web-form-flow-b
plan: 07
subsystem: ui
tags: [nextjs, app-router, tanstack-table, tanstack-query, bff, i18n, rbac, shadcn]

# Dependency graph
requires:
  - phase: 06-public-donation-web-form-flow-b
    provides: "plan 01 donations.source column + GET /api/donations?source= filter + DonationListItem.Source; plan 05 (app) route-group split (AppShell wrapper, slate/blue :root theme)"
provides:
  - "SourceBadge component (flow_a slate/UserCog, flow_b blue/Globe), source.* i18n"
  - "authenticated GET /api/bff/queue BFF (status pinned pending_review + source-token → flow_a/flow_b mapping)"
  - "/queue page (Screen 11) — QueueTable + QueueSourceFilter, implements the previously-dead nav.queue link"
  - "Screen 1 source column (DonationTable) + Screen 3 source-aware creator label (DonationDetailView)"
  - "DonationListItem.CreatedAt (json created_at) + DonationDetailResponse.Source (json source) exposed on the Go API"
affects: [06-08]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "client-driven segmented-filter + local-state pagination for a list screen (source filter/page live in page state, not the URL — mirrors AgingStatCards toggle)"
    - "authenticated BFF that pins a server-controlled query param (status=pending_review) and maps a UI token to plan 01's server-side allow-list filter (T-06-27 — client cannot inject arbitrary source into SQL)"

key-files:
  created:
    - donnarec-web/components/SourceBadge.tsx
    - donnarec-web/app/api/bff/queue/route.ts
    - donnarec-web/components/QueueSourceFilter.tsx
    - donnarec-web/components/QueueTable.tsx
    - donnarec-web/app/(app)/queue/page.tsx
  modified:
    - donnarec-web/lib/donations.ts
    - donnarec-web/components/DonationTable.tsx
    - donnarec-web/components/DonationDetailView.tsx
    - donnarec-web/messages/th.json
    - donnarec-web/messages/en.json
    - donnarec-api/internal/donation/model.go
    - donnarec-api/internal/donation/service.go

key-decisions:
  - "SourceBadge reads its visible label from the source.* i18n namespace (active-locale) but hard-codes the Thai text in aria-label — mirrors StatusBadge's always-Thai aria pattern (next-intl only exposes the active locale, so a non-active-locale aria lookup is awkward)"
  - "queue source-filter + pagination are LOCAL page state, not URL query params — the segmented-control interaction model (Screen 11) reads better as ephemeral UI state, and unlike Screen 1's shareable filtered URLs the queue is a transient work list; QueueTable pagination is therefore onPageChange-driven (href='#' + preventDefault) rather than href-based"
  - "the queue BFF pins status=pending_review server-side and maps the UI token (all/from-website/staff-entered) to flow_a/flow_b — the client never sends a raw status or source value that reaches SQL (T-06-25/T-06-27)"
  - "exposed created_at on DonationListItem and source on DonationDetailResponse — both were ALREADY selected by the underlying sqlc rows (SearchDonationsRow.CreatedAt, db.Donation.Source from plan 01's GetDonationByID SELECT), so these are pure additive struct-field exposures with NO SQL/sqlc regen (Pitfall-2-safe)"

requirements-completed: [FR-08]

coverage:
  - id: D1
    description: "Staff open /queue and see a pending-review list of both Flow A and Flow B submissions, each row carrying a SourceBadge; the source filter (all/from-website/staff-entered) narrows the query via the authenticated queue BFF"
    requirement: "FR-08"
    verification:
      - kind: build
        ref: "cd donnarec-web && npm run build — /queue and /api/bff/queue both present in the route manifest; compiles clean"
        status: pass
      - kind: e2e
        ref: "human UI walkthrough of /queue (both source chips, empty states, pagination) + the CONVENTIONS integration-test gate over HTTP request → RequireAuth → donationGroup guard → GET /api/donations?status=pending_review&source= — NOT automated in this plan"
        status: pending
    human_judgment: true
    rationale: "This plan is FE + two additive Go response fields; its verification block is build-green + manual UAT. No automated E2E over the real HTTP queue path was in scope — flagged for the phase verifier per the CONVENTIONS integration-test gate."
  - id: D2
    description: "A Flow B record's detail (Screen 3) renders a source-aware creator label ('ผู้บริจาคส่งเอง (ผ่านเว็บไซต์)' + blue badge), never the public-web system user's raw display name (T-06-26)"
    requirement: "FR-08"
    verification:
      - kind: build
        ref: "npm run build green; DonationDetailView branches on donation.source==='flow_b'; DonationDetailResponse now carries source"
        status: pass
      - kind: e2e
        ref: "human UI walkthrough — open a flow_b submission's detail and confirm the label + blue badge, confirm a flow_a record shows the real creator + slate badge"
        status: pending
    human_judgment: true
    rationale: "Requires a real flow_b record in the DB (public-submission path, plans 02/03) to visually confirm — deferred to UAT."

# Metrics
duration: ~35min
completed: 2026-07-11
status: complete
---

# Phase 6 Plan 7: Staff Pending-Review Queue (Screen 11) Summary

**FR-08 pending-review queue at /queue (slate/blue (app)) surfacing Flow A + Flow B side by side with a SourceBadge and a 3-chip source filter backed by plan 01's server-side source narg, plus the Screen 1 source column and the Screen 3 source-aware creator label.**

## Performance
- **Duration:** ~35 min
- **Tasks:** 3 completed
- **Files:** 12 (5 created, 7 modified)

## Accomplishments
- **SourceBadge** — neutral blue (`flow_b`, Globe) / slate (`flow_a`, UserCog) badge per the UI-SPEC locked "Source badge tokens"; visible label from the new `source.*` i18n namespace, always-Thai `aria-label`; unknown values default to the `flow_a` treatment (every legacy row backfilled to flow_a in plan 01).
- **Authenticated queue BFF** (`GET /api/bff/queue`) — reuses `bffForward` (session bearer, T-06-25), pins `status=pending_review` server-side, and maps the UI token (`all`→omit, `from-website`→`flow_b`, `staff-entered`→`flow_a`) into plan 01's `?source=` filter; the Go handler independently 400s any non-flow_a/flow_b value (T-06-27).
- **/queue page (Screen 11)** — the previously-dead `nav.queue` link now resolves inside `AppShell` (slate/blue `(app)`). Smart container holding source-filter + page as local state, a TanStack query against the queue BFF, source-aware empty states (`queue.empty` vs `queue.emptyFiltered`), loading skeleton, and error alert.
- **QueueTable** — TanStack table with the Screen 11 column subset (วันที่ส่ง=`created_at`, donor name, right-aligned comma amount, SourceBadge, manage link → `/donations/[id]`); NO status column (all pending_review), NO receipt column (none issued yet); 20-row client-driven pagination.
- **QueueSourceFilter** — segmented 3-chip control (`role="group"`, per-chip `aria-pressed`), active chip accent blue, default "all"; mirrors Phase 5's AgingStatCards toggle.
- **Screen 1 amendment** — DonationTable gains a "แหล่งที่มา" SourceBadge column between สถานะ and เลขที่ใบเสร็จ (no new Screen 1 filter, per D-77).
- **Screen 3 amendment** — DonationDetailView renders a source-aware "ผู้สร้าง" label: `flow_b` → "ผู้บริจาคส่งเอง (ผ่านเว็บไซต์)" + blue badge (never the synthetic public-web user name, T-06-26); `flow_a` → real creator name + slate badge for parity.
- **i18n** — `source.*`, `queue.*` (title/columns/filter/empty/emptyFiltered), `fields.source`, `detail.creatorSelfSubmitted` added to `th.json` + `en.json`; `nav.queue` reused unchanged.

## Task Commits
1. **Task 1: SourceBadge + queue BFF route + queue/source i18n** — `1d58ab2` (feat)
2. **Task 2: Screen 11 queue — QueueTable + QueueSourceFilter + /queue page** — `b2aac1d` (feat)
3. **Task 3: Screen 1 source column + Screen 3 source-aware creator label** — `c9273b4` (feat)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing critical] Exposed `created_at` on `DonationListItem` (Go API)**
- **Found during:** Task 2
- **Issue:** Screen 11's "วันที่ส่ง" column is spec'd to `created_at` (submission timestamp), but `DonationListItem` only carried `donated_at`. The plan's `files_modified` is FE-only.
- **Fix:** Added `CreatedAt string \`json:"created_at"\`` to `DonationListItem` (model.go) and mapped `row.CreatedAt.Time.Format(time.RFC3339)` in `service.go`'s Search loop. `SearchDonationsRow.CreatedAt` was ALREADY selected by the query (plan 01) — no SQL/sqlc regen needed (Pitfall-2-safe). Added `source` + `created_at` to the FE `DonationSummary` type.
- **Files:** `donnarec-api/internal/donation/{model.go,service.go}`, `donnarec-web/lib/donations.ts`
- **Verification:** `go build ./...` + `go vet ./internal/donation/...` clean; web build green.
- **Committed in:** `b2aac1d`

**2. [Rule 2 - Missing critical] Exposed `source` on `DonationDetailResponse` (Go API)**
- **Found during:** Task 3
- **Issue:** Task 3 assumed the detail response already carried `source` ("confirm GetDonationByID returns source … and the BFF/detail types carry it"). It did not — `GetDonationByID`'s row has `source` (plan 01) but `DonationDetailResponse`/`buildDetailResponse` never surfaced it. Without it the Screen 3 source-aware creator label is impossible.
- **Fix:** Added `Source string \`json:"source"\`` to `DonationDetailResponse` (model.go) and `Source: row.Source` in `buildDetailResponse` (service.go) — `db.Donation.Source` already exists. `DonationDetail` inherits `source` via `DonationSummary` on the FE.
- **Files:** `donnarec-api/internal/donation/{model.go,service.go}`
- **Verification:** `go build ./...` + `go vet` clean; web build green.
- **Committed in:** `c9273b4`

**3. [Rule 3 - Inaccurate reference] The "existing ผู้สร้าง field" on Screen 3 did not exist**
- **Found during:** Task 3
- **Issue:** The UI-SPEC "Amendment to Screen 3" and the plan refer to "the existing 'ผู้สร้าง' (creator) field", but `DonationDetailView`'s donor `<dl>` never rendered a creator row at all.
- **Fix:** Added a new `ผู้สร้าง` `<dl>` row (the source-aware label the amendment intends) rather than editing a non-existent one — functionally identical to the plan's intent.
- **Files:** `donnarec-web/components/DonationDetailView.tsx`
- **Committed in:** `c9273b4`

---
**Total deviations:** 3 auto-fixed (2 additive Go response fields, 1 inaccurate-reference). The plan's `files_modified` was FE-only; two backend files were touched additively for the plan's own stated correctness (submission-date column, source-aware label). No scope creep — no new SQL, no sqlc regen, no behavior change to existing endpoints beyond two new JSON fields.

## Threat Flags
None — no new network endpoint beyond the authenticated queue BFF (which the threat_model already covers, T-06-25/26/27). No new PII surface; the queue list is PII-free (no national ID).

## Known Stubs
None — the queue is wired end-to-end to the real `/api/bff/queue` → Go `/api/donations` path; no hardcoded/mock data.

## Issues Encountered / Flags for Verifier
- **Integration-test gate (CONVENTIONS):** this plan touches the runtime request seam (queue BFF → RequireAuth → donationGroup guard → GET /api/donations) but ships no automated E2E test over that path — its verification block is build-green + manual UAT. Per the project's integration-test gate, the phase verifier should add/confirm an E2E test driving `/api/donations?status=pending_review&source=` with a realistic Keycloak-shaped token before the phase is marked Complete.
- **Visual UAT pending:** confirming the flow_b creator label + badge requires a real flow_b record (public-submission path, plans 02/03) in the DB.

## Next Phase Readiness
- FR-08 staff-visibility surface is in place: `/queue` lists separated Flow A/Flow B pending-review records; Screen 1 shows source; Screen 3 labels web submissions correctly.
- Plan 06-08 can build on the now-source-aware detail response (`source` on `DonationDetailResponse`) and the reusable `SourceBadge`.

---
*Phase: 06-public-donation-web-form-flow-b*
*Completed: 2026-07-11*

## Self-Check: PASSED

All 5 created files + SUMMARY confirmed on disk; all 3 task commits (`1d58ab2`, `b2aac1d`, `c9273b4`) confirmed in git log.
