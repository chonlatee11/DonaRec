# Phase 1: Foundation (DB, Auth/RBAC, Audit, Retention model) - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-06-23
**Phase:** 1-foundation-db-auth-rbac-audit-retention-model
**Areas discussed:** สร้างผู้ใช้ & โมเดลบทบาท, นโยบายความปลอดภัย login, การเห็น/เปิดเลขบัตร ปชช., audit log & retention/legal hold, ล็อก tech stack, Encryption/KMS boundary, Hosting target, i18n, การใช้ Keycloak สำหรับ RBAC

---

## สร้างผู้ใช้ & โมเดลบทบาท

| Option | Description | Selected |
|--------|-------------|----------|
| ใครสร้างบัญชี — Admin เท่านั้น | ไม่มี self-signup | ✓ |
| ใครสร้างบัญชี — Admin + ตั้งรหัสชั่วคราว | Admin สร้าง + ส่งลิงก์ตั้งรหัสเอง | |
| บทบาท/คน — 1 คน 1 บทบาท | ตาม SC#1 roadmap | |
| บทบาท/คน — ถือได้หลายบทบาท | ยืดหยุ่นกว่า เสี่ยงต่อ SoD | ✓ |
| Admin scope — แยกขาด | Admin ไม่ทำหน้าที่ Maker/Checker | |
| Admin scope — ทำได้ทุกอย่าง | Admin รวมหน้าที่ Maker/Checker | ✓ |
| Bootstrap — seed script/env | สร้าง admin แรกตอน deploy | ✓ |
| Bootstrap — first-run wizard | หน้าตั้งค่ารอบแรก | |

**User's choice:** Admin เท่านั้น / Multi-role / Admin ทำได้ทุกอย่าง / Seed script
**Notes:** เลือก multi-role ขัดกับ SC#1 ("exactly one role") — ยืนยันโมเดล Multi-role + SoD ระดับรายการ (approver ≠ creator, enforce Phase 3). บันทึกเป็น deviation ที่ต้องอัปเดต roadmap.

---

## นโยบายความปลอดภัย login

| Option | Description | Selected |
|--------|-------------|----------|
| รหัสผ่าน ≥ 8 ตัว ผสมตัวอักษร/ตัวเลข | มาตรฐานทั่วไป | ✓ |
| รหัสผ่าน ≥ 12 ตัว ไม่บังคับ complexity | แนว NIST | |
| Lockout ชั่วคราวหลัง N ครั้ง | กัน brute-force | ✓ |
| Lockout จนกว่า admin ปลด | เข้มกว่า | |
| ไม่ lockout (rate-limit เท่านั้น) | เบาสุด | |
| Session หมดอายุตามเวลา + idle timeout | เหมาะ back-office มี PII | ✓ |
| Session อยู่ยาว | สะดวกแต่เสี่ยง | |
| ไม่มี MFA ใน MVP | ลด scope | ✓ |
| มี MFA ตั้งแต่แรก | ปลอดภัยสูงขึ้น | |

**User's choice:** ≥8 ผสม / lockout ชั่วคราว / time+idle timeout / ไม่มี MFA
**Notes:** ทั้งหมดตามแนะนำ. (ภายหลังย้าย authN ไป Keycloak — policy เหล่านี้ config ใน Keycloak realm.)

---

## การเห็น/เปิดเลขบัตร ปชช.

| Option | Description | Selected |
|--------|-------------|----------|
| เห็นเต็ม — Admin + Checker | least privilege, Maker เห็น mask | ✓ |
| เห็นเต็ม — Admin เท่านั้น | เข้มสุด | |
| เห็นเต็ม — Maker+Checker+Admin | กว้างเกิน | |
| Mask — โชว์ 4 ตัวท้าย | cross-check ได้ไม่เปิดเต็ม | ✓ |
| Mask — ปิดทั้งหมด | ปลอดภัยสุด | |
| Reveal audit — ทุกครั้ง | หลักฐาน PDPA | ✓ |
| Reveal audit — ไม่ต้อง | คุมด้วย role เท่านั้น | |
| Reveal — just-in-time กดทีละราย | default mask | ✓ |
| Reveal — เห็นเลยตามบทบาท | สะดวกแต่ audit เยอะ | |

**User's choice:** Admin+Checker / โชว์ 4 ตัวท้าย / audit ทุกครั้ง / just-in-time
**Notes:** ทั้งหมดตามแนะนำ. Phase 1 วางนโยบาย+กลไก; ใช้กับ donor จริง Phase 3.

---

## audit log & retention/legal hold

| Option | Description | Selected |
|--------|-------------|----------|
| Audit scope — auth + admin + reveal PII | ครอบคลุมหลัก, CRUD รายการมา Phase 3 | |
| Audit scope — ทุก mutation ในระบบ | generic interceptor | ✓ |
| ดู audit — Admin เท่านั้น | จำกัดสุด | ✓ |
| ดู audit — Admin + Checker | กว้างขึ้น | |
| Retention — config-driven | retain_until + legal_basis + legal_hold | ✓ |
| Retention — fix 5 ปีตายตัว | ง่ายแต่แก้ยาก | |
| Tamper — DB revoke เท่านั้น | append-only พื้นฐาน | |
| Tamper — เพิ่ม hash-chain | ตรวจจับการแก้แม้ DBA | ✓ |

**User's choice:** ทุก mutation / Admin เท่านั้น / config-driven / DB revoke + hash-chain
**Notes:** เลือก audit กว้างกว่า + hash-chain (เข้มกว่าแนะนำ) สะท้อนความสำคัญ tax/audit integrity.

---

## ล็อก tech stack

| Option | Description | Selected |
|--------|-------------|----------|
| ล็อก CLAUDE.md stack (NestJS/PG/Prisma) | ตามที่ research แนะ | |
| ให้ research ยืนยันก่อน | ไม่ล็อกตายตัว | |
| (Other) Go + PostgreSQL + Prisma + React/Next | ผู้ใช้ระบุเอง | ✓ |

**User's choice:** Backend Go, DB PostgreSQL, Frontend React/Next; data layer "Prisma" (ภายหลังแก้เป็น "ให้ research เลือก" เพราะ Prisma ไม่รองรับ Go)
**Notes:** override CLAUDE.md (NestJS → Go). เหตุผลหลัก 3 เรื่อง transfer ได้ครบ. แจ้งว่า Prisma Go client deprecated → research เลือก sqlc/pgx/GORM/ent.

---

## Auth/RBAC approach (Keycloak)

| Option | Description | Selected |
|--------|-------------|----------|
| Hybrid: Keycloak authN + app authZ | Keycloak login/role, แอปทำ SoD/PII/audit | ✓ |
| In-app ทั้งหมด (Passport-JWT+CASL) | เบาสุด ไม่มี server เพิ่ม | |
| Keycloak-centric | ยก user mgmt/RBAC ไป Keycloak มากสุด | |

**User's choice:** Hybrid
**Notes:** ผู้ใช้เสนอ Keycloak เอง. อธิบายว่า SoD ต่อ record + PII encrypt/mask + audit hash-chain ยังอยู่ในแอปเสมอ.

---

## Encryption / KMS boundary

| Option | Description | Selected |
|--------|-------------|----------|
| Secrets manager / env (MVP) | KEK ใน env, สลับ KMS ได้ภายหลัง | |
| Cloud KMS ตั้งแต่แรก | ผูก cloud | |
| ออกแบบ envelope ไว้ ยังไม่ผูก backend | KeyProvider abstraction | ✓ |

**User's choice:** ออกแบบ envelope + KeyProvider abstraction, ยังไม่ผูก backend
**Notes:** MVP ใช้ env ได้; เผื่อ blind index สำหรับค้นเลขบัตร Phase 3.

---

## Hosting target

| Option | Description | Selected |
|--------|-------------|----------|
| ยังไม่ตัดสิน — portable | Docker, ไม่ผูก cloud-specific | |
| On-prem รพ. | MinIO + self-managed KMS | |
| Cloud | managed services | |
| (Other) Docker, รัน local ก่อน, ย้าย cloud ได้ | ผู้ใช้ระบุเอง | ✓ |

**User's choice:** Docker-based, รันบนเครื่อง local ก่อน, ออกแบบให้ย้าย cloud ได้
**Notes:** ไม่ผูก cloud-specific service ใน MVP. Keycloak self-hosted ผ่าน Docker เข้ากันได้.

---

## i18n ตั้งแต่แรก

| Option | Description | Selected |
|--------|-------------|----------|
| วางโครง i18n ตั้งแต่แรก | message key/catalog ไทย/อังกฤษ | ✓ |
| ไทยล้วนก่อน ค่อยเพิ่มภาษา | ลด scope | |

**User's choice:** วางโครง i18n message catalog ตั้งแต่ Phase 1
**Notes:** ไม่ retrofit ภายหลัง.

---

## Go data layer (follow-up)

| Option | Description | Selected |
|--------|-------------|----------|
| ให้ research เลือก | sqlc/pgx/GORM/ent ตามต้องการ gap-less | ✓ |
| sqlc | SQL ดิบ type-safe | |
| GORM | ORM คล้าย Prisma | |

**User's choice:** ให้ research เลือก (ให้น้ำหนักความสามารถคุม FOR UPDATE เองสำหรับ Phase 2)

---

## Claude's Discretion

- รายละเอียด schema ของตาราง (users, roles, audit_log, retention fields, key metadata)
- รูปแบบ message-catalog / โครง i18n ฝั่ง Go และ Next.js
- ค่า config เฉพาะ (จำนวนครั้ง lockout, อายุ token/idle, default retain_until)

## Deferred Ideas

- อัปเดต ROADMAP SC#1 (exactly-one-role → multi-role + SoD ระดับรายการ)
- MFA/OTP (Keycloak เปิดได้ภายหลัง)
- Cloud KMS / HSM (เมื่อ stakeholder ตัดสิน hosting)
- Blind index ค้นด้วยเลขบัตร (Phase 3)
- Donor PII encrypt/decrypt/mask usage จริง (Phase 3)
- Consent capture (Phase 3 Flow A / Phase 6 Flow B)
