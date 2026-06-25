# Roadmap: DonaRec — ระบบออกใบเสร็จบริจาคอัตโนมัติสำหรับโรงพยาบาล

## Overview

DonaRec is a compliance-critical, back-office document-issuance system whose single product is a legally valid, tax-deductible PDF donation receipt issued only after human approval. The journey is correctness-first: lay the foundation (DB schema, Auth/RBAC with Maker/Checker/Admin separation, append-only audit, retention model), then build and concurrency-prove the gap-less per-fiscal-year receipt-number allocator before anything depends on it. Next, the donation lifecycle and maker-checker issuance transaction wire onto the proven allocator; the slow PDF + email side-effects run behind an async outbox worker (with a Thai-rendering spike + golden-file test). Operational tooling (e-Donation export, reports, admin config, backup) follows, and the public donation web form (Flow B) is built last because it reuses the entire back-office approval pipeline.

## Phases

**Phase Numbering:**
- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [x] **Phase 1: Foundation (DB, Auth/RBAC, Audit, Retention model)** - Security, role separation, and an immutable audit trail that everything else depends on (completed 2026-06-24)
- [ ] **Phase 2: Gap-less Receipt Numbering Core (★)** - Concurrency-proven, per-fiscal-year, gap-less number allocator built before any issuance flow
- [ ] **Phase 3: Donation Lifecycle & Maker-Checker Issuance** - Donation records, encrypted donor PII, and the single approval transaction that issues a numbered receipt
- [ ] **Phase 4: Receipt PDF + Email Delivery (Outbox Worker)** - Async Thai/EN tax-compliant PDF and email pipeline with retry, decoupled from the issuance transaction
- [ ] **Phase 5: e-Donation Export, Reports & Admin Settings** - Access-controlled e-Donation export, donation reports, no-deploy config, and verified backup/restore
- [ ] **Phase 6: Public Donation Web Form (Flow B)** - Public bilingual donation form with slip upload, consent, bot protection, and pending-review queue feeding the existing pipeline

## Phase Details

### Phase 1: Foundation (DB, Auth/RBAC, Audit, Retention model)
**Goal**: Staff can log in under enforced Maker/Checker/Admin roles, every significant action is recorded in a tamper-proof audit trail, and the data model encodes PDPA-vs-tax retention from day one.
**Mode:** mvp
**Depends on**: Nothing (first phase)
**Requirements**: NFR-01, FR-34, NFR-02, NFR-05, FR-13, NFR-03
**Success Criteria** (what must be TRUE):
  1. A user can log in with a password stored only as an argon2id hash, and an admin can create users and assign exactly one of the Maker / Checker / Admin roles.
  2. Endpoints reject actions a user's role is not permitted to perform (RBAC enforced server-side), and the national/tax-ID column is masked by default for roles that may not see it.
  3. Every login, role change, and data action writes an immutable audit row (actor, action, timestamp, before→after); attempting to UPDATE or DELETE an audit row is denied at the database level.
  4. Each donor/receipt record carries a `retain_until` value and a documented legal-basis field, and no code path can hard-delete a record under legal hold.
  5. Data in transit is served over HTTPS/TLS and the sensitive-ID column is stored encrypted at rest (application-level envelope encryption), never in plaintext.
**Plans**: 5 plans
- [x] 01-01-PLAN.md — Walking skeleton: Keycloak OIDC auth + Go RBAC guard + sqlc/pgx data layer
- [x] 01-02-PLAN.md — Append-only audit log with SHA-256 hash-chain immutability
- [x] 01-03-PLAN.md — AES-256-GCM PII envelope encryption + retention/legal-hold model
- [x] 01-04-PLAN.md — Gap closure: fix docker-compose + Keycloak realm so local stack boots (5 UAT blockers)
- [x] 01-05-PLAN.md — Gap closure: migrate init-service for cold-start + configurable OIDC issuer (2 live infra gaps)

> Note: NFR-02 (HTTPS/TLS + encryption-at-rest) is split — the encryption boundary and transport are established here; full PII encrypt/decrypt/mask usage lands with the donor module in Phase 3. NFR-03 here covers the retention/legal-basis data model and policy; the donor-facing consent capture lands in Phase 3 (Flow A) and Phase 6 (Flow B).
> **Stakeholder gate (non-blocking):** confirm exact PDPA retention period (~5 years) and erasure policy with DPO/legal; email provider / KMS / hosting (on-prem vs cloud) decisions. Model `retain_until` generically until confirmed.

### Phase 2: Gap-less Receipt Numbering Core (★)
**Goal**: The system can allocate a unique, gap-less, per-fiscal-year receipt running number inside a single short DB transaction, and this invariant is proven under concurrency and rollback before any UI depends on it.
**Mode:** mvp
**Depends on**: Phase 1
**Requirements**: FR-15, FR-16, FR-17, FR-18, NFR-04
**Success Criteria** (what must be TRUE):
  1. Allocating a number produces a formatted receipt number = fiscal year + zero-padded running number (e.g. `2569/000123`), with separator and padding read from config.
  2. A single pure `fiscalYear(issueDate)` helper, pinned to Asia/Bangkok and Buddhist-era, returns the correct fiscal year at the 30 Sep 23:59 / 1 Oct 00:00 boundaries (Oct–Dec rolls to the next BE year), proven by unit tests.
  3. The running number resets to 1 automatically when a new fiscal year begins, because the counter is keyed per fiscal year (no scheduled reset job).
  4. A concurrency + rollback test running parallel allocations asserts zero gaps and zero duplicates, and a `UNIQUE(fiscal_year, running_no)` constraint backstops any logic bug.
  5. The allocator is the only code path that can hand out a number, and it never pre-computes or reserves a number on a draft.
**Plans**: TBD

> **Research flag:** verify the chosen ORM path for `UPDATE ... RETURNING` / `SELECT FOR UPDATE` and write the concurrency harness; this is the #1 correctness risk (Pitfalls 1 & 2).

### Phase 3: Donation Lifecycle & Maker-Checker Issuance
**Goal**: A Maker can create and submit a donation record with encrypted donor details, a Checker (who is never the Maker) can approve or return it with a reason, and approval issues a numbered receipt in one atomic transaction.
**Mode:** mvp
**Depends on**: Phase 2
**Requirements**: FR-07, FR-09, FR-11, FR-10, FR-12, FR-14, FR-19, FR-29
**Success Criteria** (what must be TRUE):
  1. A Maker can create a donation record (Flow A), edit it while in draft, view any attached slip, and submit it for review; the record moves through the explicit lifecycle draft → pending_review → issued / rejected / cancelled.
  2. A Checker can approve or return a pending record with a mandatory reason on return, and the server blocks a user from approving a record they created (segregation of duties enforced in code).
  3. On approval, a single DB transaction sets status to issued, allocates the gap-less number, writes the audit row, and enqueues the side-effect job — and a receipt number exists only for issued records (never on drafts/rejected).
  4. Cancelling an issued receipt sets status to "ยกเลิก" and retains its number (no gap, never deleted), with the action audited.
  5. Donor details (name, tax/national ID, address, email) are stored with the ID encrypted at rest and masked everywhere except authorized, audited reveals; staff can search/filter records by name, date range, status, and receipt number.
**Plans**: TBD
**UI hint**: yes

> Note: the issue transaction enqueues an outbox job here, but the worker that consumes it (PDF + email) is built in Phase 4. Consent capture for Flow A donors is recorded here against the Phase 1 retention model (NFR-03).

### Phase 4: Receipt PDF + Email Delivery (Outbox Worker)
**Goal**: After a receipt is issued, an async worker reliably renders a correct Thai/English tax-compliant PDF and emails it to the donor, with delivery status and retry, without ever blocking or rolling back the issuance transaction.
**Mode:** mvp
**Depends on**: Phase 3
**Requirements**: FR-20, FR-21, FR-22, FR-24, FR-23, FR-25, FR-26, FR-27, FR-28, NFR-07, FR-33, NFR-09
**Success Criteria** (what must be TRUE):
  1. An issued receipt produces a PDF from a configurable hospital template with letterhead/seal, watermark, and signature image, rendered in the donor's language (Thai or English).
  2. The Thai PDF renders stacked tone marks and mixed Thai+Latin (including Latin-leading) strings correctly, verified by a golden-file visual test in CI, and the PDF contains the §6 tax-deduction content (incl. the 1x/2x statement) sourced from config.
  3. The PDF and email run in a worker behind a transactional outbox — a job exists if and only if a receipt was issued — so approval returns fast (PDF+email within ~2–3s/receipt target, measured off the lock-critical path).
  4. The donor receives a bilingual email with the PDF attached; send status (success/failure) is recorded, failures are retryable, and resending never allocates a new number.
  5. When a donor has no email, staff can download the receipt PDF directly, and an admin can edit templates, watermark, signature, and number format without a deploy.
**Plans**: TBD
**UI hint**: yes

> **Research flag:** Thai-PDF rendering spike with worst-case Thai text required BEFORE locking the PDF library; also email deliverability setup (SPF/DKIM/DMARC).
> **Stakeholder gate (non-blocking):** confirm §6 receipt wording and this hospital's 1x vs 2x deduction eligibility with accounting/legal; keep wording as config so corrections are no-deploy.

### Phase 5: e-Donation Export, Reports & Admin Settings
**Goal**: Staff can export issued-receipt data for manual e-Donation keying, track what has been keyed against the monthly deadline, view donation summary reports, manage settings, and rely on verified backups.
**Mode:** mvp
**Depends on**: Phase 4
**Requirements**: FR-30, FR-31, FR-32, NFR-08
**Success Criteria** (what must be TRUE):
  1. Staff can generate an access-controlled Excel/CSV export of issued records mapped to e-Donation fields (13-digit ID, donation date, cash type); the export is download-logged and restricted by role.
  2. Each record can be flagged "คีย์เข้า e-Donation แล้ว" and an aging view surfaces unkeyed records against the 5th-of-next-month deadline to prevent late/dropped entries.
  3. Staff can view donation summary reports by date range and total amount.
  4. A backup runs on a regular schedule and a documented restore has been performed successfully (restore verified, not just configured).
**Plans**: TBD
**UI hint**: yes

> Note: admin settings UI for templates/signature/number-format is delivered in Phase 4 with the config store (FR-33/NFR-09); this phase adds reporting and export-specific admin views. e-Donation export is manual Excel/CSV this milestone — no direct RD API.
> **Stakeholder gate (non-blocking):** confirm exact e-Donation field spec and whether the hospital is mandated under the 1 Jan 2026 e-Donation rule (may expand scope toward direct integration in a later milestone).

### Phase 6: Public Donation Web Form (Flow B)
**Goal**: A donor can submit a bilingual donation request with a slip and PDPA consent through a public, bot-protected web form, which lands in a pending-review queue and flows through the exact same back-office approval and issuance pipeline.
**Mode:** mvp
**Depends on**: Phase 5
**Requirements**: FR-01, FR-02, FR-03, FR-06, FR-05, FR-04, FR-08, NFR-06
**Success Criteria** (what must be TRUE):
  1. A donor can fill in the public form (donor details + amount + donation date) in Thai or English and submit, creating a record in pending_review (not draft) in the same pipeline as Flow A.
  2. The donor can upload a slip (jpg/png/pdf) that is validated server-side by real file type (magic bytes) and size, and stored outside the webroot.
  3. PDPA consent (with timestamp and text version) is shown and recorded before submission, tied to the Phase 1 retention model.
  4. The public form is protected against spam/bots via rate limiting and CAPTCHA, and the donor receives an acknowledgement email stating the request was received (explicitly not yet a receipt).
  5. Staff see a "รอตรวจสอบ" pending-review queue of web submissions, and the back-office UI is responsive and usable in Thai/English on desktop and mobile.
**Plans**: TBD
**UI hint**: yes

> Note: NFR-06 (responsive + bilingual UI) is attached here as the natural completion point for full bilingual/responsive coverage across the now-complete UI surface.

## Progress

**Execution Order:**
Phases execute in numeric order: 1 → 2 → 3 → 4 → 5 → 6

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Foundation (DB, Auth/RBAC, Audit, Retention) | 5/5 | Complete   | 2026-06-25 |
| 2. Gap-less Receipt Numbering Core | 0/TBD | Not started | - |
| 3. Donation Lifecycle & Maker-Checker Issuance | 0/TBD | Not started | - |
| 4. Receipt PDF + Email Delivery (Outbox Worker) | 0/TBD | Not started | - |
| 5. e-Donation Export, Reports & Admin Settings | 0/TBD | Not started | - |
| 6. Public Donation Web Form (Flow B) | 0/TBD | Not started | - |
