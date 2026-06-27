# Phase 3: Donation Lifecycle & Maker-Checker Issuance - Context

**Gathered:** 2026-06-27
**Status:** Ready for planning

<domain>
## Phase Boundary

สร้าง **donation record entity + maker-checker workflow** ที่จบด้วยการ **ออกใบเสร็จที่มีเลข
(เรียก allocator ของ Phase 2) ภายใน transaction เดียว** หลังผ่านการอนุมัติโดยมนุษย์ พร้อมจัดเก็บ
donor PII แบบเข้ารหัส + mask ตามบทบาท นี่คือ flow แกนกลางของระบบ (Flow A — Maker สร้างเอง)

**In scope (Phase 3):**
- donation record entity + lifecycle state machine: `draft → pending_review → issued / rejected / cancelled`
- Flow A: Maker สร้าง/แก้รายการ (ขณะ draft), ดูสลิปแนบ, submit เข้า review (FR-07/09/11)
- Maker-Checker approval: Checker อนุมัติ / ตีกลับ / ปฏิเสธ + เหตุผล; **SoD บังคับ `approver_id != created_by`** ใน code **และ** DB check (FR-12/14)
- **Issuance transaction เดียว** ตอน approve: set status=issued + `receiptno.Allocate(ctx, tx, approvedAt)` + เขียน audit row + enqueue outbox job (เรียกผ่าน `db.WithTx`, ตรง D-33)
- donor PII (ชื่อ, เลขภาษี/ปชช., ที่อยู่, อีเมล) จัดเก็บ — **เลขภาษี/ปชช. เข้ารหัส at-rest** ผ่าน `internal/crypto` envelope, **mask ตามบทบาท** ผ่าน `internal/pii` (FR-29, NFR-02 ฝั่ง usage)
- **snapshot donor fields ลง donation** ตอนสร้าง/ออกใบ (immutable tax doc) — ไม่มี donor master/dedup
- ยกเลิกใบเสร็จ issued → สถานะ "ยกเลิก" (เก็บเลข, ไม่ลบ, audited) (FR-19)
- ค้นหา/กรองรายการตาม ชื่อ, ช่วงวันที่, สถานะ, เลขที่ใบเสร็จ (FR-10)
- **object storage (MinIO/S3-compatible) + magic-byte validation + size limit** สำหรับสลิป Flow A (optional upload)
- audit ทุก action สำคัญ (create/submit/return/reject/approve/cancel/pii.reveal) ผ่าน `internal/audit` hash-chain

**Out of scope (เลื่อนไปเฟสอื่น — new capability):**
- PDF generation / email / outbox **worker** ที่ consume job → Phase 4 (Phase 3 แค่ enqueue)
- หน้าจอแก้ template/config รูปแบบเลข, ข้อความลดหย่อน 1 เท่า/2 เท่า (FR-24/33) → Phase 4
- e-Donation export / reports → Phase 5
- **Flow B (public donation form + slip upload จากผู้บริจาค)** → Phase 6 (เฟสนี้สร้าง object storage seam ให้ Phase 6 reuse)
- donor master / dedup / blind index / per-donor rollup → future (เพิ่มภายหลังได้โดยไม่ migrate snapshot)
- คิว "รอตรวจสอบ" จากเว็บ (FR-08) → Phase 6 (Flow B). Phase 3 มี pending_review queue ของ Flow A เท่านั้น

</domain>

<decisions>
## Implementation Decisions

### Donor data model (FR-29)
- **D-43:** **Snapshot-only** — แต่ละ donation เก็บ donor fields ของตัวเอง (ชื่อ/ที่อยู่/อีเมล + เลขภาษีเข้ารหัส) **ไม่มี donor master table / ไม่มี blind index** ในเฟสนี้. เหตุผล: ใบเสร็จเป็นเอกสารภาษี ณ จุดเวลา ต้อง freeze donor identity อยู่แล้ว (หลักเดียวกับ D-42 freeze เลข); e-Donation export (Phase 5) และ report ตามช่วงเวลา/ยอดรวมทำงานบน per-donation snapshot ได้ครบ; dedup/auto-fill/per-donor rollup เพิ่มทีหลังได้โดยไม่ migrate snapshot เดิม
- **D-44:** **เลขภาษี/บัตร ปชช. บังคับกรอกเสมอ** — ไม่มีเลข = ออกใบเสร็จไม่ได้ (validation ที่ขอบ API + DB NOT NULL บน ciphertext). เหตุผล: ต้องมีเลขถึงจะคีย์เข้า e-Donation RD ได้ (FR-30 downstream)

### Approval / rejection workflow (FR-11, FR-12, FR-14)
- **D-45:** **แยก 2 action ที่ Checker ทำได้เมื่อไม่อนุมัติ:**
  - **ตีกลับเพื่อแก้ (return)** → record กลับสู่ `draft`, Maker แก้แล้ว resubmit ได้ (loop ได้ไม่จำกัดรอบ) — non-terminal
  - **ปฏิเสธถาวร (reject)** → `rejected` (terminal, รายการไม่ถูกต้อง/ซ้ำ — ไม่ออกเป็นใบเสร็จ)
  - **ทั้งสอง action บังคับระบุเหตุผล** (เก็บใน record + audit)
- **State machine:** `draft —submit→ pending_review`; `pending_review —return+reason→ draft`; `pending_review —reject+reason→ rejected`; `pending_review —approve(approver≠creator)→ issued`; `issued —cancel+reason→ cancelled`. `rejected`/`cancelled` เป็น terminal
- **SoD (locked จาก CLAUDE.md):** Checker ห้าม approve record ที่ตัวเองสร้าง — บังคับ `approver_id != created_by` ทั้งใน guard/service **และ** DB check (defense-in-depth)
- **Receipt number เกิดเฉพาะตอน issued** — draft/pending/rejected/cancelled ไม่มีเลข (cancelled เก็บเลขที่เคยออก); allocate เป็น code path เดียวตอน issuance commit (D-35)

### PII reveal & masking (FR-29, NFR-02)
- **D-46:** **reveal เลขเต็ม = Checker + Admin เท่านั้น** (Maker เห็นเลขเต็มเฉพาะตอนกรอก/แก้ draft ของตัวเอง; หลัง submit เห็น mask). ตรงกับ Phase 1 **D-10** (`pii.CanRevealFull`) — **ไม่ต้องสร้างใหม่ reuse ของเดิม**
- **Mask format (locked Phase 1):** `pii.MaskNationalID` แสดง 4 ตัวท้าย (`x-xxxx-xxxxx-x1234` สำหรับเลข 13 หลัก) — เป็น default ทุกที่
- **Reveal flow (locked Phase 1, D-13):** `CanRevealFull` gate → 403 ถ้า false → `crypto.DecryptField` → เขียน audit `action="pii.reveal"` → คืน plaintext. ทุก reveal ถูก audit

### Receipt cancellation (FR-19)
- **D-47:** **ยกเลิกใบเสร็จ issued = Checker + Admin** (Maker ยกเลิกไม่ได้), **บังคับระบุเหตุผล + audited**. ยกเลิก = set status "ยกเลิก"/`cancelled` — **เก็บเลขเดิมไว้ ไม่ลบ ไม่เกิด gap** (number ยังครองอยู่ใน ledger ของ Phase 2)

### Slip upload — Flow A (FR-09; seam ให้ FR-02 Phase 6)
- **D-48:** **แนบสลิปได้แต่ไม่บังคับ (optional)** ใน Flow A (รองรับเงินสด/ไม่มีสลิป). **สร้าง object storage (MinIO/S3-compatible) + magic-byte validation (ตรวจ content ไม่ใช่ extension) + size limit ในเฟสนี้** เพราะ ROADMAP SC#1 ระบุ "view any attached slip"; Phase 6 (Flow B public upload) **reuse** seam นี้. เก็บไฟล์ใน object storage + เก็บ reference ใน DB (ห้ามเก็บ BLOB ใน DB)

### Claude's Discretion
- schema รายละเอียดของ donation/receipt entity (ชื่อ column, FK ไป ledger `receipt_numbers` ตาม D-38, index สำหรับ search FR-10) — planner ออกแบบ
- โครงสร้าง package ฝั่ง Go (เช่น `internal/donation/`, `internal/storage/`) ตาม pattern ที่ก่อตัวจาก Phase 1
- รูปแบบ migration 000005+ (donation tables, FK, SoD CHECK, status enum/constraint)
- object storage client library (`minio-go` ตาม CLAUDE.md) + endpoint/bucket config — planner/researcher เลือกตาม pattern config เดิม
- รูปแบบ outbox table + enqueue (transactional outbox ตาม CLAUDE.md) — Phase 3 เขียน enqueue, worker เป็น Phase 4
- ขอบเขต validation donor fields อื่น (อีเมล/ที่อยู่ format) — ตามมาตรฐานบัญชี

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project & Requirements
- `.planning/PROJECT.md` — core value (gap-less เลข + ทุกใบผ่านอนุมัติมนุษย์), constraints (Security/PDPA, Audit, Correctness)
- `.planning/REQUIREMENTS.md` — FR-07, FR-09, FR-11, FR-10, FR-12, FR-14, FR-19, FR-29 + traceability (NFR-02/NFR-03 secondary)
- `.planning/ROADMAP.md` §"Phase 3" — goal + 5 success criteria (donation lifecycle, SoD, issuance tx, cancel retains number, encrypted+masked PII + search)

### Load-bearing technical guidance (CLAUDE.md)
- `CLAUDE.md` §"Auth & RBAC" — SoD (`approver_id != created_by` ใน code + DB), audit append-only hash-chain
- `CLAUDE.md` §"PII Encryption-at-Rest" — envelope AES-256-GCM, blind index (เฉพาะถ้า lookup ด้วย national ID — **เฟสนี้ไม่ต้อง** ตาม D-43), role-based masking, ทุก reveal audit
- `CLAUDE.md` §"What NOT to Use" — ห้าม render PDF/email ใน issuance transaction (แค่ enqueue outbox), ห้ามเก็บ slip เป็น BLOB ใน DB (ใช้ object storage), ห้ามเชื่อ file extension (magic-byte), ห้าม pre-compute เลข
- `CLAUDE.md` §"Email Delivery" — transactional outbox + worker (Phase 3 enqueue, Phase 4 consume)

### Prior phase context (สืบทอด)
- `.planning/phases/02-gap-less-receipt-numbering-core/02-CONTEXT.md` — **D-33** (allocator caller-managed tx, Phase 3 ห่อด้วย `db.WithTx`), **D-34** (allocator คืน fiscal_year/running_no/formatted), **D-35** (code path เดียว, ห้าม pre-compute), **D-36** (bubble error, caller จัดการ rollback), **D-38** (receipts อ้างอิง ledger), **D-42** (freeze formatted snapshot)
- `.planning/phases/01-foundation-db-auth-rbac-audit-retention-model/01-CONTEXT.md` — D-10..D-14 (PII mask/reveal/audit), auth/RBAC foundation

### No additional ADRs/specs
- ยังไม่มี SPEC.md แยกสำหรับเฟสนี้ — decisions ครบใน `<decisions>` ข้างบน

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets (Phase 1 + Phase 2 — พร้อมใช้)
- **`internal/receiptno/`** (Phase 2): `Allocator.Allocate(ctx, tx, issueDate)` — เรียกใน issuance tx ของเฟสนี้ (D-33). คืน `AllocatedReceipt{FiscalYear, RunningNo, Formatted}`
- **`internal/crypto/`** (Phase 1): `EncryptField`/`DecryptField` envelope AES-256-GCM + key provider — ใช้เข้ารหัสเลขภาษี/ปชช. ของ donor (D-44)
- **`internal/pii/mask.go`** (Phase 1): `MaskNationalID` (last-4) + `CanRevealFull(claims)` (Admin+Checker) — wire เข้า donor record ตรงๆ (D-46), **ห้ามสร้างใหม่**
- **`internal/audit/`** (Phase 1): service + middleware + hash-chain immutable — เขียน audit row ใน issuance tx + ทุก action (รวม `pii.reveal`)
- **`internal/auth/`** (Phase 1): Keycloak claims + `HasRole(RoleMaker/RoleChecker/RoleAdmin)` — RBAC guard + SoD
- **`internal/db/helpers.go`** — `db.WithTx(ctx, pool, fn)` seam ห่อ issuance tx (allocate + status + audit + enqueue ใน commit เดียว)
- **sqlc + pgx/v5** (`internal/db/`): เพิ่ม query donation/receipt + named params + RETURNING
- **golang-migrate** (`migrations/`, ล่าสุด 000004) → migration ถัดไป **000005+** (donation tables, SoD CHECK, status constraint, FK→`receipt_numbers`)
- **testcontainers Postgres** (`internal/testutil/postgres.go`): integration test issuance tx + concurrency (approve พร้อมกัน) + SoD

### Established Patterns
- data = sqlc + pgx/v5, migration = golang-migrate, Go 1.25.1
- HTTP layer: ตรวจ router ที่ใช้จริง (02-CONTEXT ระบุ gin ใน go.mod) — เฟสนี้เริ่มแตะ HTTP handler (Flow A back-office API) ครั้งแรกหลัง Phase 1 auth
- query: named params (`@x`), explicit column list, RETURNING

### Integration Points (ใหม่ในเฟสนี้)
- donation entity ↔ issuance tx (`db.WithTx`) ↔ `receiptno.Allocate` (Phase 2) ↔ ledger `receipt_numbers` (FK, D-38)
- donor field encrypt ↔ `crypto.EncryptField`; reveal ↔ `pii.CanRevealFull` + `crypto.DecryptField` + audit
- approval action ↔ SoD guard (`auth` claims) + DB CHECK
- slip ↔ object storage client (ใหม่, `internal/storage/`?) + magic-byte validation
- issuance ↔ outbox table (enqueue เท่านั้น; worker = Phase 4)

</code_context>

<specifics>
## Specific Ideas

- ผู้ใช้เลือก **simple & safe** ต่อเนื่องจาก Phase 2: snapshot-only (ไม่ over-engineer dedup), บังคับเลขภาษี, role-restrict ทั้ง reveal และ cancel ที่ Checker+Admin
- ผู้ใช้แยกชัด **"ตีกลับเพื่อแก้" (loop) vs "ปฏิเสธถาวร"** — สะท้อนกระบวนการจริงของ back-office (แก้รอบเล็กน้อย vs รายการใช้ไม่ได้)
- ผู้ใช้ให้น้ำหนัก **immutability/audit ของเอกสารภาษี** (freeze donor snapshot, ยกเลิกไม่ลบเลข) — ต่อเนื่องจาก D-42
- ผู้ใช้ยอมรับการ **front-load object storage** ใน Phase 3 เพื่อให้ตรง SC "view attached slip" และ Phase 6 reuse

</specifics>

<deferred>
## Deferred Ideas

- **Donor master + dedup + blind index + per-donor rollup/auto-fill** — เพิ่มภายหลังได้โดยไม่ migrate snapshot (D-43); ยังไม่อยู่ใน requirement ของ milestone
- **Flow B public donation form + slip upload จากผู้บริจาค + pending-review queue จากเว็บ (FR-08, FR-01..06)** — Phase 6; เฟสนี้วาง object storage seam ให้ reuse
- **PDF/email/outbox worker (FR-20..28, NFR-07)** — Phase 4; เฟสนี้แค่ enqueue
- **ข้อความลดหย่อน 1 เท่า/2 เท่า + template/config UI (FR-24, FR-33, NFR-09)** — Phase 4
- **e-Donation export + reports + "คีย์แล้ว" flag (FR-30/31/32)** — Phase 5

None อื่น — discussion อยู่ในขอบเขตเฟส

</deferred>

---

*Phase: 3-donation-lifecycle-maker-checker-issuance*
*Context gathered: 2026-06-27*
