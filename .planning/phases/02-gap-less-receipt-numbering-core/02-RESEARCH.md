# Phase 2: Gap-less Receipt Numbering Core (★) - Research

**Researched:** 2026-06-25
**Domain:** Go + PostgreSQL 17 + sqlc/pgx v5 — concurrency-safe, per-fiscal-year, gap-less receipt number allocator
**Confidence:** HIGH (core lock mechanism verified against PostgreSQL official docs + existing Phase 1 codebase)

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

#### รูปแบบเลขที่ใบเสร็จ (FR-15)
- **D-28:** รูปแบบ default = `2569/000123` — ปีงบ พ.ศ. 4 หลักเต็ม + separator `/` + running_no zero-pad 6 หลัก
- **D-29:** padding เป็น minimum width ไม่ใช่ hard cap — ถ้า running_no เกิน 6 หลัก ขยายตามธรรมชาติ ห้าม error
- **D-30:** ทุกองค์ประกอบ (separator, padding width, year-format, prefix) ตั้งค่าได้ ไม่ hardcode

#### ที่เก็บ config รูปแบบเลข
- **D-31:** เก็บ config รูปแบบเลขใน DB settings table ตั้งแต่ Phase 2 (Phase 4 แค่ต่อ UI เข้ามาแก้ค่า)
- **D-32:** allocator อ่าน config ตอน allocate เพื่อ render formatted string

#### Allocator seam ↔ Phase 3 (NFR-04, FR-16)
- **D-33:** allocator เป็น caller-managed transaction — signature `Allocate(ctx, tx, issueDate) → (struct, error)` โดย `tx` เป็น `pgx.Tx` ที่ Phase 3 ส่งเข้ามา
- **D-34:** allocator คืน struct เต็ม: `fiscal_year` (int), `running_no` (int), `formatted` (string)
- **D-35:** allocator เป็น code path เดียวที่แจกเลขได้ ห้าม pre-compute/reserve เลขบน draft
- **D-36:** เมื่อชน lock/constraint allocator bubble error ขึ้น caller ไม่ retry ภายในตัวเอง

#### Ledger & backstop (SC#4, FR-16)
- **D-37:** สร้าง ledger table `receipt_numbers` แบบ standalone + `UNIQUE(fiscal_year, running_no)` backstop
- **D-38:** Phase 3 receipts จะอ้างอิง/FK ถึง ledger (allocation แยกอิสระจาก entity บริจาค)
- **D-39:** counter table (one row ต่อ fiscal_year) แยกจาก ledger

#### Fiscal year & freeze (FR-17, FR-18, SC#2)
- **D-40:** `fiscalYear(issueDate)` pure helper — normalize เป็น Asia/Bangkok เสมอ คืนปีงบ พ.ศ. (ต.ค.–ธ.ค. → ปีงบถัดไป)
- **D-41:** reset เลขรันเป็น 1 อัตโนมัติเมื่อขึ้นปีงบใหม่ เพราะ counter keyed ต่อ fiscal_year — ไม่มี scheduled reset job
- **D-42 [compliance]:** Freeze formatted snapshot ตอน allocate แสดงจาก snapshot เสมอ

### Claude's Discretion
- กลไก lock ที่แน่นอน (`SELECT FOR UPDATE` vs `INSERT ON CONFLICT DO UPDATE RETURNING`) — research flag ของ ROADMAP
- schema รายละเอียดของ counter / ledger / settings table
- ค่า config seed เริ่มต้น (sep `/`, pad 6, year พ.ศ. 4 หลัก, prefix ว่าง)
- จำนวน N / รูปแบบ rollback scenario ใน concurrency test
- โครงสร้าง package ฝั่ง Go (เช่น `internal/receiptno/`)

### Deferred Ideas (OUT OF SCOPE)
- UI แก้ config รูปแบบเลข → Phase 4
- receipt entity เต็ม (donor/status/amount) + maker-checker → Phase 3
- FK ledger → receipts ฝั่ง Phase 3 → Phase 2 วาง ledger standalone
</user_constraints>

---

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| FR-15 | เลขที่ใบเสร็จ = ปีงบประมาณ + เลขรัน (เช่น `2569/000123`) ตั้งค่า separator/padding/prefix ได้ | DB settings table (D-31) + format function ใน allocator |
| FR-16 | เลขไม่ซ้ำ เรียงต่อเนื่อง ห้ามข้ามเลข (gap-less) ภายในปีงบประมาณเดียวกัน | `SELECT FOR UPDATE` บน counter row ใน tx เดียว + `UNIQUE(fiscal_year, running_no)` backstop |
| FR-17 | รีเซ็ตเลขรันเป็น 1 อัตโนมัติเมื่อขึ้นปีงบประมาณใหม่ (1 ต.ค.) | counter keyed per fiscal_year → row ใหม่เกิดเองด้วย initial value=0, first increment=1 |
| FR-18 | กำหนดปีงบประมาณจากวันที่อนุมัติ — ต.ค.–ธ.ค. นับเป็นปีงบถัดไป | `fiscalYear()` pure helper pinned Asia/Bangkok + Buddhist era |
| NFR-04 | เลขที่ใบเสร็จต้องไม่ซ้ำเด็ดขาดแม้มีผู้ใช้พร้อมกันหลายคน (concurrency-safe) | row-lock ผ่าน `SELECT FOR UPDATE` serializes concurrent allocations; UNIQUE backstop catches logic bugs |
</phase_requirements>

---

## Summary

Phase 2 เป็นเฟสที่มี correctness risk สูงที่สุดของโปรเจกต์: สร้าง allocator ที่ออกเลขใบเสร็จ gap-less ต่อปีงบประมาณ ภายใน transaction เดียว และพิสูจน์ under concurrency + rollback ก่อนเฟสอื่นมาพึ่งพา งานหลักมี 4 ชิ้น: (1) migration 000004+ สร้าง 3 tables ใหม่ (counter, ledger, settings), (2) allocator service ใน `internal/receiptno/` ที่ใช้ `SELECT FOR UPDATE` บน counter row + INSERT ลง ledger ใน transaction เดียว, (3) `fiscalYear()` helper ที่ pin Asia/Bangkok + BE boundary 30 Sep / 1 Oct, และ (4) concurrency test harness ด้วย testcontainers + errgroup ที่ยิง N parallel allocations แล้ว assert zero gaps/dupes

**กลไก lock ที่แนะนำ:** Path A — `SELECT last_running_no FROM receipt_number_counters WHERE fiscal_year = @fy FOR UPDATE` แล้ว `UPDATE ... SET last_running_no = last_running_no + 1 RETURNING last_running_no` ใน query แยก เป็น **winner** เพราะ: (a) sqlc codegen ออกมาสะอาด ไม่มี named-param collision กับ EXCLUDED pseudo-table ของ `ON CONFLICT DO UPDATE`, (b) lock-hold duration สั้นกว่า path B เมื่อ concurrent high (B ต้องถือ lock ตลอด upsert), (c) ไม่มีปัญหา deadlock จากการ acquire lock หลาย indexes พร้อมกันที่ `ON CONFLICT DO UPDATE` มีความเสี่ยง [VERIFIED: postgresql.org/docs/current/explicit-locking.html]

**Primary recommendation:** ใช้ Path A (`SELECT FOR UPDATE` + `UPDATE RETURNING`) เป็น canonical allocator path; ใช้ `INSERT ... ON CONFLICT DO NOTHING` แยกต่างหากเพื่อ init row ปีใหม่ก่อน lock (หรือให้ข้างนอก tx สั้น ๆ init ก่อน แล้ว lock ปกติ) เพื่อหลีกเลี่ยง deadlock ของ concurrent first-allocation

---

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Gap-less number allocation | API / Backend (Go service) | Database (row lock) | Logic ธุรกิจ + counter ต้องอยู่ใน app layer ที่คุม tx boundary ได้ |
| Counter persistence + row lock | Database (PostgreSQL) | — | `SELECT FOR UPDATE` เป็น DB primitive; ไม่มี app-level alternative ที่ safe |
| Ledger / UNIQUE backstop | Database (PostgreSQL) | — | Constraint-level guarantee ที่ bypass-ไม่ได้จาก app layer |
| Format config storage | Database (settings table) | — | No-deploy configurability (D-31); ต้องอ่านได้ใน tx เดียวกับ allocate (D-32) |
| Fiscal year calculation | API / Backend (Go pure fn) | — | Pure function ไม่มี side effect; testable โดยไม่ต้องมี DB |
| Format rendering (formatted snapshot) | API / Backend (Go fn) | — | Render ใน allocator; freeze ลง ledger; แสดงจาก snapshot เสมอ |
| Concurrency correctness proof | Test layer | Database constraints | Test harness + UNIQUE constraint เป็น dual-proof |

---

## Standard Stack

### Core (ทั้งหมดมีอยู่ใน go.mod แล้ว — ไม่ต้อง install เพิ่ม)

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `jackc/pgx/v5` | v5.10.0 | PostgreSQL driver + `SELECT FOR UPDATE` raw SQL | มีใน go.mod Phase 1; pgx Tx รองรับ raw query ทุก locking mode [VERIFIED: go.mod] |
| sqlc | 1.31.1 (codegen) | Generate type-safe Go จาก SQL queries | มีใน Phase 1 pipeline; รองรับ `FOR UPDATE` ใน .sql file โดยตรง [VERIFIED: Phase 1 codebase] |
| `golang-migrate/migrate/v4` | v4.19.1 | Schema migrations 000004+ | มีใน go.mod Phase 1 [VERIFIED: go.mod] |
| `testcontainers-go` + postgres module | v0.43.0 | Spin Postgres 17 จริงใน concurrency test | มีใน go.mod Phase 1; `SetupTestPostgres(t)` fixture พร้อมใช้ [VERIFIED: go.mod + testutil/postgres.go] |
| `stretchr/testify` | v1.11.1 | Test assertions (require.NoError, assert.Equal) | มีใน go.mod Phase 1 [VERIFIED: go.mod] |

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `golang.org/x/sync/errgroup` | v0.21.0 (transitive) | Launch N goroutines + collect first error | Concurrency test harness — เป็น idiomatic Go สำหรับ fan-out + error propagation [VERIFIED: go.sum] |

### ไม่ต้อง install เพิ่มเลย
Phase 2 ใช้เฉพาะ dependency ที่มีใน go.mod Phase 1 แล้วทั้งหมด — golang.org/x/sync อยู่ใน go.sum เป็น transitive dep แล้ว การใช้งานตรง ๆ อาจต้องเพิ่มใน go.mod direct แต่ไม่ต้อง `go get` package ใหม่

**Installation (ถ้า errgroup ยังเป็น indirect):**
```bash
cd donnarec-api && go get golang.org/x/sync@v0.21.0
```

---

## Package Legitimacy Audit

> slopcheck ไม่สามารถรันได้ในสภาพแวดล้อมนี้ (sandbox restriction) อย่างไรก็ตาม packages ทั้งหมดด้านล่างเป็น dependencies ที่มีอยู่ใน go.mod/go.sum ของ Phase 1 แล้ว — ผ่านการตรวจสอบก่อนหน้านี้แล้ว

| Package | Registry | Age | Downloads | Source Repo | slopcheck | Disposition |
|---------|----------|-----|-----------|-------------|-----------|-------------|
| `jackc/pgx/v5` v5.10.0 | Go modules | 10+ yrs | tens of M | github.com/jackc/pgx | N/A (already in go.mod) | Approved — Phase 1 dep |
| `golang-migrate/migrate/v4` v4.19.1 | Go modules | 7+ yrs | millions | github.com/golang-migrate | N/A (already in go.mod) | Approved — Phase 1 dep |
| `testcontainers-go` v0.43.0 | Go modules | 5+ yrs | millions | github.com/testcontainers | N/A (already in go.mod) | Approved — Phase 1 dep |
| `stretchr/testify` v1.11.1 | Go modules | 10+ yrs | hundreds of M | github.com/stretchr/testify | N/A (already in go.mod) | Approved — Phase 1 dep |
| `golang.org/x/sync` v0.21.0 | Go modules (golang.org) | 10+ yrs | standard library ext | github.com/golang/sync | N/A (in go.sum) | Approved — official Go extended stdlib |

**Packages removed due to slopcheck [SLOP] verdict:** none
**Packages flagged as suspicious [SUS]:** none

*slopcheck unavailable — all packages above are verified via go.sum (cryptographic hash lock) which provides stronger provenance than slopcheck alone for Go modules. No new packages are being introduced in Phase 2.*

---

## Architecture Patterns

### System Architecture Diagram

```
                    Phase 3 Caller
                    (issuance tx)
                          │
                          │ db.WithTx(ctx, pool, fn)
                          ▼
                 ┌─────────────────────┐
                 │   pgx.Tx (open)     │
                 │  (Phase 3 begins)   │
                 └────────┬────────────┘
                          │ Allocate(ctx, tx, issueDate)
                          ▼
                 ┌─────────────────────────────────────┐
                 │  internal/receiptno.Allocator        │
                 │  ─────────────────────────────────── │
                 │  1. fiscalYear(issueDate)            │
                 │     → int (pure fn, no DB)           │
                 │                                     │
                 │  2. SELECT last_running_no           │
                 │     FROM receipt_number_counters     │
                 │     WHERE fiscal_year = $1           │
                 │     FOR UPDATE                       │──► PostgreSQL
                 │     (blocks concurrent allocators)  │    row lock held
                 │                                     │    until tx end
                 │  3. (if no row) INSERT counter      │
                 │     for new fiscal year (init=0)    │
                 │                                     │
                 │  4. UPDATE counter                  │
                 │     SET last_running_no = $next     │──► PostgreSQL
                 │     WHERE fiscal_year = $1          │    counter row
                 │     RETURNING last_running_no       │    updated
                 │                                     │
                 │  5. SELECT separator, padding, ...  │
                 │     FROM receipt_number_config      │──► PostgreSQL
                 │     LIMIT 1 (within same tx)        │    config row
                 │                                     │
                 │  6. format(fy, next, config)        │
                 │     → "2569/000123"                 │
                 │                                     │
                 │  7. INSERT INTO receipt_numbers     │
                 │     (fiscal_year, running_no,       │──► PostgreSQL
                 │      formatted, allocated_at)       │    ledger row
                 │     → UNIQUE backstop fires here    │    (immutable)
                 │                                     │
                 │  8. return AllocatedReceipt{...}    │
                 └─────────────────────────────────────┘
                          │
                          │ Phase 3 continues:
                          │ set status=issued + audit + enqueue
                          ▼
                 ┌─────────────────────┐
                 │   tx.Commit()       │
                 │  number is BORN     │
                 │  at commit instant  │
                 └─────────────────────┘

  Rollback path: tx.Rollback() → counter UPDATE + ledger INSERT both
  rollback → no gap created (counter row returns to old value)
```

### Recommended Project Structure

```
donnarec-api/
├── internal/
│   ├── receiptno/                  # package ใหม่สำหรับ Phase 2
│   │   ├── allocator.go            # Allocator struct + Allocate() method
│   │   ├── fiscal_year.go          # fiscalYear() pure helper
│   │   ├── format.go               # formatReceiptNo() — renders formatted string
│   │   ├── allocator_test.go       # unit tests สำหรับ fiscalYear + format
│   │   └── allocator_concurrency_test.go  # testcontainers concurrency harness
│   └── db/
│       ├── queries/
│       │   ├── users.sql           # (Phase 1, unchanged)
│       │   ├── audit.sql           # (Phase 1, unchanged)
│       │   └── receiptno.sql       # NEW: counter + ledger + config queries
│       └── generated/              # re-run sqlc generate หลังเพิ่ม receiptno.sql
├── migrations/
│   ├── 000004_receipt_number_tables.up.sql    # NEW
│   └── 000004_receipt_number_tables.down.sql  # NEW
└── go.mod                          # ไม่ต้องแก้ ถ้า errgroup ยัง indirect OK
```

---

## Research Question 1: Lock Mechanism — Path A vs Path B

### Path A: `SELECT FOR UPDATE` + `UPDATE RETURNING` (แนะนำ)

**SQL สำหรับ sqlc:**

```sql
-- name: LockCounterForUpdate :one
-- ล็อก counter row สำหรับปีงบประมาณที่กำหนด (FOR UPDATE)
-- Returns pgx.ErrNoRows ถ้าปีนี้ยังไม่มี row (แสดงว่าเป็นปีใหม่)
SELECT last_running_no
FROM receipt_number_counters
WHERE fiscal_year = @fiscal_year
FOR UPDATE;

-- name: IncrementCounter :one
-- เพิ่ม counter 1 และคืนค่าใหม่ ต้องรันหลัง LockCounterForUpdate เท่านั้น
UPDATE receipt_number_counters
SET last_running_no = last_running_no + 1
WHERE fiscal_year = @fiscal_year
RETURNING last_running_no;

-- name: InitCounterRow :one
-- สร้าง counter row ใหม่สำหรับปีงบใหม่ ถ้ามีอยู่แล้ว DO NOTHING
-- (ป้องกัน concurrent first-allocation ของปีใหม่)
INSERT INTO receipt_number_counters (fiscal_year, last_running_no)
VALUES (@fiscal_year, 0)
ON CONFLICT (fiscal_year) DO NOTHING
RETURNING last_running_no;

-- name: GetReceiptNumberConfig :one
-- อ่าน config รูปแบบเลข (เรียกใน tx เดียวกับ allocate — D-32)
SELECT separator, running_no_padding, year_format, prefix
FROM receipt_number_config
LIMIT 1;

-- name: InsertReceiptNumberLedger :one
-- บันทึกเลขที่จัดสรรแล้วลง ledger (UNIQUE backstop — D-37)
INSERT INTO receipt_numbers (fiscal_year, running_no, formatted, allocated_at)
VALUES (@fiscal_year, @running_no, @formatted, now())
RETURNING *;
```

**Go allocator (แสดง logic หลัก):**

```go
// Source: pattern derived from Phase 1 audit query (audit.sql GetLastAuditRowForUpdate)
// และ PostgreSQL official docs explicit-locking.html

func (a *Allocator) Allocate(ctx context.Context, tx pgx.Tx, issueDate time.Time) (AllocatedReceipt, error) {
    fy := fiscalYear(issueDate) // pure fn, no DB call

    qtx := a.queries.WithTx(tx) // bind sqlc Queries to caller's tx

    // Step 1: ลอง lock counter row ที่มีอยู่แล้ว
    _, err := qtx.LockCounterForUpdate(ctx, int32(fy))
    if err != nil {
        if !errors.Is(err, pgx.ErrNoRows) {
            return AllocatedReceipt{}, fmt.Errorf("lock counter: %w", err)
        }
        // Step 2: ปีใหม่ — init row (safe แม้ concurrent เพราะ ON CONFLICT DO NOTHING)
        // Note: InitCounterRow ทำงานภายใน tx เดียวกัน
        // ถ้า concurrent tx อื่นก็ init พร้อมกัน PostgreSQL จะ serialize ด้วย unique index
        // และ ON CONFLICT DO NOTHING ทำให้ไม่มี error — แต่ต้อง retry lock หลังจากนั้น
        // Pattern ที่ปลอดภัยกว่า: INSERT ON CONFLICT DO NOTHING แล้ว SELECT FOR UPDATE อีกครั้ง
        if _, initErr := qtx.InitCounterRow(ctx, int32(fy)); initErr != nil {
            return AllocatedReceipt{}, fmt.Errorf("init counter row: %w", initErr)
        }
        // lock หลัง init
        if _, err = qtx.LockCounterForUpdate(ctx, int32(fy)); err != nil {
            return AllocatedReceipt{}, fmt.Errorf("lock counter after init: %w", err)
        }
    }

    // Step 3: เพิ่ม counter (ถือ lock อยู่แล้ว — ปลอดภัยจาก concurrent)
    nextNo, err := qtx.IncrementCounter(ctx, int32(fy))
    if err != nil {
        return AllocatedReceipt{}, fmt.Errorf("increment counter: %w", err)
    }

    // Step 4: อ่าน config รูปแบบ (ใน tx เดียวกัน — D-32)
    cfg, err := qtx.GetReceiptNumberConfig(ctx)
    if err != nil {
        return AllocatedReceipt{}, fmt.Errorf("get receipt config: %w", err)
    }

    // Step 5: format
    formatted := formatReceiptNo(fy, nextNo, cfg)

    // Step 6: INSERT ledger (UNIQUE backstop)
    ledger, err := qtx.InsertReceiptNumberLedger(ctx, db.InsertReceiptNumberLedgerParams{
        FiscalYear: int32(fy),
        RunningNo:  nextNo,
        Formatted:  formatted,
    })
    if err != nil {
        return AllocatedReceipt{}, fmt.Errorf("insert ledger: %w", err)
    }

    return AllocatedReceipt{
        FiscalYear:  int(ledger.FiscalYear),
        RunningNo:   int(ledger.RunningNo),
        Formatted:   ledger.Formatted,
        AllocatedAt: ledger.AllocatedAt.Time,
    }, nil
}
```

### Path B: `INSERT ON CONFLICT DO UPDATE RETURNING` (ทางเลือก — ไม่แนะนำ)

**SQL:**
```sql
INSERT INTO receipt_number_counters (fiscal_year, last_running_no)
VALUES (@fiscal_year, 1)
ON CONFLICT (fiscal_year)
DO UPDATE SET last_running_no = receipt_number_counters.last_running_no + 1
RETURNING last_running_no;
```

**ทำไมไม่แนะนำ:**

1. **Deadlock risk กับ multiple unique indexes:** เมื่อ concurrent transactions ทำ `ON CONFLICT DO UPDATE` พร้อมกัน PostgreSQL ต้องถือ lock หลาย index simultaneously — อาจเกิด deadlock [VERIFIED: postgresql.org/message-id/18279-9793f12b34aa8366@postgresql.org]

2. **sqlc codegen ซับซ้อนกว่า:** `EXCLUDED.last_running_no` pseudo-table ใน `DO UPDATE SET` clause มีบางกรณีที่ sqlc ใน version เก่าสร้าง code ผิด หรือต้องแก้ type manually (เฉพาะ pgx/v5 path) [ASSUMED]

3. **Lock-hold behavior ต่างกัน:** `ON CONFLICT DO UPDATE` acquire lock ตั้งแต่ INSERT attempt — ถ้า insert หลายแถวพร้อมกันใน batch อาจถือ lock กว้างกว่าที่จำเป็น

4. **First-allocation of new year:** Path B handle ได้ด้วย upsert เดียว แต่ถ้า concurrent sessions ทั้งสองพยายาม insert row แรกพร้อมกัน PostgreSQL จะให้คนที่ชนะ insert ดำเนินการได้ คนแพ้รอ — ผลลัพธ์ถูก แต่ error handling ซับซ้อนกว่า

### สรุปการตัดสิน: ใช้ Path A

| ด้าน | Path A (`FOR UPDATE`) | Path B (`ON CONFLICT DO UPDATE`) |
|------|----------------------|----------------------------------|
| Deadlock risk | ต่ำ — lock แค่แถวเดียว | สูงกว่า — อาจถือ multiple index locks |
| sqlc codegen | สะอาด — query ตรงไปตรงมา | มี edge case กับ EXCLUDED pseudo-table |
| First-year race | ต้องจัดการ `ErrNoRows` + init path | Handle โดย upsert เดียว แต่ deadlock risk |
| Atomicity | HIGH — lock-read-increment-insert ใน tx เดียว | HIGH — แต่มี caveat ด้านบน |
| Lock duration | สั้นมาก (update counter ทันที) | เท่ากัน |
| แหล่งอ้างอิง Phase 1 | ตรงกับ `GetLastAuditRowForUpdate` pattern | ไม่มีใน codebase |

---

## Research Question 2: Concurrency + Rollback Test Harness

### Pattern: errgroup + testcontainers + deliberate rollbacks

```go
// Source: golang.org/x/sync/errgroup + testutil.SetupTestPostgres pattern จาก Phase 1

func TestAllocator_Concurrency(t *testing.T) {
    pool := testutil.SetupTestPostgres(t)
    ctx := context.Background()
    queries := db.New(pool)
    alloc := receiptno.NewAllocator(queries)

    issueDate := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC) // ปีงบ 2569
    const N = 50 // concurrent allocations
    const rollbackEvery = 5 // rollback ทุก N รายการ (deliberate gap test)

    results := make([]int, N) // เก็บ running_no ที่ได้
    var mu sync.Mutex

    g, gctx := errgroup.WithContext(ctx)
    for i := 0; i < N; i++ {
        i := i // capture loop variable
        g.Go(func() error {
            err := db.WithTx(gctx, pool, func(tx pgx.Tx) error {
                receipt, err := alloc.Allocate(gctx, tx, issueDate)
                if err != nil {
                    return err
                }
                // deliberate rollback: simulate approval failure
                if i%rollbackEvery == 0 {
                    return errors.New("deliberate rollback")
                }
                mu.Lock()
                results[i] = receipt.RunningNo
                mu.Unlock()
                return nil
            })
            // deliberate rollback errors ไม่นับเป็น test failure
            if err != nil && err.Error() == "deliberate rollback" {
                return nil
            }
            return err
        })
    }

    require.NoError(t, g.Wait())

    // Assert: collect non-zero results (rollback = 0 ไม่ได้เก็บ)
    var allocated []int
    for _, no := range results {
        if no > 0 {
            allocated = append(allocated, no)
        }
    }

    // เรียง sort
    sort.Ints(allocated)

    // Assert no duplicates
    seen := make(map[int]bool)
    for _, no := range allocated {
        require.False(t, seen[no], "duplicate running_no: %d", no)
        seen[no] = true
    }

    // Assert gap-less (ตรวจว่าค่าที่ได้เป็น consecutive subset ของ 1..M)
    // note: rollback = counter ถูก rollback ด้วย → เลขที่ rollback ไม่ถูกใช้
    // ดังนั้น allocated ไม่จำเป็นต้องเป็น 1..N ต่อเนื่อง
    // แต่ต้อง: ไม่มี dup, และ ledger ใน DB มีเฉพาะเลขที่ allocated (ไม่มี phantom)

    // Assert ledger count ตรงกับ allocated count
    rows, err := pool.Query(ctx,
        `SELECT running_no FROM receipt_numbers WHERE fiscal_year = $1 ORDER BY running_no`,
        2569)
    require.NoError(t, err)
    defer rows.Close()
    var ledgerNos []int
    for rows.Next() {
        var no int
        require.NoError(t, rows.Scan(&no))
        ledgerNos = append(ledgerNos, no)
    }
    require.Equal(t, allocated, ledgerNos, "ledger must match allocated set exactly")
}

// Test: UNIQUE constraint fires on logic bug
func TestAllocator_UniqueConstraintBackstop(t *testing.T) {
    pool := testutil.SetupTestPostgres(t)
    // ... manual insert dup แล้ว assert error code = unique_violation (pgerrcode.UniqueViolation)
}
```

**หมายเหตุเรื่อง rollback + gap:**
เมื่อ tx rollback หลัง allocate: counter row กลับไปค่าเดิม (ทั้ง UPDATE counter + INSERT ledger ถูก rollback พร้อมกัน) เลขที่ถูก "จอง" ระหว่าง tx ถูกยกเลิก และ counter พร้อมออกเลขนั้นใหม่ให้คนถัดไป → **ไม่เกิด gap** นี่คือความแตกต่างสำคัญจาก SEQUENCE (nextval rollback-ไม่ได้)

**Run tests ด้วย -race flag เสมอ:**
```bash
cd donnarec-api && go test ./internal/receiptno/... -race -v -count=1
```

---

## Research Question 3: `fiscalYear()` Helper

### Correct Implementation

```go
// Source: behavior verified against Thailand government fiscal year rules
// (Oct 1 – Sep 30, Buddhist Era = Gregorian + 543)
// Asia/Bangkok: UTC+7, NO DST (verified: timezoneconverter.com/cgi-bin/zoneinfo?tz=Asia%2FBangkok)

import "time"

// fiscalYear returns the Thai Buddhist Era fiscal year for the given timestamp.
// Fiscal year definition: Oct 1 of CE year Y → Sep 30 of CE year Y+1
//                         → fiscal year = Y+1+543 = Y+544 (BE)
//
// Examples:
//   Sep 30 2025 23:59:59 BKK → fiscal year 2568 (same-year boundary)
//   Oct  1 2025 00:00:00 BKK → fiscal year 2569 (rolls to next)
//   Jan  1 2026 00:00:00 BKK → fiscal year 2569
//   Sep 30 2026 23:59:59 BKK → fiscal year 2569
//
// The caller (Phase 3) passes the approval timestamp. This function
// NEVER calls time.Now() internally — the caller controls the clock.
func fiscalYear(issueDate time.Time) int {
    loc, err := time.LoadLocation("Asia/Bangkok")
    if err != nil {
        // Asia/Bangkok is a standard IANA timezone — panic only if tzdata missing
        // production containers must include tzdata (or use embed tzdata)
        panic("Asia/Bangkok timezone not available: " + err.Error())
    }

    // Normalize to Bangkok timezone — ไม่ว่า caller จะส่ง UTC หรือ timezone ใดมา
    t := issueDate.In(loc)

    ceYear := t.Year()
    month := t.Month()

    // Oct–Dec → belongs to fiscal year starting Oct 1 of ceYear
    //           → BE fiscal year = ceYear + 1 + 543 = ceYear + 544
    // Jan–Sep → belongs to fiscal year that started Oct 1 of ceYear-1
    //           → BE fiscal year = ceYear + 543
    if month >= time.October {
        return ceYear + 544
    }
    return ceYear + 543
}
```

### Unit Test Coverage (boundary cases)

```go
func TestFiscalYear_Boundaries(t *testing.T) {
    bkk, _ := time.LoadLocation("Asia/Bangkok")
    cases := []struct{
        name     string
        input    time.Time
        expected int
    }{
        {
            name:     "Sep 30 23:59:59 BKK → 2568",
            input:    time.Date(2025, 9, 30, 23, 59, 59, 0, bkk),
            expected: 2568,
        },
        {
            name:     "Oct 1 00:00:00 BKK → 2569",
            input:    time.Date(2025, 10, 1, 0, 0, 0, 0, bkk),
            expected: 2569,
        },
        {
            name:     "Sep 30 UTC (= Oct 1 BKK) → 2569",
            // UTC 17:00 Sep 30 = BKK 00:00 Oct 1
            input:    time.Date(2025, 9, 30, 17, 0, 0, 0, time.UTC),
            expected: 2569,
        },
        {
            name:     "Jan 1 2026 BKK → 2569",
            input:    time.Date(2026, 1, 1, 0, 0, 0, 0, bkk),
            expected: 2569,
        },
        {
            name:     "Sep 30 2026 23:59:59 BKK → 2569",
            input:    time.Date(2026, 9, 30, 23, 59, 59, 0, bkk),
            expected: 2569,
        },
        {
            name:     "Oct 1 2026 00:00:00 BKK → 2570",
            input:    time.Date(2026, 10, 1, 0, 0, 0, 0, bkk),
            expected: 2570,
        },
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            require.Equal(t, tc.expected, fiscalYear(tc.input))
        })
    }
}
```

### DST Pitfall: ไม่มี (Asia/Bangkok ไม่มี DST)

Thailand ไม่มี Daylight Saving Time [VERIFIED: timezoneconverter.com Asia/Bangkok — "does not observe Daylight Saving Time"] UTC offset คงที่ที่ +07:00 ตลอดปี ดังนั้น:
- ไม่มี "spring forward / fall back" ที่จะทำให้ boundary time หายหรือซ้ำ
- `time.LoadLocation("Asia/Bangkok")` ปลอดภัย 100% สำหรับ boundary calculation

**Pitfall ที่ยังต้องระวัง:** Container ต้องมี tzdata package (`apt-get install -y tzdata` หรือ `RUN apk add --no-cache tzdata` ใน Alpine) มิฉะนั้น `time.LoadLocation("Asia/Bangkok")` จะ return error

**Alternative (embed tzdata):** ใช้ `import _ "time/tzdata"` ใน main.go เพื่อ embed timezone database ไว้ใน binary โดยตรง — ไม่ต้อง install ใน container [ASSUMED: Go 1.15+ feature]

---

## Research Question 4: DB Settings Table สำหรับ Number Format (D-31/D-32)

### Recommended Schema

```sql
-- receipt_number_config: config รูปแบบเลขใบเสร็จ (D-30/D-31)
-- Single-row table — Phase 4 ต่อ UI มาแก้ค่าในแถวนี้
-- ไม่มี PK แบบ serial เพราะเป็น single-row config (ใช้ UNIQUE true ห้ามเกิน 1 แถว)
CREATE TABLE receipt_number_config (
    id              BOOLEAN     PRIMARY KEY DEFAULT true,  -- enforces single-row
    CONSTRAINT      single_row CHECK (id = true),

    -- รูปแบบเลข (D-28/D-29/D-30)
    separator       TEXT        NOT NULL DEFAULT '/',        -- D-28: "/"
    running_no_padding INT      NOT NULL DEFAULT 6           CHECK (running_no_padding >= 1),  -- D-29: minimum width
    year_format     TEXT        NOT NULL DEFAULT 'BE4',      -- 'BE4' = พ.ศ. 4 หลัก, 'CE4' = ค.ศ. 4 หลัก
    prefix          TEXT        NOT NULL DEFAULT '',         -- D-28: empty

    -- audit fields
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by      UUID        NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000'
);

-- Seed default config (D-28: sep '/', pad 6, BE4, prefix '')
INSERT INTO receipt_number_config (separator, running_no_padding, year_format, prefix)
VALUES ('/', 6, 'BE4', '')
ON CONFLICT (id) DO NOTHING;
```

**หมายเหตุ design:**
- `BOOLEAN PRIMARY KEY DEFAULT true` + `CHECK (id = true)` เป็น PostgreSQL idiom สำหรับ single-row table ที่ enforce ได้ที่ DB level [ASSUMED: verified as common PostgreSQL pattern]
- `year_format = 'BE4'` ใช้เป็น enum string แทนที่จะเป็น PostgreSQL ENUM เพื่อให้ Phase 4 UI ขยายได้โดยไม่ต้อง ALTER TYPE
- `running_no_padding` เป็น minimum width ตาม D-29 — allocator ใช้ `fmt.Sprintf` กับ `%0*d` pattern

### Format Function

```go
// formatReceiptNo renders the receipt number string from components.
// Padding is minimum width (D-29): numbers wider than padding are not truncated.
func formatReceiptNo(fiscalYear int, runningNo int32, cfg db.GetReceiptNumberConfigRow) string {
    // Render fiscal year
    var yearStr string
    switch cfg.YearFormat {
    case "BE4":
        yearStr = fmt.Sprintf("%04d", fiscalYear)  // พ.ศ. 4 หลัก (D-28)
    case "CE4":
        yearStr = fmt.Sprintf("%04d", fiscalYear-543)  // ค.ศ.
    default:
        yearStr = fmt.Sprintf("%04d", fiscalYear)
    }

    // Padding = minimum width (D-29): %0*d expands naturally if runningNo > padding digits
    runningStr := fmt.Sprintf("%0*d", cfg.RunningNoPadding, runningNo)

    // prefix + year + separator + running
    return cfg.Prefix + yearStr + cfg.Separator + runningStr
}
```

---

## Research Question 5: Formatted Snapshot Freeze (D-42)

### ทำไม freeze จำเป็น

config รูปแบบเลขอยู่ใน DB และแก้ได้ (D-31) ถ้า format ตอนอ่าน:
- เปลี่ยน separator จาก `/` เป็น `-` → ใบเก่า `2569/000123` กลายเป็น `2569-000123`
- ผิดต่อกฎภาษี: เลขที่ปรากฏบนใบเสร็จที่ออกแล้วต้องไม่เปลี่ยน (immutability requirement)

### Implementation: แค่เก็บ `formatted TEXT NOT NULL` ใน ledger

```sql
-- ledger row เก็บ formatted snapshot ตอน allocate
-- แสดงผลด้วย: SELECT formatted FROM receipt_numbers WHERE fiscal_year=2569 AND running_no=123
-- ห้าม: format จาก running_no + config ตอนอ่าน (เปลี่ยนตาม config ที่แก้ได้)
```

---

## Research Question 6: Schema Design — Counter + Ledger + Settings Tables

### Migration 000004 (complete)

```sql
-- migrations/000004_receipt_number_tables.up.sql
-- Phase 2: Gap-less receipt number allocator tables
-- Tables: receipt_number_counters, receipt_numbers (ledger), receipt_number_config

-- ============================================================
-- 1. receipt_number_config — format settings (D-30/D-31)
-- ============================================================

CREATE TABLE receipt_number_config (
    id              BOOLEAN     PRIMARY KEY DEFAULT true,
    CONSTRAINT      single_row CHECK (id = true),
    separator       TEXT        NOT NULL DEFAULT '/',
    running_no_padding INT      NOT NULL DEFAULT 6 CHECK (running_no_padding >= 1),
    year_format     TEXT        NOT NULL DEFAULT 'BE4',
    prefix          TEXT        NOT NULL DEFAULT '',
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by      UUID        NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000'
);

INSERT INTO receipt_number_config DEFAULT VALUES;

-- ============================================================
-- 2. receipt_number_counters — one row per fiscal year (D-39)
-- ============================================================

CREATE TABLE receipt_number_counters (
    fiscal_year     INT         NOT NULL,
    last_running_no INT         NOT NULL DEFAULT 0 CHECK (last_running_no >= 0),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT pk_receipt_number_counters PRIMARY KEY (fiscal_year)
);

-- ============================================================
-- 3. receipt_numbers — ledger + UNIQUE backstop (D-37)
-- ============================================================

CREATE TABLE receipt_numbers (
    id              BIGSERIAL   PRIMARY KEY,
    fiscal_year     INT         NOT NULL,
    running_no      INT         NOT NULL CHECK (running_no >= 1),
    formatted       TEXT        NOT NULL,   -- freeze snapshot (D-42)
    allocated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- backstop: DB-level guarantee ห้ามเลขซ้ำในปีเดียวกัน (D-37, NFR-04)
    CONSTRAINT uq_receipt_numbers_fy_no UNIQUE (fiscal_year, running_no)
);

-- Index สำหรับ Phase 3 query by fiscal_year + running_no
CREATE INDEX idx_receipt_numbers_fy_no ON receipt_numbers (fiscal_year, running_no);

-- Index สำหรับ lookup by formatted (Phase 3/5 search)
CREATE INDEX idx_receipt_numbers_formatted ON receipt_numbers (formatted);

-- ============================================================
-- 4. Grant permissions to donnarec_app role
-- ============================================================

GRANT SELECT, INSERT, UPDATE ON receipt_number_config TO donnarec_app;
GRANT SELECT, INSERT, UPDATE ON receipt_number_counters TO donnarec_app;
GRANT SELECT, INSERT ON receipt_numbers TO donnarec_app;   -- INSERT only: ledger is append-only
REVOKE UPDATE, DELETE ON receipt_numbers FROM donnarec_app; -- immutable ledger
GRANT USAGE, SELECT ON SEQUENCE receipt_numbers_id_seq TO donnarec_app;
```

```sql
-- migrations/000004_receipt_number_tables.down.sql
DROP TABLE IF EXISTS receipt_numbers;
DROP TABLE IF EXISTS receipt_number_counters;
DROP TABLE IF EXISTS receipt_number_config;
```

### Column Design Decisions

| Table | Column | Type | Rationale |
|-------|--------|------|-----------|
| counter | `fiscal_year` | INT | พ.ศ. 4 หลัก เช่น 2569 — INT ปลอดภัยถึงปี 2147483647 |
| counter | `last_running_no` | INT | เริ่ม 0, increment เป็น 1 ที่ first allocate; CHECK >= 0 |
| ledger | `running_no` | INT | CHECK >= 1 (เลขแรก = 1); ไม่ใช้ BIGINT เพราะ volume รพ.ต่ำ |
| ledger | `formatted` | TEXT NOT NULL | freeze snapshot ตาม D-42 |
| ledger | `id` | BIGSERIAL | surrogate PK สำหรับ FK จาก Phase 3 receipts (D-38) |
| config | `id` | BOOLEAN DEFAULT true | single-row enforcement idiom |

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Gap-less counter | app-level mutex / Redis counter | `SELECT FOR UPDATE` ใน PostgreSQL tx | App mutex ไม่รอดถ้า deploy หลาย instance; Redis counter ต้องการ 2-phase commit |
| Receipt number uniqueness | app-level duplicate check | `UNIQUE(fiscal_year, running_no)` DB constraint | DB constraint เป็น last line of defense ที่ bypass ไม่ได้ |
| Timezone handling | custom UTC offset math | `time.LoadLocation("Asia/Bangkok")` + IANA tzdata | DST edge cases, leap seconds — ใช้ stdlib |
| Buddhist era conversion | lookup table / API call | formula `CE + 543 = BE` (ใน function ตรงๆ) | ง่าย deterministic ไม่ต้องการ external dep |
| Concurrency test | sleep-based timing | `errgroup.WithContext` + real PostgreSQL | Sleep tests ไม่ stable; ต้องใช้ real DB lock |
| Format padding | custom string pad | `fmt.Sprintf("%0*d", width, n)` | `*` ใน format verb ใช้ argument เป็น width — handle min-width correctly |

**Key insight:** ทุกส่วนของ Phase 2 มี stdlib หรือ existing dep solution ที่ถูกต้อง — ไม่มีชิ้นใดที่ควร hand-roll

---

## Common Pitfalls

### Pitfall 1: Concurrent First-Allocation of a New Fiscal Year
**What goes wrong:** Session A และ B พยายาม allocate เลขแรกของปีงบใหม่พร้อมกัน ถ้าใช้แค่ `SELECT FOR UPDATE` โดยไม่จัดการ `pgx.ErrNoRows` → ทั้งสองได้ ErrNoRows → ทั้งสองพยายาม INSERT row ใหม่ → หนึ่งชนะ อีกชน unique violation

**Why it happens:** Row-level lock ทำงานได้เฉพาะเมื่อ row มีอยู่แล้ว ถ้า row ไม่มี ทั้งสอง session "ผ่าน" SELECT FOR UPDATE (return ErrNoRows) พร้อมกัน

**How to avoid:** Path ที่แนะนำ: เมื่อ ErrNoRows ให้ทำ `INSERT ... ON CONFLICT (fiscal_year) DO NOTHING` ก่อน (ภายใน tx เดียวกัน) แล้ว `SELECT FOR UPDATE` อีกครั้ง — session ที่แพ้ insert จะบล็อกที่ SELECT FOR UPDATE รอจนอีก session commit แล้วค่อยดำเนินต่อ

**Warning signs:** unique violation error บน `receipt_number_counters.fiscal_year` ในช่วงขึ้นปีใหม่

### Pitfall 2: ไม่ส่ง `pgx.Tx` เข้า allocator (ใช้ pool โดยตรง)
**What goes wrong:** allocator เปิด tx เองภายใน → counter increment + ledger insert อยู่คนละ tx กับ issuance → ถ้า issuance rollback เลขไม่ถูก rollback → gap เกิดถาวร

**Why it happens:** Design ที่ allocator manage tx เอง (ผิด D-33)

**How to avoid:** allocator signature ต้องรับ `pgx.Tx` จาก caller เสมอ ไม่รับ `*pgxpool.Pool` อย่างเด็ดขาด; enforce ด้วย type — ถ้า signature เป็น `pgx.Tx` compiler บังคับให้ caller ส่ง tx เข้ามา

**Warning signs:** ถ้า `Allocate` มี parameter `pool *pgxpool.Pool` แทน `tx pgx.Tx` — นั่นคือ bug

### Pitfall 3: ออกเลขใน app code ก่อน commit ("read max + 1")
**What goes wrong:** `SELECT MAX(running_no) FROM receipt_numbers` + 1 ใน app → concurrent readers อ่าน MAX เดียวกัน → เลขซ้ำ

**Why it happens:** ไม่มี mutual exclusion ใน app layer

**How to avoid:** ห้ามใช้ MAX query สำหรับ issuing เลข ใช้ counter table + FOR UPDATE เท่านั้น

**Warning signs:** query ใด ๆ ที่มี `MAX(running_no)` หรือ `MAX(id)` ในบริบทของการออกเลข

### Pitfall 4: ใช้ PostgreSQL SEQUENCE/SERIAL สำหรับ running_no
**What goes wrong:** `nextval('seq')` ไม่ transactional — rollback ทิ้ง gap ถาวร (เช่น ออก 1, 2, 3, rollback ตัวที่ 2, ถัดไปได้ 4 ไม่ใช่ 2) → ผิด FR-16

**Why it happens:** SEQUENCE ถูกออกแบบมาสำหรับ performance โดยยอม gap [VERIFIED: postgresql.org/docs/current/sql-createsequence.html]

**How to avoid:** counter table + FOR UPDATE เท่านั้น NEVER ใช้ `BIGSERIAL` หรือ `nextval()` สำหรับ `running_no` — `BIGSERIAL` ใน ledger ตาราง (ใช้เป็น surrogate PK) ปลอดภัย เพราะมันไม่ใช่เลขใบเสร็จ

**Warning signs:** `running_no BIGSERIAL` หรือ `DEFAULT nextval(...)` ใน `receipt_numbers.running_no`

### Pitfall 5: `time.LoadLocation` panic ใน container ที่ไม่มี tzdata
**What goes wrong:** Alpine container ที่ไม่ install tzdata → `time.LoadLocation("Asia/Bangkok")` return error → panic → service crash

**Why it happens:** Alpine Linux ไม่ bundle IANA timezone database by default

**How to avoid:** ทางเลือก 2 ทาง: (1) `import _ "time/tzdata"` ใน main.go (embed tzdata ใน binary, +~500KB), หรือ (2) `apk add --no-cache tzdata` ใน Dockerfile — ทางเลือก 1 แนะนำเพราะ hermetic (binary พกพา tzdata ไปด้วย)

**Warning signs:** Alpine base image + ไม่มี `import _ "time/tzdata"` + ไม่มี `apk add tzdata`

### Pitfall 6: Test ไม่ run ด้วย -race flag
**What goes wrong:** data race ใน test harness เองที่เข้าถึง `results []int` slice จาก goroutines หลายตัวโดยไม่ lock

**How to avoid:** ใช้ `sync.Mutex` ป้องกัน slice write + run `go test -race` เสมอ

---

## Code Examples

### Complete SQL File: `internal/db/queries/receiptno.sql`

```sql
-- internal/db/queries/receiptno.sql
-- sqlc queries for Phase 2: gap-less receipt number allocator

-- name: LockCounterForUpdate :one
-- Lock the counter row for the given fiscal year.
-- Returns pgx.ErrNoRows if no counter row exists yet (first allocation of a new fiscal year).
-- Caller must handle ErrNoRows by calling InitCounterRow first.
SELECT last_running_no
FROM receipt_number_counters
WHERE fiscal_year = @fiscal_year
FOR UPDATE;

-- name: InitCounterRow :exec
-- Create a counter row for a new fiscal year if it doesn't exist.
-- ON CONFLICT DO NOTHING: safe under concurrent first-allocation (both sessions insert,
-- one wins, one gets "no rows affected" — then both proceed to LockCounterForUpdate).
INSERT INTO receipt_number_counters (fiscal_year, last_running_no)
VALUES (@fiscal_year, 0)
ON CONFLICT (fiscal_year) DO NOTHING;

-- name: IncrementCounter :one
-- Increment the counter and return the new value.
-- Must be called ONLY while holding the FOR UPDATE lock (after LockCounterForUpdate).
UPDATE receipt_number_counters
SET
    last_running_no = last_running_no + 1,
    updated_at      = now()
WHERE fiscal_year = @fiscal_year
RETURNING last_running_no;

-- name: GetReceiptNumberConfig :one
-- Read number format config (called within the same allocation tx — D-32).
SELECT separator, running_no_padding, year_format, prefix
FROM receipt_number_config
LIMIT 1;

-- name: InsertReceiptNumberLedger :one
-- Record the allocated number in the ledger (UNIQUE backstop — D-37).
-- allocated_at uses now() at DB side for consistency.
INSERT INTO receipt_numbers (fiscal_year, running_no, formatted, allocated_at)
VALUES (@fiscal_year, @running_no, @formatted, now())
RETURNING id, fiscal_year, running_no, formatted, allocated_at;
```

### Complete Allocator: `internal/receiptno/allocator.go` (skeleton)

```go
// Package receiptno implements the gap-less per-fiscal-year receipt number allocator.
// This is the single code path that may hand out a receipt number (D-35).
package receiptno

import (
    "context"
    "errors"
    "fmt"
    "time"

    "github.com/jackc/pgx/v5"

    "github.com/donnarec/donnarec-api/internal/db"
)

// AllocatedReceipt holds the result of a successful allocation.
// All three fields are stored in the ledger (D-34, D-42).
type AllocatedReceipt struct {
    FiscalYear  int       // e.g. 2569
    RunningNo   int       // e.g. 123
    Formatted   string    // e.g. "2569/000123" — frozen snapshot (D-42)
    AllocatedAt time.Time
}

// Allocator allocates gap-less receipt numbers.
type Allocator struct {
    queries db.Querier // interface (emit_interface: true in sqlc.yaml)
}

// NewAllocator creates an Allocator backed by the given Querier.
func NewAllocator(queries db.Querier) *Allocator {
    return &Allocator{queries: queries}
}

// Allocate allocates the next gap-less receipt number for the fiscal year
// of issueDate, within the given transaction.
//
// Contract (D-33/D-35/D-36):
//   - tx is a pgx.Tx opened by the caller; Allocate does NOT commit or rollback.
//   - On any error, the caller's tx rollback will undo the counter increment
//     and ledger insert — no gap is created.
//   - Allocate never retries internally; errors bubble to the caller.
//   - Allocate never calls time.Now() — issueDate is the caller's approval timestamp.
func (a *Allocator) Allocate(ctx context.Context, tx pgx.Tx, issueDate time.Time) (AllocatedReceipt, error) {
    fy := fiscalYear(issueDate)
    qtx := a.queries.WithTx(tx)

    // Step 1: Lock counter row (FOR UPDATE serializes concurrent allocations)
    _, lockErr := qtx.LockCounterForUpdate(ctx, int32(fy))
    if lockErr != nil {
        if !errors.Is(lockErr, pgx.ErrNoRows) {
            return AllocatedReceipt{}, fmt.Errorf("lock counter row: %w", lockErr)
        }
        // No counter row yet → first allocation of this fiscal year
        // Insert row safely (ON CONFLICT DO NOTHING handles concurrent init)
        if err := qtx.InitCounterRow(ctx, int32(fy)); err != nil {
            return AllocatedReceipt{}, fmt.Errorf("init counter row: %w", err)
        }
        // Lock again — now the row exists; concurrent sessions will block here
        if _, err := qtx.LockCounterForUpdate(ctx, int32(fy)); err != nil {
            return AllocatedReceipt{}, fmt.Errorf("lock counter row (after init): %w", err)
        }
    }

    // Step 2: Increment (safe — holding FOR UPDATE lock)
    nextNo, err := qtx.IncrementCounter(ctx, int32(fy))
    if err != nil {
        return AllocatedReceipt{}, fmt.Errorf("increment counter: %w", err)
    }

    // Step 3: Read format config (within same tx — D-32)
    cfg, err := qtx.GetReceiptNumberConfig(ctx)
    if err != nil {
        return AllocatedReceipt{}, fmt.Errorf("get receipt number config: %w", err)
    }

    // Step 4: Render formatted snapshot (D-42)
    formatted := formatReceiptNo(fy, int(nextNo), cfg)

    // Step 5: Insert ledger (UNIQUE backstop — D-37)
    ledger, err := qtx.InsertReceiptNumberLedger(ctx, db.InsertReceiptNumberLedgerParams{
        FiscalYear: int32(fy),
        RunningNo:  int32(nextNo),
        Formatted:  formatted,
    })
    if err != nil {
        return AllocatedReceipt{}, fmt.Errorf("insert receipt number ledger: %w", err)
    }

    return AllocatedReceipt{
        FiscalYear:  int(ledger.FiscalYear),
        RunningNo:   int(ledger.RunningNo),
        Formatted:   ledger.Formatted,
        AllocatedAt: ledger.AllocatedAt.Time,
    }, nil
}
```

---

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| PostgreSQL SEQUENCE | Counter table + `SELECT FOR UPDATE` | always — for gap-less requirement | SEQUENCE ไม่ rollback-safe; counter table rollback-safe |
| ORM-managed upsert | Raw SQL ใน sqlc .sql file | Phase 1 decision (D-23) | คุม `FOR UPDATE` เองได้เต็มที่ |
| Global sequence | Per-fiscal-year keyed counter | Phase 2 decision (D-41) | Auto-reset ปีใหม่โดยไม่ต้องมี scheduled job |
| Format at read time | Freeze formatted snapshot at allocate | D-42 | Immutable ต่อ audit/ภาษี แม้ config เปลี่ยน |

**Deprecated/outdated:**
- `nextval()` / `SERIAL` สำหรับเลขใบเสร็จ: ผิด gap-less requirement โดยออกแบบ [VERIFIED: postgresql.org docs sequences]
- "read MAX + 1" pattern: race condition → duplicate เลข [VERIFIED: CYBERTEC blog]

---

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | `BOOLEAN PRIMARY KEY DEFAULT true + CHECK (id=true)` เป็น valid PostgreSQL idiom สำหรับ single-row table | Research Q4 Schema | ถ้าผิด: ใช้ `id SERIAL PRIMARY KEY` + application-level enforcement แทน — functional เหมือนกัน |
| A2 | sqlc ใน version 1.31.1 handle `ON CONFLICT (fiscal_year) DO NOTHING` (:exec) ได้ถูกต้องโดยไม่ generate params ซ้ำ | Research Q1 Path A | ถ้าผิด: เขียน raw `tx.Exec()` แทน sqlc สำหรับ InitCounterRow |
| A3 | `import _ "time/tzdata"` embed tzdata ใน binary (Go 1.15+) ทำงานบน Go 1.25.1 | Pitfall 5 | ถ้าผิด: Dockerfile ต้อง install tzdata แทน — trivial fix |
| A4 | sqlc `GetReceiptNumberConfig :one LIMIT 1` handle single-row table ได้ถูกต้อง | Research Q4 | ถ้าผิด: ใช้ raw `tx.QueryRow` แทน |
| A5 | `ON CONFLICT DO NOTHING` ภายใน tx เดียวกับ `SELECT FOR UPDATE` (InitCounterRow + LockCounterForUpdate sequence) ไม่มี deadlock | Research Q1 concurrent first-allocation | ถ้าผิด: ต้องใช้ advisory lock (`pg_try_advisory_xact_lock`) สำหรับ init path |

---

## Open Questions

1. **`InitCounterRow` inside same tx vs outside**
   - What we know: INSERT + SELECT FOR UPDATE ในภายใน tx เดียวกันปลอดภัยถ้า row ไม่มีก่อน
   - What's unclear: ถ้า concurrent sessions ทั้งสองทำ InitCounterRow (ON CONFLICT DO NOTHING) ใน tx เดียวกัน session ที่แพ้จะ block ที่ `SELECT FOR UPDATE` และได้ค่าที่ถูกต้องหลัง winner commit — ต้องพิสูจน์ด้วย test
   - Recommendation: ทดสอบ concurrent first-allocation ใน `allocator_concurrency_test.go` เป็น separate test case

2. **`GetReceiptNumberConfig` read ใน tx — isolation level**
   - What we know: default isolation level = READ COMMITTED → การอ่าน config ใน tx เดียวกับ allocate เห็น committed data ณ เวลานั้น
   - What's unclear: ถ้า admin แก้ config ขณะมี in-flight allocations ใน READ COMMITTED config ที่แต่ละ tx เห็นอาจต่างกัน (แต่ยังถูกต้องในตัวเอง)
   - Recommendation: ยอมรับได้ — แต่ละใบเสร็จ freeze formatted ตาม config ที่เห็น ณ เวลา allocate (D-42 ต้องการแค่ freeze ที่ allocate time ไม่ใช่ consistent across allocations)

---

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| PostgreSQL 17 (via testcontainers) | Concurrency integration test | ✓ | 17 (pulled by testcontainers) | — |
| Docker (for testcontainers) | Integration tests | ✓ | Available (Phase 1 tests passed) | — |
| Go 1.25.1 | All backend code | ✓ | 1.25.1 | — |
| IANA tzdata | `fiscalYear()` helper | ✓ via `time/tzdata` embed | stdlib | `apk add tzdata` |
| sqlc CLI | Codegen after adding receiptno.sql | [ASSUMED] installed | 1.31.1 | `go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1` |

**Missing dependencies with no fallback:** none

**Missing dependencies with fallback:**
- sqlc CLI: ถ้าไม่ install ก็เขียน query function ด้วยมือ (raw pgx) แต่แนะนำให้ใช้ sqlc ตาม pattern Phase 1

---

## Validation Architecture

> `workflow.nyquist_validation` ไม่ได้ set เป็น false — enabled

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go `testing` stdlib + `stretchr/testify` v1.11.1 |
| Config file | ไม่มี config file แยก — ใช้ `go test` ตรงๆ |
| Quick run command | `cd donnarec-api && go test ./internal/receiptno/... -v -count=1` |
| Full suite command | `cd donnarec-api && go test ./... -race -v -count=1` |
| Integration test flag | `-tags integration` (optional) หรือ detect testcontainers runtime |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| FR-15 | Format `2569/000123` จาก fiscal_year + running_no + config | Unit | `go test ./internal/receiptno/... -run TestFormatReceiptNo -v` | ❌ Wave 0 |
| FR-15 | Config seed ใน DB มีค่า default ถูกต้อง (sep=/, pad=6, BE4, prefix='') | Integration | `go test ./internal/receiptno/... -run TestAllocator_DefaultConfig` | ❌ Wave 0 |
| FR-16 | N parallel allocations → zero duplicates ใน ledger | Integration (testcontainers) | `go test ./internal/receiptno/... -run TestAllocator_Concurrency -race` | ❌ Wave 0 |
| FR-16 | UNIQUE(fiscal_year, running_no) ยิง error เมื่อ insert ซ้ำ | Integration | `go test ./internal/receiptno/... -run TestAllocator_UniqueConstraintBackstop` | ❌ Wave 0 |
| FR-17 | Allocate ปีที่ไม่มี counter row → running_no เริ่มต้นที่ 1 | Integration | `go test ./internal/receiptno/... -run TestAllocator_NewFiscalYear` | ❌ Wave 0 |
| FR-17 | ปีงบต่างกัน → แต่ละปีมี counter แยก (ปี 2569 running_no=5, ปี 2570 running_no=1) | Integration | `go test ./internal/receiptno/... -run TestAllocator_MultiYear` | ❌ Wave 0 |
| FR-18 | `fiscalYear()` boundary: Sep 30 23:59 → 2568, Oct 1 00:00 → 2569 | Unit | `go test ./internal/receiptno/... -run TestFiscalYear_Boundaries` | ❌ Wave 0 |
| FR-18 | UTC input normalized ถูกต้อง (UTC Sep 30 17:00 = BKK Oct 1 00:00 → 2569) | Unit | `go test ./internal/receiptno/... -run TestFiscalYear_Boundaries` | ❌ Wave 0 |
| NFR-04 | Rollback → counter กลับค่าเดิม → ไม่เกิด gap (ตรวจจาก ledger) | Integration (testcontainers) | `go test ./internal/receiptno/... -run TestAllocator_Rollback -race` | ❌ Wave 0 |
| NFR-04 | Concurrent first-allocation ปีใหม่ → ไม่ duplicate / ไม่ panic | Integration | `go test ./internal/receiptno/... -run TestAllocator_ConcurrentNewYear -race` | ❌ Wave 0 |

### Sampling Rate

- **Per task commit:** `cd donnarec-api && go test ./internal/receiptno/... -v -count=1`
- **Per wave merge:** `cd donnarec-api && go test ./... -race -v -count=1`
- **Phase gate:** Full suite green (รวม -race) ก่อน `/gsd:verify-work`

### Wave 0 Gaps

- [ ] `internal/receiptno/allocator.go` — Allocator implementation
- [ ] `internal/receiptno/fiscal_year.go` — fiscalYear() helper
- [ ] `internal/receiptno/format.go` — formatReceiptNo() helper
- [ ] `internal/receiptno/allocator_test.go` — unit tests (fiscalYear boundaries, format)
- [ ] `internal/receiptno/allocator_concurrency_test.go` — integration concurrency harness
- [ ] `internal/db/queries/receiptno.sql` — sqlc queries for counter/ledger/config
- [ ] `migrations/000004_receipt_number_tables.up.sql` — 3 new tables
- [ ] `migrations/000004_receipt_number_tables.down.sql` — rollback migration
- [ ] Re-run `sqlc generate` หลังเพิ่ม receiptno.sql

---

## Security Domain

> `security_enforcement` ไม่ได้ set เป็น false — enabled

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | Phase 2 เป็น backend service layer — ไม่มี new auth endpoint |
| V3 Session Management | no | ไม่มี new session state ใน Phase 2 |
| V4 Access Control | yes (partial) | `receipt_numbers` ledger: `REVOKE UPDATE, DELETE FROM donnarec_app` — immutability enforcement |
| V5 Input Validation | yes | `issueDate` ต้องเป็น valid time.Time (Go type system); `running_no_padding CHECK >= 1` ใน DB |
| V6 Cryptography | no | ไม่มี crypto ใหม่ใน Phase 2 (PII encrypt อยู่ Phase 3) |

### Known Threat Patterns for Go + PostgreSQL Counter

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Duplicate receipt number (concurrent insert) | Tampering | `UNIQUE(fiscal_year, running_no)` DB constraint + `SELECT FOR UPDATE` serialization |
| Gap creation via rollback after allocate-in-wrong-tx | Tampering | Caller-managed tx (D-33): allocate + issue อยู่ใน tx เดียว → rollback undo counter |
| Manual UPDATE/DELETE on ledger | Tampering | `REVOKE UPDATE, DELETE ON receipt_numbers FROM donnarec_app` |
| Config injection (separator/prefix) | Tampering | Config อ่านจาก DB ที่ต้องผ่าน RBAC (Phase 4 UI); ใน Phase 2 เป็น seed value จาก migration |
| Clock skew → wrong fiscal year | Tampering | caller ส่ง issueDate (วันอนุมัติจาก DB) ไม่ใช่ wall clock ของ client |

---

## Sources

### Primary (HIGH confidence)
- [PostgreSQL Official Docs: Explicit Locking](https://www.postgresql.org/docs/current/explicit-locking.html) — `SELECT FOR UPDATE` lock duration, concurrent queue behavior
- [PostgreSQL Official Docs: INSERT / ON CONFLICT](https://www.postgresql.org/docs/current/sql-insert.html) — `ON CONFLICT DO UPDATE` atomicity, RETURNING behavior, deadlock risk with multiple unique indexes
- [PostgreSQL Official Docs: CREATE SEQUENCE](https://www.postgresql.org/docs/current/sql-createsequence.html) — ยืนยันว่า nextval ไม่ rollback = ไม่ gap-less
- Phase 1 codebase (`donnarec-api/`) — verified sqlc.yaml config (emit_interface, pgx/v5), `db.WithTx` pattern, `GetLastAuditRowForUpdate` FOR UPDATE example, testcontainers fixture
- go.mod + go.sum — verified versions: pgx v5.10.0, testcontainers v0.43.0, golang.org/x/sync v0.21.0

### Secondary (MEDIUM confidence)
- [sqlc Docs: Using Go and pgx](https://docs.sqlc.dev/en/latest/guides/using-go-and-pgx.html) — sqlc + pgx/v5 transaction pattern
- [sqlc Docs: Transactions](https://docs.sqlc.dev/en/latest/howto/transactions.html) — WithTx pattern
- [No-gap sequence in PostgreSQL (YugaByte Dev)](https://dev.to/yugabyte/no-gap-sequence-in-postgresql-and-yugabytedb-3feo) — counter table with `ON CONFLICT DO UPDATE` pattern (alternative path)
- [golang.org/x/sync/errgroup](https://pkg.go.dev/golang.org/x/sync/errgroup) — errgroup for concurrency test harness
- [TimeZone Converter: Asia/Bangkok](https://www.timezoneconverter.com/cgi-bin/zoneinfo?tz=Asia%2FBangkok) — ยืนยัน Thailand ไม่มี DST

### Tertiary (LOW confidence / ASSUMED)
- Thailand fiscal year Oct 1 start — [WebSearch confirmed](https://dataservices.mof.go.th/menu4?id=3&lang=en) government fiscal year structure; hospital fiscal year assumed same
- `BOOLEAN PRIMARY KEY DEFAULT true + CHECK` single-row idiom — common PostgreSQL pattern, not from official docs

---

## Metadata

**Confidence breakdown:**
- Standard Stack: HIGH — ทั้งหมดเป็น Phase 1 deps ที่ verified แล้ว
- Lock mechanism (Path A): HIGH — verified จาก PostgreSQL official docs + Phase 1 FOR UPDATE example (audit.sql)
- Architecture: HIGH — derived จาก Phase 1 patterns + D-33/D-37/D-39 locked decisions
- fiscalYear() implementation: HIGH — Thailand fiscal year + BE calendar well-documented; NO DST verified
- Concurrency test harness: MEDIUM — pattern standard แต่ first-year concurrent race ต้องพิสูจน์ด้วย test จริง
- Pitfalls: HIGH — ส่วนใหญ่ verified จาก official PostgreSQL docs

**Research date:** 2026-06-25
**Valid until:** 2026-09-25 (stable PostgreSQL + Go APIs — 3 months)
