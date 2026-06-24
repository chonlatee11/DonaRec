---
phase: 01-foundation-db-auth-rbac-audit-retention-model
plan: "03"
subsystem: pii-encryption-retention-model
tags: [go, crypto, aes-256-gcm, envelope-encryption, pii-masking, retention, legal-hold, postgresql, tdd]
dependency_graph:
  requires:
    - "01-01: RetentionConfig, KeycloakClaims, RoleAdmin/RoleChecker/RoleMaker, testutil.SetupTestPostgres, users table with legal_hold"
  provides:
    - "internal/crypto: KeyProvider interface (WrapKey/UnwrapKey) — LOCKED boundary (Foundational Rule 5)"
    - "internal/crypto: EnvKeyProvider (reads DONAREC_KEK env), AES-256-GCM Encrypt/Decrypt, BlindIndex (HMAC-SHA256)"
    - "internal/crypto: EncryptField/DecryptField (envelope encryption per-field)"
    - "internal/pii: MaskNationalID (last-4 visible, x-xxxx-xxxxx-xNNNN format)"
    - "internal/pii: CanRevealFull (Admin+Checker=true, Maker=false)"
    - "migrations/000003: prevent_legal_hold_delete() trigger function + BEFORE DELETE on users"
    - "internal/retention: ComputeRetainUntil (config-driven), GuardHardDelete (app-level), SoftDelete"
  affects:
    - "Plan 01-04: CRUD handlers may use MaskNationalID + CanRevealFull for PII display"
    - "Plan 03-xx: donor national/tax ID encryption uses EncryptField/DecryptField + BlindIndex"
    - "Phase 3: donor/donation tables extend prevent_legal_hold_delete trigger (Phase 3 runs CREATE TRIGGER on donor/donation)"
tech_stack:
  added:
    - "AES-256-GCM via Go stdlib crypto/cipher (no external crypto lib)"
    - "HMAC-SHA256 via Go stdlib crypto/hmac + crypto/sha256"
    - "PostgreSQL plpgsql BEFORE DELETE trigger (prevent_legal_hold_delete)"
  patterns:
    - "Envelope encryption: fresh DEK per EncryptField call, wrapped by KEK via KeyProvider"
    - "KeyProvider interface abstraction: EnvKeyProvider MVP → CloudKMSProvider future (swap without call site change)"
    - "Nonce-prefixed ciphertext: nonce || ciphertext || GCM-tag in Encrypt output"
    - "BlindIndex (HMAC-SHA256 with separate index key): searchable encryption for Phase 3"
    - "Last-4 PII mask: x-xxxx-xxxxx-xNNNN format; role-gated just-in-time reveal"
    - "Defense-in-depth: app GuardHardDelete + DB trigger both block legal_hold hard-delete"
    - "Config-driven retain_until: ComputeRetainUntil reads days from RetentionConfig, zero literals in service"
key_files:
  created:
    - donnarec-api/internal/crypto/keyprovider.go
    - donnarec-api/internal/crypto/envprovider.go
    - donnarec-api/internal/crypto/aes_gcm.go
    - donnarec-api/internal/crypto/envelope.go
    - donnarec-api/internal/crypto/keyprovider_test.go
    - donnarec-api/internal/crypto/aes_gcm_test.go
    - donnarec-api/internal/pii/mask.go
    - donnarec-api/internal/pii/mask_test.go
    - donnarec-api/migrations/000003_retention_triggers.up.sql
    - donnarec-api/migrations/000003_retention_triggers.down.sql
    - donnarec-api/internal/retention/service.go
    - donnarec-api/internal/retention/retention_test.go
  modified: []
decisions:
  - "D-25 realized: KeyProvider interface (WrapKey/UnwrapKey) as LOCKED boundary — EnvKeyProvider MVP, CloudKMSProvider path open"
  - "D-26 realized: EnvKeyProvider reads DONAREC_KEK hex env; error if missing/malformed (Pitfall 5 avoided)"
  - "D-10 realized: CanRevealFull — Admin+Checker=true, Maker=false"
  - "D-11 realized: MaskNationalID — last-4 visible, x-xxxx-xxxxx-xNNNN format"
  - "D-12 realized: CanRevealFull is per-request (just-in-time), not cached"
  - "D-13 acknowledged: audit cross-reference comment in mask.go; reveal endpoint + audit wiring deferred to Phase 3"
  - "D-14 realized: Phase 1 provides mechanism only; donor PII usage in Phase 3"
  - "D-18 realized: ComputeRetainUntil config-driven (donation=cfg.DonationRetainDays, audit_log=cfg.AuditLogRetainDays)"
  - "D-19 realized: GuardHardDelete app-level + DB trigger defense-in-depth — no hard-delete under legal_hold"
metrics:
  duration_minutes: 22
  completed_date: "2026-06-24"
  tasks_completed: 3
  tasks_total: 3
  files_created: 12
  files_modified: 0
---

# Phase 01 Plan 03: PII Encryption Boundary + Retention Model Summary

**One-liner:** AES-256-GCM envelope encryption with KeyProvider KMS abstraction, last-4 PII mask with role-gated reveal (Admin/Checker), and config-driven retain_until plus defense-in-depth legal_hold guard (app + PostgreSQL trigger).

## Tasks Completed

| Task | Name | Commit | Result |
|------|------|--------|--------|
| 1 RED | crypto tests failing | 3e50115 | RED: build failed (no implementation) |
| 1 GREEN | crypto package implementation | a889f46 | GREEN: all 4 test suites pass |
| 2 RED | pii tests failing | 86cf27c | RED: build failed (no implementation) |
| 2 GREEN | pii package implementation | 62cd66c | GREEN: TestMaskNationalID + TestCanRevealFull pass |
| 3 RED | retention tests failing | 8dd6bf5 | RED: build failed (no implementation) |
| 3 GREEN | retention package + migration | 9fd5b49 | GREEN: unit + DB trigger integration test pass |

## What Was Built

### Task 1: crypto package (KeyProvider / AES-256-GCM / envelope / BlindIndex)

**KeyProvider interface (LOCKED boundary — Foundational Rule 5):**
```go
type KeyProvider interface {
    WrapKey(ctx context.Context, plaintextDEK []byte) ([]byte, error)
    UnwrapKey(ctx context.Context, wrappedDEK []byte) ([]byte, error)
}
```
Interface shape is frozen after Phase 1 commit. Phase 3 and Phase 6 consume this interface without changes.

**EnvKeyProvider:** reads `DONAREC_KEK` hex env (must be 32 bytes). Constructor errors if missing, non-hex, or wrong length (D-26, Pitfall 5 avoided). WrapKey/UnwrapKey use AES-256-GCM so wrapped DEKs are GCM-authenticated.

**AES-256-GCM (stdlib only):** `Encrypt` generates fresh nonce via `rand.Reader`, outputs `nonce || ciphertext || GCM-tag`. `Decrypt` extracts nonce from prefix, calls `gcm.Open` (tamper detection via GCM auth tag). No external crypto library imported.

**Envelope (EncryptField/DecryptField):** `EncryptField` generates fresh 32-byte DEK per call via `rand.Read`, encrypts plaintext with DEK, wraps DEK with KEK via `KeyProvider.WrapKey`. Plaintext DEK is zeroed from stack after use (best-effort). `DecryptField` unwraps DEK then decrypts ciphertext.

**BlindIndex:** HMAC-SHA256 with separate `indexKey`. Deterministic for same input+key. Phase 3 stores alongside encrypted national ID for lookupability without exposing plaintext.

### Task 2: pii package (MaskNationalID / CanRevealFull)

**MaskNationalID:** masks all but last 4 characters. Standard 13-digit Thai national ID format: `x-xxxx-xxxxx-xNNNN` (last 4 digits visible). Handles empty/short input safely without panicking.

**CanRevealFull:** role gate per D-10 — `HasRole(RoleAdmin) || HasRole(RoleChecker)`. Maker-only returns false. Just-in-time per-request (not cached). Audit cross-reference comment in `mask.go` documents that the reveal endpoint (Phase 3) must write `action="pii.reveal"` audit entry via 01-02 path (D-13).

### Task 3: retention package + migration 000003

**Migration 000003 (up):**
- `prevent_legal_hold_delete()` plpgsql function: raises `EXCEPTION 'cannot delete record under legal hold'` when `OLD.legal_hold = true`
- `BEFORE DELETE FOR EACH ROW` trigger on `users` table
- Phase 3 extension: same function re-used by adding `CREATE TRIGGER` on donor/donation tables (function unchanged)

**Migration 000003 (down):** `DROP TRIGGER IF EXISTS` then `DROP FUNCTION IF EXISTS` (correct order)

**ComputeRetainUntil:** maps entity type to config days — `"donation"` → `cfg.DonationRetainDays`, `"audit_log"` → `cfg.AuditLogRetainDays`, unknown → `DonationRetainDays` fallback. Zero hardcoded literal day counts in service logic (D-18).

**GuardHardDelete:** app-level block — returns `AppError{Code: "retention.legal_hold_delete_blocked"}` when `recordLegalHold=true`. Called before any SQL DELETE. i18n key matches the catalog entry in `th.json`/`en.json` from 01-01.

**SoftDelete:** always returns nil — documents the invariant that soft delete (UPDATE `is_active=false`) bypasses legal_hold entirely (D-19 only blocks hard DELETE).

## Test Results (GREEN)

```
TestEnvKeyProvider PASS
  ✓ valid KEK wraps and unwraps DEK round-trip
  ✓ missing DONAREC_KEK → constructor error
  ✓ short DONAREC_KEK (< 32 bytes) → constructor error
  ✓ non-hex DONAREC_KEK → constructor error

TestAESGCMRoundTrip PASS
  ✓ Encrypt then Decrypt returns original plaintext
  ✓ two Encrypt calls on same plaintext produce different ciphertext (random nonce)
  ✓ tampered ciphertext → Decrypt error (GCM auth tag detection)
  ✓ wrong key → Decrypt error
  ✓ truncated ciphertext → Decrypt error

TestEnvelopeRoundTrip PASS
  ✓ EncryptField then DecryptField returns original plaintext
  ✓ wrappedDEK is not the raw DEK (DEK is wrapped under KEK)
  ✓ tampered field ciphertext → DecryptField error

TestBlindIndex PASS
  ✓ same input + key → same output (deterministic)
  ✓ different plaintext → different output
  ✓ different key → different output
  ✓ output is 32 bytes (HMAC-SHA256 digest size)

TestEncryptDecryptEmpty PASS
  ✓ empty plaintext round-trips correctly

TestEncryptKeyLength PASS
  ✓ invalid key length (10 bytes) → Encrypt error

TestMaskNationalID PASS
  ✓ 13-digit Thai ID: last 4 visible, prefix masked
  ✓ ID ending in 0000: last 4 visible
  ✓ all-same-digit ID: last 4 visible
  ✓ empty input → safe placeholder (no panic)
  ✓ short input (< 4 chars) → safe output (no panic)
  ✓ exactly 4 chars → all visible

TestCanRevealFull PASS
  ✓ Admin role → true (D-10)
  ✓ Checker role → true (D-10)
  ✓ Maker-only → false (D-10)
  ✓ Maker+Checker → true (multi-role D-02)
  ✓ no roles → false
  ✓ unknown role → false

TestRetainUntilCalculation PASS
  ✓ donation → t0 + 1825 days (config-driven)
  ✓ audit_log → t0 + 3650 days (config-driven)
  ✓ unknown type → fallback to DonationRetainDays
  ✓ custom config (2190d) reflected correctly (not hardcoded)

TestSoftDeleteAllowed PASS
  ✓ GuardHardDelete(false) returns nil
  ✓ SoftDelete(legal_hold=true) → nil (soft delete always allowed)

TestGuardHardDeleteUnit PASS
  ✓ GuardHardDelete(false) → nil
  ✓ GuardHardDelete(true) → error containing "retention.legal_hold_delete_blocked"

TestLegalHoldDeleteBlocked PASS (integration — testcontainers PostgreSQL 17)
  ✓ hard DELETE on legal_hold=true user → DB trigger raises exception
  ✓ GuardHardDelete(true) → app-level error (blocks before DB)
  ✓ hard DELETE on legal_hold=false user → succeeds (1 row deleted)
  ✓ GuardHardDelete(false) → nil
```

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] TestAESGCMRoundTrip: nil vs empty slice assertion in TestEncryptDecryptEmpty**
- **Found during:** Task 1 GREEN verification (`go test ./internal/crypto/...`)
- **Issue:** Test expected `[]byte{}` but `cipher.GCM.Open` returns `nil` for empty plaintext. Strictly speaking, both are zero-length slices, but `assert.Equal` distinguishes `nil` from `[]byte{}`
- **Fix:** Changed assertion to `assert.True(t, len(got) == 0, ...)` — accepts both nil and empty
- **Files modified:** `donnarec-api/internal/crypto/aes_gcm_test.go`
- **Commit:** a889f46

**2. [Rule 1 - Bug] MaskNationalID: dash in format caused last-4 output chars to not match last-4 input chars**
- **Found during:** Task 2 GREEN verification (`go test ./internal/pii/...`)
- **Issue:** Format `x-xxxx-xxxxx-xN-NNN` inserted a dash before the last 3 chars, so the last 4 output chars were `-NNN` instead of `NNNN` — test assertion `last 4 chars of masked output must match last 4 of input` failed for all 13-digit IDs
- **Fix:** Changed format to `x-xxxx-xxxxx-x` + `last4` (no dash in the reveal section), so last 4 output chars are exactly the last 4 input digits
- **Files modified:** `donnarec-api/internal/pii/mask.go`
- **Commit:** 62cd66c (included in GREEN feat commit)

## Known Stubs

| Stub | File | Reason |
|------|------|--------|
| `SoftDelete` returns nil always | `internal/retention/service.go` | Stub for Phase 3; actual DB UPDATE is_active=false happens in UserService.DeactivateUser (01-04) and future DonorService. The invariant (soft delete never blocked) is tested here. |
| `// Phase 3 will use this interface...` comment | `internal/crypto/envelope.go` | EncryptField/DecryptField are implemented and tested; donor PII usage wired in Phase 3 |
| `// Phase 3: extend trigger to donor/donation tables` comment | `migrations/000003_retention_triggers.up.sql` | Function is reusable; Phase 3 adds CREATE TRIGGER for new tables |

These stubs do NOT prevent the plan's goal from being achieved. All crypto/pii/retention boundaries are fully functional.

## Threat Surface Scan

All mitigations from the plan's `<threat_model>` implemented:

| Threat ID | Mitigation Status |
|-----------|-------------------|
| T-1-pii-01 | AES-256-GCM envelope; round-trip + tamper tests GREEN |
| T-1-pii-02 | EnvKeyProvider errors if DONAREC_KEK absent; no key literal in source (TestEnvKeyProvider negative cases GREEN) |
| T-1-pii-mask | MaskNationalID last-4; CanRevealFull Admin+Checker only; audit cross-ref comment in mask.go |
| T-1-pii-03 | GCM auth tag detects tamper in TestAESGCMRoundTrip + TestEnvelopeRoundTrip |
| T-1-retention-01 | GuardHardDelete (app) + prevent_legal_hold_delete trigger (DB) both block; TestLegalHoldDeleteBlocked GREEN |
| T-1-retention-02 | accept — config-driven; schema accepts any value; DPO pending |
| T-1-pii-04 | accept — HMAC-SHA256 with separate index key; not reversible; Phase 3 usage |

No new threat surfaces introduced beyond those in the plan's threat model.

## TDD Gate Compliance

| Gate | Commit | Status |
|------|--------|--------|
| RED — crypto tests | 3e50115 | PASS (build failed — no impl) |
| GREEN — crypto impl | a889f46 | PASS (all tests GREEN) |
| RED — pii tests | 86cf27c | PASS (build failed — no impl) |
| GREEN — pii impl | 62cd66c | PASS (all tests GREEN) |
| RED — retention tests | 8dd6bf5 | PASS (build failed — no impl) |
| GREEN — retention impl + migration | 9fd5b49 | PASS (unit + integration GREEN) |

## Self-Check: PASSED

Files exist:
- [x] donnarec-api/internal/crypto/keyprovider.go
- [x] donnarec-api/internal/crypto/envprovider.go
- [x] donnarec-api/internal/crypto/aes_gcm.go
- [x] donnarec-api/internal/crypto/envelope.go
- [x] donnarec-api/internal/crypto/keyprovider_test.go
- [x] donnarec-api/internal/crypto/aes_gcm_test.go
- [x] donnarec-api/internal/pii/mask.go
- [x] donnarec-api/internal/pii/mask_test.go
- [x] donnarec-api/migrations/000003_retention_triggers.up.sql
- [x] donnarec-api/migrations/000003_retention_triggers.down.sql
- [x] donnarec-api/internal/retention/service.go
- [x] donnarec-api/internal/retention/retention_test.go

Commits exist:
- [x] 3e50115 — test(01-03): add failing tests RED for crypto package
- [x] a889f46 — feat(01-03): implement crypto package GREEN
- [x] 86cf27c — test(01-03): add failing tests RED for pii package
- [x] 62cd66c — feat(01-03): implement pii package GREEN
- [x] 8dd6bf5 — test(01-03): add failing tests RED for retention package
- [x] 9fd5b49 — feat(01-03): implement retention package GREEN
