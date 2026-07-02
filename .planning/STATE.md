---
gsd_state_version: 1.0
milestone: v1.0
milestone_name: milestone
status: executing
stopped_at: "Phase 3 REOPENED — integration gate not met (3 runtime-seam bugs found in human-verify)"
last_updated: "2026-07-02T08:40:00.000Z"
last_activity: 2026-07-02
progress:
  total_phases: 6
  completed_phases: 2
  total_plans: 17
  completed_plans: 15
  percent: 33
  note: "Phase 3 was Complete 2026-07-01 on unit-level 5/5; reopened 2026-07-02 — integration gate (criterion 6) OPEN"
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-06-22)

**Core value:** ออกใบเสร็จบริจาคที่มีเลขที่รันต่อเนื่องไม่ซ้ำ ห้ามข้ามเลข (gap-less) ตามปีงบประมาณ หลังผ่านการอนุมัติโดยมนุษย์ และส่งถึงผู้บริจาคได้อย่างถูกต้องน่าเชื่อถือ
**Current focus:** Phase 03 — donation-lifecycle-maker-checker-issuance

## Current Position

Phase: 03 (donation-lifecycle-maker-checker-issuance) — ⚠️ REOPENED (integration gate open)
Plans: 8/8 complete (criteria 1–5, unit/service-level). Integration gate (criterion 6) NOT met.
Status: Remediation in progress — see Blockers/Concerns for the 3 runtime-seam bugs.
Last activity: 2026-07-02

Context: Phase 3 was marked Complete 2026-07-01 on 5/5 unit-level verification. On 2026-07-02, standing up the real stack (docker compose; postgres remapped to host 5433 via docker-compose.override.yml; 4 users seeded) and driving it with a real Keycloak token surfaced three runtime-integration-seam bugs that unit tests structurally could not catch. New done-criterion added (Conventions → Integration-test gate; ROADMAP Phase 3 criterion 6).

## Performance Metrics

**Velocity:**

- Total plans completed: 5
- Average duration: —
- Total execution time: —

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| 01 | 5 | - | - |

**Recent Trend:**

- Last 5 plans: —
- Trend: —

*Updated after each plan completion*
| Phase 02-gap-less-receipt-numbering-core P01 | 262s | 2 tasks | 8 files |
| Phase 02 P02 | 254 | 2 tasks | 4 files |
| Phase 02 P03 | 256 | 1 tasks | 2 files |
| Phase 02-gap-less-receipt-numbering-core P04 | 502 | 2 tasks | 4 files |
| Phase 03 P05 | 120 | 3 tasks | 7 files |

## Accumulated Context

### Decisions

Decisions are logged in PROJECT.md Key Decisions table.
Recent decisions affecting current work:

- Roadmap: Correctness-first ordering — gap-less numbering (Phase 2) is built and concurrency-proven BEFORE the approval/issuance flow (Phase 3) depends on it.
- Roadmap: Foundation (audit + RBAC + retention model) first — expensive to retrofit; everything depends on it.
- Roadmap: PDF + email decoupled behind an async outbox worker (Phase 4); kept out of the issuance transaction.
- Roadmap: Public donation web form (Flow B) built LAST (Phase 6) — reuses the entire Flow-A pipeline.
- [Phase ?]: Allocator.queries field is *db.Queries (concrete) not db.Querier — WithTx bind requires concrete type (02-PATTERNS Key Observation #1)
- [Phase ?]: allocator_test.go uses black-box package receiptno_test; Allocator/NewAllocator/AllocatedReceipt are exported
- [Phase 03]: Atomic 7-step Approve tx: lock→SoD→Allocate→Issue→Audit→Outbox in one WithTx closure — any failure rolls back all effects; receipt number exists only on issued records
- [Phase 03]: SoD enforced at code layer (approverID != createdBy) AND DB CHECK (chk_sod_approver) — defense-in-depth; both layers tested by integration tests
- [Phase 03]: Checker route group RequireRoles(Checker, Admin) at HTTP layer in addition to service-layer SoD guard — defense-in-depth over service layer
  - ⚠️ CORRECTION 2026-07-02: this decision encoded the RBAC AND-bug. `RequireRoles(...)` enforces AND (user must hold ALL listed roles), so `RequireRoles(Checker, Admin)` requires checker AND admin — a checker-only user is wrongly blocked. Intent was "checker OR admin". Same bug on donationGroup `RequireRoles(Maker,Checker,Admin)`. Fix: add `RequireAnyRole` (OR) and use it at both sites. (bug #3, OPEN)
- [Phase 03] 2026-07-02: Identity model clarified — `claims.Subject` (KC sub) ≠ `users.id` (surrogate PK). All `REFERENCES users(id)` writes must resolve sub→users.id via `auth.ResolveAppUser` middleware; `audit_log.actor_id` intentionally keeps raw sub (no FK). (bug #1 fix, committed ef7ede6)
- [Phase 03] 2026-07-02: FE↔BE auth requires a Keycloak Audience mapper putting `donnarec-backend` in token `aud`, and `donnarec-frontend` must be a confidential client (NextAuth server-side). (bug #2 fix, uncommitted)

### Pending Todos

Phase 3 integration-gate remediation (blocks marking Phase 3 Complete):

1. [x] Bug #1 `created-by-fk-mismatch` — resolve sub→users.id in `auth.ResolveAppUser` middleware. FIXED + committed (ef7ede6, refactor a1e348e).
2. [ ] Bug #2 `fe-be-audience-mismatch` — audience mapper + confidential frontend client + web env. Fixed in working tree (keycloak/realm-donnarec.json, donnarec-web/.env.example, donnarec-web/.env.local[gitignored]); COMMIT PENDING.
3. [ ] Bug #3 RBAC AND-bug — add `RequireAnyRole` (OR) + use at main.go:236 (donationGroup) and :270 (checkerGroup) + test. OPEN.
4. [ ] Add E2E HTTP integration test (real router + realistic token) covering Maker create/submit + Checker approve/return — satisfies integration gate (criterion 6) and guards regressions.
5. [ ] Re-run real-token curl through core endpoints until 2xx; then human UI walkthrough (5 items from 03-VERIFICATION human_verification).
6. [ ] Consider a wider auth/RBAC/wiring seam audit (option B) for any further latent seam bugs before re-closing Phase 3.

### Blockers/Concerns

Stakeholder confirmations are gated but NON-blocking for the roadmap; resolve at the relevant phase start:

- Phase 1: PDPA retention period (~5y vs erasure); email provider / KMS / hosting choices.
- Phase 4: §6 receipt wording + 1x/2x deduction eligibility (accounting/legal sign-off).
- Phase 5: exact e-Donation field spec + 1 Jan 2026 mandate obligation.

## Deferred Items

Items acknowledged and carried forward from previous milestone close:

| Category | Item | Status | Deferred At |
|----------|------|--------|-------------|
| *(none)* | | | |

## Session Continuity

Last session: 2026-06-30T18:40:11.574Z
Stopped at: Phase 3 UI-SPEC approved
Resume file: None
