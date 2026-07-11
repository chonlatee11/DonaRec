---
phase: 6
slug: public-donation-web-form-flow-b
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-07-11
---

# Phase 6 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test + testify + testcontainers-go (backend) / vitest (frontend, ถ้ามี) |
| **Config file** | `go.mod` + `internal/testutil/` (Postgres/Keycloak fixtures) |
| **Quick run command** | `rtk go test ./internal/... -run <TestScope> -count=1` |
| **Full suite command** | `rtk go test ./... -count=1` |
| **Estimated runtime** | ~120 seconds (testcontainers spin-up) |

---

## Sampling Rate

- **After every task commit:** Run `rtk go test ./internal/... -run <TestScope> -count=1`
- **After every plan wave:** Run `rtk go test ./... -count=1`
- **Before `/gsd-verify-work`:** Full suite must be green
- **Max feedback latency:** 180 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| (planner fills per PLAN.md) | | | FR-01..FR-08, NFR-06 | | | | | | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] Integration-test seam per CONVENTIONS.md: E2E over real HTTP path (public submit endpoint → magic-byte validation → pending_review + consent + audit + outbox) driven ด้วย testcontainers Postgres
- [ ] Rate-limit + CAPTCHA verification stubs (mock Turnstile verify endpoint)

*If existing infrastructure covers: mark "Existing infrastructure covers all phase requirements."*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Responsive/bilingual UI walkthrough (desktop + mobile, TH/EN) | NFR-06 | Visual/UX judgment | เปิด public form + back-office queue บน mobile viewport, สลับภาษา, เช็ค layout |
| Real Turnstile CAPTCHA challenge | FR-08 | ต้องใช้ browser จริง + Cloudflare | submit form ผ่าน browser, ยืนยัน challenge ทำงาน |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 180s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
