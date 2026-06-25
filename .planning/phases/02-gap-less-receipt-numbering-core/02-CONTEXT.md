# Phase 2: Gap-less Receipt Numbering Core (★) - Context

**Gathered:** 2026-06-25
**Status:** Ready for planning

<domain>
## Phase Boundary

สร้าง **บริการจัดสรร "เลขที่ใบเสร็จ"** ที่ไม่ซ้ำ เรียงต่อเนื่องห้ามข้ามเลข (gap-less)
ต่อปีงบประมาณ ภายใน DB transaction สั้น ๆ เดียว — และ **พิสูจน์ invariant นี้ภายใต้
concurrency + rollback ก่อนที่ flow ใด ๆ จะมาพึ่งพา** นี่คือ correctness risk อันดับ 1 ของโปรเจกต์

**In scope (Phase 2):**
- counter table (key ต่อปีงบประมาณ) + raw `SELECT … FOR UPDATE` path ผ่าน sqlc/pgx
- `Allocate(ctx, tx, issueDate)` allocator service — caller-managed transaction
- `fiscalYear(issueDate)` pure helper (Asia/Bangkok + พ.ศ., boundary 30 ก.ย./1 ต.ค.)
- ledger table `receipt_numbers` (เลขที่จัดสรรแล้ว) + `UNIQUE(fiscal_year, running_no)` backstop
- DB settings table สำหรับ config รูปแบบเลข (separator/padding/year-format/prefix)
- การ format เลข (`2569/000123`) + freeze formatted snapshot ตอน allocate
- concurrency + rollback test harness (N parallel allocations → zero gaps/dupes)

**Out of scope (เลื่อนไปเฟสอื่น — เป็น new capability):**
- donation/receipt entity จริง (donor, status, amount) → Phase 3 (allocator ถูกเรียกจาก issuance tx ของ Phase 3)
- maker-checker approval transaction → Phase 3
- UI/หน้าจอแก้ config รูปแบบเลข → Phase 4 (FR-33/NFR-09 ต่อ UI เข้ากับ settings table ที่เฟสนี้วางไว้)
- PDF/email/outbox → Phase 4
- ไม่มี UI ใด ๆ ในเฟสนี้ (backend-only, พิสูจน์ผ่าน test)

</domain>

<decisions>
## Implementation Decisions

### รูปแบบเลขที่ใบเสร็จ (FR-15)
- **D-28:** รูปแบบ default = **`2569/000123`** — ปีงบ พ.ศ. **4 หลักเต็ม** + separator `/` + running_no zero-pad **6 หลัก** (ตรงกับ ROADMAP SC#1)
- **D-29:** padding เป็น **minimum width** ไม่ใช่ hard cap — ถ้า `running_no` เกิน 6 หลัก (>999999/ปี) เลขขยายตามธรรมชาติ (`1000000`) **ห้าม error / ห้ามบล็อกการออกเลข** (volume รพ. ต่ำมาก แทบไม่เกิด แต่ correctness ต้องมาก่อน)
- **D-30:** ทุกองค์ประกอบ (separator, padding width, year-format, prefix) **ตั้งค่าได้** ไม่ hardcode

### ที่เก็บ config รูปแบบเลข
- **D-31:** เก็บ config รูปแบบเลขใน **DB settings table ตั้งแต่ Phase 2** (เช่น `receipt_number_config` หรือ row ใน app_settings ทั่วไป) — ได้ no-deploy config ตั้งแต่แรก. **Phase 4 แค่ต่อ UI เข้ามาแก้** ค่าใน table นี้ (ไม่สร้าง store ใหม่) — schema/seam ถูกวางที่เฟสนี้
- **D-32:** allocator อ่าน config นี้ตอน allocate เพื่อ render formatted string

### Allocator seam ↔ Phase 3 (NFR-04, FR-16)
- **D-33:** allocator เป็น **caller-managed transaction** — signature แนว `Allocate(ctx, tx, issueDate) → (struct, error)` โดย `tx` เป็น `pgx.Tx` ที่ Phase 3 เปิดผ่าน `db.WithTx` แล้วส่งเข้ามา → Phase 3 รวม **allocate + set status=issued + audit + enqueue outbox ใน commit เดียว** กับการออกเลข (ตรง CLAUDE.md "ออกเลขในจังหวะ commit เดียวกับ issue"). **ห้าม** allocator คุม transaction เอง
- **D-34:** allocator คืน **struct เต็ม** — `fiscal_year` (เช่น 2569), `running_no` (เช่น 123, raw int สำหรับ query/sort), และ `formatted` (เช่น `2569/000123`). เก็บ raw ไว้ใช้เรียง/ค้น และมี formatted พร้อมแสดง
- **D-35:** allocator เป็น **code path เดียว** ที่แจกเลขได้ และ **ห้าม pre-compute / reserve เลขบน draft** (SC#5) — เลขเกิดตอน commit ของ issuance เท่านั้น
- **D-36:** **Error/retry contract** — เมื่อชน lock/constraint allocator **bubble error ขึ้น caller ไม่ retry ภายในตัวเอง**. caller (Phase 3) เป็นผู้จัดการ → tx rollback ลบ ledger row ที่ค้าง → gap-less ยังปลอดภัย และไม่ถือ row-lock นานเกิน (กัน serialize approver / NFR-07)

### Ledger & backstop (SC#4, FR-16)
- **D-37:** สร้าง **ledger table `receipt_numbers` แบบ standalone** ใน Phase 2 (อย่างน้อย: `fiscal_year`, `running_no`, `formatted`, `allocated_at`) พร้อม **`UNIQUE(fiscal_year, running_no)`** เป็น backstop ที่ **มีอยู่จริงและพิสูจน์ภายใต้ concurrency ในเฟสนี้** — allocate = INSERT 1 แถว/เลข. concurrency test ยิงจริงลง ledger แล้ว assert zero gap/zero dupe
- **D-38:** Phase 3 receipts จะ **อ้างอิง/FK ถึง ledger** (allocation แยกอิสระจาก entity บริจาค) — **ไม่** ผูก Phase 2 กับ entity design ของ Phase 3 ล่วงหน้า (ปฏิเสธทางเลือก "minimal receipts table + ALTER")
- **D-39:** counter table (one row ต่อ fiscal_year, ถือ `last_running_no`) แยกจาก ledger — counter ให้ค่าถัดไป, ledger เก็บประวัติเลขที่ออกแล้ว + backstop uniqueness

### Fiscal year & freeze (FR-17, FR-18, SC#2)
- **D-40:** `fiscalYear(issueDate)` เป็น **pure helper** — รับ `time.Time` ที่ caller (Phase 3 = เวลาอนุมัติ) ส่งเข้ามา, **normalize เป็น Asia/Bangkok เสมอ** ไม่ว่า input timezone จะเป็นอะไร, คืนปีงบ พ.ศ. (ต.ค.–ธ.ค. → ปีงบถัดไป). มี unit test ครอบ boundary 30 ก.ย. 23:59 / 1 ต.ค. 00:00. **ห้าม** allocator เรียก `now()` เอง (ทดสอบ boundary ได้ + ตรงเวลา issue)
- **D-41:** reset เลขรันเป็น 1 อัตโนมัติเมื่อขึ้นปีงบใหม่ เพราะ counter keyed ต่อ fiscal_year — **ไม่มี scheduled reset job** (counter row ของปีใหม่เกิดเองตอน allocate ครั้งแรกของปีนั้น)
- **D-42 [⚠️ compliance]:** **Freeze เลขที่แสดง** — ledger เก็บ `formatted` snapshot ตอน allocate และ **แสดงจาก snapshot เสมอ**. เนื่องจาก config รูปแบบเลขอยู่ใน DB และแก้ได้ (D-31) การ format ตอนอ่านจะทำให้ใบเก่าเปลี่ยนหน้าตาเมื่อ config เปลี่ยน — ผิดต่อ audit/ภาษี. เลขที่ออกแล้วต้อง **immutable**

### Claude's Discretion
- กลไก lock ที่แน่นอน (`SELECT … FOR UPDATE` แล้ว UPDATE vs `INSERT … ON CONFLICT DO UPDATE … RETURNING`) — ⚠️ research flag ของ ROADMAP ให้ verify path นี้ผ่าน sqlc/pgx; CLAUDE.md ระบุ `SELECT … FOR UPDATE` เป็นแนวหลัก
- schema รายละเอียดของ counter / ledger / settings table (ชื่อ column, index, FK) — planner ออกแบบตาม decisions ข้างบน
- ค่า config seed เริ่มต้นที่แน่นอน (เก็บ default ตาม D-28: sep `/`, pad 6, year พ.ศ. 4 หลัก, prefix ว่าง)
- จำนวน N / รูปแบบ rollback scenario ใน concurrency test — planner กำหนด ขอแค่ assert zero gap + zero dupe + UNIQUE holds (รวม rollback ทิ้งแล้วไม่เกิด gap)
- โครงสร้าง package ฝั่ง Go (เช่น `internal/receiptno/`) — ตาม pattern ที่ก่อตัวจาก Phase 1

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project & Requirements
- `.planning/PROJECT.md` — core value (gap-less เลขใบเสร็จ), constraints (Correctness/Concurrency)
- `.planning/REQUIREMENTS.md` — รายละเอียด FR-15, FR-16, FR-17, FR-18, NFR-04 + traceability
- `.planning/ROADMAP.md` §"Phase 2" — goal + 5 success criteria + research flag (verify ORM path / concurrency harness)
- `.planning/phases/01-foundation-db-auth-rbac-audit-retention-model/01-CONTEXT.md` — decisions ฐานราก (D-20 Go, D-21 PostgreSQL, D-23 data layer, D-24 hybrid auth) ที่เฟสนี้สืบทอด

### Load-bearing technical guidance
- `CLAUDE.md` §"The Two Load-Bearing Decisions" #1 — gap-less counter pattern (counter table + `SELECT … FOR UPDATE`, ห้าม SEQUENCE/SERIAL, key ต่อ fiscal year, UNIQUE backstop, ห้าม pre-compute). **อ่านก่อนทำเสมอ**
- `CLAUDE.md` §"What NOT to Use" — ห้าม `nextval()`/SERIAL, ห้าม "read max + 1" ใน app code, ห้าม render PDF/email ใน numbering transaction

### Existing code to reuse (Phase 1)
- `donnarec-api/internal/db/helpers.go` — `db.WithTx(ctx, pool, fn)` (Pattern B) คือ seam ที่ allocator caller-managed tx จะใช้
- `donnarec-api/internal/db/sqlc.yaml` + `internal/db/queries/*.sql` + `internal/db/generated/` — pattern sqlc + pgx/v5 (emit_interface, named params `@x`); เพิ่ม query ใหม่ที่นี่
- `donnarec-api/migrations/` — golang-migrate `NNNNNN_name.up/down.sql` (ล่าสุด 000003) → migration ถัดไป 000004+
- `donnarec-api/internal/config/config.go` — pattern โหลด config (แต่ config รูปแบบเลขเลือกเก็บ DB ตาม D-31 ไม่ใช่ env)
- `donnarec-api/internal/testutil/postgres.go` — testcontainers Postgres fixture สำหรับ concurrency/integration test
- `donnarec-api/internal/audit/` — audit service/middleware ที่ Phase 3 จะผูกกับ allocate (เฟสนี้ยังไม่ต้องเรียก)

### Source requirement document
- `requirements-ระบบออกใบเสร็จบริจาค.md` — เอกสารต้นทาง stakeholder (FR-15..18 ฝั่งเลขใบเสร็จ)

### No additional ADRs/specs
- ยังไม่มี SPEC.md/ADR แยกสำหรับเฟสนี้ — decisions ครบใน `<decisions>` ข้างบน

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- **`db.WithTx`** (`internal/db/helpers.go`): seam สำเร็จรูปสำหรับ caller-managed transaction — Phase 3 จะห่อ allocate ด้วยตัวนี้ (D-33)
- **sqlc + pgx/v5 pipeline** (`internal/db/`): สร้าง type-safe query จาก raw SQL — เขียน `SELECT … FOR UPDATE` ตรง ๆ ได้ คุม lock เองเต็มที่ (resolves D-23)
- **golang-migrate** (`migrations/`): เพิ่ม migration 000004+ สำหรับ counter / ledger / settings table
- **testcontainers Postgres** (`internal/testutil/postgres.go`): spin Postgres จริงสำหรับ concurrency test (CLAUDE.md เน้นเป็น invariant เสี่ยงสุด)

### Established Patterns
- HTTP = **gin** (go.mod), data = **sqlc + pgx/v5**, migration = golang-migrate — เฟสนี้ทำงานในชั้น data/service เป็นหลัก ยังไม่แตะ HTTP handler
- query ใช้ named params (`@email`) + explicit column list + RETURNING (`internal/db/queries/users.sql`)
- Go 1.25.1

### Integration Points
- allocator service ↔ `pgx.Tx` ที่ caller ส่งเข้า (Phase 3 issuance tx)
- allocator ↔ DB settings table (อ่าน format config ตอน allocate)
- counter table ↔ ledger table (counter ให้เลขถัดไป, ledger ถือ UNIQUE backstop + ประวัติ)
- Phase 3 receipts ↔ ledger `receipt_numbers` (FK/อ้างอิง — D-38)

</code_context>

<specifics>
## Specific Ideas

- ผู้ใช้ยืนยันรูปแบบเลข **`2569/000123`** (ปี พ.ศ. เต็ม 4 หลัก) เป็น default ชัดเจน
- ผู้ใช้เลือกวาง config ใน **DB ตั้งแต่ตอนนี้** เพื่อให้ no-deploy ได้เร็วและ Phase 4 แค่ต่อ UI
- ผู้ใช้ให้ความสำคัญกับ **immutability ของเลขที่ออกแล้ว** (เลือก freeze formatted snapshot) — สะท้อนความเป็นเอกสารภาษี/audit
- ผู้ใช้เลือกแนว **simple & safe** ทุกข้อ (bubble error ไม่ retry, padding ขยายธรรมชาติ, caller ส่งเวลา) — สอดคล้องกับ correctness-first ของโปรเจกต์

</specifics>

<deferred>
## Deferred Ideas

- **UI แก้ config รูปแบบเลข** — schema/settings table วางที่เฟสนี้ แต่หน้าจอแก้ค่าไป Phase 4 (FR-33/NFR-09)
- **receipt entity เต็ม (donor/status/amount) + maker-checker issuance tx** — Phase 3; allocator ถูกเรียกจาก tx ของ Phase 3 ผ่าน seam D-33
- **FK ledger → receipts ฝั่ง Phase 3** — Phase 2 วาง ledger standalone, Phase 3 ค่อยอ้างอิง (D-38)

None อื่น — discussion อยู่ในขอบเขตเฟส

</deferred>

---

*Phase: 2-gap-less-receipt-numbering-core*
*Context gathered: 2026-06-25*
