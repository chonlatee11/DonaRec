# Requirements: DonaRec — ระบบออกใบเสร็จบริจาคอัตโนมัติสำหรับโรงพยาบาล

**Defined:** 2026-06-22
**Core Value:** ออกใบเสร็จบริจาคที่มีเลขที่รันต่อเนื่องไม่ซ้ำ ห้ามข้ามเลข (gap-less) ตามปีงบประมาณ หลังผ่านการอนุมัติโดยมนุษย์ และส่งถึงผู้บริจาคได้อย่างถูกต้องน่าเชื่อถือ

> REQ-ID อ้างอิงรหัส FR/NFR จากเอกสารต้นทาง `requirements-ระบบออกใบเสร็จบริจาค.md` (v1.1) โดยตรงเพื่อ traceability
> ขอบเขตเฟส (milestone) นี้: **เงินบริจาคเท่านั้น** | ระดับ M=Must, S=Should, C=Could

## v1 Requirements

### Foundation & Security (รากฐาน/ความปลอดภัย)

- [x] **NFR-01**: ผู้ใช้เข้าสู่ระบบด้วยบัญชี, รหัสผ่านถูกเข้ารหัส (hash), และจำกัดสิทธิ์ตามบทบาท RBAC [M]
- [x] **FR-34**: Admin จัดการผู้ใช้และสิทธิ์ (RBAC) — Maker / Checker / Admin แยกบทบาท [M]
- [ ] **NFR-02**: เข้ารหัสข้อมูลขณะส่ง (HTTPS/TLS) และเข้ารหัสข้อมูลอ่อนไหวขณะจัดเก็บ (เลขบัตร ปชช./เลขผู้เสียภาษี) [M]
- [x] **NFR-05**: เก็บ audit log ทุกการกระทำสำคัญแบบลบไม่ได้ และเก็บได้ตามระยะเวลาที่กำหนด [M]
- [ ] **FR-13**: บันทึก audit trail ทุกการกระทำ (ใคร ทำอะไร เมื่อไร) [M]

### Receipt Numbering (เลขที่ใบเสร็จ — gap-less)

- [x] **FR-15**: เลขที่ใบเสร็จ = ปีงบประมาณ + เลขรัน (เช่น 2569/000123) ตั้งค่ารูปแบบ/ตัวคั่น/จำนวนหลักได้ [M]
- [x] **FR-16**: เลขไม่ซ้ำ เรียงต่อเนื่อง ห้ามข้ามเลข (gap-less) ภายในปีงบประมาณเดียวกัน [M]
- [x] **FR-17**: รีเซ็ตเลขรันเป็น 1 อัตโนมัติเมื่อขึ้นปีงบประมาณใหม่ (1 ต.ค.) [M]
- [x] **FR-18**: กำหนดปีงบประมาณจากวันที่ออกใบเสร็จ (วันอนุมัติ) อัตโนมัติ — ต.ค.–ธ.ค. นับเป็นปีงบประมาณถัดไป [M]
- [x] **NFR-04**: เลขที่ใบเสร็จต้องไม่ซ้ำเด็ดขาดแม้มีผู้ใช้พร้อมกันหลายคน (concurrency-safe) [M]

### Donation Records — Back Office (จัดการรายการบริจาค)

- [x] **FR-07**: เจ้าหน้าที่ (Maker) สร้างรายการบริจาคเองได้ (Flow A) [M]
- [x] **FR-09**: ดู/แก้ไขข้อมูลรายการก่อนอนุมัติ พร้อมดูสลิปแนบ [M]
- [x] **FR-11**: สถานะรายการชัดเจน: ร่าง → รอตรวจสอบ → อนุมัติ/ออกใบเสร็จแล้ว → ปฏิเสธ → ยกเลิก [M]
- [ ] **FR-08**: แสดงคิวรายการ "รอตรวจสอบ" จากเว็บ (Flow B) [M]
- [x] **FR-10**: ค้นหา/กรองรายการ (ชื่อ, ช่วงวันที่, สถานะ, เลขที่ใบเสร็จ) [M]

### Approval (ตรวจสอบและอนุมัติ — maker/checker)

- [x] **FR-12**: ผู้อนุมัติ (Checker) อนุมัติหรือตีกลับรายการได้ พร้อมระบุเหตุผลตอนตีกลับ [M]
- [x] **FR-14**: ใบเสร็จถูกสร้างก็ต่อเมื่อรายการได้รับการอนุมัติเท่านั้น (ไม่มี auto-issue; ผู้สร้างอนุมัติตัวเองไม่ได้) [M]
- [x] **FR-19**: ยกเลิกใบเสร็จใช้สถานะ "ยกเลิก" ไม่ลบเลขทิ้ง (เก็บร่องรอยเพื่อตรวจสอบบัญชี/ภาษี) [M]

### PDF Document (สร้างเอกสารใบเสร็จ)

- [x] **FR-20**: สร้างใบเสร็จ PDF จากเทมเพลตที่มีตรา/หัวจดหมายของโรงพยาบาล [M]
- [x] **FR-21**: ฝังลายน้ำ (watermark) ของโรงพยาบาลบนเอกสาร [M]
- [x] **FR-22**: แสดงลายเซ็นผู้มีอำนาจบนใบเสร็จ (รูปภาพลายเซ็นใน MVP) [M]
- [x] **FR-24**: ใบเสร็จมีข้อมูลครบตามข้อกำหนดลดหย่อนภาษี (ตามข้อ 6 — รวมข้อความสิทธิลดหย่อน 1 เท่า/2 เท่า) [M]
- [x] **FR-23**: รองรับการสร้างใบเสร็จเป็นภาษาไทยหรืออังกฤษตามผู้บริจาค [M]
- [x] **NFR-07**: สร้าง PDF และส่งอีเมลในเวลาที่ยอมรับได้ (~2–3 วินาที/ใบ) [M]

### Email Delivery (ส่งอีเมล)

- [x] **FR-25**: ส่งใบเสร็จ PDF แนบอีเมลถึงผู้บริจาคหลังอนุมัติ [M]
- [ ] **FR-27**: บันทึกสถานะการส่ง (สำเร็จ/ล้มเหลว) และส่งซ้ำได้กรณีล้มเหลว [M]
- [ ] **FR-28**: เจ้าหน้าที่ดาวน์โหลดไฟล์ PDF เองได้ กรณีผู้บริจาคไม่มีอีเมล [S]
- [x] **FR-26**: เทมเพลตอีเมลรองรับ 2 ภาษา ตามภาษาผู้บริจาค [S]

### Donor Data & e-Donation (ข้อมูลผู้บริจาคและ e-Donation)

- [x] **FR-29**: จัดเก็บข้อมูลผู้บริจาค: ชื่อ, เลขผู้เสียภาษี/บัตรประชาชน, ที่อยู่, อีเมล [M]
- [ ] **FR-30**: ส่งออกข้อมูล (Excel/CSV) เพื่อรองรับการคีย์เข้า e-Donation ด้วยตนเอง [M]
- [ ] **FR-31**: ติดสถานะ "คีย์เข้า e-Donation แล้ว" เพื่อกันคีย์ซ้ำ/ตกหล่น [S]

### PDPA & Compliance

- [ ] **NFR-03**: บันทึกความยินยอม (consent) พร้อมวันเวลา/เวอร์ชันข้อความ, ระบุวัตถุประสงค์, และรองรับสิทธิเจ้าของข้อมูล (เข้าถึง/แก้ไข; สิทธิขอลบถูกจำกัดโดยกฎหมายภาษี — retention policy ไม่ hard-delete) [M]

### Public Donation Website (เว็บไซต์รับบริจาค — Flow B)

- [ ] **FR-01**: ผู้บริจาคกรอกแบบฟอร์มบริจาค (ข้อมูลผู้บริจาค + จำนวนเงิน + วันที่บริจาค) [M]
- [ ] **FR-02**: ผู้บริจาคอัปโหลดไฟล์สลิป (jpg/png/pdf, จำกัดขนาด, ตรวจชนิดไฟล์) [M]
- [ ] **FR-03**: แสดงและบันทึกการให้ความยินยอม (consent) ตาม PDPA ก่อนส่งข้อมูล [M]
- [ ] **FR-06**: รองรับการเลือกภาษา ไทย/อังกฤษ บนเว็บฟอร์ม [M]
- [ ] **FR-05**: แจ้งสถานะ/ส่งอีเมลว่าได้รับรายการแล้ว (ยังไม่ใช่ใบเสร็จ) [S]
- [ ] **FR-04**: ป้องกันสแปม/บอท (CAPTCHA / rate limiting) [S]

### Reports & Settings (รายงานและการตั้งค่า)

- [ ] **FR-33**: Admin ตั้งค่าเทมเพลต ลายน้ำ ลายเซ็น และรูปแบบเลขที่ได้ (แยก config จากโค้ด) [S]
- [ ] **NFR-09**: แยกการตั้งค่า (เทมเพลต/เลข/ลายเซ็น) ออกจากโค้ด ปรับได้โดยไม่ต้อง deploy [M]
- [ ] **FR-32**: รายงานสรุปการบริจาค (ตามช่วงเวลา/ยอดรวม) [S]

### Cross-cutting Non-functional

- [ ] **NFR-06**: UI รองรับ 2 ภาษา ใช้งานบนเดสก์ท็อปและมือถือได้ (responsive) [M]
- [ ] **NFR-08**: สำรองข้อมูล (backup) สม่ำเสมอและกู้คืนได้ [M]

## v2 Requirements

Deferred to future milestones (ตามข้อ 10 เฟส 3 ของเอกสาร).

### Future

- **GENRECEIPT-01**: ใบเสร็จทั่วไป (non-donation)
- **REPORT-ADV-01**: รายงานเชิงลึก/วิเคราะห์
- **INTEGRATION-01**: เชื่อมต่อระบบบัญชีภายในอัตโนมัติ
- **EDONATION-API-01**: เชื่อมต่อ API ตรงกับ e-Donation กรมสรรพากร (อาจกลายเป็น Must หากเข้าเงื่อนไข mandate 1 ม.ค. 2026 — ต้องยืนยันกับฝ่ายกฎหมาย)
- **PKI-SIGN-01**: ลายเซ็นดิจิทัลแบบ PKI (digital signature ด้วยใบรับรอง) แทนรูปภาพลายเซ็น
- **REISSUE-01**: ออกใบแทน/ขอใบเสร็จย้อนหลังกรณีหาย (Open Issue #6 — ต้องตัดสินใจ)

## Out of Scope

| Feature | Reason |
|---------|--------|
| บริจาคสิ่งของ (in-kind) | เฟสนี้เน้นเงินบริจาคเท่านั้น |
| Payment gateway | ยืนยันเงินเข้าโดยเจ้าหน้าที่ตรวจสลิปแมนวล 100% |
| เชื่อมต่อ API e-Donation โดยตรง (เฟสนี้) | เจ้าหน้าที่คีย์เอง; ระบบเตรียม export — ดู v2 EDONATION-API-01 |
| เชื่อมต่อระบบบัญชีภายในอัตโนมัติ (เฟสนี้) | เป็นระบบแยกต่างหาก — ดู v2 INTEGRATION-01 |
| PKI digital signature (เฟสนี้) | MVP ใช้รูปภาพลายเซ็น — ดู v2 PKI-SIGN-01 |
| Auto-issue ใบเสร็จโดยไม่อนุมัติ | ขัดหลักการ "ทุกใบเสร็จต้องผ่านมนุษย์" (anti-feature) |
| Hard-delete ข้อมูล/เลขใบเสร็จ | ขัดข้อกำหนด audit/ภาษี (ใช้สถานะยกเลิก + retention) |

## Stakeholder Confirmations Required (ต้องยืนยันก่อน build เฟสที่เกี่ยวข้อง)

| หัวข้อ | กระทบ requirement | ผู้ยืนยัน | เฟสที่เกี่ยวข้อง |
|--------|-------------------|----------|------------------|
| ข้อความ/รูปแบบใบเสร็จตามสรรพากร + เงื่อนไขลดหย่อน 1 เท่า/2 เท่า | FR-24 | ฝ่ายบัญชี/กฎหมาย รพ. | Phase 4 |
| ระยะเวลาเก็บข้อมูล (ภาษี ~5 ปี vs PDPA) | NFR-03 | ฝ่ายกฎหมาย/DPO | Phase 1 (model), Phase 6 (donor request) |
| รูปแบบ/ฟิลด์ไฟล์ export สำหรับ e-Donation | FR-30 | จากระบบ RD จริง | Phase 5 |
| รพ. เข้าเงื่อนไข mandate e-Donation 1 ม.ค. 2026 หรือไม่ | EDONATION-API-01 (อาจขยาย scope) | ฝ่ายกฎหมาย | Phase 5 |
| Email provider / KMS / hosting (on-prem vs cloud) | FR-25, NFR-02 | ฝ่าย IT/จัดซื้อ | Phase 1 / Phase 4 |

## Traceability

Which phases cover which requirements.

| Requirement | Phase | Status |
|-------------|-------|--------|
| NFR-01 | Phase 1 | Complete |
| FR-34 | Phase 1 | Complete |
| NFR-02 | Phase 1 | Pending |
| NFR-05 | Phase 1 | Complete |
| FR-13 | Phase 1 | Pending |
| NFR-03 | Phase 1 | Pending |
| FR-15 | Phase 2 | Complete |
| FR-16 | Phase 2 | Complete |
| FR-17 | Phase 2 | Complete |
| FR-18 | Phase 2 | Complete |
| NFR-04 | Phase 2 | Complete |
| FR-07 | Phase 3 | Complete |
| FR-09 | Phase 3 | Complete |
| FR-11 | Phase 3 | Complete |
| FR-10 | Phase 3 | Complete |
| FR-12 | Phase 3 | Complete |
| FR-14 | Phase 3 | Complete |
| FR-19 | Phase 3 | Complete |
| FR-29 | Phase 3 | Complete |
| FR-20 | Phase 4 | Complete |
| FR-21 | Phase 4 | Complete |
| FR-22 | Phase 4 | Complete |
| FR-24 | Phase 4 | Complete |
| FR-23 | Phase 4 | Complete |
| NFR-07 | Phase 4 | Complete |
| FR-25 | Phase 4 | Complete |
| FR-26 | Phase 4 | Complete |
| FR-27 | Phase 4 | Pending |
| FR-28 | Phase 4 | Pending |
| FR-33 | Phase 4 | Pending |
| NFR-09 | Phase 4 | Pending |
| FR-30 | Phase 5 | Pending |
| FR-31 | Phase 5 | Pending |
| FR-32 | Phase 5 | Pending |
| NFR-08 | Phase 5 | Pending |
| FR-01 | Phase 6 | Pending |
| FR-02 | Phase 6 | Pending |
| FR-03 | Phase 6 | Pending |
| FR-06 | Phase 6 | Pending |
| FR-05 | Phase 6 | Pending |
| FR-04 | Phase 6 | Pending |
| FR-08 | Phase 6 | Pending |
| NFR-06 | Phase 6 | Pending |

**Coverage:**

- v1 requirements: 43 active (FR ×34 + NFR ×9)
- Mapped to phases: 43/43 ✓ (100%)
- Unmapped: 0

> Note: NFR-02 spans Phase 1 (transport TLS + encryption boundary) and Phase 3 (donor PII encrypt/mask usage); NFR-03 spans Phase 1 (retention/legal-basis model) and Phases 3/6 (consent capture for Flow A / Flow B). Each is assigned its primary owning phase above to keep one-phase-per-requirement; downstream phases reference them.

---
*Requirements defined: 2026-06-22*
*Last updated: 2026-06-22 after roadmap creation (traceability populated)*
