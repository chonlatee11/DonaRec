# Project Retrospective

*A living document updated after each milestone. Lessons feed forward into future planning.*

## Milestone: v1.0 — MVP

**Shipped:** 2026-07-17
**Phases:** 7 | **Plans:** 47 | **Tasks:** 91 | **Timeline:** 25 days (2026-06-22 → 2026-07-17) | **Commits:** 418

### What Was Built
- Gap-less, concurrency-safe, per-fiscal-year receipt-number allocator (counter table + `SELECT … FOR UPDATE`) — proven zero-gap/zero-duplicate under 50 parallel allocations + rollback + `UNIQUE` backstop, under `-race`.
- Full donation lifecycle with Maker-Checker Segregation of Duties (app guard + DB CHECK) and a single atomic approval transaction (issue + number + audit + outbox enqueue).
- Async PDF + email pipeline behind a transactional outbox worker: sandboxed headless Chromium renders Thai/EN tax-compliant receipts (golden-file for stacked tone marks), bilingual email with retry/dead-letter, off the issuance critical path.
- Security/PDPA foundation: Keycloak OIDC + RBAC, SHA-256 hash-chained append-only audit log (REVOKE UPDATE/DELETE), AES-256-GCM envelope encryption for PII with role-gated audited reveal.
- Operational surface: audited e-Donation xlsx/csv export, keyed-flag + aging, PII-free reports, no-deploy admin config, verified backup/restore.
- Public bilingual Flow B donation form (Turnstile + rate-limit + magic-byte slip + PDPA consent) feeding the exact same back-office approval pipeline.

### What Worked
- **Correctness-first phasing** — building and concurrency-proving the gap-less allocator (Phase 2) BEFORE any issuance flow depended on it meant the highest-risk invariant was locked early and never regressed.
- **Reusing one approval pipeline for both flows** — Flow B (Phase 6) added a public entry point but reused Phase 3's issuance transaction unchanged; the composite E2E (Phase 7) confirmed the handoff is source-agnostic.
- **Testcontainers + real-stack integration tests** — driving real PostgreSQL/Keycloak surfaced seam bugs that unit tests structurally could not.

### What Was Inefficient
- **Phase 3 shipped 3 runtime-seam bugs** (created-by FK mismatch, FE↔BE audience mismatch, RBAC AND-vs-OR) after passing 5/5 unit-level verification — rework that a real-HTTP+token integration test would have caught first time. This directly motivated the Integration-test gate convention.
- **Frontend auth-gating layer was latent-missing** for several phases (no middleware, 404 sign-in page) — discovered only at the Phase 3 UI walkthrough, not when the UI was first built.
- **Deploy-time verifications (TLS) and interactive-login browser checks** remained open at milestone close as non-blocking overrides — inherent to a local-only build, but worth planning a deploy verification phase for.

### Patterns Established
- **Integration-test gate** (CONVENTIONS.md): a phase touching the runtime request seam is not "done" without an E2E test over the real HTTP → auth → RBAC → handler → service → DB path with a realistic Keycloak token.
- **Caller-managed transaction boundary** for number allocation — `Allocate(ctx, tx, issueDate)` never opens its own tx, so the single approval transaction stays atomic.
- **Transactional outbox + worker** for all slow side-effects (PDF, email, ack email) — a job exists iff the state change committed; approval returns fast.
- **Gap-closure phases** (01-04/05, 05-08, 07) — small inserted phases to close a specific verified gap rather than reopening a large phase.

### Key Lessons
1. Unit/service tests that construct claims and user rows by hand cannot catch runtime-seam defects (audience, identity, route-guard wiring) — drive the real stack with a real token.
2. Build the auth-gating/route-protection layer as an explicit deliverable, not an assumed byproduct of wiring pages — its absence is silent until someone tries to log in.
3. For gap-less numbering, never `nextval()`/pre-compute — allocate inside the commit with a row lock, and prove it under `-race` with a `UNIQUE` backstop.
4. Thai-script PDF correctness requires a browser engine (chromedp) + `fonts-thai-tlwg`; pure-Go PDF libs mis-stack tone marks — lock this with a golden-file test.

### Cost Observations
- Model mix / session count: not tracked this milestone.
- Notable: heavy use of testcontainers-backed integration tests (real Postgres/Keycloak/MinIO/Chromium) paid for itself by catching seam bugs pre-ship.

---

## Cross-Milestone Trends

### Process Evolution

| Milestone | Phases | Plans | Key Change |
|-----------|--------|-------|------------|
| v1.0 | 7 | 47 | Established the Integration-test gate after Phase 3 seam bugs; adopted gap-closure phases |

### Cumulative Quality

| Milestone | Notable Invariants Proven | Deferred (non-blocking) |
|-----------|---------------------------|-------------------------|
| v1.0 | gap-less numbering under -race; SoD (app+DB); audit immutability; Thai PDF golden-file | HTTPS/TLS (deploy-time); interactive-login browser walkthrough |

### Top Lessons (Verified Across Milestones)

1. Drive the real runtime seam with a real token — unit tests miss integration defects. (v1.0)
