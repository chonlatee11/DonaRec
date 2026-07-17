# Milestones

## v1.0 MVP (Shipped: 2026-07-17)

**Phases completed:** 7 phases, 47 plans, 91 tasks
**Closeout type:** override_closeout (2 non-blocking verification overrides — see below)

**Key accomplishments:**

- **Foundation (Phase 1):** Keycloak OIDC auth + Go RBAC (Maker/Checker/Admin), SHA-256 hash-chained append-only `audit_log` (REVOKE UPDATE/DELETE at DB level), and AES-256-GCM envelope encryption for national/tax ID with role-gated, audited PII reveal + retention/legal-hold model.
- **Gap-less receipt numbering ★ (Phase 2):** per-fiscal-year (Asia/Bangkok, BE) counter allocated inside the issuance tx via `SELECT … FOR UPDATE` — proven zero-gap / zero-duplicate under 50 parallel allocations + rollback + `UNIQUE` backstop, all under `-race`.
- **Donation lifecycle & Maker-Checker issuance (Phase 3):** single atomic approval transaction (status→issued + allocate number + audit row + enqueue outbox), Segregation-of-Duties (approver ≠ creator) enforced in code AND DB CHECK, cancel/void-reissue retains the number (no gap). Integration-test gate met (E2E + human walkthrough 7/7).
- **Receipt PDF + email (Phase 4):** sandboxed headless Chromium renders Thai/EN tax-compliant PDF (golden-file for stacked tone marks; JS/network disabled), transactional outbox worker (`FOR UPDATE SKIP LOCKED`) freezes the PDF + emails bilingually with retry/dead-letter — off the issuance critical path (NFR-07). No-deploy admin template/number-format config.
- **e-Donation export, reports & admin (Phase 5):** audited, RBAC-gated, stream-only xlsx/csv export mapped to e-Donation fields; keyed-flag + Bangkok-aware 3-bucket aging vs the monthly deadline; PII-free summary reports; verified pg_dump + MinIO backup/restore (restore proven, not just configured).
- **Public donation web form — Flow B (Phase 6):** bilingual, bot-protected (Turnstile fail-closed + per-IP rate limit) public form → `pending_review` (source=flow_b) → the exact same approval pipeline; magic-byte slip validation, PDPA consent snapshot, bilingual "received — not yet a receipt" ack email. Responsive mobile nav + th/en i18n across UI/PDF/email.
- **Composite E2E lock (Phase 7):** `TestE2E_FlowBCompositePublicSubmitToIssued` — first automated test spanning the full public-submit → Checker-approve → issued gap-less receipt handoff. Closes v1.0 audit WARNING-1.

**Known Verification Overrides (2 — non-blocking, deferred at close):**

- **Frontend auth-gating interactive login** — route-protection layer (middleware `withAuth` + sign-in page + SessionProvider) is code-complete and TDD-green (live curl: 307 redirect + PKCE handoff verified). The final interactive Keycloak browser login remains a human checkpoint (cannot be scripted headlessly). Debug session `frontend-auth-gating-missing` status `awaiting_human_verify`.
- **HTTPS/TLS (NFR-02)** — deploy-time verification only: reverse-proxy TLS termination + `sslmode=verify-full` to be confirmed at production deploy. The app-level encryption boundary (AES-256-GCM envelope) is complete. Phase 01 HUMAN-UAT Test 2 `blocked_by: deploy-time`.

_(Phase 04 UAT is already `passed` 2/2 — surfaced by the audit query but not an override.)_
See STATE.md → **Deferred Items**. Full audit: `milestones/v1.0-MILESTONE-AUDIT.md`.

---
