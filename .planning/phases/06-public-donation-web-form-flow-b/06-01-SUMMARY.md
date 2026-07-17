---
phase: 06-public-donation-web-form-flow-b
plan: 01
subsystem: database
tags: [postgresql, golang-migrate, sqlc, pgx, gin, rbac]

# Dependency graph
requires:
  - phase: 03-approval-issuance-audit-rbac
    provides: donations table, users/user_roles schema, audit.AppendAuditEntryTx (parseUUID actor requirement), SearchDonations/CountDonations/ListFilter D-53 nullable-narg pattern
  - phase: 05-edonation-export-aging-reports
    provides: precedent for ALTER TABLE ADD COLUMN + physical-column-order sqlc regen fragility (Pitfall 2, 000013 fix)
provides:
  - donations.source column (TEXT NOT NULL DEFAULT 'flow_a', CHECK flow_a|flow_b)
  - fixed-UUID public-web system user (00000000-0000-4000-8000-000000000006) seeded in users, least-privileged 'maker' role in user_roles
  - source narg filter threaded through SearchDonations/CountDonations/GetDonationByID (sqlc regenerated)
  - ListFilter.Source / DonationListItem.Source (internal/donation/model.go)
  - GET /api/donations?source=flow_a|flow_b (400 invalid_source on any other value)
affects: [06-02 (public submission create path — created_by/audit actor FK target), 06-07 (staff pending-review queue screen — source filter consumer)]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "D-53 nullable-narg filter pattern extended (source alongside donor_name/status/date-range/receipt_no)"
    - "Pitfall-2-safe ALTER TABLE ADD COLUMN: new column appended last in migration and in every explicit SELECT column list that reads the full donations row"

key-files:
  created:
    - donnarec-api/migrations/000015_donation_source.up.sql
    - donnarec-api/migrations/000015_donation_source.down.sql
    - donnarec-api/migrations/000016_seed_public_web_user.up.sql
    - donnarec-api/migrations/000016_seed_public_web_user.down.sql
    - donnarec-api/internal/donation/search_source_test.go
  modified:
    - donnarec-api/internal/db/queries/donations.sql
    - donnarec-api/internal/db/generated/donations.sql.go
    - donnarec-api/internal/db/generated/models.go
    - donnarec-api/internal/db/generated/querier.go
    - donnarec-api/internal/db/generated/edonation.sql.go
    - donnarec-api/internal/donation/model.go
    - donnarec-api/internal/donation/service.go
    - donnarec-api/internal/donation/handler.go

key-decisions:
  - "source column is TEXT + CHECK(flow_a|flow_b), not an enum type — matches the sqlc.narg('source')::TEXT filter shape already used for every other D-53 narg filter, avoids CREATE TYPE ceremony for a two-value domain"
  - "public-web system user's id AND keycloak_subject are both the SAME fixed literal UUID (00000000-0000-4000-8000-000000000006) — prevents the Pitfall 1 audit.parseUUID rollback that a human-readable sentinel like 'public-web' would cause"
  - "public-web user assigned the least-privileged 'maker' role (T-06-03) even though it never authenticates — defense-in-depth in case that exact subject were ever presented in a forged token"
  - "ListFilter.Source and DonationListItem.Source were added to internal/donation/model.go, not service.go — the plan's files_modified listed service.go for this struct, but ListFilter/DonationListItem are actually declared in model.go (Rule 3 auto-correction, files_modified path was inaccurate)"
  - "DonationListItem gained a Source field (not strictly required by Task 2's acceptance criteria) to match 06-RESEARCH.md's exact SearchDonations code example and give plan 07's queue screen the source value per row without a second round-trip (Rule 2)"
  - "service.go's List() method (line ~451) was left untouched — it is dead code (zero callers anywhere in the codebase), issues a raw unfiltered SQL query (not even donor_name/status/date filters are wired), and is fully superseded by Search(); wiring Source into it would be inconsistent scope creep on unreachable code"

requirements-completed: [FR-01, FR-08]

coverage:
  - id: D1
    description: "donations.source column (TEXT NOT NULL DEFAULT 'flow_a', CHECK flow_a|flow_b) added via migration 000015; all pre-existing rows backfill to flow_a"
    requirement: "FR-08"
    verification:
      - kind: integration
        ref: "manual verification — full migration chain 000001-000016 applied against a throwaway Postgres 17 container; inserted a test donation row and confirmed source defaulted to flow_a; also implicitly exercised by every testcontainers-based test in internal/donation (SetupTestPostgres runs the full up chain)"
        status: pass
    human_judgment: false
  - id: D2
    description: "fixed-UUID public-web system user seeded (id=keycloak_subject=00000000-0000-4000-8000-000000000006), least-privileged 'maker' role assigned, idempotent via ON CONFLICT"
    requirement: "FR-01"
    verification:
      - kind: integration
        ref: "manual verification — queried users/user_roles after applying 000016 against a throwaway Postgres 17 container; confirmed id/keycloak_subject match and role='maker'"
        status: pass
    human_judgment: false
  - id: D3
    description: "Both down migrations (000015, 000016) cleanly reverse the up migrations"
    verification:
      - kind: integration
        ref: "manual verification — ran 000016 down then 000015 down against the throwaway container after removing the FK-referencing test donation row; confirmed user row and source column both gone"
        status: pass
    human_judgment: false
  - id: D4
    description: "SearchDonations/CountDonations source filter: flow_b excludes flow_a and vice versa; nil source returns both; CountDonations total always matches the filtered item count"
    requirement: "FR-08"
    verification:
      - kind: integration
        ref: "donnarec-api/internal/donation/search_source_test.go#TestSearchDonations_SourceFilter (3 subtests: source=flow_b_excludes_flow_a, source=flow_a_excludes_flow_b, source_unset_returns_both)"
        status: pass
    human_judgment: false
  - id: D5
    description: "GET /api/donations?source= allow-lists flow_a/flow_b and rejects any other value with 400 invalid_source"
    requirement: "FR-08"
    verification: []
    human_judgment: true
    rationale: "Handler-level 400 rejection is implemented (internal/donation/handler.go List()) but no HTTP-layer/handler-level automated test exercises it in this plan — Task 2's <behavior> block only specified SearchDonations/CountDonations-level tests, and the real HTTP path for staff list/queue endpoints is exercised end-to-end by plan 07's queue screen work. Flagging for verifier/UAT confirmation rather than asserting pass on unverified code."

# Metrics
duration: ~25min
completed: 2026-07-11
status: complete
---

# Phase 6 Plan 1: Flow B Data Foundation Summary

**donations.source column (flow_a/flow_b, gap-less backfill) + fixed-UUID public-web system user + source narg threaded through SearchDonations/CountDonations/handler, TDD-proven**

## Performance

- **Duration:** ~25 min
- **Started:** 2026-07-11T13:08:00+07:00 (approx.)
- **Completed:** 2026-07-11T13:33:00+07:00
- **Tasks:** 2
- **Files modified:** 13 (4 new migrations, 1 new test, 8 modified)

## Accomplishments
- `donations.source` column (TEXT NOT NULL DEFAULT 'flow_a', CHECK IN flow_a/flow_b) via migration 000015 — every pre-existing row backfills to `flow_a` automatically via the DEFAULT, no separate UPDATE needed
- Fixed-UUID `public-web` system user (`00000000-0000-4000-8000-000000000006` as both `id` and `keycloak_subject`) seeded via migration 000016, idempotent, least-privileged `maker` role assigned — this is the FK target future Flow-B create/audit code (plan 03) will use, and it structurally cannot trip `audit.parseUUID`'s rollback (Pitfall 1)
- `SearchDonations`/`CountDonations` sqlc queries extended with a `source` nullable-narg filter (D-53 pattern); `GetDonationByID`'s SELECT list gains `source` in the correct physical column position (Pitfall 2 avoided) — regenerated cleanly, `go build ./...` green
- `ListFilter.Source` / `DonationListItem.Source` wired through `DonationService.Search`; `GET /api/donations?source=flow_a|flow_b` allow-listed at the handler, any other value → `400 {"error":"invalid_source"}`
- `TestSearchDonations_SourceFilter` (TDD RED→GREEN) proves flow_b excludes flow_a and vice versa, nil source returns both, and `CountDonations`' total always matches the filtered item count

## Task Commits

Each task was committed atomically:

1. **Task 1: Migrations 000015 (source column + backfill) and 000016 (seed public-web system user)** - `6a045cb` (feat)
2. **Task 2: source filter on SearchDonations/CountDonations + ListFilter.Source + ?source= handler param (TDD)**
   - RED: `a825acd` (test) — failing compile (`ListFilter` had no `Source` field)
   - GREEN: `540d0b2` (feat) — sqlc regen + service/handler wiring, test passes

_No REFACTOR commit needed — GREEN implementation matched the existing D-53 pattern exactly, nothing to clean up._

## TDD Gate Compliance

Task 2 (`tdd="true"`) followed the full RED→GREEN cycle:
- RED (`a825acd`): `search_source_test.go` added referencing `ListFilter.Source`, which did not yet exist — confirmed via `go vet` reporting `unknown field Source in struct literal of type ListFilter` (compile-level RED, since this is a schema/struct-adding change rather than a runtime-assertion failure).
- GREEN (`a825acd` → `540d0b2`): `donations.sql` extended, `sqlc generate` run, `ListFilter.Source`/`DonationListItem.Source` added to `model.go`, `service.go`/`handler.go` wired. `go build ./...` green; `TestSearchDonations_SourceFilter` passes (3/3 subtests).

Both gate commits present in git log — compliant.

## Files Created/Modified
- `donnarec-api/migrations/000015_donation_source.up.sql` / `.down.sql` - adds/drops `donations.source`
- `donnarec-api/migrations/000016_seed_public_web_user.up.sql` / `.down.sql` - seeds/deletes the fixed-UUID public-web system user + role
- `donnarec-api/internal/db/queries/donations.sql` - `source` narg on SearchDonations/CountDonations; `source` added to GetDonationByID's SELECT list
- `donnarec-api/internal/db/generated/{donations.sql.go,models.go,querier.go,edonation.sql.go}` - sqlc-regenerated output (edonation.sql.go/querier.go also picked up a pre-existing stale doc-comment sync, unrelated to this plan's SQL changes)
- `donnarec-api/internal/donation/model.go` - `ListFilter.Source *string`, `DonationListItem.Source string`
- `donnarec-api/internal/donation/service.go` - `Search()` maps `filter.Source` into `SearchDonationsParams`/`CountDonationsParams` and into each `DonationListItem`
- `donnarec-api/internal/donation/handler.go` - `?source=` query param parsing + flow_a/flow_b allow-list (400 on anything else)
- `donnarec-api/internal/donation/search_source_test.go` - `TestSearchDonations_SourceFilter` (new)

## Decisions Made
- `source` is `TEXT + CHECK` rather than a Postgres enum — simpler migration, matches the existing `sqlc.narg(...)::TEXT` filter shape used by every other D-53 narg filter (donor_name/status/date-range/receipt_no); no `CREATE TYPE`/`ALTER TYPE` ceremony needed for a two-value domain.
- The seeded public-web user's `id` and `keycloak_subject` are the exact same fixed literal UUID (`00000000-0000-4000-8000-000000000006`), recorded as a comment in the migration for plan 03 to mirror as a Go constant — this is the load-bearing fix for Pitfall 1 (a readable sentinel like the string `'public-web'` would make `audit.parseUUID` reject every public submission's audit row and roll back the whole transaction).
- The public-web user is assigned the least-privileged `maker` role (T-06-03) purely as defense-in-depth — it never receives a real Keycloak credential and is never used to log in; the role exists only so that IF that exact subject were ever forged into a JWT, `auth.ResolveAppUser` would resolve it to the lowest-privilege role rather than an unassigned one.
- `ListFilter.Source`/`DonationListItem.Source` were added to `model.go`, not `service.go` as the plan's `files_modified` frontmatter listed — `ListFilter`/`DonationListItem` are actually declared in `model.go` (Rule 3: files_modified path was inaccurate; corrected without requiring a plan-check round-trip).
- `DonationListItem` gained a `Source` field even though Task 2's acceptance criteria didn't strictly require exposing it in the response — `06-RESEARCH.md`'s exact `SearchDonations` code example includes `d.source` in the SELECT, and plan 07's staff queue screen (`key_links` in this plan's frontmatter) will need the per-row source value without a second round-trip (Rule 2, minimal low-risk addition).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking/inaccurate reference] `ListFilter`/`DonationListItem` live in `model.go`, not `service.go`**
- **Found during:** Task 2
- **Issue:** Plan's `files_modified` frontmatter and Task 2's `<files>` both list `donnarec-api/internal/donation/service.go` as the location to add `ListFilter.Source`. Grepping the codebase showed `ListFilter`/`DonationListItem` are actually declared in `internal/donation/model.go`.
- **Fix:** Added `Source` fields to both structs in `model.go` instead; `service.go` was still modified as planned (for the `Search()` wiring), just not for the struct definitions.
- **Files modified:** `donnarec-api/internal/donation/model.go` (not originally in `files_modified`, but a plan-adjacent file in the same package)
- **Verification:** `go build ./...` green; `TestSearchDonations_SourceFilter` passes.
- **Committed in:** `540d0b2` (Task 2 GREEN commit)

**2. [Rule 1 - Bug/staleness] Regenerated sqlc output for `edonation.sql.go`/`querier.go` picked up a pre-existing stale doc-comment**
- **Found during:** Task 2, after running `sqlc generate`
- **Issue:** `internal/db/queries/edonation.sql`'s `SearchUnkeyedIssued` comment (committed in phase 05, `1f835ff`) had a doc-comment addition that was never regenerated into `internal/db/generated/edonation.sql.go`/`querier.go` — a pure comment-sync gap, unrelated to this plan's `source` column work, but surfaced as part of the same `sqlc generate` invocation this task required.
- **Fix:** Committed the regenerated comment sync alongside the `source`-related changes rather than hand-reverting an unrelated portion of a single deterministic `sqlc generate` output — the generated directory is marked DO NOT EDIT and should always match the current `.sql` source files.
- **Files modified:** `donnarec-api/internal/db/generated/edonation.sql.go`, `donnarec-api/internal/db/generated/querier.go` (comment-only diff, no behavior change)
- **Verification:** `go build ./...` green, `go vet ./...` clean, full `internal/*` test suites unaffected by the comment-only diff.
- **Committed in:** `540d0b2` (Task 2 GREEN commit)

---

**3. [Rule 1 - Bug] Did NOT mark FR-01/FR-08 complete in REQUIREMENTS.md despite this plan's frontmatter listing them**
- **Found during:** state/requirements update step
- **Issue:** This plan's frontmatter lists `requirements: [FR-01, FR-08]`, and the standard executor protocol step (`requirements mark-complete`) mechanically checks off every ID in that list. Grepping all 8 plans in this phase shows FR-01 is ALSO listed in 06-03 and 06-06 (the actual public form UI/submission work), and FR-08 is ALSO listed in 06-07 (the actual staff queue screen). This plan only built the data-foundation layer (source column, seed user, filter plumbing) — the donor-facing form (FR-01) and the staff-visible queue UI (FR-08) do not exist in the codebase yet. Running the mechanical mark-complete step checked both boxes and set both to "Complete" in the traceability table, which would misrepresent phase progress to anyone reading REQUIREMENTS.md.
- **Fix:** Ran `requirements mark-complete FR-01 FR-08` as instructed, observed the resulting incorrect "Complete" state, then manually reverted `.planning/REQUIREMENTS.md` to its prior state (both checkboxes back to `[ ]`, both traceability rows back to "Pending") before committing. FR-01/FR-08 will be correctly marked complete by whichever later plan (06-03/06-06 for FR-01, 06-07 for FR-08) actually finishes the last piece of user-facing functionality.
- **Files modified:** `.planning/REQUIREMENTS.md` (reverted to original, net no-op in the final commit)
- **Verification:** `git diff .planning/REQUIREMENTS.md` shows no changes after the revert.
- **Committed in:** n/a — reverted before the final metadata commit; REQUIREMENTS.md is unchanged in this plan's history.

---

**Total deviations:** 3 auto-fixed (1 blocking/inaccurate-reference, 1 bug/staleness-sync, 1 bug/premature-requirement-completion)
**Impact on plan:** All three were necessary corrections surfaced while implementing the plan exactly as specified functionally; none changed the plan's intended behavior or scope. No scope creep.

## Issues Encountered
- Task 1's specified `<verify><automated>` command (`rtk go test ./internal/testutil/... -run TestStartPostgres -count=1`) references a test name (`TestStartPostgres`) that does not exist anywhere in the codebase — `internal/testutil` has zero `_test.go` files. Ran as specified (reports "no tests found", not a failure), but since it does not actually exercise the migration chain, migration correctness (both up and down, for 000015 and 000016) was independently verified manually: applied the full 000001-000016 chain against a throwaway `postgres:17` Docker container, inserted a test donation to confirm the `source` DEFAULT backfill, then reversed 000016→000015 and confirmed both the seeded user and the `source` column were cleanly removed. This is also implicitly re-proven by every `testutil.SetupTestPostgres`-based test across the suite (all run the full up chain) — all passed.
- One flaky failure (`TestEditDraft`) surfaced on a single full-package parallel test run (`go test ./internal/donation/...`), traced to Docker/testcontainers resource contention under parallel container spin-up (not a code regression) — confirmed by rerunning the single test in isolation (pass) and rerunning the full package suite clean (pass, `ok ... 62.242s`).

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- Plan 03 (public submission create path) can now FK `created_by` to the fixed-UUID public-web user (`00000000-0000-4000-8000-000000000006`) and use its `keycloak_subject` as the synthetic audit actor without tripping `audit.parseUUID`.
- Plan 07 (staff pending-review queue screen) can call the existing `GET /api/donations?source=flow_b&status=pending_review` — both the `source` and `status` narg filters compose via `AND`, exactly matching the D-53 pattern, and `DonationListItem.Source` is already in the response payload.
- No blockers identified for downstream plans in this phase.

---
*Phase: 06-public-donation-web-form-flow-b*
*Completed: 2026-07-11*

## Self-Check: PASSED

All 9 files referenced in this summary confirmed present on disk; all 3 commit hashes (`6a045cb`, `a825acd`, `540d0b2`) confirmed present in `git log --oneline --all`.
