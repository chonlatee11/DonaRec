---
phase: 05-e-donation-export-reports-admin-settings
plan: 01
subsystem: database
tags: [postgres, sqlc, excelize, jsonb, golang-migrate, pgx]

requires:
  - phase: 04-receipt-pdf-email-delivery-outbox-worker
    provides: receipt_template_config single-row config-store pattern (000011) mirrored here for edonation_config
provides:
  - Migrations 000013 (donations.edonation_keyed_at/edonation_keyed_by) and 000014 (edonation_config single-row table)
  - sqlc query set for export/aging/keyed/report/config (SearchIssuedForExport, SearchUnkeyedIssued, SetKeyedBulk, GetEdonationConfig, UpdateEdonationConfig, SummaryByMonth, SummaryByDay)
  - internal/exportfile stream-only .xlsx/.csv writer (StreamXLSX, StreamCSV, SetDownloadHeaders)
  - internal/edonation Config accessor + config-driven FieldMapping (D-75)
  - excelize/v2 v2.11.0 dependency
affects: [05-02, 05-03, 05-04, 05-05, 05-06, 05-07]

tech-stack:
  added: [github.com/xuri/excelize/v2 v2.11.0]
  patterns:
    - "Stream-only file generation: io.Writer-only signatures, never os.Create/TempFile (D-74)"
    - "Config-driven column order: FieldMapping JSONB decoded to a typed, ordered column list — HeaderRow/RowValues are the single source of export column order/names (D-75)"
    - "Single-row config table (id BOOLEAN PRIMARY KEY DEFAULT true + CHECK(id=true)) — third instance of this pattern (000004, 000011, now 000014)"

key-files:
  created:
    - donnarec-api/migrations/000013_edonation_keyed_metadata.up.sql
    - donnarec-api/migrations/000013_edonation_keyed_metadata.down.sql
    - donnarec-api/migrations/000014_edonation_config.up.sql
    - donnarec-api/migrations/000014_edonation_config.down.sql
    - donnarec-api/internal/db/queries/edonation.sql
    - donnarec-api/internal/db/queries/reports.sql
    - donnarec-api/internal/exportfile/writer.go
    - donnarec-api/internal/exportfile/writer_test.go
    - donnarec-api/internal/edonation/config.go
    - donnarec-api/internal/edonation/config_test.go
  modified:
    - donnarec-api/go.mod
    - donnarec-api/go.sum
    - donnarec-api/internal/db/queries/donations.sql
    - donnarec-api/internal/db/generated/donations.sql.go
    - donnarec-api/internal/db/generated/models.go
    - donnarec-api/internal/db/generated/querier.go

key-decisions:
  - "GetDonationByID's SELECT list extended to include edonation_keyed_at/edonation_keyed_by (in physical column order) so sqlc keeps reusing the Donation model type instead of splitting a divergent *Row type — required to keep go build green after the 000013 ALTER TABLE"
  - "FieldMapping.RowValues(row map[string]string) takes a plain column_key->value map rather than a concrete ExportRow type, since ExportRow belongs to a downstream export-slice plan (05-02+) not yet built"
  - "Config accessor merges DTO + accessor into one type (edonation.Config, constructed via NewConfig(*db.Queries)) per the plan's literal GetConfig(ctx)(Config,error)/UpdateConfig(ctx,Config,pgtype.UUID)error contract"

patterns-established:
  - "edonation_config.field_mapping empty-array/nil fallback: DecodeFieldMapping falls back to a Go-side default mapping (mirroring migration 000014's seed) only when the config is genuinely empty — never silently overrides a real admin-configured mapping"

requirements-completed: [FR-30, FR-31, FR-32]

coverage:
  - id: D1
    description: "Migrations 000013/000014 apply and roll back cleanly; edonation_config seeded with a usable default field_mapping"
    requirement: "FR-30"
    verification:
      - kind: integration
        ref: "go test -count=1 ./internal/edonation/... -run TestConfig_GetConfig_RoundTrip (real postgres:17 testcontainer, migrations 000001-000014 applied)"
        status: pass
    human_judgment: false
  - id: D2
    description: "sqlc query set (SearchIssuedForExport, SearchUnkeyedIssued, SetKeyedBulk, GetEdonationConfig, UpdateEdonationConfig, SummaryByMonth, SummaryByDay) generates typed Go, no hand-edited generated code"
    requirement: "FR-30"
    verification:
      - kind: unit
        ref: "go build ./... && sqlc generate -f internal/db/sqlc.yaml (zero diff on second consecutive run)"
        status: pass
    human_judgment: false
  - id: D3
    description: "internal/exportfile streams valid .xlsx (ZIP signature) and BOM-prefixed UTF-8 .csv with Thai text round-tripping, io.Writer only, no temp file"
    requirement: "FR-30"
    verification:
      - kind: unit
        ref: "internal/exportfile/writer_test.go — TestStreamXLSX_ZipSignature, TestStreamXLSX_ThaiRoundTrip, TestStreamXLSX_IOWriterOnly, TestStreamCSV_BOMLeadingBytes, TestStreamCSV_ThaiTextPresent, TestStreamCSV_IOWriterOnly, TestSetDownloadHeaders"
        status: pass
    human_judgment: false
  - id: D4
    description: "internal/edonation Config accessor reads/writes field mapping + near_due_days from edonation_config; FieldMapping.HeaderRow ordering is driven by JSONB config, not hardcoded"
    requirement: "FR-31"
    verification:
      - kind: unit
        ref: "internal/edonation/config_test.go — TestConfig_DecodeFieldMapping_JSONB, TestConfig_HeaderRow_Ordering, TestConfig_RowValues_FollowsColumnOrder, TestConfig_DecodeFieldMapping_EmptyFallsBackToDefault"
        status: pass
      - kind: integration
        ref: "internal/edonation/config_test.go — TestConfig_GetConfig_RoundTrip, TestConfig_UpdateConfig_ReordersHeaders (real postgres:17 testcontainer)"
        status: pass
    human_judgment: false

duration: 20min
completed: 2026-07-07
status: complete
---

# Phase 5 Plan 01: Shared Data Layer & File-Streaming Substrate Summary

**Migrations 000013/000014 + sqlc query set (export/aging/keyed/report/config) + a stream-only excelize/csv writer + a config-driven FieldMapping accessor — the shared substrate the export/aging/report slices (05-02..05-07) build on.**

## Performance

- **Duration:** ~20 min
- **Started:** 2026-07-07T11:20:00Z (approx.)
- **Completed:** 2026-07-07T11:41:10Z
- **Tasks:** 2
- **Files modified:** 20 (14 in Task 1, 6 in Task 2)

## Accomplishments
- Migration 000013 adds `donations.edonation_keyed_at`/`edonation_keyed_by` (D-51 metadata columns); migration 000014 creates the single-row `edonation_config` table (field_mapping JSONB, cash_type_label, near_due_days) seeded with a usable default 5-column export mapping
- Full sqlc query set for Phase 5 generated: `SearchIssuedForExport`, `SearchUnkeyedIssued`, `SetKeyedBulk`, `GetEdonationConfig`, `UpdateEdonationConfig` (edonation.sql); `SummaryByMonth`, `SummaryByDay` (reports.sql) — all filters via `sqlc.narg(...)`, matching `donations.sql`'s nullable-filter discipline
- `internal/exportfile` streams `.xlsx` (ZIP-signature-verified) and BOM-prefixed UTF-8 `.csv` directly to an `io.Writer`, with Thai header/cell text proven to round-trip byte-for-byte; `SetDownloadHeaders` emits both an ASCII `filename=` fallback and an RFC 5987 `filename*=UTF-8''...` parameter for Thai filenames
- `internal/edonation` `Config` accessor reads/writes `edonation_config`; `FieldMapping` decodes the JSONB column into an ordered, typed column list with `HeaderRow(locale)`/`RowValues(row)` as the single source of export column order (D-75) — proven end-to-end against a real Postgres instance (reordering the config's JSONB reorders the derived header row)
- `excelize/v2 v2.11.0` added as a direct dependency (verified [Approved] in 05-RESEARCH's Package Legitimacy Audit)

## Task Commits

Each task was committed atomically:

1. **Task 1: Migrations 000013 + 000014, sqlc query set, and excelize dependency** - `c20706f` (feat)
2. **Task 2: Stream-only xlsx/csv writer + edonation config accessor** - `a98dc90` (feat, TDD: RED tests written and confirmed failing before writer.go/config.go existed, then implemented to GREEN)

## Files Created/Modified
- `donnarec-api/migrations/000013_edonation_keyed_metadata.up/down.sql` - donations.edonation_keyed_at/edonation_keyed_by (D-51)
- `donnarec-api/migrations/000014_edonation_config.up/down.sql` - edonation_config single-row table, seeded default field_mapping
- `donnarec-api/internal/db/queries/edonation.sql` - SearchIssuedForExport, SearchUnkeyedIssued, SetKeyedBulk, GetEdonationConfig, UpdateEdonationConfig
- `donnarec-api/internal/db/queries/reports.sql` - SummaryByMonth, SummaryByDay (no-PII aggregates)
- `donnarec-api/internal/db/queries/donations.sql` - GetDonationByID extended to select the two new columns (Rule 1 fix, see Deviations)
- `donnarec-api/internal/exportfile/writer.go` - StreamXLSX, StreamCSV, SetDownloadHeaders
- `donnarec-api/internal/exportfile/writer_test.go` - RED-then-GREEN tests (ZIP signature, Thai round-trip, BOM, Content-Disposition)
- `donnarec-api/internal/edonation/config.go` - FieldMapping, FieldMappingColumn, Config accessor (NewConfig/GetConfig/UpdateConfig)
- `donnarec-api/internal/edonation/config_test.go` - unit tests (decode/ordering/fallback) + testcontainers integration tests
- `donnarec-api/go.mod` / `go.sum` - excelize/v2 v2.11.0 (direct dependency after go mod tidy)

## Decisions Made
- `GetDonationByID`'s explicit column list now includes `edonation_keyed_at`/`edonation_keyed_by` in physical table-column order — after migration 000013 adds two columns to `donations`, sqlc otherwise stops reusing the `Donation` model type for that query (since its column list no longer matches the full table) and instead generates a diverging `GetDonationByIDRow` type, which broke every existing caller in `internal/donation/service.go` and `internal/worker/issue_receipt.go` that declares `db.Donation`-typed variables. Extending the SELECT list (in the same physical order sqlc requires to reuse the named struct) is a minimal, low-risk fix consistent with `GetDonationByID`'s own "full read of a donation row" contract.
- `FieldMapping.RowValues` accepts a plain `map[string]string` rather than a concrete `ExportRow` DTO — `ExportRow` is owned by a later export-slice plan (05-02+) not yet built; keeping the substrate package's public API free of a forward dependency lets 05-02 define its own row type without a breaking change here.
- `edonation.Config` merges the DTO (FieldMapping/CashTypeLabel/NearDueDays/UpdatedAt/UpdatedBy) and the accessor (constructed via `NewConfig(*db.Queries)`, holding an unexported `queries` field) into one type, matching the plan's literal `GetConfig(ctx) (Config, error)` / `UpdateConfig(ctx, Config, pgtype.UUID) error` method signatures.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Extended `GetDonationByID`'s SELECT list to keep `go build` green after migration 000013**
- **Found during:** Task 1 (`go build ./...` after `sqlc generate`)
- **Issue:** Migration 000013 adds `edonation_keyed_at`/`edonation_keyed_by` to `donations`. `internal/db/queries/donations.sql`'s `GetDonationByID` query lists columns explicitly (not `SELECT *`) and previously matched the table's full column set exactly, so sqlc reused the `Donation` model struct as its return type. After the ALTER TABLE, that explicit list no longer covered all columns, so sqlc generated a new `GetDonationByIDRow` type instead — breaking every existing call site typed as `db.Donation` (`internal/donation/service.go`, `internal/worker/issue_receipt.go`).
- **Fix:** Added `edonation_keyed_at, edonation_keyed_by` to `GetDonationByID`'s SELECT list, in the same order they physically appear on the table (required for sqlc to keep reusing the `Donation` struct name), then regenerated.
- **Files modified:** `donnarec-api/internal/db/queries/donations.sql`, `donnarec-api/internal/db/generated/donations.sql.go`, `models.go`, `querier.go`
- **Verification:** `go build ./...` succeeds repo-wide; `go test -short ./...` green across all 19 packages (no regressions in `internal/donation`, `internal/worker`)
- **Committed in:** `c20706f` (Task 1 commit)

---

**Total deviations:** 1 auto-fixed (1 bug fix, Rule 1)
**Impact on plan:** Necessary to keep the existing donation lifecycle code compiling after adding the two new columns this plan's own migration introduces. No scope creep — the fix stays entirely within `GetDonationByID`'s existing "full row read" contract.

## Issues Encountered
- `go mod tidy` immediately after `go get github.com/xuri/excelize/v2@v2.11.0` in Task 1 pruned the dependency again (nothing imported it yet, since `internal/exportfile` — the only importer — is created in Task 2). Resolved by running `go get` alone (without `tidy`) for Task 1, satisfying that task's "excelize present in go.mod" acceptance criterion without a false GREEN, then running `go mod tidy` in Task 2 once `writer.go` genuinely imports the package (promoting it from `// indirect` to a direct require).

## TDD Gate Compliance

Task 2 (`tdd="true"`) followed RED→GREEN: `writer_test.go`/`config_test.go` were written and confirmed failing (package build errors — "no non-test Go files") before `writer.go`/`config.go` existed, then implementation was added and all tests turned green (including the two Postgres-testcontainer integration tests). However, per this executor's standard task-commit protocol, RED and GREEN were committed together in a single `feat(05-01): ...` commit (`a98dc90`) rather than as separate `test(...)` → `feat(...)` commits — there is no standalone `test(05-01): ...` commit in the git history for Task 2. The RED state was verified via `go vet`/`go build` output at the time (captured in this execution's transcript), not via a separate commit. No REFACTOR step was needed (implementation matched the RED tests' expectations on the first GREEN pass).

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- 05-02 (export slice) can now build `edonation.Service.Export` directly on `SearchIssuedForExport`, `internal/exportfile.StreamXLSX/StreamCSV`, and `edonation.Config`/`FieldMapping` — no further schema or streaming-substrate work needed.
- 05-04/05-05 (aging, reports) can build directly on `SearchUnkeyedIssued`/`SetKeyedBulk` and `SummaryByMonth`/`SummaryByDay` respectively.
- 05-07 (admin settings 5th tab) can build directly on `edonation.NewConfig(queries).GetConfig`/`UpdateConfig`.
- No blockers. The real RD e-Donation field-spec confirmation (stakeholder gate, STATE.md Blockers/Concerns) remains open but non-blocking — `edonation_config.field_mapping` is admin-editable without a deploy once that spec is confirmed (D-75's whole point).

---
*Phase: 05-e-donation-export-reports-admin-settings*
*Completed: 2026-07-07*

## Self-Check: PASSED

All 10 created files verified present on disk; both task commits (`c20706f`, `a98dc90`) verified present in git history.
