# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

**สำคัญ: ตอบกลับเป็นภาษาไทยเท่านั้น**
**สำคัญ: ตั้งคำถามเป็นภาษาไทยเท่านั้น**

---

<!-- GSD:project-start source:PROJECT.md -->
## Project

**DonaRec — ระบบออกใบเสร็จบริจาคอัตโนมัติสำหรับโรงพยาบาล**

ระบบเว็บสำหรับโรงพยาบาลในการออกใบเสร็จรับเงินบริจาคในรูปแบบ PDF ที่ถูกต้องตามข้อกำหนดการลดหย่อนภาษี
ส่งให้ผู้บริจาคทางอีเมลอัตโนมัติ และจัดเก็บข้อมูลผู้บริจาคอย่างเป็นระบบเพื่อรองรับการคีย์เข้าระบบ
e-Donation ของกรมสรรพากร โดยทุกใบเสร็จต้องผ่านการตรวจสอบและอนุมัติโดยเจ้าหน้าที่ก่อนออกเสมอ
มาแทนกระบวนการแมนวลที่ใช้อยู่ในปัจจุบัน

ผู้ใช้งานหลัก: เจ้าหน้าที่โรงพยาบาล (Maker), ผู้อนุมัติ (Checker), ผู้ดูแลระบบ (Admin) และผู้บริจาค (ในเฟสเว็บสาธารณะ)

**Core Value:** ออกใบเสร็จบริจาคที่มี **เลขที่รันต่อเนื่องไม่ซ้ำ ห้ามข้ามเลข (gap-less) ตามปีงบประมาณ** หลังผ่านการอนุมัติโดยมนุษย์
และส่งถึงผู้บริจาคได้อย่างถูกต้องน่าเชื่อถือ — ความถูกต้องของเลขใบเสร็จและการควบคุมการอนุมัติคือหัวใจที่ห้ามพลาด

### Constraints

- **Security/PDPA**: เข้ารหัสข้อมูลอ่อนไหวขณะจัดเก็บ + HTTPS/TLS + จำกัดการเห็นเลขบัตร ปชช. ตามบทบาท — ข้อกำหนดทางกฎหมาย
- **Correctness**: เลขที่ใบเสร็จต้อง gap-less และ concurrency-safe — ต้องออกเลขในจังหวะ commit ของ DB (sequence/atomic counter ระดับ transaction) ไม่ใช่คำนวณล่วงหน้า
- **Compliance**: รูปแบบ/ข้อความใบเสร็จต้องถูกต้องตามประมวลรัษฎากร (รวมเงื่อนไขลดหย่อน 1 เท่า/2 เท่า) — ต้องยืนยันกับฝ่ายบัญชี/กฎหมายโรงพยาบาล
- **Audit**: ทุกการกระทำสำคัญต้องมี audit trail ที่ลบไม่ได้ เพื่อการตรวจสอบบัญชี/ภาษี
- **i18n**: รองรับไทย/อังกฤษ ทั้ง UI, PDF และอีเมล
- **Integration**: ไม่มี payment gateway / ไม่ต่อ API e-Donation โดยตรง — export แมนวลเท่านั้น
- **Maintainability**: เทมเพลต/ลายเซ็น/รูปแบบเลข ตั้งค่าได้โดยไม่ต้อง deploy
<!-- GSD:project-end -->

<!-- GSD:stack-start source:research/STACK.md -->
## Technology Stack

## TL;DR — The Recommended Stack
| Concern | Recommendation | One-line why |
|---------|----------------|--------------|
| Language/Framework | **TypeScript + NestJS 11** | Enterprise structure (DI, modules, guards), first-class RBAC via guards, one language across back-office UI + API + PDF service |
| Database | **PostgreSQL 17** | Transactional integrity for gap-less numbering; `pgcrypto`, strong row-locking, mature backup/PITR |
| Query layer | **Prisma 6** for app CRUD **+ raw SQL (`$queryRaw … FOR UPDATE`) inside an interactive transaction for the counter** | Prisma gives type-safe productivity; the counter needs raw pessimistic lock because Prisma has no native `SELECT FOR UPDATE` |
| PDF generation | **Playwright (headless Chromium) rendering an HTML/CSS template** | The browser engine is the only approach that renders Thai vowel/tone-mark stacking *correctly* with zero shaping bugs; watermark + signature = plain CSS/img |
| Thai font | **TH Sarabun New** (or Sarabun from Google Fonts) embedded via `@font-face`, plus OS package `fonts-thai-tlwg` in the container | Government/tax document standard font in Thailand; container needs the Thai font package or you get tofu boxes |
| Email | **Amazon SES** (region `ap-southeast-1` Singapore) via SDK, OR **Postmark** if deliverability-managed is preferred | PDF attachment support, delivery/bounce webhooks, data residency near TH |
| Auth | **Passport (JWT) + argon2 password hashing + NestJS guards** | Standard, well-supported; argon2 is current best-practice hashing |
| RBAC | **NestJS guards + CASL** | Maker/Checker/Admin roles with attribute-level rules (e.g. "Checker cannot approve own Maker record") |
| PII encryption-at-rest | **Application-level AES-256-GCM (envelope encryption)** for national ID / tax ID, key in a KMS/secrets manager; plus disk/volume encryption as baseline | PDPA: DB admins must not see plaintext national IDs → app-level beats `pgcrypto` for this field |
| File uploads | **Local/S3-compatible object storage + `file-type` (magic-byte) validation + size limit + Multer** | Validate by content not extension; keep slip images out of the DB |
| Excel/CSV export | **ExcelJS** (xlsx) / built-in CSV | e-Donation manual keying needs real .xlsx with Thai text |
| i18n | **i18next** (`nestjs-i18n` on the server, `react-i18next` / `next-intl` on the front-end) | One catalog format drives UI + PDF + email templates |
| Audit log | **Append-only PostgreSQL table** + DB triggers / revoke UPDATE/DELETE; optionally hash-chain rows | Immutable trail for tax/accounting audit |
## Recommended Stack — Detail
### Core Technologies
| Technology | Version | Purpose | Why Recommended |
|------------|---------|---------|-----------------|
| TypeScript | 5.x | Language | Single language across API, PDF service, and the React back-office; strong typing reduces correctness bugs in a system where correctness is the headline requirement |
| NestJS | 11.1.x (11.x line, released Jan 2025) | Backend framework | Opinionated enterprise architecture (modules/controllers/services + DI) mirroring Spring; **guards** make RBAC and Maker/Checker separation a first-class concern; structured logging built in (NestJS 11 JSON logger) for audit/observability |
| PostgreSQL | 17 (16+ acceptable) | Primary relational DB | **The gap-less, concurrency-safe receipt sequence (NFR-04/FR-16) is the single hardest requirement and PostgreSQL's row-level locking + transactional commit semantics solve it cleanly.** Also: `pgcrypto`, generated columns, PITR/backups, mature ecosystem |
| Playwright | 1.5x (latest) | Server-side PDF rendering (headless Chromium) | Renders an HTML/CSS receipt template through a real browser engine — **the only reliably correct way to render Thai script** (complex tone-mark/vowel stacking). Faster + smaller output than Puppeteer, native ARM64 build, official maintained Docker image |
| React + Next.js (App Router) | React 19 / Next 15 | Back-office UI (and Phase-2 public donation form) | SSR/SSG for the future public form, mature i18n, shares TypeScript types with the API; pairs naturally with NestJS |
### Supporting Libraries
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| Prisma | 6.x | ORM / type-safe data access | All standard CRUD: donations, donors, users, statuses, audit reads |
| @nestjs/passport + passport-jwt | current | Authentication | Login + JWT session for back-office (NFR-01) |
| argon2 | 0.41.x | Password hashing | Hash staff passwords (NFR-01) — argon2id is current best practice; do **not** use bcrypt-with-low-cost or plain SHA |
| @casl/ability | 6.x | Authorization rules engine | Encode Maker/Checker/Admin permissions + segregation-of-duties rules (Checker ≠ Maker of same record) |
| nestjs-i18n + i18next | current | Server-side i18n | Localize PDF text, email subject/body, API validation messages (FR-06/FR-23/FR-26) |
| ExcelJS | 4.x | Excel/CSV export | e-Donation export with Thai column headers (FR-30) |
| file-type | 19.x | Magic-byte file validation | Validate uploaded slips are truly jpg/png/pdf, not renamed executables (FR-02) |
| Multer (@nestjs/platform-express) | current | Multipart upload handling | Receive slip uploads with size cap |
| sharp | 0.33.x | Image processing | Normalize/strip EXIF from slip images, thumbnail for review screen |
| Zod | 3.x | Runtime validation | Validate form inputs (donor data, amounts) at API boundary |
| BullMQ + Redis | current | Background job queue | Run PDF render + email send off the request path; enables retry/resend (FR-27) and keeps NFR-07 latency acceptable |
| @aws-sdk/client-ses or Postmark SDK | current | Email delivery | Send receipt PDF as attachment + capture delivery/bounce status |
| pino (via NestJS logger) | current | Structured logging | Feeds audit/observability |
### Development Tools
| Tool | Purpose | Notes |
|------|---------|-------|
| Docker / docker-compose | Reproducible env; **must include `fonts-thai-tlwg`** | Use Microsoft's official Playwright image (`mcr.microsoft.com/playwright`) — ships Chromium + system libs; then `apt-get install fonts-thai-tlwg` (or bake the TH Sarabun .ttf into the image) |
| Vitest / Jest | Testing | **Write a concurrency test that hammers the counter from N parallel transactions and asserts zero gaps/dupes** — this is the project's highest-risk invariant |
| Prisma Migrate | Schema migrations | Version-controlled DDL |
| ESLint + Prettier | Lint/format | Standard |
## The Two Load-Bearing Decisions (read these carefully)
### 1. Gap-less, concurrency-safe, per-fiscal-year receipt number
### 2. Thai-script PDF with watermark + embedded signature
- **Watermark (FR-21):** CSS positioned background element / `opacity` overlay, or a faint `<img>`.
- **Signature image (FR-22):** an `<img src="data:…">` placed by CSS.
- **Thai/English layouts (FR-23):** swap the i18n catalog; same template.
- **Letterhead/seal (FR-20):** HTML + images.
- **Configurable templates (NFR-09/FR-33):** store the HTML template + asset references in DB/config, edit without redeploy.
## PII Encryption-at-Rest (PDPA — NFR-02, FR-29)
- Encrypt the national ID/tax ID **in the application** before it reaches PostgreSQL, so DB operators/backups never contain plaintext. This is the decisive reason to prefer app-level over `pgcrypto` (with `pgcrypto`, plaintext + key transit through the server).
- **Envelope scheme:** a Data Encryption Key (DEK) encrypts the field; a Key Encryption Key (KEK) in a KMS/secrets manager wraps the DEK. Never hardcode keys or store them in the DB. AES-256-**GCM** gives authenticated encryption (confidentiality + tamper detection).
- **Searchability:** if you must look up by national ID, add a **blind index** column (keyed HMAC of the value) — never index the ciphertext directly.
- **Baseline layers underneath:** full-disk/volume encryption on the DB host + TLS (`verify-full`) in transit. These are necessary but not sufficient on their own.
- **Role-based field masking:** decrypt + display the national ID only for roles permitted by RBAC; mask for others.
## Email Delivery (FR-25/26/27/28)
- **SES:** lowest cost, attachment support, deliverability at scale; but you build logging/webhooks yourself (SNS → your endpoint). Good when volume grows and you have ops capacity.
- **Postmark:** best-in-class deliverability for transactional mail, clean delivery/bounce webhooks, message-stream isolation; mid-price. Good fit because hospital receipt volume is modest and reliability/trust > cost.
- Persist a `email_delivery` record per send (status, provider message id, attempts, error) to satisfy FR-27 resend.
- **Do not self-host SMTP** for this — deliverability of raw SMTP is far worse (40–60% inbox vs 90–99% managed) and mail-ops is a liability for a hospital team. A managed SMTP relay is the fallback middle ground if API integration is undesirable.
- Send via the **BullMQ queue** so a transient email failure never blocks/derails the approval transaction, and resend is just re-enqueue.
## Auth & RBAC (NFR-01, FR-34, Maker/Checker)
- **Authentication:** NestJS + Passport JWT; passwords hashed with **argon2id**. Enforce HTTPS/TLS (NFR-02).
- **RBAC:** NestJS **guards** + **CASL** abilities. Roles: Donor (Phase 2), Maker, Checker, Admin.
- **Segregation of duties (key decision in PROJECT.md):** CASL rule that a Checker cannot approve a record they created as Maker. Encode as an attribute rule (`approver_id !== created_by`), enforced in the guard/service, **and** as a DB check for defense-in-depth.
- **Audit trail (FR-13/NFR-05):** append-only `audit_log` table (who/what/when/before/after). Make it immutable: `REVOKE UPDATE, DELETE` from the app role, write via a dedicated insert path or trigger; optionally hash-chain each row (store hash of previous row) so tampering is detectable. Retention configurable for tax-audit timeframes.
## Installation
# Backend core
# Data layer
# PDF + Thai rendering
# (container) apt-get install -y fonts-thai-tlwg   # or bake TH Sarabun New .ttf
# Jobs / email / files / export / i18n
# Dev
## Alternatives Considered
| Recommended | Alternative | When to Use Alternative |
|-------------|-------------|-------------------------|
| NestJS (TypeScript) | **Spring Boot (Java)** | If the hospital IT is a JVM shop or demands the deepest enterprise security tooling (Spring Security fine-grained RBAC, SAML/LDAP). Pairs naturally with iText+pdfCalligraph for Thai PDF. Strongest "boring/safe" enterprise choice |
| NestJS (TypeScript) | **Django (Python)** | If team is Python-first; built-in admin + auth/permissions accelerate the back-office. Pair with WeasyPrint for Thai PDF. Weaker for fine-grained RBAC out of the box |
| Prisma + raw counter | **TypeORM / Drizzle** | If you want native pessimistic locking (`SELECT FOR UPDATE`) as a first-class ORM feature — eliminates the one raw-SQL escape hatch. Strong argument given numbering is the core invariant |
| Playwright (Chromium) | **iText 7 + pdfCalligraph (Java)** | JVM stack; willing to pay commercial license; want vector-perfect programmatic layout + native digital-signature path (relevant if PKI signing is added later) |
| Playwright (Chromium) | **WeasyPrint (Python)** | Python stack; HTML/CSS → PDF without a full browser; good Thai shaping, lighter than Chromium |
| Amazon SES | **Postmark** | Want managed deliverability + rich webhooks with minimal plumbing; modest volume; trust > cost |
| Application-level AES-GCM | **pgcrypto (`pgp_sym_encrypt` aes256)** | Only for less-sensitive fields where DB-admin plaintext exposure is acceptable; simpler but weaker boundary |
## What NOT to Use
| Avoid | Why | Use Instead |
|-------|-----|-------------|
| **PostgreSQL native `SEQUENCE` / `SERIAL` for the receipt number** | `nextval()` is non-transactional — rollbacks leave permanent gaps, violating FR-16 gap-less requirement | Counter table + `SELECT … FOR UPDATE` in the same commit |
| **pdf-lib / jsPDF / pdfkit for Thai receipts** | Documented Thai vowel/tone-mark stacking bugs (pdf-lib #675) — produces malformed diacritics on a legal tax document | Headless Chromium (Playwright) HTML→PDF, or iText+pdfCalligraph |
| **Computing the next number in app code before commit** ("read max + 1") | Race condition: two concurrent approvers read the same max and produce duplicates (violates NFR-04) | DB row lock at commit time (see pattern above) |
| **`pgcrypto` as the primary protection for national ID** | Plaintext + key pass through the server; DB admins/backups can see PII — fails PDPA "limit who sees national ID" intent | Application-level AES-256-GCM envelope encryption |
| **MD5/SHA/bcrypt-low-cost for passwords** | Weak/outdated for password storage | argon2id |
| **MongoDB / document DB as primary store** | Gap-less serialized counters + multi-row ACID approval transactions + audit integrity are exactly what a relational DB with row locking does best; document stores fight you here | PostgreSQL |
| **Self-hosted SMTP server** | Poor deliverability (40–60% inbox), reputation/IP management is a full ops job a hospital team shouldn't own | Managed provider (SES/Postmark) |
| **Storing slip images as BLOBs in the DB** | Bloats DB/backups, slows the table that needs fast transactional access | Object storage (S3-compatible) + DB reference |
| **Trusting file extension / MIME header for uploads** | Trivially spoofed; security risk on a public Phase-2 form | Magic-byte validation (`file-type`) + size cap + (optionally) re-encode images via sharp |
| **Doing PDF render + email send inside the numbering transaction** | Holds the per-year row lock for seconds, serializing all approvers and risking NFR-07 | Commit fast, then enqueue render+email on BullMQ |
## Stack Patterns by Variant
- Use **Spring Boot + PostgreSQL + iText 7/pdfCalligraph** (or **.NET + IronPDF**, which renders Thai via Chromium natively).
- Gap-less pattern is identical (counter table + pessimistic lock — JPA/Hibernate has first-class `PESSIMISTIC_WRITE`).
- Because: leverages existing ops/skill base; Spring Security gives the deepest RBAC.
- Use **Django + PostgreSQL + WeasyPrint + Celery** (queue) + django-guardian (object-level perms).
- Because: Django admin accelerates back-office; WeasyPrint handles Thai shaping; same DB locking pattern (`select_for_update()`).
- Headless-Chromium PDFs are not natively PKI-signed — add a post-processing signing step (e.g. a PDF signing library / service) or move to iText (native signing). Plan the PDF pipeline so a signing stage can be inserted after render.
- Add **rate limiting** (`@nestjs/throttler`) + **CAPTCHA** (hCaptcha/Turnstile) per FR-04, and harden upload validation. The Next.js front-end already covers the public form with i18n.
## Version Compatibility
| Package | Compatible With | Notes |
|---------|-----------------|-------|
| NestJS 11.x | Express 5 / Fastify 5, Node 20+ | NestJS 11 upgraded to Express 5 — check middleware compatibility |
| Prisma 6.x | PostgreSQL 12–17, Node 18+ | Interactive transactions required for `FOR UPDATE` via `$queryRaw` |
| Playwright 1.5x | Use the official `mcr.microsoft.com/playwright` Docker image | Native ARM64 Chromium (unlike Puppeteer); add `fonts-thai-tlwg` |
| argon2 0.41.x | Node 18+ | Native addon — ensure build toolchain in container |
| BullMQ | Redis 6+ | Needs a Redis instance |
## Open Items to Confirm with Stakeholders (not blockers, but they shape the stack)
- **Email provider** — depends on hospital procurement + PDPA data-residency policy (prefer SG region or a TH-based provider). Confirm before committing SES vs Postmark.
- **KMS/secrets store** — which KMS holds the KEK (cloud KMS vs on-prem HSM vs self-managed) drives the encryption module implementation.
- **Hosting** — on-prem hospital data center vs cloud changes object-storage choice (MinIO vs S3) and KMS options. PDPA may push toward on-prem/TH-region hosting.
- **JVM constraint?** — if hospital IT mandates Java, switch to the Spring Boot variant above (everything else, especially the gap-less pattern and Thai-PDF reasoning, transfers directly).
## Sources
- PostgreSQL official docs — sequences are non-gap-less by design; encryption options; pgcrypto AES-256 — https://www.postgresql.org/docs/current/encryption-options.html , https://www.postgresql.org/docs/current/pgcrypto.html — HIGH
- CYBERTEC: "Gaps in sequences in PostgreSQL, causes and remedies" + gapless-via-counter-table+FOR UPDATE pattern — https://www.cybertec-postgresql.com/en/gaps-in-sequences-postgresql/ — HIGH
- pdf-lib Thai shaping bug (stacked vowels/tone marks misalign) — https://github.com/Hopding/pdf-lib/issues/675 — HIGH
- Puppeteer troubleshooting: install `fonts-thai-tlwg` for Thai in headless Chromium — https://pptr.dev/troubleshooting — HIGH
- Fonts-TLWG (Thai scalable fonts, the `fonts-thai-tlwg` package) — https://linux.thai.net/projects/fonts-tlwg — HIGH
- HTML→PDF benchmark (Playwright faster/smaller than Puppeteer; ARM64) — https://pdf4.dev/blog/html-to-pdf-benchmark-2026 — MEDIUM
- Generating PDFs with Playwright (page.pdf, Chromium-only) — https://www.checklyhq.com/docs/learn/playwright/generating-pdfs/ — HIGH
- iText pdfCalligraph (commercial add-on, automatic Thai shaping) + watermark APIs — https://itextpdf.com/en/products/itext-7/pdfcalligraph , https://kb.itextpdf.com/itext/watermark-examples — HIGH
- NestJS 11 release notes + version (11.1.x) — https://trilon.io/blog/announcing-nestjs-11-whats-new , https://www.npmjs.com/package/@nestjs/core — HIGH
- NestJS authorization / RBAC + CASL — https://docs.nestjs.com/security/authorization — HIGH
- Prisma: no native SELECT FOR UPDATE; use $queryRaw in interactive transaction (issues #8580/#17136, discussion #21335) — https://github.com/prisma/prisma/issues/8580 , https://www.prisma.io/docs/orm/prisma-client/queries/transactions — HIGH
- Prisma vs TypeORM (TypeORM has native pessimistic locking) — https://www.prisma.io/docs/orm/more/comparisons/prisma-and-typeorm — MEDIUM
- PostgreSQL column encryption: app-level AES-256-GCM vs pgcrypto, envelope encryption, blind index — https://www.crunchydata.com/blog/data-encryption-in-postgres-a-guidebook , https://moldstud.com/articles/p-choosing-the-right-encryption-method-for-postgresql — MEDIUM-HIGH
- PDPA encryption + RBAC best practices (AES, least privilege) — https://www.globalprivacynetwork.com/best-practices-pdpa-pdpl/ — MEDIUM
- Transactional email comparison (SES/Postmark attachments, webhooks, deliverability; avoid raw SMTP) — https://www.buildmvpfast.com/blog/resend-vs-ses-vs-postmark-transactional-email-deliverability-saas-2026 , https://knock.app/blog/the-top-transactional-email-services-for-developers — MEDIUM
- Framework comparison NestJS/Django/Spring Boot for enterprise RBAC + PDPA — https://betterstack.com/community/guides/scaling-python/spring-boot-vs-django/ , https://outplane.com/blog/spring-boot-vs-nestjs — MEDIUM
<!-- GSD:stack-end -->

<!-- GSD:conventions-start source:CONVENTIONS.md -->
## Conventions

Conventions not yet established. Will populate as patterns emerge during development.
<!-- GSD:conventions-end -->

<!-- GSD:architecture-start source:ARCHITECTURE.md -->
## Architecture

Architecture not yet mapped. Follow existing patterns found in the codebase.
<!-- GSD:architecture-end -->

<!-- GSD:skills-start source:skills/ -->
## Project Skills

No project skills found. Add skills to any of: `.claude/skills/`, `.agents/skills/`, `.cursor/skills/`, `.github/skills/`, or `.codex/skills/` with a `SKILL.md` index file.
<!-- GSD:skills-end -->

<!-- GSD:workflow-start source:GSD defaults -->
## GSD Workflow Enforcement

Before using Edit, Write, or other file-changing tools, start work through a GSD command so planning artifacts and execution context stay in sync.

Use these entry points:
- `/gsd-quick` for small fixes, doc updates, and ad-hoc tasks
- `/gsd-debug` for investigation and bug fixing
- `/gsd-execute-phase` for planned phase work

Do not make direct repo edits outside a GSD workflow unless the user explicitly asks to bypass it.
<!-- GSD:workflow-end -->



<!-- GSD:profile-start -->
## Developer Profile

> Profile not yet configured. Run `/gsd-profile-user` to generate your developer profile.
> This section is managed by `generate-claude-profile` -- do not edit manually.
<!-- GSD:profile-end -->
