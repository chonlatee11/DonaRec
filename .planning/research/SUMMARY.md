# Project Research Summary

**Project:** DonaRec — ระบบออกใบเสร็จบริจาคอัตโนมัติสำหรับโรงพยาบาล (Thai hospital donation e-receipt system)
**Domain:** Compliance-critical back-office document-issuance system (maker-checker approval + gap-less fiscal-year receipt numbering + server-side Thai PDF + transactional email + PDPA-grade PII handling)
**Researched:** 2026-06-22
**Confidence:** HIGH on the load-bearing decisions; MEDIUM on framework choice and a small set of stakeholder-gated policy/legal items

## Executive Summary

DonaRec is **not** a payment or donation-collection platform — it is a back-office system whose single product is a *legally valid, tax-deductible PDF receipt issued only after human approval*. Experts build this class of system as a **modular monolith with one async worker** for slow side-effects (PDF render + email). The domain is one bounded context, volume is low (hundreds–low-thousands of receipts/year for a hospital), and the hardest requirement — gap-less, concurrency-safe receipt numbering inside a single DB transaction — is *easiest and safest* when everything shares one PostgreSQL database. Microservices would add distributed-transaction risk for zero benefit. The recommended stack is **TypeScript + NestJS 11 + PostgreSQL 17 + Prisma (with raw SQL for the counter) + headless-Chromium (Playwright) HTML→PDF + a transactional-outbox worker (BullMQ/Redis or a Postgres job table) + managed transactional email (SES/Postmark)**, with application-level AES-256-GCM envelope encryption for the national/tax ID.

This domain carries an unusually high "correctness tax": a duplicate or skipped receipt number, a malformed Thai tone mark on a legal document, or the erasure of a record the Revenue Code requires you to keep are audit/legal failures, not cosmetic bugs. The **#1 correctness risk is gap-less per-fiscal-year numbering** — it must be allocated by an atomic counter table (keyed by fiscal year, `UPDATE ... RETURNING` row-lock) *inside the same transaction* that flips the record to "issued," and it must be concurrency-tested early before any UI depends on it. The **#2 risk is Thai PDF rendering**: pure-JS PDF builders (pdf-lib/jsPDF/pdfkit) and wkhtmltopdf have documented Thai vowel/tone-mark shaping bugs, so a real browser/HarfBuzz engine plus an embedded Thai font (TH Sarabun New / `fonts-thai-tlwg`) and a golden-file visual test are mandatory — do a rendering spike before locking the PDF library.

The remaining load-bearing conclusions are organizational/compliance: enforce **maker-checker with server-side no-self-approval** and an **append-only, immutable audit trail** from day one (both expensive to retrofit); treat the **PDPA right-to-erasure vs ~5-year tax-retention conflict** as a policy-gated workflow (no hard delete — model `retain_until` + legal basis, prefer anonymization) pending DPO confirmation; keep **e-Donation as manual Excel/CSV export this phase** but flag the 2026 e-Donation mandate trend and the monthly (by the 5th) submission deadline as a compliance watch-item. Several decisions are **blocked on stakeholder confirmation** and should be resolved at phase start: exact receipt wording incl. 1x vs 2x deduction eligibility, the legal retention period, the e-Donation field spec, and email provider / KMS / hosting choices.

## Key Findings

### Recommended Stack

A single TypeScript stack spans the back-office UI, API, and PDF service, minimizing context-switching while keeping the correctness-critical pieces in PostgreSQL where transactional guarantees are strongest. The two load-bearing decisions are independent of framework taste and transfer directly to a Spring Boot (Java) or Django (Python) variant if hospital IT mandates a different stack. See `STACK.md` for full rationale, alternatives, and a "What NOT to Use" list.

**Core technologies:**
- **PostgreSQL 17**: primary store — row-level locking + transactional commit semantics make gap-less numbering, multi-row approval transactions, and immutable audit cleanly solvable (relational, not document, DB)
- **TypeScript + NestJS 11**: backend framework — guards make RBAC and maker-checker segregation a first-class concern; one language across UI/API/PDF
- **Playwright (headless Chromium) HTML→PDF**: the only reliably correct way to render Thai complex-script (tone-mark/vowel stacking); watermark + signature become trivial CSS/img
- **Prisma 6 + raw `$queryRaw` for the counter**: type-safe CRUD everywhere, with a tightly-contained raw-SQL escape hatch for the locked counter (Prisma has no native `SELECT FOR UPDATE`; TypeORM/Drizzle are the alternative if native pessimistic locking is preferred)
- **Application-level AES-256-GCM envelope encryption (KMS-wrapped DEK)** for national/tax ID — so DB admins/backups never see plaintext (decisive over `pgcrypto`); blind-index HMAC column if lookup-by-ID is needed
- **Transactional outbox + worker (BullMQ/Redis, or a Postgres job table with `FOR UPDATE SKIP LOCKED` for MVP)**: decouples PDF/email from the issuance transaction; guarantees "receipt issued ⇔ job exists"
- **Managed transactional email (Amazon SES `ap-southeast-1` or Postmark)**: PDF attachments, delivery/bounce webhooks, far better deliverability than self-hosted SMTP
- **TH Sarabun New + `fonts-thai-tlwg` in the container**: government/tax-document standard Thai font; without it Chromium renders tofu boxes
- **argon2id, CASL, ExcelJS, file-type (magic-byte upload validation), nestjs-i18n, append-only audit table**

### Expected Features

This is a back-office document-issuance system; the bar for "table stakes" is high because correctness + auditability *are* the product. Scope this phase = **cash donations only, Flow A (staff-created) back office**. See `FEATURES.md` for the full FR/NFR mapping and prioritization matrix.

**Must have (table stakes — Phase 1):**
- Auth + RBAC with distinct Maker/Checker/Admin and **server-enforced no-self-approval** (NFR-01, FR-34)
- Status lifecycle state machine + approve/return-with-reason (FR-11, FR-12)
- **Receipt created only on approval** + **gap-less, concurrency-safe, per-fiscal-year numbering** + FY auto-detect + cancel-not-delete (FR-14/15/16/17/18/19, NFR-04)
- Tax-compliant PDF (template + watermark + signature image + §6 content incl. 1x/2x statement) (FR-20/21/22/24)
- Email receipt + send-status/resend + manual download fallback (FR-25/27/28)
- Encrypted donor store + PDPA consent record (FR-29, NFR-02/03)
- e-Donation Excel/CSV export (FR-30), append-only audit trail (FR-13/NFR-05), search/filter (FR-10), backup/restore (NFR-08)

**Should have (add after validation):**
- "Keyed into e-Donation" status flag (FR-31) — cheap, high operational value against the monthly deadline; pull forward if easy
- Donation summary reports (FR-32); admin-configurable templates/number format (FR-33, NFR-09); bilingual PDF + email (FR-23/26)

**Defer (Phase 2+):**
- Public donor web form (Flow B) + slip upload + consent + bot protection (FR-01–05); pending-queue dashboard (FR-08); full bilingual UI (NFR-06)
- Reissue/duplicate-on-request (policy decision first); general non-donation receipts, deep reports, auto e-Donation/accounting integration (Phase 3)

**Explicit anti-features (do NOT build):** in-kind receipts, payment gateway, direct e-Donation API, auto-issue without approval, hard-delete, PKI digital signature (image signature for MVP).

### Architecture Approach

A **modular monolith organized by domain module** (not technical layer), with a single async worker for PDF + email behind a **transactional outbox**. Modules own their routes, services, data access, and tests so the two critical boundaries — the gap-less number allocator and the PII encryption boundary — are explicit and impossible to bypass accidentally. The issuance transaction is short (status→issued + number allocation + outbox job + audit row, all in one commit); all slow I/O runs afterward in the worker. See `ARCHITECTURE.md` for the full component diagram, build order, and anti-patterns.

**Major components:**
1. **Receipt-number module (★)** — gap-less per-fiscal-year allocator; the *only* path to a number is its `UPDATE ... RETURNING` allocator inside the issue transaction
2. **Approval/workflow** — maker submit, checker approve/reject, server-side SoD; orchestrates the single issue transaction
3. **Donation core + state machine** — `donation_record` aggregate, canonical transition table, search/filter
4. **Donor/PII module** — sole owner of plaintext national/tax ID; encrypts at rest, masks everywhere, audits every reveal/export
5. **Audit log** — append-only, immutable (revoke UPDATE/DELETE grants), written by every module
6. **Job outbox + worker** — PDF render (template/watermark/signature/lang) + email send + delivery status + retry
7. **Config/template store** — templates, watermark, signature, number format editable without deploy (NFR-09)

### Critical Pitfalls

1. **Gap-less numbering treated as a DB sequence / `MAX+1` / pre-computed on the draft** — sequences leave gaps on rollback by design; `MAX+1` duplicates under concurrency. Use a fiscal-year-keyed counter table with `UPDATE ... RETURNING` inside the issue transaction, a `UNIQUE(fiscal_year, running_no)` backstop, and a **concurrency + rollback test** as the first thing built.
2. **Wrong fiscal-year derivation at the Oct–Dec boundary / Buddhist-era off-by-one** — Thai FY is 1 Oct–30 Sep; Oct–Dec rolls to the next BE year (+543). Centralize one pure `fiscalYear(issueDate)` function pinned to `Asia/Bangkok`, unit-tested at 30 Sep 23:59 / 1 Oct 00:00 boundaries; FY-keyed counter auto-resets to 1.
3. **Thai tone marks broken in the PDF** — pure-JS libs and wkhtmltopdf misplace stacked marks (esp. strings starting with a Latin char). Use a HarfBuzz/Chromium renderer, embed a Thai font (incl. Latin glyphs), and add a **golden-file visual test** in CI. Do a rendering spike before locking the PDF lib.
4. **Approval workflow without true SoD or an immutable audit trail** — enforce approver ≠ creator in code; audit append-only (DB grants revoke UPDATE/DELETE); cancel = status change, never delete (FR-19). Foundational, expensive to retrofit.
5. **PDF/email coupled to the issuance transaction** — holds the per-year lock during slow I/O, and a transient email failure can roll back issuance. Commit number+record first (short tx), then render+email async with retry; resend must not re-number.
6. **PDPA erasure vs tax retention handled as "just delete it" / sensitive ID stored insecurely** — model `retain_until` + legal basis, prefer anonymization, never hard-delete; encrypt national ID at rest, mask by default, keep it out of logs, access-control the export file. Confirm the retention period with DPO/legal.
7. **Receipt legal content assumed instead of confirmed** — treat §6 wording and the 1x/2x deduction statement as regulated content needing accounting/legal sign-off; make it config (NFR-09); verify this hospital's 2x eligibility and its e-Donation obligation under the 2026 mandate.

## Implications for Roadmap

The architecture's build order and the pitfalls' phase mapping agree: build the **foundation and the riskiest correctness pieces first**, prove them under concurrency *before* layering UI, and defer the public web form (Flow B) to last because it reuses the entire Flow-A pipeline. Suggested phase structure:

### Phase 0: Foundation (DB, Auth/RBAC, Audit)
**Rationale:** RBAC gates everything and the append-only audit log is a dependency of literally every later component — retrofitting audit risks gaps in the trail (NFR-05).
**Delivers:** DB schema + migrations, container with `fonts-thai-tlwg`, auth (argon2id + JWT), RBAC (CASL: Maker/Checker/Admin), append-only audit writer (UPDATE/DELETE revoked), `retain_until`/legal-hold data model.
**Addresses:** NFR-01, FR-34, FR-13, NFR-05; data-model side of PDPA retention.
**Avoids:** Pitfalls 5 (SoD/audit) and 3-retention-model groundwork.

### Phase 1: Numbering core + concurrency proof (★)
**Rationale:** The single highest-risk invariant; the issue transaction cannot be wired until the allocator exists and is *proven* concurrency-safe.
**Delivers:** `receipt_counter` table, the one `fiscalYear(issueDate)` helper (Asia/Bangkok, BE), `UPDATE ... RETURNING` allocator, `UNIQUE(fiscal_year, running_no)` constraint, and a **concurrency + rollback test asserting zero gaps/dupes**.
**Addresses:** FR-15/16/17/18/19, NFR-04.
**Avoids:** Pitfalls 1 and 2.

### Phase 2: Donation core + maker-checker issuance
**Rationale:** Wires the state machine and the single issue transaction (status→issued + number + outbox + audit) on top of the proven allocator; PII module needed to store donors.
**Delivers:** donation_record + state machine, create/edit/view-slip, search/filter, maker submit, checker approve/reject with reason + server-side SoD, encrypted Donor/PII module with masking, PDPA consent capture.
**Addresses:** FR-07/09/10/11/12/14/29, NFR-02/03.
**Avoids:** Pitfalls 5 and 8.

### Phase 3: Issuance side-effects — PDF + email (outbox worker)
**Rationale:** Decouples slow I/O from the issuance transaction; config store must land alongside so templates and number format are configurable from day one. **Do a Thai-PDF rendering spike before locking the library.**
**Delivers:** transactional-outbox worker, PDF pipeline (template/watermark/signature/Thai+EN) with golden-file visual test, email pipeline (2-lang, delivery status, retry, idempotent resend), manual download fallback, config/template store.
**Uses:** Playwright + TH Sarabun New, BullMQ/Postgres job table, SES/Postmark.
**Implements:** outbox + worker + config components.
**Addresses:** FR-20/21/22/23/24/25/26/27/28, FR-33, NFR-07/09.
**Avoids:** Pitfalls 3, 4 (wording), 7.

### Phase 4: e-Donation export + reports + admin
**Rationale:** Needs issued records + donor PII; operationally time-critical against the monthly (by the 5th) deadline.
**Delivers:** access-controlled Excel/CSV export mapped to e-Donation fields, "keyed" status + aging view (FR-31), donation summary reports (FR-32), admin settings UI, backup/restore verification.
**Addresses:** FR-30/31/32, NFR-08.
**Avoids:** Pitfall 6 (e-Donation field mapping + deadline).

### Phase 5: Public donation form (Flow B) — Phase 2 per requirements
**Rationale:** Reuses the entire Flow-A pipeline (creates records in `pending_review` instead of `draft`); should not be built until that pipeline is solid.
**Delivers:** public form, slip upload with magic-byte/size validation, consent capture, rate-limit + CAPTCHA, acknowledgement email, pending-queue dashboard, fuller bilingual UI.
**Addresses:** FR-01/02/03/04/05/06/08, NFR-06.

### Phase Ordering Rationale
- **Correctness before UI:** numbering (Phase 1) precedes approval (Phase 2) because the issue transaction depends on a proven allocator; architecture build order makes this an explicit constraint.
- **Foundation first:** audit + RBAC + retention model (Phase 0) are dependencies of everything and painful to retrofit.
- **Slow path decoupled:** PDF/email (Phase 3) live behind an outbox so the locked issuance transaction stays short (Pitfall 7, NFR-07).
- **Flow B last:** it is additive on top of the maker-checker engine, matching the requirements' explicit Phase-1-then-Phase-2 phasing.

### Research Flags

Phases likely needing deeper research / a spike during planning:
- **Phase 1 (numbering):** verify the chosen ORM path for `SELECT FOR UPDATE` / `UPDATE ... RETURNING` and write the concurrency harness — highest-risk invariant.
- **Phase 3 (PDF):** **rendering spike with real worst-case Thai text required before locking the PDF library** (stacked tone marks, Latin-leading strings, Thai-word amounts); also email deliverability setup (SPF/DKIM/DMARC).
- **Phase 4 (e-Donation export):** confirm exact e-Donation field spec + monthly deadline mechanics; verify hospital's obligation under the 2026 mandate (may expand scope toward direct integration later).

Phases with standard patterns (can skip deep research):
- **Phase 0 (auth/RBAC/audit):** well-documented NestJS guards + CASL + append-only audit patterns.
- **Phase 5 (public form):** standard form + upload-validation + rate-limit/CAPTCHA patterns once the core pipeline exists.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH (framework: MEDIUM) | Load-bearing decisions (PostgreSQL, gap-less counter, Chromium Thai PDF, app-level encryption) verified against official docs + multiple sources. NestJS is recommended but Spring Boot/Django are defensible if hospital IT dictates. |
| Features | HIGH | Mapped 1:1 to the requirements doc + verified Thai Revenue Department e-Donation sources + maker-checker industry patterns. |
| Architecture | HIGH | Numbering concurrency design verified against PostgreSQL docs + expert sources; component structure is standard maker-checker/document-workflow. |
| Pitfalls | HIGH (receipt wording: MEDIUM) | Gap-less, PDPA-vs-retention, Thai PDF, email deliverability, e-Donation rules all multi-source verified; specific legal receipt wording must be confirmed with hospital accounting/legal. |

**Overall confidence:** HIGH

### Gaps to Address (stakeholder-gated — resolve at the relevant phase start)
- **Receipt wording + 1x vs 2x deduction eligibility** (§6, FR-24): confirm this hospital's category (public/state hospitals qualify for 2x, capped at 10% of net income; private generally do not) and exact legal wording with accounting/legal before building the PDF template (Phase 3).
- **Retention period for PDPA-vs-tax conflict**: confirm the exact retention period (commonly ~5 years) and erasure policy with DPO/legal; model `retain_until` + legal basis now, no hard delete (Phase 0 model, Phase 5 donor request UI).
- **e-Donation field spec + 2026 mandate obligation**: confirm required export fields, the monthly/by-the-5th deadline mechanics, and whether this hospital is exempt or mandated under the 1 Jan 2026 rule (Phase 4).
- **Email provider / KMS / hosting**: SES vs Postmark, which KMS holds the KEK, and on-prem vs cloud — driven by hospital procurement + PDPA data-residency policy (Phase 0/3 ops decisions).
- **JVM constraint?**: if hospital IT mandates Java/.NET, switch to the Spring Boot + iText/pdfCalligraph variant — the gap-less and Thai-PDF reasoning transfer directly.
- **PKI signing (Open Issue #1)**: image signature for MVP; if legal weight is later required, plan the PDF pipeline so a signing stage can be inserted post-render.

## Sources

### Primary (HIGH confidence)
- PostgreSQL official docs — sequences are non-gap-less by design; encryption options / pgcrypto — postgresql.org
- CYBERTEC — gaps in sequences + gapless counter-table + `UPDATE ... RETURNING` / `FOR UPDATE` pattern — cybertec-postgresql.com
- pdf-lib #675 Thai shaping bug; wkhtmltopdf Thai issues; Puppeteer/`fonts-thai-tlwg`; Fonts-TLWG — github.com / pptr.dev / linux.thai.net
- Thai Revenue Department — 2x-eligible hospital list (rd.go.th/27811.html) + e-Donation user manual (monthly/by-5th deadline, batch Excel template)
- PwC Worldwide Tax Summaries — Thailand deductions (public-hospital 2x, 10% cap, cash-only)
- NestJS 11 docs (authorization/RBAC + CASL); Prisma docs (no native `SELECT FOR UPDATE`, interactive transactions)
- OneTrust / DLA Piper / Norton Rose Fulbright — Thai PDPA (right to erasure not absolute; legal-obligation override)
- Project requirements `requirements-ระบบออกใบเสร็จบริจาค.md` v1.1 + `.planning/PROJECT.md`

### Secondary (MEDIUM confidence)
- HTML→PDF benchmark (Playwright vs Puppeteer); transactional-email comparison (SES/Postmark attachments, webhooks, deliverability)
- Crunchy Data / MoldStud — PostgreSQL app-level AES-256-GCM vs pgcrypto, envelope encryption, blind index
- Bangkok Global Law / Forvis Mazars — e-Donation 2026 mandate + system overview
- Maker-checker / four-eyes / SoD + audit-trail industry patterns
- Framework comparison (NestJS / Django / Spring Boot for enterprise RBAC + PDPA)

### Tertiary (LOW confidence / needs validation via spike or stakeholder)
- WeasyPrint/HarfBuzz Thai shaping superiority — verify with a rendering spike (Phase 3)
- This specific hospital's 2x eligibility, retention period, and e-Donation obligation — confirm with hospital legal/accounting/DPO

---
*Research completed: 2026-06-22*
*Ready for roadmap: yes*
