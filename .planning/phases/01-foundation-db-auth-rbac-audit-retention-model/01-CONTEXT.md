# Phase 1: Foundation (DB, Auth/RBAC, Audit, Retention model) - Context

**Gathered:** 2026-06-23
**Status:** Ready for planning

<domain>
## Phase Boundary

วางรากฐานความปลอดภัยที่ทุกเฟสพึ่งพา: ระบบ login + แยกบทบาท Maker/Checker/Admin (RBAC),
audit trail แบบลบไม่ได้ (append-only), data model ที่ฝัง retention (PDPA vs ภาษี) ตั้งแต่แรก,
และขอบเขตการเข้ารหัส PII + transport TLS

**In scope (Phase 1):**
- การ login + นโยบายรหัสผ่าน/lockout/session (authN ผ่าน Keycloak)
- การจัดการผู้ใช้ + การกำหนดบทบาท (RBAC) + การบังคับสิทธิ์ฝั่ง server (authZ ในแอป)
- Audit log แบบ append-only + tamper-evidence
- Retention/legal-basis data model (`retain_until`, `legal_basis`, `legal_hold`)
- ขอบเขตการเข้ารหัส PII (envelope/KeyProvider abstraction) + TLS
- โครง i18n message catalog

**Out of scope (เลื่อนไปเฟสอื่น — เป็น new capability):**
- การ encrypt/decrypt/mask ข้อมูลผู้บริจาคจริง (donor module → Phase 3; Phase 1 วางแค่ boundary)
- การเก็บ consent ของ donor (Flow A → Phase 3, Flow B → Phase 6)
- gap-less receipt numbering (Phase 2)
- donation lifecycle / maker-checker issuance transaction (Phase 3)
- audit ของ domain CRUD รายการบริจาค (กลไก audit วางที่นี่ แต่ entity บริจาคมา Phase 3)

</domain>

<decisions>
## Implementation Decisions

### ผู้ใช้ & โมเดลบทบาท
- **D-01:** Admin เท่านั้นที่สร้าง/จัดการบัญชีผู้ใช้ — ไม่มี self-signup ในเฟสนี้
- **D-02:** **Multi-role** — 1 คนถือได้หลายบทบาท (ไม่ใช่ 1-คน-1-บทบาท), Admin ทำได้ทุกอย่าง (รวมหน้าที่ Maker/Checker)
- **D-03:** **⚠️ DEVIATION จาก ROADMAP SC#1** — Success Criterion ข้อ 1 เขียน "assign exactly one of the Maker/Checker/Admin roles" แต่ผู้ใช้เลือก multi-role → SC#1 ต้องอัปเดต roadmap หลังเฟสนี้ (ดู Deferred → roadmap edit)
- **D-04:** **Segregation of Duties บังคับระดับรายการ ไม่ใช่ระดับบทบาท** — กฎ `approver_id ≠ created_by` ต่อ record (enforce จริงใน Phase 3). คนถือทั้ง Maker+Checker สร้าง record เองได้ แต่อนุมัติใบที่ตัวเองสร้างไม่ได้ → SoD ยังครบแม้ multi-role
- **D-05:** Bootstrap admin คนแรกผ่าน seed script / env (ไม่มีหน้า setup wizard)

### นโยบายความปลอดภัย login
- **D-06:** รหัสผ่าน ≥ 8 ตัว ผสมตัวอักษร/ตัวเลข; hash ด้วย argon2id (ถ้า password อยู่ในแอป) — แต่ดู D-15: authN ย้ายไป Keycloak ดังนั้น password policy/hashing บังคับใน Keycloak realm config
- **D-07:** Lockout ชั่วคราวหลัง login ผิด N ครั้ง (เช่น 5 ครั้ง → ล็อกชั่วคราว) — config ใน Keycloak
- **D-08:** Session หมดอายุตามเวลา + idle timeout (เช่น token ~8 ชม. + idle ~30 นาที) — เหมาะกับ back-office ที่มี PII
- **D-09:** ไม่มี MFA/OTP ใน MVP (Keycloak เปิดเพิ่มได้ภายหลังโดยไม่แก้แอป)

### การเห็น/เปิดเลขบัตร ปชช. (PII visibility policy)
- **D-10:** บทบาทที่เห็นเลขเต็มได้: **Admin + Checker** (Maker เห็นแค่ค่า mask) — least privilege
- **D-11:** Mask = โชว์ 4 ตัวท้าย (เช่น `x-xxxx-xxxxx-x5-67`)
- **D-12:** Reveal แบบ **just-in-time** — default แสดง mask, ต้องกดปุ่ม "แสดงเลขเต็ม" ทีละราย
- **D-13:** การ reveal เลขเต็ม **ต้อง audit ทุกครั้ง** (ใคร/เมื่อไร/record ไหน) — หลักฐาน PDPA
- **D-14:** หมายเหตุ: Phase 1 วาง "นโยบาย + กลไก masking/reveal/audit-on-reveal" ให้พร้อม; การใช้กับข้อมูล donor จริงเกิด Phase 3

### Audit log & retention
- **D-15:** Audit scope = **ทุก mutation ในระบบ** (generic audit interceptor/pattern) + auth events (login/logout/login ผิด/สร้าง-แก้-ปิดผู้ใช้/เปลี่ยน role/reveal PII) — Phase 1 entity มีน้อย (user/role/config/retention) จึงครอบคลุมง่าย และขยายอัตโนมัติเมื่อมี entity ใหม่ใน Phase 3+
- **D-16:** ดู audit log ได้: **Admin เท่านั้น**
- **D-17:** Tamper-evidence = **DB revoke UPDATE/DELETE (append-only) + hash-chain** ต่อแถว (เก็บ hash ของแถวก่อน) — ตรวจจับการแก้ไขแม้ระดับ DBA
- **D-18:** Retention model = **config-driven ไม่ hardcode** — เก็บ `retain_until` ต่อ record + `legal_basis` (enum) + `legal_hold` (flag); ค่า default ระยะเก็บ (~5 ปี) อ่านจาก config รอ DPO ยืนยัน
- **D-19:** ไม่มี code path ใด hard-delete record ที่อยู่ภายใต้ `legal_hold` (เฉพาะ soft-delete/สถานะ); enforce ทั้งในแอปและระดับ DB เป็น defense-in-depth

### Tech Stack & Architecture
- **D-20:** **Backend = Go (golang)** — ⚠️ override คำแนะนำ CLAUDE.md ทั้งหมด (ซึ่งอิง NestJS/TypeScript). เหตุผลหลัก 3 เรื่องของโปรเจกต์ยัง transfer ครบ: gap-less counter ด้วย row-lock (`SELECT … FOR UPDATE`), app-level AES-256-GCM, Thai PDF ผ่าน headless Chromium
- **D-21:** Database = **PostgreSQL** (17 ตามแนะนำ; 16+ รับได้) — จำเป็นต่อ row-level locking สำหรับ gap-less counter
- **D-22:** Frontend = **React / Next.js** (back-office UI; Phase 6 public form ใช้ตัวเดียวกัน)
- **D-23:** **Data layer ฝั่ง Go ให้ research เลือก** — Prisma ไม่รองรับ Go (client deprecated) → เทียบ sqlc / pgx / GORM / ent โดยให้น้ำหนักความสามารถคุม `SELECT … FOR UPDATE` เองสำหรับ Phase 2 (sqlc เป็น candidate แข็งแรง). ตัดสินใน research/plan
- **D-24:** **Auth = Hybrid** — Keycloak ทำ authN (login, password policy, lockout, session, role assignment, MFA-ready) ผ่าน OIDC; **แอป Go ทำ authZ ละเอียดเอง** (RBAC guard + SoD ต่อ record + PII masking + audit). Keycloak ให้แค่ "ใครมี role อะไร" — logic ธุรกิจ (SoD/PII/audit hash-chain) อยู่ในแอปเสมอ
- **D-25:** **Encryption/KMS = ออกแบบ envelope + KeyProvider abstraction** (DEK เข้ารหัส field, KEK wrap DEK ผ่าน interface) — MVP ผูก KeyProvider กับ env/secrets manager, สลับไป cloud KMS/HSM ได้ภายหลังโดยไม่แก้ call site. ต้องมี blind index (keyed HMAC) สำหรับค้นด้วยเลขบัตรในเฟสถัดไป — Phase 1 เผื่อ schema/interface ไว้
- **D-26:** **Hosting = Docker-based, รัน local ก่อน, ออกแบบให้ย้าย cloud ได้** — ไม่ผูก cloud-specific service ใน MVP (เช่น object storage ใช้ MinIO/local, KMS ใช้ env). Keycloak self-hosted ผ่าน Docker เข้ากับแนวนี้
- **D-27:** **i18n วางโครงตั้งแต่ Phase 1** — error/validation message ของ auth ใช้ message key/catalog (ไทย/อังกฤษ) ตั้งแต่แรก ไม่ retrofit ภายหลัง

### Claude's Discretion
- รายละเอียด schema ของตาราง (users, roles, audit_log, retention fields, key metadata) — ให้ planner ออกแบบตาม decisions ข้างบน
- รูปแบบ message-catalog / โครง i18n ฝั่ง Go และฝั่ง Next.js — research/plan เลือก library
- ค่า config เฉพาะเจาะจง (จำนวนครั้ง lockout, อายุ token/idle ที่แน่นอน, default retain_until) — เริ่มด้วยค่าเหมาะสม ปรับผ่าน config ได้

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project & Requirements
- `.planning/PROJECT.md` — core value, constraints, key decisions, out-of-scope
- `.planning/REQUIREMENTS.md` — รายละเอียด NFR-01, FR-34, NFR-02, NFR-05, FR-13, NFR-03 + traceability
- `.planning/ROADMAP.md` §"Phase 1" — goal + 5 success criteria + stakeholder gates
- `CLAUDE.md` — stack guidance (อิง NestJS; ⚠️ backend ถูก override เป็น Go ตาม D-20 — ใช้เป็น reference เหตุผล/pattern ได้ แต่ library ฝั่ง backend ให้ research หา Go equivalent)

### Source requirement document
- `requirements-ระบบออกใบเสร็จบริจาค.md` (v1.1 Draft, 22 มิ.ย. 2569) — เอกสารต้นทางจาก stakeholder (ดู §6 สำหรับ Phase 4; §10 phasing)

### Stakeholder gates relevant to Phase 1 (non-blocking — confirm at phase start)
- PDPA retention period (~5 ปี vs erasure) + erasure policy → ฝ่ายกฎหมาย/DPO (ดู REQUIREMENTS.md NFR-03; model `retain_until` generically จนกว่าจะยืนยัน)
- KMS/secrets store + hosting (on-prem vs cloud) → ฝ่าย IT/จัดซื้อ (Phase 1 ออกแบบ KeyProvider abstraction รอตัดสิน — D-25/D-26)

### No additional ADRs/specs
- ยังไม่มี ADR แยกในโปรเจกต์ — decisions ที่ตัดสินจับไว้ใน `<decisions>` ข้างบนครบถ้วน

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- Greenfield — ยังไม่มีโค้ดในโปรเจกต์ (มีแต่ `.planning/` docs และ config). Phase 1 เป็นการ scaffold ระบบครั้งแรก

### Established Patterns
- ยังไม่มี codebase map (`.planning/codebase/` ว่าง) — pattern จะเริ่มก่อตัวที่ Phase 1 นี้
- การตัดสิน Go + Hybrid Keycloak + envelope encryption จะกลายเป็น foundational pattern ให้ทุกเฟสถัดไป

### Integration Points
- Keycloak (OIDC) ↔ Go backend: token validation, role claim mapping
- Go backend ↔ PostgreSQL: data layer (TBD ใน research) + raw-SQL row lock path สำหรับ Phase 2
- KeyProvider interface ↔ secrets/env (MVP) → cloud KMS (future)
- i18n catalog ↔ ทั้ง Go (API/validation messages) และ Next.js (UI)

</code_context>

<specifics>
## Specific Ideas

- ผู้ใช้ระบุชัดว่าต้องการ backend เป็น **Go** และใช้ **Keycloak** สำหรับส่วน authN/RBAC แม้ CLAUDE.md จะแนะ NestJS — เป็นความตั้งใจที่ downstream ต้องเคารพ
- ผู้ใช้ต้องการ **hash-chain** audit (มากกว่าค่าแนะนำ append-only เปล่า ๆ) — สะท้อนความสำคัญของ tax/accounting audit integrity
- ผู้ใช้ต้องการ **audit ทุก mutation** (กว้างกว่าค่าแนะนำ) — ออกแบบเป็น generic interceptor ตั้งแต่แรก
- ผู้ใช้ต้องการรัน Docker บนเครื่องตัวเองก่อน แล้วย้าย cloud ได้ → ต้องคุม portability เป็นข้อจำกัดออกแบบ

</specifics>

<deferred>
## Deferred Ideas

- **อัปเดต ROADMAP SC#1** — แก้ success criterion ข้อ 1 ของ Phase 1 จาก "exactly one role" เป็น multi-role + SoD ระดับรายการ (ดู D-03). ทำหลังเฟสนี้ผ่าน `/gsd-phase` edit หรือ transition
- **MFA/OTP** — ไม่ทำใน MVP แต่ Keycloak รองรับ เปิดได้ภายหลังโดยไม่แก้แอป (D-09)
- **Cloud KMS / HSM** — ผูก KeyProvider กับ cloud KMS เมื่อ stakeholder ตัดสิน hosting (D-25/D-26)
- **Blind index ค้นด้วยเลขบัตร** — กลไกเต็มใช้ใน Phase 3 (donor search); Phase 1 เผื่อ schema/interface
- **Donor PII encrypt/decrypt/mask usage** — ลงมือจริง Phase 3 (NFR-02 ส่วนที่สอง)
- **Consent capture** — Flow A → Phase 3, Flow B → Phase 6 (ผูกกับ retention model ของ Phase 1)

</deferred>

---

*Phase: 1-foundation-db-auth-rbac-audit-retention-model*
*Context gathered: 2026-06-23*
