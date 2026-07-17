# DonaRec — ระบบออกใบเสร็จบริจาคอัตโนมัติสำหรับโรงพยาบาล

## What This Is

ระบบเว็บสำหรับโรงพยาบาลในการออกใบเสร็จรับเงินบริจาคในรูปแบบ PDF ที่ถูกต้องตามข้อกำหนดการลดหย่อนภาษี
ส่งให้ผู้บริจาคทางอีเมลอัตโนมัติ และจัดเก็บข้อมูลผู้บริจาคอย่างเป็นระบบเพื่อรองรับการคีย์เข้าระบบ
e-Donation ของกรมสรรพากร โดยทุกใบเสร็จต้องผ่านการตรวจสอบและอนุมัติโดยเจ้าหน้าที่ก่อนออกเสมอ
มาแทนกระบวนการแมนวลที่ใช้อยู่ในปัจจุบัน

ผู้ใช้งานหลัก: เจ้าหน้าที่โรงพยาบาล (Maker), ผู้อนุมัติ (Checker), ผู้ดูแลระบบ (Admin) และผู้บริจาค (ในเฟสเว็บสาธารณะ)

## Core Value

ออกใบเสร็จบริจาคที่มี **เลขที่รันต่อเนื่องไม่ซ้ำ ห้ามข้ามเลข (gap-less) ตามปีงบประมาณ** หลังผ่านการอนุมัติโดยมนุษย์
และส่งถึงผู้บริจาคได้อย่างถูกต้องน่าเชื่อถือ — ความถูกต้องของเลขใบเสร็จและการควบคุมการอนุมัติคือหัวใจที่ห้ามพลาด

## Requirements

### Validated

<!-- Shipped and confirmed valuable. -->

ครบทั้ง 47 requirements (34 FR + 13 NFR) — shipped ใน **v1.0 MVP** (Phases 1-7, 2026-07-17). audit 47/47 SATISFIED. รายละเอียด traceability: `milestones/v1.0-REQUIREMENTS.md`.

**Flow B — เว็บรับบริจาคสาธารณะ**
- ✓ FR-01 กรอกฟอร์มบริจาค / FR-02 อัปโหลดสลิป (magic-byte) / FR-03 PDPA consent / FR-04 Turnstile + rate-limit / FR-05 ack email (ยังไม่ใช่ใบเสร็จ) / FR-06 เลือกภาษา — v1.0 (Phase 6)

**Back office + อนุมัติ**
- ✓ FR-07 Flow A / FR-08 คิวรอตรวจสอบ / FR-09 ดู-แก้ + สลิป / FR-10 ค้นหา-กรอง / FR-11 lifecycle / FR-12 approve-return / FR-13 audit trail / FR-14 ออกใบเสร็จหลังอนุมัติเท่านั้น — v1.0 (Phase 1/3/6)

**เลขที่ใบเสร็จ gap-less ★**
- ✓ FR-15 รูปแบบเลข / FR-16 gap-less / FR-17 reset ปีงบใหม่ / FR-18 fiscal year จากวันอนุมัติ / FR-19 ยกเลิกไม่ลบเลข — v1.0 (Phase 2/3)

**PDF + อีเมล**
- ✓ FR-20 template+letterhead / FR-21 watermark / FR-22 signature / FR-23 ไทย-อังกฤษ / FR-24 ข้อความลดหย่อน (config) / FR-25 ส่ง PDF / FR-26 อีเมล 2 ภาษา / FR-27 สถานะส่ง+resend / FR-28 staff download — v1.0 (Phase 4)

**e-Donation + รายงาน + ตั้งค่า**
- ✓ FR-29 จัดเก็บข้อมูล (PII encrypted) / FR-30 export xlsx/csv / FR-31 keyed flag + aging / FR-32 รายงานสรุป / FR-33 admin config no-deploy / FR-34 RBAC — v1.0 (Phase 5/4/1)

**NFR**
- ✓ NFR-01 auth+RBAC / NFR-04 concurrency-safe gap-less (proven under -race) / NFR-05 audit retention / NFR-07 PDF+email latency / NFR-08 backup+restore (verified) / NFR-09 config no-deploy — v1.0
- ✓ NFR-03 PDPA consent+retention model — v1.0 (Phase 1/3/6)
- ✓ NFR-06 responsive + bilingual UI — v1.0 code-complete (Phase 6) — ⚠️ human walkthrough UAT 3/3 passed; interactive-login browser checkpoint deferred (see Constraints/override)
- ✓ NFR-02 PII encryption-at-rest (AES-256-GCM envelope) — v1.0 — ⚠️ HTTPS/TLS transport = deploy-time verification (deferred)

### Active

<!-- Current scope for NEXT milestone. Fresh requirements defined via /gsd-new-milestone. -->

(ยังไม่กำหนด — v1.0 ปิดแล้ว; รอบถัดไปเริ่มด้วย `/gsd-new-milestone`)

**Deferred verification จาก v1.0 (ต้องปิดตอน deploy จริง):**
- [ ] NFR-02 — ยืนยัน HTTPS/TLS + reverse-proxy + `sslmode=verify-full` บน production (deploy-time)
- [ ] auth-gating — interactive Keycloak browser login walkthrough (โค้ดเสร็จ + TDD-green แล้ว; เหลือ human verify)

### Out of Scope

<!-- Explicit boundaries. Includes reasoning to prevent re-adding. -->

- ใบเสร็จทั่วไป (non-donation) — เลื่อนไปเฟสถัดไป (เฟส 3) ตามเอกสาร
- บริจาคสิ่งของ (in-kind) — ไม่รองรับ เฟสนี้เน้นเงินบริจาคเท่านั้น
- การเชื่อมต่อ payment gateway — ปัจจุบันยืนยันเงินเข้าโดยเจ้าหน้าที่ตรวจสลิปแมนวล 100%
- การเชื่อมต่ออัตโนมัติกับระบบบัญชีภายในเดิม — เป็นระบบแยกต่างหากในเฟสนี้
- การเชื่อมต่อ API ตรงกับ e-Donation กรมสรรพากร — เจ้าหน้าที่คีย์ข้อมูลเอง ระบบเพียงเตรียม export
- PKI digital signature — เฟส MVP ใช้รูปภาพลายเซ็นก่อน; PKI พิจารณาภายหลังหากต้องการความน่าเชื่อถือทางกฎหมาย

## Context

- มาแทนกระบวนการออกใบเสร็จบริจาคแบบแมนวลที่โรงพยาบาลใช้อยู่ปัจจุบัน
- เป็น **ระบบแยกต่างหาก** จากระบบบัญชีภายในเดิม (ไม่เชื่อมต่ออัตโนมัติในเฟสนี้)
- มีกระบวนการ 2 รูปแบบ: Flow A (เจ้าหน้าที่สร้างรายการเอง) และ Flow B (ผู้บริจาคทำผ่านเว็บ แล้วเจ้าหน้าที่ตรวจสลิป)
- หลักการสำคัญ: **ทุกใบเสร็จต้องผ่านการตรวจสอบโดยมนุษย์ก่อนส่งเสมอ** — ไม่มีการออกใบเสร็จอัตโนมัติทันทีโดยไม่อนุมัติ
- ปีงบประมาณ = รอบ 1 ต.ค. – 30 ก.ย. (เอกสารที่ออก ต.ค.–ธ.ค. นับเป็นปีงบประมาณถัดไป)
- ข้อมูลอ่อนไหว (เลขบัตร ปชช./เลขผู้เสียภาษี) อยู่ภายใต้ PDPA และต้องเข้ารหัสจัดเก็บ
- ข้อขัดแย้งที่ต้องระวัง: กฎหมายภาษีต้องเก็บเอกสารอย่างน้อย ~5 ปี ซึ่งอาจขัดสิทธิขอลบของ PDPA → ต้องมีนโยบายชัดเจน
- เอกสารต้นทางอ้างอิง: `requirements-ระบบออกใบเสร็จบริจาค.md` (เวอร์ชัน 1.1 Draft, 22 มิ.ย. 2569)

**Shipped state (v1.0 MVP, 2026-07-17):**
- Stack ที่ build จริง: Go backend (gin + sqlc/pgx) + Keycloak OIDC + PostgreSQL 17 + Next.js 15 (App Router) + MinIO + headless Chromium (chromedp). ~41k LOC (Go + TS/TSX, non-test).
- 7 phases, 47 plans, 91 tasks, 25 วัน (22 มิ.ย. – 17 ก.ค. 2569), 418 commits.
- Flow A (staff) และ Flow B (public web) ทั้งคู่ไหลผ่าน approval pipeline เดียวกัน; ออกเลข gap-less ในจังหวะ commit เดียว
- Known deferred (non-blocking): HTTPS/TLS = deploy-time; frontend interactive-login browser walkthrough = human checkpoint (โค้ดเสร็จ)

## Constraints

- **Security/PDPA**: เข้ารหัสข้อมูลอ่อนไหวขณะจัดเก็บ + HTTPS/TLS + จำกัดการเห็นเลขบัตร ปชช. ตามบทบาท — ข้อกำหนดทางกฎหมาย
- **Correctness**: เลขที่ใบเสร็จต้อง gap-less และ concurrency-safe — ต้องออกเลขในจังหวะ commit ของ DB (sequence/atomic counter ระดับ transaction) ไม่ใช่คำนวณล่วงหน้า
- **Compliance**: รูปแบบ/ข้อความใบเสร็จต้องถูกต้องตามประมวลรัษฎากร (รวมเงื่อนไขลดหย่อน 1 เท่า/2 เท่า) — ต้องยืนยันกับฝ่ายบัญชี/กฎหมายโรงพยาบาล
- **Audit**: ทุกการกระทำสำคัญต้องมี audit trail ที่ลบไม่ได้ เพื่อการตรวจสอบบัญชี/ภาษี
- **i18n**: รองรับไทย/อังกฤษ ทั้ง UI, PDF และอีเมล
- **Integration**: ไม่มี payment gateway / ไม่ต่อ API e-Donation โดยตรง — export แมนวลเท่านั้น
- **Maintainability**: เทมเพลต/ลายเซ็น/รูปแบบเลข ตั้งค่าได้โดยไม่ต้อง deploy

## Key Decisions

<!-- Decisions that constrain future work. Add throughout project lifecycle. -->

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| เฟส 1 เป็น MVP Back office ก่อน (Flow A) แล้วค่อยทำเว็บรับบริจาคสาธารณะ (Flow B) | ตามข้อเสนอ phasing ในเอกสาร ข้อ 10 — ส่งมอบคุณค่าหลัก (ออกใบเสร็จถูกต้อง) ได้เร็วที่สุด | ✓ Good — Flow B (Phase 6) reuse pipeline เดิมได้ 100% |
| MVP ใช้รูปภาพลายเซ็น ไม่ใช่ PKI digital signature | ลดความซับซ้อน/ต้นทุน certificate ในช่วงแรก; PKI เลื่อนพิจารณาภายหลัง (Open Issue #1) | ✓ Good — signature image ผ่าน CSS/img ใน PDF (Phase 4); PKI ยัง out-of-scope |
| แยกบทบาท Maker (ผู้สร้าง) ออกจาก Checker (ผู้อนุมัติ) | segregation of duties เพื่อการควบคุมภายในตามคำแนะนำในเอกสาร (Open Issue #2) | ✓ Good — SoD บังคับทั้ง app guard และ DB CHECK (approver ≠ creator) |
| Tech stack: Go + Keycloak + PostgreSQL + Next.js (override NestJS เดิม, D-20..D-26) | ผู้ใช้กำหนด Go backend + Keycloak; คุม FOR UPDATE / transaction boundary ตรง | ✓ Good — 3 เหตุผลหลัก (gap-less counter, AES-GCM envelope, Thai PDF) transfer ครบ |
| ออกเลขที่ใบเสร็จด้วย counter table + `SELECT … FOR UPDATE` แยกตามปีงบประมาณ (ห้าม SEQUENCE) | รับประกัน gap-less + ไม่ซ้ำใน multi-user (NFR-04, FR-16); nextval() ไม่ transactional | ✓ Good — proven zero-gap/zero-dup, 50 parallel + rollback + UNIQUE backstop under -race (Phase 2) |
| Thai PDF ผ่าน headless Chromium (chromedp) ไม่ใช่ pure-Go PDF lib | ทางเดียวที่ render Thai vowel/tone-mark stacking ถูก; sandbox JS/network off | ✓ Good — golden-file byte-exact สำหรับ worst-case stacked tone marks (Phase 4) |
| PDF+email รันหลัง transactional outbox worker (ไม่อยู่ใน numbering tx) | approval commit เร็ว ไม่ hold row lock; retry ได้ (NFR-07) | ✓ Good — FOR UPDATE SKIP LOCKED, backoff→dead-letter (Phase 4) |
| Integration-test gate เป็น done-criterion (E2E ผ่าน real HTTP+token) | Phase 3 ผ่าน 5/5 unit แต่ยัง ship 3 seam bugs | ✓ Good — เพิ่มใน CONVENTIONS; Phase 7 ปิด composite E2E seam สุดท้าย |

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition** (via `/gsd-transition`):
1. Requirements invalidated? → Move to Out of Scope with reason
2. Requirements validated? → Move to Validated with phase reference
3. New requirements emerged? → Add to Active
4. Decisions to log? → Add to Key Decisions
5. "What This Is" still accurate? → Update if drifted

**After each milestone** (via `/gsd:complete-milestone`):
1. Full review of all sections
2. Core Value check — still the right priority?
3. Audit Out of Scope — reasons still valid?
4. Update Context with current state

---
*Last updated: 2026-07-17 after v1.0 MVP milestone*
