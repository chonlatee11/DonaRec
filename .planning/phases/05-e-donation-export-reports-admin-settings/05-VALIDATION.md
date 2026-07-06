---
phase: 05
slug: e-donation-export-reports-admin-settings
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-07-06
---

# Phase 05 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.
> Source of truth for the test map: `05-RESEARCH.md` § "Validation Architecture".

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test + testify; testcontainers-go (Postgres + MinIO); Next.js: existing Vitest/Playwright per Phase 3–4 |
| **Config file** | none new — existing `go.mod` / `internal/testutil/` fixtures |
| **Quick run command** | `go test ./internal/edonation/... ./internal/report/... ./internal/exportfile/... ./internal/backupverify/...` |
| **Full suite command** | `go test -race ./...` |
| **Estimated runtime** | ~fill during Wave 0 (testcontainers spin-up dominates) |

---

## Sampling Rate

- **After every task commit:** Run the package quick command for the touched package
- **After every plan wave:** Run `go test -race ./...`
- **Before `/gsd-verify-work`:** Full suite green + E2E HTTP-path test (integration-test gate) green
- **Max feedback latency:** fill during Wave 0

---

## Per-Task Verification Map

> Planner populates this from `05-RESEARCH.md` § "Phase Requirements → Test Map" and each plan's `<acceptance_criteria>`.

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 05-01-01 | 01 | 1 | FR-30/31/32 | T-05-01-SQLI | migrations 000013/000014 + sqlc set apply; parameterized queries | build | `go build ./... && sqlc generate` | ❌ W0 | ⬜ pending |
| 05-01-02 | 01 | 1 | FR-30/32 | T-05-01-PERSIST | stream-only xlsx/BOM-csv writer; io.Writer only, no temp file | unit+integration | `go test -short ./internal/exportfile/... && go test ./internal/edonation/... -run TestConfig` | ❌ W0 | ⬜ pending |
| 05-02-01 | 02 | 2 | FR-30 | T-05-02-PERSIST, T-05-02-UNAUDITED | issued-only decrypt; ONE audit row/export; no os.Create | integration | `go test ./internal/edonation/... -run TestExport` | ❌ W0 | ⬜ pending |
| 05-02-03 | 02 | 2 | FR-30 | T-05-02-RBAC | Maker 403 / Checker+Admin 200; stream-only over real HTTP path | E2E | `go test ./cmd/server/... -run TestE2E_EdonationExport` | ❌ W0 | ⬜ pending |
| 05-03-02 | 03 | 1 | NFR-08 | T-05-03-INCOMPLETE, T-05-03-UNVERIFIED | real restore into fresh DB+MinIO; asserted row/object counts | integration | `go test ./internal/backupverify/... -run TestRestoreProof` | ❌ W0 | ⬜ pending |
| 05-04-01 | 04 | 3 | FR-31 | T-05-04-TZ | Bangkok deadline; Dec→Jan rollover; boundary instants; no time.Now() | unit | `go test -short ./internal/edonation/... -run 'TestComputeBucket\|TestComputeDeadline'` | ❌ W0 | ⬜ pending |
| 05-04-02 | 04 | 3 | FR-31 | T-05-04-IDOR, T-05-04-AUDITGAP | issued-only bulk update; one audit row/donation | integration | `go test ./internal/edonation/... -run 'TestSetKeyed\|TestAging'` | ❌ W0 | ⬜ pending |
| 05-04-03 | 04 | 3 | FR-31 | T-05-04-RBAC | Maker 403 on /keyed + /aging over real HTTP path | E2E | `go test ./cmd/server/... -run TestE2E_EdonationKeyedAndAging` | ❌ W0 | ⬜ pending |
| 05-05-01 | 05 | 4 | FR-32 | T-05-05-PII | aggregate SUM/COUNT; no PII column; cancelled excluded | integration | `go test ./internal/report/... -run TestReportSummary` | ❌ W0 | ⬜ pending |
| 05-05-03 | 05 | 4 | FR-32 | T-05-05-AUTHZ-DRIFT | Maker gets 200 (all-staff, no gate) over real HTTP path | E2E | `go test ./cmd/server/... -run TestE2E_Reports` | ❌ W0 | ⬜ pending |
| 05-06 | 06 | 5 | FR-30/31 | T-05-06-TOKEN | BFF bearer server-side; nav gate UX-only | typecheck/lint | `npx tsc --noEmit && npx eslint app/e-donation components` | ➖ FE | ⬜ pending |
| 05-07 | 07 | 6 | FR-32 | T-05-07-CONFIGAUTH | ungated report screen; Admin-gated config tab via Go authority | typecheck/lint | `npx tsc --noEmit && npx eslint app/reports components` | ➖ FE | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

> Planner sets the concrete stub/fixture list. New test targets (created within each owning plan, RED-first where TDD):

- [ ] `internal/exportfile/writer_test.go` — 05-01 (BOM leading bytes, ZIP signature, Thai round-trip, io.Writer-only stream)
- [ ] `internal/edonation/config_test.go` — 05-01 (JSONB decode, HeaderRow ordering, default-mapping fallback)
- [ ] `internal/edonation/service_test.go` + `export_test.go` — 05-02 FR-30 (issued-only, cancelled-excluded, 13-digit decrypt, ONE audit row/export)
- [ ] `internal/edonation/aging_test.go` — 05-04 FR-31 (Bangkok deadline = 5th of approval-month+1, Dec→Jan rollover, boundary instants)
- [ ] `internal/edonation/keyed_test.go` — 05-04 FR-31 (issued-only bulk update, N-audit-rows-for-N-donations, unmark path, aging buckets)
- [ ] `internal/report/service_test.go` — 05-05 FR-32 (monthly/daily SUM/COUNT, cancelled excluded, empty-range zero, no-PII)
- [ ] `internal/backupverify/restore_test.go` — 05-03 NFR-08 (TestRestoreProof + TestRestoreProof_MinIO; fresh DB+MinIO, asserted data — D-73 evidence)
- [ ] `cmd/server/e2e_test.go` additions — 05-02/05-04/05-05 real HTTP-path E2E: `TestE2E_EdonationExport`, `TestE2E_EdonationKeyedAndAging`, `TestE2E_Reports` (integration-test gate)

*If none: "Existing infrastructure covers all phase requirements."*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Restore-verified drill (fresh DB + MinIO, assert data present) | NFR-08 | Full stack restore; evidence captured in runbook | Follow backup/restore runbook; record evidence in phase docs |
| Human UI walkthrough (Export / Aging / Reports screens) | FR-30/31/32 | Visual + interaction correctness | `/gsd-verify-work` walkthrough per UI-SPEC |

*If none: "All phase behaviors have automated verification."*

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency recorded after Wave 0
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
