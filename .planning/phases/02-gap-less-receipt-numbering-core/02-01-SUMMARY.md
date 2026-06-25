---
phase: 02-gap-less-receipt-numbering-core
plan: "01"
subsystem: db-schema
tags: [migration, sqlc, gap-less, receipt-number, schema]
dependency_graph:
  requires: []
  provides:
    - receipt_number_config table (configurable format, seeded D-28 defaults)
    - receipt_number_counters table (per-fiscal-year counter, SELECT FOR UPDATE path)
    - receipt_numbers ledger table (append-only, UNIQUE backstop)
    - sqlc Querier methods: LockCounterForUpdate, InitCounterRow, IncrementCounter, GetReceiptNumberConfig, InsertReceiptNumberLedger
  affects:
    - donnarec-api/internal/db/generated/ (regenerated)
    - Phase 3 (will call Allocate using these sqlc methods)
    - Phase 4 (UI edits receipt_number_config row)
tech_stack:
  added: []
  patterns:
    - counter table + SELECT FOR UPDATE (Path A, not SEQUENCE/SERIAL)
    - BOOLEAN PRIMARY KEY DEFAULT true + CHECK single_row constraint
    - REVOKE UPDATE, DELETE for immutable ledger at DB level
    - ON CONFLICT (fiscal_year) DO NOTHING for safe concurrent first-year init
    - sqlc :exec for InitCounterRow, :one RETURNING for all read/write queries
key_files:
  created:
    - donnarec-api/migrations/000004_receipt_number_tables.up.sql
    - donnarec-api/migrations/000004_receipt_number_tables.down.sql
    - donnarec-api/internal/db/queries/receiptno.sql
    - donnarec-api/internal/db/generated/receiptno.sql.go
  modified:
    - donnarec-api/internal/db/generated/querier.go (5 new Querier methods)
    - donnarec-api/internal/db/generated/models.go (ReceiptNumber, ReceiptNumberConfig, ReceiptNumberCounter structs)
    - donnarec-api/internal/db/generated/audit.sql.go (regenerated, no logic change)
    - donnarec-api/internal/db/generated/users.sql.go (regenerated, no logic change)
decisions:
  - key: counter-table-not-sequence
    summary: "ใช้ counter table + SELECT FOR UPDATE แทน PostgreSQL SEQUENCE — rollback-safe, gap-less (D-39, CLAUDE.md)"
  - key: boolean-pk-single-row
    summary: "BOOLEAN PRIMARY KEY DEFAULT true + CHECK (id = true) enforce single-row config โดยไม่ต้องใช้ application logic"
  - key: revoke-immutable-ledger
    summary: "REVOKE UPDATE, DELETE ON receipt_numbers FROM donnarec_app บังคับ immutable ledger ที่ DB level (T-02-01)"
  - key: initcounterrow-exec-not-one
    summary: "InitCounterRow ใช้ :exec (ไม่ใช่ :one) ตาม RESEARCH.md final SQL — ON CONFLICT DO NOTHING ไม่คืนแถวเสมอ"
metrics:
  duration: "4m 22s"
  completed: "2026-06-25"
  tasks_completed: 2
  tasks_total: 2
  files_created: 4
  files_modified: 4
---

# Phase 02 Plan 01: DB Schema + sqlc Queries for Gap-less Receipt Number Allocator Summary

**One-liner:** Migration 000004 creates 3 tables (config/counter/ledger) with UNIQUE backstop + REVOKE immutable ledger, and sqlc generates 5 typed Go methods (LockCounterForUpdate, InitCounterRow, IncrementCounter, GetReceiptNumberConfig, InsertReceiptNumberLedger) for the Path-A counter allocation pattern.

## What Was Built

### Task 1: Migration 000004 — config, counter, ledger tables
สร้าง 3 ตารางรากฐานของ allocator:

**receipt_number_config** — single-row config (BOOLEAN PK + CHECK enforce ไม่เกิน 1 แถว)
- default: separator=`/`, running_no_padding=6, year_format=`BE4`, prefix=``
- seeded ด้วย `INSERT INTO receipt_number_config DEFAULT VALUES` (idempotent)
- Phase 4 UI จะแก้ค่าในแถวนี้โดยไม่ต้อง deploy

**receipt_number_counters** — counter table (1 row ต่อปีงบ)
- PRIMARY KEY (fiscal_year) — auto-reset ปีใหม่โดย row ใหม่เกิดตอน first allocation (D-41)
- ไม่ใช้ SERIAL/SEQUENCE — counter controlled ด้วย SELECT FOR UPDATE + UPDATE RETURNING

**receipt_numbers** — ledger ที่ append-only
- BIGSERIAL id เป็น surrogate PK (สำหรับ FK จาก Phase 3) ไม่ใช่เลขใบเสร็จ
- running_no เป็น plain INT, CHECK (running_no >= 1)
- UNIQUE (fiscal_year, running_no) เป็น DB-level backstop ป้องกัน logic bug (D-37)
- REVOKE UPDATE, DELETE FROM donnarec_app — immutable ที่ DB level (T-02-01)
- formatted TEXT NOT NULL — frozen snapshot ตอน allocate (D-42)

### Task 2: sqlc queries receiptno.sql + regenerate
5 queries ด้วย Path A (SELECT FOR UPDATE + UPDATE RETURNING):

| Query | Type | Purpose |
|-------|------|---------|
| LockCounterForUpdate | :one | SELECT FOR UPDATE — serialize concurrent allocations |
| InitCounterRow | :exec | INSERT ON CONFLICT DO NOTHING — safe first-year init (Pitfall 1) |
| IncrementCounter | :one | UPDATE RETURNING — increment while holding lock |
| GetReceiptNumberConfig | :one | Read format config in same tx (D-32) |
| InsertReceiptNumberLedger | :one | INSERT + RETURNING — UNIQUE backstop fires here (D-37) |

sqlc v1.31.1 generate สำเร็จ: ReceiptNumber/ReceiptNumberConfig/ReceiptNumberCounter structs ใน models.go, 5 methods ใน querier.go interface, receiptno.sql.go.

`go build ./...` exits 0.

## Commits

| Task | Commit | Description |
|------|--------|-------------|
| Task 1 | e44b87b | feat(02-01): add migration 000004 for receipt number tables |
| Task 2 | 841bc6a | feat(02-01): add receiptno.sql queries and regenerate sqlc |

## Deviations from Plan

None - plan executed exactly as written.

The only minor deviation from the RESEARCH.md example SQL was:
- InitCounterRow uses `:exec` (not `:one RETURNING`) matching the final SQL in RESEARCH.md "Complete SQL File" section — the earlier example in Research Q1 showed `:one RETURNING` but the definitive query file section used `:exec` which is correct because `ON CONFLICT DO NOTHING` does not always return rows.

This is consistent with the PLAN.md spec (`InitCounterRow :exec`) and the final RESEARCH.md SQL file. No impact on allocator correctness.

## Known Stubs

None — this plan creates schema and sqlc query layer only. No UI components, no hardcoded values that flow to rendering. The `receipt_number_config` DEFAULT VALUES seed is intentional and correct (D-28 defaults).

## Threat Flags

None — all security measures align with the plan's threat_model:
- T-02-01 mitigated: REVOKE UPDATE, DELETE ON receipt_numbers FROM donnarec_app (Task 1)
- T-02-02 mitigated: UNIQUE (fiscal_year, running_no) constraint (Task 1)
- T-02-03 mitigated: sqlc named params only, no string concatenation (Task 2)
- T-02-04 mitigated: running_no is plain INT, not SERIAL/BIGSERIAL/nextval (Task 1)

## Self-Check: PASSED

Files exist:
- [x] donnarec-api/migrations/000004_receipt_number_tables.up.sql
- [x] donnarec-api/migrations/000004_receipt_number_tables.down.sql
- [x] donnarec-api/internal/db/queries/receiptno.sql
- [x] donnarec-api/internal/db/generated/receiptno.sql.go
- [x] donnarec-api/internal/db/generated/querier.go (updated with 5 new methods)
- [x] donnarec-api/internal/db/generated/models.go (3 new structs)

Commits exist:
- [x] e44b87b — feat(02-01): add migration 000004 for receipt number tables
- [x] 841bc6a — feat(02-01): add receiptno.sql queries and regenerate sqlc

Build: go build ./... exits 0
