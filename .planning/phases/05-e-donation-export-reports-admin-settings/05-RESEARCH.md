# Phase 5: e-Donation Export, Reports & Admin Settings - Research

**Researched:** 2026-07-06
**Domain:** Access-controlled bulk PII export (Go + excelize/CSV, streamed, never persisted), gap-less-adjacent "keyed" bookkeeping flag with bulk mutation + audit, deadline/aging bucket computation pinned to Asia/Bangkok, no-PII aggregate reporting, and Postgres+MinIO backup/restore verification inside the existing docker-compose stack.
**Confidence:** HIGH (codebase patterns for decrypt/audit/config-store/testcontainers are directly reused and verified by reading Phase 1–4 source; excelize version and stdlib CSV/BOM behavior verified against the official Go module proxy and library docs; MinIO `mc mirror` / pg_dump-pg_restore mechanics verified against official docs; the exact e-Donation field spec and the report's date-basis are explicitly flagged LOW/ASSUMED per CONTEXT.md's stakeholder gate)

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**e-Donation Export (FR-30, SC#1)**
- **D-62:** Generate BOTH `.xlsx` (excelize) and `.csv`. `.xlsx` is primary output (Thai/UTF-8 renders correctly in Excel, matches CLAUDE.md FR-30 guidance); CSV is a secondary option (must handle BOM/encoding so Excel reads Thai correctly).
- **D-63:** Export access = **Checker + Admin only** — file contains large volumes of plaintext national IDs; restricted to the same roles already trusted with PII reveal (Phase 3 least-privilege precedent).
- **D-64:** National ID in export = **full 13 digits** + **every export writes an audit row** + **download-logged** (SC#1). Export must go through the same audited decrypt path as Phase 3's PII reveal. ⚠️ downstream: UI must warn about file custody after download.
- **D-65:** Cash type = **constant value** ("เงินสด/โอน") — no per-record field (in-kind out of scope).
- **D-66:** Export scope = donations with **status = issued**, filterable by date range (month/fiscal year) + keyed-status; **cancelled excluded**. Connects to the aging view (D-68).

**Keyed flag + Aging (FR-31, SC#2)**
- **D-67:** "คีย์เข้า e-Donation แล้ว" flag settable via **bulk multi-select AND per-row**; every mark/unmark is audited. **Requires a new migration** (flag + timestamp + actor — column vs side table is this document's call, see Pattern 4 below).
- **D-68:** Aging = **3 buckets** (not yet due / near due / overdue) against deadline = **5th of the month after the receipt's issue month** (issue month M → deadline 5th of M+1). "Near due" = **≤3 days** before deadline by default, **stored as adjustable config**. Base date = **issue month (approval date)**, i.e. `approved_at`, NOT `donated_at`.
- **D-69:** Rights to set/unset the flag = **same as export** (Checker + Admin).

**Reports (FR-32, SC#3)**
- **D-70:** Report = date-range picker → total amount + count + month/day breakdown; table + summary cards; exportable to Excel/CSV; **no chart** in MVP.
- **D-71:** Report visible to **all staff** (Maker/Checker/Admin) — no PII, so no RBAC gate needed.

**Backup / Restore (NFR-08, SC#4)**
- **D-72:** Backup = **pg_dump on a schedule inside docker-compose** (companion container/cron) + retention, covering **both DB and MinIO** (slip + frozen-PDF buckets) — object storage cannot be recovered from the DB alone.
- **D-73:** "Restore verified" = **runbook + an actual test restore** against a fresh DB + MinIO, with **assertion that data is complete**, and **recorded evidence** — not merely "cron is configured."

**PII export file handling (PDPA)**
- **D-74:** Export files containing full national IDs = **stream-only download, never persisted** on server or MinIO — generated in memory, sent immediately, nothing plaintext-PII lingers anywhere. (Report export, containing no PII, has no such restriction.)

**e-Donation field mapping (NFR-09, stakeholder gate)**
- **D-75:** Field mapping (column order/names/cash-type constant) = **config-driven**, extending the Phase 4 (D-58) config-store pattern — editable without deploy once the real e-Donation spec is confirmed by stakeholders.

### Claude's Discretion

- Exact schema of the keyed flag (column on `donations` vs side table), export-audit fields, config keys for mapping/threshold — **this document's recommendation: Pattern 4 below**.
- Migration numbering (000013+) and Go package layout (`internal/edonation/`, `internal/report/`, ops-only backup — no Go package needed) — **this document's recommendation: Architecture Patterns below**.
- Numeric defaults: aging "near due" threshold (recommend ≤3 days, config-adjustable), backup retention (days/count), cron schedule — **this document's recommendation: Pattern 5/8 below**.
- Report group-by query (monthly/daily), CSV BOM/encoding strategy — **this document's recommendation: Pattern 2/7 below**.
- MinIO backup mechanism (`mc mirror` vs S3 API dump) and restore-runbook shape — **this document's recommendation: Pattern 8 below**.

### Deferred Ideas (OUT OF SCOPE)

- Direct e-Donation API integration with the Revenue Department — manual export only this milestone (may become in-scope if the 1 Jan 2026 mandate applies; legal sign-off pending).
- Charts/graphs in reports — table + summary cards suffice for MVP.
- Cash type as a per-record field / per-donation 1x/2x — still global config.
- Flow B public donation form (FR-01..06) — Phase 6.
- A dedicated "Exporter"/"Accounting" Keycloak role — Checker+Admin used for now.

</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| FR-30 | ส่งออกข้อมูล (Excel/CSV) เพื่อรองรับการคีย์เข้า e-Donation ด้วยตนเอง | Pattern 1 (stream-only excelize + CSV+BOM generation), Pattern 3 (audited decrypt reuse), Pattern 6 (config-driven field mapping) |
| FR-31 | ติดสถานะ "คีย์เข้า e-Donation แล้ว" เพื่อกันคีย์ซ้ำ/ตกหล่น | Pattern 4 (extend existing `edonation_keyed` column + migration for actor/timestamp), Pattern 5 (aging bucket computation) |
| FR-32 | รายงานสรุปการบริจาค (ตามช่วงเวลา/ยอดรวม) | Pattern 7 (SQL GROUP BY aggregation, no PII, exported via the same excelize/CSV helpers) |
| NFR-08 | สำรองข้อมูล (backup) สม่ำเสมอและกู้คืนได้ | Pattern 8 (pg_dump + `mc mirror` companion services) + Pattern 9 (testcontainers-based restore-proof) |

</phase_requirements>

## Project Constraints (from CLAUDE.md)

These are load-bearing, already-locked stack decisions this phase must not contradict:

- **Stack override:** Go + chi/gin backend, Keycloak OIDC, PostgreSQL 17, Next.js — NOT the original NestJS/TypeScript suggestion (D-20..D-26). Confirmed in codebase: `donnarec-api` uses **gin-gonic**, not chi — CLAUDE.md's chi recommendation was superseded by the actual Phase 1 implementation choice; this research follows the codebase (gin), not the CLAUDE.md router suggestion.
- **excelize (`github.com/xuri/excelize`)** for `.xlsx`; stdlib `encoding/csv` for CSV — both explicitly named in CLAUDE.md's Supporting Libraries table. Neither is in `go.mod` yet.
- **PII encryption-at-rest:** app-level AES-256-GCM envelope (`internal/crypto`), decrypt only through an audited reveal path (`internal/pii.CanRevealFull` gate + `AppendAuditEntryTx` BEFORE returning plaintext) — export must reuse this discipline, not re-implement decryption.
- **Audit trail:** append-only, hash-chained (`internal/audit`), `REVOKE UPDATE/DELETE` from the app role — every export/mark/unmark write goes through `AuditService.AppendAuditEntryTx`.
- **RBAC:** `RequireAnyRole` (OR-guard) — the codebase already fixed the AND-vs-OR bug (bug #3); Phase 5 routes MUST use `RequireAnyRole(Checker, Admin)` for export/keyed-flag routes, and **no route guard at all** for the reports route (all staff).
- **What NOT to use:** no `SEQUENCE`/`SERIAL` for anything gap-less-adjacent (N/A here — the keyed flag is not a sequence); no BLOB storage for files (backup of MinIO must be file-based, not a DB dump); no `pgcrypto` as the PII control (irrelevant here — export reuses existing envelope decryption, doesn't add new encryption).
- **Integration-test gate (CLAUDE.md Conventions):** new endpoints (export, mark/unmark, report query) touch the runtime request seam (HTTP → RequireAuth → RequireAnyRole/ResolveAppUser → handler → service → DB) and therefore **require a real end-to-end integration test with a real Keycloak-shaped token** before the phase can be marked Complete — unit tests calling the service directly are insufficient (Phase 3's three seam bugs are the documented precedent).

---

## Summary

Phase 5 is almost entirely **new orchestration over existing, proven primitives** — there is very little genuinely new infrastructure. The riskiest-looking piece, the "keyed" flag, is **already half-built**: migration `000005_donations.up.sql` added `edonation_keyed BOOLEAN NOT NULL DEFAULT false` in Phase 3 (D-51) specifically so Phase 5 could set it; nothing currently writes `true` to it. The PII-export decrypt path, the audit hash-chain, the config-store pattern (single-row table + admin-editable JSON/text fields), and testcontainers-based integration testing are all established in Phases 1–4 and should be reused verbatim, not reinvented.

The two pieces requiring net-new library/pattern work are: (1) **excelize**, not yet in `go.mod` — confirmed on the official Go module proxy at v2.11.0 (go1.25.0 minimum, compatible with this project's go1.25.1) with a stable, older v2.10.1 fallback; its `File.Write(io.Writer, ...Options) error` method streams the composed workbook directly to an `http.ResponseWriter`, satisfying D-74's "never persist" requirement with zero temp-file code; and (2) **backup/restore**, which has no code precedent in this repo at all — it is a docker-compose companion-service + shell-script + runbook exercise, mirroring the project's existing pattern of building its own minimal Dockerfile (as it already did for the `chrome` sidecar) rather than adopting an unvetted third-party backup image.

**Primary recommendation:** Add `excelize` v2.11.0 to `go.mod`; build one new Go package (`internal/edonation/`) that owns export generation, the keyed-flag mutation, and aging computation (they share the same table, RBAC, and workflow), plus a separate `internal/report/` package (different RBAC — no gate — and no PII, so it is a clean seam to keep isolated); extend `donations` with two nullable columns (`edonation_keyed_at`, `edonation_keyed_by`) rather than a side table; add one new single-row config table (`edonation_config`, sibling to `receipt_template_config`, not an ALTER of it) for field-mapping + aging-threshold config; and implement backup/restore as two companion docker-compose services built from a custom minimal Dockerfile (matching the existing `chrome` sidecar convention) plus a `internal/backupverify` Go integration test that actually restores a real dump into a fresh testcontainers Postgres + MinIO and asserts row/object counts — this is the "recorded evidence" D-73 requires.

---

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| e-Donation export generation (decrypt, xlsx/csv build, stream) | API/Backend (Go) | Database (source query + decrypt inputs) | Decryption keys and the audited-reveal discipline live only in the Go service layer; the file is assembled and streamed entirely server-side, never touching the browser/CDN tier as an intermediate artifact |
| Export confirmation UI + download trigger | Browser/Client | Frontend Server (SSR, BFF proxy) | Next.js BFF forwards the Bearer token server-side (existing pattern) and proxies the binary response through to the browser; no export logic lives client-side |
| Keyed-flag bulk/per-row mutation | API/Backend (Go) | Database (UPDATE + audit write in one tx) | RBAC + audit-write discipline is backend-owned; the aging table's optimistic UI update is a thin reflection of the mutation result |
| Aging bucket computation | API/Backend (Go) | Database (raw `approved_at`/`edonation_keyed` read) | Deadline math must be pure, testable, Bangkok-tz-aware Go (mirrors `receiptno.fiscalYear`'s pattern) — not embedded in SQL, for the same testability reason that package already established |
| Aging threshold + field-mapping config | Database/Storage (config table) | API/Backend (read/write via `internal/settings`-style service) | Same tier split as Phase 4's template config — DB is the source of truth, no-deploy editability (NFR-09) |
| Donation summary report aggregation | Database (SQL `SUM`/`COUNT`/`GROUP BY`) | API/Backend (assemble response DTO) | Aggregation is cheap and correct in SQL; no PII touches this path so no decrypt/mask step is needed at any tier |
| Report/aging/export screens (nav, tables, filters) | Browser/Client | Frontend Server (SSR route gating hint only) | Real RBAC authority is the Go `RequireAnyRole` guard; Next.js route/nav gating is UX-only, per the existing `isAdminViewer()` precedent |
| Backup (pg_dump + MinIO mirror) | Database / Storage (docker-compose companion services) | — | Zero application-tier code; this is infra/ops living beside, not inside, the Go binary — consistent with UI-SPEC's explicit "zero UI footprint" framing |
| Restore verification | Database / Storage (testcontainers-driven Go test) | API/Backend (test harness only, not production code) | The verification harness borrows `internal/testutil`'s testcontainers patterns but produces no production code path — it is proof-of-restorability, run on demand/CI schedule, not a runtime feature |

---

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/xuri/excelize/v2` | **v2.11.0** [VERIFIED: Go module proxy] (go1.25.0+ min; v2.10.1 [VERIFIED: Go module proxy] is a slightly older, more widely-battle-tested fallback requiring go1.24.0+) | Generate `.xlsx` workbooks with correct Thai/UTF-8 cell text | Already named in CLAUDE.md's Supporting Libraries table; the only mature pure-Go Excel writer with full Unicode/CJK/Thai support and an `io.Writer`-based streaming API (no temp files) |
| `encoding/csv` (stdlib) | Go 1.25.1 (project's pinned toolchain) | Generate `.csv` with a manually-written UTF-8 BOM prefix | Zero new dependency; BOM-prefixing is the standard, well-documented trick for Excel to auto-detect UTF-8 CSV instead of guessing a legacy codepage and mangling Thai text |

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/jackc/pgx/v5` (already in go.mod) | v5.10.0 | Query issued donations for export/aging/report | Already the project's sole DB driver — no new dependency |
| `github.com/minio/minio-go/v7` (already in go.mod) | v7.2.1 | N/A for app code this phase — backup/restore instead uses the `mc` **CLI** (a separate binary inside a companion container), not this Go SDK | The Go SDK is for application runtime object access (slip/receipt buckets); backup/restore is an ops/infra concern and correctly uses the officially-supported `mc` CLI, not custom Go code |
| `github.com/testcontainers/testcontainers-go` + `.../modules/postgres` + `.../modules/minio` (already in go.mod) | v0.43.0 | Restore-verification integration test: spin a **fresh, unmigrated** Postgres + a fresh MinIO, `pg_restore`/`mc mirror` a real backup artifact into them, assert data presence | Exact same dependency the project already uses for `testutil.SetupTestPostgres`/`StartMinio` — reused for a new purpose (restore proof) rather than the usual purpose (schema-migrated test fixture) |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| excelize | `github.com/tealeg/xlsx` | Older, smaller community, weaker streaming-write story for large sheets — excelize is the more actively maintained option and is already the CLAUDE.md-prescribed choice |
| Custom-built minimal backup Dockerfile (`postgres:17` + `minio/mc` binary + cron) | `prodrigestivill/postgres-backup-local` (popular community Docker Hub image) | Faster to stand up, but it's an unaffiliated third-party image the project would need to pin by digest and audit; the project's own precedent (`docker/chrome.Dockerfile`) is to build a small custom image from an official base rather than adopt a third-party ops image — recommend the custom approach for consistency and supply-chain minimalism |
| `mc mirror` for MinIO backup | Direct S3 API `ListObjects`+`GetObject` loop via `minio-go` Go code | `mc mirror` is the officially documented, purpose-built tool for exactly this (rsync-like bucket→filesystem sync); writing custom Go code to reimplement it would be "don't hand-roll" territory (see below) |
| Column extension on `donations` for keyed metadata | New `edonation_keyed_events` side table (one row per mark/unmark) | A side table would give a full mark/unmark history without depending on `audit_log` joins, but `audit_log` (append-only, hash-chained) already IS that history (action = `edonation.mark_keyed`/`edonation.unmark_keyed`) — a side table would duplicate data the audit trail already guarantees, for no read benefit the aging query needs |

**Installation:**
```bash
cd donnarec-api
go get github.com/xuri/excelize/v2@v2.11.0
go mod tidy
```

**Version verification:** Confirmed directly against the Go module proxy (the authoritative source for Go modules):
```bash
curl -s https://proxy.golang.org/github.com/xuri/excelize/v2/@v/list | sort -V | tail -5
# v2.9.0 v2.9.1 v2.10.0 v2.10.1 v2.11.0
curl -s https://proxy.golang.org/github.com/xuri/excelize/v2/@v/v2.11.0.mod   # go 1.25.0
curl -s https://proxy.golang.org/github.com/xuri/excelize/v2/@v/v2.10.1.mod  # go 1.24.0
```
Both satisfy the project's `go 1.25.1` toolchain. Note: the proxy's `@latest` endpoint returns v2.10.1 even though v2.11.0 is a higher, more recently tagged semver — this is a known quirk of the Go module proxy's `@latest` resolution and not a sign v2.11.0 is broken/retracted (no `retract` directive found in its `go.mod`). Either version is safe; v2.10.1 has more accumulated community mileage if the planner prefers the conservative pick.

---

## Package Legitimacy Audit

> This phase's only new external dependency is a **Go module** (`excelize`), not an npm/PyPI/crates package — the `package-legitimacy check` seam in this toolchain only covers `npm|pypi|crates` ecosystems and does not support `go` as an ecosystem argument (confirmed: `gsd-tools query package-legitimacy check --ecosystem go excelize` errors with "Usage: ... --ecosystem <npm|pypi|crates>"). Verification was therefore done manually against the authoritative Go module registry (`proxy.golang.org`) plus the module's long public history on GitHub.

| Package | Registry | Age | Downloads/Popularity | Source Repo | Verdict | Disposition |
|---------|----------|-----|----------------------|-------------|---------|-------------|
| `github.com/xuri/excelize/v2` | Go module proxy (`proxy.golang.org`) [VERIFIED: Go module proxy] | First v2.0.0 tag 2020; 20+ tagged releases since; also already named as the project's chosen library in CLAUDE.md's own Supporting Libraries table, sourced with HIGH confidence | Widely used (one of the most-imported Go Excel libraries; referenced across Go ecosystem tooling posts, blog series, and `pkg.go.dev`) | `github.com/qax-os/excelize` (canonical upstream; `github.com/xuri/excelize` is the same maintainer's continuation) — active, long-running | OK | Approved |

**Packages removed due to [SLOP] verdict:** none.
**Packages flagged as suspicious [SUS]:** none.

No other new external packages are introduced this phase. Backup/restore uses officially-distributed CLI binaries (`pg_dump`/`pg_restore` bundled in the official `postgres:17` Docker image; `mc` from the official `minio/mc` Docker image) rather than any new Go/npm dependency — these are Docker base images, not language-package-manager dependencies, and are pinned by tag in the compose/Dockerfile recommendations below.

---

## Architecture Patterns

### System Architecture Diagram

```
┌─────────────────────────────── Browser ───────────────────────────────┐
│  /e-donation (Tabs: Export | Aging)         /reports                  │
│  ExportPanel → ExportConfirmDialog          ReportSummaryCards        │
│  AgingStatCards → AgingTable → BulkActionBar ReportBreakdownTable     │
└───────────────┬─────────────────────────────────────┬─────────────────┘
                │ fetch (same-origin)                  │ fetch
                ▼                                       ▼
┌────────────────────── Next.js BFF (Frontend Server) ───────────────────┐
│  app/api/bff/edonation/export  app/api/bff/edonation/keyed             │
│  app/api/bff/edonation/aging   app/api/bff/reports                     │
│  — obtains Keycloak Bearer token server-side (getServerSession),       │
│    forwards to Go API; binary xlsx/csv response is proxied through     │
│    unmodified (Content-Type/Content-Disposition preserved)             │
└───────────────┬──────────────────────────────────────────────────────┘
                │ Bearer token
                ▼
┌────────────────────────── Go API (donnarec-api) ────────────────────────┐
│  RequireAuth → RequireAnyRole(Checker,Admin) → ResolveAppUser            │
│  (report route: RequireAuth only — no role gate, D-71)                  │
│                                                                          │
│  GET  /api/edonation/export?from=&to=&keyed_status=&format=xlsx|csv     │
│    → edonation.Service.Export                                          │
│        1. SearchIssuedForExport (SQL, status='issued', date+keyed filter)│
│        2. per-row: crypto.DecryptField(donor_tax_id_enc, dek)           │
│        3. ONE audit.AppendAuditEntryTx("edonation.export", count, range)│
│        4. commit tx, THEN build workbook/csv in memory                 │
│        5. stream via excelize File.Write(c.Writer) / csv.Writer(c.Writer)│
│                                                                          │
│  POST /api/edonation/keyed  {donation_ids:[...], keyed:true|false}      │
│    → edonation.Service.SetKeyed (bulk, one tx, N audit rows or 1 batched)│
│                                                                          │
│  GET  /api/edonation/aging                                              │
│    → edonation.Service.Aging                                           │
│        1. SearchUnkeyedIssued (SQL: status='issued', edonation_keyed=false)│
│        2. per-row (Go, pure fn): computeBucket(approved_at, nearDueDays)│
│                                                                          │
│  GET  /api/reports/summary?from=&to=&group_by=month|day                │
│    → report.Service.Summary                                            │
│        SQL: SUM(amount), COUNT(*) GROUP BY date_trunc(...) WHERE        │
│             status='issued' (no PII columns selected — no decrypt)     │
│                                                                          │
│  GET  /api/reports/export?...&format=xlsx|csv  → same excelize/csv helper│
└───────────────┬─────────────────────────────┬──────────────────────────┘
                │                              │
                ▼                              ▼
┌────────────────── PostgreSQL 17 ──────────────┐   ┌── audit_log (hash-chain) ──┐
│ donations (edonation_keyed, edonation_keyed_at,│   │ edonation.export            │
│   edonation_keyed_by — new cols, Pattern 4)    │   │ edonation.mark_keyed        │
│ edonation_config (new single-row table)        │   │ edonation.unmark_keyed      │
└─────────────────────────────────────────────────┘   └──────────────────────────┘

┌──────────────── Backup/Restore (zero app-tier code, D-72/73) ────────────────┐
│ docker-compose companion services (custom minimal image, cron-scheduled):     │
│   db-backup:   pg_dump -Fc → /backups/donnarec_YYYYMMDD.dump  (+ retention)   │
│   minio-backup: mc mirror donnarec/donnarec-slips     /backups/minio/slips   │
│                 mc mirror donnarec/donnarec-receipts  /backups/minio/receipts│
│ Restore proof (on-demand / CI, internal/backupverify):                       │
│   fresh empty postgres testcontainer ← pg_restore(dump) → assert row counts  │
│   fresh empty minio testcontainer    ← mc mirror(backup)→ assert object list │
└────────────────────────────────────────────────────────────────────────────┘
```

### Recommended Project Structure

```
donnarec-api/
├── internal/
│   ├── edonation/                  # NEW — export, keyed-flag, aging (shared RBAC/table)
│   │   ├── model.go                #   ExportFilter, AgingBucket, KeyedRequest DTOs
│   │   ├── service.go              #   Export/SetKeyed/Aging — decrypt+audit reuse
│   │   ├── handler.go              #   Gin handlers (mirrors donation/handler.go Pattern A/C)
│   │   ├── xlsx.go                 #   excelize workbook builder (field-mapping-config driven)
│   │   ├── csv.go                  #   BOM + csv.Writer builder
│   │   ├── aging.go                #   pure computeBucket(approvedAt, now, nearDueDays) — no time.Now() inside, Bangkok-aware (mirrors receiptno/fiscal_year.go)
│   │   └── *_test.go
│   ├── report/                     # NEW — no-PII aggregate report (separate RBAC: none)
│   │   ├── service.go              #   Summary/Breakdown — SQL aggregation only
│   │   ├── handler.go
│   │   └── *_test.go
│   ├── db/queries/
│   │   ├── edonation.sql           # NEW — SearchIssuedForExport, SearchUnkeyedIssued, SetKeyedBulk, GetEdonationConfig, UpdateEdonationConfig
│   │   └── reports.sql             # NEW — SummaryByPeriod (GROUP BY date_trunc)
│   └── (existing: donation, pii, crypto, audit, settings, storage, receiptno, i18n, testutil — all reused unchanged)
├── internal/backupverify/          # NEW — restore-proof integration test (no production code)
│   └── restore_test.go
├── migrations/
│   ├── 000013_edonation_keyed_metadata.up/down.sql   # ALTER donations: + edonation_keyed_at, edonation_keyed_by
│   └── 000014_edonation_config.up/down.sql           # CREATE edonation_config (single-row, sibling to receipt_template_config)
├── docker/
│   └── backup.Dockerfile           # NEW — postgres:17-based (matching pg_dump/server version) + mc binary + cron
├── docker-compose.yml               # + db-backup, minio-backup services
└── docs/
    └── BACKUP_RESTORE_RUNBOOK.md    # NEW — D-73 evidence document
```

### Pattern 1: Stream-only `.xlsx` generation via `excelize.File.Write(io.Writer)`

**What:** Build the workbook entirely in memory (`excelize.NewFile()`, `SetCellValue`/`SetSheetRow`), then call `f.Write(w io.Writer, opts ...Options) error` directly against the HTTP `ResponseWriter` — no temp file is ever created on disk, satisfying D-74.
**When to use:** Any export endpoint that must not persist plaintext-PII files server-side.
**Example:**
```go
// Source: pkg.go.dev/github.com/xuri/excelize/v2 (File.Write signature verified)
func (s *Service) writeXLSX(w http.ResponseWriter, rows []ExportRow, mapping FieldMapping) error {
    f := excelize.NewFile()
    defer f.Close()
    sheet := "e-Donation"
    f.SetSheetName("Sheet1", sheet)

    // Header row from config-driven field mapping (D-75) — never hardcoded column order.
    for col, header := range mapping.HeaderRow() {
        cellRef, _ := excelize.CoordinatesToCellName(col+1, 1)
        f.SetCellValue(sheet, cellRef, header) // excelize handles UTF-8/Thai natively
    }
    for i, row := range rows {
        for col, val := range mapping.RowValues(row) { // includes constant cash-type (D-65)
            cellRef, _ := excelize.CoordinatesToCellName(col+1, i+2)
            f.SetCellValue(sheet, cellRef, val)
        }
    }

    w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
    // RFC 5987 extended parameter for a Thai filename alongside an ASCII fallback:
    w.Header().Set("Content-Disposition",
        `attachment; filename="edonation_export.xlsx"; filename*=UTF-8''`+url.QueryEscape("ส่งออก_e-donation.xlsx"))
    return f.Write(w) // streams directly — no os.Create/TempFile anywhere
}
```
**Note:** `application/vnd.openxmlformats-officedocument.spreadsheetml.sheet` is the standard HTTP `Content-Type` for `.xlsx` (Microsoft/IANA-registered) — do not confuse it with excelize's *internal* `ContentTypeSheetML` constant (`...sheet.main+xml`), which names an OOXML *part* inside the zip, not the HTTP response type.

### Pattern 2: CSV with UTF-8 BOM so Excel reads Thai correctly

**What:** Write the 3-byte BOM (`0xEF 0xBB 0xBF`) directly to the `ResponseWriter` **before** constructing `csv.NewWriter` — this is unrelated to (and unaffected by) the separate `encoding/csv` *Reader*-side BOM-handling bug ([golang/go#33887](https://github.com/golang/go/issues/33887)), which is about parsing, not writing.
**When to use:** Every `.csv` export in this phase (D-62's secondary format, and report CSV export).
**Example:**
```go
// Source: standard Go BOM-prefix pattern for Excel UTF-8 CSV compatibility [CITED: multiple corroborating community references, cross-checked against golang/go#33887 for the reader-side caveat]
func (s *Service) writeCSV(w http.ResponseWriter, rows []ExportRow, mapping FieldMapping) error {
    w.Header().Set("Content-Type", "text/csv; charset=utf-8")
    w.Header().Set("Content-Disposition",
        `attachment; filename="edonation_export.csv"; filename*=UTF-8''`+url.QueryEscape("ส่งออก_e-donation.csv"))

    if _, err := w.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil { // BOM FIRST, before csv.Writer touches w
        return err
    }
    cw := csv.NewWriter(w)
    if err := cw.Write(mapping.HeaderRow()); err != nil {
        return err
    }
    for _, row := range rows {
        if err := cw.Write(mapping.RowValuesAsStrings(row)); err != nil {
            return err
        }
    }
    cw.Flush()
    return cw.Error()
}
```

### Pattern 3: Bulk-decrypt export reuses Phase 3's audited-reveal discipline, not a new decrypt path

**What:** `RevealPII` (03) already establishes the rule "audit BEFORE returning plaintext, in the same transaction, via `AppendAuditEntryTx`." Export must apply the exact same rule to a **batch** of rows: decrypt every row inside one transaction, write **one** summary audit entry (actor, filter range, record count — not per-row, to avoid audit-log flooding for a 500-row export), commit, and only THEN build/stream the file (streaming happens after commit, outside the transaction, since it's a slow I/O step that must never hold a DB tx open).
**When to use:** `edonation.Service.Export`.
**Example:**
```go
// Source: mirrors internal/donation/service.go RevealPII (verified in this codebase, D-13/D-46 pattern)
func (s *Service) Export(ctx context.Context, filter ExportFilter, claims auth.KeycloakClaims) ([]ExportRow, error) {
    if !claims.HasRole(auth.RoleChecker) && !claims.HasRole(auth.RoleAdmin) {
        return nil, ErrForbidden // D-63
    }
    var rows []ExportRow
    err := dbhelpers.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
        qtx := s.queries.WithTx(tx)
        dbRows, err := qtx.SearchIssuedForExport(ctx, filter.ToParams()) // status='issued' only, D-66
        if err != nil { return err }

        for _, r := range dbRows {
            plaintext, err := crypto.DecryptField(ctx, s.keyProvider, r.DonorTaxIDEnc, r.DonorTaxIDDek)
            if err != nil { return fmt.Errorf("decrypt row %s: %w", r.ID, err) }
            rows = append(rows, ExportRow{ /* ... */ NationalID: string(plaintext) })
        }

        afterJSON, _ := json.Marshal(map[string]any{
            "action": "edonation.export", "count": len(rows),
            "from": filter.From, "to": filter.To, "keyed_status": filter.KeyedStatus,
        })
        return s.auditSvc.AppendAuditEntryTx(ctx, tx, audit.AuditEntry{
            ActorID: claims.Subject, ActorEmail: claims.ActorIdentity(),
            Action: "edonation.export", Resource: "/api/edonation/export", AfterJSON: afterJSON,
        }) // D-64: audit BEFORE returning plaintext to caller — tx rollback if this fails
    })
    return rows, err // caller streams AFTER this returns successfully (Pattern 1/2, outside any tx)
}
```

### Pattern 4: Extend the EXISTING `edonation_keyed` column — do not create a new flag or side table

**What:** Migration `000005_donations.up.sql` (Phase 3, D-51) already added `edonation_keyed BOOLEAN NOT NULL DEFAULT false` to `donations`, specifically documented as "ตั้ง true ตอน export ใน Phase 5". **No query currently sets it to `true`** — `LockDonationForUpdate` and `SearchDonations` only read it (used today solely to gate the cancel-confirmation-reason requirement, D-51). Phase 5's job is to add the write path, not invent new storage.
**Recommendation:** Add two nullable columns for actor/timestamp (needed for the "who/when marked" question without forcing every aging-page read to join `audit_log`), via a new migration:
```sql
-- migrations/000013_edonation_keyed_metadata.up.sql
ALTER TABLE donations
    ADD COLUMN edonation_keyed_at TIMESTAMPTZ,
    ADD COLUMN edonation_keyed_by UUID REFERENCES users(id);
-- edonation_keyed itself is untouched — already exists (000005, D-51).
-- No GRANT needed — UPDATE on donations already granted to donnarec_app (000005).
```
**Why not a side table:** A side table (`edonation_keyed_events`) would duplicate what `audit_log` already provides (append-only, hash-chained history of every mark/unmark, actor, timestamp) for zero read-path benefit — the aging query only needs the CURRENT flag state (`WHERE edonation_keyed = false`), which the existing column already serves. Two extra columns are simpler than a new table + new FK + new join, and every historical mark/unmark event is still fully recoverable from `audit_log` if ever needed.
**Bulk mutation:** One `UPDATE ... WHERE id = ANY($1::uuid[])` statement, wrapped in a transaction with ONE audit entry per record (not one batched entry) — because each donation is a materially distinct thing being flagged (D-67 "ทุกการติ๊ก/เอาออก audit"), unlike export's single-summary-entry rationale in Pattern 3 (export's audit subject is "the export event", not "each individual row disclosed").

### Pattern 5: Aging bucket computation — pure Go function, Bangkok-aware, mirrors `receiptno.fiscalYear`

**What:** Deadline = 5th of the month **after** `approved_at`'s month, computed in **Asia/Bangkok** local time (not UTC) — same timezone-normalization discipline `receiptno/fiscal_year.go` already established (load `Asia/Bangkok` once via `sync.Once`, normalize the input `time.Time` via `.In(loc)`, never call `time.Now()` inside the pure function — the caller passes "now" explicitly for testability).
**When to use:** `edonation.aging.go`.
**Example:**
```go
// Source: pattern mirrors internal/receiptno/fiscal_year.go (verified in this codebase)
type AgingBucket string

const (
    BucketNotDue  AgingBucket = "not_due"
    BucketNearDue AgingBucket = "near_due"
    BucketOverdue AgingBucket = "overdue"
)

// computeDeadline returns the 5th of the month after approvedAt's month, in Asia/Bangkok.
// approvedAt and the returned time are both normalized to Asia/Bangkok before any date math.
func computeDeadline(approvedAt time.Time) time.Time {
    loc := bangkok() // sync.Once-loaded, panics on missing tzdata — same guard as receiptno
    t := approvedAt.In(loc)
    firstOfNextMonth := time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, loc)
    return firstOfNextMonth.AddDate(0, 0, 4) // day 1 + 4 = day 5
}

// computeBucket NEVER calls time.Now() — caller passes `now` explicitly (testability,
// same discipline as receiptno.fiscalYear never calling time.Now()).
func computeBucket(approvedAt, now time.Time, nearDueDays int) AgingBucket {
    deadline := computeDeadline(approvedAt)
    daysRemaining := int(deadline.Sub(now.In(deadline.Location())).Hours() / 24)
    switch {
    case daysRemaining < 0:
        return BucketOverdue
    case daysRemaining <= nearDueDays: // config-adjustable, default 3 (D-68)
        return BucketNearDue
    default:
        return BucketNotDue
    }
}
```
**Pitfall avoided:** `time.Date(t.Year(), t.Month()+1, ...)` correctly rolls December → January of the next year — Go's `time.Date` normalizes overflowing month values, so no explicit December special-case is needed (verified Go stdlib behavior).
**Note on package boundary:** `receiptno`'s own `loadBangkok`/`bangkokLoc` are unexported — `internal/edonation` cannot import them directly. Duplicate the same `sync.Once` + `LoadLocation("Asia/Bangkok")` + panic-on-missing-tzdata guard locally in `edonation/aging.go` (a few lines, matching the existing pitfall-5 doc-comment about needing `import _ "time/tzdata"` in `main.go` or the `tzdata` OS package in the container — already satisfied since `receiptno` requires the same thing today).

### Pattern 6: e-Donation field mapping as a new sibling config table (not an ALTER of `receipt_template_config`)

**What:** Mirror migration `000011`'s own documented rationale ("deliberately a SIBLING table, not an ALTER of receipt_number_config") — create `edonation_config` as its own single-row table, following the identical `id BOOLEAN PRIMARY KEY DEFAULT true` + `CHECK (id = true)` shape already used twice (`receipt_number_config` in 000004, `receipt_template_config` in 000011).
**Example:**
```sql
-- migrations/000014_edonation_config.up.sql
CREATE TABLE edonation_config (
    id                   BOOLEAN PRIMARY KEY DEFAULT true,
    CONSTRAINT           single_row CHECK (id = true),
    -- Field mapping (D-75) — stored as JSONB so column order/names can be edited
    -- without a deploy once the real RD e-Donation spec is confirmed (stakeholder gate).
    field_mapping        JSONB NOT NULL DEFAULT '[]'::jsonb,
    cash_type_label      TEXT NOT NULL DEFAULT 'เงินสด/โอน', -- D-65 constant, still editable
    -- Aging threshold (D-68) — "near due" window, days before deadline.
    near_due_days        INTEGER NOT NULL DEFAULT 3 CHECK (near_due_days >= 0),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by           UUID REFERENCES users(id)
);
INSERT INTO edonation_config DEFAULT VALUES ON CONFLICT (id) DO NOTHING;
GRANT SELECT, INSERT, UPDATE ON edonation_config TO donnarec_app;
```
This is read by a small service function (parallel shape to `settings.SettingsService.GetSettings`/`SaveSettings`) and, per the UI-SPEC's own note, can be exposed as a 5th tab on the existing `SettingsTabs` component — no new Admin Settings screen is needed.

### Anti-Patterns to Avoid

- **Computing "next receipt-style sequence number" logic for the keyed flag:** there is no sequence here — it's a plain boolean; do not add any allocator/locking machinery. `edonation_keyed` is a simple `UPDATE`, not a gap-less counter.
- **Doing aging-bucket date math in raw SQL with naive `date_trunc`:** Postgres CAN do `date_trunc('month', approved_at AT TIME ZONE 'Asia/Bangkok') + interval '1 month 4 days'`, but this project's own precedent (`receiptno.fiscalYear`) deliberately keeps timezone-sensitive date logic in **testable Go**, not SQL, specifically so unit tests can pin exact instants without spinning up Postgres — follow that precedent, don't reintroduce SQL-side timezone math this codebase avoided before.
- **Writing the export file to a temp path "just to be safe" before streaming:** directly contradicts D-74. `excelize.File.Write(io.Writer)` and `csv.NewWriter(io.Writer)` both support writing straight to the `http.ResponseWriter` — there is no reason to touch `os.TempFile` anywhere in this phase.
- **One audit row per exported record:** would flood `audit_log` with (potentially hundreds of) rows for a single export click and defeats the "who/when/what range/how many" summary D-64 actually asks for — one audit row per export **event** is correct (contrast with the keyed-flag bulk mutation, which correctly DOES want one row per donation, since each is a distinct state change being recorded).

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| MinIO bucket → local filesystem backup sync | A custom Go loop calling `ListObjects`+`GetObject` per key | `mc mirror ALIAS/BUCKET /backup/path` (official MinIO CLI) | `mc mirror` already handles incremental sync, retries, and is the officially documented backup mechanism [CITED: docs.min.io/community/minio-object-store/reference/minio-mc/mc-mirror.html] — reimplementing it in Go adds a maintenance burden with no benefit |
| Postgres full-database dump/restore | Custom `pg_dump`-equivalent using `pgx` to `SELECT *` every table and re-`INSERT` | `pg_dump -Fc` / `pg_restore` (bundled with the official `postgres:17` image) | These are the Postgres project's own dump/restore tools, transaction-consistent (`pg_dump` takes a consistent snapshot) and handle sequences/constraints/ordering correctly — a hand-rolled table-by-table exporter would silently break on FK ordering or sequence state |
| Excel `.xlsx` binary format (zip + OOXML XML parts) | Any custom XML/zip writer | `excelize` | `.xlsx` is a non-trivial OOXML zip container; excelize already handles this correctly with Unicode support, per CLAUDE.md's own explicit guidance |
| UTF-8 BOM detection/handling for CSV | A custom encoding-sniffing layer | The 3-byte literal `0xEF 0xBB 0xBF` prefix, written once before `csv.NewWriter` | This is a fixed, universally-recognized 3-byte sequence — there is nothing to "detect," only to prepend |

**Key insight:** Every genuinely hard problem in this phase (decrypt-with-audit, hash-chained audit, RBAC guard, config-store shape, Bangkok-timezone date math, testcontainers fixtures) already has a proven, working implementation in this exact codebase from Phases 1–4. The only pieces with zero precedent are excelize (a well-documented external library) and backup/restore (an ops concern solved by two decades-old, official CLI tools). There is no part of this phase that legitimately needs new cryptography, new concurrency primitives, or a new sequence/locking scheme.

---

## Runtime State Inventory

> This phase is net-new-capability, not a rename/refactor/migration of existing behavior — this section is included only for completeness regarding the ONE piece of "runtime state that already exists but nothing writes to it yet."

| Category | Items Found | Action Required |
|----------|-------------|------------------|
| Stored data | `donations.edonation_keyed` (BOOLEAN, default `false`) exists in every donation row since Phase 3 (migration 000005) but is currently always `false` in production/dev data — no historical data to migrate, only a new write path to add | Code edit only (add `UPDATE`/mutation path) — no backfill needed since the column's default (`false`) is already the correct "not yet keyed" state for every existing record |
| Live service config | None — Keycloak realm roles (Checker/Admin) already exist from Phase 1/3; no new external service config is introduced | None |
| OS-registered state | None — no task-scheduler/pm2/systemd entries are created by this phase (the backup cron lives INSIDE a docker-compose container's own crond, not on the host OS) | None |
| Secrets/env vars | New: MinIO backup destination path / retention-days env var (e.g. `BACKUP_RETENTION_DAYS`) if the compose companion services are made configurable — not yet defined anywhere | Code edit only — define with a sane default (see Pattern 8), document in `.env.example` |
| Build artifacts | None — `go.mod`/`go.sum` will gain the excelize entry; no stale installed-package artifacts exist yet to clean up | `go mod tidy` after `go get` |

---

## Common Pitfalls

### Pitfall 1: `time.Date` month-overflow is safe, but December handling is easy to get wrong by hand
**What goes wrong:** A naive implementation might special-case `if month == 12 { year++; month = 1 } else { month++ }` and introduce an off-by-one bug.
**Why it happens:** Developers unfamiliar with Go's `time.Date` normalization write manual month-rollover logic instead of trusting the stdlib.
**How to avoid:** `time.Date(year, month+1, day, ...)` is safe for ANY month value including 13 — Go's `time` package documents that "the month, day, hour, min, sec, and nsec values may be outside their usual ranges and will be normalized during the conversion." Rely on this; do not hand-write December rollover.
**Warning signs:** A unit test asserting `computeDeadline(approvedAt in December) == January 5th of next year` should be added explicitly (mirrors `receiptno/fiscalyear_test.go`'s existing Oct/Dec boundary tests).

### Pitfall 2: Content-Disposition filename with Thai characters breaks naive quoting
**What goes wrong:** Putting raw Thai UTF-8 bytes directly inside `filename="..."` produces a header some browsers/HTTP clients mis-decode, or that fails strict HTTP header validation entirely (RFC 7230 header field values are constrained to a visible-ASCII-ish subset).
**Why it happens:** `filename="ใบเสร็จ.xlsx"` is not valid per RFC 6266/2616 grammar for the basic `filename` parameter, which expects ASCII (or specific fallback encodings) — non-ASCII must go through the `filename*=UTF-8''<percent-encoded>` extended parameter (RFC 5987/8187).
**How to avoid:** Always send BOTH: an ASCII-safe `filename="edonation_export.xlsx"` fallback AND a `filename*=UTF-8''...` percent-encoded Thai name — exactly as shown in Pattern 1/2's code. Every modern browser prefers `filename*` when present; older/non-browser HTTP clients fall back to the ASCII `filename`.
**Warning signs:** Downloaded files silently save as garbled or blank filenames in some browsers/download managers if only the raw-UTF-8 `filename=` form is used.

### Pitfall 3: Holding a DB transaction open across a slow excelize workbook build / large decrypt loop
**What goes wrong:** If decrypt + audit + workbook-building all happen inside one `WithTx` closure, a large export (hundreds of records, each requiring an AES-GCM decrypt) holds the transaction — and any row locks it implicitly takes — open far longer than necessary, risking lock contention with concurrent approvals.
**Why it happens:** It's tempting to do "everything in one transaction" for simplicity.
**How to avoid:** Per Pattern 3, keep the transaction scoped to: query + decrypt (needed to know exact plaintext values and exact row count for the audit summary) + the ONE audit write. **Commit**, THEN build/stream the file entirely outside any transaction. The decrypt step itself does not need row-level locks (it's reading, not locking, `LockDonationForUpdate` is a different, approval-specific pattern) — `SearchIssuedForExport` should be a plain (non-`FOR UPDATE`) `SELECT`.
**Warning signs:** Export latency scaling with file size while simultaneously slowing down unrelated approval transactions (lock contention).

### Pitfall 4: `pg_dump` without `-Fc` (custom format) makes selective/parallel restore impossible
**What goes wrong:** A plain-SQL dump (`-Fp`, the default) can only be restored serially via `psql < dump.sql`, has no built-in integrity/TOC, and cannot do parallel restore (`pg_restore -j N`) or selective table restore — all of which matter once the DB grows.
**Why it happens:** `-Fp` is `pg_dump`'s default if no `-F` flag is given, so a naive `pg_dump -f backup.sql dbname` command silently produces the weaker format.
**How to avoid:** Always pass `-Fc` (custom format) explicitly: `pg_dump -Fc -f /backups/donnarec_$(date +%Y%m%d).dump donnarec_app`. Restore with `pg_restore --no-owner --role=donnarec_app -d donnarec_app /backups/donnarec_YYYYMMDD.dump` [CITED: postgresql.org/docs/current/app-pgdump.html].
**Warning signs:** `pg_restore: error: input file does not appear to be a valid archive` when trying to `pg_restore` a plain-SQL dump — this is the signal the wrong format flag was used upstream.

### Pitfall 5: `time.LoadLocation("Asia/Bangkok")` fails if the container is missing tzdata — same known pitfall as `receiptno`
**What goes wrong:** If `internal/edonation`'s own Bangkok-location loader is added without checking the existing `main.go`/container tzdata setup, it is easy to assume it "just works" because `receiptno` already works — but if this new package is ever used in a build/test context that DOESN'T import the same binary (e.g. a standalone CLI tool), the panic resurfaces.
**Why it happens:** `time.LoadLocation` depends on IANA tzdata being present either via the OS package or Go's `time/tzdata` embed.
**How to avoid:** Confirm `main.go` already has `import _ "time/tzdata"` (or the Dockerfile installs the `tzdata` OS package) — this was already solved for `receiptno` in Phase 2; Phase 5 only needs to confirm it's still true, not re-solve it.
**Warning signs:** A panic with message "Asia/Bangkok timezone not available" surfacing specifically in a NEW package/binary that Phase 2 didn't need to touch (e.g., if the restore-verification test runs in a separate `go test` binary that somehow doesn't share `main.go`'s import — in practice `internal/edonation`'s own tests link against the same module, so this should not actually occur, but is worth an explicit assertion in a smoke test).

---

## Code Examples

### Gin route wiring (mirrors existing `donationGroup`/`checkerGroup` shape in `cmd/server/main.go`)
```go
// Source: mirrors cmd/server/main.go's existing donationGroup/adminGroup wiring pattern
edonationGroup := api.Group("/edonation")
edonationGroup.Use(auth.RequireAnyRole(auth.RoleChecker, auth.RoleAdmin)) // D-63/D-69
edonationGroup.Use(auth.ResolveAppUser(appUserResolver, logger))          // for keyed_by/audit actor resolution
edonationGroup.GET("/export", edonationHandler.Export)
edonationGroup.POST("/keyed", edonationHandler.SetKeyed)
edonationGroup.GET("/aging", edonationHandler.Aging)

reportGroup := api.Group("/reports") // D-71: no RequireAnyRole — all authenticated staff
reportGroup.GET("/summary", reportHandler.Summary)
reportGroup.GET("/export", reportHandler.Export)
```

### Report aggregation query (no PII columns selected — no decrypt/mask step needed anywhere on this path)
```sql
-- internal/db/queries/reports.sql
-- name: SummaryByMonth :many
-- Aggregates issued donations by calendar month (donated_at is a DATE column,
-- no timezone conversion needed — see donations.donated_at column type, migration 000005).
-- Excludes non-issued statuses — cancelled/draft/rejected are not "donations received" (D-70 assumption, see Open Questions).
SELECT
    date_trunc('month', donated_at)::date AS period,
    COUNT(*)          AS receipt_count,
    SUM(amount)        AS total_amount
FROM donations
WHERE status = 'issued'
  AND (sqlc.narg('from_date')::DATE IS NULL OR donated_at >= sqlc.narg('from_date'))
  AND (sqlc.narg('to_date')::DATE   IS NULL OR donated_at <= sqlc.narg('to_date'))
GROUP BY period
ORDER BY period;

-- name: SummaryByDay :many
-- Same shape, daily granularity — donated_at is already a DATE so no truncation is needed.
SELECT
    donated_at        AS period,
    COUNT(*)          AS receipt_count,
    SUM(amount)        AS total_amount
FROM donations
WHERE status = 'issued'
  AND (sqlc.narg('from_date')::DATE IS NULL OR donated_at >= sqlc.narg('from_date'))
  AND (sqlc.narg('to_date')::DATE   IS NULL OR donated_at <= sqlc.narg('to_date'))
GROUP BY donated_at
ORDER BY donated_at;
```

### Aging + export source query (both need `approved_at`, `edonation_keyed`, and the encrypted PII columns; export additionally needs the ciphertext)
```sql
-- internal/db/queries/edonation.sql
-- name: SearchUnkeyedIssued :many
-- Aging view source (FR-31/D-68): all issued, not-yet-keyed donations with their
-- approval timestamp (aging base date per D-68) — bucket computed in Go (Pattern 5).
SELECT id, donor_name, receipt_formatted, approved_at, edonation_keyed
FROM donations
WHERE status = 'issued' AND edonation_keyed = false
ORDER BY approved_at ASC;

-- name: SearchIssuedForExport :many
-- Export source (FR-30/D-66): issued only, date-range + keyed-status filter.
-- Ciphertext columns are decrypted at the SERVICE layer (Pattern 3) — never in SQL.
SELECT id, donor_name, donor_tax_id_enc, donor_tax_id_dek, donated_at, receipt_formatted, edonation_keyed
FROM donations
WHERE status = 'issued'
  AND (sqlc.narg('from_date')::DATE IS NULL OR donated_at >= sqlc.narg('from_date'))
  AND (sqlc.narg('to_date')::DATE   IS NULL OR donated_at <= sqlc.narg('to_date'))
  AND (sqlc.narg('keyed_status')::BOOLEAN IS NULL OR edonation_keyed = sqlc.narg('keyed_status'))
ORDER BY donated_at ASC;

-- name: SetKeyedBulk :exec
-- Bulk mark/unmark (D-67) — one statement covers the whole selection; caller writes
-- one audit row PER donation_id afterward (Pattern 4 — distinct from export's single summary row).
UPDATE donations
SET edonation_keyed = @keyed, edonation_keyed_at = @keyed_at, edonation_keyed_by = @keyed_by
WHERE id = ANY(@donation_ids::uuid[]) AND status = 'issued';
```

---

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|---------------|--------|
| N/A — this is a greenfield capability within the project | N/A | N/A | This phase does not replace or deprecate any prior Phase 1–4 mechanism; it is additive |

**Deprecated/outdated:** None applicable — no prior e-Donation export/report/backup mechanism exists in this codebase to deprecate.

---

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Report period basis (FR-32 "ช่วงเวลา") uses `donated_at` (money-received date), matching the existing `SearchDonations`/`DonationFilterBar` date-range convention from Phase 3 — NOT `approved_at` (issue date), which the export screen explicitly uses instead (UI-SPEC labels Screen 7's filter "วันที่ออกใบเสร็จ" but Screen 8's filter only "ช่วงเวลา", ambiguous) | Code Examples (`SummaryByMonth`/`SummaryByDay`), Pattern 7 | If accounting actually wants the report period keyed to issue date (consistency with the aging/export screens' date basis), the SQL WHERE clause and GROUP BY column need to switch from `donated_at` to `approved_at` (which is a TIMESTAMPTZ requiring `AT TIME ZONE 'Asia/Bangkok'` truncation, unlike `donated_at`'s plain DATE) — low implementation cost to fix, but should be confirmed with the planner/stakeholder before building |
| A2 | Report aggregation excludes `cancelled` donations (only `status='issued'` is summed), by analogy with D-66's explicit exclusion of cancelled from export — CONTEXT.md's D-70 does not explicitly state this for reports | Code Examples (`SummaryByMonth`) | If a cancelled donation's original amount SHOULD still count toward a period's gross total (e.g., for reconciliation purposes distinct from "net standing donations"), the query needs an additional bucket/column for cancelled totals — currently they are silently excluded entirely |
| A3 | The keyed-flag mutation writes ONE audit row per donation ID in a bulk mark/unmark request (not one row for the whole batch) — inferred from D-67's "ทุกการติ๊ก/เอาออก audit" (every tick/untick is audited) reading as "every individual record's state change," but could alternatively be read as "every mark/unmark UI action" | Pattern 4 | If a single audit row per batch action is actually intended (matching export's single-summary-row precedent in Pattern 3), a 50-row bulk mark would currently produce 50 audit rows instead of 1 — this is a straightforward code-level choice to reverse if the planner disagrees, but affects `audit_log` volume/query patterns |
| A4 | `excelize` v2.11.0 (rather than the `@latest`-endpoint-reported v2.10.1) is safe to adopt despite the Go module proxy's `@latest` quirk — based on no `retract` directive found in its `go.mod` and a normal-looking, recently-dated tag | Standard Stack | If v2.11.0 has an undiscovered regression the proxy's `@latest` heuristic is actually protecting against, `go.mod` should pin v2.10.1 instead — trivial one-line change either way |

**If this table is empty:** N/A — see rows above; all other claims in this document are either [VERIFIED] against the Go module proxy/codebase or [CITED] to official excelize/PostgreSQL/MinIO documentation.

---

## Open Questions

1. **Does the Reports screen's date-range filter key off `donated_at` or `approved_at`?**
   - What we know: Screen 7 (Export) explicitly labels its filter "วันที่ออกใบเสร็จ" (issued date) per the UI-SPEC; Screen 8 (Reports) only says "ช่วงเวลา" (period) with no explicit date-basis label, and the UI-SPEC says it "reuses the Calendar Popover date-range pattern from Phase 3" — which in Phase 3 filtered on `donated_at`.
   - What's unclear: Whether accounting wants the summary report to reflect "money donated during X" (`donated_at`) or "receipts issued during X" (`approved_at`) — these can differ by days when approval lags donation.
   - Recommendation: Default to `donated_at` (Assumption A1) since it matches the established filter convention and requires no additional timezone-conversion SQL (it's a plain `DATE` column); flag this explicitly for the planner/discuss-phase to confirm with the user before locking the query.

2. **Should the 5th-of-month deadline account for weekends/holidays (i.e., if the 5th falls on a Saturday, does the effective deadline move to the next business day)?**
   - What we know: D-68 states the deadline plainly as "5th of the month after issue month" with no mention of business-day adjustment; the e-Donation manual-keying deadline in the real Revenue Department process may or may not have such an adjustment (this is part of the still-open "exact e-Donation field spec" stakeholder gate noted in REQUIREMENTS.md).
   - What's unclear: Real-world RD deadline rules are outside this session's ability to verify without the stakeholder confirmation already flagged in REQUIREMENTS.md's "Stakeholder Confirmations Required" table.
   - Recommendation: Implement the literal calendar-5th rule now (matches CONTEXT.md exactly); document it as adjustable if/when the RD spec confirmation lands — the `near_due_days` config column (Pattern 6) is the natural extension point if a business-day rule is added later.

---

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | All backend code | ✓ | go1.25.1 (matches `go.mod`'s `go 1.25.1`) | — |
| Docker + Docker Compose | testcontainers (all integration tests), backup companion services | ✓ | Docker 28.1.1, Compose v2.39.2 | — |
| `pg_dump`/`pg_restore` (host) | Manual/ad-hoc backup invocation outside Docker | ✗ (not installed on this dev host) | — | Not needed as a host dependency — the recommended design runs `pg_dump`/`pg_restore` INSIDE the `postgres:17` container image (which bundles a matching-version client), never on the host |
| `mc` (MinIO client, host) | Manual/ad-hoc MinIO mirror invocation outside Docker | ✗ (not installed on this dev host) | — | Same as above — runs inside the official `minio/mc` container image, not the host |
| `github.com/xuri/excelize/v2` | Export/report `.xlsx` generation | ✗ (not yet in `go.mod`) | Add v2.11.0 (or v2.10.1) | None needed — `go get` resolves this at build time |

**Missing dependencies with no fallback:** none — every missing item above has a documented in-container fallback that requires no host tooling.
**Missing dependencies with fallback:** `pg_dump`/`pg_restore`/`mc` on the host (fallback: run inside the already-available Docker containers, which is also the recommended production design, not merely a workaround).

---

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` + `testify` (require/assert) + `testcontainers-go` (already in `go.mod`) |
| Config file | none — Go tests are discovered by file naming (`*_test.go`), no separate config |
| Quick run command | `go test -short -count=1 ./internal/edonation/... ./internal/report/...` (unit-level: aging bucket math, field-mapping/CSV row building — no Docker needed) |
| Full suite command | `go test -count=1 ./...` (requires Docker for testcontainers — matches the existing `make test` target) |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|--------------------|--------------|
| FR-30 | Export returns only `issued` donations within date range, excludes `cancelled`, full 13-digit ID decrypted correctly | integration (real Postgres via testcontainers, real crypto keyprovider) | `go test -count=1 -run TestExport ./internal/edonation/...` | ❌ Wave 0 — new package |
| FR-30 | Export writes exactly ONE audit row per export event with correct actor/range/count (D-64) | integration | `go test -count=1 -run TestExport_Audit ./internal/edonation/...` | ❌ Wave 0 |
| FR-30 | Export route requires Checker or Admin — Maker gets 403 (D-63) — **must be a real HTTP-path E2E test with a real signed Keycloak-shaped token**, per CLAUDE.md's Integration-test gate (Phase 3's precedent: unit-level RBAC checks structurally cannot catch route-wiring bugs) | E2E (real router + real token, mirrors `cmd/server/e2e_test.go`) | `go test -count=1 -run TestE2E ./cmd/server/...` | ❌ Wave 0 — extend existing `e2e_test.go` |
| FR-31 | Bulk mark/unmark updates all selected rows atomically; per-row toggle updates exactly one row; every mutation is audited | integration | `go test -count=1 -run TestSetKeyed ./internal/edonation/...` | ❌ Wave 0 |
| FR-31 | Aging bucket boundaries correct at exact threshold instants (e.g., exactly 3 days before deadline = near_due, 3 days + 1 second before = not_due) | unit (pure function, no DB, mirrors `receiptno/fiscalyear_test.go`'s boundary-instant style) | `go test -short -run TestComputeBucket ./internal/edonation/...` | ❌ Wave 0 |
| FR-31 | December issue month rolls deadline to January correctly (Pitfall 1) | unit | `go test -short -run TestComputeDeadline_DecemberRollover ./internal/edonation/...` | ❌ Wave 0 |
| FR-32 | Monthly/daily breakdown sums match a known fixture set exactly; cancelled donations excluded from totals (Assumption A2) | integration | `go test -count=1 -run TestReportSummary ./internal/report/...` | ❌ Wave 0 |
| FR-32 | Report route accessible to Maker (no RBAC 403) — real HTTP path | E2E | `go test -count=1 -run TestE2E ./cmd/server/...` | ❌ Wave 0 — extend existing `e2e_test.go` |
| NFR-08 | A real `pg_dump` artifact restores into a FRESH, unmigrated testcontainers Postgres and produces the exact expected row counts for a known fixture | integration (restore-proof, Pattern 9) | `go test -count=1 -run TestRestoreProof ./internal/backupverify/...` | ❌ Wave 0 — new package, this IS the "recorded evidence" D-73 requires |
| NFR-08 | `mc mirror` restores a known set of MinIO objects (slip + frozen-PDF buckets) into a fresh testcontainers MinIO and every expected object key is present with matching content | integration | `go test -count=1 -run TestRestoreProof_MinIO ./internal/backupverify/...` | ❌ Wave 0 |

### Sampling Rate
- **Per task commit:** `go test -short -count=1 ./internal/edonation/... ./internal/report/...`
- **Per wave merge:** `go test -count=1 ./...` (full suite, Docker required)
- **Phase gate:** Full suite green + the real-HTTP-path E2E test (extending `cmd/server/e2e_test.go`) green + a human UI walkthrough of Screens 7/8, before `/gsd-verify-work`

### Wave 0 Gaps
- [ ] `internal/edonation/` package does not exist yet — needs `service.go`, `handler.go`, `xlsx.go`, `csv.go`, `aging.go` + corresponding `_test.go` files
- [ ] `internal/report/` package does not exist yet
- [ ] `internal/backupverify/` package does not exist yet — this is the single most important NEW test file for satisfying D-73's "recorded evidence" requirement; it should produce durable output (a log file or test artifact checked into the phase's evidence, per D-73) proving a real restore succeeded, not just a green CI checkmark
- [ ] `cmd/server/e2e_test.go` needs new subtests for the export/keyed/aging/report routes (extends the existing E2E test file rather than creating a new one — matches Phase 3's established single-E2E-file convention)
- [ ] Framework install: none — `testify`/`testcontainers-go` already in `go.mod`; only `excelize` needs `go get`

---

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|----------------|---------|-------------------|
| V2 Authentication | Indirect (Keycloak OIDC, already Phase 1) | No new work — reuses `auth.RequireAuth()` |
| V3 Session Management | No | N/A — session handling unchanged |
| V4 Access Control | **Yes** | `auth.RequireAnyRole(RoleChecker, RoleAdmin)` for export/keyed-flag routes; deliberately NO route guard on `/reports` (D-71, by design — not an omission) |
| V5 Input Validation | Yes | Date-range params validated (from ≤ to, valid dates); `donation_ids` array in bulk-keyed request validated as well-formed UUIDs before the `ANY($1::uuid[])` query runs; `keyed_status`/`format` query params validated against an allowlist (`xlsx`/`csv` only) |
| V6 Cryptography | Yes (reuse only — no new crypto) | Export reuses the EXISTING `internal/crypto` AES-256-GCM envelope decrypt (`crypto.DecryptField`) — this phase must NOT introduce any new encryption scheme, key material, or KMS interaction |
| V7 Error Handling & Logging | Yes | Export/keyed/aging/report handlers must follow the codebase's Pattern C (no PII in logs — log `donation_id`/count only, never the plaintext national ID, matching `RevealPII`'s existing logging discipline exactly) |
| V9 Communications | No new surface | HTTPS/TLS already enforced at the ingress level (Phase 1, NFR-02) — export responses carry no new transport requirement beyond what already applies to every API response |
| V12 Files and Resources | Yes | The export file is a **response body**, never a server-side-persisted file (D-74) — there is no new file-upload surface in this phase, so upload-side ASVS controls (already covered by `internal/storage`'s magic-byte validation for slips/template images) are unaffected |

### Known Threat Patterns for this stack

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|----------------------|
| Bulk PII disclosure via export with no rate limit (a compromised Checker/Admin account could exfiltrate the entire donor database in one request) | Information Disclosure | D-64's mandatory audit-per-export gives after-the-fact detectability (who/when/range/count); consider (planner discretion, not CONTEXT-locked) capping the maximum date range or record count per single export request as defense-in-depth, though CONTEXT.md does not explicitly require a hard cap |
| SQL injection via unvalidated `from`/`to`/`keyed_status`/`donation_ids` query params | Tampering | `sqlc`-generated parameterized queries (already the project-wide convention, `sqlc.narg(...)` pattern) — no string concatenation anywhere in the new `edonation.sql`/`reports.sql` files, mirroring `donations.sql`'s existing discipline |
| Left-over plaintext-PII file if a client-side crash/interrupt occurs mid-download | Information Disclosure | D-74's stream-only design means the SERVER never writes the file to disk in the first place — a client-side interruption only affects the browser's own partial-download temp file (outside this system's control, and outside this phase's scope) |
| IDOR on the bulk-keyed endpoint (crafting arbitrary `donation_ids` not matching the caller's actual filter view) | Tampering / Elevation of Privilege | The `SetKeyedBulk` query's `WHERE ... AND status = 'issued'` clause is itself a scope guard (only issued donations can ever be marked/unmarked, regardless of which IDs are submitted) — combined with the RBAC gate (Checker/Admin only), this bounds the blast radius to "any issued donation," which is the same access level the RBAC role already implies via the export/reveal endpoints |
| Report aggregation timing/enumeration leak (an unauthenticated-adjacent low-trust user inferring donor activity patterns from aggregate totals) | Information Disclosure | D-71 deliberately accepts this as a NON-issue — the report contains zero PII and is explicitly meant to be transparent to all staff; this is a documented, intentional design choice, not a gap |

---

## Sources

### Primary (HIGH confidence)
- [VERIFIED: Go module proxy] `proxy.golang.org/github.com/xuri/excelize/v2/@v/list` and `@v/v2.11.0.mod` / `@v/v2.10.1.mod` — version list and minimum-Go-version requirements
- [VERIFIED: codebase read] `donnarec-api/migrations/000005_donations.up.sql` — confirms `edonation_keyed` column already exists (D-51)
- [VERIFIED: codebase read] `donnarec-api/internal/donation/service.go` (`RevealPII`, `Resend`, `DownloadReceipt`) — audited-decrypt and audit-write patterns
- [VERIFIED: codebase read] `donnarec-api/internal/audit/service.go` — hash-chain append mechanics, `AppendAuditEntryTx` vs `AppendAuditEntry`
- [VERIFIED: codebase read] `donnarec-api/internal/receiptno/fiscal_year.go` — Bangkok-timezone pure-function pattern to mirror for aging
- [VERIFIED: codebase read] `donnarec-api/internal/settings/service.go` + `migrations/000011_receipt_template_config.up.sql` — config-store single-row-table pattern to mirror
- [VERIFIED: codebase read] `donnarec-api/internal/testutil/postgres.go` + `minio.go` — testcontainers fixture patterns to reuse for restore-proof
- [VERIFIED: codebase read] `donnarec-api/go.mod` — confirms go1.25.1, absence of excelize, presence of pgx/testcontainers/minio-go

### Secondary (MEDIUM confidence)
- [CITED: pkg.go.dev/github.com/xuri/excelize/v2] `File.Write(io.Writer, ...Options) error`, `File.WriteTo`, `NewStreamWriter`/`StreamWriter.Flush` signatures
- [CITED: docs.min.io/community/minio-object-store/reference/minio-mc/mc-mirror.html] `mc mirror ALIAS/BUCKET LOCAL_DIRECTORY` syntax and semantics
- [CITED: postgresql.org/docs/current/app-pgdump.html and app-pgrestore.html] `-Fc` custom format + `pg_restore` usage (training-knowledge cross-checked against WebSearch results describing the same flags)
- [CITED: github.com/golang/go/issues/33887] `encoding/csv` Reader-side BOM handling caveat (confirms this is a reader-only issue, not relevant to the Writer-side BOM-prefix pattern used here)

### Tertiary (LOW confidence)
- WebSearch results describing community blog posts on Go CSV+BOM+Excel patterns and Docker Postgres backup companion-container examples (`kartoza/docker-pg-backup`, generic pg_dump cron tutorials) — used only to corroborate the CITED official-doc patterns above, not as a standalone source of truth; no specific numeric claims were taken from these without cross-checking against official docs

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — excelize version/compatibility verified directly against the Go module proxy; all other "new" dependencies are zero (stdlib/already-in-go.mod)
- Architecture: HIGH — every pattern except backup/restore directly mirrors an existing, working, tested pattern already in this codebase (RevealPII, receiptno, settings, testutil)
- Pitfalls: HIGH for Go/timezone/HTTP-header pitfalls (verified against Go stdlib docs and RFC references); MEDIUM for the exact pg_dump/pg_restore flags (cross-checked against official docs but not executed live in this session)
- Report date-basis (Assumption A1) and cancelled-exclusion (Assumption A2): LOW — genuinely ambiguous from CONTEXT.md/UI-SPEC, flagged explicitly for planner/user confirmation

**Research date:** 2026-07-06
**Valid until:** 30 days (stable Go/Postgres/MinIO ecosystem; re-verify excelize version if this phase's planning is delayed more than a month, as it releases fairly frequently)
