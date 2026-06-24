---
phase: 01-foundation-db-auth-rbac-audit-retention-model
plan: "02"
subsystem: audit-log-hash-chain
tags: [go, postgresql, audit, hash-chain, immutability, tdd, concurrency]
dependency_graph:
  requires:
    - "01-01: WithTx helper, db.Queries, KeycloakClaims, testutil.SetupTestPostgres"
  provides:
    - "migrations/000002_audit_log: append-only hash-chained audit_log table"
    - "donnarec_app role: SELECT+INSERT only on audit_log (REVOKE UPDATE/DELETE)"
    - "SetupTestPostgresAsAppRole: testutil function for REVOKE integration testing"
    - "internal/audit/service.go: AuditEntry, AuditService, computeRowHash, AppendAuditEntryTx, VerifyChain"
    - "internal/audit/middleware.go: AuditMiddleware(svc), isPIIRevealEndpoint, deriveAction"
    - "internal/db/generated/audit.sql.go: InsertAuditLog, GetLastAuditRowForUpdate, ListAuditLogs, ListAllAuditForVerify"
  affects:
    - "Plan 01-03: SetupTestPostgresAsAppRole now available in testutil"
    - "Plan 01-04: handlers should call AppendAuditEntryTx in their transaction"
    - "Phase 3+: all mutations auto-audited via AuditMiddleware; PII reveal via /reveal suffix"
tech_stack:
  added:
    - "crypto/sha256 + encoding/hex: SHA-256 hash-chain computation (stdlib)"
    - "pg_advisory_xact_lock: serialization primitive for concurrent audit appends"
    - "nextval('audit_log_id_seq'): pre-reserve id for no-UPDATE hash strategy"
    - "donnarec_app LOGIN role: restricted DB role with REVOKE UPDATE/DELETE on audit_log"
  patterns:
    - "Advisory lock pattern: pg_advisory_xact_lock before GetLastAuditRowForUpdate"
    - "No-UPDATE hash: nextval + NOW() in tx → compute hash → INSERT with pre-computed hash"
    - "PII-reveal endpoint detection: isPIIRevealEndpoint via /reveal suffix"
    - "SetupTestPostgresAsAppRole: dual-pool pattern (superuser seed + restricted app test)"
key_files:
  created:
    - donnarec-api/migrations/000002_audit_log.up.sql
    - donnarec-api/migrations/000002_audit_log.down.sql
    - donnarec-api/internal/db/queries/audit.sql
    - donnarec-api/internal/db/generated/audit.sql.go
    - donnarec-api/internal/audit/service.go
    - donnarec-api/internal/audit/middleware.go
    - donnarec-api/internal/audit/immutability_test.go
    - donnarec-api/internal/audit/service_test.go
    - donnarec-api/internal/audit/concurrent_test.go
    - donnarec-api/internal/audit/middleware_test.go
  modified:
    - donnarec-api/internal/db/generated/models.go (AuditLog model added)
    - donnarec-api/internal/db/generated/querier.go (audit queries interface added)
    - donnarec-api/internal/testutil/postgres.go (SetupTestPostgresAsAppRole added)
    - donnarec-api/cmd/server/main.go (AuditService wired; AuditMiddleware registered)
decisions:
  - "D-17 realized: REVOKE UPDATE/DELETE on audit_log from donnarec_app + hash-chain"
  - "D-15 realized: AuditMiddleware covers all mutations + auth events (router.Use before RequireAuth)"
  - "D-13 mechanism realized: isPIIRevealEndpoint /reveal suffix → pii.reveal action"
  - "D-16 documented: Admin-only audit read enforced in service layer (not DB)"
  - "Advisory lock chosen over FOR UPDATE: FOR UPDATE on empty table provides no serialization"
  - "No-UPDATE hash strategy: nextval + NOW() pre-compute avoids REVOKE UPDATE conflict"
  - "donnarec_app as LOGIN role: allows testcontainer tests to connect as restricted identity"
metrics:
  duration_minutes: 45
  completed_date: "2026-06-24"
  tasks_completed: 3
  tasks_total: 3
  files_created: 10
  files_modified: 4
---

# Phase 01 Plan 02: Audit Log Hash-Chain Slice Summary

**One-liner:** SHA-256 hash-chained append-only audit_log with REVOKE UPDATE/DELETE at DB level, advisory-lock concurrent serialization, and generic Gin middleware covering all mutations plus PII-reveal GETs.

## Tasks Completed

| Task | Name | Commit | Result |
|------|------|--------|--------|
| 1 | audit_log migration + sqlc queries + immutability test | 13b781c | GREEN: REVOKE enforced, columns verified |
| 2 (RED) | hash-chain service tests (RED gate) | 9aead15 | RED: build fails — service not yet defined |
| 2 (GREEN) | hash-chain service implementation | eb265eb | GREEN: chain verify + 50-goroutine -race PASS |
| 3 (RED) | middleware tests (RED gate) | 85888a0 | RED: build fails — middleware not yet defined |
| 3 (GREEN) | audit middleware + router wiring | d36250d | GREEN: all middleware tests PASS |

## What Was Built

### Migration 000002 (audit_log)

- `audit_log` table: BIGSERIAL PK, actor_id/email, action, resource, before/after JSONB, ip_address INET, created_at TIMESTAMPTZ, prev_hash + row_hash TEXT NOT NULL
- `donnarec_app` LOGIN role: `GRANT SELECT, INSERT` then `REVOKE UPDATE, DELETE` (D-17)
- Indexes: `idx_audit_actor(actor_id, created_at DESC)`, `idx_audit_created(created_at DESC)`
- Full GRANT for other tables (users, user_roles, retention_config) so the app role can operate as sole API identity in production

### AuditService (internal/audit/service.go)

- `computeRowHash(id, actorID, action, resource, createdAt, prevHash)`: SHA-256 pipe-delimited, hex-encoded
- `AppendAuditEntryTx(ctx, tx, entry)`: advisory lock → read last row → nextval → NOW() → compute hash → INSERT (one statement, no UPDATE)
- `AppendAuditEntry(ctx, entry)`: own-tx wrapper via WithTx
- `VerifyChain(ctx)`: reads all rows in id-ASC order, recomputes each hash; returns (false, brokenID) on first mismatch

### AuditMiddleware (internal/audit/middleware.go)

- Skips non-reveal GETs; audits all mutations (POST/PUT/PATCH/DELETE)
- `isPIIRevealEndpoint`: route suffix `/reveal` → audits GET too (D-13 mechanism)
- `deriveAction`: HTTP method + route → dot-notation action (`user.create`, `pii.reveal`, etc.)
- Audit error logged via zap; request NOT aborted (compliance: user experience not degraded)
- Wired in main.go BEFORE RequireAuth (Pattern D — captures auth events too)

### Test Infrastructure Addition

- `SetupTestPostgresAsAppRole(t)`: returns (superPool, appPool) where appPool connects as restricted `donnarec_app` role — enables REVOKE integration testing

## Test Results (GREEN, -race)

```
TestAuditImmutability PASS (testcontainers)
  ✓ superuser INSERT succeeds
  ✓ donnarec_app UPDATE rejected: "permission denied" (T-1-audit-01, D-17)
  ✓ donnarec_app DELETE rejected: "permission denied"
  ✓ donnarec_app SELECT + INSERT still works

TestAuditRetainColumns PASS
  ✓ all required columns present (id, actor_id, actor_email, action, resource,
    before_json, after_json, ip_address, created_at, prev_hash, row_hash)
  ✓ prev_hash + row_hash NOT NULL

TestHashChainVerification PASS
  ✓ 5 sequential entries → VerifyChain true
  ✓ superuser tamper row 3 → VerifyChain (false, 3) ← tamper detected (T-1-audit-02)

TestConcurrentAuditInserts -race PASS
  ✓ 50 goroutines, no error
  ✓ exactly 50 rows
  ✓ zero duplicate prev_hash (T-1-audit-conc)
  ✓ VerifyChain true after concurrent inserts

TestAuditMiddlewareCoverage PASS
  ✓ POST writes 1 audit row
  ✓ PUT writes 1 audit row
  ✓ PATCH writes 1 audit row
  ✓ DELETE writes 1 audit row
  ✓ GET /api/items writes 0 rows (FR-13)

TestPIIRevealAudit PASS
  ✓ GET /api/donors/:id/reveal writes 1 row with action="pii.reveal" (D-13)

TestAuditMiddlewareNoAbortOnError PASS
  ✓ closed pool forces audit fail → handler still returns 201 (audit must not abort)
```

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] FOR UPDATE alone fails on empty table (concurrent chain corruption)**
- **Found during:** Task 2 TestConcurrentAuditInserts RED→GREEN
- **Issue:** When audit_log is empty, `SELECT ... FOR UPDATE` returns ErrNoRows (no lock acquired). Concurrent goroutines all see ErrNoRows simultaneously, use "GENESIS" as prev_hash, and insert multiple rows with duplicate prev_hash — chain integrity broken.
- **Fix:** Added `pg_advisory_xact_lock(auditChainLockKey)` BEFORE `GetLastAuditRowForUpdate`. Advisory locks are transaction-scoped and serialize even when the table is empty. The FOR UPDATE is still called but serves as a secondary guarantee once at least one row exists.
- **Files modified:** `internal/audit/service.go`
- **Commit:** eb265eb

**2. [Rule 2 - Critical functionality] donnarec_app as LOGIN role for REVOKE testing**
- **Found during:** Task 1 TestAuditImmutability — test ran as superuser (table owner), which bypasses REVOKE in PostgreSQL
- **Issue:** Table owner bypasses all GRANT/REVOKE — `REVOKE FROM test` and `REVOKE FROM PUBLIC` do not apply to the table owner. Without a non-owner connection, the REVOKE cannot be proven at DB level.
- **Fix:** Made `donnarec_app` a LOGIN role with password 'donnarec_app_test' (test-only). Added `SetupTestPostgresAsAppRole(t)` to testutil returning both superuser and restricted pools. Updated `TestAuditImmutability` to use the restricted pool for UPDATE/DELETE attempts.
- **Files modified:** `migrations/000002_audit_log.up.sql`, `internal/testutil/postgres.go`, `internal/audit/immutability_test.go`
- **Commit:** 13b781c

**3. [Rule 1 - Design] No-UPDATE hash strategy (REVOKE UPDATE conflict)**
- **Found during:** Task 2 — initial design used INSERT-then-UPDATE to set row_hash after getting id from RETURNING; this conflicts with `REVOKE UPDATE ON audit_log FROM donnarec_app`
- **Fix:** Pre-reserve sequence id via `SELECT nextval('audit_log_id_seq')`, capture `NOW()` within the transaction, compute SHA-256 hash, then INSERT with all fields including the pre-computed hash. No UPDATE is needed. The approach is atomic within the transaction and requires no permission beyond INSERT.
- **Files modified:** `internal/audit/service.go` (removed UpdateAuditRowHash query; removed from audit.sql.go)
- **Commit:** eb265eb

### Out-of-scope Discoveries (deferred)

None — all issues were directly caused by this plan's task changes.

## Known Stubs

| Stub | File | Location | Reason |
|------|------|----------|--------|
| `isPIIRevealEndpoint /reveal suffix` | `internal/audit/middleware.go` | `isPIIRevealEndpoint()` | Mechanism for D-13; actual donor reveal endpoint wired in Phase 3 |
| `deriveAction` heuristic noun extraction | `internal/audit/middleware.go` | `extractNoun()` | Simple singularize covers current routes; Phase 3+ may need domain-specific mapping |

These stubs do NOT prevent the plan's goal from being achieved. Audit immutability, hash-chain integrity, and middleware coverage are all operational.

## Threat Surface Scan

All mitigations from the threat register implemented:

| Threat ID | Mitigation Status |
|-----------|------------------|
| T-1-audit-01 | REVOKE UPDATE/DELETE from donnarec_app; TestAuditImmutability GREEN |
| T-1-audit-02 | SHA-256 hash-chain; VerifyChain returns (false, 3) on tamper; TestHashChainVerification GREEN |
| T-1-audit-conc | pg_advisory_xact_lock serializes concurrent appends; TestConcurrentAuditInserts -race GREEN |
| T-1-audit-03 | AuditMiddleware covers all mutations; audit written in AppendAuditEntry (WithTx, no goroutine) |
| T-1-audit-04 | isPIIRevealEndpoint flags /reveal GETs; TestPIIRevealAudit GREEN |
| T-1-audit-05 | before/after stored in DB only; zap error log never includes JSON payload |

No new threat surfaces beyond the plan's threat model.

## Self-Check: PASSED

Files exist:
- [x] donnarec-api/migrations/000002_audit_log.up.sql
- [x] donnarec-api/migrations/000002_audit_log.down.sql
- [x] donnarec-api/internal/db/queries/audit.sql
- [x] donnarec-api/internal/db/generated/audit.sql.go
- [x] donnarec-api/internal/audit/service.go
- [x] donnarec-api/internal/audit/middleware.go
- [x] donnarec-api/internal/audit/immutability_test.go
- [x] donnarec-api/internal/audit/service_test.go
- [x] donnarec-api/internal/audit/concurrent_test.go
- [x] donnarec-api/internal/audit/middleware_test.go

Commits exist:
- [x] 13b781c — feat(01-02): audit_log migration + sqlc queries + immutability test GREEN
- [x] 9aead15 — test(01-02): add failing hash-chain + concurrent audit tests (RED)
- [x] eb265eb — feat(01-02): hash-chain service + concurrency-safe append GREEN
- [x] 85888a0 — test(01-02): add failing audit middleware coverage tests (RED)
- [x] d36250d — feat(01-02): generic audit middleware + router wiring GREEN

Final verification:
- [x] `go build ./...` PASS
- [x] `go test ./internal/audit/... -race -count=1` PASS (all 7 tests)
