# Phase 3: Donation Lifecycle & Maker-Checker Issuance - Pattern Map

**Mapped:** 2026-06-28
**Files analyzed:** 23 new/modified files
**Analogs found:** 20 / 23

---

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `internal/donation/handler.go` | controller | request-response | `internal/users/handler.go` | exact |
| `internal/donation/service.go` | service | CRUD + event-driven (tx) | `internal/users/service.go` | exact |
| `internal/donation/model.go` | model | transform | `internal/users/service.go` (User/Params structs) | role-match |
| `internal/donation/service_test.go` | test | request-response | `internal/users/service_test.go` | role-match |
| `internal/donation/service_integration_test.go` | test | CRUD + concurrency | `internal/receiptno/allocator_concurrency_test.go` | exact |
| `internal/storage/client.go` | service | file-I/O | none — no object storage analog | none |
| `internal/storage/client_test.go` | test | file-I/O | `internal/receiptno/allocator_concurrency_test.go` (testcontainers pattern) | partial |
| `migrations/000005_donations.up.sql` | migration | batch | `migrations/000004_receipt_number_tables.up.sql` | exact |
| `migrations/000005_donations.down.sql` | migration | batch | `migrations/000004_receipt_number_tables.down.sql` | exact |
| `migrations/000006_slip_attachments.up.sql` | migration | batch | `migrations/000001_init_schema.up.sql` | role-match |
| `migrations/000006_slip_attachments.down.sql` | migration | batch | `migrations/000001_init_schema.down.sql` | role-match |
| `migrations/000007_outbox_jobs.up.sql` | migration | batch | `migrations/000004_receipt_number_tables.up.sql` | role-match |
| `migrations/000007_outbox_jobs.down.sql` | migration | batch | `migrations/000004_receipt_number_tables.down.sql` | role-match |
| `internal/db/queries/donations.sql` | utility | CRUD + FOR UPDATE | `internal/db/queries/receiptno.sql` + `internal/db/queries/users.sql` | exact |
| `internal/db/queries/outbox.sql` | utility | CRUD | `internal/db/queries/users.sql` | role-match |
| `internal/config/config.go` (modified) | config | request-response | self (`internal/config/config.go`) | exact |
| `cmd/server/main.go` (modified) | config | request-response | self (`cmd/server/main.go`) | exact |
| `donnarec-web/` (bootstrap) | component | request-response | none — no frontend exists yet | none |
| `donnarec-web/app/donations/**` (pages) | component | request-response | none — no frontend exists yet | none |
| `donnarec-web/components/StatusBadge.tsx` | component | transform | none — no frontend exists yet | none |
| `donnarec-web/components/MaskedIdField.tsx` | component | request-response | none — no frontend exists yet | none |
| `donnarec-web/components/SlipUploadZone.tsx` | component | file-I/O | none — no frontend exists yet | none |
| `donnarec-web/components/ReviewReasonDialog.tsx` | component | request-response | none — no frontend exists yet | none |

---

## Pattern Assignments

### `internal/donation/handler.go` (controller, request-response)

**Analog:** `internal/users/handler.go`

**Imports pattern** (lines 1-11):
```go
package users

import (
    "net/http"

    "github.com/donnarec/donnarec-api/internal/auth"
    "github.com/gin-gonic/gin"
    "github.com/go-playground/validator/v10"
    "go.uber.org/zap"
)
```
For `donation/handler.go` replace `auth` import with `auth` + `donation` (self) + `uuid`. Add `errors` for sentinel error mapping.

**Handler struct pattern** (lines 15-28):
```go
type UserHandler struct {
    svc      *UserService
    validate *validator.Validate
    logger   *zap.Logger
}

func NewUserHandler(svc *UserService, logger *zap.Logger) *UserHandler {
    return &UserHandler{
        svc:      svc,
        validate: validator.New(),
        logger:   logger,
    }
}
```
`DonationHandler` adds `storageSvc *storage.Client` to the struct for slip upload.

**Claims extraction pattern** (lines 37-48 of handler.go):
```go
raw, exists := c.Get("claims")
if !exists {
    c.JSON(http.StatusUnauthorized, gin.H{"error": "missing_auth_context"})
    return
}
claims, ok := raw.(auth.KeycloakClaims)
if !ok {
    c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid_claims_type"})
    return
}
```
Every handler that reads the actor (approve, return, reject, cancel, pii.reveal) copies this block verbatim.

**Request bind + validate pattern** (lines 76-88):
```go
var req CreateUserRequest
if err := c.ShouldBindJSON(&req); err != nil {
    c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request_body"})
    return
}
if err := h.validate.Struct(req); err != nil {
    c.JSON(http.StatusUnprocessableEntity, gin.H{
        "error":   "validation_failed",
        "details": err.Error(),
    })
    return
}
```

**Error logging pattern — no PII in logs** (lines 101-107):
```go
h.logger.Error("failed to create user in handler",
    zap.String("operation", "CreateUser"),
    zap.Error(err),
    // ห้าม log PII: no email or national_id in error logs (Pattern C)
)
c.JSON(http.StatusInternalServerError, gin.H{"error": "user_creation_failed"})
```
For donation handler: log `donation_id`, `status` only. Never log `donor_tax_id`, `donor_name`, or any PII field.

**Audit marker pattern** (line 112):
```go
c.Set("audit_after", user)
```
Set `c.Set("audit_after", ...)` at the end of every mutating handler so the AuditMiddleware captures the after-state.

**SoD error mapping** — new for Phase 3. Handler must map service-layer sentinel errors to HTTP status codes:
```go
// In the approve handler, after calling h.svc.Approve(...)
switch {
case errors.Is(err, donation.ErrInvalidTransition):
    c.JSON(http.StatusConflict, gin.H{"error": "status_conflict"})
case errors.Is(err, donation.ErrSoDViolation):
    c.JSON(http.StatusForbidden, gin.H{"error": "sod_violation"})
case errors.Is(err, donation.ErrMissingReason):
    c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "reason_required"})
default:
    h.logger.Error("approve failed", zap.String("donation_id", donationID), zap.Error(err))
    c.JSON(http.StatusInternalServerError, gin.H{"error": "approve_failed"})
}
```

---

### `internal/donation/service.go` (service, CRUD + event-driven)

**Analog:** `internal/users/service.go`

**Imports pattern** (lines 1-16 of users/service.go):
```go
package users

import (
    "context"
    "errors"
    "fmt"

    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgtype"
    "github.com/jackc/pgx/v5/pgxpool"
    "go.uber.org/zap"

    dbhelpers "github.com/donnarec/donnarec-api/internal/db"
    db "github.com/donnarec/donnarec-api/internal/db/generated"
)
```
For `donation/service.go` add:
```go
"github.com/donnarec/donnarec-api/internal/audit"
"github.com/donnarec/donnarec-api/internal/auth"
"github.com/donnarec/donnarec-api/internal/crypto"
"github.com/donnarec/donnarec-api/internal/pii"
"github.com/donnarec/donnarec-api/internal/receiptno"
```

**Service struct + constructor pattern** (lines 49-62):
```go
type UserService struct {
    pool    *pgxpool.Pool
    queries *db.Queries
    logger  *zap.Logger
}

func NewUserService(pool *pgxpool.Pool, queries *db.Queries, logger *zap.Logger) *UserService {
    return &UserService{
        pool:    pool,
        queries: queries,
        logger:  logger,
    }
}
```
`DonationService` extends this with `allocator *receiptno.Allocator`, `auditSvc *audit.AuditService`, `keyProvider crypto.KeyProvider`.

**WithTx pattern for issuance transaction** (lines 79-125 of users/service.go):
```go
var result *User
err := dbhelpers.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
    qtx := s.queries.WithTx(tx)

    row, err := qtx.CreateUser(ctx, db.CreateUserParams{ ... })
    if err != nil {
        s.logger.Error("failed to create user",
            zap.String("operation", "CreateUser"),
            zap.Error(err),
        )
        return fmt.Errorf("create user: %w", err)
    }
    // ... more steps ...
    return nil
})
if err != nil {
    return nil, err
}
```
The issuance transaction in `DonationService.Approve` follows this exact structure. Copy `WithTx` wrapper → inside: `LockDonationForUpdate` → precondition check → SoD check → `s.allocator.Allocate(ctx, tx, approvedAt)` → `qtx.IssueDonation(...)` → `s.auditSvc.AppendAuditEntryTx(ctx, tx, ...)` → `qtx.EnqueueOutboxJob(...)`.

**Key rule from allocator.go (lines 93-117):** `Allocate` takes `pgx.Tx` NOT `*pgxpool.Pool`. The `tx` comes from the `func(tx pgx.Tx) error` closure of `dbhelpers.WithTx`. Passing `pool` instead of `tx` is a hard error.

**Error wrapping pattern** (lines 91-94 of users/service.go):
```go
return fmt.Errorf("create user: %w", err)
```
All wrapped errors follow `"<operation>: %w"` convention. Sentinel errors (`ErrInvalidTransition`, `ErrSoDViolation`, `ErrMissingReason`) are package-level `var` declarations, not inline errors.

**PII encrypt on write** — from `internal/crypto/envelope.go` (lines 29-51):
```go
// In CreateDonation, before INSERT:
ciphertext, wrappedDEK, err := crypto.EncryptField(ctx, s.keyProvider, []byte(req.DonorTaxID))
if err != nil {
    return fmt.Errorf("encrypt tax id: %w", err)
}
// Store ciphertext in donor_tax_id_enc, wrappedDEK in donor_tax_id_dek
// NEVER log req.DonorTaxID (Pattern C)
```

**PII mask on read** — from `internal/pii/mask.go` (lines 60-85):
```go
// In response serialization — always default to masked:
resp.DonorTaxIDMasked = pii.MaskNationalID(plaintext_or_empty_placeholder)
```

**PII reveal path** — from `internal/pii/mask.go` (lines 104-115) + `internal/crypto/envelope.go` (lines 67-81):
```go
// GET /api/donations/:id/pii — Checker/Admin only
if !pii.CanRevealFull(claims) {
    return ErrForbidden  // handler maps to 403
}
plaintext, err := crypto.DecryptField(ctx, s.keyProvider, row.DonorTaxIDEnc, row.DonorTaxIDDek)
if err != nil { return fmt.Errorf("decrypt tax id: %w", err) }
// MUST write audit BEFORE returning plaintext (D-13):
if err := s.auditSvc.AppendAuditEntryTx(ctx, tx, audit.AuditEntry{
    Action:  "pii.reveal",
    ...
}); err != nil { return fmt.Errorf("audit pii reveal: %w", err) }
return plaintext
```

---

### `internal/donation/model.go` (model, transform)

**Analog:** struct definitions in `internal/users/service.go` (lines 19-45)

**Pattern** — package-local request/response structs, never expose DB rows directly:
```go
// In users/service.go — the pattern to copy:
type User struct {
    ID          string     `json:"id"`
    Email       string     `json:"email"`
    DisplayName string     `json:"display_name"`
    IsActive    bool       `json:"is_active"`
    Roles       []UserRole `json:"roles"`
}

type CreateUserParams struct {
    Email       string
    DisplayName string
    Roles       []UserRole
}
```
For `donation/model.go`: define `DonationResponse` (always with `DonorTaxIDMasked string`, never `DonorTaxIDEnc []byte`), `CreateDonationRequest`, `ApproveDonationRequest`, `CancelDonationRequest` (includes `RDConfirmationReason string` for `edonation_keyed=true` path per D-51).

**Validation tags pattern** (lines 59-63 of handler.go):
```go
type CreateUserRequest struct {
    Email           string   `json:"email"            validate:"required,email,max=255"`
    DisplayName     string   `json:"display_name"     validate:"required,min=2,max=100"`
    KeycloakSubject string   `json:"keycloak_subject" validate:"required,min=1,max=255"`
    Roles           []string `json:"roles"            validate:"required,min=1,dive,oneof=maker checker admin"`
}
```

---

### `internal/donation/service_integration_test.go` (test, concurrency)

**Analog:** `internal/receiptno/allocator_concurrency_test.go`

**File header + skip guard pattern** (lines 1-57 of allocator_concurrency_test.go):
```go
package receiptno_test

import (
    "context"
    "sync"
    "testing"
    "time"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "golang.org/x/sync/errgroup"

    dbhelpers "github.com/donnarec/donnarec-api/internal/db"
    db "github.com/donnarec/donnarec-api/internal/db/generated"
    "github.com/donnarec/donnarec-api/internal/receiptno"
    "github.com/donnarec/donnarec-api/internal/testutil"
)

func TestXxx(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test in short mode")
    }
    pool := testutil.SetupTestPostgres(t)
    // ...
}
```

**Parallel goroutine pattern with errgroup** (lines 64-86):
```go
const N = 50
var mu sync.Mutex
committed := make([]int, 0, N)

g, gctx := errgroup.WithContext(ctx)
for i := 0; i < N; i++ {
    g.Go(func() error {
        return dbhelpers.WithTx(gctx, pool, func(tx pgx.Tx) error {
            r, err := alloc.Allocate(gctx, tx, issueDate)
            if err != nil { return err }
            mu.Lock()
            committed = append(committed, r.RunningNo)
            mu.Unlock()
            return nil
        })
    })
}
require.NoError(t, g.Wait(), "all N parallel operations must succeed")
```
For `TestConcurrentApproval` in `donation/service_integration_test.go`: replace `alloc.Allocate` with `svc.Approve(...)`. Assert that exactly ONE approval succeeds and the second returns `ErrInvalidTransition` (not an internal error), and exactly one receipt_numbers row exists for that donation.

**DB-level assertion pattern** (lines 107-114):
```go
var totalCount, distinctCount int
err := pool.QueryRow(ctx,
    `SELECT COUNT(*), COUNT(DISTINCT running_no) FROM receipt_numbers WHERE fiscal_year = $1`,
    2569).Scan(&totalCount, &distinctCount)
require.NoError(t, err)
assert.Equal(t, N, totalCount)
assert.Equal(t, N, distinctCount)
```

---

### `migrations/000005_donations.up.sql` (migration, CRUD)

**Analog:** `donnarec-api/migrations/000004_receipt_number_tables.up.sql`

**File header pattern** (lines 1-18 of 000004):
```sql
-- migrations/000004_receipt_number_tables.up.sql
-- Phase 2: ...
--
-- Design decisions realized here:
--   D-30: ...
--   D-39: ...
--
-- The application connects as role 'donnarec_app'.
-- This migration:
--   1. Creates ...
--   2. Grants minimal privileges to donnarec_app; REVOKEs UPDATE/DELETE on ...
```

**CREATE TYPE (enum) pattern** (from 000001, lines 10-17):
```sql
CREATE TYPE legal_basis_enum AS ENUM (
    'tax_obligation',
    'consent',
    'legitimate_interest'
);
CREATE TYPE user_role_enum AS ENUM ('maker', 'checker', 'admin');
```
For `000005`: `CREATE TYPE donation_status AS ENUM ('draft', 'pending_review', 'issued', 'rejected', 'cancelled');`

**Table with CHECK constraints pattern** (from 000004, lines 54-62):
```sql
CREATE TABLE receipt_number_counters (
    fiscal_year         INT         NOT NULL,
    last_running_no     INT         NOT NULL DEFAULT 0
                            CHECK (last_running_no >= 0),
    ...
    CONSTRAINT pk_receipt_number_counters PRIMARY KEY (fiscal_year)
);
```
SoD CHECK constraint (from RESEARCH.md pattern 2):
```sql
CONSTRAINT chk_sod_approver
    CHECK (approved_by IS NULL OR approved_by != created_by)
```
Receipt-number-only-on-issued CHECK:
```sql
CONSTRAINT chk_receipt_only_on_issued_or_cancelled
    CHECK (
        (status IN ('issued','cancelled') AND receipt_number_id IS NOT NULL AND receipt_formatted IS NOT NULL)
        OR (status NOT IN ('issued','cancelled') AND receipt_number_id IS NULL AND receipt_formatted IS NULL)
    )
```

**GRANT/REVOKE pattern** (from 000004, lines 96-112):
```sql
GRANT SELECT, INSERT, UPDATE ON donations TO donnarec_app;
-- No DELETE — records are never deleted (FR-19, immutability)
REVOKE DELETE ON donations FROM donnarec_app;
GRANT USAGE, SELECT ON SEQUENCE receipt_numbers_id_seq TO donnarec_app;
```

**ON CONFLICT DO NOTHING seed pattern** (from 000004, lines 45-47):
```sql
INSERT INTO receipt_number_config DEFAULT VALUES
ON CONFLICT (id) DO NOTHING;
```

---

### `internal/db/queries/donations.sql` (utility, CRUD + FOR UPDATE)

**Analog:** `internal/db/queries/receiptno.sql` (FOR UPDATE path) + `internal/db/queries/users.sql` (named params pattern)

**File header pattern** (lines 1-10 of receiptno.sql):
```sql
-- internal/db/queries/receiptno.sql
-- sqlc queries for Phase 2: gap-less receipt number allocator
-- All queries use named @params and explicit column lists (no SELECT * in writes).
-- Parameterized only — no string concatenation (T-02-03 mitigation).
```

**FOR UPDATE query pattern** (lines 18-21 of receiptno.sql):
```sql
-- name: LockCounterForUpdate :one
SELECT last_running_no
FROM receipt_number_counters
WHERE fiscal_year = @fiscal_year
FOR UPDATE;
```
For donations:
```sql
-- name: LockDonationForUpdate :one
SELECT id, status, created_by, receipt_number_id, edonation_keyed
FROM donations
WHERE id = @id
FOR UPDATE;
```

**UPDATE RETURNING pattern** (lines 35-43 of receiptno.sql):
```sql
-- name: IncrementCounter :one
UPDATE receipt_number_counters
SET
    last_running_no = last_running_no + 1,
    updated_at      = now()
WHERE fiscal_year = @fiscal_year
RETURNING last_running_no;
```

**Named @param INSERT RETURNING pattern** (lines 53-59 of receiptno.sql):
```sql
-- name: InsertReceiptNumberLedger :one
INSERT INTO receipt_numbers (fiscal_year, running_no, formatted, allocated_at)
VALUES (@fiscal_year, @running_no, @formatted, now())
RETURNING id, fiscal_year, running_no, formatted, allocated_at;
```

**UPDATE with precondition pattern** (from RESEARCH.md — safe IssueDonation):
```sql
-- name: IssueDonation :exec
UPDATE donations
SET status            = 'issued',
    approved_by       = @approved_by,
    approved_at       = @approved_at,
    receipt_number_id = @receipt_number_id,
    receipt_formatted = @receipt_formatted,
    updated_at        = now()
WHERE id = @id
  AND status = 'pending_review';  -- extra safety precondition
```

**Search query with nullable params pattern** (from RESEARCH.md):
```sql
-- name: SearchDonations :many
SELECT d.id, d.status, d.donor_name, d.donated_at, d.amount,
       d.receipt_formatted, d.created_at, d.approved_at
FROM donations d
WHERE
    (@donor_name::TEXT IS NULL  OR d.donor_name ILIKE '%' || @donor_name || '%')
    AND (@status::donation_status IS NULL OR d.status = @status)
    AND (@from_date::DATE IS NULL OR d.donated_at >= @from_date)
    AND (@to_date::DATE IS NULL   OR d.donated_at <= @to_date)
    AND (@receipt_no::TEXT IS NULL OR d.receipt_formatted = @receipt_no)
ORDER BY d.created_at DESC
LIMIT @limit OFFSET @offset;
```

---

### `internal/config/config.go` (modified, config)

**Analog:** self — `internal/config/config.go`

**Env var field + fallback pattern** (lines 69-82):
```go
cfg := &Config{
    Port:             getEnvStr("PORT", "8000"),
    DatabaseURL:      os.Getenv("DATABASE_URL"),
    KeycloakBaseURL:  keycloakBaseURL,
    DonarecKEK:       os.Getenv("DONAREC_KEK"),
    Retention: RetentionConfig{
        DonationRetainDays: getEnvInt("RETENTION_DONATION_DAYS", 1825),
        DefaultLegalBasis:  getEnvStr("RETENTION_DEFAULT_LEGAL_BASIS", "tax_obligation"),
    },
}
```
Add `MinIO` sub-struct following `RetentionConfig` pattern:
```go
type MinIOConfig struct {
    Endpoint  string
    AccessKey string
    SecretKey string
    Bucket    string
    Secure    bool
}
```
Load from `MINIO_ENDPOINT`, `MINIO_ACCESS_KEY`, `MINIO_SECRET_KEY`, `MINIO_BUCKET`, `MINIO_SECURE`. Add `MINIO_ENDPOINT` to `validate()` required map.

**Validate required fields pattern** (lines 93-108):
```go
func (c *Config) validate() error {
    required := map[string]string{
        "DATABASE_URL":      c.DatabaseURL,
        "KEYCLOAK_BASE_URL": c.KeycloakBaseURL,
        "DONAREC_KEK":       c.DonarecKEK,
    }
    for name, val := range required {
        if val == "" {
            return fmt.Errorf("required environment variable %s is not set", name)
        }
    }
    if len(c.DonarecKEK) != 64 {
        return fmt.Errorf("DONAREC_KEK must be exactly 64 hex characters ...")
    }
    return nil
}
```

---

### `cmd/server/main.go` (modified, config)

**Analog:** self — `cmd/server/main.go`

**Wiring order pattern** (lines 1-183, comment at top):
```go
// Wiring order: pool → queries → services → handlers → router → server.
// All dependencies are constructor-injected; no global state.
```
Phase 3 extends this order:
```
pool → queries
  → storageClient (minio.New from cfg.MinIO)
  → donationSvc  (pool, queries, allocator, auditSvc, keyProvider, storageClient, logger)
  → donationHandler (donationSvc, storageClient, logger)
  → setupRouter (add donation routes)
```

**Router group wiring pattern** (lines 148-180 of main.go):
```go
api := router.Group("/api")
api.Use(authMW.RequireAuth())
api.GET("/me", userHandler.Me)

adminGroup := api.Group("/admin")
adminGroup.Use(auth.RequireRoles(auth.RoleAdmin))
adminGroup.POST("/users", userHandler.CreateUser)
```
For donation routes (copy this group pattern):
```go
// Maker: create/edit/submit/view donations
makerGroup := api.Group("/donations")
makerGroup.Use(auth.RequireRoles(auth.RoleMaker, auth.RoleChecker, auth.RoleAdmin))  // all staff
makerGroup.POST("", donationHandler.Create)
makerGroup.GET("", donationHandler.List)
makerGroup.GET("/:id", donationHandler.GetByID)
makerGroup.PUT("/:id", donationHandler.Update)         // draft only
makerGroup.POST("/:id/submit", donationHandler.Submit)
makerGroup.POST("/:id/slip", donationHandler.UploadSlip)
makerGroup.GET("/:id/pii", donationHandler.RevealPII)  // Checker/Admin gated in service

// Checker/Admin: review actions
checkerGroup := api.Group("/donations")
checkerGroup.Use(auth.RequireRoles(auth.RoleChecker, auth.RoleAdmin))
checkerGroup.POST("/:id/approve", donationHandler.Approve)
checkerGroup.POST("/:id/return", donationHandler.Return)
checkerGroup.POST("/:id/reject", donationHandler.Reject)
checkerGroup.POST("/:id/cancel", donationHandler.Cancel)
```

---

### `internal/storage/client.go` (service, file-I/O)

**No analog** — no object storage exists yet. Use RESEARCH.md Pattern 4 + Pattern 5 as the primary reference.

**Constructor pattern to follow** (mirror `audit.NewAuditService` panic-guard style from audit/service.go lines 88-100):
```go
func NewStorageClient(endpoint, accessKey, secretKey, bucket string, secure bool) (*StorageClient, error) {
    client, err := minio.New(endpoint, &minio.Options{
        Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
        Secure: secure,
    })
    if err != nil {
        return nil, fmt.Errorf("storage: minio client init: %w", err)
    }
    return &StorageClient{client: client, bucket: bucket}, nil
}
```

**Size + magic-byte validation pattern** — use `github.com/gabriel-vasile/mimetype` (already in go.mod as indirect). Pattern from RESEARCH.md section "Pattern 4":
```go
const maxSlipSize = 10 << 20  // 10 MB
// Detect mime type from first bytes, not file extension or Content-Type header
mime := mimetype.Detect(buf[:n])
allowed := map[string]bool{
    "image/jpeg": true, "image/png": true, "application/pdf": true,
}
if !allowed[mime.String()] {
    return "", ErrUnsupportedFileType
}
```

---

## Shared Patterns

### Pattern A: Auth Claims Extraction
**Source:** `donnarec-api/internal/users/handler.go` lines 37-55; `donnarec-api/internal/auth/claims.go` lines 10-60
**Apply to:** All `internal/donation/handler.go` handler functions that act on behalf of a user (create, submit, approve, return, reject, cancel, pii.reveal)

```go
// Extract claims (copy verbatim into each handler)
raw, exists := c.Get("claims")
if !exists {
    c.JSON(http.StatusUnauthorized, gin.H{"error": "missing_auth_context"})
    return
}
claims, ok := raw.(auth.KeycloakClaims)
if !ok {
    c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid_claims_type"})
    return
}

// Use claims.Subject as actor UUID; claims.ActorIdentity() for audit email
// claims.HasRole(auth.RoleChecker) for role checks
// pii.CanRevealFull(claims) for PII reveal gate
```

### Pattern B: WithTx Wrapper
**Source:** `donnarec-api/internal/db/helpers.go` lines 25-40; used in `internal/users/service.go` lines 79-125
**Apply to:** `DonationService.Approve`, `DonationService.Cancel`, `DonationService.Create` (all multi-step mutations)

```go
err := dbhelpers.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
    qtx := s.queries.WithTx(tx)
    // ... all steps ...
    return nil  // commit on nil; any error triggers rollback
})
```
Critical: `tx.Rollback` is deferred inside `WithTx` — callers MUST NOT call `tx.Rollback` themselves. The `defer tx.Rollback(ctx)` is a no-op after `tx.Commit`.

### Pattern C: No PII in Logs
**Source:** `donnarec-api/internal/users/handler.go` line 105; `donnarec-api/internal/users/service.go` line 92
**Apply to:** All `internal/donation/` files — handler, service, storage client

```go
// CORRECT:
s.logger.Error("failed to create donation",
    zap.String("operation", "CreateDonation"),
    zap.String("created_by", creatorID.String()),  // UUID only
    zap.Error(err),
)
// FORBIDDEN:
// zap.String("tax_id", req.DonorTaxID)      -- PII
// zap.String("donor_name", req.DonorName)   -- PII
// err.Error() containing plaintext tax ID   -- log sanitize
```

### Pattern D: AuditMiddleware + AppendAuditEntryTx
**Source:** `donnarec-api/internal/audit/service.go` lines 129-227; `donnarec-api/cmd/server/main.go` lines 157-160
**Apply to:** Every state-changing action in `DonationService` — approve, return, reject, cancel, pii.reveal

Two different audit paths exist — use the CORRECT one per context:

```go
// WRONG (best-effort, post-commit — for middleware only):
s.auditSvc.AppendAuditEntry(ctx, entry)

// CORRECT for issuance / cancel / pii.reveal (atomic with data mutation):
s.auditSvc.AppendAuditEntryTx(ctx, tx, audit.AuditEntry{
    ActorID:    claims.Subject,
    ActorEmail: claims.ActorIdentity(),
    Action:     "donation.approve",     // or "donation.cancel", "pii.reveal"
    Resource:   "/api/donations/" + donationID.String() + "/approve",
    AfterJSON:  marshalJSON(map[string]any{"receipt": receipt.Formatted}),
})
```
`AppendAuditEntryTx` acquires `pg_advisory_xact_lock(auditChainLockKey)` internally — no extra locking needed by caller.

### Pattern E: RequireRoles Middleware at Router Level
**Source:** `donnarec-api/internal/auth/rbac.go` lines 33-62; `donnarec-api/cmd/server/main.go` lines 175-179
**Apply to:** All route registrations in updated `cmd/server/main.go` for donation routes

```go
// Role constants — never bare strings:
auth.RoleMaker   // "maker"
auth.RoleChecker // "checker"
auth.RoleAdmin   // "admin"

// Middleware must come AFTER RequireAuth() in chain (claims must exist):
adminGroup := api.Group("/admin")
adminGroup.Use(auth.RequireRoles(auth.RoleAdmin))  // logical AND for multiple roles
```
SoD is NOT enforced by `RequireRoles` — it is a separate per-record check (`approver_id != created_by`) in `DonationService.Approve`. Both layers are required (CLAUDE.md defense-in-depth).

### Pattern F: sqlc Named Params + Explicit Column Lists
**Source:** `donnarec-api/internal/db/queries/receiptno.sql` lines 1-10; `donnarec-api/internal/db/queries/users.sql` lines 1-5
**Apply to:** All new `.sql` files under `internal/db/queries/`

```sql
-- Rules (from existing query files):
-- 1. Always use @param_name (not $1 positional) for sqlc named params
-- 2. Always name queries: -- name: QueryName :one/:many/:exec
-- 3. No bare SELECT * in INSERT/UPDATE; use explicit column list
-- 4. created_at/updated_at omitted from INSERT VALUES — rely on DEFAULT now()
-- 5. No string concatenation — all parameterized
-- 6. FOR UPDATE queries: named :one, returns the locked columns needed for precondition checks
```

### Pattern G: Migration Structure
**Source:** `donnarec-api/migrations/000004_receipt_number_tables.up.sql` lines 1-112
**Apply to:** All `migrations/000005+` files

```sql
-- File header: document decisions (D-xx) + responsibilities
-- Structure: 1. Enums → 2. Tables with CHECK constraints → 3. Indexes → 4. GRANT → 5. REVOKE
-- Always: GRANT SELECT, INSERT, UPDATE TO donnarec_app
-- Immutable tables: REVOKE UPDATE, DELETE FROM donnarec_app (donations, slip_attachments)
-- Sequences: GRANT USAGE, SELECT ON SEQUENCE <table>_id_seq TO donnarec_app
-- Seed data: always ON CONFLICT (...) DO NOTHING for idempotency
```

### Pattern H: testutil.SetupTestPostgres
**Source:** `donnarec-api/internal/testutil/postgres.go` lines 25-70
**Apply to:** All `*_integration_test.go` files in `internal/donation/` and `internal/storage/`

```go
func TestSomething(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test in short mode")
    }
    pool := testutil.SetupTestPostgres(t)  // spins Postgres 17, runs all migrations, auto-cleanup
    ctx := context.Background()
    queries := db.New(pool)
    // ... test body ...
}
```
Migration path in testutil is `file://../../migrations` (relative from `internal/<package>/`). New queries in `internal/db/queries/donations.sql` are picked up automatically after `sqlc generate`.

---

## No Analog Found

| File | Role | Data Flow | Reason |
|------|------|-----------|--------|
| `internal/storage/client.go` | service | file-I/O | No object storage client exists in codebase; use RESEARCH.md Pattern 4+5 |
| `donnarec-web/` (entire frontend) | component | request-response | No Next.js project exists; bootstrap from scratch with `npx create-next-app@latest` (Next.js 15, App Router, TypeScript, Tailwind) then `npx shadcn@latest init` per UI-SPEC.md |

---

## Critical Wiring Notes for Planner

### Issuance Transaction — Exact Sequence
The `DonationService.Approve` method is the load-bearing function. Inside one `dbhelpers.WithTx` closure, in this order:
1. `qtx.LockDonationForUpdate(ctx, donationID)` — D-52, serializes concurrent approvals
2. Check `donation.Status == "pending_review"` → `ErrInvalidTransition` if not
3. Check `donation.CreatedBy != approverID` → `ErrSoDViolation` if equal
4. `s.allocator.Allocate(ctx, tx, time.Now())` — D-33, pass the closure's `tx` NOT pool
5. `qtx.IssueDonation(ctx, ...)` — stamps status=issued + receipt fields
6. `s.auditSvc.AppendAuditEntryTx(ctx, tx, ...)` — audit inside tx (NFR-05)
7. `qtx.EnqueueOutboxJob(ctx, ...)` — outbox INSERT in same tx (atomically linked)

If any step returns an error, `WithTx` rolls back all 7 effects. This is the ONLY place where `Allocate` is ever called (D-35).

### Allocator — Never Call Outside Issuance tx
`receiptno.Allocator.Allocate` signature: `func (a *Allocator) Allocate(ctx context.Context, tx pgx.Tx, issueDate time.Time) (AllocatedReceipt, error)` (allocator.go lines 93-189).

The `tx` parameter must be the exact `pgx.Tx` from the `WithTx` closure. Passing `nil` or a separate `pool.Begin` tx violates D-33 and D-35 — the UNIQUE backstop and counter rollback-safety depend on counter + ledger being in the same tx as the donation status update.

### Crypto KeyProvider — Wire from Config
`crypto.EncryptField` and `crypto.DecryptField` require a `crypto.KeyProvider`. The existing implementation is `crypto.NewEnvKeyProvider(cfg.DonarecKEK)` (from `internal/crypto/envprovider.go`). `DonationService` receives this via constructor injection from `main.go`.

---

## Metadata

**Analog search scope:** `donnarec-api/internal/`, `donnarec-api/migrations/`, `donnarec-api/internal/db/queries/`, `donnarec-api/cmd/`
**Files scanned:** 28 Go source files + 4 migration files + 4 SQL query files
**Pattern extraction date:** 2026-06-28
