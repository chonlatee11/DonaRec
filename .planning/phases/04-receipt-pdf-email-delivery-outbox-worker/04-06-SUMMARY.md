---
phase: 04-receipt-pdf-email-delivery-outbox-worker
plan: 06
subsystem: api
tags: [go, resend, presigned-url, next-intl, tanstack-query, e2e]

# Dependency graph
requires:
  - phase: 04-receipt-pdf-email-delivery-outbox-worker
    plan: 01
    provides: "donations.donor_language/receipt_pdf_object_key columns, EnqueueOutboxJob"
  - phase: 04-receipt-pdf-email-delivery-outbox-worker
    plan: 05
    provides: "outbox worker freeze-idempotency (D-56) — a re-enqueued issue_receipt job reuses the frozen PDF instead of re-rendering; storage.StorageClient.PresignedGet"
provides:
  - "Donation.Resend(ctx, id, appUserID) — Checker/Admin only, re-enqueues an issue_receipt outbox job for an already-issued donation without allocating a new receipt number or re-rendering"
  - "Donation.DownloadReceipt(ctx, id) — any staff role, returns a 15-min presigned GET URL for the frozen receipt PDF"
  - "donor_language ('th'|'en', default 'th') captured on create/edit, frozen into the snapshot, returned in the detail response"
  - "FE: EmailDeliveryPanel + DeliveryStatusBadge on Donation Detail (Screen 3b), BFF resend/receipt-pdf proxy routes, donor_language Select on DonationForm"
affects: [04-07, 04-08]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "checkerGroup.POST(/:id/resend) vs donationGroup.GET(/:id/receipt-pdf) — resend is Checker/Admin-only (RequireAnyRole) while download is placed on the broader donationGroup so a Maker gets 200, not 403 (D-57 'staff download always')"
    - "Resend never touches the numbering allocator or the renderer — it only calls the same EnqueueOutboxJob(issue_receipt) path Approve uses in 04-01/04-03, relying entirely on 04-05's freeze-idempotency to avoid re-rendering"

key-files:
  created: []
  modified:
    - donnarec-api/internal/donation/model.go
    - donnarec-api/internal/donation/service.go
    - donnarec-api/internal/donation/handler.go
    - donnarec-api/internal/donation/errors.go
    - donnarec-api/cmd/server/main.go
    - donnarec-api/cmd/server/e2e_test.go
    - donnarec-api/internal/db/queries/donations.sql
    - donnarec-api/internal/db/generated/donations.sql.go
    - donnarec-web/app/api/bff/donations/[id]/resend/route.ts
    - donnarec-web/app/api/bff/donations/[id]/receipt-pdf/route.ts
    - donnarec-web/components/DonationForm.tsx
    - donnarec-web/components/EmailDeliveryPanel.tsx
    - donnarec-web/components/DeliveryStatusBadge.tsx
    - donnarec-web/components/DonationDetailView.tsx
    - donnarec-web/lib/donations.ts
    - donnarec-web/lib/bff.ts
    - donnarec-web/messages/th.json
    - donnarec-web/messages/en.json

key-decisions:
  - "D-55: donor_language captured on create/edit, defaults 'th', frozen at create-time like other snapshot fields (D-43 precedent) — drives PDF/email language."
  - "D-56/D-57: Resend only inserts a NEW outbox_jobs row for the same donation_id; it never allocates a new receipt number and never re-renders — 04-05's freeze-idempotency (receipt_pdf_object_key already set) guarantees the worker reuses the stored PDF."
  - "Resend route lives on checkerGroup (RequireAnyRole Checker/Admin); download route lives on the broader donationGroup so any staff role (Maker included) can download — matches D-57 'staff download always, resend is Checker/Admin only'."
  - "[Rule 2] Added a Rule-2 fix mid-plan (commit 3659dbf): the donation detail response was not surfacing the latest email_delivery row, which would have left the FE EmailDeliveryPanel with nothing to render even after a successful send — buildDetailResponse now includes it."

requirements-completed: [FR-27, FR-28, FR-23]

coverage:
  - id: D1
    description: "Maker can pick donor_language (th|en) when creating/editing a Flow A donation; persisted, defaulted to 'th' when omitted, frozen into the snapshot, and returned in the detail response"
    requirement: "FR-23"
    verification:
      - kind: e2e
        ref: "donnarec-api/cmd/server/e2e_test.go#TestE2E_MakerCheckerIssuancePipeline/DonorLanguage_PersistsAndDefaults (pass)"
        status: pass
    human_judgment: false
  - id: D2
    description: "Checker/Admin can resend a receipt email; resend re-enqueues an outbox job and never allocates a new number or re-renders; Maker is rejected with 403"
    requirement: "FR-27"
    verification:
      - kind: e2e
        ref: "donnarec-api/cmd/server/e2e_test.go#TestE2E_MakerCheckerIssuancePipeline/ResendAndDownload_RealPath (pass)"
        status: pass
    human_judgment: false
  - id: D3
    description: "Any staff role can download the frozen receipt PDF via a short-lived (15-min) presigned URL; a not-ready error is returned before the PDF is frozen"
    requirement: "FR-28"
    verification:
      - kind: e2e
        ref: "donnarec-api/cmd/server/e2e_test.go#TestE2E_MakerCheckerIssuancePipeline/ResendAndDownload_RealPath (pass)"
        status: pass
    human_judgment: false
  - id: D4
    description: "go build/go vet/npm run build all pass with the new backend routes and FE EmailDeliveryPanel/DeliveryStatusBadge/BFF routes/donor_language Select wired"
    verification:
      - kind: other
        ref: "cd donnarec-api && go build ./... ; cd donnarec-web && npm run lint && npm run build (all pass, this session)"
        status: pass
    human_judgment: false
  - id: D5
    description: "The Donation Detail screen (Screen 3b) shows email delivery status + recipient + attempts, and staff resend/download buttons with correct role-gating (Resend hidden from Maker, Download always available when a PDF exists) — visually correct against the running stack"
    verification: []
    human_judgment: true
    rationale: "This is Task 4 of the plan (checkpoint:human-verify, gate=blocking) — a human browser walkthrough against the live stack. The user has explicitly decided to DEFER this walkthrough to phase-end verification (/gsd-verify-work) rather than perform it now. See 'Deferred: Task 4 Human UI Walkthrough' section below for the exact steps and credential prerequisites verify-work must satisfy."

# Metrics
duration: ~35min (tasks 1-3 only; task 4 deferred, not executed)
completed: 2026-07-04
status: complete
---

# Phase 4 Plan 6: Resend/Download + Donor Language Summary

**Checker/Admin-gated resend (re-enqueue only, never re-numbers or re-renders) + staff-wide presigned-URL download + donor_language capture on create/edit, proven over the real HTTP path; Screen 3b's manual UI walkthrough deferred to phase-end verify-work**

## Performance

- **Duration:** ~35 min (Tasks 1-3)
- **Started:** 2026-07-04T17:10:13+07:00
- **Completed:** 2026-07-04T17:37:19+07:00
- **Tasks:** 3 of 4 executed (Task 4 is a `checkpoint:human-verify` gate deferred by user decision, not executed)
- **Files modified:** 20

## Accomplishments

- `donor_language` ('th'|'en', default 'th') is captured on donation create/edit, validated (`oneof=th en`), frozen into the snapshot at create-time (D-43 precedent, D-55), and returned in the detail response — proven end-to-end via a real-HTTP E2E subtest asserting both the explicit-'en' round-trip and the default-'th' path.
- `Resend` (Checker/Admin only via `checkerGroup` `RequireAnyRole`): validates the donation is `issued` with a frozen `receipt_pdf_object_key`, inserts a **new** `outbox_jobs` row (`issue_receipt`, same `donation_id`) reusing the exact `EnqueueOutboxJob` path Approve already uses, and audits `receipt_resend`. It never touches the numbering allocator and never re-renders — the E2E test asserts `receipt_no` is byte-identical before and after resend, and that a Maker attempting resend gets 403.
- `DownloadReceipt` (any staff role via the broader `donationGroup`): returns a 15-minute presigned GET URL for `receipt_pdf_object_key` via the existing receipts-bucket `StorageClient.PresignedGet`, audits `receipt_download`, and returns a distinct not-ready domain error when the PDF is not yet frozen.
- FE: `donnarec-web/app/api/bff/donations/[id]/resend/route.ts` and `.../receipt-pdf/route.ts` — thin BFF proxies (server-side bearer only, Go re-enforces RBAC); `DonationForm.tsx` gained a donor_language Select (ไทย/English, default ไทย); `EmailDeliveryPanel.tsx` + `DeliveryStatusBadge.tsx` render on `DonationDetailView` for `issued`/`cancelled` donations, showing status badge, recipient, last-attempt timestamp, attempts, and resend/download buttons with role-gating; `emailDelivery.*` message keys added to both `th.json`/`en.json`.
- **[Rule 2 auto-fix]** `buildDetailResponse` did not surface the latest `email_delivery` row on the donation detail response — without this, the FE `EmailDeliveryPanel` would have nothing to render even after the worker successfully sent an email. Fixed in commit `3659dbf`.

## Task Commits

Each task was committed atomically (Tasks 1 and 2 are TDD: RED then GREEN):

1. **Task 1: donor_language capture on create/edit (backend) + E2E persistence**
   - RED — `6f9ad34` (test): `DonorLanguage_PersistsAndDefaults` E2E subtest added, failing (field didn't exist)
   - GREEN — `d09419d` (feat): donor_language threaded through model/service/handler + sqlc query params, test passes
2. **Task 2: Resend + DownloadReceipt endpoints (backend) + E2E**
   - RED — `743389c` (test): `ResendAndDownload_RealPath` E2E subtest added, failing (endpoints didn't exist)
   - GREEN — `7264491` (feat): `Resend`/`DownloadReceipt` service+handler+routes, test passes
   - Rule-2 fix — `3659dbf` (fix): surface latest `email_delivery` on donation detail (found while wiring the FE panel; without it the panel had no data to render)
3. **Task 3: FE — donor_language field, resend/download BFF + EmailDeliveryPanel (Screen 3b)** - `2173be9` (feat)

**Task 4 (checkpoint:human-verify, gate=blocking): NOT EXECUTED — deferred by explicit user decision.** See "Deferred: Task 4 Human UI Walkthrough" below.

**Plan metadata:** (this commit, docs: complete plan)

## Files Created/Modified

- `donnarec-api/internal/donation/model.go` - `DonorLanguage` on create/update DTOs and detail response
- `donnarec-api/internal/donation/service.go` - `Resend`, `DownloadReceipt`, donor_language threading into `CreateDonation`/`UpdateDraftDonation`, latest-`email_delivery` lookup in `buildDetailResponse`
- `donnarec-api/internal/donation/handler.go` - `Resend`/`DownloadReceipt` handlers (Pattern A claims + app_user_id extraction + Pattern C logging + audit)
- `donnarec-api/internal/donation/errors.go` - domain errors for resend/download guard conditions (not-issued, no-frozen-PDF)
- `donnarec-api/cmd/server/main.go` - `checkerGroup.POST(/:id/resend)`, `donationGroup.GET(/:id/receipt-pdf)`
- `donnarec-api/cmd/server/e2e_test.go` - `DonorLanguage_PersistsAndDefaults`, `ResendAndDownload_RealPath` subtests
- `donnarec-api/internal/db/queries/donations.sql` / `.../generated/donations.sql.go` - donor_language params on create/update queries
- `donnarec-web/app/api/bff/donations/[id]/resend/route.ts` - BFF proxy → `POST /api/donations/:id/resend`
- `donnarec-web/app/api/bff/donations/[id]/receipt-pdf/route.ts` - BFF proxy → `GET /api/donations/:id/receipt-pdf`
- `donnarec-web/components/DonationForm.tsx` - donor_language Select
- `donnarec-web/components/EmailDeliveryPanel.tsx` - Screen 3b panel (status, recipient, attempts, resend/download actions)
- `donnarec-web/components/DeliveryStatusBadge.tsx` - status badge mapping sent/failed/pending/no_email → UI-SPEC tokens
- `donnarec-web/components/DonationDetailView.tsx` - mounts `EmailDeliveryPanel` for issued/cancelled donations
- `donnarec-web/lib/donations.ts`, `donnarec-web/lib/bff.ts` - resend/download client-side fetchers
- `donnarec-web/messages/th.json`, `donnarec-web/messages/en.json` - `emailDelivery.*` keys

## Decisions Made

- D-55: donor_language defaults 'th', frozen at create-time (matches D-43's snapshot-freezing precedent for other donation fields).
- D-56/D-57: Resend is enqueue-only — it never calls the numbering allocator and never re-renders; it depends entirely on 04-05's freeze-idempotency (`receipt_pdf_object_key` already set) for the worker to reuse the stored PDF.
- Route placement: resend on `checkerGroup` (Checker/Admin only), download on `donationGroup` (all staff roles) — intentionally asymmetric per D-57 "staff download always, resend is Checker/Admin only".
- [Rule 2] `buildDetailResponse` extended to surface the latest `email_delivery` row — necessary for the FE panel to have any data to render; not explicitly called out in the plan's action text but required for the stated `must_haves` truth "the Donation Detail screen shows email delivery status".

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical] Surface latest `email_delivery` on donation detail response**
- **Found during:** Task 2/3 boundary — while wiring `EmailDeliveryPanel` against the detail response, discovered `buildDetailResponse` never queried `email_delivery` at all.
- **Issue:** Without this, the FE panel (Task 3, plan-mandated per `must_haves`) would have had no status/recipient/attempts data to render even for donations with successful sends — a plan-stated truth ("the Donation Detail screen shows email delivery status...") would have been unmet.
- **Fix:** `buildDetailResponse` now looks up and includes the latest `email_delivery` row for the donation.
- **Files modified:** `donnarec-api/internal/donation/service.go`
- **Verification:** Exercised by `ResendAndDownload_RealPath` E2E subtest and `go build ./...`.
- **Committed in:** `3659dbf`

---

**Total deviations:** 1 auto-fixed (1 missing-critical)
**Impact on plan:** Necessary for the plan's own stated `must_haves` truth to be satisfiable by the FE panel built in Task 3. No architectural scope creep.

## Issues Encountered

None beyond the Rule-2 fix above.

## Deferred: Task 4 Human UI Walkthrough

**Status: DEFERRED to phase-end verification (`/gsd-verify-work`)** — by explicit user decision. Code for Tasks 1-3 is complete, committed, and proven via automated E2E tests over the real HTTP path (real router → real Keycloak-shaped token → RBAC → handler → service → DB). What remains unverified is the **visual/manual** confirmation of Screen 3b's behavior against the live running stack (API + worker + web + MinIO + chrome sidecar + Keycloak), which the automated E2E tests do not exercise (no browser, no real email send/render round-trip observed by a human, no visual confirmation of role-gated button visibility).

**Do NOT perform this walkthrough now.** This section exists so `/gsd-verify-work` (or whichever agent picks up phase-end UAT) has everything needed to execute it without re-deriving context.

### Credential / environment prerequisites (must be satisfied before the walkthrough can run)

1. **Keycloak `donnarec-frontend` confidential client secret** — the frontend client must be confidential (NextAuth server-side session), per the Phase 3 fix (bug #2, commit `8604caa`). Confirm the client secret is present in the environment the web app reads from (see next item).
2. **`donnarec-web/.env.local`** — must have the NextAuth/Keycloak environment variables populated (issuer URL, client id, client secret, `NEXTAUTH_URL`, `NEXTAUTH_SECRET`). Without this, login will fail before the walkthrough can even start.
3. **Test-account passwords** — `admin-test`, `maker-test`, `checker-test` (or the equivalently-named seeded users — STATE.md references `maker1`/`checker1`/`admin`/`makerchecker` at password `DonaRec123` as of the Phase 3 verification; confirm current seed data matches or re-seed) must be known/working in Keycloak for the walkthrough's three role logins.
4. **Running stack**: API on `:8000`, outbox worker process running, web on `:3000`, MinIO up, chrome sidecar reachable (internal compose network only, per 04-02), Keycloak on `:8080`. `docker compose up` (with the existing `docker-compose.override.yml` port remap for Postgres, per Phase 3 notes) should bring all of this up.

### Exact walkthrough steps (copied from 04-06-PLAN.md Task 4, unchanged)

1. Log in as a Maker; create a Flow A donation with `donor_language = English`; submit.
2. Log in as a Checker; approve it → receipt issued.
3. Wait a few seconds; open the donation detail — the Email Delivery panel shows a status badge (sent, or failed if the dev sender is configured to fail) with recipient + attempt count.
4. Click "ดาวน์โหลด PDF" — the frozen receipt PDF downloads and renders Thai/English correctly.
5. As Checker, click "ส่งอีเมลอีกครั้ง" — a success toast appears; verify the receipt number is unchanged and no new number was allocated.
6. Confirm a Maker viewing the same receipt sees Download but NOT Resend.

### What automated coverage already proves (so the walkthrough is confirming presentation, not correctness)

- Resend/download RBAC, receipt-number invariance, and audit-row writing are already proven by `ResendAndDownload_RealPath` (real HTTP path, real token).
- donor_language persistence/default is already proven by `DonorLanguage_PersistsAndDefaults`.
- What is NOT proven by automation: the visual badge/color mapping, the toast copy, whether the panel actually renders in the real browser DOM, and whether the downloaded PDF genuinely opens and displays Thai/English text correctly to a human eye.

## User Setup Required

None new — this plan reuses infrastructure (Keycloak, MinIO, chrome sidecar, worker) wired in prior 04-* plans. The credential prerequisites listed above are existing environment setup, not new configuration introduced by this plan.

## Next Phase Readiness

- 04-07/04-08 can proceed; nothing in this plan's deferred item blocks their `depends_on` (04-06 backend/FE surface is fully built and E2E-proven).
- Task 4 (Screen 3b manual walkthrough) must be picked up by `/gsd-verify-work` before Phase 4 is marked Complete, per the Conventions integration-test gate (human UI walkthrough is part of the done-criterion for runtime-integration phases).
- No blockers to continuing execution of remaining Phase 4 plans.

## Self-Check: PASSED

Verified commit hashes present in `git log --oneline --all`: `6f9ad34`, `d09419d`, `743389c`, `7264491`, `3659dbf`, `2173be9` (all FOUND, confirmed via `git log --oneline --grep=04-06`). Verified files exist on disk: `donnarec-api/internal/donation/service.go`, `donnarec-api/internal/donation/handler.go`, `donnarec-web/components/EmailDeliveryPanel.tsx`, `donnarec-api/cmd/server/e2e_test.go` (all FOUND).

---
*Phase: 04-receipt-pdf-email-delivery-outbox-worker*
*Completed: 2026-07-04*
