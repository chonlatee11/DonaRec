---
phase: 05-e-donation-export-reports-admin-settings
audited: 2026-07-11T02:40:54Z
asvs_level: 1
block_on: high
threats_total: 31
threats_mitigate: 27
threats_accept: 4
threats_closed: 31
threats_open: 0
unregistered_flags: 1
status: SECURED
---

# Phase 05: Security Audit Report

**Audited:** 2026-07-11
**ASVS Level:** 1 (grep-level: mitigation present in the cited file)
**block_on:** high (only high/critical OPEN threats would block ship)

## Methodology

Every threat declared in `05-01..05-07-PLAN.md`'s `<threat_model>` blocks was verified
against the CURRENT implementation (not SUMMARY.md claims, not stale VERIFICATION.md
snapshots). `mitigate` threats were closed only on a positive code-location match for
the exact mechanism the plan declared (route guard, query parameterization, grep for
forbidden APIs, etc.) — code structure or a plausible-looking function name was never
accepted as a substitute for a located line. `accept` threats were checked for a
plan-time rationale and recorded here as the accepted-risk log. Where the register's
own PLAN text pointed at an E2E test as evidence, that test was **re-run live against
real Docker/testcontainers in this audit** rather than trusted from the SUMMARY:

```
go test -count=1 ./cmd/server/... -run 'TestE2E_EdonationExport|TestE2E_EdonationKeyedAndAging|TestE2E_Reports'
  --- PASS: TestE2E_EdonationExport (22.90s)          [6/6 subtests pass]
  --- PASS: TestE2E_EdonationKeyedAndAging (22.26s)   [5/5 subtests pass]
  --- PASS: TestE2E_Reports (21.80s)                  [5/5 subtests pass]

go test -count=1 ./internal/backupverify/... -run 'TestRestoreProof'
  --- PASS: TestRestoreProof (6.24s)        — users=3 donations=5 audit_log=4 restored exactly
  --- PASS: TestRestoreProof_MinIO (2.39s)  — 4 objects, byte-exact content

go build ./...   — clean
```

## Threat Verification

### 05-01 — Shared substrate

| Threat ID | Category | Severity | Disposition | Status | Evidence |
|-----------|----------|----------|-------------|--------|----------|
| T-05-01-SQLI | Tampering | high | mitigate | CLOSED | `internal/db/queries/edonation.sql` (SearchIssuedForExport, SetKeyedBulk, GetEdonationConfig, UpdateEdonationConfig) and `reports.sql` (SummaryByMonth/Day) — every filter uses `sqlc.narg(...)` or `@param` placeholders; zero string concatenation. |
| T-05-01-PERSIST | Info Disclosure | high | mitigate | CLOSED | `internal/exportfile/writer.go` — `StreamXLSX`/`StreamCSV` take `io.Writer` only; `grep -c 'os.Create\|os.TempFile' internal/exportfile/*.go internal/edonation/*.go internal/report/*.go` = 0 across all three packages. |
| T-05-01-CONFIG | Tampering | medium | mitigate | CLOSED | `migrations/000014_edonation_config.up.sql:71` — `GRANT SELECT, INSERT, UPDATE ON edonation_config TO donnarec_app;` — no DELETE granted. Write path further gated by `RequireRoles(Admin)` on `adminGroup` (`cmd/server/main.go:338-361`). |
| T-05-01-SC (T-05-SC) | Tampering (supply chain) | low | accept | CLOSED-accepted | `go.mod:23` — `github.com/xuri/excelize/v2 v2.11.0` pinned exactly as declared. Accepted-risk rationale (05-RESEARCH Package Legitimacy Audit — Approved, Go module proxy, 20+ tagged releases) recorded in `05-01-PLAN.md`'s threat register; logged here as the accepted-risks entry. |

### 05-02 — e-Donation export

| Threat ID | Category | Severity | Disposition | Status | Evidence |
|-----------|----------|----------|-------------|--------|----------|
| T-05-02-RBAC | Elevation of Privilege | high | mitigate | CLOSED | `cmd/server/main.go:420-423` — `edonationGroup.Use(auth.RequireAnyRole(auth.RoleChecker, auth.RoleAdmin))` (OR-guard) before `GET /export`. Live-verified: `TestE2E_EdonationExport/Maker_Forbidden_403` PASS, `/Checker_200_XLSX_ZipSignature` PASS, `/Admin_200_XLSX` PASS. |
| T-05-02-PERSIST | Info Disclosure | high | mitigate | CLOSED | `internal/edonation/service.go` never imports `internal/exportfile` (streaming happens in handler.go after commit, per Pitfall-3 discipline); `TestE2E_EdonationExport/NoBucketWrites_D74` PASS (live). |
| T-05-02-UNAUDITED | Repudiation | high | mitigate | CLOSED | `internal/edonation/service.go:130-144` — exactly ONE `AppendAuditEntryTx` (`action="edonation.export"`) inside the same `dbhelpers.WithTx` closure as the query+decrypt, committed before `rows` is returned to the caller. `internal/db/*.go WithTx`: `defer tx.Rollback(ctx)` + `Commit` only reached if `fn` returns nil — an audit-write failure aborts the transaction, so plaintext is never handed back on a failed audit. |
| T-05-02-BULK | Info Disclosure | medium | accept | CLOSED-accepted | Plan-documented rationale (D-64's after-the-fact audit gives who/when/range/count detectability; hard record-cap deferred as an MVP accepted risk) — `05-02-PLAN.md` threat register; logged here as the accepted-risks entry. |
| T-05-02-LOGPII | Info Disclosure | medium | mitigate | CLOSED | `internal/edonation/service.go:154-157` — `s.logger.Info("edonation export", zap.Int("count", ...), zap.String("actor", ...))` — no plaintext ID field anywhere in the log call; same discipline in `handler.go:167-180`. |
| T-05-02-SQLI | Tampering | medium | mitigate | CLOSED | `SearchIssuedForExport` uses `sqlc.narg` (see T-05-01-SQLI); `handler.go:52-82` — `allowedExportFormats = {"xlsx","csv"}` allowlist rejects any other `format` value with 400 before any DB call. |

### 05-03 — Backup & restore (NFR-08)

| Threat ID | Category | Severity | Disposition | Status | Evidence |
|-----------|----------|----------|-------------|--------|----------|
| T-05-03-DUMPLEAK | Info Disclosure | high | mitigate | CLOSED | Root `.gitignore:` `backups/`; `donnarec-api/.gitignore:5` `backups/`; `docker-compose.yml:290-291,317-318` — the `backup` service mounts a NAMED volume (`backups:/backups`), never a repo bind mount. |
| T-05-03-INCOMPLETE | DoS (data loss) | high | mitigate | CLOSED | `scripts/backup.sh:52-56` — `mc mirror` runs against BOTH `donnarec-slips` and `donnarec-receipts` (env-configurable, defaults present). |
| T-05-03-BADFORMAT | DoS | medium | mitigate | CLOSED | `scripts/backup.sh:43-46` — `pg_dump -Fc` (custom format), never plain/default. |
| T-05-03-UNVERIFIED | Repudiation | high | mitigate | CLOSED | `internal/backupverify/restore_test.go` — re-run live in this audit: `TestRestoreProof` PASS (6.24s, "users=3 donations=5 audit_log=4 restored... exactly matching the seeded source fixture"), `TestRestoreProof_MinIO` PASS (2.39s, "4 objects... every key present with byte-exact content"). Both target a genuinely fresh/unmigrated container, not the source. |

### 05-04 — Keyed status + aging

| Threat ID | Category | Severity | Disposition | Status | Evidence |
|-----------|----------|----------|-------------|--------|----------|
| T-05-04-RBAC | Elevation of Privilege | high | mitigate | CLOSED | `cmd/server/main.go:426-427` — `POST /keyed`, `GET /aging` on the same `edonationGroup` (`RequireAnyRole(Checker,Admin)`). Live-verified: `TestE2E_EdonationKeyedAndAging/Maker_Forbidden_403_Keyed` + `/Maker_Forbidden_403_Aging` PASS. |
| T-05-04-IDOR | Tampering | medium | mitigate | CLOSED | `internal/edonation/service.go:217-243` — pre-update `SELECT ... WHERE status='issued' AND edonation_keyed <> $2` scopes the audit/update blast radius; `SetKeyedBulk` (`edonation.sql:50-56`) independently re-guards `WHERE status='issued'` (defense-in-depth). |
| T-05-04-SQLI | Tampering | medium | mitigate | CLOSED | `internal/edonation/handler.go:307-309,347-366` — `KeyedRequestBody.DonationIDs` validated `required,min=1,dive,uuid` BEFORE any `pgtype.UUID.Scan`/DB call; malformed id → 422, never reaches the query. Live-verified: `TestE2E_EdonationKeyedAndAging/Malformed_DonationID_422NotDB500` PASS. |
| T-05-04-AUDITGAP | Repudiation | medium | mitigate | CLOSED | `internal/edonation/service.go:257-274` — one `AppendAuditEntryTx` per matched donation inside the same `WithTx`; a failure on any row aborts the whole transaction (`WithTx`'s commit-only-on-nil-error semantics, see T-05-02-UNAUDITED evidence). |
| T-05-04-TZ | Tampering | low | mitigate | CLOSED | `internal/edonation/aging.go` — `computeDeadline`/`computeBucket` never call `time.Now()` (grep confirms 0 matches); `sync.Once` + panic-on-missing-tzdata Bangkok loader; December→January rollover handled via `time.Date` month-overflow (no hand-written special case). |

### 05-05 — Donation summary report

| Threat ID | Category | Severity | Disposition | Status | Evidence |
|-----------|----------|----------|-------------|--------|----------|
| T-05-05-PII | Info Disclosure | high | mitigate | CLOSED | `internal/report/service.go` — constructor takes only `*db.Queries` (no `keyProvider`, no `auditSvc`); `SummaryByMonth`/`SummaryByDay` select only `period`/`receipt_count`/`total_amount` — no donor/PII column, no `DecryptField` call anywhere in the package. |
| T-05-05-ENUM | Info Disclosure | low | accept | CLOSED-accepted | D-71 — intentional, report has zero PII; plan-documented rationale in `05-05-PLAN.md`'s threat register, logged here as the accepted-risks entry. |
| T-05-05-SQLI | Tampering | medium | mitigate | CLOSED | `reports.sql` uses `sqlc.narg` (T-05-01-SQLI evidence); `internal/report/handler.go:40-47,92-97,176-180` — `allowedGroupBy={"month","day"}` and `allowedReportExportFormats={"xlsx","csv"}` allowlists reject any other value with 400. |
| T-05-05-AUTHZ-DRIFT | Elevation of Privilege | low | mitigate | CLOSED | `cmd/server/main.go:434-436` — `reportGroup` block contains no `RequireAnyRole`/`RequireRoles` call. Live-verified: `TestE2E_Reports/Maker_200_AllStaffAccess` PASS (asserts 200, not 403, with correct aggregate JSON). |

### 05-06 — Screen 7 UI (Export + Aging)

| Threat ID | Category | Severity | Disposition | Status | Evidence |
|-----------|----------|----------|-------------|--------|----------|
| T-05-06-TOKEN | Info Disclosure | high | mitigate | CLOSED | `lib/bff.ts:22-25,105-121` — `getBffToken()`/`bffForward()` resolve the Keycloak access token via `getServerSession` (server-side only) and attach it as `Authorization: Bearer` on the outbound Go fetch; the token is never serialized into any BFF route's response body. |
| T-05-06-UXGATE | Elevation of Privilege | medium | mitigate | CLOSED | `app/e-donation/page.tsx:16-26` — `isCheckerOrAdminViewer()` gate is explicitly documented and structurally only a `redirect()`; the real authority is Go's `RequireAnyRole` (T-05-04-RBAC/T-05-02-RBAC evidence), independently re-checked by every BFF call regardless of this redirect. |
| T-05-06-FILECUSTODY | Info Disclosure | medium | mitigate | CLOSED | `components/ExportConfirmDialog.tsx` renders the amber-50 `role="alert"` banner; `messages/th.json:318-319` — warning copy explicitly instructs "กรุณาจัดเก็บและลบไฟล์อย่างปลอดภัยหลังใช้งาน ห้ามอัปโหลดไปยังระบบอื่นที่ไม่ปลอดภัย" (store/delete the file securely; never upload to an insecure system). |
| T-05-06-BINARY | Tampering | low | mitigate | CLOSED | `app/api/bff/edonation/export/route.ts:52-58` — reads `goRes.arrayBuffer()` and forwards bytes + `Content-Type`/`Content-Disposition` verbatim; no JSON parse, no re-encode step in the binary path. |

### 05-07 — Screen 8 UI + admin config tab

| Threat ID | Category | Severity | Disposition | Status | Evidence |
|-----------|----------|----------|-------------|--------|----------|
| T-05-07-PII | Info Disclosure | medium | mitigate | CLOSED | Same evidence as T-05-05-PII (shared `internal/report` package) — no PII column selected server-side; `ReportSummaryCards`/`ReportBreakdownTable` render only period/count/amount fields, no donor identifier field exists in the response DTO to render. |
| T-05-07-CONFIGAUTH | Elevation of Privilege | high | mitigate | CLOSED | `cmd/server/main.go:338-339,360-361` — `adminGroup.Use(auth.RequireRoles(auth.RoleAdmin))` registered BEFORE `GET/PUT /edonation-config` are added to the same group. `app/api/bff/edonation-config/route.ts` is a bare `bffForward` passthrough with no client-side bypass of the Go check. |
| T-05-07-TOKEN | Info Disclosure | high | mitigate | CLOSED | Same evidence as T-05-06-TOKEN (`lib/bff.ts` shared by all Phase-5 BFF routes, including `reports/route.ts`, `reports/export/route.ts`, `edonation-config/route.ts`). |
| T-05-07-ENUM | Info Disclosure | low | accept | CLOSED-accepted | D-71 — same rationale as T-05-05-ENUM; plan-documented in `05-07-PLAN.md`'s threat register, logged here as the accepted-risks entry. |

## Accepted Risks Log

| Threat ID | Rationale | Recorded |
|-----------|-----------|----------|
| T-05-SC / T-05-01-SC | `excelize/v2` verified [Approved] in 05-RESEARCH Package Legitimacy Audit (Go module proxy, 20+ tagged releases); pinned to v2.11.0 exactly — no floating version. | `05-01-PLAN.md` threat register; this audit confirms the pin holds in `go.mod`. |
| T-05-02-BULK | A compromised/malicious Checker or Admin could still bulk-export a large PII set in one call; D-64's audit trail (actor/range/count) gives after-the-fact detectability, not prevention. A hard per-export record cap was explicitly deferred — not in this phase's CONTEXT-locked scope. | `05-02-PLAN.md` threat register. Recommend revisiting a record-cap or anomaly-alerting control in a future phase. |
| T-05-05-ENUM / T-05-07-ENUM | D-71 (deliberate): the donation summary report has zero PII and is intended to be transparently visible to every authenticated staff role, including Maker. This is a designed disclosure, not an oversight. | `05-05-PLAN.md` / `05-07-PLAN.md` threat registers; confirmed by code (no PII column exists to leak) and by the live `Maker_200_AllStaffAccess` E2E assertion. |

## Unregistered Flags (new attack surface, no threat-model mapping)

No `## Threat Flags` section was present in any of the 8 SUMMARY.md files for this phase — SUMMARY-declared new attack surface is empty. However, this audit does **not** treat that as a complete picture (per the adversarial-stance requirement not to assume SUMMARY.md's Threat Flags is exhaustive) and additionally cross-checked `05-REVIEW.md` (the phase's independent code-review artifact), which surfaced one genuine security defect outside the declared threat register:

| Flag | Category | Severity (reviewer-assigned) | Found in | Status | Evidence |
|------|----------|-------------------------------|----------|--------|----------|
| CSV/Formula Injection (CWE-1236) — unescaped donor-controlled text in `.csv` exports, adjacent to plaintext national ID | Tampering / Info Disclosure | Critical (per `05-REVIEW.md` CR-01) | `internal/exportfile/writer.go` `StreamCSV` (shared by both the e-Donation export and the report export paths) | **CLOSED (fixed post-review)** | `writer.go:70-100` — `sanitizeCSVField`/`sanitizeCSVRow` now prefix a leading apostrophe on any cell beginning with `=`,`+`,`-`,`@`,tab, or CR, applied to every header and data cell inside `StreamCSV` itself (protects both the e-Donation export and the report export by construction). Fix commit: `9d5226f "fix(05): CR-01 sanitize CSV export fields against formula injection (CWE-1236)"`, confirmed present on the current branch tip (`git log --oneline` shows it between `74925cf` code-review-doc and the later WR-01/WR-02/WR-03/IN-01/IN-02 fix commits, all ancestors of current `HEAD` `64f46b9`). |

**Important discrepancy flagged for the record:** `05-VERIFICATION.md` (dated 2026-07-07T15:37:36Z, still on disk unchanged) states CR-01 is "consciously deferred... independently re-confirmed still present in the current codebase" — that was true AT THE TIME that verification pass ran, but is **no longer true**. Commit `9d5226f` (and the subsequent WR-01/WR-02/WR-03/IN-01/IN-02 fix commits, all independently spot-checked against current source during this audit — e.g. `service.go`'s `edonation_keyed <> $2` WR-02 guard, `numericToRat`/`big.Rat` WR-03 accumulation) landed after that verification snapshot and before the current branch tip. `05-VERIFICATION.md` was not updated to reflect this — it is now a stale artifact with respect to CR-01/WR-01/WR-02/WR-03/IN-01/IN-02. This audit is based on the CURRENT repository state (`git log` HEAD `64f46b9`), where all six review findings are fixed. No action required on the implementation; flagging that `05-VERIFICATION.md` should be refreshed or annotated so a future reader does not mistakenly re-open CR-01 as still-outstanding.

This flag does not count toward `threats_open` (it was not a declared threat and is CLOSED as of current HEAD), but is recorded here per the adversarial-stance requirement to surface new attack surface discovered outside the declared register.

## Gaps Summary

**No blocking gaps.** All 27 `mitigate` threats and 4 `accept` threats across `05-01` through `05-07` resolved to CLOSED against the current implementation, independently verified via direct code inspection (not SUMMARY.md claims) and live re-execution of the E2E/integration test suites this audit could reach (all passed against real Docker). `threats_open = 0` at `block_on: high`.

One out-of-register Critical finding (CSV/formula injection) was located via cross-referencing `05-REVIEW.md` and confirmed fixed in the current codebase — flagged as an `unregistered_flag`, not a blocker, with a note that the phase's own `05-VERIFICATION.md` is stale on this point.

**Phase 05 is SECURED under this audit's threat register.**
