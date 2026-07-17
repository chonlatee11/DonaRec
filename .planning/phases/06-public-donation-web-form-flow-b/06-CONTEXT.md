# Phase 6: Public Donation Web Form (Flow B) - Context

**Gathered:** 2026-07-11
**Status:** Ready for planning

<domain>
## Phase Boundary

เว็บฟอร์ม **สาธารณะ (unauthenticated) 2 ภาษา** ให้ผู้บริจาคกรอกข้อมูลเอง (donor details + จำนวนเงิน + วันที่บริจาค)
แนบสลิป + ให้ consent PDPA + ผ่านการกันบอท → ลงเป็น record สถานะ **pending_review** (ไม่ใช่ draft)
ในคิว "รอตรวจสอบ" → **ไหลเข้า pipeline อนุมัติ/ออกใบเสร็จ + PDF/email เดิมทั้งหมด** (Flow B reuse Phase 3–5)
donor ได้ **ack email "รับเรื่องแล้ว ยังไม่ใช่ใบเสร็จ"**. ปิดท้ายด้วยความครบของ responsive + bilingual (NFR-06).

**In scope (Phase 6):**
- **Public donation form (FR-01, FR-06):** หน้าเว็บสาธารณะ 2 ภาษา (ไทย/อังกฤษ) กรอก donor fields + amount + donated_at
- **Public slip upload (FR-02):** **บังคับแนบ** jpg/png/pdf, validate ด้วย **magic-byte + size limit** ฝั่ง server, เก็บนอก webroot (reuse object storage seam Phase 3)
- **PDPA consent (FR-03):** แสดง + บันทึก consent (flag + timestamp + text version + purpose) ก่อน submit ผูก retention model Phase 1 (reuse D-49 snapshot pattern)
- **Bot/spam protection (FR-04):** Cloudflare Turnstile CAPTCHA + per-IP rate limiting (defense-in-depth)
- **Acknowledgement (FR-05):** ยืนยันบนจอ + reference no. + ack email ผ่าน **outbox job ใหม่ `ack_email`** (ระบุชัดว่า *ยังไม่ใช่ใบเสร็จ*)
- **Pending-review queue จากเว็บ (FR-08):** staff เห็นคิว Flow B แยกจาก Flow A ผ่าน `source` column ในหน้า back-office เดิม
- **Responsive + bilingual (NFR-06):** back-office + public form ใช้งานบนมือถือ/เดสก์ท็อป ครบ 2 ภาษา

**Out of scope (เลื่อน / ไม่ทำในเฟสนี้):**
- **Donor status tracking / portal / login** (ดูสถานะ รอตรวจ/ออกแล้ว) — capability ใหม่, เฟสถัดไป (D-86)
- **Donor master / dedup / auto-fill / blind index** — ยัง snapshot-only (D-43 Phase 3); เพิ่มภายหลังได้
- **Payment gateway / ยืนยันเงินเข้าอัตโนมัติ** — เจ้าหน้าที่ตรวจสลิปมือ 100% (PROJECT.md Out of Scope)
- **แก้ pipeline อนุมัติ/ออกเลข/PDF/email** — Flow B **reuse** ของเดิม ไม่แตะ (Phase 2–5)
- **ใบเสร็จทั่วไป non-donation / in-kind** — milestone อื่น

</domain>

<decisions>
## Implementation Decisions

### Public submission seam (created_by / source / API surface)
- **D-76:** **created_by ของ Flow B = seed "system user" เฉพาะ (`public-web`)** — donor ไม่มี login แต่ `created_by` เป็น `NOT NULL REFERENCES users(id)` (migration 000005). seed 1 แถวใน `users` เป็น actor ของทุก record ที่มาจากเว็บ → **คง NOT NULL FK ไว้ (migration น้อยสุด, ไม่อ่อน invariant Phase 3)**, audit actor ชัด. ไม่เลือก created_by nullable เพราะจะอ่อน FK invariant ที่ Phase 3 ตั้งไว้
- **D-77:** **เพิ่มคอลัมน์ `source` ('flow_a' / 'flow_b') บน `donations`** (migration ใหม่ 000015+; default 'flow_a' backfill record เดิม) — แยกคิว "รอตรวจสอบ" ของเว็บออกจาก Flow A อย่าง **explicit** filter/badge ได้ตรง FR-08. ไม่ infer จาก created_by (เปราะ/implicit)
- **D-78:** **public form ยิงเข้า Go API เดิมผ่าน route group ใหม่ `/api/public/donations`** — group นี้ **ไม่ผ่าน `RequireAuth`** (unauthenticated) แต่กั้นด้วย **CAPTCHA + rate-limit middleware** แทน แล้ว **reuse donation service / crypto encrypt / storage (magic-byte) / audit เดิม**. ไม่แยก microservice / ไม่ให้ Next.js server action ยิง DB ตรง (จะ bypass encryption + validation ของ Go). ⚠️ เป็น **public/unauthenticated seam ครั้งแรกของระบบ** — ทุก route เดิมอยู่ใต้ `RequireAuth` หมด

### Donor fields + slip บน public form
- **D-79:** **เลขผู้เสียภาษี/บัตร ปชช. (13 หลัก) = บังคับกรอกบน public form** — คง `donor_tax_id_enc` **NOT NULL (D-44)** ไว้ → pipeline เหมือน Flow A เป๊ะ, e-Donation export (Phase 5) ใช้ได้ทันที. เหตุผล: donor ที่อยากได้ใบลดหย่อนย่อมยินยอมให้เลข (เข้ารหัส at-rest ผ่าน `internal/crypto` envelope). ไม่เลือก optional เพราะจะต้องแก้ schema ให้ nullable + block approve จนกว่าจะเติมครบ (ซับซ้อนกว่า)
- **D-80:** **slip = บังคับแนบใน Flow B** (ต่างจาก Flow A ที่ optional ตาม D-48) — Flow B ไม่มีเจ้าหน้าที่เห็นเงินเข้า ต้องมีสลิปยืนยันก่อนส่งตรวจ. **reuse magic-byte validation + size limit + object storage seam เดิม (D-48/D-54)** — validate content ไม่ใช่ extension, เก็บนอก webroot
- **D-81:** **reuse consent snapshot D-49** (`consent_given` + `consent_at` + `consent_text_version` + `consent_purpose`) — donor ติ๊กยินยอมก่อน submit; เก็บ **`consent_text_version` ชุดข้อความเฉพาะ public form** (แยกจาก Flow A ที่เจ้าหน้าที่กรอกแทน). ผูก retention model Phase 1 (`retain_until`/`legal_basis`), สิทธิขอลบถูกจำกัดโดยกฎหมายภาษี (ไม่ hard-delete)

### Bot / spam protection (FR-04)
- **D-82:** **CAPTCHA = Cloudflare Turnstile** หลัง **config-swappable verifier interface** — privacy-first ไม่ track ผู้ใช้แบบ Google reCAPTCHA (เข้ากับจุดยืน PDPA-first ของโปรเจกต์), verify token ฝั่ง Go ก่อนรับ submit. ⚠️ Turnstile ต้อง **egress ออกเน็ตได้** → ขึ้นกับ hosting (on-prem vs cloud) = **stakeholder gate เดิม**; abstract เป็น interface (แนว email provider ที่ยัง TBD) เพื่อสลับ provider ได้ถ้า on-prem ปิด egress
- **D-83:** **per-IP rate limiting (Go middleware) + CAPTCHA คู่กัน (defense-in-depth)** — จำกัด submit/upload ต่อ IP ต่อช่วงเวลา; กัน automated flood แม้ CAPTCHA หลุด/โดน solve farm. ใช้กับทั้ง form submit และ slip upload endpoint

### Post-submit UX + acknowledgement (FR-05)
- **D-84:** **หลัง submit สำเร็จ = ยืนยันบนจอ + reference no.** — reference เป็นเลขอ้างอิงของ *รายการที่ส่ง* (เช่น donation id / short code) **ไม่ใช่เลขใบเสร็จ** (receipt number เกิดเฉพาะตอน approve ผ่าน allocator Phase 2 เท่านั้น — ห้าม pre-compute). ให้ donor ไว้อ้างอิงติดต่อเจ้าหน้าที่
- **D-85:** **ack email = outbox job_type ใหม่ `ack_email`** ผ่าน worker/outbox เดิม (Phase 4) — decouple, retry ได้, ไม่ block submit response (คุม NFR-07), reuse `internal/mailer` + i18n 2 ภาษา. เนื้อหา **ระบุชัด "รับเรื่องแล้ว ยังไม่ใช่ใบเสร็จ"**. ไม่ส่ง inline (จะให้ email fail กระทบ submit / ช้า response)
- **D-86:** **ไม่มี donor status tracking ในเฟสนี้** — ack email + on-screen confirmation พอสำหรับ MVP; donor portal/login/link ดูสถานะ = capability ใหม่ → **deferred** (กันเพิ่ม attack surface + scope creep บน public seam)

### Claude's Discretion
- **schema รายละเอียด:** column `source` type (enum vs text+CHECK), ค่า/ชื่อ `public-web` system user + วิธี seed (migration seed row vs bootstrap), reference-no format (donation id vs short code), migration number (ต่อจาก **000014** → 000015+)
- **โครงสร้าง package Go:** public handler อยู่ใน `internal/donation/` (reuse) หรือ subpackage ใหม่ (เช่น `internal/publicform/`); CAPTCHA verifier interface + Turnstile impl ตำแหน่งไหน (`internal/captcha/`?), rate-limit middleware ตำแหน่งไหน
- **rate-limit ตัวเลข default:** submit/upload ต่อ IP ต่อช่วงเวลา (config ได้), storage ของ counter (in-memory vs Redis — ปัจจุบัน stack ไม่มี Redis → เริ่ม in-memory/DB ได้)
- **outbox `ack_email` payload/dispatch:** โครง payload + วิธี dispatch แยกจาก `issue_receipt` ใน worker (switch job_type) ตาม pattern Phase 4
- **Next.js public form:** route/URL ของหน้า public (แยก path จาก back-office ที่ต้อง login), Turnstile widget integration, การ detect/สลับภาษา default (next-intl เดิม)
- **responsive audit scope (NFR-06):** จอ/breakpoint ไหนต้องไล่ (Screen เดิม Phase 3–5 + public form)

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project & Requirements
- `.planning/PROJECT.md` — core value (ทุกใบผ่านอนุมัติมนุษย์เสมอ, ไม่มี auto-issue), constraints (Security/PDPA, Audit, Correctness), Flow A vs Flow B model, Out of Scope (payment gateway — ตรวจสลิปมือ)
- `.planning/REQUIREMENTS.md` — **FR-01, FR-02, FR-03, FR-06, FR-05, FR-04** (public website), **FR-08** (pending-review queue จากเว็บ), **NFR-06** (responsive + bilingual), NFR-03 (consent)
- `.planning/ROADMAP.md` §"Phase 6" — goal + 5 success criteria (public form → pending_review same pipeline, server-side slip validation, PDPA consent, bot protection + ack email, responsive Thai/EN queue)

### Load-bearing technical guidance (CLAUDE.md)
- `CLAUDE.md` §"What NOT to Use" — **ห้ามเชื่อ file extension/MIME header → magic-byte validation + size cap** (สำคัญบน public form — spoof ง่าย), ห้ามเก็บ slip เป็น BLOB ใน DB (object storage), ห้าม pre-compute เลขใบเสร็จ, ห้าม render PDF/email ใน issuance tx (outbox)
- `CLAUDE.md` §"PII Encryption-at-Rest" — AES-256-GCM envelope สำหรับเลขภาษี/ปชช. (D-79 encrypt at ingest), role-based masking + audited reveal เดิม
- `CLAUDE.md` §"Email Delivery" — transactional outbox + worker (D-85 ack email เดินตาม), เก็บ delivery record
- `CLAUDE.md` §"Auth & RBAC" — audit trail append-only hash-chain (public submit ต้อง audit ด้วย actor = public-web)
- `CLAUDE.md` §"Integration-test gate" — **endpoint public/donations ใหม่ต้องมี E2E ผ่าน real HTTP path** (router → CAPTCHA/rate-limit middleware → handler → service → DB) ก่อนถือว่า done; +human UI walkthrough (public form + staff queue) ผ่าน

### Prior phase context (สืบทอด — reuse ทั้งหมด)
- `.planning/phases/03-donation-lifecycle-maker-checker-issuance/03-CONTEXT.md` — **D-43** snapshot-only donor, **D-44** tax_id NOT NULL (D-79 คงไว้), **D-48** object storage + magic-byte seam (D-80 reuse), **D-49** consent snapshot (D-81 reuse), **D-54** slip soft-delete-retain, state machine `draft→pending_review→issued/...`, `internal/donation` service, outbox enqueue
- `.planning/phases/04-receipt-pdf-email-delivery-outbox-worker/04-CONTEXT.md` — outbox **worker + job_type dispatch** (D-85 เพิ่ม `ack_email`), `internal/mailer` + bilingual i18n email, NFR-07 latency (submit ต้องเร็ว)
- `.planning/phases/05-e-donation-export-reports-admin-settings/05-CONTEXT.md` — `edonation_keyed`, config store (Turnstile config อาจต่อยอด), Flow B ต่อ queue/aging เดิม
- `.planning/phases/02-gap-less-receipt-numbering-core/02-CONTEXT.md` — allocator เกิดเฉพาะตอน approve; reference-no ของ Flow B (D-84) **ห้ามชน/pre-compute เลขใบเสร็จ**

### No additional ADRs/specs
- ยังไม่มี SPEC.md แยกสำหรับเฟสนี้ — decisions ครบใน `<decisions>` ข้างบน

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets (พร้อมใช้จาก Phase 1–5)
- **`internal/donation/` + `DonationHandler`/`SlipHandler` + service** — public handler reuse service create (ลง pending_review), magic-byte slip upload path เดิม (D-78/D-80)
- **`internal/crypto/` (AES-256-GCM envelope)** — encrypt `donor_tax_id_enc` ตอนรับจาก public form (D-79)
- **`internal/storage/` (MinIO client) + magic-byte + size limit** — slip Flow B เก็บ bucket เดิม, validate content (D-80/D-48)
- **`internal/audit/` (append-only hash-chain)** — audit ทุก public submit (actor = public-web system user)
- **`internal/mailer/` + `internal/worker/` outbox (`issue_receipt.go`)** — เพิ่ม `ack_email` job_type คู่ `issue_receipt` (D-85); worker dispatch by job_type
- **`internal/i18n/` (go-i18n) + next-intl (FE)** — ack email + public form + back-office 2 ภาษา (FR-06/NFR-06)
- **`internal/users/`** — seed `public-web` system user (D-76)
- **`internal/config/` / `internal/settings/`** — CAPTCHA site/secret key + rate-limit threshold config (D-82/D-83, no-deploy)
- **Next.js back-office + BFF + TanStack + next-intl** (Phase 3–5) — public form หน้าใหม่ + queue Flow B filter (source) + responsive audit
- **testcontainers Postgres + MinIO (`internal/testutil/`)** — E2E public submit path (integration-test gate)

### Established Patterns
- data = sqlc + pgx/v5; migration = golang-migrate (ล่าสุด **000014**) → Phase 6 ต่อ **000015+** (source column + system user seed)
- HTTP = gin; router setup `cmd/server/main.go` `setupRouter()` — **ทุก /api group ปัจจุบันอยู่ใต้ `RequireAuth`**; Phase 6 เพิ่ม **public group แรกที่ไม่ auth** (มี CAPTCHA+rate-limit middleware แทน)
- middleware chain: Recovery → zapLogger → AuditMiddleware → (RequireAuth) — public group เสียบ CAPTCHA/rate-limit หลัง audit ก่อน handler
- `RequireAnyRole` (OR-guard) + `ResolveAppUser` (created-by FK) — public group ไม่ใช้ (created_by = fixed system user)

### Integration Points (ใหม่ในเฟสนี้)
- public form (Next.js) ↔ `POST /api/public/donations` (no auth) ↔ CAPTCHA verify (Turnstile) + rate-limit ↔ donation service (create pending_review, source='flow_b', created_by=public-web) ↔ crypto encrypt + storage slip + audit
- submit สำเร็จ ↔ enqueue outbox `ack_email` ↔ worker ↔ mailer i18n → donor inbox
- staff back-office queue ↔ filter `source='flow_b'` + status='pending_review' (FR-08) → ไหลเข้า approve/issue pipeline เดิม
- CAPTCHA/rate-limit config ↔ config/settings store (no-deploy)

</code_context>

<specifics>
## Specific Ideas

- ผู้ใช้คงแนว **reuse-first / ไม่แตะ pipeline แกน**: Flow B ลง pending_review เข้าท่ออนุมัติ+ออกเลข+PDF+email เดิมทั้งหมด — Phase 6 เพิ่มแค่ "ปากทาง" สาธารณะ + กันบอท + ack
- ผู้ใช้เลือก **คง schema invariant เดิมให้มากสุด**: created_by NOT NULL (system user แทน nullable), tax_id NOT NULL (บังคับกรอกแทน optional) — ลด migration ที่อ่อน invariant Phase 3
- ผู้ใช้ **PDPA-first ต่อเนื่อง**: เลือก Turnstile (ไม่ track แบบ Google), ไม่มี donor portal (ลด attack surface + PII exposure), slip validate ด้วย content
- **slip บังคับใน Flow B** เพราะไม่มีเจ้าหน้าที่เห็นเงินเข้า — ต่างจาก Flow A (เงินสด/เคาน์เตอร์ optional ได้)
- ผู้ใช้ย้ำ **reference no. ≠ เลขใบเสร็จ** — สอดคล้องหลักออกเลขเฉพาะตอนอนุมัติ (Phase 2)
- **defense-in-depth บน public seam**: CAPTCHA + per-IP rate limit + magic-byte + server-side validation — เพราะเป็น endpoint เดียวที่เปิดสาธารณะ

</specifics>

<deferred>
## Deferred Ideas

- **Donor status tracking / portal / login / link ดูสถานะรายการ** (D-86) — capability ใหม่, เพิ่ม attack surface + PII exposure บน public; เฟส/milestone ถัดไป
- **Donor master + dedup + auto-fill + blind index** (D-43 Phase 3) — ยัง snapshot-only; เพิ่มภายหลังโดยไม่ migrate snapshot เดิม
- **สลับ CAPTCHA provider / self-host** — abstract เป็น interface แล้ว (D-82); เปลี่ยน default ได้ตอน hosting ยืนยัน (stakeholder gate egress)
- **e-Donation API ตรง / รายงานเชิงลึก / PKI signature** — milestone อื่น (REQUIREMENTS.md v2)

None อื่น — discussion อยู่ในขอบเขตเฟส

</deferred>

---

*Phase: 6-public-donation-web-form-flow-b*
*Context gathered: 2026-07-11*
