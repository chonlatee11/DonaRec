# Phase 2: Gap-less Receipt Numbering Core - Pattern Map

**Mapped:** 2026-06-25
**Files analyzed:** 8 (new/modified files for Phase 2)
**Analogs found:** 8 / 8

---

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|---|---|---|---|---|
| `migrations/000004_receipt_number_tables.up.sql` | migration | CRUD | `migrations/000002_audit_log.up.sql` | exact |
| `migrations/000004_receipt_number_tables.down.sql` | migration | CRUD | `migrations/000002_audit_log.down.sql` | exact |
| `internal/db/queries/receiptno.sql` | query | CRUD + row-lock | `internal/db/queries/audit.sql` | exact |
| `internal/receiptno/allocator.go` | service | CRUD + row-lock | `internal/audit/service.go` | exact |
| `internal/receiptno/fiscal_year.go` | utility | transform | `internal/pii/mask.go` (pure-fn pattern) | role-match |
| `internal/receiptno/format.go` | utility | transform | `internal/pii/mask.go` (pure-fn pattern) | role-match |
| `internal/receiptno/allocator_test.go` | test | unit | `internal/audit/service_test.go` | exact |
| `internal/receiptno/allocator_concurrency_test.go` | test | integration + concurrency | `internal/audit/concurrent_test.go` | exact |

---

## Pattern Assignments

### `migrations/000004_receipt_number_tables.up.sql` (migration, CRUD)

**Analog:** `donnarec-api/migrations/000002_audit_log.up.sql`

**File header convention** (lines 1-16):
```sql
-- migrations/000002_audit_log.up.sql
-- Append-only hash-chained audit log table (D-17, NFR-05, FR-13)
--
-- Design decisions realized here:
--   D-15: Audit scope covers all mutations + auth events via generic interceptor
--   D-17: Tamper-evidence via REVOKE UPDATE/DELETE + SHA-256 hash-chain per row
--   D-16: Admin-only read access (enforced in service/middleware, not DB)
--
-- The application connects as role 'donnarec_app'.
-- This migration:
--   1. Creates the audit_log table (append-only, hash-chained)
--   2. Creates the donnarec_app role if not exists
--   3. Grants SELECT + INSERT only; explicitly REVOKEs UPDATE + DELETE
--   4. Also applies REVOKE to the 'test' role (used in integration tests)
```
- เลียนแบบ comment header: ชื่อไฟล์ + decisions ที่ implement + bullet list ของสิ่งที่ migration ทำ

**Section separator convention** (lines 19-21):
```sql
-- ============================================================
-- 1. audit_log table (append-only, hash-chained)
-- ============================================================
```
- ทุก section ใช้ separator แบบนี้พร้อมหมายเลข

**Table definition convention** (lines 22-46):
```sql
CREATE TABLE audit_log (
    id          BIGSERIAL   PRIMARY KEY,
    actor_id    UUID        NOT NULL,
    ...
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    prev_hash   TEXT        NOT NULL,
    row_hash    TEXT        NOT NULL
);
```
- column aligned — type column เว้น tab เท่ากัน
- `TIMESTAMPTZ NOT NULL DEFAULT now()` คือ pattern สำหรับ timestamp columns
- CHECK constraint เขียนใน column definition โดยตรง

**GRANT/REVOKE convention** (lines 88-102):
```sql
GRANT SELECT, INSERT ON audit_log TO donnarec_app;
REVOKE UPDATE, DELETE ON audit_log FROM donnarec_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON users TO donnarec_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO donnarec_app;
```
- Receipt_numbers ต้องการ `REVOKE UPDATE, DELETE` เช่นเดียวกัน (immutable ledger)
- `GRANT USAGE, SELECT ON SEQUENCE receipt_numbers_id_seq TO donnarec_app` สำหรับ BIGSERIAL PK

**DO $$ block สำหรับ conditional role creation** (lines 66-82):
```sql
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'donnarec_app') THEN
        CREATE ROLE donnarec_app LOGIN PASSWORD 'donnarec_app_test';
    END IF;
END
$$;
```
- ใช้ pattern นี้สำหรับ idempotent role operations

---

### `migrations/000004_receipt_number_tables.down.sql` (migration, rollback)

**Analog:** `migrations/000002_audit_log.down.sql` (และ `migrations/000003_retention_triggers.down.sql`)

```sql
-- migrations/000003_retention_triggers.down.sql
DROP TABLE IF EXISTS receipt_numbers;
DROP TABLE IF EXISTS receipt_number_counters;
DROP TABLE IF EXISTS receipt_number_config;
```
- down.sql มีเฉพาะ `DROP ... IF EXISTS` ตามลำดับ FK dependency (ลบ child ก่อน parent)
- ไม่มี comment เพิ่มเติม — down.sql สั้นและตรงไปตรงมา

---

### `internal/db/queries/receiptno.sql` (query, CRUD + row-lock)

**Analog:** `donnarec-api/internal/db/queries/audit.sql`

**File header convention** (lines 1-4):
```sql
-- audit.sql — sqlc queries for the audit_log table
-- All queries use explicit column lists (no SELECT * in writes per Foundational Rule 4).
-- Parameterized queries only — no string concatenation (T-1-tamper-01).
```

**FOR UPDATE query pattern** (lines 12-21):
```sql
-- name: GetLastAuditRowForUpdate :one
-- Fetches the most recent audit row's id and row_hash, locking it with FOR UPDATE.
-- This serializes concurrent hash-chain appends: the next INSERT cannot proceed
-- until the current transaction releases this lock (Pitfall 2 mitigation, D-17).
-- Returns pgx.ErrNoRows if audit_log is empty (caller sets prevHash = "GENESIS").
SELECT id, row_hash
FROM audit_log
ORDER BY id DESC
LIMIT 1
FOR UPDATE;
```
- comment บน query อธิบายว่า lock ทำงานอย่างไร + pitfall ที่มัน mitigate
- `FOR UPDATE` เขียนต่อท้าย SELECT โดยตรง — sqlc 1.31.1 รองรับ

**Named param convention** (จาก `users.sql` lines 16-22):
```sql
INSERT INTO users (
    email,
    display_name,
    keycloak_subject,
    is_active,
    legal_hold
) VALUES (
    @email,
    @display_name,
    @keycloak_subject,
    true,
    false
) RETURNING *;
```
- params ใช้ `@param_name` รูปแบบ named params (ไม่ใช่ `$1`)
- `RETURNING *` สำหรับ INSERT ที่ต้องการ row กลับ
- explicit column list ในทุก INSERT (ไม่ใช้ `INSERT INTO table VALUES(...)`)

**ON CONFLICT DO NOTHING pattern** (จาก `users.sql` lines 45-49):
```sql
INSERT INTO user_roles (user_id, role)
VALUES (@user_id, @role)
ON CONFLICT (user_id, role) DO NOTHING
RETURNING user_id, role;
```
- pattern นี้ใช้สำหรับ `InitCounterRow` ของ Phase 2: `ON CONFLICT (fiscal_year) DO NOTHING`
- ใช้ `:exec` annotation (ไม่ต้องการ RETURNING) สำหรับ init path ที่ไม่ต้องการค่ากลับ

**UPDATE RETURNING pattern** (จาก `users.sql` lines 57-61):
```sql
UPDATE users
SET is_active = false, updated_at = now()
WHERE id = @id
RETURNING id, email, is_active, updated_at;
```
- `RETURNING` ใช้ใน UPDATE เพื่อรับค่าหลัง update — `IncrementCounter` ใช้ pattern นี้

---

### `internal/receiptno/allocator.go` (service, CRUD + row-lock)

**Analog:** `donnarec-api/internal/audit/service.go`

**Import block pattern** (lines 34-49):
```go
import (
    "context"
    "crypto/sha256"
    "encoding/hex"
    "fmt"
    "net"
    "net/netip"
    "time"

    dbhelpers "github.com/donnarec/donnarec-api/internal/db"
    db "github.com/donnarec/donnarec-api/internal/db/generated"
    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgtype"
    "github.com/jackc/pgx/v5/pgxpool"
    "go.uber.org/zap"
)
```
- stdlib imports ก่อน (กลุ่มแรก) แล้วตามด้วย internal imports แล้ว third-party
- internal db alias: `dbhelpers` สำหรับ `internal/db` และ `db` สำหรับ `internal/db/generated`

**Service struct + constructor pattern** (lines 82-100):
```go
type AuditService struct {
    pool    *pgxpool.Pool
    queries *db.Queries
    logger  *zap.Logger
}

func NewAuditService(pool *pgxpool.Pool, queries *db.Queries, logger *zap.Logger) *AuditService {
    if pool == nil {
        panic("audit.NewAuditService: pool must not be nil")
    }
    if queries == nil {
        panic("audit.NewAuditService: queries must not be nil")
    }
    if logger == nil {
        panic("audit.NewAuditService: logger must not be nil")
    }
    return &AuditService{pool: pool, queries: queries, logger: logger}
}
```
- nil guard ใน constructor พร้อม panic message `"pkg.NewService: field must not be nil"`
- allocator ใช้ `db.Querier` interface (ไม่ใช่ `*db.Queries`) เพราะ `emit_interface: true`

**FOR UPDATE + ErrNoRows handling pattern** (lines 144-157 ของ service.go):
```go
// Step 2: Read the last row's hash (now serialized by the advisory lock).
prevHash := genesisHash
lastRow, err := qtx.GetLastAuditRowForUpdate(ctx)
if err != nil && err != pgx.ErrNoRows {
    return fmt.Errorf("audit: lock last row: %w", err)
}
if err == nil {
    prevHash = lastRow.RowHash
}
```
- `errors.Is(err, pgx.ErrNoRows)` หรือ `err != pgx.ErrNoRows` เป็น pattern จัดการ empty table
- allocator ใช้ pattern เดียวกันสำหรับ "first allocation of new year"

**`WithTx` seam สำหรับ caller-managed tx** (lines 139 ของ service.go):
```go
func (s *AuditService) AppendAuditEntryTx(ctx context.Context, tx pgx.Tx, entry AuditEntry) error {
    qtx := s.queries.WithTx(tx)
    ...
}
```
- `qtx := s.queries.WithTx(tx)` — bind sqlc Queries ให้ทำงานใน tx ของ caller
- Phase 2 allocator ทำสิ่งเดียวกันทุกประการ: รับ `pgx.Tx` จาก caller แล้ว `qtx := a.queries.WithTx(tx)`

**Error wrapping convention** (ทั่วทั้ง service.go):
```go
return fmt.Errorf("audit: acquire chain lock: %w", err)
return fmt.Errorf("audit: lock last row: %w", err)
return fmt.Errorf("audit: reserve sequence id: %w", err)
```
- `fmt.Errorf("package: operation: %w", err)` — 2-part prefix: package name + operation
- allocator ใช้: `fmt.Errorf("lock counter row: %w", err)`, `fmt.Errorf("increment counter: %w", err)` ฯลฯ

**`AppendAuditEntry` (own-tx wrapper) pattern** (lines 238-242):
```go
func (s *AuditService) AppendAuditEntry(ctx context.Context, entry AuditEntry) error {
    return dbhelpers.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
        return s.AppendAuditEntryTx(ctx, tx, entry)
    })
}
```
- service มี 2 method: `...Tx` (caller-managed) และ ไม่มี Tx suffix (own-tx)
- allocator เปิดเผยเฉพาะ `Allocate(ctx, tx, issueDate)` — ไม่มี own-tx wrapper เพราะ D-33 ห้าม allocator คุม tx เอง

**`db.WithTx` usage pattern** (จาก `users/service.go` lines 79-124):
```go
err := dbhelpers.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
    qtx := s.queries.WithTx(tx)

    row, err := qtx.CreateUser(ctx, db.CreateUserParams{...})
    if err != nil {
        s.logger.Error("failed to create user",
            zap.String("operation", "CreateUser"),
            zap.Error(err),
        )
        return fmt.Errorf("create user: %w", err)
    }
    ...
    return nil
})
```
- `dbhelpers.WithTx` + closure + `qtx := s.queries.WithTx(tx)` ใน 3 บรรทัดแรกของ closure
- Phase 3 จะห่อ `alloc.Allocate(ctx, tx, issueDate)` ใน closure แบบนี้

---

### `internal/receiptno/fiscal_year.go` (utility, transform)

**Analog:** `donnarec-api/internal/pii/mask.go` (pure function pattern)

ไม่มี analog โดยตรงใน codebase แต่ pattern ของ pure utility function ที่ไม่มี side effect:

**Package declaration + doc comment**:
```go
// Package receiptno implements the gap-less per-fiscal-year receipt number allocator.
// This is the single code path that may hand out a receipt number (D-35).
package receiptno
```
- package `receiptno` — functions เล็กที่เป็น pure ให้ unexported (`fiscalYear` ไม่ใช่ `FiscalYear`)
- exported เฉพาะ `Allocator` struct และ `AllocatedReceipt` type, `NewAllocator()`

**Pure function signature** (ตาม RESEARCH.md pattern):
```go
// fiscalYear returns the Thai Buddhist Era fiscal year for the given timestamp.
// ... (godoc with examples)
func fiscalYear(issueDate time.Time) int {
    loc, err := time.LoadLocation("Asia/Bangkok")
    if err != nil {
        panic("Asia/Bangkok timezone not available: " + err.Error())
    }
    t := issueDate.In(loc)
    ...
}
```
- panic สำหรับ programming error (missing tzdata) ไม่ใช่ runtime error — pattern เดียวกับ constructor nil guard

---

### `internal/receiptno/format.go` (utility, transform)

**Analog:** `donnarec-api/internal/pii/mask.go` (pure function pattern)

**Pure format function** (ตาม RESEARCH.md):
```go
func formatReceiptNo(fiscalYear int, runningNo int, cfg db.GetReceiptNumberConfigRow) string {
    ...
    runningStr := fmt.Sprintf("%0*d", cfg.RunningNoPadding, runningNo)
    return cfg.Prefix + yearStr + cfg.Separator + runningStr
}
```
- unexported เหมือน `fiscalYear` — เรียกเฉพาะจาก `allocator.go`
- รับ `db.GetReceiptNumberConfigRow` (sqlc-generated type) โดยตรง — ไม่ define config struct ซ้ำ

---

### `internal/receiptno/allocator_test.go` (test, unit)

**Analog:** `donnarec-api/internal/audit/service_test.go`

**Test file package convention** (lines 1-2):
```go
// Package audit_test tests the AuditService hash-chain implementation.
package audit_test
```
- ใช้ `package receiptno_test` (black-box test) ไม่ใช่ `package receiptno`

**Import block ใน test** (lines 4-16):
```go
import (
    "context"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"

    "github.com/donnarec/donnarec-api/internal/audit"
    db "github.com/donnarec/donnarec-api/internal/db/generated"
    "github.com/donnarec/donnarec-api/internal/testutil"
    "go.uber.org/zap"
)
```
- `require` สำหรับ fatal assertions, `assert` สำหรับ non-fatal
- `testutil.SetupTestPostgres(t)` หรือ `SetupTestPostgresAsAppRole(t)` เป็น first call ใน test

**Short mode guard** (lines 23-25):
```go
if testing.Short() {
    t.Skip("skipping integration test in short mode")
}
```
- integration tests (ใช้ testcontainers) ต้องมี short mode guard

**Test setup pattern**:
```go
superPool, _ := testutil.SetupTestPostgresAsAppRole(t)
ctx := context.Background()
logger, _ := zap.NewDevelopment()

queries := db.New(superPool)
svc := audit.NewAuditService(superPool, queries, logger)
```
- `db.New(pool)` → `NewAllocator(queries)` เป็น pattern เดียวกัน
- `zap.NewDevelopment()` สำหรับ test logger

**Unit test (ไม่ใช้ testcontainers)** สำหรับ `TestFiscalYear_Boundaries` และ `TestFormatReceiptNo`:
```go
// ไม่มี testcontainers — เรียก function ตรงๆ
func TestFiscalYear_Boundaries(t *testing.T) {
    bkk, _ := time.LoadLocation("Asia/Bangkok")
    cases := []struct{
        name     string
        input    time.Time
        expected int
    }{ ... }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            require.Equal(t, tc.expected, fiscalYear(tc.input))
        })
    }
}
```
- table-driven test สำหรับ boundary cases — pattern มาตรฐาน Go

---

### `internal/receiptno/allocator_concurrency_test.go` (test, integration + concurrency)

**Analog:** `donnarec-api/internal/audit/concurrent_test.go`

**Concurrency test structure** (lines 23-108):
```go
func TestConcurrentAuditInserts(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test in short mode")
    }

    superPool, _ := testutil.SetupTestPostgresAsAppRole(t)
    ctx := context.Background()

    const goroutines = 50
    var wg sync.WaitGroup
    errs := make([]error, goroutines)

    for i := 0; i < goroutines; i++ {
        wg.Add(1)
        go func(idx int) {
            defer wg.Done()
            errs[idx] = svc.AppendAuditEntry(ctx, ...)
        }(i)
    }

    wg.Wait()

    for i, err := range errs {
        assert.NoError(t, err, "goroutine %d must not error", i)
    }
    ...
}
```
- Phase 2 ใช้ `sync.WaitGroup` + `errs []error` pattern เดียวกัน
- หรือสลับเป็น `errgroup.WithContext` (จาก RESEARCH.md) ที่ return error แรกทันที — ขึ้นกับ planner

**Inline SQL assertion pattern** (lines 62-79):
```go
var count int
err := superPool.QueryRow(ctx, `SELECT COUNT(*) FROM audit_log`).Scan(&count)
require.NoError(t, err)
assert.Equal(t, goroutines, count, "must have exactly 50 rows in audit_log")

var dupCount int
err = superPool.QueryRow(ctx, `
    SELECT COUNT(*) FROM (
        SELECT prev_hash, COUNT(*) c
        FROM audit_log
        GROUP BY prev_hash
        HAVING COUNT(*) > 1
    ) dups
`).Scan(&dupCount)
require.NoError(t, err)
assert.Equal(t, 0, dupCount, "no prev_hash must appear more than once ...")
```
- ใช้ raw SQL ผ่าน `pool.QueryRow` สำหรับ assertions ใน test (ไม่ต้องสร้าง query ผ่าน sqlc)
- Phase 2: ตรวจ `SELECT running_no FROM receipt_numbers WHERE fiscal_year = $1 ORDER BY running_no`

**Deliberate rollback test** — RESEARCH.md ระบุ pattern นี้เพิ่มเติม:
```go
// Phase 2 pattern (จาก RESEARCH.md):
// - ทุก i%rollbackEvery == 0: return errors.New("deliberate rollback")
// - assert: ledger count = (N - rollback_count), ไม่มี gap ใน ledger rows
// - assert: counter value หลัง rollback กลับค่าเดิม (SELECT last_running_no)
```

---

## Shared Patterns

### `db.WithTx` Seam
**Source:** `donnarec-api/internal/db/helpers.go` (lines 25-40)
**Apply to:** `allocator_concurrency_test.go` เมื่อเรียก `Allocate` ใน test

```go
func WithTx(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
    tx, err := pool.Begin(ctx)
    if err != nil {
        return fmt.Errorf("begin tx: %w", err)
    }
    defer tx.Rollback(ctx) //nolint:errcheck // rollback is a best-effort cleanup; commit error is returned

    if err := fn(tx); err != nil {
        return err
    }

    if err := tx.Commit(ctx); err != nil {
        return fmt.Errorf("commit tx: %w", err)
    }
    return nil
}
```
- `defer tx.Rollback(ctx)` เป็น no-op หลัง commit สำเร็จ — pgx ออกแบบมาแบบนี้
- Phase 2 test: `db.WithTx(ctx, pool, func(tx pgx.Tx) error { return alloc.Allocate(ctx, tx, issueDate) })`

### `queries.WithTx(tx)` Bind Pattern
**Source:** `donnarec-api/internal/db/generated/db.go` (line 28-32) + `audit/service.go` (line 139)
**Apply to:** `allocator.go`

```go
// generated/db.go
func (q *Queries) WithTx(tx pgx.Tx) *Queries {
    return &Queries{db: tx}
}

// usage in service (audit/service.go line 139):
qtx := s.queries.WithTx(tx)
```
- `qtx := a.queries.WithTx(tx)` — บรรทัดแรกใน `Allocate()` method

### FOR UPDATE + ErrNoRows Pattern
**Source:** `donnarec-api/internal/audit/service.go` (lines 149-157) + `internal/db/queries/audit.sql` (lines 12-21)
**Apply to:** `allocator.go` (LockCounterForUpdate step), `receiptno.sql` (LockCounterForUpdate query)

```go
// service.go pattern:
lastRow, err := qtx.GetLastAuditRowForUpdate(ctx)
if err != nil && err != pgx.ErrNoRows {
    return fmt.Errorf("audit: lock last row: %w", err)
}
if err == nil {
    prevHash = lastRow.RowHash
}

// SQL pattern (audit.sql):
SELECT id, row_hash
FROM audit_log
ORDER BY id DESC
LIMIT 1
FOR UPDATE;
```
- Phase 2 ใช้ `errors.Is(err, pgx.ErrNoRows)` (idiomatic) หรือ `err == pgx.ErrNoRows` (ตาม service.go)
- `LockCounterForUpdate` เป็น `:one` — return `pgx.ErrNoRows` เมื่อปีใหม่ยังไม่มี counter row

### Error Wrapping Convention
**Source:** `donnarec-api/internal/audit/service.go` (ทั่วทั้งไฟล์) + `users/service.go`
**Apply to:** `allocator.go`

```go
return fmt.Errorf("lock counter row: %w", err)
return fmt.Errorf("init counter row: %w", err)
return fmt.Errorf("lock counter row (after init): %w", err)
return fmt.Errorf("increment counter: %w", err)
return fmt.Errorf("get receipt number config: %w", err)
return fmt.Errorf("insert receipt number ledger: %w", err)
```
- pattern: `"operation description: %w"` — ไม่มี package prefix ใน allocator (เพราะ errors bubble ขึ้น caller)
- audit service ใช้ `"audit: operation: %w"` เพราะ caller ไม่รู้ว่ามาจาก package ไหน

### testcontainers Setup
**Source:** `donnarec-api/internal/testutil/postgres.go`
**Apply to:** `allocator_concurrency_test.go`, `allocator_test.go` (integration tests)

```go
// ใช้ superuser pool สำหรับ integration tests ปกติ
pool := testutil.SetupTestPostgres(t)

// ใช้ app-role pool สำหรับ permission tests (ตรวจ REVOKE)
superPool, appPool := testutil.SetupTestPostgresAsAppRole(t)
```
- migration path: `"file://../../migrations"` — relative จาก `internal/receiptno/` → `../../migrations/`
- ไม่ต้อง seed data เพราะ `INSERT INTO receipt_number_config DEFAULT VALUES` อยู่ใน migration

### sqlc.yaml — ไม่ต้องแก้ไข
**Source:** `donnarec-api/internal/db/sqlc.yaml`
**Apply to:** `receiptno.sql` (query file ใหม่ที่เพิ่มใน `./queries/` directory)

```yaml
version: "2"
sql:
  - engine: "postgresql"
    queries: "./queries"          # scan ทุก .sql ใน directory นี้
    schema: "../../migrations"    # schema จาก migration files
    gen:
      go:
        package: "db"
        out: "./generated"
        sql_package: "pgx/v5"
        emit_interface: true      # สร้าง Querier interface (ใช้ใน allocator)
        emit_json_tags: true
        emit_pointers_for_null_types: true
        emit_db_tags: true
```
- `emit_interface: true` → sqlc generate `Querier` interface ที่มี method ใหม่จาก `receiptno.sql` โดยอัตโนมัติ
- หลังเพิ่ม `receiptno.sql` ให้รัน `cd donnarec-api/internal/db && sqlc generate`

### Querier Interface Pattern
**Source:** `donnarec-api/internal/db/generated/querier.go` (lines 13-40)
**Apply to:** `allocator.go` (ใช้ `db.Querier` ไม่ใช่ `*db.Queries`)

```go
type Querier interface {
    AssignRole(ctx context.Context, arg AssignRoleParams) (UserRole, error)
    CreateUser(ctx context.Context, arg CreateUserParams) (User, error)
    GetLastAuditRowForUpdate(ctx context.Context) (GetLastAuditRowForUpdateRow, error)
    ...
}

var _ Querier = (*Queries)(nil) // compile-time interface check
```
- `Allocator.queries` field ต้องเป็น type `db.Querier` (interface) ไม่ใช่ `*db.Queries` (concrete)
- เหตุผล: ทดสอบได้ง่ายกว่า + conform กับ `emit_interface: true` pattern ของ project

---

## No Analog Found

ไม่มีไฟล์ใดใน Phase 2 ที่ไม่มี analog — ทุกไฟล์มี match ที่ชัดเจนใน Phase 1 codebase

---

## Key Observations for Planner

### 1. `Allocator.queries` ต้องเป็น `db.Querier` (interface)
audit service ใช้ `*db.Queries` (concrete) แต่ allocator ควรใช้ `db.Querier` (interface) เพราะ:
- `WithTx(tx)` ของ `*Queries` คืน `*Queries` ไม่ใช่ `Querier` — ต้องใช้ concrete type ใน step `qtx := a.queries.WithTx(tx)`
- ตรวจสอบ generated code หลัง `sqlc generate` ว่า `WithTx` return type เป็นอะไร ก่อนตัดสินใจ field type

### 2. Migration ลำดับ 000004 — section 4 (GRANT) ต้องครบ
ดู `000002_audit_log.up.sql` สำหรับ pattern GRANT ที่ครบถ้วน: `donnarec_app` role ถูกสร้างใน 000002 แล้ว — 000004 ต้องแค่ GRANT/REVOKE บน tables ใหม่

### 3. Migration path ใน testcontainers
`testutil/postgres.go` line 63: `"file://../../migrations"` — ถ้า test อยู่ใน `internal/receiptno/` path เดียวกันถูกต้อง

### 4. `golang.org/x/sync/errgroup` อาจต้องเพิ่มเป็น direct dep
`go.mod` line 98: `golang.org/x/sync v0.21.0 // indirect` — ถ้า concurrency test ใช้ `errgroup.WithContext` โดยตรงให้รัน `cd donnarec-api && go get golang.org/x/sync@v0.21.0`

### 5. `jackc/pgerrcode` มีอยู่ใน go.mod แล้ว
`go.mod` line 51: `github.com/jackc/pgerrcode v0.0.0-20220416144525-469b46aa5efa // indirect` — ใช้ `pgerrcode.UniqueViolation` ใน `TestAllocator_UniqueConstraintBackstop` ได้โดยตรง

---

## Metadata

**Analog search scope:** `donnarec-api/migrations/`, `donnarec-api/internal/db/`, `donnarec-api/internal/audit/`, `donnarec-api/internal/users/`, `donnarec-api/internal/testutil/`
**Files scanned:** 18 files
**Pattern extraction date:** 2026-06-25
