---
phase: 04
slug: receipt-pdf-email-delivery-outbox-worker
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-07-04
---

# Phase 04 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test + testify + testcontainers-go |
| **Config file** | none — existing `internal/testutil/` fixtures |
| **Quick run command** | `go test ./internal/...` |
| **Full suite command** | `go test ./... && (cd web && npm test)` |
| **Estimated runtime** | ~60–120 seconds (testcontainers Postgres + Chromium render) |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/<package>/...`
- **After every plan wave:** Run `go test ./...`
- **Before `/gsd-verify-work`:** Full suite must be green (incl. golden-file PDF test)
- **Max feedback latency:** 120 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 04-01-01 | 01 | 0 | NFR-07 | — | worker polls without holding issuance lock | integration | `go test ./internal/worker/...` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky. This map is populated by the planner from PLAN.md must_haves.*

---

## Wave 0 Requirements

- [ ] `internal/pdf/render_golden_test.go` — golden-file worst-case Thai render (SC#2)
- [ ] `internal/worker/worker_test.go` — outbox poll `FOR UPDATE SKIP LOCKED` + idempotency + backoff
- [ ] Docker-compose Chromium sidecar + `fonts-thai-tlwg` + TH Sarabun available in CI

*If none: "Existing infrastructure covers all phase requirements."*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Admin live preview smoothness (debounce/no jank) | FR-33/NFR-09/D-61 | Perceptual UX, not assertable | Edit template in Admin UI, confirm preview updates debounced without full re-render per keystroke |
| Bilingual email visually renders w/ PDF attached in a real inbox | FR-25/FR-26 | Requires real mail client rendering | Send to test inbox, confirm TH/EN subject+body + PDF attachment opens |

*If none: "All phase behaviors have automated verification."*

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 120s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
