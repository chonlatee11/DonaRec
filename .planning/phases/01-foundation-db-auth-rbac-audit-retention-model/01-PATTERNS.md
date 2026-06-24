# Phase 1: Foundation (DB, Auth/RBAC, Audit, Retention model) - Pattern Map

**Mapped:** 2026-06-23
**Files analyzed:** 27 (new files — greenfield scaffold)
**Analogs found:** 0 / 27 (greenfield — no in-repo source code exists)

---

## GREENFIELD NOTICE

โปรเจกต์นี้เป็น **greenfield** อย่างสมบูรณ์ — ไม่มีซอร์สโค้ดใดๆ ในเลย
(ตรวจสอบแล้วด้วย `find . -name "*.go" -o -name "*.ts" -o -name "*.py"` → ไม่มีผลลัพธ์)
มีเฉพาะไฟล์ planning (`.planning/`) + `CLAUDE.md` + `README.md`

Phase 1 คือ **การ scaffold ระบบครั้งแรก** — pattern ที่กำหนดที่นี่จะกลายเป็น
**foundational pattern ให้ทุกเฟสถัดไป (Phase 2–6) อ้างอิง**

ทุก "Analog" ที่ระบุด้านล่างคือ **canonical external pattern** จาก official docs /
library conventions — ไม่ใช่ไฟล์ในโปรเจกต์

---

## File Classification

| New File | Role | Data Flow | Canonical External Pattern | Match Quality |
|----------|------|-----------|---------------------------|---------------|
| `donnarec-api/cmd/server/main.go` | entrypoint | request-response | Standard Go `cmd/` layout (golang-standards/project-layout) | canonical |
| `donnarec-api/go.mod` | config | — | Go modules convention (`go mod init`) | canonical |
| `donnarec-api/docker-compose.yml` | config | — | Docker Compose multi-service pattern | canonical |
| `donnarec-api/internal/auth/middleware.go` | middleware | request-response | `coreos/go-oidc/v3` OIDC verifier pattern (RESEARCH.md Pattern 1) | canonical |
| `donnarec-api/internal/auth/claims.go` | model | — | golang-jwt/jwt v5 custom claims struct | canonical |
| `donnarec-api/internal/auth/rbac.go` | middleware | request-response | Gin middleware factory function pattern (RESEARCH.md Pattern 2) | canonical |
| `donnarec-api/internal/audit/middleware.go` | middleware | event-driven | Gin response-interceptor middleware pattern (RESEARCH.md Pattern 3) | canonical |
| `donnarec-api/internal/audit/service.go` | service | event-driven | SHA-256 hash-chain compute pattern (RESEARCH.md Pattern 4) | canonical |
| `donnarec-api/internal/audit/query.sql` | — | CRUD | sqlc `.sql` file convention (docs.sqlc.dev) | canonical |
| `donnarec-api/internal/crypto/keyprovider.go` | utility | — | Envelope encryption KeyProvider interface (RESEARCH.md Pattern 5) | canonical |
| `donnarec-api/internal/crypto/envprovider.go` | utility | — | EnvKeyProvider MVP implementation pattern | canonical |
| `donnarec-api/internal/crypto/aes_gcm.go` | utility | — | Go stdlib `crypto/cipher` AES-256-GCM pattern | canonical |
| `donnarec-api/internal/i18n/bundle.go` | utility | — | go-i18n v2 `NewBundle` + `NewLocalizer` pattern (RESEARCH.md Code Examples) | canonical |
| `donnarec-api/internal/i18n/locales/th.json` | config | — | go-i18n v2 JSON message catalog format | canonical |
| `donnarec-api/internal/i18n/locales/en.json` | config | — | go-i18n v2 JSON message catalog format | canonical |
| `donnarec-api/internal/users/handler.go` | controller | request-response | Gin handler function + `c.ShouldBindJSON` + validator pattern | canonical |
| `donnarec-api/internal/users/service.go` | service | CRUD | Go constructor-injection service pattern (`NewUserService(pool, queries)`) | canonical |
| `donnarec-api/internal/users/query.sql` | — | CRUD | sqlc `.sql` annotated query file (`-- name: CreateUser :one`) | canonical |
| `donnarec-api/internal/config/retention.go` | config | — | Go struct with env-var defaults (`os.Getenv` + fallback) | canonical |
| `donnarec-api/internal/retention/service.go` | service | CRUD | Go service with DB trigger defense-in-depth (RESEARCH.md Pattern 7) | canonical |
| `donnarec-api/internal/db/sqlc.yaml` | config | — | sqlc v2 config schema (docs.sqlc.dev) | canonical |
| `donnarec-api/internal/db/generated/` | — | — | sqlc codegen output (`db.New(pool)`, `Queries`, `Querier` interface) | canonical |
| `donnarec-api/migrations/000001_init_schema.up.sql` | migration | — | golang-migrate numbered SQL file convention | canonical |
| `donnarec-api/migrations/000001_init_schema.down.sql` | migration | — | golang-migrate down migration convention | canonical |
| `donnarec-api/internal/testutil/postgres.go` | test | — | testcontainers-go `PostgresContainer` setup pattern | canonical |
| `donnarec-web/src/messages/th.json` | config | — | next-intl JSON message catalog (next-intl.dev App Router docs) | canonical |
| `donnarec-web/src/messages/en.json` | config | — | next-intl JSON message catalog | canonical |

---

## Pattern Assignments

### `donnarec-api/cmd/server/main.go` (entrypoint, request-response)

**Canonical Pattern:** Standard Go project layout — `cmd/<binary>/main.go` เป็น thin entrypoint
**External Source:** https://github.com/golang-standards/project-layout

**Pattern to copy:**
```go
// cmd/server/main.go
// - อ่าน config จาก env (ไม่ใช้ flags ใน MVP)
// - wire dependencies ด้วย constructor injection (ไม่มี DI framework)
// - เรียก setupRouter() แล้ว srv.ListenAndServe()
// - graceful shutdown ด้วย signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)

package main

import (
    "context"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "go.uber.org/zap"
)

func main() {
    logger, _ := zap.NewProduction()
    defer logger.Sync()

    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()

    // wire: pool → queries → services → handlers → router
    // srv := &http.Server{Addr: ":8000", Handler: setupRouter(...)}
    // go srv.ListenAndServe()
    // <-ctx.Done()
    // srv.Shutdown(context.Background())
}
```

**Key rules:**
- ไม่มี global state — ทุก dependency pass ผ่าน constructor
- Logger inject ทุก layer (ไม่ใช้ `log.Println`)
- Port/config อ่านจาก env (`os.Getenv("PORT")`) ไม่ hardcode

---

### `donnarec-api/internal/auth/middleware.go` (middleware, request-response)

**Canonical Pattern:** `coreos/go-oidc/v3` OIDC token validation middleware
**External Source:** RESEARCH.md Pattern 1 (lines 294–343) + https://pkg.go.dev/github.com/coreos/go-oidc/v3/oidc

**Imports pattern:**
```go
import (
    "context"
    "fmt"
    "net/http"
    "strings"

    "github.com/coreos/go-oidc/v3/oidc"
    "github.com/gin-gonic/gin"
    "go.uber.org/zap"
)
```

**Core middleware pattern:**
```go
type AuthMiddleware struct {
    verifier *oidc.IDTokenVerifier
    logger   *zap.Logger
}

func NewAuthMiddleware(keycloakBaseURL, realm, clientID string, logger *zap.Logger) (*AuthMiddleware, error) {
    ctx := context.Background()
    providerURL := fmt.Sprintf("%s/realms/%s", keycloakBaseURL, realm)
    provider, err := oidc.NewProvider(ctx, providerURL) // auto-fetches JWKS via discovery
    if err != nil {
        return nil, fmt.Errorf("oidc provider init: %w", err)
    }
    verifier := provider.Verifier(&oidc.Config{ClientID: clientID})
    return &AuthMiddleware{verifier: verifier, logger: logger}, nil
}

func (m *AuthMiddleware) RequireAuth() gin.HandlerFunc {
    return func(c *gin.Context) {
        rawToken := extractBearerToken(c)
        if rawToken == "" {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing_token"})
            return
        }
        idToken, err := m.verifier.Verify(c.Request.Context(), rawToken)
        if err != nil {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid_token"})
            return
        }
        var claims KeycloakClaims
        if err := idToken.Claims(&claims); err != nil {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "claims_parse_error"})
            return
        }
        c.Set("claims", claims)
        c.Next()
    }
}

func extractBearerToken(c *gin.Context) string {
    h := c.GetHeader("Authorization")
    if !strings.HasPrefix(h, "Bearer ") {
        return ""
    }
    return strings.TrimPrefix(h, "Bearer ")
}
```

**Critical pitfalls (จาก RESEARCH.md Pitfall 1 & 3):**
- ตรวจ `iss` claim ต้อง match `providerURL` (go-oidc ทำให้อัตโนมัติ)
- ตรวจ `aud` claim ต้อง include `clientID` (pass `ClientID` ใน `oidc.Config`)
- ห้าม extract roles จาก top-level `roles` — ต้องใช้ `realm_access.roles` (Pitfall 1)

---

### `donnarec-api/internal/auth/claims.go` (model)

**Canonical Pattern:** golang-jwt/jwt v5 custom claims + Keycloak nested role structure
**External Source:** RESEARCH.md Pattern 1 (lines 336–343)

**Pattern to copy:**
```go
// KeycloakClaims maps Keycloak JWT nested structure
// ห้ามใช้ top-level "roles" field — Keycloak วาง roles ใน realm_access.roles เสมอ
type KeycloakClaims struct {
    Subject     string     `json:"sub"`
    Email       string     `json:"email"`
    RealmAccess RealmRoles `json:"realm_access"`
    // resource_access สำหรับ client roles (future)
}

type RealmRoles struct {
    Roles []string `json:"roles"`
}

// HasRole: helper สำหรับ RBAC guard
func (kc KeycloakClaims) HasRole(role string) bool {
    for _, r := range kc.RealmAccess.Roles {
        if r == role {
            return true
        }
    }
    return false
}
```

---

### `donnarec-api/internal/auth/rbac.go` (middleware, request-response)

**Canonical Pattern:** Gin middleware factory returning `gin.HandlerFunc`
**External Source:** RESEARCH.md Pattern 2 (lines 352–373)

**Core pattern:**
```go
// RequireRoles: middleware factory — ใช้เป็น router.Use() หรือ route-level
func RequireRoles(requiredRoles ...string) gin.HandlerFunc {
    return func(c *gin.Context) {
        raw, exists := c.Get("claims")
        if !exists {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "no_auth_context"})
            return
        }
        kc := raw.(KeycloakClaims)
        for _, r := range requiredRoles {
            if !kc.HasRole(r) {
                c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "insufficient_role"})
                return
            }
        }
        c.Next()
    }
}

// Role constants — ใช้ constant แทน string literal ทุกที่
const (
    RoleMaker   = "maker"
    RoleChecker = "checker"
    RoleAdmin   = "admin"
)

// SoD stub สำหรับ Phase 3 (ไม่ implement Phase 1 แต่ลาย interface ไว้)
// func RequireNotCreator(resourceCreatorID string) gin.HandlerFunc { ... }
```

---

### `donnarec-api/internal/audit/middleware.go` (middleware, event-driven)

**Canonical Pattern:** Gin post-handler audit interceptor
**External Source:** RESEARCH.md Pattern 3 (lines 382–402)

**Core pattern:**
```go
// AuditMiddleware: ครอบทุก mutation; เรียก auditSvc หลัง handler ทำงานเสร็จ
func AuditMiddleware(auditSvc *AuditService) gin.HandlerFunc {
    return func(c *gin.Context) {
        // skip read-only requests ที่ไม่ใช่ PII reveal
        if c.Request.Method == http.MethodGet && !isPIIRevealEndpoint(c) {
            c.Next()
            return
        }
        // capture actor ก่อน handler ทำงาน
        claims, _ := c.Get("claims")

        c.Next() // execute handler first

        // หลัง handler: เขียน audit (ใน goroutine ห้าม — ต้อง synchronous หรือ same tx)
        entry := AuditEntry{
            Actor:     extractActor(claims),
            Action:    deriveAction(c),       // เช่น "user.create", "pii.reveal"
            Resource:  c.FullPath(),
            IPAddress: c.ClientIP(),
            StatusCode: c.Writer.Status(),
            Timestamp: time.Now().UTC(),
        }
        // before/after snapshots: handler เซต ผ่าน c.Set("audit_before", ...) / c.Set("audit_after", ...)
        if before, ok := c.Get("audit_before"); ok { entry.BeforeJSON = before }
        if after, ok := c.Get("audit_after"); ok { entry.AfterJSON = after }

        if err := auditSvc.AppendAuditEntry(c.Request.Context(), entry); err != nil {
            // log error แต่ห้าม abort — audit failure ไม่ควรทำ user request fail
            // (แต่ alert ควร trigger)
        }
    }
}
```

**Critical rule (RESEARCH.md Anti-Patterns):**
- ห้ามเขียน audit ใน goroutine แยก หรือ `defer` นอก transaction
- Audit entry ต้องเขียนใน transaction เดียวกับ data mutation เสมอ

---

### `donnarec-api/internal/audit/service.go` (service, event-driven)

**Canonical Pattern:** SHA-256 hash-chain compute + PostgreSQL advisory lock
**External Source:** RESEARCH.md Pattern 4 (lines 434–444) + Pitfall 2 (advisory lock)

**Core hash-chain pattern:**
```go
import (
    "crypto/sha256"
    "encoding/hex"
    "fmt"
    "time"
)

// computeRowHash: SHA-256(id||actor_id||action||resource||created_at||prev_hash)
func computeRowHash(entry AuditEntry, prevHash string) string {
    h := sha256.New()
    fmt.Fprintf(h, "%d|%s|%s|%s|%s|%s",
        entry.ID,
        entry.ActorID,
        entry.Action,
        entry.Resource,
        entry.CreatedAt.Format(time.RFC3339Nano),
        prevHash,
    )
    return hex.EncodeToString(h.Sum(nil))
}

// AppendAuditEntry: ใช้ SELECT FOR UPDATE บน audit_sequence เพื่อป้องกัน race condition
// (RESEARCH.md Open Question 2 recommendation)
func (s *AuditService) AppendAuditEntry(ctx context.Context, entry AuditEntry) error {
    return pgx_helpers.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
        // 1. lock audit_sequence row (SELECT FOR UPDATE) — serializes concurrent inserts
        var prevHash string
        err := tx.QueryRow(ctx,
            `SELECT row_hash FROM audit_log ORDER BY id DESC LIMIT 1 FOR UPDATE`).
            Scan(&prevHash)
        if err != nil { prevHash = "GENESIS" } // first row

        // 2. compute hash
        entry.PrevHash = prevHash
        // entry.ID จะได้จาก RETURNING หลัง INSERT
        // entry.RowHash = computeRowHash(entry, prevHash) — set หลังได้ ID

        // 3. INSERT
        return s.queries.WithTx(tx).InsertAuditLog(ctx, entry)
    })
}
```

---

### `donnarec-api/internal/audit/query.sql` (sqlc query file, CRUD)

**Canonical Pattern:** sqlc annotated SQL query file convention
**External Source:** https://docs.sqlc.dev/en/latest/tutorials/getting-started-postgresql.html

**Pattern to copy:**
```sql
-- name: InsertAuditLog :one
INSERT INTO audit_log (
    actor_id, actor_email, action, resource,
    before_json, after_json, ip_address,
    prev_hash, row_hash
) VALUES (
    @actor_id, @actor_email, @action, @resource,
    @before_json, @after_json, @ip_address,
    @prev_hash, @row_hash
) RETURNING *;

-- name: ListAuditLogs :many
-- Admin only
SELECT * FROM audit_log
WHERE (@actor_id::uuid IS NULL OR actor_id = @actor_id)
ORDER BY created_at DESC
LIMIT @limit_n OFFSET @offset_n;
```

**Key rule:** ทุก query ต้องมี `-- name: <Name> :<return_type>` annotation
ห้ามใช้ `*` ใน INSERT columns (เขียน explicit เสมอ)

---

### `donnarec-api/internal/crypto/keyprovider.go` (utility)

**Canonical Pattern:** Go interface สำหรับ KMS abstraction (envelope encryption)
**External Source:** RESEARCH.md Pattern 5 (lines 452–487) + https://www.lambrospetrou.com/articles/encryption/

**Interface pattern:**
```go
// KeyProvider: swap implementation โดยไม่แก้ call site
// MVP = EnvKeyProvider; Future = CloudKMSProvider (AWS KMS / GCP KMS / HashiCorp Vault)
type KeyProvider interface {
    WrapKey(ctx context.Context, plaintextDEK []byte) ([]byte, error)
    UnwrapKey(ctx context.Context, wrappedDEK []byte) ([]byte, error)
}
```

**Critical rule:** Interface นี้เป็น boundary ที่ Phase 3 และ Phase 6 ใช้จริง
ห้ามเปลี่ยน signature หลังจาก Phase 1 commit แล้ว ถ้าต้องการ capability ใหม่ให้ embed interface แทน

---

### `donnarec-api/internal/crypto/envprovider.go` (utility)

**Canonical Pattern:** MVP KeyProvider implementation ใช้ env var เก็บ KEK
**External Source:** RESEARCH.md Pattern 5 + Pitfall 5

```go
// EnvKeyProvider: KEK มาจาก env var DONAREC_KEK (hex-encoded 32 bytes)
// ห้าม hardcode KEK ใน source code เด็ดขาด
type EnvKeyProvider struct {
    kek []byte // 32-byte AES-256 key
}

func NewEnvKeyProvider() (*EnvKeyProvider, error) {
    kekHex := os.Getenv("DONAREC_KEK")
    if kekHex == "" {
        return nil, fmt.Errorf("DONAREC_KEK environment variable not set")
    }
    kek, err := hex.DecodeString(kekHex)
    if err != nil || len(kek) != 32 {
        return nil, fmt.Errorf("DONAREC_KEK must be 32-byte hex string")
    }
    return &EnvKeyProvider{kek: kek}, nil
}

// WrapKey / UnwrapKey ใช้ AES-256-GCM จาก crypto/cipher (stdlib เท่านั้น)
```

---

### `donnarec-api/internal/crypto/aes_gcm.go` (utility)

**Canonical Pattern:** Go stdlib `crypto/cipher` AES-256-GCM — ห้ามใช้ external crypto library
**External Source:** https://pkg.go.dev/crypto/cipher + RESEARCH.md "Don't Hand-Roll" table

```go
import (
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "fmt"
    "io"
    "crypto/hmac"
    "crypto/sha256"
    "golang.org/x/crypto/argon2" // ถ้าต้องการ key derivation
)

// Encrypt: AES-256-GCM authenticated encryption
// Output: nonce || ciphertext (nonce prefix convention)
func Encrypt(key, plaintext []byte) ([]byte, error) {
    block, err := aes.NewCipher(key)
    if err != nil {
        return nil, fmt.Errorf("aes cipher: %w", err)
    }
    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return nil, fmt.Errorf("gcm: %w", err)
    }
    nonce := make([]byte, gcm.NonceSize())
    if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
        return nil, fmt.Errorf("nonce: %w", err)
    }
    return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// Decrypt: แยก nonce ออกจาก ciphertext prefix แล้ว Open
func Decrypt(key, ciphertext []byte) ([]byte, error) { ... }

// BlindIndex: HMAC-SHA256 สำหรับ searchable encryption (Phase 3)
// ใช้ index key แยกจาก DEK เสมอ
func BlindIndex(plaintext, indexKey []byte) []byte {
    mac := hmac.New(sha256.New, indexKey)
    mac.Write(plaintext)
    return mac.Sum(nil)
}
```

---

### `donnarec-api/internal/i18n/bundle.go` (utility)

**Canonical Pattern:** go-i18n v2 `NewBundle` + `NewLocalizer` per-request
**External Source:** RESEARCH.md Code Examples (lines 698–707) + https://pkg.go.dev/github.com/nicksnyder/go-i18n/v2/i18n

```go
import (
    "encoding/json"
    "github.com/nicksnyder/go-i18n/v2/i18n"
    "golang.org/x/text/language"
)

// SetupBundle: เรียก 1 ครั้งตอน startup — bundle เป็น global read-only state
func SetupBundle(localesDir string) (*i18n.Bundle, error) {
    bundle := i18n.NewBundle(language.Thai) // Thai เป็น default locale
    bundle.RegisterUnmarshalFunc("json", json.Unmarshal)
    for _, lang := range []string{"th", "en"} {
        if _, err := bundle.LoadMessageFile(filepath.Join(localesDir, lang+".json")); err != nil {
            return nil, fmt.Errorf("load %s messages: %w", lang, err)
        }
    }
    return bundle, nil
}

// NewLocalizer: เรียกต่อ request (อ่าน Accept-Language header)
// ใช้ใน Gin handler หรือ middleware:
// localizer := i18n.NewLocalizer(bundle, c.GetHeader("Accept-Language"), "th")
// msg := localizer.MustLocalize(&i18n.LocalizeConfig{MessageID: "auth.invalid_password"})
```

**Key rule (RESEARCH.md Pitfall 6):**
ทุก error response ต้องใช้ message key เสมอ ห้าม hardcode string ใน handler
```go
// WRONG:
c.JSON(400, gin.H{"error": "รหัสผ่านไม่ถูกต้อง"})
// CORRECT:
c.JSON(400, gin.H{"error": localizer.MustLocalize(&i18n.LocalizeConfig{MessageID: "auth.invalid_password"})})
```

---

### `donnarec-api/internal/i18n/locales/th.json` และ `en.json` (config)

**Canonical Pattern:** go-i18n v2 JSON message catalog format
**External Source:** https://pkg.go.dev/github.com/nicksnyder/go-i18n/v2/i18n#hdr-Message_Files

```json
// th.json — Phase 1 initial catalog (auth + validation scope เท่านั้น)
{
    "auth.missing_token": {
        "other": "ไม่พบ token การยืนยันตัวตน"
    },
    "auth.invalid_token": {
        "other": "token ไม่ถูกต้องหรือหมดอายุ"
    },
    "auth.insufficient_role": {
        "other": "ไม่มีสิทธิ์เข้าถึง"
    },
    "auth.invalid_password": {
        "other": "รหัสผ่านไม่ถูกต้อง"
    },
    "retention.legal_hold_delete_blocked": {
        "other": "ไม่สามารถลบข้อมูลที่อยู่ภายใต้ legal hold"
    }
}
```

**Key convention:** `<domain>.<action>` format สำหรับ message key (ไม่ใช่ sentence)

---

### `donnarec-api/internal/users/handler.go` (controller, request-response)

**Canonical Pattern:** Gin handler function pattern — bind, validate, call service, respond
**External Source:** https://gin-gonic.com/docs/ + go-playground/validator struct tags

```go
// handler function signature: รับ *gin.Context เท่านั้น
// dependency inject ผ่าน closure (struct method หรือ closure capture)

type UserHandler struct {
    svc    *UserService
    bundle *i18n.Bundle
    logger *zap.Logger
}

func (h *UserHandler) CreateUser(c *gin.Context) {
    localizer := i18n.NewLocalizer(h.bundle, c.GetHeader("Accept-Language"), "th")

    var req CreateUserRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        // ใช้ validator error → i18n message key
        c.JSON(http.StatusBadRequest, gin.H{"error": translateValidationError(localizer, err)})
        return
    }

    // claims inject โดย auth middleware แล้ว
    claims := c.MustGet("claims").(KeycloakClaims)

    result, err := h.svc.CreateUser(c.Request.Context(), claims, req)
    if err != nil {
        handleServiceError(c, localizer, err)
        return
    }

    c.Set("audit_after", result) // สำหรับ audit middleware
    c.JSON(http.StatusCreated, gin.H{"data": result})
}

// handleServiceError: centralized error → HTTP status mapping
// ห้าม leak internal errors ไปยัง client
func handleServiceError(c *gin.Context, localizer *i18n.Localizer, err error) {
    var appErr *AppError
    if errors.As(err, &appErr) {
        c.JSON(appErr.StatusCode, gin.H{"error": localizer.MustLocalize(...)})
        return
    }
    c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_server_error"})
}
```

---

### `donnarec-api/internal/users/service.go` (service, CRUD)

**Canonical Pattern:** Go constructor-injection service — ไม่มี DI framework
**External Source:** RESEARCH.md "Key insight" (line 593)

```go
// UserService: รับ dependencies ผ่าน constructor
type UserService struct {
    pool     *pgxpool.Pool
    queries  *db.Queries   // sqlc-generated
    kp       crypto.KeyProvider
    auditSvc *audit.AuditService
    logger   *zap.Logger
}

func NewUserService(
    pool *pgxpool.Pool,
    queries *db.Queries,
    kp crypto.KeyProvider,
    auditSvc *audit.AuditService,
    logger *zap.Logger,
) *UserService {
    return &UserService{pool: pool, queries: queries, kp: kp, auditSvc: auditSvc, logger: logger}
}

// CreateUser: ตรวจ Admin role → create user → audit (ใน transaction เดียว)
func (s *UserService) CreateUser(ctx context.Context, actor KeycloakClaims, req CreateUserRequest) (*User, error) {
    if !actor.HasRole(RoleAdmin) {
        return nil, &AppError{StatusCode: 403, Code: "insufficient_role"}
    }
    // transaction: create + audit
    var result *User
    err := pgx_helpers.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
        qtx := s.queries.WithTx(tx)
        user, err := qtx.CreateUser(ctx, db.CreateUserParams{...})
        if err != nil { return err }
        result = mapToUser(user)
        // audit ใน transaction เดียวกัน (RESEARCH.md Anti-Patterns)
        return s.auditSvc.AppendAuditEntryTx(ctx, tx, AuditEntry{...})
    })
    return result, err
}
```

---

### `donnarec-api/internal/users/query.sql` (sqlc query file, CRUD)

**Canonical Pattern:** sqlc PostgreSQL query with explicit column list
**External Source:** https://docs.sqlc.dev/en/latest/tutorials/getting-started-postgresql.html

```sql
-- name: CreateUser :one
INSERT INTO users (
    id, email, display_name, keycloak_subject,
    is_active, created_at, updated_at
) VALUES (
    gen_random_uuid(), @email, @display_name, @keycloak_subject,
    true, now(), now()
) RETURNING *;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = @id AND is_active = true;

-- name: ListUsers :many
SELECT id, email, display_name, is_active, created_at
FROM users
ORDER BY created_at DESC
LIMIT @limit_n OFFSET @offset_n;

-- name: AssignRole :one
INSERT INTO user_roles (user_id, role)
VALUES (@user_id, @role)
ON CONFLICT (user_id, role) DO NOTHING
RETURNING *;
```

---

### `donnarec-api/internal/config/retention.go` (config)

**Canonical Pattern:** Go config struct with env var fallback defaults
**External Source:** Standard Go env config pattern (12-factor app)

```go
type RetentionConfig struct {
    DonationRetainDays int    // default: 1825 (5 ปี) — รอ DPO ยืนยัน (RESEARCH.md A2)
    AuditLogRetainDays int    // default: 3650 (10 ปี) — tax audit requirement
    DefaultLegalBasis  string // default: "tax_obligation"
}

func LoadRetentionConfig() RetentionConfig {
    return RetentionConfig{
        DonationRetainDays: getEnvInt("RETENTION_DONATION_DAYS", 1825),
        AuditLogRetainDays: getEnvInt("RETENTION_AUDIT_DAYS", 3650),
        DefaultLegalBasis:  getEnvStr("RETENTION_DEFAULT_LEGAL_BASIS", "tax_obligation"),
    }
}

func getEnvInt(key string, fallback int) int {
    if v := os.Getenv(key); v != "" {
        if i, err := strconv.Atoi(v); err == nil { return i }
    }
    return fallback
}
```

---

### `donnarec-api/internal/db/sqlc.yaml` (config)

**Canonical Pattern:** sqlc v2 config schema
**External Source:** RESEARCH.md Code Examples (lines 677–692) + https://docs.sqlc.dev/en/stable/reference/config.html

```yaml
version: "2"
sql:
  - engine: "postgresql"
    queries: "./queries"          # .sql files ที่มี -- name: annotations
    schema: "./migrations"        # golang-migrate .up.sql files
    gen:
      go:
        package: "db"
        out: "./generated"
        sql_package: "pgx/v5"         # ใช้ pgx/v5 (ห้ามใช้ database/sql)
        emit_interface: true           # Querier interface สำหรับ mock testing
        emit_json_tags: true
        emit_pointers_for_null_types: true
        emit_db_tags: true
```

---

### `donnarec-api/migrations/000001_init_schema.up.sql` (migration)

**Canonical Pattern:** golang-migrate numbered SQL migration convention
**External Source:** https://github.com/golang-migrate/migrate + RESEARCH.md Pattern 4 & 7

**Naming convention:** `{seq:6digits}_{description}.{up|down}.sql`

**Structure pattern:**
```sql
-- migrations/000001_init_schema.up.sql
-- Phase 1 foundation tables (เรียงตาม dependency order)

-- 1. Enums ก่อน (PostgreSQL ต้องสร้าง type ก่อน table ที่ใช้)
CREATE TYPE legal_basis_enum AS ENUM (
    'tax_obligation',
    'consent',
    'legitimate_interest'
);

CREATE TYPE user_role_enum AS ENUM ('maker', 'checker', 'admin');

-- 2. Core tables
CREATE TABLE users (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    email             TEXT        NOT NULL UNIQUE,
    display_name      TEXT        NOT NULL,
    keycloak_subject  TEXT        NOT NULL UNIQUE, -- Keycloak 'sub' claim
    is_active         BOOLEAN     NOT NULL DEFAULT true,
    -- retention fields
    legal_hold        BOOLEAN     NOT NULL DEFAULT false,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE user_roles (
    user_id  UUID           NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role     user_role_enum NOT NULL,
    PRIMARY KEY (user_id, role)
);

-- 3. Retention config table
CREATE TABLE retention_config (
    id                  SERIAL      PRIMARY KEY,
    entity_type         TEXT        NOT NULL UNIQUE,
    default_retain_days INT         NOT NULL,
    legal_basis         legal_basis_enum NOT NULL,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by          UUID        NOT NULL REFERENCES users(id)
);

-- 4. Audit log (append-only, hash-chained) — RESEARCH.md Pattern 4
CREATE TABLE audit_log (
    id          BIGSERIAL   PRIMARY KEY,
    actor_id    UUID        NOT NULL,
    actor_email TEXT        NOT NULL,
    action      TEXT        NOT NULL,
    resource    TEXT        NOT NULL,
    before_json JSONB,
    after_json  JSONB,
    ip_address  INET,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    prev_hash   TEXT        NOT NULL,
    row_hash    TEXT        NOT NULL
);

-- 5. Revoke UPDATE/DELETE จาก app role (audit immutability — D-17)
-- REVOKE UPDATE, DELETE ON audit_log FROM app_role;
-- (จะ uncomment หลังสร้าง app_role ใน production setup)

-- 6. Indexes
CREATE INDEX idx_users_email       ON users(email);
CREATE INDEX idx_users_keycloak    ON users(keycloak_subject);
CREATE INDEX idx_audit_actor       ON audit_log(actor_id, created_at DESC);
CREATE INDEX idx_audit_created     ON audit_log(created_at DESC);
```

---

### `donnarec-api/internal/testutil/postgres.go` (test utility)

**Canonical Pattern:** testcontainers-go `PostgresContainer` shared fixture
**External Source:** https://testcontainers.com/guides/getting-started-with-testcontainers-for-go/

```go
// TestPostgresContainer: shared test fixture — เรียก 1 ครั้งต่อ test package
// ใช้ TestMain(m *testing.M) { container = SetupTestPostgres(...) }
func SetupTestPostgres(t *testing.T) *pgxpool.Pool {
    ctx := context.Background()
    pgContainer, err := postgres.RunContainer(ctx,
        testcontainers.WithImage("postgres:17"),
        postgres.WithDatabase("donnarec_test"),
        postgres.WithUsername("test"),
        postgres.WithPassword("test"),
        testcontainers.WithWaitStrategy(
            wait.ForLog("database system is ready to accept connections")),
    )
    require.NoError(t, err)
    t.Cleanup(func() { pgContainer.Terminate(ctx) })

    connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
    require.NoError(t, err)

    pool, err := pgxpool.New(ctx, connStr)
    require.NoError(t, err)

    // run migrations
    m, err := migrate.New("file://../../migrations", connStr)
    require.NoError(t, err)
    require.NoError(t, m.Up())

    return pool
}
```

---

### `donnarec-api/docker-compose.yml` (config)

**Canonical Pattern:** Docker Compose multi-service pattern (PostgreSQL + Keycloak)
**External Source:** RESEARCH.md Code Examples + Pitfall 4 (Keycloak production mode)

```yaml
# docker-compose.yml
services:
  postgres:
    image: postgres:17
    environment:
      POSTGRES_USER: donnarec
      POSTGRES_PASSWORD: ${DB_PASSWORD}
      POSTGRES_MULTIPLE_DATABASES: donnarec_app,donnarec_keycloak  # RESEARCH.md Open Q3
    volumes:
      - postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U donnarec"]
      interval: 10s
      timeout: 5s
      retries: 5

  keycloak:
    image: quay.io/keycloak/keycloak:26.6.3
    command: start  # ห้ามใช้ start-dev ใน production (RESEARCH.md Pitfall 4)
    environment:
      KC_DB: postgres
      KC_DB_URL: jdbc:postgresql://postgres/donnarec_keycloak
      KC_DB_USERNAME: donnarec
      KC_DB_PASSWORD: ${DB_PASSWORD}
      KC_HOSTNAME: ${KEYCLOAK_HOSTNAME}
    depends_on:
      postgres:
        condition: service_healthy

  api:
    build: ./donnarec-api
    environment:
      DATABASE_URL: postgres://donnarec:${DB_PASSWORD}@postgres/donnarec_app
      KEYCLOAK_BASE_URL: http://keycloak:8080
      KEYCLOAK_REALM: donnarec
      KEYCLOAK_CLIENT_ID: donnarec-backend
      DONAREC_KEK: ${DONAREC_KEK}  # 32-byte hex — ห้าม hardcode
    depends_on:
      postgres:
        condition: service_healthy

volumes:
  postgres_data:
```

---

### `donnarec-web/` (Next.js frontend)

**Canonical Pattern:** next-intl App Router setup (Next.js 15)
**External Source:** https://next-intl.dev/docs/getting-started/app-router

```typescript
// donnarec-web/src/i18n/config.ts
import { Pathnames } from "next-intl/navigation";

export const locales = ["th", "en"] as const;
export type Locale = (typeof locales)[number];
export const defaultLocale: Locale = "th";
```

```typescript
// donnarec-web/middleware.ts
import createMiddleware from "next-intl/middleware";
import { locales, defaultLocale } from "./src/i18n/config";

export default createMiddleware({ locales, defaultLocale });
export const config = { matcher: ["/((?!api|_next|.*\\..*).*)"] };
```

```json
// donnarec-web/src/messages/th.json — Phase 1 scope
{
    "auth": {
        "login": "เข้าสู่ระบบ",
        "logout": "ออกจากระบบ",
        "unauthorized": "ไม่มีสิทธิ์เข้าถึง"
    },
    "users": {
        "title": "จัดการผู้ใช้",
        "create": "เพิ่มผู้ใช้"
    }
}
```

---

## Shared Patterns

### Pattern A: Error Handling (AppError)

**Apply to:** ทุก service และ handler file
**Pattern:** Typed error ที่พกพา HTTP status code + i18n message key

```go
// internal/errors/errors.go
type AppError struct {
    StatusCode int
    Code       string // i18n message key เช่น "auth.insufficient_role"
    Cause      error  // internal error (ไม่ส่งไป client)
}

func (e *AppError) Error() string {
    if e.Cause != nil {
        return fmt.Sprintf("%s: %v", e.Code, e.Cause)
    }
    return e.Code
}

// ห้าม return raw errors จาก database ไปยัง handler โดยตรง
// ห้าม log PII ใน error message (log record ID แทน)
```

### Pattern B: Transaction Helper

**Apply to:** ทุก service ที่ต้องการ atomicity (users, audit, retention)
**Pattern:** Generic `WithTx` wrapper ที่ rollback อัตโนมัติ

```go
// internal/db/helpers.go
func WithTx(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
    tx, err := pool.Begin(ctx)
    if err != nil {
        return fmt.Errorf("begin tx: %w", err)
    }
    defer tx.Rollback(ctx) // rollback ถ้า fn ไม่ commit
    if err := fn(tx); err != nil {
        return err
    }
    return tx.Commit(ctx)
}
```

### Pattern C: Structured Logging

**Apply to:** ทุก layer (middleware, service, handler)
**Source:** go.uber.org/zap v1.28.0

```go
// ห้ามใช้ fmt.Println หรือ log.Println
// ห้าม log PII — ใช้ record ID หรือ masked value แทน

logger.Info("user created",
    zap.String("user_id", userID.String()),
    zap.String("actor_id", actor.Subject),
    // ห้ามใส่ email หรือ national_id ตรงๆ
)

logger.Error("database error",
    zap.String("operation", "CreateUser"),
    zap.Error(err),
    // ห้ามใส่ req.NationalID
)
```

### Pattern D: Gin Router Setup

**Apply to:** `cmd/server/main.go`
**Pattern:** Middleware chain ที่ถูกต้องตาม security requirement

```go
// ลำดับ middleware สำคัญมาก — ห้ามสลับ
router := gin.New()
router.Use(gin.Recovery())                  // 1. recover from panics ก่อนเสมอ
router.Use(zapLogger(logger))               // 2. request logging
router.Use(auditMiddleware(auditSvc))       // 3. audit interceptor (ก่อน auth เพื่อ log auth events ด้วย)

api := router.Group("/api")
api.Use(authMiddleware.RequireAuth())       // 4. JWT validation (protected routes)

adminGroup := api.Group("/admin")
adminGroup.Use(RequireRoles(RoleAdmin))     // 5. role-specific guard
```

### Pattern E: Input Validation (struct tags)

**Apply to:** ทุก request struct ใน handler files
**Source:** go-playground/validator v10

```go
// Request struct — ใช้ validate tags เสมอ
type CreateUserRequest struct {
    Email       string `json:"email"        validate:"required,email,max=255"`
    DisplayName string `json:"display_name" validate:"required,min=2,max=100"`
    Roles       []string `json:"roles"      validate:"required,min=1,dive,oneof=maker checker admin"`
}

// bind + validate ใน handler:
if err := c.ShouldBindJSON(&req); err != nil { ... }
if err := validator.New().Struct(req); err != nil { ... }
```

---

## No Analog Found

ไฟล์ทั้งหมดใน Phase 1 ไม่มี in-repo analog เนื่องจากเป็น greenfield

| File | Role | Data Flow | External Pattern Source |
|------|------|-----------|------------------------|
| ทุกไฟล์ข้างต้น | ดูตาราง File Classification | ดูตาราง | ดู Pattern Assignments แต่ละส่วน |

**แนวทางสำหรับ planner:** ใช้ RESEARCH.md code examples และ canonical external pattern ที่ระบุใน Pattern Assignments แต่ละส่วนแทน in-repo analog

---

## Foundational Rules (ทุกเฟสถัดไปต้องปฏิบัติตาม)

Pattern ที่กำหนดใน Phase 1 นี้คือ **baseline ที่ Phase 2–6 ต้องสืบทอด:**

1. **Constructor injection เสมอ** — ไม่มี global state ยกเว้น logger และ i18n bundle
2. **Audit-in-transaction** — ทุก mutation เขียน audit entry ใน transaction เดียวกัน ห้ามแยก
3. **i18n message key เสมอ** — ห้าม hardcode error string ใน Go code หรือ Next.js component
4. **sqlc query file เท่านั้น** — ห้ามสร้าง SQL string ด้วย string concatenation ในแอป
5. **KeyProvider interface** — ทุก encryption ผ่าน interface นี้; ห้าม call AES-GCM ตรงๆ นอก `crypto/` package
6. **REVOKE UPDATE/DELETE บน audit_log** — enforce ใน migration; อย่าข้าม
7. **zap logger** — ไม่มีข้อยกเว้น; ห้าม log PII plaintext ทุกกรณี
8. **golang-migrate** — ทุก schema change ต้องมี `.up.sql` + `.down.sql` คู่กัน

---

## Metadata

**Analog search scope:** Entire repository (greenfield — no source files found)
**Files scanned:** 0 source files (planning docs only)
**Pattern extraction date:** 2026-06-23
**Pattern source:** Official library docs + RESEARCH.md canonical references
**Valid until:** Phase 2 implementation begins (patterns become in-repo analogs หลัง Phase 1 commit)
