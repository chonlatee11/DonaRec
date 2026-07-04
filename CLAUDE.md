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

> **Stack ที่ใช้จริง = Go backend + Keycloak (hybrid auth) + PostgreSQL + Next.js**
> ตัดสินใน `.planning/` (decisions **D-20..D-26**) ซึ่ง **override** คำแนะนำ NestJS/TypeScript เดิมทั้งหมด
> ผู้ใช้ระบุชัดว่าต้องการ backend เป็น Go และใช้ Keycloak สำหรับ authN — downstream ต้องเคารพ
> เหตุผลหลัก 3 ข้อของโปรเจกต์ transfer ครบ: gap-less counter ด้วย row-lock (`SELECT … FOR UPDATE`),
> app-level AES-256-GCM envelope encryption, และ Thai PDF ผ่าน headless Chromium

## TL;DR — The Stack
| Concern | Choice | One-line why |
|---------|--------|--------------|
| Language | **Go (golang)** | คำสั่งของผู้ใช้ (D-20); คุม `SELECT … FOR UPDATE` / transaction boundary ได้ตรงไปตรงมา, single static binary deploy ง่าย, concurrency เป็น first-class |
| HTTP layer | **`net/http` + chi router** (recommended) | idiomatic, stdlib-compatible middleware; gin/echo/fiber เป็น alternative — router-level เลือกตอน plan |
| Database | **PostgreSQL 17** (16+ acceptable) | row-level locking + transactional commit แก้ gap-less counter (NFR-04/FR-16) ได้สะอาด; PITR/backup โตเต็มวัย |
| Data layer | **sqlc + pgx** (queries → type-safe Go) **+ raw `SELECT … FOR UPDATE`** สำหรับ counter | sqlc generate code จาก SQL จริง คุม lock เองได้เต็มที่ (D-23). pgx เป็น driver. GORM/ent เป็น alternative |
| Migrations | **golang-migrate** (`migrations/NNNNNN_*.up/down.sql`) | versioned DDL, ใช้คู่ sqlc ได้ดี |
| AuthN | **Keycloak (OIDC)** — login, password policy, lockout, session, MFA-ready | hybrid: Keycloak ให้ "ใครมี role อะไร" ผ่าน OIDC token; argon2id/password policy บังคับใน realm config (D-24). แอป Go ไม่เก็บ password เอง |
| AuthZ | **Go app เอง** — RBAC guard + SoD ต่อ record + PII masking + audit | logic ธุรกิจ (Checker ≠ Maker, PII reveal, audit hash-chain) อยู่ในแอปเสมอ ไม่ฝากไว้กับ Keycloak |
| PDF generation | **Headless Chromium ผ่าน Go (chromedp/rod)** render HTML/CSS template | browser engine คือทางเดียวที่ render Thai vowel/tone-mark stacking *ถูกต้อง*; watermark + signature = CSS/img ⚠️ research flag Phase 4 |
| Thai font | **TH Sarabun New** ฝังผ่าน `@font-face` + OS package `fonts-thai-tlwg` ใน container | ฟอนต์มาตรฐานเอกสารราชการ/ภาษีไทย; container ต้องมี Thai font package ไม่งั้นได้ tofu boxes |
| Email | **Amazon SES** (`ap-southeast-1`) ผ่าน `aws-sdk-go-v2` **หรือ Postmark** | PDF attachment + delivery/bounce webhook; provider TBD (stakeholder gate) |
| PII encryption-at-rest | **App-level AES-256-GCM (envelope: DEK/KEK)** สำหรับ national/tax ID, KEK จาก env (MVP) → KMS ภายหลัง | PDPA: DB admin/backup ต้องไม่เห็น plaintext → app-level เหนือกว่า `pgcrypto` |
| File uploads | **MinIO / S3-compatible object storage + magic-byte validation + size limit** | validate ด้วย content ไม่ใช่ extension; เก็บ slip ออกจาก DB |
| Excel/CSV export | **excelize** (`github.com/xuri/excelize`) / built-in `encoding/csv` | e-Donation manual keying ต้องการ .xlsx จริงที่มีข้อความไทย (FR-30) |
| i18n | **go-i18n** ฝั่ง Go (`nicksnyder/go-i18n`) + **next-intl / react-i18next** ฝั่ง Next.js | catalog เดียวขับ UI + PDF + email |
| Audit log | **Append-only PostgreSQL table** + `REVOKE UPDATE/DELETE` + hash-chain (SHA-256 ต่อแถว) | immutable trail สำหรับตรวจบัญชี/ภาษี (NFR-05/FR-13) |
| Frontend | **React 19 + Next.js 15 (App Router)** | back-office UI + Phase-6 public form; SSR/SSG, i18n โตเต็มวัย |

## Detail — Core Technologies
| Technology | Version | Purpose | Why |
|------------|---------|---------|-----|
| Go | 1.23+ | Backend language | คำสั่งผู้ใช้ (D-20); goroutine + `database/sql`/pgx ให้คุม transaction/lock ตรงไปตรงมา; static binary + Docker เล็ก deploy ง่าย |
| PostgreSQL | 17 (16+ acceptable) | Primary relational DB | **gap-less, concurrency-safe receipt sequence (NFR-04/FR-16) คือ requirement ที่ยากที่สุด** และ row-level locking + transactional commit ของ Postgres แก้ได้สะอาด; generated columns, PITR/backup, ecosystem โตเต็มวัย |
| sqlc + pgx | sqlc 1.x / pgx v5 | Type-safe data access | generate Go จาก SQL จริง — ไม่มี ORM มาบังจังหวะ lock; raw `SELECT … FOR UPDATE` เขียนตรงใน query สำหรับ counter (Phase 2) |
| golang-migrate | 4.x | Schema migrations | versioned up/down DDL |
| Keycloak | latest LTS | AuthN (OIDC provider) | login, password policy/lockout, session, role assignment, MFA-ready — self-hosted ผ่าน Docker (D-24/D-26) |
| Headless Chromium (chromedp/rod) | latest | Server-side PDF rendering | render HTML/CSS receipt ผ่าน browser engine จริง — **ทางเดียวที่ render Thai script ถูกต้อง** (tone-mark/vowel stacking) ⚠️ ยืนยัน library ตอน Phase 4 spike |
| React + Next.js (App Router) | React 19 / Next 15 | Back-office UI + Phase-6 public form | SSR/SSG, i18n, แยก service จาก Go API ผ่าน HTTP/OIDC |

## Detail — Supporting Libraries (Go)
| Library | Purpose | When to use |
|---------|---------|-------------|
| `jackc/pgx` v5 | Postgres driver/pool | data layer + raw-SQL row-lock path สำหรับ counter |
| `golang-jwt/jwt` + OIDC discovery (เช่น `coreos/go-oidc`) | ตรวจ Keycloak token + map role claim | auth middleware (NFR-01/FR-34) |
| `go-playground/validator` | Runtime validation | validate donor data / amount ที่ขอบ API |
| `gabriel-vasile/mimetype` (หรือ `net/http.DetectContentType`) | Magic-byte file validation | ยืนยัน slip เป็น jpg/png/pdf จริง ไม่ใช่ไฟล์ rename (FR-02) |
| `minio-go` | Object storage client | เก็บ slip ใน MinIO/S3-compatible |
| `disintegration/imaging` (หรือ govips) | Image processing | normalize/strip EXIF, thumbnail หน้า review |
| `xuri/excelize` | Excel/CSV export | e-Donation export พร้อม header ไทย (FR-30) |
| `nicksnyder/go-i18n` | Server-side i18n | localize PDF text, email subject/body, validation messages |
| `aws-sdk-go-v2` (SES) หรือ Postmark Go client | Email delivery | ส่ง PDF เป็น attachment + เก็บ delivery/bounce status |
| `log/slog` (stdlib) หรือ zerolog | Structured logging | feed audit/observability |
| Background worker: **transactional outbox table + worker goroutine** (asynq/River ถ้ามี Redis) | Async PDF+email | รัน render+email นอก request path; retry/resend (FR-27); keep NFR-07 latency |

## Development Tools
| Tool | Purpose | Notes |
|------|---------|-------|
| Docker / docker-compose | Reproducible env | compose: Go app + Postgres + Keycloak + MinIO; **container ที่ render PDF ต้องมี Chromium + `fonts-thai-tlwg`** (หรือ bake TH Sarabun .ttf) |
| `go test` + testify | Testing | **เขียน concurrency test ที่ยิง counter จาก N parallel transactions แล้ว assert zero gaps/dupes** — invariant เสี่ยงที่สุดของโปรเจกต์ |
| `testcontainers-go` | Integration test fixtures | spin Postgres + Keycloak จริงใน test (เห็นใน `internal/testutil/postgres.go`, `keycloak.go`) |
| sqlc | Codegen | generate Go จาก `internal/db/queries/*.sql` ตาม `sqlc.yaml` |
| golangci-lint + gofmt | Lint/format | standard |

## The Two Load-Bearing Decisions (read these carefully)
### 1. Gap-less, concurrency-safe, per-fiscal-year receipt number
- **ห้าม** ใช้ `SEQUENCE`/`SERIAL` — `nextval()` ไม่ transactional, rollback ทิ้ง gap ถาวร
- ใช้ **counter table + `SELECT … FOR UPDATE`** ภายใน transaction เดียวกับการ issue, ออกเลขในจังหวะ commit
- counter keyed **ต่อ fiscal year** (BE, Asia/Bangkok) → reset เป็น 1 อัตโนมัติเมื่อขึ้นปีใหม่ ไม่ต้องมี job
- `UNIQUE(fiscal_year, running_no)` เป็น backstop กัน logic bug
- ใน Go: เขียน raw query ผ่าน sqlc/pgx — **ห้าม** pre-compute/reserve เลขบน draft (Phase 2 core)

### 2. Thai-script PDF with watermark + embedded signature
- **Watermark (FR-21):** CSS positioned background / `opacity` overlay หรือ faint `<img>`
- **Signature image (FR-22):** `<img src="data:…">` วางด้วย CSS
- **Thai/English layouts (FR-23):** สลับ i18n catalog, template เดียวกัน
- **Letterhead/seal (FR-20):** HTML + images
- **Configurable templates (NFR-09/FR-33):** เก็บ HTML template + asset references ใน DB/config, แก้ได้โดยไม่ deploy
- ⚠️ **Phase 4 spike:** ยืนยัน chromedp vs rod ด้วย worst-case Thai text ก่อน lock library

## PII Encryption-at-Rest (PDPA — NFR-02, FR-29)
- เข้ารหัส national/tax ID **ในแอป Go** ก่อนถึง Postgres → DB operator/backup ไม่มีวันเห็น plaintext (เหตุผลชี้ขาดที่เลือก app-level เหนือ `pgcrypto`)
- **Envelope scheme:** DEK เข้ารหัส field; KEK wrap DEK. MVP เก็บ KEK ใน **env** (D-26), ออกแบบให้ย้าย KMS ได้. ห้าม hardcode key / ห้ามเก็บ key ใน DB. AES-256-**GCM** = authenticated encryption (confidentiality + tamper detection)
- **Searchability:** ถ้าต้อง lookup ด้วย national ID ใช้ **blind index** (keyed HMAC-SHA256, index key แยกต่างหาก) — ห้าม index ciphertext ตรงๆ
- **Baseline ข้างใต้:** full-disk encryption + TLS (`verify-full`) — จำเป็นแต่ไม่พอลำพัง
- **Role-based masking:** decrypt + แสดง national ID เฉพาะ role ที่ RBAC อนุญาต, mask ที่เหลือ; ทุก reveal ถูก audit

## Email Delivery (FR-25/26/27/28)
- **SES:** ถูกสุด, รองรับ attachment, deliverability สเกลได้; แต่ต้องสร้าง logging/webhook เอง (SNS → endpoint)
- **Postmark:** deliverability ดีที่สุดสำหรับ transactional, webhook สะอาด; ราคากลาง — เหมาะเพราะ volume โรงพยาบาลไม่มาก, trust > cost
- เก็บ `email_delivery` record ต่อการส่ง (status, provider message id, attempts, error) เพื่อรองรับ resend (FR-27)
- **ห้าม self-host SMTP** — deliverability แย่ (40–60% inbox vs 90–99% managed), mail-ops เป็นภาระที่ทีมโรงพยาบาลไม่ควรแบก
- ส่งผ่าน **transactional outbox + worker** เพื่อให้ email fail ชั่วคราวไม่ block/rollback approval transaction; resend = re-enqueue

## Auth & RBAC (NFR-01, FR-34, Maker/Checker)
- **AuthN (Keycloak):** login, password policy + argon2id hashing, lockout, session — บังคับใน Keycloak realm config; แอป Go รับ OIDC token แล้ว validate + map role claim. บังคับ HTTPS/TLS (NFR-02)
- **AuthZ (Go app):** RBAC guard ใน middleware/service. Roles: Donor (Phase 6), Maker, Checker, Admin
- **Segregation of duties:** Checker ห้าม approve record ที่ตัวเองสร้างเป็น Maker — บังคับเป็น attribute rule (`approver_id != created_by`) ใน guard/service **และ** เป็น DB check เพื่อ defense-in-depth
- **Audit trail (FR-13/NFR-05):** append-only `audit_log` (who/what/when/before→after). immutable ด้วย `REVOKE UPDATE, DELETE` จาก app role + hash-chain (SHA-256 ของ prev_hash ต่อแถว) → ตรวจจับการแก้ไขกลางทางได้. retention configurable

## Alternatives Considered
| Chosen | Alternative | When to use alternative |
|--------|-------------|-------------------------|
| Go + chi | **NestJS (TypeScript)** | คำแนะนำเดิม; เลือกถ้าทีมเป็น TS-first และอยากได้ guards/CASL out of the box (ถูก override เป็น Go ตามคำสั่งผู้ใช้) |
| Go + chi | **Spring Boot (Java)** | ถ้า hospital IT เป็น JVM shop / ต้องการ Spring Security + SAML/LDAP เชิงลึก; pair กับ iText+pdfCalligraph |
| sqlc + pgx | **GORM / ent** | ถ้าต้องการ ORM เต็มรูป (relations, migrations อัตโนมัติ); แต่ต้องระวังให้ counter path ยังคุม `FOR UPDATE` ได้เอง |
| chromedp/rod (Chromium) | **iText 7 + pdfCalligraph (JVM)** | JVM stack; ยอมจ่าย license; อยากได้ vector-perfect layout + native PKI signing |
| chromedp/rod (Chromium) | **gofpdf / Maroto (pure Go)** | ⚠️ **อย่าใช้กับใบเสร็จไทย** — pure-Go PDF libs ไม่ทำ complex Thai shaping; ใช้ได้เฉพาะเอกสาร Latin-only เท่านั้น |
| App-level AES-GCM | **pgcrypto (`pgp_sym_encrypt` aes256)** | เฉพาะ field ที่อ่อนไหวน้อยและยอมรับการที่ DB admin เห็น plaintext ได้ |
| Amazon SES | **Postmark** | อยากได้ managed deliverability + webhook สำเร็จรูป; volume น้อย; trust > cost |

## What NOT to Use
| Avoid | Why | Use Instead |
|-------|-----|-------------|
| **PostgreSQL native `SEQUENCE`/`SERIAL` สำหรับเลขใบเสร็จ** | `nextval()` ไม่ transactional — rollback ทิ้ง gap ถาวร, ผิด FR-16 | Counter table + `SELECT … FOR UPDATE` ใน commit เดียวกัน |
| **gofpdf / Maroto / pdfkit-class libs สำหรับใบเสร็จไทย** | pure-code PDF libs มีบั๊ก Thai vowel/tone-mark stacking — diacritic เพี้ยนบนเอกสารภาษีตามกฎหมาย | Headless Chromium (chromedp/rod) HTML→PDF |
| **คำนวณเลขถัดไปใน app code ก่อน commit** ("read max + 1") | race: approver พร้อมกันอ่าน max เดียวกัน → เลขซ้ำ (ผิด NFR-04) | DB row lock ตอน commit |
| **`pgcrypto` เป็นการป้องกันหลักของ national ID** | plaintext + key ผ่าน server; DB admin/backup เห็น PII — ผิดเจตนา PDPA | App-level AES-256-GCM envelope |
| **เก็บ password ในแอป / MD5/SHA/bcrypt-low-cost** | authN ย้ายไป Keycloak แล้ว; แอปไม่ควรถือ password. ถ้าจำเป็นต้อง hash ใช้ argon2id | Keycloak realm (argon2id) |
| **MongoDB / document DB เป็น primary store** | gap-less counter + multi-row ACID approval + audit integrity คือสิ่งที่ relational + row lock ทำได้ดีที่สุด | PostgreSQL |
| **Self-hosted SMTP server** | deliverability แย่ (40–60% inbox), IP/reputation เป็นงาน ops เต็มเวลา | Managed provider (SES/Postmark) |
| **เก็บ slip images เป็น BLOB ใน DB** | bloat DB/backup, ช้า table ที่ต้องการ transactional access เร็ว | Object storage (MinIO/S3) + DB reference |
| **เชื่อ file extension / MIME header ของ upload** | spoof ง่าย; เสี่ยงบน public form (Phase 6) | Magic-byte validation + size cap + (option) re-encode รูป |
| **render PDF + ส่ง email ใน numbering transaction** | ถือ row lock เป็นวินาที, serialize approver ทั้งหมด, เสี่ยง NFR-07 | commit เร็ว แล้ว enqueue ผ่าน outbox worker |

## Version Compatibility
| Component | Compatible With | Notes |
|-----------|-----------------|-------|
| Go 1.23+ | pgx v5, sqlc 1.x | สำหรับ raw `FOR UPDATE` ผ่าน sqlc-generated query |
| pgx v5 | PostgreSQL 12–17 | driver/pool หลักของ data layer |
| golang-migrate 4.x | PostgreSQL | up/down SQL ใน `migrations/` |
| chromedp/rod | ใช้ official Chromium ใน container | ต้องมี `fonts-thai-tlwg`; ยืนยัน lib ตอน Phase 4 |
| Keycloak | OIDC (`go-oidc` / `golang-jwt`) | discovery + JWKS validation |
| Outbox worker (asynq/River) | Redis 6+ (ถ้าเลือก Redis-backed) | option; DB-backed outbox ไม่ต้องใช้ Redis |

## Open Items to Confirm with Stakeholders (not blockers, but they shape the stack)
- **Email provider** — ขึ้นกับ procurement + PDPA data-residency (prefer SG หรือ TH-based). ยืนยันก่อน commit SES vs Postmark
- **KMS/secrets store** — KEK เก็บที่ไหน (cloud KMS vs on-prem HSM vs env-managed) กำหนด encryption module. MVP ใช้ env แล้วออกแบบให้ย้ายได้
- **Hosting** — on-prem vs cloud เปลี่ยน object storage (MinIO vs S3) และ KMS options. PDPA อาจดันไป on-prem/TH-region
- **Data layer final pick** — sqlc+pgx เป็น path ที่ใช้ใน Phase 1; ยืนยันว่าครอบคลุม Phase 2 counter ก่อน lock (research flag เดิม)

## Sources
- PostgreSQL official docs — sequences ไม่ gap-less by design; encryption options; pgcrypto AES-256 — https://www.postgresql.org/docs/current/encryption-options.html , https://www.postgresql.org/docs/current/pgcrypto.html — HIGH
- CYBERTEC: "Gaps in sequences in PostgreSQL" + gapless-via-counter-table+FOR UPDATE pattern — https://www.cybertec-postgresql.com/en/gaps-in-sequences-postgresql/ — HIGH
- pdf-lib Thai shaping bug (stacked vowels/tone marks misalign) — เหตุผลว่าทำไมต้อง browser engine สำหรับ Thai PDF — https://github.com/Hopding/pdf-lib/issues/675 — HIGH
- Puppeteer troubleshooting: install `fonts-thai-tlwg` สำหรับ Thai ใน headless Chromium — https://pptr.dev/troubleshooting — HIGH
- Fonts-TLWG (Thai scalable fonts, package `fonts-thai-tlwg`) — https://linux.thai.net/projects/fonts-tlwg — HIGH
- sqlc — generate type-safe Go จาก SQL — https://docs.sqlc.dev — HIGH
- pgx (PostgreSQL driver/toolkit สำหรับ Go) — https://github.com/jackc/pgx — HIGH
- golang-migrate — https://github.com/golang-migrate/migrate — HIGH
- chromedp (drive headless Chromium จาก Go) — https://github.com/chromedp/chromedp — HIGH
- go-rod (alternative Chromium automation, Go) — https://github.com/go-rod/rod — MEDIUM
- Keycloak securing apps / OIDC — https://www.keycloak.org/docs/latest/securing_apps/ — HIGH
- coreos/go-oidc (OIDC token verification, Go) — https://github.com/coreos/go-oidc — HIGH
- excelize (Excel ใน Go, รองรับข้อความไทย/Unicode) — https://github.com/qax-os/excelize — HIGH
- testcontainers-go (Postgres/Keycloak fixtures ใน test) — https://golang.testcontainers.org — HIGH
- PostgreSQL column encryption: app-level AES-256-GCM vs pgcrypto, envelope, blind index — https://www.crunchydata.com/blog/data-encryption-in-postgres-a-guidebook — MEDIUM-HIGH
- PDPA encryption + RBAC best practices (AES, least privilege) — https://www.globalprivacynetwork.com/best-practices-pdpa-pdpl/ — MEDIUM
- Transactional email comparison (SES/Postmark attachments, webhooks; avoid raw SMTP) — https://knock.app/blog/the-top-transactional-email-services-for-developers — MEDIUM
<!-- GSD:stack-end -->

<!-- GSD:conventions-start source:CONVENTIONS.md -->
## Conventions

### Integration-test gate (done-criterion for runtime-integration phases)

A phase that touches the **runtime request seam** — HTTP routing, auth middleware (Keycloak/OIDC token verification), RBAC/route guards, identity resolution, or DB writes behind those layers — is **NOT "done"** until an **end-to-end integration test** exercises the real path: `HTTP request → RequireAuth (real token: sub/aud/realm_access.roles) → RequireRoles / ResolveAppUser → handler → service → DB`, driven by a realistic Keycloak-shaped token (audience includes the backend client; roles in `realm_access.roles`; `sub` is a UUID that must resolve to a provisioned `users.id`).

Unit/service tests that construct claims or user rows by hand and call services **directly** do NOT satisfy this gate — they structurally cannot catch seam defects (audience mismatch, `RequireRoles` AND-vs-OR misuse, `claims.Subject`-vs-`users.id` identity, route-guard wiring). Evidence: Phase 3 passed 5/5 unit-level verification yet shipped three such seam bugs (`created-by-fk-mismatch`, `fe-be-audience-mismatch`, an RBAC AND-bug) that only surfaced when the real stack was driven with a real token.

Phase verification MUST include this gate before a phase is marked **Complete**: (a) an automated E2E integration test over the real HTTP path for the phase's critical flows, AND (b) the human UI walkthrough (where a UI exists) passing. Full text: `.planning/CONVENTIONS.md`.
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
