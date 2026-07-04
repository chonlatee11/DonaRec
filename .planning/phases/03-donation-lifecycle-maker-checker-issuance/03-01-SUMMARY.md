---
phase: 03-donation-lifecycle-maker-checker-issuance
plan: "01"
subsystem: backend/data-layer
tags: [migration, sqlc, schema, pii, sod, outbox, wave0, tdd]
dependency_graph:
  requires:
    - "02-04-SUMMARY.md (receipt_numbers ledger + FK target for donations.receipt_number_id)"
    - "01-01-SUMMARY.md (users table for created_by/approved_by/cancelled_by FKs)"
    - "01-02-SUMMARY.md (audit_log infrastructure — used by 03-05)"
  provides:
    - "donations table + donation_status enum (schema bedrock for all 03-xx plans)"
    - "outbox_jobs table (Phase 4 worker target)"
    - "sqlc-generated donation + outbox query types (LockDonationForUpdate, IssueDonation, etc.)"
    - "donation package sentinel errors (ErrInvalidTransition, ErrSoDViolation, etc.)"
    - "Wave 0 test contract (13 integration stubs + 4 RED unit stubs)"
  affects:
    - "03-03 (CreateDonation service — uses CreateDonation, UpdateDraftDonation)"
    - "03-05 (issuance tx — uses LockDonationForUpdate + IssueDonation + EnqueueOutboxJob)"
    - "03-06 (cancel service — uses CancelDonation + SetReplacedBy)"
    - "03-07 (list/search API — uses SearchDonations)"
tech_stack:
  added:
    - "donation_status PostgreSQL ENUM (5 values)"
    - "donations table (32 columns, UUID PK, per-donation donor snapshot)"
    - "outbox_jobs table (BIGSERIAL PK, JSONB payload, status CHECK)"
    - "internal/donation package (errors.go — sentinel errors, Wave 0 stubs)"
    - "internal/storage package (client.go — ErrUnsupportedFileType stub)"
  patterns:
    - "Pattern G: Migration Structure (enum → table → CHECK → index → GRANT/REVOKE)"
    - "Pattern F: sqlc Named Params (@param syntax, explicit column lists)"
    - "SELECT FOR UPDATE pattern (LockDonationForUpdate, D-52)"
    - "Nullable filter params via @param::TYPE IS NULL (SearchDonations, D-53)"
    - "Wave 0 TDD contract (t.Fatal RED stubs + t.Skip integration stubs)"
key_files:
  created:
    - donnarec-api/migrations/000005_donations.up.sql
    - donnarec-api/migrations/000005_donations.down.sql
    - donnarec-api/migrations/000007_outbox_jobs.up.sql
    - donnarec-api/migrations/000007_outbox_jobs.down.sql
    - donnarec-api/internal/db/queries/donations.sql
    - donnarec-api/internal/db/queries/outbox.sql
    - donnarec-api/internal/db/generated/donations.sql.go
    - donnarec-api/internal/db/generated/outbox.sql.go
    - donnarec-api/internal/donation/errors.go
    - donnarec-api/internal/donation/service_test.go
    - donnarec-api/internal/donation/service_integration_test.go
    - donnarec-api/internal/storage/client.go
    - donnarec-api/internal/storage/client_test.go
  modified:
    - donnarec-api/internal/db/generated/models.go
    - donnarec-api/internal/db/generated/querier.go
decisions:
  - "D-43 realized: no donor master table; donor fields are a per-donation snapshot on donations"
  - "D-44 realized: donor_tax_id_enc/dek BYTEA NOT NULL — ciphertext mandatory, plaintext never stored"
  - "D-47 realized: CancelDonation keeps receipt_number_id + chk_receipt_only_on_issued_or_cancelled enforces it"
  - "D-50 realized: replaces/replaced_by self-FKs on donations(id) for void & reissue"
  - "D-51 realized: edonation_keyed BOOLEAN NOT NULL DEFAULT false column present"
  - "D-52 realized: LockDonationForUpdate uses SELECT … FOR UPDATE (serializes concurrent approvals)"
  - "D-53 realized: SearchDonations @param::TYPE IS NULL nullable filter pattern"
  - "SoD DB backstop: chk_sod_approver CHECK (approved_by IS NULL OR approved_by != created_by)"
metrics:
  duration: "~15 minutes (including testcontainers run)"
  completed: "2026-06-30T12:06:44Z"
  tasks_completed: 3
  tasks_total: 3
  files_created: 13
  files_modified: 2
---

# Phase 03 Plan 01: Data-Layer Foundation (Migration + sqlc + Wave 0 Tests) Summary

**One-liner:** Donations schema bedrock with UUID PK, per-donation donor snapshot (AES-256-GCM PII columns), SoD DB CHECK backstop, gap-less receipt FK, transactional outbox table, 11 sqlc-generated query methods, and Wave 0 test contract (13 integration stubs + 4 RED unit stubs).

## What Was Built

### Task 1: Migrations 000005 (donations) + 000007 (outbox_jobs)

**`migrations/000005_donations.up.sql`**
- `CREATE TYPE donation_status AS ENUM ('draft','pending_review','issued','rejected','cancelled')` — 5 states (FR-11)
- `CREATE TABLE donations` with 32 columns covering: UUID PK, lifecycle timestamps, donor snapshot, PII BYTEA ciphertext, amount/date, consent fields (D-49), retention, submit/review/approval/receipt/cancellation/reissue columns
- `CONSTRAINT chk_sod_approver CHECK (approved_by IS NULL OR approved_by != created_by)` — DB SoD backstop (T-03-01, CLAUDE.md defense-in-depth)
- `CONSTRAINT chk_receipt_only_on_issued_or_cancelled` — ensures receipt_number_id IS NOT NULL IFF status IN ('issued','cancelled') (T-03-02, D-47)
- 6 indexes for FR-10 search: donor_name, donated_at, status, receipt_number_id (partial), created_by, approved_by (partial)
- `GRANT SELECT, INSERT, UPDATE ON donations TO donnarec_app` + `REVOKE DELETE` (FR-19, T-03-03)

**`migrations/000007_outbox_jobs.up.sql`**
- `CREATE TABLE outbox_jobs` with BIGSERIAL PK, job_type TEXT, JSONB payload, status CHECK ('pending','processing','done','failed'), timestamps, attempts, last_error
- Partial index `idx_outbox_jobs_pending WHERE status IN ('pending','failed')` for efficient Phase 4 worker polling
- `GRANT SELECT, INSERT, UPDATE` + `GRANT USAGE, SELECT ON SEQUENCE outbox_jobs_id_seq`

**Verification:** `TestMigrationsApplyAndRollback` PASSED — testcontainers Postgres 17 applied migrations 000001..000007, confirmed `donations` + `outbox_jobs` tables and both CHECK constraints exist.

### Task 2: sqlc Queries (donations.sql + outbox.sql) + Generate

**`internal/db/queries/donations.sql`** (11 named queries):
| Query | Type | Purpose |
|-------|------|---------|
| `LockDonationForUpdate` | `:one` | SELECT FOR UPDATE — D-52 double-issuance guard |
| `CreateDonation` | `:one` | INSERT donor snapshot + PII ciphertext, RETURNING id/status/timestamps |
| `GetDonationByID` | `:one` | Full read (32 cols including PII ciphertext for authorized reveal) |
| `UpdateDraftDonation` | `:exec` | Update Maker-editable fields WHERE status='draft' (FR-09) |
| `SubmitDonation` | `:exec` | draft → pending_review + submitted_at = now() |
| `ReturnDonation` | `:exec` | pending_review → draft + reviewed_by/at/reason (D-45) |
| `RejectDonation` | `:exec` | pending_review → rejected + reviewed_by/at/reason (D-45) |
| `IssueDonation` | `:exec` | pending_review → issued + receipt FK + formatted snapshot (D-38/D-42) |
| `CancelDonation` | `:exec` | issued → cancelled + cancel fields; receipt FK **retained** (FR-19, D-47) |
| `SetReplacedBy` | `:exec` | Links cancelled record to reissued successor (D-50) |
| `SearchDonations` | `:many` | Nullable @param::TYPE IS NULL filters for name/date/status/receipt (D-53) |

**`internal/db/queries/outbox.sql`** (1 query):
- `EnqueueOutboxJob :exec` — INSERT inside issuance tx, atomically linked to receipt issuance

**Verification:** `sqlc generate` exits 0; `go build ./...` exits 0; generated `querier.go` contains `LockDonationForUpdate(` and `EnqueueOutboxJob(`.

### Task 3: Wave 0 Test Scaffolds

**`internal/donation/errors.go`** — Package declaration + 7 sentinel errors:
`ErrInvalidTransition`, `ErrSoDViolation`, `ErrMissingReason`, `ErrNotFound`, `ErrForbidden`, `ErrDraftOnly`, `ErrEDonationKeyedCancel`

**`internal/donation/service_test.go`** (package donation, white-box):
- `TestMigrationsApplyAndRollback` — PASSES (migration validator for Task 1)
- `TestStateMachine_InvalidTransitions` — RED (`t.Fatal`, INV-6, 03-05)
- `TestMandatoryReason` — RED (`t.Fatal`, D-45, 03-05)
- `TestConsentCapture` — RED (`t.Fatal`, D-49, 03-03)
- `TestEDonationKeyedGuard` — RED (`t.Fatal`, D-51, 03-06)

**`internal/donation/service_integration_test.go`** (package donation_test, black-box):
13 `t.Skip` stubs covering all 7 hardest invariants + FR-07/09/10/19/D-50:
`TestIssuanceTransaction_RollbackOnError`, `TestOutboxAtomicity`, `TestSoD_ApproverCannotBeCreator`, `TestSoD_DBCheckConstraint`, `TestConcurrentApproval_ExactlyOneSucceeds`, `TestCancelRetainsReceiptNumber`, `TestPII_TaxIDStoredEncrypted`, `TestPII_RevealRequiresCheckerOrAdmin`, `TestPII_MaskDefault`, `TestVoidAndReissue`, `TestSearchDonations`, `TestCreateDonation`, `TestEditDraft`

**`internal/storage/client.go`** — Package stub + `ErrUnsupportedFileType` + `ErrFileTooLarge`

**`internal/storage/client_test.go`** (package storage_test): 3 `t.Skip` stubs — `TestMagicByteRejectsSpoofed`, `TestSizeLimit`, `TestAllowedTypes`

**Verification:** `go vet ./internal/donation/... ./internal/storage/...` exits 0; `-short` run shows 4 RED + 17 skipped.

## Deviations from Plan

None — plan executed exactly as written. All 4 migration constraints, all 11 sqlc queries, and all Wave 0 test signatures implemented per plan specification.

## Threat Surface Scan

No new network endpoints or auth paths introduced in this plan (schema + codegen only). Threat mitigations T-03-01 through T-03-04 are confirmed present:
- T-03-01: `chk_sod_approver` CHECK constraint ✓
- T-03-02: `chk_receipt_only_on_issued_or_cancelled` CHECK constraint ✓
- T-03-03: `REVOKE DELETE ON donations FROM donnarec_app` ✓
- T-03-04: `donor_tax_id_enc BYTEA NOT NULL` + `donor_tax_id_dek BYTEA NOT NULL` — no plaintext column ✓

## Known Stubs

The following items are intentional Wave 0 stubs, not defects:

| Stub | File | Reason |
|------|------|--------|
| `package storage` has no `StorageClient` struct | `internal/storage/client.go` | Full implementation in plan 03-04 (MinIO client + magic-byte validation) |
| `package donation` has no `DonationService` struct | `internal/donation/errors.go` | Service implementation split across plans 03-03 (create), 03-05 (approve), 03-06 (cancel) |
| 4 RED unit tests with `t.Fatal` | `internal/donation/service_test.go` | Intentional Wave 0 RED contract — filled by 03-03/05/06 |
| 13 `t.Skip` integration tests | `internal/donation/service_integration_test.go` | Intentional Wave 0 contract — filled by 03-03 through 03-07 |
| 3 `t.Skip` storage tests | `internal/storage/client_test.go` | Intentional Wave 0 contract — filled by 03-04 |

## Self-Check: PASSED

All 14 created/modified files confirmed present on disk.

All 3 task commits verified in git log:
- `19f4e79` feat(03-01): add donations + outbox_jobs migrations (000005, 000007)
- `0cf6f26` feat(03-01): add donation + outbox sqlc queries and regenerate Go types
- `bb42a7d` test(03-01): add Wave 0 test scaffolds — donation + storage 7-invariant contract
