---
phase: 03-donation-lifecycle-maker-checker-issuance
plan: "04"
subsystem: slip-storage
tags: [storage, minio, magic-byte, soft-delete, pdpa, tdd]
dependency_graph:
  requires: ["03-03"]
  provides: ["slip-upload-api", "minio-storage-seam", "slip-presigned-url"]
  affects: ["03-05", "03-06", "06-public-form"]
tech_stack:
  added:
    - "github.com/minio/minio-go/v7 v7.2.1 — MinIO S3-compatible client"
  patterns:
    - "ValidateSlip (exported) — magic-byte validation without live MinIO (unit testable)"
    - "io.MultiReader reassembly — prepend consumed header bytes after magic detection"
    - "io.LimitReader defense-in-depth — caps stream even if declared size lies"
    - "soft-delete via UPDATE deleted_at + REVOKE DELETE at DB level (D-54)"
    - "transactional outbox pattern: InsertSlip + AppendAuditEntryTx in single WithTx"
key_files:
  created:
    - donnarec-api/internal/storage/client.go
    - donnarec-api/internal/storage/client_test.go
    - donnarec-api/migrations/000006_slip_attachments.up.sql
    - donnarec-api/migrations/000006_slip_attachments.down.sql
    - donnarec-api/internal/db/queries/slip.sql
    - donnarec-api/internal/db/generated/slip.sql.go
    - donnarec-api/internal/donation/slip_service.go
    - donnarec-api/internal/donation/slip_handler.go
  modified:
    - donnarec-api/internal/config/config.go
    - donnarec-api/docker-compose.yml
    - donnarec-api/cmd/server/main.go
    - donnarec-api/go.mod
    - donnarec-api/go.sum
    - donnarec-api/internal/db/generated/models.go
    - donnarec-api/internal/db/generated/querier.go
decisions:
  - "D-48 honored: slip is optional — ErrSlipNotFound (404) for cash/no-slip donations is normal"
  - "D-54 honored: REVOKE DELETE on slip_attachments + SoftDeleteSlip UPDATE — files never hard-deleted"
  - "T-03-14: mimetype.Detect(buf[:512]) — magic bytes, not filename/Content-Type header"
  - "T-03-15: dual guard — declared-size fast-reject + io.LimitReader defense-in-depth"
  - "T-03-16: PresignedGet with 15-min TTL; objectKey contains UUID (not guessable)"
  - "T-03-17: REVOKE DELETE on slip_attachments prevents DB-level hard delete even on logic bug"
  - "Re-integration strategy: copied slip-only files verbatim from salvage/03-04-stale; hand-integrated delta onto canonical main.go/config.go/docker-compose.yml; regenerated sqlc (did not copy salvage generated files)"
metrics:
  duration: "~45 minutes"
  completed: "2026-07-01"
  tasks_completed: 3
  files_changed: 15
---

# Phase 03 Plan 04: Slip Object-Storage Slice Summary

**One-liner:** MinIO storage seam with magic-byte validation (JPEG/PNG/PDF), 10 MB cap, soft-delete-retain, and presigned URL view — wired onto the existing donation route group.

## What Was Built

### Task 1: MinIO Storage Client + Validation Tests (TDD GREEN)

`internal/storage/client.go` provides:
- `NewStorageClient(endpoint, accessKey, secretKey, bucket, secure)` — wraps minio-go/v7 with StaticV4 creds
- `ValidateSlip(r, size)` — exported for unit testing: fast-rejects by declared size (`> 10 MB`), then reads first 512 bytes via `io.ReadFull`, calls `mimetype.Detect`, checks allowlist `{image/jpeg, image/png, application/pdf}`
- `PutSlip(ctx, r, size, donationID)` — calls validateSlip, reassembles reader via `io.MultiReader(bytes.NewReader(head), io.LimitReader(remaining, ...))`, streams to MinIO with `objectKey = slips/{donationID}/{uuid}{ext}`
- `PresignedGet(ctx, objectKey, ttl)` — delegates to `minio.PresignedGetObject` with caller-specified TTL

`internal/storage/client_test.go` — 8 unit tests (all pass, no live MinIO):
- `TestAllowedTypes` (3 subtests: jpeg, png, pdf)
- `TestMagicByteRejectsSpoofed` — shell script bytes rejected with `ErrUnsupportedFileType`
- `TestSizeLimit` (2 subtests: over-limit rejected, at-limit accepted)

### Task 2: Migration + Queries + Config + docker-compose

`migrations/000006_slip_attachments.up.sql`:
- `slip_attachments` table: id UUID PK, `donation_id REFERENCES donations(id)`, `object_key TEXT`, `mime_type TEXT`, `size_bytes BIGINT`, `uploaded_by REFERENCES users(id)`, `uploaded_at TIMESTAMPTZ DEFAULT now()`, `deleted_at TIMESTAMPTZ NULL`, `deleted_by UUID NULL REFERENCES users(id)`
- Partial index `WHERE deleted_at IS NULL` for fast active-slip lookup
- `GRANT SELECT, INSERT, UPDATE; REVOKE DELETE` — DB-level D-54 enforcement

`internal/db/queries/slip.sql` + generated `slip.sql.go`:
- `InsertSlip :one` — inserts reference after successful PutSlip
- `GetActiveSlipByDonation :one` — `WHERE donation_id = $1 AND deleted_at IS NULL`; returns `pgx.ErrNoRows` for cash donations
- `SoftDeleteSlip :exec` — `UPDATE SET deleted_at = now(), deleted_by = $1 WHERE id = $2`

`internal/config/config.go` — added `MinIOConfig{Endpoint, AccessKey, SecretKey, Bucket, Secure}` loaded from `MINIO_*` env vars; `MINIO_ENDPOINT` added to `validate()` required map; `getEnvBool` helper added.

`docker-compose.yml` — added `minio` service (image `minio/minio:latest`, ports 9000/9001, healthcheck, `minio_data` volume), MinIO env on `api` service, `api` depends_on `minio: service_healthy`.

### Task 3: Slip Service + Handler + Routes

`internal/donation/slip_service.go`:
- `SlipService{pool, queries, storage, auditSvc, logger}` + constructor
- `UploadSlip`: checks for existing active slip (ErrSlipAlreadyExists), calls `storage.PutSlip`, then `WithTx { InsertSlip + AppendAuditEntryTx("slip.upload") }`
- `ViewSlip`: `GetActiveSlipByDonation` → `storage.PresignedGet(15m)` → `SlipViewResponse{url, expires_in_seconds: 900}`
- `RemoveSlip`: `GetActiveSlipByDonation` → `WithTx { SoftDeleteSlip + AppendAuditEntryTx("slip.remove") }` — no file deletion (D-54)

`internal/donation/slip_handler.go`:
- `SlipHandler{svc, logger}` + constructor
- `Upload` — `c.FormFile("file")`, maps `ErrFileTooLarge→413`, `ErrUnsupportedFileType→415`, `ErrSlipAlreadyExists→409`
- `View` — returns presigned URL; `ErrSlipNotFound→404`
- `Remove` — soft-delete; returns `204 No Content`

`cmd/server/main.go`:
- Constructs `storage.NewStorageClient(cfg.MinIO.*)` + `donation.NewSlipService` + `donation.NewSlipHandler`
- Registers on existing `/api/donations` group: `POST /:id/slip`, `GET /:id/slip`, `DELETE /:id/slip`
- All existing donation routes (Create, List, GetByID, Update, Submit) preserved

## Verification

- `go build ./...` — passes
- `go vet ./...` — no issues
- `go test ./internal/storage/... ./internal/donation/... -timeout 240s` — 17 passed, 9 skipped, 2 pre-existing failures (Wave-0 scaffolds for 03-05/03-06, unrelated to this plan)
- `docker compose config` — valid (minio service with port 9000 confirmed in output)

## Deviations from Plan

### Re-integration (expected — documented in prompt)

**Context:** A prior 03-04 executor had branched from a stale base. This execution re-integrated the slip-specific work onto the correct canonical HEAD (which already had 03-01 data layer, 03-02 frontend, 03-03 donation service).

**Method:**
- Verbatim copy: `storage/client.go`, `storage/client_test.go`, `slip_service.go`, `slip_handler.go`, `migrations/000006_*.sql`, `db/queries/slip.sql` — read from `salvage/03-04-stale` via `git show`
- Hand-integrated delta: `config.go`, `docker-compose.yml`, `cmd/server/main.go` — applied only the slip-specific additions onto the current canonical files
- Regenerated: `internal/db/generated/` — ran `sqlc generate` fresh (did not copy salvage's generated files to avoid divergent model contamination)

## Known Stubs

None — all slip endpoints are fully wired. The slip attachment is optional (D-48), so a 404 from `GET /:id/slip` is the correct response for cash/no-slip donations, not a stub.

## Threat Flags

No new threat surface beyond what is in the plan's `<threat_model>`. All STRIDE threats (T-03-14 through T-03-SC) are mitigated:
- T-03-14: magic-byte validation implemented and unit-tested
- T-03-15: dual-layer size cap implemented and unit-tested
- T-03-16: 15-min presigned TTL + UUID in object key
- T-03-17: REVOKE DELETE in migration + SoftDeleteSlip in service (no hard delete path)
- T-03-SC: minio-go/v7 v7.2.1 from official MinIO org; checksum verified by `go get`

## Self-Check: PASSED

Files exist:
- `/home/chonlatee/Desktop/Lab/DonaRec/donnarec-api/internal/storage/client.go` — FOUND
- `/home/chonlatee/Desktop/Lab/DonaRec/donnarec-api/internal/donation/slip_service.go` — FOUND
- `/home/chonlatee/Desktop/Lab/DonaRec/donnarec-api/internal/donation/slip_handler.go` — FOUND
- `/home/chonlatee/Desktop/Lab/DonaRec/donnarec-api/migrations/000006_slip_attachments.up.sql` — FOUND
- `/home/chonlatee/Desktop/Lab/DonaRec/donnarec-api/internal/db/generated/slip.sql.go` — FOUND

Commits exist:
- `541862a` feat(03-04): storage client + magic-byte/size validation
- `cfc11d5` feat(03-04): migration 000006 + slip queries + MinIO config + docker-compose
- `1786ff1` feat(03-04): slip service+handler wired into donation routes
