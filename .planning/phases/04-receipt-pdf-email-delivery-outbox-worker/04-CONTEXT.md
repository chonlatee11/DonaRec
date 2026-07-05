# Phase 4: Receipt PDF + Email Delivery (Outbox Worker) - Context

**Gathered:** 2026-07-04
**Status:** Ready for planning

<domain>
## Phase Boundary

สร้าง **outbox worker + PDF render pipeline + email delivery** ที่ทำงาน **แบบ async หลัง receipt ถูก issued**
โดย consume `outbox_jobs` (job_type `issue_receipt`, payload `{donation_id}`) ที่ Phase 3 enqueue ไว้แล้ว
ใน issuance transaction → render ใบเสร็จ PDF ไทย/อังกฤษที่ถูกต้องตามประมวลรัษฎากร (letterhead/seal, watermark,
signature, ข้อความ §6 + 1x/2x) → เก็บไฟล์ → ส่งอีเมลแนบ PDF ให้ผู้บริจาค พร้อมบันทึกสถานะ/retry/resend
**โดยไม่บล็อกและไม่ rollback issuance transaction** (NFR-07)

**In scope (Phase 4):**
- **Outbox worker** (goroutine) — poll `outbox_jobs` (pending/failed) ผ่าน partial index ที่มีอยู่ → process นอก lock path → update status/attempts/last_error (Phase 3 grant SELECT/UPDATE ให้แล้ว)
- **PDF render** ผ่าน headless Chromium (chromedp/rod — ยืนยัน lib ตอน spike) จาก HTML/CSS template + TH Sarabun font (`fonts-thai-tlwg` ใน container): letterhead/seal (FR-20), watermark (FR-21), signature image (FR-22), ข้อความ §6 ลดหย่อนภาษี (FR-24) รวมประโยค 1 เท่า/2 เท่า
- **ภาษาเอกสาร** ไทย/อังกฤษ ตาม `donor_language` (D-55) ผ่าน go-i18n bundle เดิม (`internal/i18n`) — template เดียวสลับ catalog
- **Golden-file visual test** ใน CI: worst-case Thai (stacked tone marks + Thai+Latin, Latin-leading) render ถูกต้อง (SC#2)
- **PDF persistence** — freeze ไฟล์ที่ออกจริงเก็บใน MinIO (`internal/storage` เดิม) (D-56)
- **Email delivery** — ส่ง PDF แนบอีเมล 2 ภาษา ตาม `donor_language` (FR-25/26), บันทึก `email_delivery` (status/provider msg id/attempts/error), auto-retry + backoff, staff resend มือ (FR-27), staff ดาวน์โหลด PDF เองได้กรณีไม่มีอีเมล (FR-28)
- **Config store (DB)** + **Admin settings UI พร้อม live preview** — template HTML, watermark/signature/seal/letterhead images, ข้อความ §6 + 1x/2x, รูปแบบเลข — แก้ได้โดยไม่ deploy + **แสดง preview ผลลัพธ์ตอนแก้** และ **performance สมูท** (FR-33/NFR-09, D-58/D-59/D-61)
- **`donor_language` column** บน donation (migrate + เพิ่ม field ในฟอร์ม Flow A ของ Phase 3) (D-55)

**Out of scope (เลื่อน / ไม่ทำในเฟสนี้):**
- **การเลือก email provider จริง (SES vs Postmark)** — stakeholder gate; เฟสนี้ build ต่อ `EmailSender` interface + dev/local capture, provider จริงเสียบภายหลัง (ดู D-60)
- e-Donation export / reports / "คีย์แล้ว" flag setting → Phase 5 (FR-30/31/32)
- Backup/restore verification (NFR-08) → Phase 5
- Flow B public form + acknowledgement email (FR-05) → Phase 6
- PKI digital signature — MVP ใช้รูปภาพลายเซ็นเท่านั้น (PROJECT.md Out of Scope)
- การยืนยันข้อความ §6 + สิทธิ 1x/2x กับบัญชี/กฎหมาย — stakeholder gate (เก็บเป็น config ให้แก้ได้ ไม่ block build)

</domain>

<decisions>
## Implementation Decisions

### ภาษาเอกสาร (FR-23)
- **D-55:** **เพิ่ม column `donor_language` บน donation** (`th`/`en`, default `th`), Maker เลือกตอนสร้างรายการ (Flow A). ภาษาถูก **freeze เป็นส่วนหนึ่งของ snapshot** (หลักเดียวกับ D-43) → PDF + email ทั้งหมดใช้ภาษานี้. ต้องมี migration ใหม่ + เพิ่ม field ในฟอร์ม create/edit ของ Phase 3 (FE) + default `th` สำหรับ record เดิม. เหตุผล: FR-23 บังคับออกตามภาษาผู้บริจาคจริง — ต้อง persist ไม่ใช่ toggle ชั่วคราว

### PDF persistence (FR-24, immutability)
- **D-56:** **Freeze — render ครั้งเดียวตอน worker process job แล้วเก็บไฟล์ PDF ใน MinIO** (bucket แยกจาก slip เช่น `donnarec-receipts`, เก็บ reference ใน DB). resend/download ใช้ไฟล์เดิมเสมอ — **ไม่ re-render**. เหตุผล: ใบเสร็จเป็นเอกสารภาษี ณ จุดเวลา ต้อง immutable แม้ template/config เปลี่ยนภายหลัง (สอดคล้อง D-42 freeze เลข + D-43 freeze donor snapshot). ห้ามเก็บ PDF เป็น BLOB ใน DB (CLAUDE.md)

### Email retry / resend / download (FR-27/28, NFR-07)
- **D-57:** **Auto-retry + backoff + manual resend:**
  - worker auto-retry เมื่อ send fail ด้วย **backoff** (จำนวนครั้ง/exponential — planner กำหนดตัวเลข)
  - เกิน max attempts → job/record เป็นสถานะ **`failed`** (dead-letter ในตัว) → **ไม่ retry อัตโนมัติต่อ**
  - staff **เห็นสถานะการส่งในหน้ารายการ** และ **กด resend เองได้** (re-enqueue) — resend **ห้าม allocate เลขใหม่** ใช้ PDF ที่ freeze ไว้ (D-56)
  - staff **ดาวน์โหลด PDF เองได้เสมอ** (กรณีผู้บริจาคไม่มีอีเมล — FR-28 — หรือ email fail ถาวร)
  - บันทึก `email_delivery` record ต่อการส่ง (status/provider message id/attempts/error) เพื่อรองรับ resend (FR-27)

### Config store + Admin settings UI (FR-33/NFR-09)
- **D-58:** **Full config store (DB) + Admin UI ที่แก้ได้ครบ รวม HTML template editor** — admin แก้ template HTML, upload ภาพ (letterhead/seal/signature/watermark), ข้อความ §6 + 1x/2x, รูปแบบเลข ได้โดยไม่ deploy. config เก็บใน DB (พิจารณาต่อยอด/รวมกับ config table ของ Phase 2 ที่เก็บ separator/padding เลข)
  - ⚠️ **SECURITY FLAG (downstream ต้องแก้):** PDF render ผ่าน headless Chromium จาก **admin-supplied HTML** → มี template-injection / stored-XSS / SSRF surface. researcher/planner **ต้อง**ออกแบบ mitigation: (a) render ใน Chromium ที่ **ปิด JavaScript + network isolation** (ไม่ให้โหลด external resource / no outbound), (b) จำกัด/whitelist placeholder ที่ยอมรับ (templating ปลอดภัย ไม่ใช่ raw eval), (c) sanitize/scope asset upload (magic-byte เหมือน slip), (d) จำกัดสิทธิ์แก้ template = Admin เท่านั้น + audit ทุกการแก้. อย่า render HTML ดิบโดยไม่มี sandbox
- **D-61:** **Admin template editor ต้องมี live preview + performance สมูท** (คำขอผู้ใช้):
  - ตอนแก้ template/config admin ต้อง **เห็น preview ผลลัพธ์** ก่อนบันทึก (ไม่แก้แบบตาบอด)
  - **UX ต้องสมูทที่สุด** — ห้าม re-render หนักทุก keystroke; ใช้ **debounce/throttle** + update preview แบบ incremental
  - ⚠️ **จุดตึง fidelity ที่ downstream ต้องตัดสิน (research/planner):** PDF จริง render ผ่าน **headless Chromium** เพราะเป็นทางเดียวที่ Thai shaping ถูกต้อง (D-56 + CLAUDE.md). Live preview ในเบราว์เซอร์ล้วน (iframe HTML) อาจ **ไม่ตรง PDF จริง 100%** (font rendering/ pagination ต่างกัน). ต้องเลือก strategy อย่างใดอย่างหนึ่ง/ผสม:
    - **(a) HTML iframe preview** — เร็ว/สมูท แต่ approximate; ต้องฝัง **@font-face TH Sarabun ตัวเดียวกับ PDF** + sandbox iframe (sandbox attribute, no JS, no network — สอดคล้อง D-58 security) เพื่อให้ใกล้ที่สุด
    - **(b) Server-side render sample เป็น PNG/PDF ผ่าน Chromium เดิม** (debounced, cache) — ตรง PDF จริง แต่ latency สูงกว่า → ต้อง debounce + loading state ให้ยัง "สมูท"
    - **(c) ผสม:** live HTML preview ระหว่างพิมพ์ (a) + ปุ่ม **"เรนเดอร์ PDF จริง"** (b) สำหรับตรวจ final ก่อน publish — **แนะนำ** เพราะได้ทั้งสมูทและความแม่นตอนสำคัญ
  - preview ต้องใช้ **sample/mock donation data** (ไม่ใช่ PII จริง) และ apply **security sandbox เดียวกับ D-58**

### สิทธิลดหย่อน 1x/2x (FR-24, compliance)
- **D-59:** **สิทธิลดหย่อน 1 เท่า/2 เท่า = Global config ระดับโรงพยาบาล** (ค่าเดียวใน config store, ใบเสร็จทุกใบใช้ข้อความ §6 เดียวกัน) — **ไม่มี** field 1x/2x ต่อ donation ใน MVP. เหตุผล: โรงพยาบาลมีสถานะลดหย่อนเดียว; ลดความซับซ้อน schema/UI/validation (แนว simple & safe ต่อเนื่องจาก Phase 2/3). ถ้าอนาคตต้องแยกตามประเภทบริจาค → เพิ่ม field ต่อ donation ภายหลังได้

### Email provider (stakeholder gate — resolved-for-build)
- **D-60:** **Build ต่อ `EmailSender` interface (abstraction) + dev/local implementation** (เช่น log/local SMTP capture) ในเฟสนี้; **provider จริง SES vs Postmark ยังเป็น stakeholder gate** (procurement + PDPA data-residency) → เสียบ implementation ภายหลังโดยไม่แก้ worker. ห้าม self-host SMTP เป็น production path (CLAUDE.md). ต้องเผื่อ deliverability setup (SPF/DKIM/DMARC) เป็น ops item ตอนเลือก provider

### Claude's Discretion
- **Worker trigger model** — polling loop (partial index `idx_outbox_jobs_pending` มีแล้ว) vs LISTEN/NOTIFY vs asynq/River — planner เลือก; DB-backed poll เพียงพอสำหรับ volume โรงพยาบาล (ไม่ต้อง Redis)
- **chromedp vs rod** — ยืนยันด้วย spike (research flag) ก่อน lock; worst-case Thai text เป็นเกณฑ์ตัดสิน
- **จำนวน retry / backoff schedule / max attempts** ที่แน่นอน — planner กำหนด (D-57 กำหนดแค่รูปแบบ)
- **Schema รายละเอียด** ของ `email_delivery` table, config table(s), receipt-PDF reference, `donor_language` column, MinIO bucket/naming — planner ออกแบบตาม pattern Phase 1–3
- **โครงสร้าง package Go** (เช่น `internal/pdf/`, `internal/mailer/`, `internal/worker/`, `internal/settings/`) — ตาม pattern ที่ก่อตัวจาก Phase 1–3
- **รูปแบบ migration 000008+** (donor_language, email_delivery, config/settings, receipt pdf ref)

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project & Requirements
- `.planning/PROJECT.md` — core value (ส่งถึงผู้บริจาคถูกต้องน่าเชื่อถือ), constraints (Compliance §6 1x/2x, i18n, Maintainability no-deploy config), Out of Scope (PKI = รูปภาพลายเซ็น, self-host SMTP)
- `.planning/REQUIREMENTS.md` — FR-20, FR-21, FR-22, FR-24, FR-23, FR-25, FR-26, FR-27, FR-28, NFR-07, FR-33, NFR-09
- `.planning/ROADMAP.md` §"Phase 4" — goal + 5 success criteria + **research flag** (Thai-PDF spike ก่อน lock lib; email deliverability SPF/DKIM/DMARC) + **stakeholder gate** (§6 wording + 1x/2x)

### Load-bearing technical guidance (CLAUDE.md)
- `CLAUDE.md` §"The Two Load-Bearing Decisions" #2 — Thai-script PDF: watermark (CSS/opacity), signature (`<img data:>`), i18n template เดียว, configurable template ใน DB, ⚠️ Phase 4 spike chromedp vs rod
- `CLAUDE.md` §"Email Delivery" — transactional outbox + worker, `email_delivery` record, resend = re-enqueue, ห้าม render/email ใน numbering tx, ห้าม self-host SMTP
- `CLAUDE.md` §"What NOT to Use" — ห้าม render PDF/email ใน issuance tx, ห้าม gofpdf/Maroto สำหรับใบเสร็จไทย, ห้ามเก็บไฟล์เป็น BLOB ใน DB, ห้ามเชื่อ file extension (magic-byte)
- `CLAUDE.md` §"Detail — Supporting Libraries" — chromedp/rod, aws-sdk-go-v2 (SES)/Postmark, background worker (outbox + goroutine), TH Sarabun + `fonts-thai-tlwg`

### Prior phase context (สืบทอด)
- `.planning/phases/03-donation-lifecycle-maker-checker-issuance/03-CONTEXT.md` — **D-42/D-43** (freeze เลข + donor snapshot → หลัก immutability ที่ D-55/D-56 ต่อยอด), issuance tx enqueue outbox (Phase 4 consume), object storage seam (`internal/storage`), audit hash-chain, PII mask/reveal
- `.planning/phases/02-gap-less-receipt-numbering-core/02-CONTEXT.md` — receipt number config (separator/padding) เก็บใน config table 000004 — **base ให้ D-58 config store ต่อยอด/รวม**; `formatReceiptNo`, freeze formatted snapshot

### No additional ADRs/specs
- ยังไม่มี SPEC.md แยกสำหรับเฟสนี้ — decisions ครบใน `<decisions>` ข้างบน

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets (พร้อมใช้จาก Phase 1–3)
- **`internal/db/queries/outbox.sql` + `outbox_jobs` table** (Phase 3): enqueue พร้อมแล้ว (`EnqueueOutboxJob`), status CHECK (pending/processing/done/failed), attempts, last_error, partial index `idx_outbox_jobs_pending`. app role grant SELECT/INSERT/UPDATE. **เฟส 4 เพิ่ม worker queries** (poll `FOR UPDATE SKIP LOCKED`, mark processing/done/failed, bump attempts)
- **`internal/i18n/` (go-i18n bundle + `locales/th.json`,`en.json`)** — reuse สำหรับข้อความ PDF + email subject/body ตาม `donor_language`
- **`internal/storage/client.go` (MinIO)** — reuse สำหรับเก็บ PDF ที่ freeze (D-56); bucket แยกจาก slip
- **`internal/config/config.go`** — env config (Server/DB/Keycloak/KEK/Retention/MinIO). ⚠️ ปัจจุบัน **ไม่มี DB config store / no hot-reload** → D-58 ต้องเพิ่ม DB-backed settings (แยกจาก env config)
- **`internal/audit/`** — audit ทุกการแก้ template/config, resend, download reveal
- **`internal/donation/service.go`** — issuance tx ที่ enqueue outbox (Step 7, บรรทัด ~596); worker อ่าน `donation_id` จาก payload มา build PDF
- **`internal/receiptno/` (format config)** + migration 000004 config table — base ของ D-58 (รูปแบบเลข)
- **testcontainers Postgres** (`internal/testutil/`) — integration test worker + email_delivery + config

### Established Patterns
- data = sqlc + pgx/v5, migration = golang-migrate (ล่าสุด 000007) → เฟส 4 ต่อ **000008+**
- HTTP layer + BFF: Go API (chi/gin ตาม go.mod) + Next.js 15 BFF proxy (`app/api/bff`) + TanStack Query/Table (จาก Phase 3) — Admin settings UI + resend/download endpoints ต่อ pattern นี้
- go-i18n (BE) + next-intl (FE) catalog เดียว
- **Integration-test gate (CLAUDE.md Conventions)** — endpoint ใหม่ (resend/download/settings) ต้องมี E2E ผ่าน real HTTP path + real Keycloak token; worker มี integration test จริง (poll → render → store → send)

### Integration Points (ใหม่ในเฟสนี้)
- worker (goroutine) ↔ `outbox_jobs` (poll/update) ↔ donation record (อ่าน snapshot + `donor_language`) ↔ `receiptno` (เลข formatted ที่ freeze ไว้)
- worker ↔ PDF render (chromedp/rod + Chromium container + TH Sarabun) ↔ config store (template/images/§6/format) ↔ MinIO (freeze PDF)
- worker ↔ `EmailSender` interface ↔ dev/local capture (prod: SES/Postmark ภายหลัง) ↔ `email_delivery` table
- Admin UI ↔ settings API ↔ config store (+ audit); resend/download API ↔ donation + MinIO + email_delivery
- donation create/edit (Phase 3 FE) ↔ `donor_language` field ใหม่

</code_context>

<specifics>
## Specific Ideas

- ผู้ใช้คงแนว **simple & safe / immutability-first** ต่อเนื่องจาก Phase 2/3: freeze PDF (ไม่ re-render), freeze ภาษาใน snapshot, 1x/2x เป็น global config (ไม่ over-engineer ต่อ donation)
- ผู้ใช้เลือก **Full Admin HTML editor** (แก้ layout ได้เต็ม ไม่ใช่แค่ค่าคงที่) — ยอมรับงานใหญ่ขึ้นเพื่อความยืดหยุ่น no-deploy; **แลกมากับ security surface** ที่ downstream ต้อง sandbox Chromium + templating ปลอดภัย (D-58 flag)
- ผู้ใช้ **ยืนยัน Full (ไม่ถอย Lean)** และเพิ่มเงื่อนไข: ตอนแก้ template **ต้องมี live preview** เห็นผลก่อนบันทึก และ **performance ต้องสมูทที่สุด** (D-61) — ให้ความสำคัญกับ editor UX ระดับ production ไม่ใช่แค่ textarea ดิบ
- ผู้ใช้ต้องการ **failure UX ชัดเจนสำหรับ email**: auto-retry แต่ไม่วน infinite, สถานะ `failed` ให้ staff เห็น + resend มือ + ดาวน์โหลดเองได้เสมอ (สะท้อนกระบวนการ back-office จริง)
- resend **ห้าม allocate เลขใหม่** — ผู้ใช้ย้ำหลักความถูกต้องของเลขใบเสร็จต่อเนื่อง (ใช้ PDF/เลขเดิม)

</specifics>

<deferred>
## Deferred Ideas

- **การเลือก email provider จริง (SES vs Postmark) + deliverability (SPF/DKIM/DMARC)** — stakeholder gate; เฟสนี้วาง `EmailSender` interface + dev capture ให้เสียบภายหลัง (D-60)
- **1x/2x ต่อ donation (เลือกต่อรายการ)** — ยังไม่ทำ; MVP ใช้ global config (D-59). เพิ่ม field ภายหลังได้โดยไม่ migrate ของเดิม
- **e-Donation export + reports + backup/restore (FR-30/31/32, NFR-08)** — Phase 5 (settings UI ของ export/report แยกจาก config store เฟสนี้)
- **Flow B public form + acknowledgement email (FR-01..06, FR-05)** — Phase 6
- **PKI digital signature** — เกินขอบเขต MVP (รูปภาพลายเซ็นก่อน)

None อื่น — discussion อยู่ในขอบเขตเฟส

</deferred>

---

*Phase: 4-receipt-pdf-email-delivery-outbox-worker*
*Context gathered: 2026-07-04*
