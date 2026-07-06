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
| **Quick run command** | `go test ./internal/export/... ./internal/report/... ./internal/backup/...` |
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
| 05-01-01 | 01 | 1 | FR-30 | — | export restricted to Checker+Admin; download audited | integration | `go test ./internal/export/...` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

> Planner sets the concrete stub/fixture list. Expected new test targets:

- [ ] `internal/export/*_test.go` — stubs for FR-30 (xlsx/CSV mapping, Thai/BOM, audited decrypt, stream-only)
- [ ] `internal/report/*_test.go` — stubs for FR-32 (aggregate SUM/COUNT group-by, no-PII)
- [ ] `internal/backup/*_test.go` (or restore-verify harness) — stubs for NFR-08 (restore-verified)
- [ ] aging bucket unit tests — FR-31 (Bangkok-aware deadline = 5th of issue-month+1)
- [ ] `cmd/server/e2e_test.go` additions — real HTTP-path E2E for new export/flag/report endpoints (integration-test gate)

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
