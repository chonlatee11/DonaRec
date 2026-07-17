# Phase 5: e-Donation Export, Reports & Admin Settings - Context

**Gathered:** 2026-07-06
**Status:** Ready for planning

<domain>
## Phase Boundary

เครื่องมือปฏิบัติงานหลังบ้าน **หลังใบเสร็จออกแล้ว** เพื่อรองรับการคีย์ e-Donation แบบแมนวล
ติดตามงานเทียบเดดไลน์ ดูสรุปบริจาค และมั่นใจว่าข้อมูลกู้คืนได้ (FR-30/31/32, NFR-08)

**In scope (Phase 5):**
- **e-Donation Export** — สร้างไฟล์ `.xlsx` (excelize) + `CSV` ของ record ที่ **issued** map เป็นฟิลด์ e-Donation
  (เลขบัตร/เลขผู้เสียภาษี 13 หลัก, วันที่บริจาค, ประเภทเงินสด); เข้าถึงได้เฉพาะ **Checker + Admin**;
  ทุกครั้ง **audit + download-logged**; filter ตามช่วงวันที่ + สถานะคีย์ (FR-30, SC#1)
- **สถานะ "คีย์เข้า e-Donation แล้ว"** — flag ต่อ record ตั้งได้แบบ **bulk multi-select และต่อแถว**, ทุกการติ๊ก/เอาออก audit;
  สิทธิ์ = เท่ากับ export (Checker + Admin) (FR-31, SC#2)
- **Aging view** — surface record ที่ยังไม่คีย์ เทียบเดดไลน์ **วันที่ 5 ของเดือนถัดจากเดือนที่ออกใบเสร็จ**,
  จัด **3 กลุ่ม** (ยังไม่ถึงกำหนด / ใกล้ครบ / เกินกำหนด) กันงานตกหล่น (SC#2)
- **รายงานสรุปบริจาค (FR-32)** — เลือกช่วงเวลา → ยอดรวม + จำนวนใบ + breakdown รายเดือน/วัน; แสดงเป็น **ตาราง + card สรุปยอด**;
  export เป็น Excel/CSV ได้ (ไม่มี PII); ดูได้ **ทุก staff (Maker/Checker/Admin)** (SC#3)
- **Backup/Restore (NFR-08)** — **pg_dump ตามตารางใน docker-compose** (companion/cron), ครอบคลุม **DB + MinIO** (slip + PDF ที่ freeze),
  พิสูจน์ด้วย **runbook + ทดสอบ restore จริง บันทึกหลักฐาน** (restore verified ไม่ใช่แค่ configured) (SC#4)

**Out of scope (เลื่อน / ไม่ทำในเฟสนี้):**
- **Admin settings UI สำหรับ template/ลายเซ็น/รูปแบบเลข** — ส่งมอบแล้วใน Phase 4 (D-58/D-59/D-61 config store);
  เฟสนี้ **ต่อยอด** config store เพื่อเก็บ e-Donation field mapping เท่านั้น ไม่สร้าง config store ใหม่
- **การเชื่อม API e-Donation กรมสรรพากรโดยตรง** — milestone นี้ export แมนวล Excel/CSV เท่านั้น (PROJECT.md Out of Scope)
- **chart/กราฟในรายงาน** — MVP ใช้ตาราง + card สรุปยอดพอ (เพิ่มภายหลังได้)
- **field 1x/2x ต่อ donation** — ยัง global config (D-59)
- Flow B public form (FR-01..06) → Phase 6

</domain>

<decisions>
## Implementation Decisions

### e-Donation Export (FR-30, SC#1)
- **D-62:** **สร้างทั้ง `.xlsx` (excelize) และ `CSV`** — .xlsx เป็น output หลัก (รองรับข้อความไทย/UTF-8 เปิดใน Excel ไม่เพี้ยน,
  ตรงคำแนะนำ CLAUDE.md FR-30); CSV เป็นทางเลือก (ต้องระวัง BOM/encoding ให้ Excel อ่านภาษาไทยได้)
- **D-63:** **สิทธิ์ export = Checker + Admin** — ไฟล์มีเลขบัตร ปชช. plaintext จำนวนมาก; จำกัดเฉพาะ role ที่อนุมัติใบเสร็จอยู่แล้ว
  (สอดคล้องหลัก least-privilege ของ PII reveal ใน Phase 3)
- **D-64:** **เลขบัตรใน export = เต็ม 13 หลัก + audit ทุกครั้ง + download-logged** — e-Donation ต้องเลขเต็มถึงคีย์ได้ →
  ไฟล์มี plaintext จริง. **ทุกการ export เขียน audit row** (ใคร/เมื่อไร/ช่วงที่เลือก/จำนวน record) และ **download-logged** (SC#1).
  ⚠️ downstream: ต้องเตือน UI เรื่องการเก็บรักษาไฟล์; export ต้องผ่าน decrypt path เดียวกับ audited PII reveal (Phase 3)
- **D-65:** **cash type = ค่าคงที่ ('เงินสด/โอน')** — ทุกรายการเป็นเงินทั้งหมด (in-kind อยู่ Out of Scope) → ใส่ค่าคงที่ตามสเปก e-Donation
  ไม่ต้องเพิ่ม field ต่อรายการ (simple & safe)
- **D-66:** **Export scope = record สถานะ issued, filter ตามช่วงวันที่ (เดือน/fiscal year) + กรองสถานะคีย์** — เลือกเฉพาะที่ยังไม่คีย์ได้;
  **cancelled ไม่รวม** (ไม่คีย์ e-Donation). เชื่อมกับ aging view (ดู D-68)

### สถานะ "คีย์แล้ว" + Aging (FR-31, SC#2)
- **D-67:** **flag "คีย์เข้า e-Donation แล้ว" ตั้งได้แบบ bulk multi-select และต่อแถว** — ติ๊กหลายแถวในหน้า aging แล้ว mark ครั้งเดียว;
  ทุกการ mark/unmark เขียน audit. **ต้อง migration ใหม่** (คอลัมน์ flag + timestamp + actor บน donations หรือ side table — planner ออกแบบ)
- **D-68:** **Aging = 3 กลุ่ม** (ยังไม่ถึงกำหนด / ใกล้ครบ / เกินกำหนด) เทียบเดดไลน์ **วันที่ 5 ของเดือนถัดจากเดือนที่ออกใบเสร็จ**
  (issue เดือน M → เดดไลน์ 5 ของ M+1). "ใกล้ครบ" = เหลือ **≤ 3 วัน** ก่อนเดดไลน์ (default; **เก็บ threshold เป็น config ปรับได้**).
  base date อิงเดือนที่ issue (วันอนุมัติ)
- **D-69:** **สิทธิ์ตั้ง/เอาออก flag = Checker + Admin** (เท่ากับ export) — คน export คือคนเดียวกับที่รู้ว่าคีย์แล้ว → workflow เดียวกัน สอดคล้อง

### รายงานสรุปบริจาค (FR-32, SC#3)
- **D-70:** **รายงาน = เลือกช่วงเวลา → ยอดรวม + จำนวนใบ + breakdown รายเดือน/วัน; แสดงเป็นตาราง + card สรุปยอด; export Excel/CSV ได้**
  ครอบคลุม FR-32 โดยไม่ over-build; ไม่มี chart ใน MVP. รายงาน export ไม่มี PII → ไม่ต้องคุมเข้มเท่า export ปชช.
- **D-71:** **สิทธิ์ดูรายงาน = ทุก staff (Maker/Checker/Admin)** — รายงานเป็นยอดสรุป ไม่มี PII → โปร่งใส ช่วยงานหน้างานทุกคน

### Backup / Restore (NFR-08, SC#4)
- **D-72:** **Backup = pg_dump ตามตารางใน docker-compose (companion container/cron) + retention** — portable ไม่ผูก cloud vendor,
  เข้ากับ stack docker ที่มีอยู่. **ครอบคลุม DB + MinIO** (slip + PDF ที่ freeze) — object storage กู้คืนไม่ได้จาก DB → ต้องสำรองคู่กันถึงกู้ครบ
- **D-73:** **"restore verified" = runbook เอกสารขั้นตอน + ทดสอบ restore จริง (รันจริงลง fresh DB + MinIO แล้ว assert ข้อมูลครบ) + บันทึกหลักฐาน**
  — ตรง SC#4 "ทำจริงสำเร็จ" ไม่ใช่แค่ตั้งค่า. เก็บ verification evidence ในเอกสารเฟส

### PII export file handling (PDPA)
- **D-74:** **ไฟล์ export ที่มีเลขบัตรเต็ม = Stream download อย่างเดียว ไม่เก็บไฟล์บน server/MinIO** — gen ใน memory → ส่งให้โหลดทันที →
  ไม่มี plaintext-PII ไฟล์ค้างที่ไหน (ลด attack surface สุด). ตัว audit การ export อยู่ใน log แล้ว (D-64).
  ต่างจาก report export (ไม่มี PII) ที่ไม่มีข้อจำกัดนี้

### e-Donation field mapping (NFR-09, stakeholder gate)
- **D-75:** **field mapping (ลำดับ/ชื่อคอลัมน์/ค่าคงที่ cash type) = config-driven ต่อยอด D-58 config store** — แก้ได้ไม่ deploy
  เมื่อ stakeholder ยืนยันสเปก e-Donation จริง (ตรง NFR-09). รับมือกับ stakeholder gate ที่ยังค้าง (สเปกฟิลด์แน่นอน + กฎ 1 ม.ค. 2026)

### Claude's Discretion
- **schema รายละเอียด** ของ keyed flag (column บน donations vs side table), export-audit fields, config keys ของ mapping/threshold — planner ออกแบบตาม pattern Phase 1–4
- **migration number** (ต่อจาก 000012 → 000013+) และโครงสร้าง package Go (เช่น `internal/export/`, `internal/report/`, `internal/backup/`)
- **default ตัวเลข**: aging "ใกล้ครบ" threshold (แนะนำ ≤3 วัน แต่ config ได้), backup retention (จำนวนวัน/ชุด), schedule cron
- **query การ group รายงาน** (รายเดือน/วัน), CSV BOM/encoding strategy สำหรับภาษาไทย
- **กลไก MinIO backup** (mc mirror vs API dump) และรูปแบบ restore runbook — planner/research เลือกตาม hosting ที่มี

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project & Requirements
- `.planning/PROJECT.md` — core value, constraints (Security/PDPA จำกัดเห็นเลขบัตรตามบทบาท, Maintainability no-deploy config),
  Out of Scope (ไม่ต่อ API e-Donation ตรง — export แมนวลเท่านั้น)
- `.planning/REQUIREMENTS.md` — FR-30 (export), FR-31 (คีย์แล้ว flag), FR-32 (รายงาน), NFR-08 (backup/restore), NFR-09 (config no-deploy)
- `.planning/ROADMAP.md` §"Phase 5" — goal + 4 success criteria + **stakeholder gate** (สเปกฟิลด์ e-Donation จริง + กฎ 1 ม.ค. 2026)

### Load-bearing technical guidance (CLAUDE.md)
- `CLAUDE.md` §"TL;DR — The Stack" / "Detail — Supporting Libraries" — **excelize** (`github.com/xuri/excelize`) สำหรับ .xlsx ภาษาไทย,
  `encoding/csv` (stdlib) สำหรับ CSV
- `CLAUDE.md` §"PII Encryption-at-Rest" — role-based masking, decrypt + แสดงเฉพาะ role ที่อนุญาต, **ทุก reveal ถูก audit** (export ปชช. คือ reveal จำนวนมาก)
- `CLAUDE.md` §"Auth & RBAC" — audit trail append-only + hash-chain; RBAC guard ใน Go (export/flag = Checker+Admin, report = ทุก staff)
- `CLAUDE.md` §"What NOT to Use" — ห้ามเก็บ slip/ไฟล์เป็น BLOB ใน DB (backup ต้องคู่ DB+object storage)

### Prior phase context (สืบทอด)
- `.planning/phases/04-receipt-pdf-email-delivery-outbox-worker/04-CONTEXT.md` — **D-58 config store + Admin Settings UI** (เฟสนี้ต่อยอด mapping ไม่สร้างใหม่),
  freeze PDF ใน MinIO (backup ต้องรวม), donor snapshot immutability
- `.planning/phases/03-donation-lifecycle-maker-checker-issuance/03-CONTEXT.md` — **audited PII reveal** pattern (export decrypt เดินตามนี้),
  donations schema + `donor_tax_id_enc`, RBAC/SoD, audit hash-chain, `RequireAnyRole` (OR) สำหรับ multi-role guard
- `.planning/phases/02-gap-less-receipt-numbering-core/02-CONTEXT.md` — receipt number config table (base ของ config store), fiscalYear helper (Asia/Bangkok, BE) — ใช้ใน grouping รายงาน/fiscal-year filter

### Conventions
- `CLAUDE.md` §"Integration-test gate" — endpoint ใหม่ (export/flag/report) ต้องมี E2E ผ่าน real HTTP path + real Keycloak token
  (router → RequireAuth → RequireAnyRole/ResolveAppUser → handler → service → DB) ก่อนถือว่า "done"; backup/restore มี test-restore จริง

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets (พร้อมใช้จาก Phase 1–4)
- **`internal/donation/` + donations table + `internal/db/queries/donations.sql`** — record source ของ export/report/flag; search/filter (D-53) ต่อยอด date-range + keyed-status filter
- **`internal/pii/` + `internal/crypto/` (AES-256-GCM envelope)** — decrypt `donor_tax_id_enc` สำหรับ export (D-64); ต้องผ่าน audited path
- **`internal/audit/`** — audit ทุกการ export, mark/unmark flag, download (append-only hash-chain)
- **`internal/settings/` + migration 000011 config store (D-58)** — base ให้ D-75 เก็บ e-Donation field mapping + aging threshold config
- **`internal/storage/` (MinIO client)** — object storage ที่ backup (D-72) ต้องสำรอง (slip bucket + receipts bucket ที่ freeze PDF)
- **`internal/i18n/`** — header ภาษาไทยใน export/report (excelize รองรับ Unicode)
- **`internal/receiptno/` + fiscalYear helper (Phase 2)** — grouping รายงานตามปีงบประมาณ/เดือน
- **testcontainers Postgres + MinIO (`internal/testutil/`)** — integration test export/flag/report + test-restore (D-73)
- **Next.js back-office UI + BFF proxy + TanStack Query/Table** (Phase 3–4) — หน้า Export/Aging/Reports ต่อ pattern เดิม

### Established Patterns
- data = sqlc + pgx/v5, migration = golang-migrate (ล่าสุด **000012**) → เฟส 5 ต่อ **000013+**
- HTTP: Go API + `RequireAnyRole` (OR-guard, bug#3 fix) + `auth.ResolveAppUser` + Next.js BFF; excelize ยัง**ไม่อยู่ใน go.mod** → เพิ่มใหม่
- go-i18n (BE) + next-intl (FE) catalog เดียว
- docker-compose stack (postgres host 5433 override, minio, keycloak, chrome sidecar) — backup companion เสียบเข้า compose เดิม

### Integration Points (ใหม่ในเฟสนี้)
- export handler ↔ donations (issued, filter) ↔ `pii`/`crypto` decrypt ↔ excelize/csv ↔ audit + download-log → **stream response** (D-74 ไม่เก็บไฟล์)
- flag mutation ↔ donations (keyed column/side table) ↔ audit; aging view ↔ query เดดไลน์ + 3-bucket b: base = issue month
- report handler ↔ donations aggregate (SUM/COUNT group by เดือน/วัน) ↔ TanStack Table + summary card ↔ export (no PII)
- config store (D-58) ↔ e-Donation field mapping + aging threshold (D-75)
- backup companion (cron) ↔ pg_dump (Postgres) + MinIO mirror/dump → backup target; restore runbook ↔ fresh DB+MinIO verify

</code_context>

<specifics>
## Specific Ideas

- ผู้ใช้คงแนว **simple & safe / least-privilege** ต่อเนื่อง: cash type ค่าคงที่ (ไม่เพิ่ม field), รายงานไม่มี chart, export ปชช. เฉพาะ Checker+Admin
- ผู้ใช้ให้ **PDPA-first สำหรับไฟล์ export**: เลือก **stream-only ไม่เก็บไฟล์ plaintext-PII** ที่ไหนเลย (ลด attack surface สุด) — สะท้อนความใส่ใจกฎหมายตลอดโปรเจกต์
- **workflow เดียวกันของ export + flag**: คน export คือคนที่คีย์ → mark "คีย์แล้ว" → aging เตือน. ผูกสิทธิ์และลำดับงานให้ตรงกัน
- **รายงานโปร่งใสให้ทุก staff** (รวม Maker) เพราะเป็นยอดสรุปไม่มี PII — ตรงข้ามกับ export ที่คุมเข้ม
- ผู้ใช้ยืนยัน backup ต้อง **กู้ครบทั้ง DB + ไฟล์** และ **พิสูจน์ด้วยการ restore จริง** ไม่ใช่แค่ตั้ง cron
- mapping e-Donation **config-driven** เพราะสเปกฟิลด์จริงยังเป็น stakeholder gate — เตรียมให้ปรับได้ทันทีเมื่อคอนเฟิร์ม

</specifics>

<deferred>
## Deferred Ideas

- **เชื่อม API e-Donation กรมสรรพากรโดยตรง** — เกินขอบเขต milestone นี้ (export แมนวล); อาจขยายใน milestone ถัดไปหากโรงพยาบาลอยู่ใต้บังคับกฎ 1 ม.ค. 2026 (stakeholder gate)
- **chart/กราฟในรายงาน** — MVP ใช้ตาราง + card; เพิ่ม visualization ภายหลังได้
- **cash type เป็น field ต่อรายการ / 1x/2x ต่อ donation** — ยังใช้ค่าคงที่ + global config; เพิ่ม field ภายหลังได้โดยไม่ migrate ของเดิม
- **Flow B public form + acknowledgement email (FR-01..06)** — Phase 6
- **role 'Exporter'/'Accounting' แยกเฉพาะ export** — ยังไม่ทำ; ใช้ Checker+Admin ก่อน เพิ่ม role ใน Keycloak realm ภายหลังได้

None อื่น — discussion อยู่ในขอบเขตเฟส

</deferred>

---

*Phase: 5-e-donation-export-reports-admin-settings*
*Context gathered: 2026-07-06*
