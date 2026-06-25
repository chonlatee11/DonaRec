# Phase 1: Foundation (DB, Auth/RBAC, Audit, Retention model) - Research

**Researched:** 2026-06-23
**Domain:** Go backend foundation — Auth (Keycloak OIDC + Go authZ), PostgreSQL, RBAC, append-only audit, retention model, PII encryption boundary, i18n scaffold
**Confidence:** HIGH (stack ยืนยันจาก Go module proxy + official docs; pattern มี multiple sources ยืนยัน)

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Admin เท่านั้นที่สร้าง/จัดการบัญชีผู้ใช้ — ไม่มี self-signup ในเฟสนี้
- **D-02:** Multi-role — 1 คนถือได้หลายบทบาท (ไม่ใช่ 1-คน-1-บทบาท), Admin ทำได้ทุกอย่าง
- **D-03:** DEVIATION จาก ROADMAP SC#1 — Success Criterion ข้อ 1 บอก "exactly one role" แต่ผู้ใช้เลือก multi-role → SC#1 ต้องอัปเดต roadmap หลังเฟสนี้
- **D-04:** Segregation of Duties บังคับระดับรายการ ไม่ใช่ระดับบทบาท — `approver_id ≠ created_by` ต่อ record (enforce จริงใน Phase 3)
- **D-05:** Bootstrap admin คนแรกผ่าน seed script / env
- **D-06..D-09:** Password policy/lockout/session ผ่าน Keycloak realm config
- **D-10..D-13:** PII visibility policy — Admin+Checker เห็นเลขเต็ม, Maker เห็นแค่ mask (4 ตัวท้าย), just-in-time reveal, reveal ต้อง audit
- **D-14:** Phase 1 วาง policy + mechanism; donor PII จริงมา Phase 3
- **D-15:** Audit ทุก mutation + auth events ผ่าน generic interceptor/middleware
- **D-16:** ดู audit log ได้: Admin เท่านั้น
- **D-17:** Tamper-evidence = DB REVOKE UPDATE/DELETE + hash-chain ต่อแถว
- **D-18:** Retention = config-driven: `retain_until`, `legal_basis` (enum), `legal_hold` (flag)
- **D-19:** ห้าม hard-delete record ที่อยู่ภายใต้ `legal_hold` (แอป + DB defense-in-depth)
- **D-20:** Backend = Go (golang) — override NestJS ทั้งหมดใน CLAUDE.md
- **D-21:** Database = PostgreSQL 17 (16+ ok)
- **D-22:** Frontend = React / Next.js
- **D-23:** Data layer ฝั่ง Go ให้ research เลือก — sqlc / pgx / GORM / ent (sqlc เป็น strong candidate)
- **D-24:** Auth = Hybrid: Keycloak ทำ authN (OIDC), Go app ทำ authZ ละเอียดเอง
- **D-25:** Encryption = envelope + KeyProvider abstraction; MVP ผูกกับ env/secrets; plan blind index schema
- **D-26:** Hosting = Docker-based, local-first, portable to cloud
- **D-27:** i18n วางโครงตั้งแต่ Phase 1 (TH/EN error/validation messages)

### Claude's Discretion
- รายละเอียด schema ของตาราง (users, roles, audit_log, retention fields, key metadata)
- รูปแบบ message-catalog / โครง i18n ฝั่ง Go และ Next.js — research/plan เลือก library
- ค่า config เฉพาะเจาะจง (lockout count, token TTL, default retain_until) — เริ่มด้วยค่าเหมาะสม ปรับได้ผ่าน config

### Deferred Ideas (OUT OF SCOPE)
- MFA/OTP — ไม่ทำใน MVP
- Cloud KMS / HSM — ตัดสินใจ hosting ก่อน
- Blind index search เต็มรูปแบบ — Phase 3
- Donor PII encrypt/decrypt/mask usage — Phase 3
- Consent capture — Phase 3 (Flow A) / Phase 6 (Flow B)
- อัปเดต ROADMAP SC#1 — ทำหลังเฟสนี้ผ่าน
</user_constraints>

---

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| NFR-01 | ผู้ใช้เข้าสู่ระบบด้วยบัญชี, รหัสผ่านถูกเข้ารหัส (hash), และจำกัดสิทธิ์ตามบทบาท RBAC | Keycloak ทำ authN+password hashing (argon2id ผ่าน realm config); Go middleware ทำ authZ; RBAC guard pattern ด้วย Gin middleware |
| FR-34 | Admin จัดการผู้ใช้และสิทธิ์ (RBAC) — Maker/Checker/Admin แยกบทบาท | User management ผ่าน Admin UI + API endpoint ที่ guard ด้วย Admin role; multi-role เก็บใน users_roles junction table |
| NFR-02 | เข้ารหัสข้อมูลขณะส่ง (HTTPS/TLS) และเข้ารหัสข้อมูลอ่อนไหวขณะจัดเก็บ | TLS ผ่าน reverse proxy (nginx); AES-256-GCM envelope + KeyProvider interface; Phase 1 วาง boundary, Phase 3 ใช้จริง |
| NFR-05 | เก็บ audit log ทุกการกระทำสำคัญแบบลบไม่ได้ | append-only table + REVOKE UPDATE/DELETE + SHA-256 hash-chain ต่อแถว; generic middleware interceptor |
| FR-13 | บันทึก audit trail ทุกการกระทำ (ใคร ทำอะไร เมื่อไร) | audit_log table + Go middleware; before/after JSON เก็บใน JSONB column |
| NFR-03 | บันทึก consent + ระบุวัตถุประสงค์ + รองรับสิทธิเจ้าของข้อมูล; retention ไม่ hard-delete | retain_until + legal_basis enum + legal_hold flag ใน schema; application guard + DB trigger ป้องกัน hard-delete |
</phase_requirements>

---

## Summary

Phase 1 วางรากฐาน 5 ชั้นที่ทุกเฟสถัดไปพึ่งพา: (1) **Keycloak OIDC authN** จัดการ login/session/lockout ทั้งหมด; (2) **Go Gin middleware authZ** ทำ RBAC guard + SoD rule + PII masking ฝั่งแอป; (3) **sqlc + pgx/v5** เป็น data layer ที่ให้ control เต็มที่บน `SELECT … FOR UPDATE` สำหรับ Phase 2; (4) **append-only audit log + SHA-256 hash-chain** บน PostgreSQL พร้อม REVOKE UPDATE/DELETE; (5) **retention schema** (`retain_until`, `legal_basis`, `legal_hold`) + **KeyProvider abstraction** (AES-256-GCM envelope) วางโครงไว้สำหรับ Phase 3.

เฟสนี้เป็น **greenfield scaffold** — ยังไม่มีโค้ด ทุก pattern เริ่มต้นใหม่ การตัดสินใจสำคัญที่สุดใน Phase 1 คือโครงสร้าง Go project layout, Keycloak realm config ที่ถูกต้อง, และ audit middleware ที่ wrap ทุก handler ตั้งแต่วันแรก เพราะ retrofit ภายหลังแพงมาก

**Primary recommendation:** ใช้ `sqlc + pgx/v5` เป็น data layer (ควบคุม SQL ได้เต็มที่ สำคัญมากสำหรับ gap-less counter ใน Phase 2), `coreos/go-oidc/v3` สำหรับ Keycloak token validation, Gin สำหรับ HTTP framework + middleware pipeline, และออกแบบ audit middleware เป็น Gin middleware แรกที่ทุก route ผ่านก่อน

---

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Authentication (login, password, session, lockout) | Keycloak (external IdP) | Go app (token validation middleware) | Keycloak เป็น dedicated IAM; Go app validate JWT เท่านั้น |
| Authorization / RBAC guard | Go API (middleware) | PostgreSQL (DB-level check เป็น defense-in-depth) | Business logic อยู่ฝั่งแอป; DB เป็น fallback ป้องกัน bypass |
| Audit logging | Go API (middleware interceptor) | PostgreSQL (append-only table) | Middleware capture ทุก action; DB enforce immutability |
| PII encryption boundary | Go API (application layer) | PostgreSQL (stores ciphertext only) | App-level encryption ป้องกัน DBA เห็น plaintext (PDPA) |
| Retention model / legal hold | Go API (service layer) | PostgreSQL (DB trigger + constraint) | Application ตรวจ legal_hold; DB trigger เป็น backstop |
| User management (CRUD) | Go API + Keycloak Admin API | PostgreSQL (app user table mirrors Keycloak) | Keycloak เก็บ credentials; app DB เก็บ business roles/profile |
| i18n message catalog | Go API (error/validation messages) | Next.js (UI messages) | Separate catalogs, same key convention |
| TLS / transport security | Reverse proxy (nginx/Caddy) | Go app (enforce HTTPS redirect) | Standard practice; TLS termination di proxy layer |
| Schema migrations | golang-migrate (CLI) | PostgreSQL | Version-controlled DDL; run ก่อน app start |

---

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go | 1.25.1 (installed) | Language | Single language; strong typing; concurrency-safe counters ใน Phase 2 |
| PostgreSQL | 17 (docker image) | Database | row-level locking สำหรับ gap-less counter; audit constraint |
| `github.com/gin-gonic/gin` | v1.12.0 | HTTP framework + middleware pipeline | 48% ecosystem share; mature middleware API; ดีสำหรับ auth guard chain |
| `github.com/sqlc-dev/sqlc` | v1.31.1 (CLI tool) | SQL codegen → type-safe Go | เขียน SQL ตรง; `FOR UPDATE` อยู่ใน .sql file เลย; zero runtime magic |
| `github.com/jackc/pgx/v5` | v5.10.0 | PostgreSQL driver + pgxpool | Pure Go; ดีที่สุดใน ecosystem; sqlc รองรับ pgx/v5 โดยตรง |
| `github.com/golang-migrate/migrate/v4` | v4.19.1 | Schema migration (CLI+lib) | Up/down migration files; pgx/v5 backend มีใน stdlib |
| `github.com/coreos/go-oidc/v3` | v3.19.0 | OIDC token validation (Keycloak JWKS) | Auto-discovers JWKS via Keycloak discovery URL; key caching built-in |
| `github.com/golang-jwt/jwt/v5` | v5.3.1 | JWT claims extraction | ใช้คู่กับ go-oidc; parse realm_access / resource_access claims |
| `golang.org/x/crypto` | v0.53.0 | argon2id (ถ้าจำเป็น), AES-GCM utilities | stdlib-adjacent; ใช้สำหรับ HMAC blind index key generation |
| `github.com/nicksnyder/go-i18n/v2` | v2.6.1 | Server-side i18n (TH/EN message catalog) | CLDR pluralization; JSON catalog; no global state |
| `go.uber.org/zap` | v1.28.0 | Structured logging | Structured JSON log; audit trail–friendly; pino equivalent ใน Go |

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/go-playground/validator/v10` | v10.30.3 | Request validation (struct tags) | Validate API input; เทียบเท่า Zod ใน NestJS |
| `github.com/stretchr/testify` | v1.11.1 | Test assertions | Standard Go testing helper |
| `github.com/testcontainers/testcontainers-go` | v0.43.0 | Integration tests กับ real PostgreSQL | ทดสอบ audit REVOKE, hash-chain, RBAC enforcement ต่อ real DB |
| `golang.org/x/text` | v0.38.0 | Language tag parsing (BCP 47) | ใช้คู่กับ go-i18n สำหรับ Accept-Language header |
| Keycloak | 26.x (Docker) | AuthN / IdP | Self-hosted Docker; realm config ผ่าน Admin UI หรือ import JSON |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| sqlc | GORM | GORM ง่ายกว่าแต่ abstraction ทำให้ `FOR UPDATE` ใน Phase 2 เดินบน Clauses API ที่มีความเสี่ยง misuse; sqlc ให้ control เต็มที่ |
| sqlc | ent | ent มี `.ForUpdate()` native แต่ codegen complex กว่า; sqlc โปร่งใสกว่าสำหรับ audit/security review |
| sqlc | pgx (raw) | pgx raw ให้ control เต็มที่แต่ไม่มี type-safe generated code; sqlc ใช้ pgx เป็น driver อยู่แล้ว |
| Gin | Echo | Echo มี centralized error handling ดีกว่า แต่ Gin ecosystem ใหญ่กว่า; ทั้งคู่รองรับ middleware เท่ากัน |
| coreos/go-oidc | gocloak | gocloak v13.9.0 ค้างอยู่ที่ 2024-02-01; go-oidc v3 active มากกว่า (v3.19.0 Jun 2026) |
| golang-migrate | goose | goose ใช้งานได้ดีเช่นกัน แต่ golang-migrate มี pgx/v5 backend โดยตรง |

### Installation

```bash
# Backend (Go module)
go get github.com/gin-gonic/gin@v1.12.0
go get github.com/jackc/pgx/v5@v5.10.0
go get github.com/coreos/go-oidc/v3@v3.19.0
go get github.com/golang-jwt/jwt/v5@v5.3.1
go get github.com/nicksnyder/go-i18n/v2@v2.6.1
go get golang.org/x/text@v0.38.0
go get golang.org/x/crypto@v0.53.0
go get go.uber.org/zap@v1.28.0
go get github.com/go-playground/validator/v10@v10.30.3
go get github.com/stretchr/testify@v1.11.1
go get github.com/testcontainers/testcontainers-go@v0.43.0

# sqlc CLI (codegen tool — ไม่ใส่ใน go.mod)
go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1

# golang-migrate CLI
go install github.com/golang-migrate/migrate/v4/cmd/migrate@v4.19.1

# Keycloak (Docker)
# quay.io/keycloak/keycloak:26.6.3
```

### Version Verification

ตรวจสอบจาก Go module proxy (https://proxy.golang.org) ทุก package ผ่าน — ทุกตัวมี active releases ใน 2025–2026 [VERIFIED: Go module proxy]

---

## Package Legitimacy Audit

> slopcheck ใช้กับ PyPI เท่านั้น ไม่รองรับ Go modules การตรวจสอบทำผ่าน Go module proxy ซึ่งเป็น authoritative registry สำหรับ Go ecosystem แทน

| Package | Registry | Age | Source Repo | Module Proxy | Disposition |
|---------|----------|-----|-------------|--------------|-------------|
| `github.com/gin-gonic/gin` | Go modules | ~10+ ปี | github.com/gin-gonic/gin | v1.12.0 (2026-02) | Approved |
| `github.com/sqlc-dev/sqlc` | Go modules | ~6 ปี | github.com/sqlc-dev/sqlc | v1.31.1 (2026-04) | Approved |
| `github.com/jackc/pgx/v5` | Go modules | ~10+ ปี | github.com/jackc/pgx | v5.10.0 (2026-06) | Approved |
| `github.com/coreos/go-oidc/v3` | Go modules | ~9 ปี | github.com/coreos/go-oidc | v3.19.0 (2026-06) | Approved |
| `github.com/golang-jwt/jwt/v5` | Go modules | ~5 ปี | github.com/golang-jwt/jwt | v5.3.1 (2026-01) | Approved |
| `github.com/golang-migrate/migrate/v4` | Go modules | ~8 ปี | github.com/golang-migrate/migrate | v4.19.1 (2025-11) | Approved |
| `github.com/nicksnyder/go-i18n/v2` | Go modules | ~10+ ปี | github.com/nicksnyder/go-i18n | v2.6.1 (2026-01) | Approved |
| `go.uber.org/zap` | Go modules | ~9 ปี | github.com/uber-go/zap | v1.28.0 (2026-04) | Approved |
| `golang.org/x/crypto` | Go modules | Official Go sub-repo | go.googlesource.com/crypto | v0.53.0 (2026-06) | Approved |
| `golang.org/x/text` | Go modules | Official Go sub-repo | go.googlesource.com/text | v0.38.0 (2026-06) | Approved |
| `github.com/go-playground/validator/v10` | Go modules | ~10+ ปี | github.com/go-playground/validator | v10.30.3 (2026-05) | Approved |
| `github.com/stretchr/testify` | Go modules | ~13 ปี | github.com/stretchr/testify | v1.11.1 (2025-08) | Approved |
| `github.com/testcontainers/testcontainers-go` | Go modules | ~7 ปี | github.com/testcontainers/testcontainers-go | v0.43.0 (2026-06) | Approved |

**Packages removed due to slopcheck [SLOP] verdict:** none (slopcheck ไม่รองรับ Go; verification ผ่าน module proxy แทน)
**Packages flagged as suspicious [SUS]:** none

*หมายเหตุ: slopcheck เป็น PyPI-only tool; Go ecosystem ใช้ module proxy (proxy.golang.org) เป็น authoritative source แทน ทุก package ข้างบนยืนยันผ่าน module proxy แล้ว [VERIFIED: Go module proxy]*

---

## Architecture Patterns

### System Architecture Diagram

```
Browser / Next.js (React)
         │
         │ HTTPS (TLS termination at proxy)
         ▼
   nginx / Caddy (reverse proxy)
    ├──► /auth/*  ──────────────────────► Keycloak :8080
    │                                          │ OIDC authN
    │                                          │ (login, session, lockout)
    │                                          ▼
    └──► /api/*  ──────────────────────► Go Gin API :8000
                                              │
                         ┌────────────────────┼─────────────────────┐
                         ▼                    ▼                     ▼
               Gin Middleware           RBAC Guard           Audit Middleware
               (JWT validate)       (role+SoD check)       (append audit row)
               coreos/go-oidc           per handler          every mutation
                         │                    │                     │
                         └────────────────────┴─────────────────────┘
                                              │
                                     sqlc + pgx/v5
                                              │
                                     PostgreSQL :5432
                                    ┌─────────┴──────────┐
                                    ▼                    ▼
                               app tables           audit_log table
                             (users, roles,        (append-only,
                              retention cfg,       REVOKE UPDATE/DELETE,
                              key_metadata)        hash-chain)
```

**Data flow หลัก (login + API call):**
1. Next.js redirect ไป Keycloak login page
2. Keycloak issue JWT access token (ใส่ realm roles ใน `realm_access.roles`)
3. Next.js ส่ง `Authorization: Bearer <token>` ทุก request ไป Go API
4. Go Gin middleware ตรวจ JWT signature ผ่าน Keycloak JWKS (auto-cached)
5. RBAC guard extract roles จาก claims → check permission
6. Handler เรียก sqlc query → PostgreSQL
7. Audit middleware เขียน audit row ต่อท้าย (append-only, hash-chain ต่อจากแถวก่อน)

### Recommended Project Structure

```
donnarec-api/                   # Go backend
├── cmd/
│   └── server/
│       └── main.go             # entrypoint
├── internal/
│   ├── auth/
│   │   ├── middleware.go       # Gin JWT middleware (go-oidc)
│   │   ├── claims.go           # custom claims struct สำหรับ Keycloak roles
│   │   └── rbac.go             # role check helpers + SoD rule
│   ├── audit/
│   │   ├── middleware.go       # Gin audit middleware (ทุก mutation)
│   │   ├── service.go          # append audit row + compute hash-chain
│   │   └── query.sql           # sqlc query สำหรับ audit_log
│   ├── crypto/
│   │   ├── keyprovider.go      # KeyProvider interface
│   │   ├── envprovider.go      # EnvKeyProvider (MVP: key ใน env)
│   │   └── aes_gcm.go          # AES-256-GCM encrypt/decrypt helpers
│   ├── i18n/
│   │   ├── bundle.go           # go-i18n bundle setup
│   │   └── locales/
│   │       ├── th.json         # Thai messages
│   │       └── en.json         # English messages
│   ├── users/
│   │   ├── handler.go          # Gin handlers
│   │   ├── service.go          # business logic
│   │   └── query.sql           # sqlc queries
│   ├── config/
│   │   └── retention.go        # RetentionConfig (default values)
│   └── db/
│       ├── sqlc.yaml           # sqlc config
│       └── generated/          # sqlc output (type-safe Go)
├── migrations/                 # golang-migrate files
│   ├── 000001_init_schema.up.sql
│   └── 000001_init_schema.down.sql
├── docker-compose.yml          # postgres + keycloak + app
├── go.mod
└── go.sum

donnarec-web/                   # Next.js frontend
├── src/
│   ├── app/
│   │   └── [locale]/           # next-intl locale routing
│   ├── i18n/
│   │   ├── config.ts
│   │   └── request.ts
│   └── messages/
│       ├── th.json
│       └── en.json
├── middleware.ts                # next-intl locale middleware
└── next.config.ts
```

### Pattern 1: Keycloak OIDC Token Validation Middleware

**What:** Gin middleware ที่ตรวจ Bearer JWT ทุก request โดย validate signature ผ่าน Keycloak JWKS endpoint
**When to use:** ทุก protected route (วาง `router.Use(authMiddleware)` ก่อน register routes)

```go
// Source: https://pkg.go.dev/github.com/coreos/go-oidc/v3/oidc
// internal/auth/middleware.go

type AuthMiddleware struct {
    verifier *oidc.IDTokenVerifier
}

func NewAuthMiddleware(keycloakBaseURL, realm, clientID string) (*AuthMiddleware, error) {
    ctx := context.Background()
    providerURL := fmt.Sprintf("%s/realms/%s", keycloakBaseURL, realm)
    provider, err := oidc.NewProvider(ctx, providerURL) // auto-fetches JWKS via discovery
    if err != nil {
        return nil, fmt.Errorf("oidc provider init: %w", err)
    }
    verifier := provider.Verifier(&oidc.Config{ClientID: clientID})
    return &AuthMiddleware{verifier: verifier}, nil
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

// KeycloakClaims maps Keycloak JWT structure
type KeycloakClaims struct {
    Subject      string     `json:"sub"`
    Email        string     `json:"email"`
    RealmAccess  RealmRoles `json:"realm_access"`
}
type RealmRoles struct {
    Roles []string `json:"roles"`
}
```

### Pattern 2: RBAC Guard Middleware

**What:** Gin middleware ที่รับ required roles แล้วตรวจว่า claims ใน context มีหรือไม่
**When to use:** วางต่อจาก RequireAuth ที่ route group ที่ต้องการ role เฉพาะ

```go
// internal/auth/rbac.go
func RequireRoles(requiredRoles ...string) gin.HandlerFunc {
    return func(c *gin.Context) {
        claims, exists := c.Get("claims")
        if !exists {
            c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "no_auth_context"})
            return
        }
        kc := claims.(KeycloakClaims)
        for _, required := range requiredRoles {
            if !hasRole(kc.RealmAccess.Roles, required) {
                c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "insufficient_role"})
                return
            }
        }
        c.Next()
    }
}

// SoD: ตรวจ approver ≠ creator — สำหรับ Phase 3 เตรียมไว้
// func RequireNotCreator(donationID int64) gin.HandlerFunc { ... }
```

### Pattern 3: Audit Middleware (Generic Interceptor)

**What:** Gin middleware บันทึกทุก mutation (POST/PUT/PATCH/DELETE) และ auth events
**When to use:** `router.Use(auditMiddleware)` ก่อน register routes ทุกตัว

```go
// internal/audit/middleware.go
// Pattern: บันทึก after handler ด้วย response writer interceptor
func AuditMiddleware(auditSvc *AuditService) gin.HandlerFunc {
    return func(c *gin.Context) {
        // skip GETs ที่ไม่ใช่ reveal-PII
        if c.Request.Method == http.MethodGet && !isPIIRevealEndpoint(c) {
            c.Next()
            return
        }
        claims, _ := c.Get("claims")
        c.Next() // execute handler
        // after handler: เขียน audit
        auditSvc.AppendAuditEntry(c.Request.Context(), AuditEntry{
            Actor:     extractActor(claims),
            Action:    deriveAction(c),
            Resource:  c.FullPath(),
            Timestamp: time.Now().UTC(),
            // before/after snapshots เก็บใน handler ผ่าน c.Set()
        })
    }
}
```

### Pattern 4: Append-Only Audit Log + Hash Chain

**What:** PostgreSQL audit_log table พร้อม REVOKE UPDATE/DELETE และ hash-chain ต่อแถว
**When to use:** สร้างใน migration แรก; ใช้กับ application role ที่จำกัด INSERT เท่านั้น

```sql
-- migrations/000001_init_schema.up.sql
-- audit_log: append-only, hash-chained
CREATE TABLE audit_log (
    id          BIGSERIAL PRIMARY KEY,
    actor_id    UUID        NOT NULL,
    actor_email TEXT        NOT NULL,
    action      TEXT        NOT NULL,          -- e.g. "user.create", "pii.reveal"
    resource    TEXT        NOT NULL,          -- e.g. "/api/users/123"
    before_json JSONB,                         -- snapshot ก่อนเปลี่ยน
    after_json  JSONB,                         -- snapshot หลังเปลี่ยน
    ip_address  INET,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    prev_hash   TEXT        NOT NULL,          -- SHA-256 ของแถวก่อน (GENESIS สำหรับแถวแรก)
    row_hash    TEXT        NOT NULL           -- SHA-256(fields + prev_hash)
);

-- REVOKE UPDATE/DELETE จาก application role
REVOKE UPDATE, DELETE ON audit_log FROM app_role;

-- Index สำหรับ audit lookup โดย Admin
CREATE INDEX idx_audit_log_actor ON audit_log(actor_id, created_at DESC);
CREATE INDEX idx_audit_log_created ON audit_log(created_at DESC);
```

```go
// internal/audit/service.go
// คำนวณ row_hash = SHA-256(id||actor_id||action||resource||created_at||prev_hash)
func computeRowHash(entry AuditEntry, prevHash string) string {
    h := sha256.New()
    fmt.Fprintf(h, "%d|%s|%s|%s|%s|%s",
        entry.ID, entry.ActorID, entry.Action,
        entry.Resource, entry.CreatedAt.Format(time.RFC3339Nano), prevHash)
    return hex.EncodeToString(h.Sum(nil))
}
```

### Pattern 5: KeyProvider Interface (Envelope Encryption)

**What:** Interface สำหรับ wrap/unwrap DEK; MVP implementation ใช้ env var สำหรับ KEK
**When to use:** ทุกที่ที่ต้องการ encrypt/decrypt PII field (Phase 3 จะใช้จริง; Phase 1 ออกแบบ interface)

```go
// Source: https://www.lambrospetrou.com/articles/encryption/
// internal/crypto/keyprovider.go

// KeyProvider abstracts KMS operations — swap implementation โดยไม่แก้ call site
type KeyProvider interface {
    WrapKey(ctx context.Context, plaintextDEK []byte) ([]byte, error)   // KEK encrypts DEK
    UnwrapKey(ctx context.Context, wrappedDEK []byte) ([]byte, error)   // KEK decrypts DEK
}

// EnvKeyProvider: MVP — KEK มาจาก environment variable
type EnvKeyProvider struct {
    kek []byte // 32-byte AES-256 key จาก env
}

func NewEnvKeyProvider(kekHex string) (*EnvKeyProvider, error) {
    // decode hex KEK จาก env var DONAREC_KEK
    ...
}

// EncryptField: สร้าง DEK ใหม่, encrypt plaintext ด้วย AES-256-GCM, wrap DEK ด้วย KEK
func EncryptField(ctx context.Context, kp KeyProvider, plaintext []byte) (ciphertext, wrappedDEK []byte, err error) {
    dek := make([]byte, 32)
    rand.Read(dek)
    // AES-256-GCM encrypt
    ...
    wrappedDEK, err = kp.WrapKey(ctx, dek)
    return
}

// BlindIndex: HMAC-SHA256 ของ plaintext ด้วย index key แยกต่างหาก
func BlindIndex(plaintext, indexKey []byte) []byte {
    mac := hmac.New(sha256.New, indexKey)
    mac.Write(plaintext)
    return mac.Sum(nil)
}
```

### Pattern 6: sqlc + pgx/v5 Transaction Pattern

**What:** ใช้ sqlc Queries.WithTx() สำหรับ transaction; raw SQL ด้วย pgx.Tx.QueryRow สำหรับ `FOR UPDATE`
**When to use:** Phase 1 ใช้สำหรับ transaction ปกติ; Phase 2 เพิ่ม `FOR UPDATE` query ใน .sql file

```yaml
# db/sqlc.yaml
version: "2"
sql:
  - engine: "postgresql"
    queries: "./queries"
    schema: "./migrations"
    gen:
      go:
        package: "db"
        out: "./generated"
        sql_package: "pgx/v5"
```

```go
// Source: https://docs.sqlc.dev/en/latest/howto/transactions.html
// transaction pattern ด้วย sqlc
func (s *UserService) CreateUserTx(ctx context.Context, params CreateUserParams) error {
    return pgx_helpers.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
        qtx := s.queries.WithTx(tx)
        user, err := qtx.CreateUser(ctx, params)
        if err != nil {
            return err
        }
        // audit เขียนใน transaction เดียวกัน
        return qtx.InsertAuditLog(ctx, buildAuditEntry(user))
    })
}

// Phase 2 gap-less counter: SELECT … FOR UPDATE อยู่ใน query.sql ตรงๆ
// ไม่ต้องใช้ ORM abstraction เลย — sqlc generate เป็น type-safe function ให้เลย
```

### Pattern 7: Retention Model Schema

**What:** Fields `retain_until`, `legal_basis`, `legal_hold` ใน tables ที่มีข้อมูลต้องเก็บ
**When to use:** วางใน migration แรก; ทุก table ที่เก็บ personal data หรือ donation record ต้องมี

```sql
-- legal_basis enum ตาม PDPA
CREATE TYPE legal_basis_enum AS ENUM (
    'tax_obligation',     -- เก็บตาม ป.รัษฎากร (หลัก)
    'consent',            -- เก็บโดยความยินยอม donor
    'legitimate_interest' -- เก็บโดย legitimate interest
);

-- retention_config: ค่า default อ่านจาก config ไม่ hardcode
CREATE TABLE retention_config (
    id              SERIAL PRIMARY KEY,
    entity_type     TEXT NOT NULL UNIQUE,     -- 'donation', 'donor', 'audit_log'
    default_retain_days INT NOT NULL,         -- เช่น 1825 = 5 ปี
    legal_basis     legal_basis_enum NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by      UUID NOT NULL
);

-- ตัวอย่าง retention fields ที่ต้องใส่ใน table ที่มี personal data:
-- retain_until   TIMESTAMPTZ,              -- null = ใช้ default จาก retention_config
-- legal_basis    legal_basis_enum,
-- legal_hold     BOOLEAN NOT NULL DEFAULT false
```

```sql
-- DB trigger ป้องกัน DELETE record ที่อยู่ใต้ legal_hold (defense-in-depth)
CREATE OR REPLACE FUNCTION prevent_legal_hold_delete()
RETURNS TRIGGER AS $$
BEGIN
    IF OLD.legal_hold = true THEN
        RAISE EXCEPTION 'cannot delete record under legal hold';
    END IF;
    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

-- (สร้าง trigger บน tables ที่มี legal_hold field เมื่อ Phase 3 เพิ่ม donor/donation)
```

### Anti-Patterns to Avoid

- **JWT claims เป็น source of truth สำหรับ authZ:** Keycloak ให้แค่ role assignment; business SoD (`approver_id ≠ created_by`) ต้องตรวจฝั่งแอปเสมอ ไม่ใช่แค่ดู JWT
- **Audit ใน finally block หรือ goroutine แยก:** Audit entry ต้องเขียนใน transaction เดียวกับ data mutation เสมอ มิฉะนั้น crash ระหว่าง commit ทำให้ audit หายได้
- **GET prev_hash ก่อน insert audit ใน transaction แยก:** ต้องใช้ PostgreSQL advisory lock หรือ `SELECT … FOR UPDATE` เพื่อป้องกัน concurrent audit entries ที่ใช้ prev_hash เดียวกัน
- **Store DEK plaintext ใน DB:** DEK ต้อง wrap ด้วย KEK ก่อนเก็บ; plaintext DEK ต้องอยู่ใน memory เท่านั้น
- **Hardcode KEK ใน code:** KEK ต้องมาจาก env var หรือ secrets manager เสมอ

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| OIDC token validation + JWKS caching | Custom JWT parser | `coreos/go-oidc/v3` | JWKS rotation, key caching, signature algo verification มี edge case มาก |
| Password policy + lockout + session | Custom auth server | Keycloak realm config | Brute force protection, session revocation, MFA toggle ซับซ้อนมาก |
| Structured logging | `fmt.Println` / custom logger | `go.uber.org/zap` | Concurrent-safe; JSON output; สำหรับ audit tail ต้อง structured |
| DB schema migration | Manual SQL scripts | golang-migrate | Atomic up/down; lock table; idempotent run |
| Input validation | Manual type checks | `go-playground/validator` | ครอบคลุม 100+ rules; struct tag convention ป้องกัน missed validation |
| BCP 47 language matching | Switch/case on lang string | `golang.org/x/text/language` | Language matching มี fallback rules ซับซ้อน (e.g., zh-Hans ≠ zh-Hant) |
| AES-GCM | Third-party crypto library | `crypto/cipher` stdlib | Go stdlib มี AES-GCM ครบ ไม่ต้องพึ่ง external |

**Key insight:** ใน Go ต่างจาก NestJS — ไม่มี DI framework ช่วย wire dependencies อัตโนมัติ ต้องออกแบบ dependency injection เอง (constructor injection pattern); แต่ stdlib + ecosystem ครอบคลุมทุก need โดยไม่ต้องพึ่ง magical framework

---

## Common Pitfalls

### Pitfall 1: Keycloak roles ใน JWT มาจาก wrong claim path
**What goes wrong:** Extract roles จาก `roles` field ตรงๆ แทนที่จะเป็น `realm_access.roles` — ได้ empty array ทำ RBAC bypass ได้โดยไม่รู้
**Why it happens:** Keycloak ใช้ nested structure; คนเขียน Go struct ผิดชื่อ field
**How to avoid:** Test middleware ด้วย real Keycloak token (ใช้ testcontainers); ใส่ unit test ที่ parse claims และ assert ว่า roles ถูก extract ถูกต้อง
**Warning signs:** RBAC guard ไม่ deny ทั้งที่ user ไม่มี role

### Pitfall 2: Audit hash-chain race condition
**What goes wrong:** 2 requests เขียน audit log พร้อมกัน ทั้งคู่ query prev_hash ได้ค่าเดียวกัน → chain แตก
**Why it happens:** `SELECT MAX(id)` แล้ว `INSERT` ไม่ใช่ atomic operation
**How to avoid:** ใช้ PostgreSQL advisory lock (`pg_advisory_xact_lock`) ก่อน insert audit entry หรือใช้ `INSERT … RETURNING id` ใน transaction เดียวกับ SELECT prev_hash ด้วย `FOR UPDATE`
**Warning signs:** hash-chain verification fails; duplicate `prev_hash` values ใน audit_log

### Pitfall 3: JWT validation ไม่ตรวจ issuer / audience
**What goes wrong:** รับ JWT จาก Keycloak realm อื่นได้ (ถ้า attacker มี Keycloak instance) → authentication bypass
**Why it happens:** go-oidc config ไม่ set ClientID หรือ issuer ไม่ match
**How to avoid:** ตรวจ `iss` claim ต้อง match realm URL; ตรวจ `aud` claim ต้อง include client ID ของ backend API
**Warning signs:** ทดสอบด้วย JWT จาก realm อื่นแล้วผ่าน middleware

### Pitfall 4: Keycloak ใน Docker start-dev mode ใช้ใน production
**What goes wrong:** `start-dev` ใช้ H2 in-memory DB → data หาย เมื่อ container restart
**Why it happens:** copy Docker command จาก Keycloak quickstart โดยไม่เปลี่ยน
**How to avoid:** ใช้ `start` (production mode) + configure `KC_DB=postgres` ชี้ไปที่ PostgreSQL; สำหรับ dev ใช้ `start-dev` แต่ document ชัดว่าห้ามใช้ใน production
**Warning signs:** Keycloak restart แล้ว users หายหมด

### Pitfall 5: KeyProvider ผูกกับ AWS KMS ตั้งแต่ MVP
**What goes wrong:** แอปรัน local ไม่ได้เพราะต้องการ AWS credentials
**Why it happens:** implement encryption โดยไม่มี abstraction layer
**How to avoid:** ออกแบบ `KeyProvider` interface ก่อน; `EnvKeyProvider` เป็น default MVP; swap ได้เมื่อ hosting ตัดสินใจ (D-26)
**Warning signs:** `go test` fail บน local เพราะไม่มี AWS credentials

### Pitfall 6: i18n ใช้ hardcoded strings ใน error responses
**What goes wrong:** Retrofit i18n ภายหลังต้อง touch ทุก handler ใหม่
**Why it happens:** เริ่มด้วย `"error: invalid password"` แทน `c.JSON(…, gin.H{"error": i18n.T(c, "auth.invalid_password")})`
**How to avoid:** ตั้ง i18n bundle ตั้งแต่วันแรก; ทุก error ใช้ message key; เริ่ม Phase 1 ด้วย catalog เล็กๆ ขยายได้
**Warning signs:** frontend แสดง Thai text ใน English mode หรือกลับกัน

---

## Code Examples

### Keycloak Realm Config (recommended values สำหรับ Phase 1)

```json
// keycloak/realm-donnarec.json (import file)
{
  "realm": "donnarec",
  "passwordPolicy": "length(8) and upperCase(1) and digits(1) and notUsername()",
  "bruteForceProtected": true,
  "permanentLockout": false,
  "maxLoginFailures": 5,
  "waitIncrementSeconds": 60,
  "maxFailureWaitSeconds": 900,
  "failureResetTimeSeconds": 43200,
  "ssoSessionIdleTimeout": 1800,
  "ssoSessionMaxLifespan": 28800,
  "accessTokenLifespan": 300,
  "clients": [
    {
      "clientId": "donnarec-backend",
      "bearerOnly": true
    },
    {
      "clientId": "donnarec-frontend",
      "publicClient": true,
      "redirectUris": ["http://localhost:3000/*"]
    }
  ]
}
```

**Config สำคัญ (D-07..D-09):**
- `maxLoginFailures: 5` → lockout ชั่วคราวหลัง 5 ครั้ง [ASSUMED — ปรับได้ผ่าน config]
- `ssoSessionIdleTimeout: 1800` (30 นาที) → idle timeout
- `ssoSessionMaxLifespan: 28800` (8 ชั่วโมง) → max session
- `accessTokenLifespan: 300` (5 นาที) → JWT อายุสั้น; frontend ใช้ refresh token

### sqlc config + transaction example

```yaml
# db/sqlc.yaml — [VERIFIED: docs.sqlc.dev]
version: "2"
sql:
  - engine: "postgresql"
    queries: "./queries"
    schema: "./migrations"
    gen:
      go:
        package: "db"
        out: "./generated"
        sql_package: "pgx/v5"
        emit_interface: true          # generates Querier interface สำหรับ mock testing
        emit_json_tags: true
        emit_pointers_for_null_types: true
```

### go-i18n v2 bundle setup

```go
// Source: https://pkg.go.dev/github.com/nicksnyder/go-i18n/v2/i18n
// internal/i18n/bundle.go
bundle := i18n.NewBundle(language.Thai) // default locale = Thai
bundle.RegisterUnmarshalFunc("json", json.Unmarshal)
bundle.LoadMessageFile("locales/th.json")
bundle.LoadMessageFile("locales/en.json")

// per-request localizer (จาก Accept-Language header)
localizer := i18n.NewLocalizer(bundle, c.GetHeader("Accept-Language"), "th")
msg := localizer.MustLocalize(&i18n.LocalizeConfig{MessageID: "auth.invalid_password"})
```

---

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `lib/pq` PostgreSQL driver | `pgx/v5` | 2022 (lib/pq → maintenance mode) | Pure Go; ดีกว่า; sqlc รองรับ pgx/v5 โดยตรง |
| gorilla/mux HTTP router | Chi หรือ Gin (Go 1.22+ ServeMux) | 2023 (gorilla archived) | gorilla archived — ห้ามใช้ |
| go-oidc v1 | `coreos/go-oidc/v3` | 2021 | v1 deprecated; v3 เปลี่ยน API + `*RemoteKeySet` type |
| Keycloak `start-dev` | `start` + PostgreSQL backend | Keycloak 17+ | `start-dev` ใช้ H2; production ต้องใช้ `start` |
| Keycloak default pbkdf2-sha256 hashing | pbkdf2-sha512 | Keycloak 24 (2024) | Security upgrade; ผลกระทบต่อ existing password hashes |
| go-i18n v1 (deprecated) | `go-i18n/v2` | 2018 | v2 rewrite; no global state; BCP 47 |

**Deprecated/outdated:**
- `gorilla/mux`: archived 2023 — ห้ามใช้ ใช้ Chi หรือ Gin แทน
- `go-oidc v1` (non-versioned import): deprecated — ใช้ v3 เท่านั้น
- `lib/pq`: maintenance mode — ใช้ pgx/v5 แทน
- `Nerzal/gocloak v13`: last release 2024-02-01 — inactive; ไม่แนะนำสำหรับ Phase 1 ที่ต้องการ active security library

---

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Keycloak lockout config: 5 ครั้ง → lockout ชั่วคราว, idle 30 นาที, max 8 ชั่วโมง — ค่าเหมาะสมสำหรับ back-office | Code Examples | ถ้าโรงพยาบาลต้องการค่าต่างออกไป → แก้ realm config เท่านั้น ไม่กระทบ code |
| A2 | Default retain_until = 5 ปี (1825 วัน) สำหรับ donation records | Retention Model | ถ้า DPO ยืนยัน period ต่างออกไป → แก้ retention_config table เท่านั้น; model ยังรองรับ |
| A3 | PostgreSQL เป็น Keycloak backend ใน production (ใช้ instance เดียวกัน หรือ DB แยก) | Architecture | ควรใช้ DB แยกเพื่อ isolation แต่ไม่ blocking Phase 1 |
| A4 | Gin เป็น HTTP framework ที่เลือก (ไม่ใช่ Echo หรือ Chi) — ตัดสินใจในเฟสนี้ | Standard Stack | ถ้าต้องการ Echo แทน → เปลี่ยน middleware API เล็กน้อย แต่ pattern เดิมทั้งหมด |

---

## Open Questions

1. **Keycloak realm roles vs client roles สำหรับ Maker/Checker/Admin**
   - What we know: Keycloak รองรับทั้ง realm roles (ข้าม clients) และ client roles (เฉพาะ client)
   - What's unclear: DonaRec ควรใช้ realm roles หรือ client roles? Realm roles ง่ายกว่า แต่ client roles แยก scope ชัดกว่า
   - Recommendation: ใช้ **realm roles** สำหรับ MVP (Maker, Checker, Admin) เพราะแอปมี backend เดียว; เปลี่ยนเป็น client roles ได้ในอนาคตถ้ามี multi-service

2. **Audit advisory lock approach**
   - What we know: ต้องการ atomic prev_hash ↔ insert audit
   - What's unclear: PostgreSQL advisory lock `pg_advisory_xact_lock` เหมาะกว่าหรือ `SELECT … FOR UPDATE` บน audit_sequence table?
   - Recommendation: ใช้ dedicated `audit_sequence` table + `SELECT FOR UPDATE` (pattern เดียวกับ gap-less counter ใน Phase 2 — consistent)

3. **Keycloak DB isolation**
   - What we know: Keycloak ต้องการ PostgreSQL backend ใน production
   - What's unclear: ใช้ PostgreSQL instance เดียวกับ app (database แยก schema) หรือ PostgreSQL container แยก?
   - Recommendation: Docker compose ใช้ PostgreSQL instance เดียว แต่แยก database (`donnarec_app` และ `donnarec_keycloak`) เพื่อ isolation

---

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go | Backend compilation | ✓ | 1.25.1 | — |
| Docker | PostgreSQL + Keycloak containers | ✓ | 28.1.1 | — |
| Docker Compose | Multi-container dev env | ✓ | v2.39.2 | — |
| Node.js | Next.js frontend | ✓ | v24.10.0 | — |
| npm | Frontend deps | ✓ | 11.6.0 | — |
| PostgreSQL (client) | Manual DB inspection | ✗ | — | ใช้ `docker exec` หรือ pgAdmin |
| Keycloak | AuthN | ✗ (not running) | — | Start via `docker compose up keycloak` |
| Redis | Not needed in Phase 1 (BullMQ เป็น Phase 4) | ✗ | — | N/A — Phase 1 ไม่ใช้ |
| make | Build automation | ✓ | GNU 4.3 | sh scripts |

**Missing dependencies with no fallback:**
- PostgreSQL + Keycloak ต้อง start ผ่าน docker-compose ก่อน dev/test (ไม่มี fallback แต่ setup ง่ายมาก)

**Missing dependencies with fallback:**
- PostgreSQL client (psql): ใช้ `docker exec -it <postgres-container> psql` แทน

---

## Validation Architecture

> nyquist_validation = true ใน .planning/config.json

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go standard `testing` + `testify` v1.11.1 |
| Integration tests | `testcontainers-go` v0.43.0 (real PostgreSQL + Keycloak) |
| Config file | ไม่มี config file แยก — `go test ./...` |
| Quick run command | `go test ./... -short` (unit tests เท่านั้น, skip integration) |
| Full suite command | `go test ./... -timeout 120s` (รวม integration tests) |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|--------------|
| NFR-01 | Login ด้วย valid credentials ผ่าน Keycloak OIDC | integration | `go test ./internal/auth/... -run TestOIDCMiddleware` | ❌ Wave 0 |
| NFR-01 | RBAC guard reject request ที่ role ไม่พอ → 403 | unit | `go test ./internal/auth/... -run TestRequireRoles` | ❌ Wave 0 |
| NFR-01 | RBAC guard allow request ที่ role พอ → 200 | unit | `go test ./internal/auth/... -run TestRequireRoles` | ❌ Wave 0 |
| FR-34 | Admin สร้าง user + assign roles สำเร็จ | integration | `go test ./internal/users/... -run TestAdminCreateUser` | ❌ Wave 0 |
| FR-34 | Non-admin ไม่สามารถ create user → 403 | unit | `go test ./internal/users/... -run TestCreateUserRBAC` | ❌ Wave 0 |
| NFR-02 | KeyProvider interface: wrap/unwrap key สำเร็จ | unit | `go test ./internal/crypto/... -run TestEnvKeyProvider` | ❌ Wave 0 |
| NFR-02 | AES-256-GCM: encrypt แล้ว decrypt ได้ plaintext เดิม | unit | `go test ./internal/crypto/... -run TestAESGCMRoundTrip` | ❌ Wave 0 |
| NFR-02 | BlindIndex: HMAC output คงที่สำหรับ input เดียวกัน | unit | `go test ./internal/crypto/... -run TestBlindIndex` | ❌ Wave 0 |
| NFR-05 | Audit log: UPDATE/DELETE บน audit_log denied ที่ DB | integration | `go test ./internal/audit/... -run TestAuditImmutability` | ❌ Wave 0 |
| NFR-05 | Audit log: hash-chain integrity ตรวจสอบได้ (verify chain) | integration | `go test ./internal/audit/... -run TestHashChainVerification` | ❌ Wave 0 |
| NFR-05 | Audit log: concurrent inserts ไม่ทำให้ chain แตก | integration | `go test ./internal/audit/... -run TestConcurrentAuditInserts` | ❌ Wave 0 |
| FR-13 | ทุก POST/PUT/DELETE request มี audit entry | integration | `go test ./internal/audit/... -run TestAuditMiddlewareCoverage` | ❌ Wave 0 |
| FR-13 | PII reveal event ถูก audit (actor, resource, timestamp) | unit | `go test ./internal/audit/... -run TestPIIRevealAudit` | ❌ Wave 0 |
| NFR-03 | legal_hold=true: hard DELETE blocked (DB trigger) | integration | `go test ./internal/retention/... -run TestLegalHoldDeleteBlocked` | ❌ Wave 0 |
| NFR-03 | legal_hold=false: soft delete (status change) ผ่านได้ | unit | `go test ./internal/retention/... -run TestSoftDeleteAllowed` | ❌ Wave 0 |
| NFR-03 | retain_until field ถูก set ตาม retention_config | unit | `go test ./internal/retention/... -run TestRetainUntilCalculation` | ❌ Wave 0 |

### Sampling Rate

- **Per task commit:** `go test ./... -short` (unit tests; < 10 วินาที)
- **Per wave merge:** `go test ./... -timeout 120s` (full suite รวม integration)
- **Phase gate:** Full suite green ก่อน `/gsd:verify-work`

### Wave 0 Gaps

- [ ] `internal/auth/middleware_test.go` — ครอบคลุม NFR-01 (RBAC guard unit tests)
- [ ] `internal/auth/middleware_integration_test.go` — OIDC with real Keycloak via testcontainers
- [ ] `internal/audit/service_test.go` — hash-chain unit test
- [ ] `internal/audit/immutability_test.go` — testcontainers: REVOKE UPDATE/DELETE enforcement
- [ ] `internal/audit/concurrent_test.go` — concurrent inserts ไม่แตก chain
- [ ] `internal/crypto/keyprovider_test.go` — AES-GCM round-trip + blind index
- [ ] `internal/retention/retention_test.go` — legal_hold delete block
- [ ] `internal/users/user_test.go` — RBAC user management
- [ ] `testcontainers` setup helper: `internal/testutil/postgres.go` — shared test DB fixture
- [ ] Framework install: `go get github.com/testcontainers/testcontainers-go@v0.43.0`
- [ ] Docker Compose setup สำหรับ integration tests (PostgreSQL + Keycloak)

---

## Security Domain

> security_enforcement ไม่ได้ set เป็น false ใน config.json → enabled

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | YES | Keycloak realm (password policy, lockout, session timeout); argon2id ผ่าน Keycloak config |
| V3 Session Management | YES | Keycloak session idle/max timeout; JWT access token short-lived (5 min); refresh token rotation |
| V4 Access Control | YES | Gin RBAC middleware; SoD rule `approver_id ≠ created_by`; least-privilege role assignment |
| V5 Input Validation | YES | `go-playground/validator` struct tags; Gin binding validation บน all request bodies |
| V6 Cryptography | YES | AES-256-GCM envelope encryption; HMAC-SHA256 blind index; SHA-256 hash-chain audit; golang.org/x/crypto |
| V7 Error Handling & Logging | YES | zap structured logging; i18n error messages ไม่ leak internal details |
| V8 Data Protection | YES | PII masking (4-digit tail) สำหรับ Maker; just-in-time reveal ต้อง audit; encrypted at rest |
| V9 Communication | YES | TLS at reverse proxy layer; HTTPS-only; Keycloak OIDC ผ่าน HTTPS |

### Known Threat Patterns for Go + Keycloak + PostgreSQL Stack

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| JWT replay / token theft | Spoofing | Short-lived access token (5 min); refresh token rotation; HTTPS-only transport |
| RBAC bypass via missing role check | Elevation of privilege | ทุก route ผ่าน `RequireRoles` middleware; DB-level check เป็น defense-in-depth |
| Audit tampering by DBA | Tampering | REVOKE UPDATE/DELETE จาก app role; hash-chain ทำให้แก้ตรวจเจอ |
| PII exposure ผ่าน log | Information disclosure | zap ต้องไม่ log raw PII; log masked value หรือ record ID แทน |
| Key exposure ใน source code | Information disclosure | KEK ต้องมาจาก env var / secrets manager เท่านั้น; ห้าม commit |
| SQL injection | Tampering | sqlc-generated parameterized queries ทุกตัว; ไม่ใช้ string concatenation |
| Mass assignment | Tampering | Gin binding กับ explicit struct fields; ไม่รับ `*map[string]interface{}` |
| Broken authentication (Keycloak misconfiguration) | Spoofing | Test realm config ด้วย automated check; validate issuer + audience ใน go-oidc |
| Session fixation | Spoofing | Keycloak จัดการ session lifecycle; front-channel logout invalidates session |
| Audit_log race condition (hash-chain break) | Tampering | Advisory lock / SELECT FOR UPDATE บน audit_sequence ก่อน insert |

---

## Sources

### Primary (HIGH confidence)
- [Go module proxy (proxy.golang.org)](https://proxy.golang.org) — version verification ทุก package [VERIFIED: Go module proxy]
- [sqlc docs — Using Go and pgx](https://docs.sqlc.dev/en/latest/guides/using-go-and-pgx.html) — sqlc.yaml config, pgx/v5 setup [VERIFIED: official docs]
- [sqlc docs — Transactions](https://docs.sqlc.dev/en/latest/howto/transactions.html) — WithTx pattern [VERIFIED: official docs]
- [coreos/go-oidc — pkg.go.dev](https://pkg.go.dev/github.com/coreos/go-oidc/v3/oidc) — NewProvider, IDTokenVerifier, RemoteKeySet [VERIFIED: official Go pkg docs]
- [go-i18n — pkg.go.dev](https://pkg.go.dev/github.com/nicksnyder/go-i18n/v2/i18n) — bundle, localizer API [VERIFIED: official Go pkg docs]
- [Keycloak docs — Running in Container](https://www.keycloak.org/server/containers) — Docker config, memory requirements [CITED: keycloak.org]
- [Keycloak 24.0.0 Release Notes](https://www.keycloak.org/2024/03/keycloak-2400-released) — pbkdf2-sha512 change, brute force event [CITED: keycloak.org]
- [Go crypto/cipher package](https://pkg.go.dev/crypto/cipher) — AES-GCM standard library [VERIFIED: official Go pkg docs]
- [Lambrospetrou.com — Envelope Encryption with KMS in Go](https://www.lambrospetrou.com/articles/encryption/) — KeyEncryptionWrapper pattern [CITED: official blog]
- [next-intl — App Router setup](https://next-intl.dev/docs/getting-started/app-router) — Next.js 15 i18n setup [CITED: next-intl.dev]

### Secondary (MEDIUM confidence)
- [Appmaster.io — Tamper-evident audit trails in PostgreSQL](https://appmaster.io/blog/tamper-evident-audit-trails-postgresql) — hash-chain pattern [CITED]
- [Tracehold.ai — Immutable audit log HMAC hash chain](https://tracehold.ai/blog/immutable-audit-log-hmac-hash-chain/) — hash-chain detail [CITED]
- [Bytebase — Golang ORM/Query Builder 2025](https://www.bytebase.com/blog/golang-orm-query-builder/) — sqlc/pgx/GORM comparison [CITED]
- [Glukhov.org — Comparing Go ORMs](https://www.glukhov.org/post/2025/09/comparing-go-orms-gorm-ent-bun-sqlc/) — FOR UPDATE comparison [CITED]
- [Encore.dev — Comparing Go ORMs](https://encore.dev/resources/go-orms) — ecosystem comparison [CITED]
- [JetBrains — Go Ecosystem 2025](https://blog.jetbrains.com/go/2025/11/10/go-language-trends-ecosystem-2025/) — framework market share [CITED]
- [Maciej.litwiniuk.net — Searchability for Encrypted Records](https://maciej.litwiniuk.net/posts/2026-02-25-searchability-for-encrypted-records/) — blind index pattern [CITED]
- [Logto.io — Protect Gin API with RBAC and JWT](https://docs.logto.io/api-protection/go/gin) — OIDC middleware pattern [CITED]

### Tertiary (LOW confidence)
- WebSearch aggregations สำหรับ Keycloak realm timeout recommendations — [ASSUMED] ค่าเฉพาะ (5 ครั้ง lockout, 30 นาที idle) ปรับได้ผ่าน config

---

## Metadata

**Confidence breakdown:**
- Standard stack (sqlc/pgx/Gin/go-oidc): HIGH — ยืนยันผ่าน module proxy + official docs
- Architecture patterns (Keycloak hybrid auth, audit hash-chain): HIGH — multiple authoritative sources
- Keycloak realm config values: MEDIUM — official docs บอก field names; specific values เป็น ASSUMED
- Retention/PDPA compliance period: LOW — รอ DPO ยืนยัน; schema ออกแบบ config-driven พร้อมรับค่าใดก็ได้

**Research date:** 2026-06-23
**Valid until:** 2026-07-23 (30 days — Go ecosystem stable; Keycloak minor releases ไม่กระทบ architecture)
