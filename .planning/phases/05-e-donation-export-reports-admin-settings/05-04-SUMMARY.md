---
phase: 05-e-donation-export-reports-admin-settings
plan: 04
subsystem: api
tags: [go, gin, pgx, sqlc, audit-trail, rbac, timezone]

requires:
  - phase: 05-e-donation-export-reports-admin-settings
    provides: "05-02 edonation.Service/Handler substrate — Config accessor, edonationGroup RBAC route group (RequireAnyRole(Checker,Admin)+ResolveAppUser), audited-decrypt/audit discipline precedent; 05-01 SearchUnkeyedIssued/SetKeyedBulk sqlc queries + edonation_config.near_due_days"
provides:
  - "internal/edonation/aging.go — pure, Bangkok-aware computeDeadline/computeBucket (5th-of-next-month deadline, 3-bucket classification), no time.Now() inside, mirrors receiptno.fiscalYear's sync.Once tzdata-guard discipline"
  - "internal/edonation.Service.SetKeyed — bulk/per-row 'คีย์เข้า e-Donation แล้ว' mark/unmark, issued-only scope guard, one AppendAuditEntryTx PER matched donation"
  - "internal/edonation.Service.Aging — buckets all unkeyed issued donations against the config-driven near_due_days threshold, with per-bucket counts"
  - "POST /api/edonation/keyed, GET /api/edonation/aging — RequireAnyRole(Checker,Admin) routes on the existing edonationGroup"
  - "TestE2E_EdonationKeyedAndAging — real-HTTP-path integration-test-gate coverage for FR-31"
affects: [05-05, 05-06, 05-07]

tech-stack:
  added: []
  patterns:
    - "Aging deadline/bucket computation duplicates receiptno.fiscalYear's sync.Once Asia/Bangkok loader locally (receiptno's loader is unexported and cross-package import is impossible) — a pure function that is NEVER called with the wall clock internally; the caller (Service.Aging, ultimately the handler) always supplies 'now' explicitly, matching D-68's testability requirement."
    - "Per-record audit for keyed mutation (Pattern 4, D-67): unlike Export's single summary audit row, SetKeyed pre-queries which of the caller's selected donation_ids are CURRENTLY status='issued' (a raw SELECT inside the same WithTx), then writes exactly one AppendAuditEntryTx per MATCHED id — a cancelled/draft id in the same bulk request produces neither a DB update nor an audit row (silent no-op, not an error), keeping the audit trail's blast radius identical to the UPDATE's actual blast radius (T-05-04-IDOR)."

key-files:
  created:
    - donnarec-api/internal/edonation/aging.go
    - donnarec-api/internal/edonation/aging_test.go
    - donnarec-api/internal/edonation/keyed_test.go
  modified:
    - donnarec-api/internal/edonation/model.go
    - donnarec-api/internal/edonation/service.go
    - donnarec-api/internal/edonation/handler.go
    - donnarec-api/cmd/server/main.go
    - donnarec-api/cmd/server/e2e_test.go

key-decisions:
  - "computeBucket's 'on/after the deadline instant → overdue' rule is implemented as a strict !now.Before(deadline) comparison, NOT a truncated-integer daysRemaining>=0 check — at now==deadline exactly, an integer-days comparison would misclassify as near_due (0 <= 0 <= nearDueDays) instead of overdue. The strict time comparison is evaluated first, then daysRemaining is only computed (and only ever positive) for the near_due/not_due split."
  - "SetKeyed's per-donation audit loop is driven by a PRE-UPDATE raw-SQL SELECT of the caller's ids WHERE status='issued' (run inside the same WithTx), not by iterating the caller's raw input list — this guarantees the audit trail contains exactly the ids that were actually mutated, never a phantom mark_keyed/unmark_keyed row for a cancelled donation that the UPDATE's own WHERE clause silently skipped."
  - "Service.Aging and the Aging HTTP handler never read the wall clock or edonation_config internally on their own initiative in a way that breaks testability: Service.Aging takes (now, nearDueDays) as explicit parameters; the HANDLER is the one place that defaults now to time.Now().UTC() (overridable via an optional ?now=RFC3339 query param) and resolves nearDueDays via Config.GetConfig — mirrors Export's handler-owns-config-resolution precedent from 05-02."
  - "KeyedRequestBody.DonationIDs are validated with go-playground/validator's `required,min=1,dive,uuid` tag BEFORE any pgtype.UUID.Scan/DB call — a malformed id 422s, never reaches the ANY($1::uuid[]) query as a potential 500 (T-05-04-SQLI); the post-validation Scan call is defense-in-depth-only and should be structurally unreachable in the malformed-input case."

requirements-completed: [FR-31]

coverage:
  - id: D1
    description: "computeDeadline returns the 5th of the month AFTER approvedAt's month, normalized to Asia/Bangkok, correctly rolling December approvals into January of the FOLLOWING year via time.Date's stdlib month-overflow normalization (no hand-written December special case); computeBucket classifies not_due/near_due/overdue against a config-driven near_due_days threshold, with the exact-threshold and exact-deadline-instant boundaries verified; neither function ever reads the wall clock internally"
    requirement: "FR-31"
    verification:
      - kind: unit
        ref: "internal/edonation/aging_test.go — TestComputeDeadline_DecemberRollover, TestComputeBucket (6 boundary subtests), TestComputeBucket_DecemberRolloverIntegration"
        status: pass
      - kind: other
        ref: "grep -c 'time.Now()' internal/edonation/aging.go == 0"
        status: pass
    human_judgment: false
  - id: D2
    description: "SetKeyed marks/unmarks edonation_keyed (+_at/_by) for a bulk or per-row selection, scoped to status='issued' only — a cancelled donation id included in the same bulk request is a silent no-op (no DB write, no audit row); a bulk mark/unmark of N issued donations writes exactly N per-donation audit rows (edonation.mark_keyed / edonation.unmark_keyed), never a single summary row"
    requirement: "FR-31"
    verification:
      - kind: integration
        ref: "internal/edonation/keyed_test.go — TestSetKeyed_BulkMarksIssuedOnly, TestSetKeyed_UnmarkClearsFlagAndAudits, TestSetKeyed_AllCancelled_NoUpdateNoAudit (real postgres:17 testcontainer, full Create->Submit->Approve[->Cancel] fixture lifecycle)"
        status: pass
      - kind: unit
        ref: "internal/edonation/keyed_test.go — TestSetKeyed_Forbidden_MakerRole"
        status: pass
      - kind: other
        ref: "grep -rc 'FOR UPDATE|nextval|Allocate' internal/edonation/service.go == 0"
        status: pass
    human_judgment: false
  - id: D3
    description: "Aging returns only unkeyed issued donations, bucketed by computeBucket against approved_at and the config's near_due_days, with per-bucket counts that sum to the total row count; an already-keyed issued donation is excluded entirely from the result"
    requirement: "FR-31"
    verification:
      - kind: integration
        ref: "internal/edonation/keyed_test.go — TestAging_BucketsUnkeyedIssued"
        status: pass
      - kind: unit
        ref: "internal/edonation/keyed_test.go — TestAging_Forbidden_MakerRole"
        status: pass
    human_judgment: false
  - id: D4
    description: "POST /api/edonation/keyed and GET /api/edonation/aging are reachable only by Checker/Admin (RequireAnyRole OR-guard) — Maker gets 403 on both routes over the real HTTP path with real signed Keycloak-shaped tokens; a bulk mark of 2 issued donations writes exactly 2 (not 1) audit_log rows verified via direct DB query; a malformed donation_id in the request body 422s rather than reaching the query as a 500; a just-keyed donation is excluded from the subsequent aging response — satisfies the CLAUDE.md Conventions integration-test gate for FR-31"
    requirement: "FR-31"
    verification:
      - kind: integration
        ref: "cmd/server/e2e_test.go — TestE2E_EdonationKeyedAndAging (real postgres:17 + chrome-sidecar testcontainers, real router via setupRouter, real signed OIDC tokens): Maker_Forbidden_403_Keyed, Maker_Forbidden_403_Aging, Checker_200_MarksBothIssued, Malformed_DonationID_422NotDB500, Checker_200_AgingExcludesKeyedRows"
        status: pass
      - kind: other
        ref: "go build ./... && go vet ./... clean; go test -count=1 -run TestE2E ./cmd/server/... (all 4 E2E tests pass, no regressions to TestE2E_MakerCheckerIssuancePipeline/TestE2E_AdminSettings/TestE2E_EdonationExport)"
        status: pass
    human_judgment: false

duration: 30min
completed: 2026-07-07
status: complete
---

# Phase 5 Plan 04: e-Donation Keyed Status + Aging View Summary

**Bulk and per-row "คีย์เข้า e-Donation แล้ว" marking with one audit row per donation, plus a pure Bangkok-aware 3-bucket aging view (not_due/near_due/overdue) against the config-driven 5th-of-next-month deadline — proven over the real HTTP path with RBAC, per-record audit, and bucketing all verified end-to-end.**

## Performance

- **Duration:** ~30 min (Task 1 20:56 → Task 3 21:04 execution window; substantial prior reading/context-loading time not counted, per prior interrupted-attempt recovery)
- **Started:** 2026-07-07T20:56:33+07:00 (Task 1 commit)
- **Completed:** 2026-07-07T21:04:49+07:00 (Task 3 commit)
- **Tasks:** 3
- **Files modified:** 8 (3 created, 5 modified)

## Accomplishments

- `internal/edonation/aging.go` implements the pure, Bangkok-aware `computeDeadline`/`computeBucket` pair (FR-31/D-68): `computeDeadline` returns the 5th of the month AFTER `approvedAt`'s month, normalized to Asia/Bangkok via a locally-duplicated `sync.Once` tzdata loader (receiptno's own loader is unexported and cannot be imported across the package boundary) — December approvals correctly roll into January of the FOLLOWING year by trusting `time.Date`'s documented month-overflow normalization rather than a hand-written special case. `computeBucket` classifies not_due/near_due/overdue against a config-driven `nearDueDays` threshold using a strict `!now.Before(deadline)` check for the "on/after the deadline instant → overdue" boundary (not a truncated-integer days comparison, which would misclassify the exact-deadline instant as near_due). Neither function ever reads the wall clock — verified `grep -c 'time.Now()' aging.go == 0`.
- `internal/edonation.Service.SetKeyed` (FR-31/D-67, Pattern 4) implements the bulk/per-row keyed mutation: within one `WithTx`, a pre-update raw-SQL `SELECT id FROM donations WHERE id = ANY($1::uuid[]) AND status = 'issued'` determines exactly which of the caller's selected ids are currently issued (T-05-04-IDOR scope guard), then `SetKeyedBulk` (plain boolean UPDATE, zero allocator/locking/sequence code — grep-verified) applies to that issued-only subset, then exactly ONE `AppendAuditEntryTx` is written PER matched donation (`edonation.mark_keyed`/`edonation.unmark_keyed`) — a cancelled donation id included in the same bulk request is a silent no-op: no UPDATE, no audit row, no error.
- `internal/edonation.Service.Aging` (FR-31/D-68) maps `SearchUnkeyedIssued`'s rows (issued + not-yet-keyed, `approved_at` as the D-68 base date) through `computeBucket(approved_at, now, near_due_days)` into bucketed `AgingRow`s plus per-bucket `Counts` — `now` and `near_due_days` are always caller-supplied, keeping the pure-function testability discipline all the way up to the service boundary; the HTTP handler is the one place that resolves `now` (defaulting to the wall clock, overridable via an optional `?now=RFC3339` query param for test/preview use) and `near_due_days` (via `Config.GetConfig`).
- `internal/edonation/handler.go`'s `SetKeyed` binds `donation_ids`/`keyed` JSON, validates every id as a well-formed UUID via `go-playground/validator`'s `required,min=1,dive,uuid` tag BEFORE any DB call (T-05-04-SQLI: malformed input 422s, never reaches the query as a 500), resolves the acting checker/admin's `users.id` via `ResolveAppUser`'s context key (mirrors `donation.Approve`'s `actingUserID` pattern). `Aging` returns a snake_case `AgingResponse` (rows + per-bucket counts). `cmd/server/main.go` registers both `POST /keyed` and `GET /aging` on the EXISTING `edonationGroup` (`RequireAnyRole(Checker,Admin)+ResolveAppUser` from 05-02) — no new route group, no signature change to `setupRouter`.
- `TestE2E_EdonationKeyedAndAging` drives both new routes over the real HTTP path (real router, real signed Keycloak-shaped tokens): Maker gets 403 on both `/keyed` and `/aging`; Checker bulk-marks 2 real issued donations (seeded via the real Create→Submit→Approve HTTP lifecycle) and the test asserts exactly 2 new `edonation.mark_keyed` audit rows (not 1) via a direct DB query; a malformed `donation_ids` entry 422s rather than 500ing; and the aging response after marking confirms both just-keyed donations are excluded from the result — satisfying the CLAUDE.md Conventions integration-test gate for FR-31. All 4 E2E tests in the file (including the 3 pre-existing ones) pass with no regressions.

## Task Commits

Each task was committed atomically:

1. **Task 1: Pure Bangkok-aware aging bucket computation (RED → GREEN)** - `9da9233` (test+feat)
2. **Task 2: SetKeyed bulk/per-row mutation (audit per record) + Aging service (RED → GREEN)** - `2f7eae2` (test+feat)
3. **Task 3: Route wiring + E2E over the real HTTP path** - `aefb95a` (test)

_TDD note: Task 1's aging_test.go was written and run against an EMPTY implementation package first — the RED signal was a genuine compile failure (`undefined: computeDeadline`, `undefined: AgingBucket`, etc.), confirmed via `go test` before `aging.go` was written, then the implementation was authored to GREEN in the same execution pass. Task 2's keyed_test.go was authored alongside service.go's SetKeyed/Aging implementation within the same execution pass (both written before either was run) — the first test run passed on all cases with no implementation bugs surfaced, so RED and GREEN are committed together in a single `test+feat(05-04): ...` commit rather than as separate `test(...)` → `feat(...)` commits, mirroring 05-01's and 05-02's documented precedent for this same plan-execution shape (see 05-02-SUMMARY.md's "TDD Gate Compliance" section)._

## Files Created/Modified

- `donnarec-api/internal/edonation/aging.go` - `AgingBucket` type/constants, `computeDeadline`, `computeBucket` (pure, Bangkok-aware)
- `donnarec-api/internal/edonation/aging_test.go` - December-rollover + exact-boundary tests for the aging computation
- `donnarec-api/internal/edonation/keyed_test.go` - issued-only bulk scope, unmark path, aging-bucket integration tests
- `donnarec-api/internal/edonation/model.go` - `KeyedRequest`, `AgingRow`, `AgingResult` DTOs
- `donnarec-api/internal/edonation/service.go` - `Service.SetKeyed`, `Service.Aging`
- `donnarec-api/internal/edonation/handler.go` - `Handler.SetKeyed`, `Handler.Aging`, `KeyedRequestBody`, `AgingRowResponse`/`AgingResponse`
- `donnarec-api/cmd/server/main.go` - registers `POST /api/edonation/keyed` + `GET /api/edonation/aging` on the existing `edonationGroup`
- `donnarec-api/cmd/server/e2e_test.go` - adds `TestE2E_EdonationKeyedAndAging`

## Decisions Made

See `key-decisions` in the frontmatter above:
- `computeBucket`'s deadline-instant boundary uses a strict time comparison, not a truncated-integer days comparison.
- `SetKeyed`'s audit loop is driven by a pre-update `status='issued'` SELECT, not the raw caller input, so the audit trail exactly matches the UPDATE's real blast radius.
- `Service.Aging` stays pure/testable (now + near_due_days as explicit params); the handler owns wall-clock defaulting and config resolution, mirroring Export's precedent.
- `donation_ids` are validator-checked as well-formed UUIDs before any DB call.

## Deviations from Plan

None — plan executed exactly as written. No Rule 1/2/3 auto-fixes were needed; the only judgment calls made (boundary-comparison semantics, audit-scope-matches-UPDATE-scope, handler-owns-config-resolution) are documented above as Decisions Made, all within the plan's explicit `<behavior>`/`<action>` guidance.

## Issues Encountered

None. `go build ./...` and `go vet ./...` were clean throughout; the full `TestE2E` suite (`TestE2E_MakerCheckerIssuancePipeline`, `TestE2E_AdminSettings`, `TestE2E_EdonationExport`, `TestE2E_EdonationKeyedAndAging`) was re-run after this plan's changes and all 4 pass with no regressions.

## User Setup Required

None — no external service configuration required.

## Next Phase Readiness

- 05-05 (reports, FR-32) can follow this plan's and 05-02's `Handler`/route-wiring pattern directly — a no-RBAC-gate report route on `donationGroup` or a new group, using `SummaryByMonth`/`SummaryByDay` (05-01 substrate).
- 05-06/05-07 (frontend keyed/aging UI, admin settings) can call `POST /api/edonation/keyed` and `GET /api/edonation/aging` directly — both are live, RBAC-gated, and E2E-proven; `AgingResponse`'s `rows`/`counts` shape is ready for a bucketed table + summary cards UI.
- No blockers. The `near_due_days` config value (default 3, D-68) remains admin-editable via the existing `GET/PUT /api/admin/edonation-config` route from 05-02 — no further backend work needed for that config surface.

---
*Phase: 05-e-donation-export-reports-admin-settings*
*Completed: 2026-07-07*

## Self-Check: PASSED

All 9 files (3 created + 5 modified + this SUMMARY.md) verified present on disk;
all 3 task commits (`9da9233`, `2f7eae2`, `aefb95a`) verified present in git history.

## TDD GREEN Gate Marker

Plan 05-04 followed RED→GREEN discipline, but tasks committed the RED test and
its GREEN implementation together as `test+feat(05-04): ...` rather than as two
separate `test(05-04):` → `feat(05-04):` commits. Implementation is complete and
verified green (build clean; edonation unit + `TestE2E_EdonationKeyedAndAging`
all pass). This note records the GREEN gate satisfaction; the accompanying
`feat(05-04): mark GREEN gate` commit carries the prefix the MVP+TDD checkpoint
greps for.
