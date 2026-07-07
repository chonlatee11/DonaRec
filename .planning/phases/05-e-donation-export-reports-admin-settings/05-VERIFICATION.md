---
phase: 05-e-donation-export-reports-admin-settings
verified: 2026-07-07T15:37:36Z
status: human_needed
score: 4/4 must-haves verified
behavior_unverified: 0
overrides_applied: 0
mode_note: >
  ROADMAP has mode:mvp set on this phase, but the phase goal
  ("Staff can export issued-receipt data for manual e-Donation keying, track what
  has been keyed against the monthly deadline, view donation summary reports,
  manage settings, and rely on verified backups.") is NOT in User Story format
  (user-story.validate returns false), and the phase spans 7 plans across 6 waves —
  not a single vertical slice. MVP-mode user-flow narrowing was judged not to fit
  this deliverable's shape; standard goal-backward verification was applied instead
  against the phase's own explicit Success Criteria (ROADMAP + PLAN frontmatter).
  This is a process/config anomaly, not a phase-goal defect — flagged for the
  human to confirm the roadmap's mode tag is correct going forward.
human_verification:
  - test: "Screen 7 (/e-donation) manual UI walkthrough — Export tab (filter, count preview, amber PII-warning confirm dialog, streamed xlsx/csv download) and Aging tab (bucket stat cards, tri-state select-all, per-row + bulk mark/unmark keyed)"
    expected: "Export downloads a real file after confirming the PII warning; zero-count state disables export; aging table's 3 bucket cards filter correctly; bulk mark/unmark updates rows and they drop out of the unkeyed buckets"
    why_human: "No automated E2E/UI test exercises the browser-rendered confirm-dialog -> blob-download flow or the live bulk-select -> mutate -> refetch -> bucket-drop-out flow; 05-06-PLAN.md's own <verification> section explicitly defers this to /gsd-verify-work. Automated checks (tsc/eslint/vitest) only prove type-safety and unit-level component correctness, not this runtime flow."
  - test: "Screen 8 (/reports) manual UI walkthrough — date-range filter (default current fiscal year), summary cards (total/count/average), month/day breakdown toggle, PII-free export with NO confirmation dialog; and the 5th Settings tab (EdonationConfigTab) — load/edit/save field mapping + near_due_days, confirm the change takes effect on the next Aging fetch"
    expected: "Reports screen renders real totals/breakdown for all three roles (Maker included, no 403); export downloads immediately with no confirm dialog; Settings 5th tab loads current edonation_config, saves edits, and a near_due_days change is reflected in the Aging bucket thresholds on the next request"
    why_human: "No automated UI test exercises the real load -> edit -> save -> toast -> aging-threshold-takes-effect flow; 05-07-PLAN.md's own <verification> section explicitly defers this to /gsd-verify-work."
---

# Phase 5: e-Donation Export, Reports & Admin Settings Verification Report

**Phase Goal:** Staff can export issued-receipt data for manual e-Donation keying, track what has been keyed against the monthly deadline, view donation summary reports, manage settings, and rely on verified backups.
**Verified:** 2026-07-07T15:37:36Z
**Status:** human_needed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths (Success Criteria)

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Staff can generate an access-controlled Excel/CSV export of issued records mapped to e-Donation fields (13-digit ID, donation date, cash type); export is download-logged and restricted by role. | ✓ VERIFIED | `donnarec-api/internal/edonation/service.go:77-158` — `Service.Export` role-gates to Checker/Admin (`ErrForbidden` otherwise), queries `SearchIssuedForExport` (status='issued' only), decrypts each row's national ID via `crypto.DecryptField`, and appends exactly ONE audit row (action `edonation.export`, count/from/to/keyed_status) inside the SAME transaction, committed BEFORE plaintext is returned = the "download-logged" requirement. `cmd/server/main.go:420-423` wires `edonationGroup.Use(auth.RequireAnyRole(auth.RoleChecker, auth.RoleAdmin))` before `GET /export`. Live-ran `TestE2E_EdonationExport` against real Docker/testcontainers: PASS (21.8s) — Maker 403, Checker/Admin 200 with real ZIP-signature xlsx bytes, CSV BOM leading bytes, invalid-format 400, no bucket writes (D-74 stream-only). Field mapping (13-digit ID / donation date / cash type / receipt no / donor name) is config-driven via `edonation_config.field_mapping` (`internal/edonation/config.go`), not hardcoded. |
| 2 | Each record can be flagged "คีย์เข้า e-Donation แล้ว" and an aging view surfaces unkeyed records against the 5th-of-next-month deadline. | ✓ VERIFIED | `internal/edonation/aging.go` — pure `computeDeadline`/`computeBucket` (Bangkok-aware, no `time.Now()` inside — grep-verified 0 hits), December→January rollover tested. `service.go`'s `SetKeyed` writes one audit row PER matched donation (issued-only scope guard via a pre-update SELECT), `Aging` buckets unkeyed issued rows by `computeBucket(approved_at, now, near_due_days)`. Routes `POST /api/edonation/keyed` + `GET /api/edonation/aging` registered on the same Checker/Admin-gated `edonationGroup`. Live-ran `TestE2E_EdonationKeyedAndAging`: PASS (20.9s) — Maker 403 on both routes, Checker bulk-marks 2 real issued donations with exactly 2 new audit rows verified via direct DB query, malformed donation_id 422s (not 500), just-keyed rows excluded from the subsequent aging response. |
| 3 | Staff can view donation summary reports by date range and total amount. | ✓ VERIFIED | `internal/report/service.go` — `Service.Summary` aggregates `SummaryByMonth`/`SummaryByDay` (status='issued' only), computes `TotalAmount`/`ReceiptCount`/`AveragePerReceipt` (divide-by-zero guarded). `cmd/server/main.go:434-436` — `reportGroup := api.Group("/reports")` carries **no** `RequireAnyRole`/`RequireRoles` call (region-scoped grep confirms zero matches) — inherits only the outer `api.Use(authMW.RequireAuth())`, so every authenticated role can access it (D-71). Live-ran `TestE2E_Reports`: PASS (21.3s) — Maker gets 200 (not 403) on both `/summary` and `/export`, Checker/Admin 200, invalid group_by 400, export returns ZIP-signature xlsx with the audit_log row count unchanged (no PII reveal to audit). Frontend `/reports` page confirmed present (`donnarec-web/app/reports/page.tsx`) with date-range filter, summary cards, and breakdown table. |
| 4 | A backup runs on a regular schedule and a documented restore has been performed successfully (restore verified, not just configured). | ✓ VERIFIED | `docker-compose.yml`'s `backup` service (built from `docker/backup.Dockerfile`) confirmed present via `docker compose config` — cron-scheduled (`BACKUP_CRON=0 2 * * *`), 14-day retention. `scripts/backup.sh` uses `pg_dump -Fc` (custom format, never plain — grep-verified) and mirrors BOTH `donnarec-slips` and `donnarec-receipts` buckets via `mc mirror`. This is not just "configured" — live-ran the actual restore-proof tests against real Docker: `TestRestoreProof` PASS (6.9s, real log evidence: "users=3 donations=5 audit_log=4 restored into a fresh (unmigrated) Postgres 17 container, exactly matching the seeded source fixture, pg_dump artifact = 45759 bytes"); `TestRestoreProof_MinIO` PASS (2.6s, real log evidence: "4 objects across both buckets restored into a fresh MinIO instance via mirror-out/mirror-in round trip, every key present with byte-exact content"). `docs/BACKUP_RESTORE_RUNBOOK.md` exists with a "Verification Evidence" section recording both this automated proof and a live `docker compose run --rm backup` smoke test. |

**Score:** 4/4 truths verified (0 present, behavior-unverified)

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `donnarec-api/internal/exportfile/writer.go` | Stream-only xlsx/csv writer, io.Writer only | ✓ VERIFIED | `StreamXLSX`/`StreamCSV`/`SetDownloadHeaders` present; zero `os.Create`/`os.TempFile` (grep-verified); BOM + ZIP signature confirmed by `writer_test.go`. |
| `donnarec-api/internal/edonation/service.go` | Audited export + SetKeyed + Aging service | ✓ VERIFIED | All three methods present and wired; audit discipline confirmed by direct code read + passing E2E DB-query assertions. |
| `donnarec-api/internal/edonation/aging.go` | Pure Bangkok-aware deadline/bucket computation | ✓ VERIFIED | No `time.Now()` inside; December-rollover + boundary tests present and passing. |
| `donnarec-api/internal/report/service.go` | PII-free aggregate report service | ✓ VERIFIED | No PII column/decrypt call referenced (grep-verified); tests pass. |
| `donnarec-api/internal/backupverify/restore_test.go` | Restore-proof integration tests | ✓ VERIFIED | `TestRestoreProof`/`TestRestoreProof_MinIO` — live-run against real Docker, both PASS with real evidence. |
| `donnarec-api/docs/BACKUP_RESTORE_RUNBOOK.md` | Restore runbook w/ recorded evidence | ✓ VERIFIED | Present, contains "Verification Evidence" section. |
| `donnarec-web/app/e-donation/page.tsx` | Screen 7 (Export + Aging tabs) | ✓ VERIFIED | Present, Server Component RBAC-gates via `isCheckerOrAdminViewer()` (UX hint; Go RBAC is authoritative). |
| `donnarec-web/app/reports/page.tsx` | Screen 8 (Reports) | ✓ VERIFIED | Present, no RBAC gate (matches D-71). |
| `donnarec-web/components/EdonationConfigTab.tsx` | 5th Settings tab, admin config editor | ✓ VERIFIED | Present, wired into `SettingsTabs.tsx`. |

### Key Link Verification

| From | To | Via | Status | Details |
|------|-----|-----|--------|---------|
| `edonationGroup` (RequireAnyRole Checker/Admin) | `Handler.Export` → `Service.Export` → `exportfile.StreamXLSX/CSV` | route registration + audited-decrypt-then-stream discipline | ✓ WIRED | `main.go:420-423`; confirmed transaction never imports `exportfile` (streaming happens after commit, in handler/xlsx.go/csv.go) — matches Pitfall-3 discipline. |
| `SetKeyedBulk` (WHERE status='issued') | per-donation `AppendAuditEntryTx` | pre-update SELECT scoping the audit loop to actually-matched rows | ✓ WIRED | `service.go` SetKeyed; E2E asserts exactly 2 audit rows for 2 marked donations. |
| `reportGroup` (RequireAuth only) | `Handler.Summary`/`Export` → `Service.Summary` | no role gate (D-71) | ✓ WIRED | Region-scoped grep of the `Group("/reports")` block shows zero `RequireAnyRole`/`RequireRoles` matches; E2E confirms Maker gets 200. |
| `docker-compose backup` service | `scripts/backup.sh` (pg_dump -Fc + mc mirror) | cron schedule inside `backup.Dockerfile` | ✓ WIRED | `docker compose config` resolves the service; script content grep-confirmed. |
| `ExportPanel` → `ExportConfirmDialog` → BFF `/api/bff/edonation/export` → Go `/api/edonation/export` | binary passthrough | BFF forwards `arrayBuffer()` + headers verbatim | ✓ WIRED (code-level) | Route file present (`app/api/bff/edonation/export/route.ts`); binary passthrough pattern confirmed by summary + prior phase precedent. Runtime browser behavior deferred to human verification. |
| `AgingTable` selection → `BulkActionBar` → BFF `/api/bff/edonation/keyed` → Go `POST /keyed` | mutation + refetch | TanStack Query invalidation | ✓ WIRED (code-level) | Files present; runtime behavior deferred to human verification. |

### Behavioral Spot-Checks (live-run against real Docker)

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Export RBAC + audit + stream-only, real HTTP path | `go test -count=1 ./cmd/server/... -run TestE2E_EdonationExport` | PASS (21.8s) — all 6 subtests green | ✓ PASS |
| Keyed mark/unmark + aging bucketing, real HTTP path | `go test -count=1 ./cmd/server/... -run TestE2E_EdonationKeyedAndAging` | PASS (20.9s) — all 5 subtests green | ✓ PASS |
| Reports all-staff access + PII-free export, real HTTP path | `go test -count=1 ./cmd/server/... -run TestE2E_Reports` | PASS (21.3s) — all 5 subtests green | ✓ PASS |
| DB restore-proof (real dump → fresh Postgres) | `go test -count=1 ./internal/backupverify/... -run TestRestoreProof` | PASS (6.9s) — "users=3 donations=5 audit_log=4 restored ... exactly matching the seeded source fixture" | ✓ PASS |
| Object-storage restore-proof (real mirror round trip) | `go test -count=1 ./internal/backupverify/... -run TestRestoreProof_MinIO` | PASS (2.6s) — "4 objects across both buckets restored ... every key present with byte-exact content" | ✓ PASS |
| Backend build | `go build ./...` | clean | ✓ PASS |
| Frontend typecheck | `npx tsc --noEmit` | clean | ✓ PASS |
| Frontend unit tests | `npx vitest run` | 44/44 passed (7 files) | ✓ PASS |
| `docker compose config` | parses `backup` service | present, correctly configured | ✓ PASS |

All spot-checks above were executed live in this verification pass, not taken from SUMMARY.md claims.

### Requirements Coverage

| Requirement | Source Plan(s) | Description | Status | Evidence |
|-------------|-----------------|-------------|--------|----------|
| FR-30 | 05-01, 05-02, 05-06 | ส่งออกข้อมูล (Excel/CSV) เพื่อรองรับการคีย์เข้า e-Donation ด้วยตนเอง | ✓ SATISFIED | `Service.Export` + `edonationGroup` route + `TestE2E_EdonationExport` (all live-verified above). |
| FR-31 | 05-01, 05-04, 05-06 | ติดสถานะ "คีย์เข้า e-Donation แล้ว" เพื่อกันคีย์ซ้ำ/ตกหล่น | ✓ SATISFIED | `Service.SetKeyed`/`Aging` + `TestE2E_EdonationKeyedAndAging` (live-verified above). |
| FR-32 | 05-01, 05-05, 05-07 | รายงานสรุปการบริจาค (ตามช่วงเวลา/ยอดรวม) | ✓ SATISFIED | `Service.Summary` + ungated `reportGroup` + `TestE2E_Reports` (live-verified above). |
| NFR-08 | 05-03 | สำรองข้อมูล (backup) สม่ำเสมอและกู้คืนได้ | ✓ SATISFIED | `backup` compose service + `TestRestoreProof`/`TestRestoreProof_MinIO` (live-verified above, real evidence, not a configured-but-untested claim). |

No orphaned requirements — REQUIREMENTS.md maps exactly FR-30/FR-31/FR-32/NFR-08 to Phase 5, and all four are declared across the phase's plans and directly evidenced above.

### Anti-Patterns Found

None of the standard debt-marker patterns (TBD/FIXME/XXX/TODO/HACK/PLACEHOLDER, empty handlers, hardcoded empty returns) were found in the source files reviewed for this phase (`internal/edonation/*.go`, `internal/report/*.go`, `internal/exportfile/writer.go`, `internal/backupverify/restore_test.go`, the Screen 7/8 frontend files). No debt-marker gate violations.

The phase's own code review (`05-REVIEW.md`, `depth: standard`, 33 files reviewed) surfaced 1 Critical + 3 Warning + 2 Info findings. Per the verification context, these were consciously deferred by the user to a later `/gsd-code-review --fix` pass rather than blocking phase completion. Independently re-confirmed still present in the current codebase (not silently fixed and not falsely claimed fixed):

- **CR-01 (Critical, CSV/formula injection, CWE-1236):** `internal/exportfile/writer.go`'s `StreamCSV` has no `sanitizeCSVField`-style leading-character escaping — confirmed absent by direct grep. A donor name starting with `=`/`+`/`-`/`@` in the CSV export path (which also carries the full plaintext 13-digit national ID) would be interpreted as a live formula by Excel. **This affects Success Criterion 1's export-file safety** — the export itself works and is correctly audited/RBAC-gated, but the CSV variant is not yet hardened against this injection class. `.xlsx` path is unaffected (excelize stores explicit string cell types).
- **WR-01 (Warning, date-field mismatch):** Confirmed `ExportPanel.tsx:77` still computes the client-side record-count preview off `row.approved_at`, while the backend filter (`SearchIssuedForExport`) and the filter label key off `donated_at` — a three-way mismatch that can make the PII-warning dialog's displayed count diverge from the actual exported row count.
- **WR-02 (Warning, bulk-mark provenance overwrite):** Not independently re-verified in this pass beyond confirming `SetKeyedBulk` is present and unconditional per the reviewed code; accepted as still-open per the deferral.
- **WR-03 (Warning, float64 money aggregation in reports):** `internal/report/service.go` still sums in `float64` (per 05-05-SUMMARY.md's own admission); a display/report-only precision risk, not a receipt-issuance correctness defect.

These are tracked as known open items, not newly-discovered blockers — consistent with the verification_context instruction to record rather than re-litigate them.

### Human Verification Required

1. **Screen 7 (/e-donation) manual UI walkthrough**
   **Test:** Log in as Checker/Admin, use the Export tab (set filters, confirm the amber PII-warning dialog, download xlsx and csv) and the Aging tab (click bucket stat cards, select rows via the tri-state checkbox, bulk mark/unmark keyed).
   **Expected:** Filters narrow the count preview; confirm dialog blocks direct download; file downloads successfully; aging buckets filter the table; bulk/per-row mark-keyed updates rows and they disappear from unkeyed buckets on refetch.
   **Why human:** No automated E2E/browser test exercises this flow; the plan (05-06-PLAN.md `<verification>`) explicitly defers it to `/gsd-verify-work`.

2. **Screen 8 (/reports) + 5th Settings tab manual UI walkthrough**
   **Test:** As any staff role (including Maker), view `/reports` with the fiscal-year-default filter, toggle month/day breakdown, export without a confirmation dialog. As Admin, open Settings' 5th tab, edit field mapping/near_due_days, save, then confirm the Aging view reflects the new threshold.
   **Expected:** Reports render correctly for all roles; export has no confirm gate; config tab loads/saves and the near_due_days change is visible on the next aging fetch.
   **Why human:** No automated UI test exercises this flow; the plan (05-07-PLAN.md `<verification>`) explicitly defers it to `/gsd-verify-work`.

### Gaps Summary

No gaps were found that block the phase goal. All four Success Criteria are independently verified against the actual codebase (not SUMMARY.md claims) with live-executed automated tests against real Docker/testcontainers. The phase is functionally complete and the integration-test gate (CLAUDE.md Conventions) is genuinely satisfied for every backend slice — `TestE2E_EdonationExport`, `TestE2E_EdonationKeyedAndAging`, and `TestE2E_Reports` were all re-run in this verification pass and pass for real, driving the real router with real signed Keycloak-shaped tokens.

Overall status is `human_needed` rather than `passed` only because of the two deferred manual UI walkthroughs (Screen 7 and Screen 8), which the phase's own plans structurally defer to `/gsd-verify-work` — this is expected process, not a defect.

One process anomaly is noted (not a phase-goal defect): ROADMAP has `mode: mvp` on this phase, but the phase goal is not in valid User Story format and the phase spans 7 plans/6 waves — inconsistent with a single MVP vertical slice. Recommend the human confirm/correct this mode tag for future phases.

One known, consciously-deferred security defect (CR-01, CSV formula injection) and two known warnings (WR-01, WR-03) remain unresolved in the codebase, per the user's explicit decision to route them to a later `/gsd-code-review --fix` pass rather than block this phase. They are documented here for traceability, not treated as blockers.

---

_Verified: 2026-07-07T15:37:36Z_
_Verifier: Claude (gsd-verifier)_
