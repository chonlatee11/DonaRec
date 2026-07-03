---
phase: 03-donation-lifecycle-maker-checker-issuance
plan: 09
subsystem: api
tags: [go, sqlc, postgresql, pgx, pagination, rest-api]

# Dependency graph
requires:
  - phase: 03-donation-lifecycle-maker-checker-issuance
    provides: donation service/handler/E2E harness (plans 03-01..03-08), D-R2 contract decision (03-CONTEXT-remediation.md)
provides:
  - "GET /api/donations returns the D-R2 pagination envelope {\"data\":{\"items\":[...],\"total\":N,\"page\":P,\"per_page\":20}}"
  - CountDonations sqlc query (real COUNT mirroring SearchDonations filters)
  - DonationListItem/DonationListResult response types (PII-free list row + creator display name/UUID)
  - E2E regression guard for the new list envelope over the real HTTP router
affects: [03-10 (frontend list migration to TanStack Query/Table consuming this envelope)]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "sqlc.narg('name') for nullable filter params instead of bare @param — makes the 'nil = skip this filter' pointer-type contract regeneration-safe"
    - "CountDonations mirrors SearchDonations' WHERE predicate byte-for-byte so total is never derived from len(items)"

key-files:
  created: []
  modified:
    - donnarec-api/internal/db/queries/donations.sql
    - donnarec-api/internal/db/generated/donations.sql.go (+ querier.go, models.go, and other generated files — sqlc version-header regen only)
    - donnarec-api/internal/donation/model.go
    - donnarec-api/internal/donation/service.go
    - donnarec-api/internal/donation/handler.go
    - donnarec-api/internal/donation/service_integration_test.go
    - donnarec-api/cmd/server/e2e_test.go

key-decisions:
  - "Fixed a latent generated-code fragility: the committed internal/db/generated/donations.sql.go had hand-edited *string/*DonationStatus param types (violating the sqlc DO NOT EDIT contract) to work around a bare-@param nullability gap. Replaced with sqlc.narg(...) in the .sql source so `sqlc generate` reproduces the correct nullable types on every run — this was a Rule 1 fix, not scope creep, since a plain `sqlc generate` per the plan's own action step would have silently reverted the hand-edit and broken the D-53 nullable-filter skip semantics for every existing donor_name/status/receipt_no search."
  - "Search/List GREEN implementation landed as a single commit spanning service.go + handler.go: they are in the same Go package (donation) and the same compilation unit as handler.go's call site, so Task 2 and Task 3's implementation halves could not be split into separately-buildable commits without a broken intermediate state — matches the repo's prior convention of one feat(...) GREEN commit per TDD plan cycle (e.g. 6d7463c)."

requirements-completed: [FR-10]

# Metrics
duration: ~35min
completed: 2026-07-03
---

# Phase 3 Plan 09: Donation list pagination envelope (backend) Summary

**`GET /api/donations` now returns the D-R2 envelope `{"data":{"items":[...],"total":N,"page":P,"per_page":20}}` with a real COUNT and creator display-name/UUID per row — fixing the bare-array contract that crashed the frontend's `DonationTable`.**

## Performance

- **Duration:** ~35 min
- **Completed:** 2026-07-03
- **Tasks:** 3 (of plan) executed as 2 commits (RED, GREEN) — Tasks 2+3 coupled by Go package compilation
- **Files modified:** 7 substantive (+ 8 sqlc-generated files touched only by version-header regen)

## Accomplishments

- `CountDonations` sqlc query added, sharing the identical 5-predicate WHERE clause with `SearchDonations` (name ILIKE / status / from_date / to_date / receipt_no) — `total` is always a real `COUNT(*)` over the same filter, never `len(items)`.
- `SearchDonations` now `LEFT JOIN`s `users` to expose `created_by_name`, so each list row carries both the creator's display name and their raw `users.id` UUID without a second round-trip.
- `DonationService.Search` signature changed to `([]DonationListItem, int64, error)`; `DonationListItem` is PII-free by construction (no tax/national ID field — D-53).
- `DonationHandler.List` now emits the nested D-R2 envelope instead of the bare-array shape that was bug #5's root cause.
- Extended `TestE2E_MakerCheckerIssuancePipeline`'s `HappyPath_CreateSubmitApproveList` subtest to assert `page`, `per_page`, `total`, and the creator `created_by`/`created_by_id` fields over the real HTTP router with a real minted Keycloak token — run against Docker/testcontainers, passing.
- Fixed a pre-existing generated-code fragility (hand-edited pointer types in `donations.sql.go`) at its source using `sqlc.narg(...)`, so `sqlc generate` is now safe to re-run without silently breaking the D-53 nullable-filter semantics.

## Task Commits

Each task was committed atomically per the plan's RED/GREEN TDD structure (Tasks 2 and 3's implementation halves are coupled by Go package compilation and landed in one GREEN commit — see Deviations):

1. **RED — Tasks 1+2+3 (test scaffolding + schema): failing tests for D-R2 envelope** - `f80061b` (test)
   - `CountDonations` SQL query + users JOIN on `SearchDonations`, regenerated via sqlc
   - `DonationListItem`/`DonationListResult` types in model.go
   - `TestSearchDonations` updated to the new 3-return `Search` signature (fails to build — RED confirmed via `go vet`/`go test`)
   - `e2e_test.go` `listEnvelope` + `HappyPath` step 4 updated to the nested envelope shape
2. **GREEN — Tasks 2+3: Search returns (items,total) + List handler emits envelope** - `a385953` (feat)
   - `DonationService.Search` implementation: builds shared nullable filter values, calls `SearchDonations` + `CountDonations`, maps rows to `DonationListItem`
   - `DonationHandler.List` builds `{"data":{"items":...,"total":...,"page":...,"per_page":20}}`
   - Verified against real Postgres (Docker): `TestSearchDonations` and `TestE2E_MakerCheckerIssuancePipeline` (5/5 subtests) pass; full `go test -short ./...` (96 tests, 15 packages) green

**Plan metadata:** (this commit) `docs(03-09): complete D-R2 donation list envelope plan`

## Files Created/Modified

- `donnarec-api/internal/db/queries/donations.sql` - Added `CountDonations :one`; `SearchDonations` gains `LEFT JOIN users` + `created_by_name`; both queries use `sqlc.narg(...)` for nullable filter params
- `donnarec-api/internal/db/generated/donations.sql.go` - Regenerated: `CountDonations`, `SearchDonationsRow.CreatedByName`, nullable `*string`/`*DonationStatus` param types (sqlc-native now, not hand-edited)
- `donnarec-api/internal/db/generated/{audit,outbox,receiptno,slip,users}.sql.go`, `db.go`, `models.go`, `querier.go` - Touched only by `sqlc generate`'s version-header bump (v1.29.0 → v1.31.1 comment) + the new `CountDonations`/`SetReplaces` entries in the `Querier` interface; no functional changes to unrelated queries
- `donnarec-api/internal/donation/model.go` - Added `DonationListItem` (PII-free) + `DonationListResult` (items/total/page/per_page)
- `donnarec-api/internal/donation/service.go` - `Search` signature and implementation changed to return `(items, total, err)`
- `donnarec-api/internal/donation/handler.go` - `List` builds the nested D-R2 envelope; `page` is now tracked explicitly instead of being discarded after computing `Offset`
- `donnarec-api/internal/donation/service_integration_test.go` - `TestSearchDonations` updated to the new signature; asserts `total` is a real COUNT (including on a 1-row partial page) and asserts the creator join fields
- `donnarec-api/cmd/server/e2e_test.go` - `listEnvelope` decodes the nested envelope; `HappyPath` step 4 asserts `page`/`per_page`/`total`/`created_by`/`created_by_id`

## Decisions Made

- Used `sqlc.narg('name')` (sqlc's nullable-named-arg helper) instead of bare `@name` for all five list filter params in both `SearchDonations` and `CountDonations`. This was necessary because the currently-installed `sqlc v1.31.1` infers non-nullable `string`/`DonationStatus` param types for bare `@name` params compared against `IS NULL`, whereas the previously-committed generated file had these hand-patched to pointer types outside of sqlc's control. Regenerating per the plan's literal instruction ("run `sqlc generate`... Do NOT hand-edit generated files") would have silently reverted that patch and broken the D-53 "skip this filter when absent" semantics for the three existing filter dimensions that depend on it (donor_name, status, receipt_no) — an empty string is not SQL `NULL`. `sqlc.narg()` is the sqlc-native, regeneration-stable way to express this, so it was applied as a Rule 1 (bug) auto-fix within Task 1's scope.
- Task 2 (`model.go`/`service.go`) and Task 3 (`handler.go`/`e2e_test.go`-implementation) GREEN work landed in a single commit rather than two, because `service.go` and `handler.go` live in the same Go package and `handler.go`'s call site would not compile until `service.go`'s new signature existed — there is no intermediate state where only one of the two builds. This matches the plan's own type: tdd frontmatter (single feature, RED→GREEN cycle) and the repo's established precedent (e.g. commit `6d7463c`, one `feat(03-06)` GREEN commit spanning multiple task-level deliverables).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Replaced hand-edited generated-code pointer types with `sqlc.narg(...)` in the SQL source**
- **Found during:** Task 1 (CountDonations query + sqlc regenerate)
- **Issue:** The committed `internal/db/generated/donations.sql.go` had `SearchDonationsParams.DonorName`/`.Status`/`.ReceiptNo` hand-edited to pointer types (with non-sqlc-style comments like `// nil = skip ILIKE filter`) to support the D-53 nullable-filter pattern. The plan's Task 1 action explicitly instructs `sqlc generate` and "Do NOT hand-edit generated files." Running a plain `sqlc generate` against the existing bare-`@param` query text reverted those types to non-nullable `string`/`DonationStatus`, which would have broken the "nil = skip this filter" semantics for `donor_name`, `status`, and `receipt_no` (an empty string sent to Postgres is not `NULL`, so the `IS NULL OR ...` guard would always evaluate false-then-true incorrectly, applying an unwanted empty-string filter instead of skipping it).
- **Fix:** Converted the five filter parameters in both `SearchDonations` and the new `CountDonations` from bare `@param` to `sqlc.narg('param')`, which is sqlc's supported mechanism for explicitly nullable named parameters. Regenerating from this source now reproduces the correct `*string`/`*DonationStatus` pointer types natively, with no hand-editing required — the fix is durable across future `sqlc generate` runs.
- **Files modified:** `donnarec-api/internal/db/queries/donations.sql`, `donnarec-api/internal/db/generated/donations.sql.go` (regenerated)
- **Verification:** `TestSearchDonations` (Docker/testcontainers) passes, including all 5 filter-dimension assertions (name/status/date-range/receipt_no/no-filter) and the new total-COUNT assertions.
- **Committed in:** `f80061b` (RED commit — the fix is in the query source/generated code, which is scaffolding for the RED test to compile against)

---

**Total deviations:** 1 auto-fixed (Rule 1 — bug in existing hand-edited generated code, uncovered by following the plan's own regeneration instruction)
**Impact on plan:** Necessary for correctness — without this fix, regenerating sqlc per the plan's literal Task 1 instruction would have silently broken the D-53 nullable-filter contract for three of five search filters. No scope creep: fixed at the same query file the plan was already modifying, using the sqlc-idiomatic mechanism for exactly this pattern.

## Issues Encountered

None beyond the deviation above — build, vet, short tests, and Docker-backed integration/E2E tests all passed on the first attempt after the GREEN implementation.

## User Setup Required

None — no external service configuration required. Docker/testcontainers were available in the execution environment and used to run the real integration and E2E tests (not just `-short`).

## Next Phase Readiness

- Plan 03-10 (frontend list migration to TanStack Query/Table) can now consume `GET /api/donations` directly — the envelope shape, `total`, `page`, `per_page`, and `created_by`/`created_by_id` fields are all live and E2E-verified.
- The integration-test gate (`.planning/CONVENTIONS.md`) is satisfied for this slice: `TestE2E_MakerCheckerIssuancePipeline` exercises the real HTTP path (`RequireAuth` → `RequireAnyRole` → `ResolveAppUser` → handler → service → DB`) with a real signed Keycloak-shaped token and asserts the new envelope shape.
- No blockers for 03-10.

---
*Phase: 03-donation-lifecycle-maker-checker-issuance*
*Completed: 2026-07-03*
