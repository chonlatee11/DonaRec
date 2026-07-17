---
phase: 05-e-donation-export-reports-admin-settings
reviewed: 2026-07-07T00:00:00Z
depth: standard
files_reviewed: 33
files_reviewed_list:
  - donnarec-api/cmd/server/main.go
  - donnarec-api/internal/edonation/aging.go
  - donnarec-api/internal/edonation/config.go
  - donnarec-api/internal/edonation/csv.go
  - donnarec-api/internal/edonation/errors.go
  - donnarec-api/internal/edonation/handler.go
  - donnarec-api/internal/edonation/model.go
  - donnarec-api/internal/edonation/service.go
  - donnarec-api/internal/edonation/xlsx.go
  - donnarec-api/internal/exportfile/writer.go
  - donnarec-api/internal/report/errors.go
  - donnarec-api/internal/report/handler.go
  - donnarec-api/internal/report/model.go
  - donnarec-api/internal/report/service.go
  - donnarec-api/internal/db/queries/edonation.sql
  - donnarec-api/migrations/000013_edonation_keyed_metadata.up.sql
  - donnarec-api/migrations/000014_edonation_config.up.sql
  - donnarec-api/scripts/backup.sh
  - donnarec-api/scripts/restore.sh
  - donnarec-web/app/e-donation/page.tsx
  - donnarec-web/app/reports/page.tsx
  - donnarec-web/app/api/bff/edonation/export/route.ts
  - donnarec-web/app/api/bff/edonation/keyed/route.ts
  - donnarec-web/app/api/bff/edonation/aging/route.ts
  - donnarec-web/app/api/bff/edonation-config/route.ts
  - donnarec-web/app/api/bff/reports/route.ts
  - donnarec-web/app/api/bff/reports/export/route.ts
  - donnarec-web/components/ExportPanel.tsx
  - donnarec-web/components/ExportConfirmDialog.tsx
  - donnarec-web/components/AgingTable.tsx
  - donnarec-web/components/AgingStatCards.tsx
  - donnarec-web/components/BulkActionBar.tsx
  - donnarec-web/components/KeyedStatusBadge.tsx
  - donnarec-web/components/AgingBucketBadge.tsx
  - donnarec-web/components/ReportSummaryCards.tsx
  - donnarec-web/components/ReportBreakdownTable.tsx
  - donnarec-web/components/EdonationConfigTab.tsx
  - donnarec-web/components/SettingsTabs.tsx
  - donnarec-web/components/AppShell.tsx
  - donnarec-web/components/ui/checkbox.tsx
  - donnarec-web/lib/edonation.ts
  - donnarec-web/lib/reports.ts
  - donnarec-web/lib/session-role.ts
findings:
  critical: 1
  warning: 3
  info: 2
  total: 6
status: issues-found
---

# Phase 05: Code Review Report

**Reviewed:** 2026-07-07
**Depth:** standard
**Files Reviewed:** 33 (source files; generated sqlc code and `_test.go` files out of scope per instructions)
**Status:** issues_found

## Summary

Phase 5 (e-Donation Export, Reports & Admin Settings) is broadly well-structured: RBAC wiring in `cmd/server/main.go` correctly separates the audited/PII-bearing e-Donation export/keyed/aging routes (`RequireAnyRole(Checker,Admin)`) from the deliberately-open, PII-free Reports routes (`RequireAuth()` only, D-71), the export/report writers are genuinely stream-only (no temp files, `io.Writer`-only signatures), and the audited-decrypt discipline for `Service.Export` (one summary audit row committed before plaintext is ever returned) mirrors the existing `RevealPII` pattern correctly. BFF routes correctly keep the Keycloak bearer token server-side and correctly bypass JSON parsing for binary xlsx/csv passthrough.

However, one genuine, unmitigated **CSV/formula injection vulnerability** was found in the e-Donation CSV export path — donor-supplied free text flows unescaped into a CSV file that is also carrying the donor's full plaintext national ID, which is a real security defect given this project's PII-sensitivity requirements. Three further correctness/quality issues were found: a three-way date-field mismatch between the Export tab's filter label, its client-side "record count" preview, and the backend's actual filter column (undermining the PII-export confirmation dialog's purpose); a bulk "mark keyed" action that can silently overwrite keying provenance for already-keyed rows; and a money aggregate computed in `float64` where the project's own conventions call for precision-preserving numeric types. Two minor Info-level findings (dead sentinel error, missing field-level config validation) round out the list.

## Critical Issues

### CR-01: CSV/formula injection (CWE-1236) in the e-Donation CSV export — unescaped donor-controlled text next to plaintext PII

**File:** `donnarec-api/internal/exportfile/writer.go:75-96` (`StreamCSV`), `donnarec-api/internal/edonation/csv.go:16-19` (`WriteCSV`), `donnarec-api/internal/edonation/xlsx.go:22-30` (`rowToMap`)

**Issue:** `StreamCSV` writes each row's string values directly via `encoding/csv` with no inspection of the leading character. `ExportRow.DonorName` (and, in principle, any other free-text column an admin later adds to `field_mapping`) is sourced from `donations.donor_name`, whose only server-side validation is `validate:"required,min=1,max=255"` (`donnarec-api/internal/donation/model.go:19`) — there is no character allowlist. If a donor name is entered (or maliciously crafted) starting with `=`, `+`, `-`, or `@`, Microsoft Excel (and many other spreadsheet tools) will interpret that cell as a live formula/DDE payload the moment a Checker/Admin opens the exported `.csv` file — the canonical CSV/Formula Injection class (OWASP, CWE-1236). This is materially worse than a typical CSV-injection finding because the SAME exported row also carries the donor's full plaintext 13-digit national/tax ID (D-64) — an attacker who can influence a donation's `donor_name` (a Maker mistake today; a future public donation form in Phase 6, per CLAUDE.md's stack notes) can plant a payload that executes on the machine of the staff member who just decrypted and is viewing the most sensitive PII this system produces.

Note: the `.xlsx` path (`internal/exportfile/writer.go`'s `StreamXLSX`) is NOT vulnerable to this specific class — `excelize`'s `SetCellValue`/`SetCellStr` stores string values with an explicit string cell type (never a formula type `f`), so Excel does not reinterpret `.xlsx` string cells as formulas on open. Only the CSV path is exploitable, because Excel's CSV importer has no prior type information and applies its own leading-character heuristic.

**Fix:**
```go
// internal/exportfile/writer.go
func sanitizeCSVField(v string) string {
	if v == "" {
		return v
	}
	switch v[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + v // leading apostrophe forces text interpretation in Excel/Sheets/LibreOffice
	}
	return v
}

// in StreamCSV, before cw.Write(headers) / cw.Write(row):
sanitizedHeaders := make([]string, len(headers))
for i, h := range headers {
	sanitizedHeaders[i] = sanitizeCSVField(h)
}
...
sanitizedRow := make([]string, len(row))
for i, v := range row {
	sanitizedRow[i] = sanitizeCSVField(v)
}
```
Apply this inside `exportfile.StreamCSV` itself (not per-caller) so every current and future CSV export in this codebase (e-Donation export, report export) is protected by construction, not by caller discipline.

## Warnings

### WR-01: Export "record count" preview and the PII-warning dialog use a different date field than the actual export filter — three-way mismatch

**File:** `donnarec-web/components/ExportPanel.tsx:72-82`, `donnarec-api/internal/edonation/model.go:20-25`, `donnarec-api/internal/db/queries/edonation.sql:22-26`

**Issue:** The Export tab's date-range filter label reads "วันที่ออกใบเสร็จ ตั้งแต่" (receipt-issuance date — `messages/th.json`'s `eDonationExport.filterFrom`). The actual backend filter (`SearchIssuedForExport`, `internal/db/queries/edonation.sql:23-24`) filters on `donated_at` — documented explicitly in `ExportFilter`'s doc comment as "inclusive lower bound on donated_at" (`internal/edonation/model.go:21`). But the client-side "record count preview" shown above the export buttons, and reused verbatim inside `ExportConfirmDialog`'s PII-warning text ("ไฟล์นี้มีเลขประจำตัว...ของผู้บริจาค {n} ราย"), is computed by filtering the shared Aging query's rows on a **third** field, `row.approved_at` (`ExportPanel.tsx:77`: `const approvedDate = row.approved_at.slice(0, 10)`). For any donation where `donated_at`, `approved_at`, and the intuitive "receipt issued date" diverge (the normal case — approval happens some time after donation), the number a Checker sees and confirms before an audited PII export can differ from the number of rows actually streamed, undermining the entire purpose of showing a count in the confirmation dialog.

**Fix:** Pick one canonical date field for this workflow and use it consistently in the label, the preview computation, and the backend filter — e.g. relabel the filter "วันที่บริจาค ตั้งแต่" (donation date) and derive the client-side preview from `donated_at` (would require exposing `donated_at` on the aging response, or adding a lightweight `?count=1` mode to the export endpoint that returns only a count without decrypting/auditing).

### WR-02: Bulk "Mark" action re-writes keying provenance for already-keyed rows in a mixed selection

**File:** `donnarec-web/components/AgingTable.tsx:136,284-291`, `donnarec-api/internal/edonation/service.go:239-249`

**Issue:** `canMark` only requires that **at least one** selected row is unmarked (`canMark = selectedRows.some((r) => !r.keyed)`, `AgingTable.tsx:136`), but clicking "Mark" submits the **entire** current selection (`onMark={() => keyedMutation.mutate({ ids: Array.from(selectedIds), keyed: true })}`, `AgingTable.tsx:289`) — not just the unmarked subset. On the backend, `SetKeyedBulk` (`service.go:242-249`) unconditionally overwrites `edonation_keyed_at`/`edonation_keyed_by` for every matched `status='issued'` id, regardless of whether that row was already keyed. If a Checker selects a mix of already-keyed and not-yet-keyed rows and clicks "Mark," every already-keyed row silently loses its original "who/when this was first keyed" metadata and gets a fresh, misleading `edonation.mark_keyed` audit_log entry that looks like a new keying event even though nothing changed from the donor/e-Donation-filing perspective.

**Fix:** Either filter the submitted id set on the client (`selectedRows.filter(r => !r.keyed).map(r => r.id)` for Mark, the symmetric filter for Unmark), or make `SetKeyedBulk`/`Service.SetKeyed` a true no-op (no UPDATE, no audit row) for rows whose `edonation_keyed` already equals the requested target value — mirroring the existing "cancelled donation in the same batch = silent no-op" discipline already applied to the `status='issued'` guard (`service.go:232-237`).

### WR-03: Donation summary report aggregates money as `float64`, contrary to CLAUDE.md's money-precision convention

**File:** `donnarec-api/internal/report/service.go:101-138`

**Issue:** `Service.Summary` converts every breakdown row's `pgtype.Numeric` amount to `float64` via `numericToFloat64` (`service.go:126-138`) and then sums/divides in `float64` (`service.go:101-110`) to compute `TotalAmount`/`AveragePerReceipt`. CLAUDE.md is explicit that "Money amounts must not lose precision (numeric, not int64)" — while this code avoids the int64 pitfall the project specifically calls out, `float64` carries the same category of risk (IEEE-754 cannot exactly represent most base-10 currency fractions), and summing many donation amounts can accumulate visible rounding drift relative to the DB's authoritative `NUMERIC(15,2)` total. This is a report/display path (not the source of truth used to issue receipts), which lowers severity, but it directly conflicts with a load-bearing project convention for a report explicitly meant to reconcile against the DB.

**Fix:** Accumulate `TotalAmount` using `pgtype.Numeric`/`big.Rat` (or integer satang) across the breakdown rows and only convert to `float64`/a formatted string at the JSON/display boundary, mirroring how `donation`/`receiptfmt` already handle money elsewhere in this codebase.

## Info

### IN-01: e-Donation field-mapping config accepts empty `column_key`/`header_th`/`header_en` with no validation

**File:** `donnarec-api/internal/edonation/handler.go:183` (`ConfigRequest.FieldMapping validate:"required,min=1,dive"`), `donnarec-api/internal/edonation/config.go:90-96` (`RowValues`)

**Issue:** `validate:"...,dive"` descends into each `FieldMappingColumn` element, but `FieldMappingColumn` (`config.go:22-26`) carries no field-level `validate` tags of its own — an Admin can save a column mapping with an empty `column_key`, `header_th`, or `header_en`. An empty/unknown `column_key` silently resolves to `""` for every export row (`FieldMapping.RowValues`'s documented "missing keys map to the empty string" behavior), and an empty header produces a blank spreadsheet/CSV header — with no error surfaced anywhere in the admin UI or the API response.

**Fix:** Add `validate:"required"` to `FieldMappingColumn.ColumnKey`/`HeaderTh`/`HeaderEn`, and consider validating `ColumnKey` against the known set (`national_id`, `donated_at`, `cash_type`, `receipt_no`, `donor_name`) so a typo is rejected at save time instead of silently producing blank columns at export time.

### IN-02: `ErrNoRecords` sentinel is declared and documented but never returned

**File:** `donnarec-api/internal/edonation/errors.go:14-18`, `donnarec-api/internal/edonation/handler.go:14` (doc comment), `donnarec-api/internal/edonation/handler.go:131-134` (actual check)

**Issue:** `Handler`'s package doc comment documents `ErrNoRecords → 404 Not Found` as part of the sentinel-error-to-HTTP-status mapping convention this file otherwise follows consistently (`ErrForbidden`, etc.), but `Service.Export` never actually returns `ErrNoRecords` — the empty-result 404 is instead implemented via a direct `len(rows) == 0` check in the handler (`handler.go:131-134`). The sentinel is dead code that also makes the doc comment misleading about how the 404 path actually works.

**Fix:** Either have `Service.Export` return `ErrNoRecords` when the filtered result set is empty and map it via `errors.Is` in the handler (consistent with `ErrForbidden`'s handling), or remove the unused sentinel and correct the doc comment.

---

_Reviewed: 2026-07-07_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
