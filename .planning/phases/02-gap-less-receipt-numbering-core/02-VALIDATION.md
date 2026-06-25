---
phase: 2
slug: gap-less-receipt-numbering-core
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-06-25
---

# Phase 2 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test + testify + testcontainers-go (Postgres) |
| **Config file** | none — uses `donnarec-api/internal/testutil/postgres.go` fixture |
| **Quick run command** | `cd donnarec-api && go test ./internal/receiptno/... -count=1` |
| **Full suite command** | `cd donnarec-api && go test ./... -count=1` |
| **Estimated runtime** | ~30–90 seconds (testcontainers Postgres spin-up dominates) |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/receiptno/... -count=1`
- **After every plan wave:** Run `go test ./... -count=1`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 90 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| (to be filled by planner/executor) | — | — | FR-15/16/17/18, NFR-04 | — | — | unit/integration | `go test ./internal/receiptno/...` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/receiptno/fiscalyear_test.go` — boundary tests for `fiscalYear()` (30 Sep 23:59 / 1 Oct 00:00) — SC#2 / FR-17, FR-18
- [ ] `internal/receiptno/allocator_concurrency_test.go` — N-parallel allocation harness asserting zero gaps + zero dupes — SC#4 / NFR-04
- [ ] `internal/receiptno/allocator_rollback_test.go` — rollback leaves no gap; UNIQUE backstop fires — SC#4 / FR-16
- [ ] reuse `internal/testutil/postgres.go` — shared testcontainers Postgres fixture (no new framework install needed)

*If none: "Existing infrastructure covers all phase requirements."*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| (none expected) | — | — | — |

*All phase behaviors target automated verification — this is a backend-only, test-proven phase.*

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 90s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
