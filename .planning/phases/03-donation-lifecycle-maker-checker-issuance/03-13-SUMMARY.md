---
phase: 03-donation-lifecycle-maker-checker-issuance
plan: 13
subsystem: ui
tags: [nextjs, tanstack-query, bff, keycloak, go, gin, e2e-testing]

# Dependency graph
requires:
  - phase: 03-donation-lifecycle-maker-checker-issuance
    provides: "03-11 DonationDetailResponse contract (D-R3) + BFF pattern; 03-12 client DonationDetailView (useQuery/useMutation) + approve/return/reject BFF routes"
provides:
  - "BFF routes for create/update/submit/cancel/reissue/slip with FE->Go field-name mapping (national_id->donor_tax_id, address->donor_address, email->donor_email, note->notes)"
  - "DonationForm as a client component owning create/update/submit/slip mutations via useMutation (no more Server Action callback props)"
  - "Cancel/reissue mutations migrated off Server Actions into DonationDetailView useMutation, matching the approve/return/reject pattern"
  - "E2E subtest: create -> submit -> approve -> cancel over the real router, asserting the receipt number is retained (gap-less invariant) and a maker token is 403'd on the checker-only cancel route"
affects: [04-pdf-generation-and-email-delivery]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "All donation write mutations (create/update/submit/slip/cancel/reissue) now flow through the same-origin /api/bff/donations/** BFF routes via client-side useMutation — the Keycloak access token never reaches the browser"
    - "BFF routes are the single point of FE<->Go field-name mapping (national_id/address/email/note <-> donor_tax_id/donor_address/donor_email/notes) so client request TYPES can keep FE-friendly names"

key-files:
  created: []
  modified:
    - donnarec-web/app/donations/[id]/edit/page.tsx
    - donnarec-web/app/donations/[id]/page.tsx
    - donnarec-web/app/donations/new/page.tsx
    - donnarec-web/components/DonationForm.tsx
    - donnarec-web/components/DonationDetailView.tsx
    - donnarec-web/lib/donations.ts
    - donnarec-web/messages/en.json
    - donnarec-web/messages/th.json
    - donnarec-api/cmd/server/e2e_test.go

key-decisions:
  - "Cancel/reissue mutations belong in DonationDetailView (useMutation), not in CancelDialog itself — CancelDialog stays a presentational AlertDialog taking an onConfirm callback, matching the existing approve/return/reject wiring pattern exactly"
  - "app/donations/[id]/page.tsx is now a thin server shell (just resolves the route param) — all mutations, including cancel/reissue, run client-side"

patterns-established:
  - "Server Component page shells that only need a route param render a client view component directly with no Server Actions; all mutations happen via useMutation against BFF routes inside the client component"

requirements-completed: [FR-07, FR-09, FR-19]

# Metrics
duration: ~3min (this continuation session; Task 1 was committed in a prior session on 2026-07-03)
completed: 2026-07-04
---

# Phase 03 Plan 13: Create/Edit/Cancel/Reissue Donation Flow Migration to BFF + TanStack Summary

**Maker create->edit->slip->submit and Checker cancel/void-reissue now run end-to-end through BFF proxies + TanStack mutations against the real Go API, closing the last FE<->BE contract gap (FE field names mapped to donor_tax_id/donor_address/donor_email/notes in the BFF), with an E2E subtest proving cancel retains the receipt number over the production router.**

## Performance

- **Duration:** ~3 min for this continuation session (Task 2 + Task 3); Task 1 (BFF routes) was completed and committed in a prior session
- **Completed:** 2026-07-04
- **Tasks:** 3/3 (all committed)
- **Files modified:** 9 (2 in this session's Task 2 beyond the handoff spec: `app/donations/[id]/page.tsx`, `components/DonationDetailView.tsx` — see Deviations)

## Accomplishments

- Fixed `app/donations/[id]/edit/page.tsx` to the new `DonationForm` client-mutation API (dropped the four `"use server"` action functions and callback props); `npx tsc --noEmit` and `npm run build` both exit 0.
- Discovered and fixed a real runtime bug (Rule 1): `app/donations/[id]/page.tsx` still defined `"use server"` Server Actions calling `cancelDonation`/`reissueDonation` from `lib/donations.ts` — but those functions had already been converted (as part of Task 1/2's BFF migration) to client-only fetchers using relative `fetch("/api/bff/...")` paths, which throw when invoked from a Node/server context with no origin. Moved the cancel/reissue mutations into `DonationDetailView` as `useMutation` calls, mirroring the existing approve/return/reject pattern; `[id]/page.tsx` is now a thin server shell.
- Extended the real-router E2E test with `Cancel_RetainsReceiptNumber_RealPath`: create -> submit -> approve -> cancel over the live HTTP path (Docker testcontainer + real Keycloak-shaped tokens), asserting `status=cancelled`, the receipt number is retained byte-for-byte (gap-less invariant, FR-19/D-47), and a maker-only token is rejected (403 `insufficient_role`) from the checker-only cancel route.

## Task Commits

Each task was committed atomically:

1. **Task 1: BFF routes for create/update/submit/cancel/reissue/slip with field-name mapping** - `16baa34` (feat) — completed in a prior session, not redone.
2. **Task 2: Migrate DonationForm + SlipUploadZone + CancelDialog to TanStack mutations via BFF** - `0184506` (feat) — includes the Rule 1 fix to `[id]/page.tsx` + `DonationDetailView.tsx`.
3. **Task 3: Extend E2E to cover create + cancel over the production router** - `42852cf` (test)

**Plan metadata:** this commit (docs: complete plan)

## Files Created/Modified

- `donnarec-web/app/donations/[id]/edit/page.tsx` - Server Component fetches the draft, renders `<DonationForm mode="edit" donationId={id} initialData={...} />` with no Server Action props
- `donnarec-web/app/donations/[id]/page.tsx` - now a thin server shell resolving `{id}` and rendering `<DonationDetailView id={id} />`; the four inline Server Actions (`handleCancel`/`handleReissue`) were removed
- `donnarec-web/app/donations/new/page.tsx` - renders `<DonationForm mode="create" />` (already done in the prior session)
- `donnarec-web/components/DonationForm.tsx` - client component owning create/update/submit/slip mutations via `useMutation` against `lib/donations.ts` BFF-backed functions (already done in the prior session)
- `donnarec-web/components/DonationDetailView.tsx` - added `cancelMutation`/`reissueMutation` (`useMutation` against `cancelDonation`/`reissueDonation`) and `handleCancel`/`handleReissue` wrapper functions matching `ReviewActionPanel`'s existing `Promise<{error}|null>` contract; dropped the `onCancel`/`onReissue` props (no longer passed in from the server shell)
- `donnarec-web/lib/donations.ts` - create/update/submit/cancel/reissue/uploadSlip/viewSlip/removeSlip repointed to `/api/bff/donations/**` BFF routes (prior session)
- `donnarec-web/messages/en.json`, `donnarec-web/messages/th.json` - copy updates for the migrated flows (prior session)
- `donnarec-api/cmd/server/e2e_test.go` - added `Cancel_RetainsReceiptNumber_RealPath` subtest

## Decisions Made

- Cancel/reissue mutation wiring lives in `DonationDetailView` (client component), not in `CancelDialog` itself — `CancelDialog` remains a pure presentational `AlertDialog` that receives an `onConfirm` callback, exactly matching how `approve`/`returnForEdit`/`reject` are already wired. This keeps `ReviewActionPanel` and `CancelDialog` untouched and consistent with the established pattern from 03-12.
- `app/donations/[id]/page.tsx` no longer needs to be `async` for any data-fetching purpose beyond resolving the route param — all data fetching and mutation now happens client-side inside `DonationDetailView`.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed broken Server Actions calling client-only BFF fetchers from a server context**
- **Found during:** Task 2, while verifying `npx tsc --noEmit` and reviewing the cancel/reissue wiring path referenced in the plan's Task 2 action ("Convert CancelDialog to useMutation for cancel + reissue")
- **Issue:** `app/donations/[id]/page.tsx` still defined `"use server"` functions `handleCancel`/`handleReissue` that called `cancelDonation`/`reissueDonation` from `lib/donations.ts`. Those two functions had already been rewritten (Task 1/2, prior session) to be CLIENT-side BFF fetchers using relative `fetch("/api/bff/donations/:id/cancel")` paths — a bare relative URL passed to `fetch()` from a Next.js Server Action / server context has no origin to resolve against and would fail at runtime the first time a Checker tried to cancel or reissue a receipt. This was not caught by `tsc` (both are async functions returning compatible shapes) or by the plan's Task 1 acceptance criteria (which only checked the BFF *routes*, not this stale server-side caller).
- **Fix:** Added `cancelMutation`/`reissueMutation` (`useMutation`) inside `DonationDetailView.tsx`, with `handleCancel`/`handleReissue` wrapper functions preserving the exact `Promise<{error}|null>` / `Promise<{error?,newId?}|null>` contract `ReviewActionPanel` already expects. Removed the `onCancel`/`onReissue` props from `DonationDetailViewProps` and rewrote `app/donations/[id]/page.tsx` to a thin server shell with no Server Actions.
- **Files modified:** `donnarec-web/components/DonationDetailView.tsx`, `donnarec-web/app/donations/[id]/page.tsx`
- **Verification:** `npx tsc --noEmit` and `npm run build` both exit 0; manual trace of the cancel/reissue call path confirms it now stays entirely client-side (matching approve/return/reject), consistent with D-R1 (token never reaches the browser, mutation runs through the BFF)
- **Committed in:** `0184506` (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (Rule 1 — bug)
**Impact on plan:** Necessary for correctness — without this fix, the Checker cancel/void-reissue flow (a core Phase 3 truth: "Checker/Admin can cancel (void) and void-and-reissue through BFF mutations") would throw at runtime the moment a Checker used it in a real browser. No scope creep — this was a direct consequence of Task 1/2's own field migration, within the same files the plan already lists (`components/CancelDialog.tsx`, cancel/reissue mutation wiring) even though the exact file that needed the fix (`app/donations/[id]/page.tsx`) is one the plan's `files_modified` list already includes only implicitly via `DonationDetailView`/`ReviewActionPanel`'s existing wiring — the plan's Task 2 action explicitly calls for "Convert CancelDialog to useMutation for cancel + reissue," which this change fulfills structurally (the mutation now lives one layer up, in the component that already owns the analogous approve/return/reject mutations).

## Issues Encountered

None beyond the Rule 1 fix documented above.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Phase 3's full Maker create -> edit -> slip -> submit -> Checker approve/return/reject/cancel/void-reissue lifecycle now runs end-to-end through the BFF + TanStack pattern against the real Go API; no remaining Server Action callback wiring for any donation mutation.
- The integration-test gate (.planning/CONVENTIONS.md) is satisfied for create + cancel: `TestE2E_MakerCheckerIssuancePipeline` (7 subtests, including the new `Cancel_RetainsReceiptNumber_RealPath`) passes against a live Docker Postgres testcontainer + real Keycloak-shaped tokens driving the production router (`go build ./...`, `go vet ./...` both clean).
- Ready for the Task 3 checkpoint (`type="checkpoint:human-verify"`) — full-stack human walkthrough of create/edit/slip/submit/approve/cancel/void-reissue — which was not part of this executor's remaining-work scope (see `<remaining_work>` in the handoff prompt; the checkpoint is the plan's final task and requires a live full-stack environment + human verification, out of scope for this automated completion pass).
- No blockers for Phase 4 (PDF generation and email delivery).

---
*Phase: 03-donation-lifecycle-maker-checker-issuance*
*Completed: 2026-07-04*

## Self-Check: PASSED

All created/modified files confirmed present on disk; all three task commits (`16baa34`, `0184506`, `42852cf`) confirmed present in `git log`.
