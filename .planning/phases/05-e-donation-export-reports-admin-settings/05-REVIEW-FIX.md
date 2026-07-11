---
phase: 05-e-donation-export-reports-admin-settings
fixed_at: 2026-07-11T00:00:00Z
review_path: .planning/phases/05-e-donation-export-reports-admin-settings/05-REVIEW.md
iteration: 1
findings_in_scope: 6
fixed: 6
skipped: 0
status: all_fixed
---

# Phase 05: Code Review Fix Report

**Fixed at:** 2026-07-11
**Source review:** .planning/phases/05-e-donation-export-reports-admin-settings/05-REVIEW.md
**Iteration:** 1

**Summary:**
- Findings in scope: 6 (fix_scope = all — Critical + Warning + Info)
- Fixed: 6
- Skipped: 0

All fixes were applied inside an isolated git worktree, verified per-fix (gofmt +
`go build` for Go, `tsc --noEmit` for TypeScript, plus `go test ./internal/report/...`
where relevant), and committed atomically. The three Warning fixes involve runtime
behavior/logic that structural checks cannot fully confirm and are flagged
**requires human verification** below.

## Fixed Issues

### CR-01: CSV/formula injection (CWE-1236) in the e-Donation CSV export

**Files modified:** `donnarec-api/internal/exportfile/writer.go`
**Commit:** 9d5226f
**Status:** fixed
**Applied fix:** Added `sanitizeCSVField` + `sanitizeCSVRow` inside the shared
`exportfile` package and routed every header and data cell in `StreamCSV` through
them. A leading `=`, `+`, `-`, `@`, tab, or CR is prefixed with an apostrophe so
Excel/Sheets/LibreOffice treat the cell as literal text. Applied inside
`StreamCSV` itself so every current and future CSV export (e-Donation + reports)
is protected by construction, not by caller discipline. The `.xlsx` path is
correctly left unchanged (excelize stores explicit string cell types).

### WR-01: Export count-preview + label used a different date field than the backend filter

**Files modified:** `donnarec-api/internal/db/queries/edonation.sql`,
`donnarec-api/internal/db/generated/edonation.sql.go`,
`donnarec-api/internal/edonation/model.go`,
`donnarec-api/internal/edonation/service.go`,
`donnarec-api/internal/edonation/handler.go`,
`donnarec-web/messages/th.json`, `donnarec-web/messages/en.json`,
`donnarec-web/lib/edonation.ts`, `donnarec-web/components/ExportPanel.tsx`
**Commit:** 1f835ff
**Status:** fixed: requires human verification
**Applied fix:** Chose `donated_at` (the field the export endpoint already
filters on, D-66) as the single canonical date field. Exposed `donated_at` on
the aging query/response (`SearchUnkeyedIssued` SELECT + generated Row/Scan +
`AgingRow` model + `AgingRowResponse` DTO + TS `AgingRow` type), switched the
`ExportPanel` client-side count preview to filter on `row.donated_at`, and
relabeled the filter to "วันที่บริจาค / Donation date". Now label, preview, and
backend filter all agree.
**Why human verification:** sqlc was not installed in this environment, so the
generated `edonation.sql.go` was hand-synced to the SQL change; the added
`donated_at` column selection could not be validated against a live Postgres.
`go build` and `tsc` pass, but a run against the real stack should confirm the
aging endpoint returns `donated_at` and the preview count now matches the rows
the export actually streams.

### WR-02: Bulk "Mark" overwrote keying provenance for already-keyed rows

**Files modified:** `donnarec-api/internal/edonation/service.go`
**Commit:** 0424eed
**Status:** fixed: requires human verification
**Applied fix:** Chose the robust backend (defense-by-construction) option from
the review. Extended the pre-update scope SELECT in `Service.SetKeyed` with
`AND edonation_keyed <> $2` (the requested target), so `issuedIDs` now contains
only rows that will actually transition. Already-keyed rows in a mixed selection
are excluded from both the `SetKeyedBulk` UPDATE and the per-donation audit loop
— they keep their original `edonation_keyed_at`/`edonation_keyed_by` and produce
no misleading "new keying event" audit row. The all-no-op case returns cleanly
(no UPDATE, no audit, no error), mirroring the existing all-cancelled-ids no-op.
**Why human verification:** state-handling logic change on an audited path;
structural checks can't confirm the audit-row count now matches the true blast
radius. A run marking a mixed already-keyed + not-yet-keyed selection should
confirm provenance is preserved and only new transitions are audited.

### WR-03: Donation summary report aggregated money as `float64`

**Files modified:** `donnarec-api/internal/report/service.go`
**Commit:** c595545
**Status:** fixed: requires human verification
**Applied fix:** Replaced the float64 accumulation with exact `math/big.Rat`
math. Added `numericToRat` (converts a `pgtype.Numeric`'s `Int × 10^Exp` to an
exact rational, rejecting NaN/±Infinity) and a running `totalAmountRat`; the
top-line `TotalAmount` and `AveragePerReceipt` are derived from the rational
total and converted to float64 only once, at the JSON/display boundary — in line
with CLAUDE.md's money-precision convention. Per-row display values remain a
single-SUM float (the drift risk was in cross-row accumulation). Existing report
package tests pass (5/5).
**Why human verification:** money-math logic change; while unit tests pass, a
reconciliation of a real multi-row report total against the DB's authoritative
`NUMERIC(15,2)` sum is the definitive confirmation.

### IN-01: e-Donation field-mapping config accepted empty column_key/headers

**Files modified:** `donnarec-api/internal/edonation/config.go`
**Commit:** ed7de78
**Status:** fixed
**Applied fix:** Added `validate` tags to `FieldMappingColumn` (which
`handler.ConfigRequest`'s `dive` already descends into): `HeaderTh`/`HeaderEn`
get `required`, and `ColumnKey` gets `required,oneof=national_id donated_at
cash_type receipt_no donor_name`. The allowlist was verified against the export
service's row map keys (`xlsx.go` `rowToMap`), so a typo'd or blank column key is
now rejected at save time instead of silently producing a blank export column.

### IN-02: `ErrNoRecords` sentinel declared but never returned

**Files modified:** `donnarec-api/internal/edonation/errors.go`,
`donnarec-api/internal/edonation/handler.go`
**Commit:** f34dd96
**Status:** fixed
**Applied fix:** Removed the dead `ErrNoRecords` sentinel and corrected both the
`errors.go` and handler package doc comments to describe the actual empty-export
404 path (handler emits it directly on `len(rows)==0` after `Service.Export`
returns). Chose removal over wiring the sentinel through the service because the
existing `len(rows)==0` handler check already implements the D-74 empty-file
guard correctly; verified no other reference to the symbol remains.

## Skipped Issues

None — all in-scope findings were fixed.

---

_Fixed: 2026-07-11_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 1_
