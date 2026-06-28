# Phase 3: Donation Lifecycle & Maker-Checker Issuance - Research

**Researched:** 2026-06-28
**Domain:** Go (Gin) + PostgreSQL 17 + MinIO — donation state machine, issuance transaction, PII encryption, object storage
**Confidence:** HIGH

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**Donor data model (FR-29)**
- D-43: Snapshot-only — ไม่มี donor master table / ไม่มี blind index ในเฟสนี้
- D-44: เลขภาษี/บัตร ปชช. บังคับกรอกเสมอ (NOT NULL on ciphertext column, validation at API boundary)

**Approval/rejection workflow (FR-11, FR-12, FR-14)**
- D-45: แยก 2 action: ตีกลับ (return → draft, non-terminal) vs ปฏิเสธถาวร (reject → rejected, terminal) — ทั้งสอง บังคับระบุเหตุผล
- State machine: `draft —submit→ pending_review —return+reason→ draft —reject+reason→ rejected —approve→ issued —cancel+reason→ cancelled`. rejected/cancelled เป็น terminal
- SoD (locked จาก CLAUDE.md): `approver_id != created_by` ทั้งใน code guard AND DB CHECK (defense-in-depth)
- D-52: กัน double-issuance ด้วย `SELECT ... FOR UPDATE` บน donation row + status precondition ภายใน issuance tx

**PII reveal & masking (FR-29, NFR-02)**
- D-46: reveal เลขเต็ม = Checker + Admin เท่านั้น (Maker เห็นเลขเต็มเฉพาะตอนกรอก/แก้ draft ของตัวเอง)
- ใช้ `pii.MaskNationalID` + `pii.CanRevealFull` จาก Phase 1 — ห้ามสร้างใหม่

**Consent capture (NFR-03)**
- D-49: consent_given + consent_at + consent_text_version + consent_purpose + retain_until + legal_basis ต่อ snapshot

**Receipt cancellation — Void & Reissue (FR-19)**
- D-47: ยกเลิก = Checker + Admin เท่านั้น, บังคับเหตุผล, เก็บเลขเดิม (ไม่ลบ, ไม่เกิด gap)
- D-50: Void (ยกเลิกเฉยๆ) vs Void & Reissue (ออกใบแทน = donation record ใหม่เดิน lifecycle ปกติ + link replaces/replaced_by)
- D-51: edonation_keyed flag — ถ้า true ต้องแสดง warning + บังคับเหตุผล RD reconciliation ก่อน cancel

**Search/filter scope (FR-10)**
- D-53: ค้นด้วย ชื่อ/ช่วงวันที่/สถานะ/เลขที่ใบเสร็จ เท่านั้น ไม่ค้นด้วยเลขภาษี/ปชช.

**Slip retention (D-54)**
- ไม่ hard-delete ไฟล์สลิป: soft-delete reference, ไฟล์ยังอยู่ใน object storage, audited

**Slip upload — Flow A (D-48)**
- แนบสลิปได้แต่ไม่บังคับ (optional); สร้าง MinIO/S3-compatible + magic-byte validation + size limit ในเฟสนี้; Phase 6 reuse seam

**Allocator seam from Phase 2**
- D-33: `Allocate(ctx, tx pgx.Tx, issueDate time.Time) (AllocatedReceipt, error)` — caller-managed tx
- D-35: เลขเกิดตอน commit เท่านั้น ห้าม pre-compute บน draft
- D-42: freeze formatted snapshot — แสดงจาก snapshot เสมอ

### Claude's Discretion

- schema รายละเอียดของ donation entity (ชื่อ column, FK ไป ledger receipt_numbers ตาม D-38, index สำหรับ search FR-10) ต้องครอบคลุม consent/reissue link/edonation_keyed column ใหม่ตาม decisions
- โครงสร้าง package ฝั่ง Go (internal/donation/, internal/storage/) ตาม pattern Phase 1
- Migration 000005+ (donation tables, FK, SoD CHECK, status enum/constraint)
- Object storage client library (minio-go ตาม CLAUDE.md) + endpoint/bucket config
- รูปแบบ outbox table + enqueue (transactional outbox ตาม CLAUDE.md) — Phase 3 เขียน enqueue, worker เป็น Phase 4
- ขอบเขต validation donor fields อื่น (email/address format)

### Deferred Ideas (OUT OF SCOPE)

- Donor master + dedup + blind index + per-donor rollup — future
- Flow B public donation form + slip upload จากผู้บริจาค + FR-08 queue — Phase 6
- PDF/email/outbox worker (FR-20..28, NFR-07) — Phase 4
- ข้อความลดหย่อน 1 เท่า/2 เท่า + template/config UI (FR-24, FR-33, NFR-09) — Phase 4
- e-Donation export + reports (FR-30/31/32) — Phase 5
</user_constraints>

---

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| FR-07 | Maker สร้างรายการบริจาคเอง (Flow A) | `internal/donation/` handler + service + migration 000005 |
| FR-09 | ดู/แก้ไขข้อมูลรายการก่อนอนุมัติ พร้อมดูสลิปแนบ | Edit draft API + slip_attachments table + MinIO GetObject |
| FR-11 | สถานะรายการชัดเจน: ร่าง → รอตรวจสอบ → อนุมัติ/ออกใบเสร็จ → ปฏิเสธ → ยกเลิก | donation_status enum + state machine guard |
| FR-10 | ค้นหา/กรอง (ชื่อ, ช่วงวันที่, สถานะ, เลขที่ใบเสร็จ) | PostgreSQL ILIKE + date range + enum + receipt_formatted index |
| FR-12 | Checker อนุมัติหรือตีกลับ พร้อมระบุเหตุผล | return/reject actions + reason NOT NULL guard |
| FR-14 | ใบเสร็จสร้างก็ต่อเมื่ออนุมัติ (ผู้สร้างอนุมัติตัวเองไม่ได้) | Issuance tx + SoD guard (code + DB CHECK) |
| FR-19 | ยกเลิกใบเสร็จใช้สถานะ "ยกเลิก" ไม่ลบเลข | cancel action + cancelled state keeps receipt_number_id |
| FR-29 | จัดเก็บข้อมูลผู้บริจาค (ชื่อ/เลขผู้เสียภาษี/ที่อยู่/อีเมล) | Snapshot columns + AES-256-GCM on national/tax ID |
| NFR-02 | เข้ารหัสข้อมูลอ่อนไหวขณะจัดเก็บ | crypto.EncryptField/DecryptField (Phase 1) wired to donor_tax_id |
| NFR-03 | บันทึก consent + วันเวลา + เวอร์ชันข้อความ + วัตถุประสงค์ | consent_given/consent_at/consent_text_version/consent_purpose columns (D-49) |
| NFR-04 | เลขใบเสร็จไม่ซ้ำแม้มีผู้ใช้พร้อมกัน | SELECT … FOR UPDATE บน donation row (D-52) + allocator Phase 2 (D-33) |
| NFR-05 | Audit log ลบไม่ได้ | audit.AppendAuditEntryTx inside issuance tx + ทุก action |
</phase_requirements>

---

## Summary

Phase 3 สร้าง flow แกนกลางของระบบ: Maker สร้าง/แก้/submit donation record (พร้อม PII เข้ารหัส), Checker อนุมัติ/ตีกลับ/ปฏิเสธ, และการอนุมัติ trigger issuance transaction เดียวที่ทำ 4 สิ่งพร้อมกัน (set status=issued + allocate gap-less number จาก Phase 2 + เขียน audit row + enqueue outbox job) หรือ rollback ทั้งหมด

สิ่งที่ซับซ้อนที่สุดใน Phase 3 คือ **issuance transaction** — ต้อง serialize concurrent approvals ด้วย `SELECT ... FOR UPDATE` บน donation row (D-52), เรียก allocator Phase 2 (D-33) ภายใน tx เดียวกัน, และ enqueue outbox job แบบ transactional เพื่อให้ Phase 4 worker (PDF+email) ทำงานนอก lock path เพื่อไม่กระทบ NFR-07 latency

Phase 3 เป็นเฟสแรกที่แตะ HTTP handler layer (Phase 1/2 เป็น backend-only). ยังไม่มี Next.js frontend ในโปรเจกต์ — ต้อง bootstrap ใน Wave 0 ของ UI path. Backend package pattern จาก `internal/users/` เป็น template สำหรับ `internal/donation/` package ใหม่

MinIO ยังไม่อยู่ใน docker-compose.yml — ต้อง add service + config env vars. `gabriel-vasile/mimetype` v1.4.13 มีใน go.mod แล้ว (indirect); เป็น direct dependency ใน Phase 3 สำหรับ magic-byte validation. `github.com/minio/minio-go/v7` v7.2.1 เป็น package ใหม่เดียวที่ต้องเพิ่ม

**Primary recommendation:** ใช้ `db.WithTx` + `receiptno.Allocator.Allocate` + `audit.AppendAuditEntryTx` + outbox INSERT ใน function closure เดียว — pattern นี้ให้ all-or-nothing guarantee โดยไม่ต้องเพิ่ม library ใหม่

---

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Donation CRUD (create/edit draft) | API / Backend | DB Storage | Business logic + validation ใน Go service; DB เก็บ state |
| State machine transitions | API / Backend | DB (enum + check constraints) | Guard logic ใน Go service; DB constraint เป็น defense-in-depth |
| SoD enforcement (approver ≠ creator) | API / Backend | DB (CHECK constraint) | Code guard primary; DB backstop secondary (CLAUDE.md requirement) |
| Issuance transaction (atomic: status+number+audit+outbox) | DB / Storage | API trigger | Transaction boundary ที่ DB level ผ่าน pgx.Tx; API trigger เท่านั้น |
| Gap-less receipt number | DB / Storage | — | Owned by Phase 2 allocator: counter table + SELECT FOR UPDATE |
| PII encryption at rest | API / Backend | — | App-level AES-256-GCM (PDPA, CLAUDE.md) — ห้ามฝากไว้ที่ DB pgcrypto |
| PII masking in responses | API / Backend | — | `pii.MaskNationalID` ใน response serialization layer |
| PII reveal (authorized) | API / Backend | Audit | `pii.CanRevealFull` gate → `crypto.DecryptField` → `audit.AppendAuditEntryTx` |
| Slip file storage | Object Storage (MinIO) | DB (reference) | ไม่เก็บ BLOB ใน DB; DB เก็บ object_key reference เท่านั้น (CLAUDE.md) |
| Magic-byte file validation | API / Backend | — | `gabriel-vasile/mimetype.DetectReader` ก่อน PutObject |
| Audit trail | DB (append-only) | — | `audit.AppendAuditEntryTx` inside tx; REVOKE UPDATE/DELETE (Phase 1) |
| Outbox enqueue | DB / Storage | — | INSERT outbox_jobs row ใน issuance tx — atomically linked to receipt issuance |
| Search/filter | DB / Storage | API | SQL query with index; API ทำ pagination + response mapping |
| Consent capture | API / Backend | DB | Validated at request boundary; persisted ใน donation snapshot |
| edonation_keyed guard | API / Backend | DB (column) | Guard logic ใน cancel service method; DB stores flag for Phase 5 |
| Back-office UI (forms, lists) | Frontend (Next.js) | API | Next.js 15 App Router calls Go API via OIDC bearer token |

---

## Standard Stack

### Core — Backend (Go, ทั้งหมด verified จาก go.mod)

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/gin-gonic/gin` | v1.12.0 | HTTP router + middleware | **ใช้งานจริงในโปรเจกต์** (go.mod + main.go); Phase 3 เป็นเฟสแรกที่ใช้ handler จริง |
| `github.com/jackc/pgx/v5` | v5.10.0 | PostgreSQL driver/pool + `pgx.Tx` | carrier ของ issuance tx; `pgx.Tx` คือ parameter ของ `Allocate()` |
| sqlc (codegen) | 1.x | Generate type-safe Go จาก SQL | ใช้ใน Phase 1/2; เพิ่ม donation queries ใน `internal/db/queries/donations.sql` |
| `github.com/golang-migrate/migrate/v4` | v4.19.1 | Schema migrations | `migrations/000005+` สำหรับ donation tables |
| `github.com/go-playground/validator/v10` | v10.30.3 | Runtime validation | ใช้ใน handler pattern Phase 1 (users/handler.go) |
| `go.uber.org/zap` | v1.28.0 | Structured logging | ใช้ทั่วโปรเจกต์; Pattern C: ห้าม log PII |
| `github.com/gabriel-vasile/mimetype` | v1.4.13 | Magic-byte file type detection | มีใน go.mod แล้ว (indirect); Phase 3 ใช้ direct สำหรับ slip validation |
| `github.com/testcontainers/testcontainers-go` | v0.43.0 | Integration test fixtures | `testutil.SetupTestPostgres(t)` pattern จาก Phase 2 |

### New Package — Phase 3 เท่านั้น

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/minio/minio-go/v7` | v7.2.1 | Object storage client (MinIO/S3) | Official MinIO Go client; S3-compatible; CLAUDE.md กำหนด `minio-go` explicitly |

[VERIFIED: Go proxy — `v7.2.1` confirmed via `go list -m github.com/minio/minio-go/v7@latest`]
[CITED: github.com/minio/minio-go — official MinIO Go client, High reputation, 753 code snippets on Context7]

### Supporting — Reused from Phase 1 (ไม่ต้องเพิ่ม dependency)

| Package (internal) | Purpose | Phase Origin |
|-------------------|---------|--------------|
| `internal/receiptno.Allocator` | gap-less number allocation | Phase 2 |
| `internal/crypto.EncryptField` / `DecryptField` | AES-256-GCM envelope PII | Phase 1 |
| `internal/pii.MaskNationalID` / `CanRevealFull` | masking + reveal gate | Phase 1 |
| `internal/audit.AppendAuditEntryTx` | in-tx audit append | Phase 1 |
| `internal/db.WithTx` | caller-managed tx helper | Phase 1 |
| `internal/auth.RequireRoles` / `KeycloakClaims` | RBAC guard + role constants | Phase 1 |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `minio-go/v7` PutObject | AWS SDK v2 S3 PutObject | minio-go รองรับ MinIO + S3 ในตัว; AWS SDK v2 ใช้เฉพาะ Phase 4 สำหรับ SES email |
| DB-backed outbox table | asynq (Redis) / River (Postgres) | DB-backed outbox ไม่เพิ่ม dependency; Phase 3 แค่ enqueue — worker ซับซ้อนกว่าเป็น Phase 4 |
| `gin.ShouldBindJSON` + validator | custom decoder | Pattern เดิมจาก Phase 1 users/handler.go — ความสม่ำเสมอสำคัญกว่า |

**Installation (new package only):**
```bash
cd donnarec-api && go get github.com/minio/minio-go/v7@v7.2.1
```

---

## Package Legitimacy Audit

> Phase 3 เพิ่มเพียง 1 Go package ใหม่. Go packages ไม่ผ่าน slopcheck (npm tool) — ใช้ Go module proxy + official source verification แทน.

| Package | Registry | Age | Downloads | Source Repo | Slopcheck Equiv | Disposition |
|---------|----------|-----|-----------|-------------|-----------------|-------------|
| `github.com/minio/minio-go/v7` v7.2.1 | Go proxy (GOPROXY) | 9+ ปี (repo created 2015) | Stars: 2.4k+; ใช้ใน production กว้างขวาง | github.com/minio/minio-go | OK — official MinIO org, High reputation (Context7) | Approved |
| `github.com/gabriel-vasile/mimetype` v1.4.13 | Go proxy (GOPROXY) | 6+ ปี | มีใน go.mod Phase 1 แล้ว (indirect) | github.com/gabriel-vasile/mimetype | OK — ใช้งานในโปรเจกต์อยู่แล้ว | Approved |

**Packages removed due to [SLOP] verdict:** none

**Packages flagged as suspicious [SUS]:** none

*Note: slopcheck ไม่รองรับ Go modules. Verification ใช้ Go module proxy (`go list -m <pkg>@latest`), official GitHub source, และ Context7 source reputation. `minio-go/v7` confirmed v7.2.1 via proxy.*

---

## Architecture Patterns

### System Architecture Diagram

```
Browser (Staff)
    │
    │ OIDC Token (Keycloak JWT)
    ▼
Next.js 15 App Router  ◄──── Keycloak (OIDC)
(donnarec-web)               port 8080
    │
    │ HTTP + Bearer Token
    ▼
Gin Router (donnarec-api:8000)
    │
    ├── RequireAuth middleware (validates Keycloak JWT)
    ├── RequireRoles middleware (Maker / Checker / Admin)
    │
    ├── DonationHandler
    │       │
    │       ├── create/edit/submit/view  ──────► DonationService
    │       │                                         │
    │       └── approve/return/reject/cancel          │  crypto.EncryptField (PII)
    │                                                 │  pii.MaskNationalID (response)
    │                                                 │  pii.CanRevealFull (reveal gate)
    │                                                 │
    │                                           db.WithTx (pgx.Tx)
    │                                                 │
    │                                     ┌───────────┼────────────────┐
    │                                     │           │                │
    │                              SELECT FOR UPDATE  │           outbox_jobs
    │                              (donation row)     │           (INSERT in tx)
    │                                     │    receiptno.Allocate     │
    │                              UPDATE donations   (Phase 2)        │
    │                              SET status=issued  │                │
    │                                     │   audit.AppendAuditEntryTx│
    │                                     │           │                │
    │                                     └───────────┴────────────────┘
    │                                           PostgreSQL 17
    │
    └── SlipHandler
            │
            ├── upload  ──► mimetype.DetectReader ──► minio.PutObject ──► MinIO
            └── view    ──► minio.PresignedGetObject (short-lived URL)
```

### Recommended Project Structure (Go backend)

```
donnarec-api/
├── internal/
│   ├── donation/           # NEW — Phase 3 core domain
│   │   ├── handler.go      # Gin HTTP handlers (Maker/Checker endpoints)
│   │   ├── service.go      # Business logic: state machine, issuance tx, SoD guard
│   │   ├── model.go        # Request/Response Go structs (never DB models directly)
│   │   ├── service_test.go # Unit tests (state machine, SoD, mock tx)
│   │   └── service_integration_test.go  # testcontainers: issuance tx, concurrency
│   ├── storage/            # NEW — MinIO/S3 client wrapper
│   │   ├── client.go       # NewStorageClient, PutSlip, PresignedGet
│   │   └── client_test.go  # Integration: testcontainers + minio container OR mock
│   ├── receiptno/          # Phase 2 — reuse Allocator unchanged
│   ├── crypto/             # Phase 1 — reuse EncryptField/DecryptField unchanged
│   ├── pii/                # Phase 1 — reuse MaskNationalID/CanRevealFull unchanged
│   ├── audit/              # Phase 1 — reuse AppendAuditEntryTx unchanged
│   ├── auth/               # Phase 1 — reuse RequireRoles/KeycloakClaims unchanged
│   └── db/
│       ├── queries/
│       │   ├── donations.sql     # NEW: sqlc queries for donations
│       │   └── outbox.sql        # NEW: sqlc queries for outbox_jobs
│       └── generated/            # regenerate after new .sql files
├── migrations/
│   ├── 000005_donations.up.sql   # NEW: donation entity + enums + SoD CHECK
│   ├── 000005_donations.down.sql
│   ├── 000006_slip_attachments.up.sql  # NEW: slip reference table
│   ├── 000006_slip_attachments.down.sql
│   ├── 000007_outbox_jobs.up.sql       # NEW: transactional outbox
│   └── 000007_outbox_jobs.down.sql
└── docker-compose.yml      # ADD minio service + MINIO_* env vars
```

### Pattern 1: Issuance Transaction (All-or-Nothing)

**What:** approve action ทำ 4 สิ่งใน tx เดียว: lock donation row → check preconditions → allocate number → update status → write audit → enqueue outbox

**When to use:** ทุกครั้งที่ Checker POST /api/donations/:id/approve

```go
// Source: patterns from internal/db/helpers.go (db.WithTx) + internal/receiptno/allocator.go
// + internal/audit/service.go (AppendAuditEntryTx) — all Phase 1/2 patterns

func (s *DonationService) Approve(ctx context.Context, donationID uuid.UUID, claims auth.KeycloakClaims) error {
    approverID, _ := uuid.Parse(claims.Subject)

    return db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
        qtx := s.queries.WithTx(tx)

        // Step 1: Lock donation row FOR UPDATE (D-52 — gäns double-issuance)
        donation, err := qtx.LockDonationForUpdate(ctx, donationID)
        if err != nil { return fmt.Errorf("lock donation: %w", err) }

        // Step 2: Precondition — only pending_review can be approved
        if donation.Status != "pending_review" {
            return ErrInvalidTransition
        }

        // Step 3: SoD — approver must not be the creator (D-04, CLAUDE.md)
        if donation.CreatedBy == approverID {
            return ErrSoDViolation  // 403 at handler layer
        }

        // Step 4: Allocate gap-less receipt number (D-33 — Phase 2 allocator)
        // issueDate = now (wall clock OK here; DB NOW() preferred for consistency)
        receipt, err := s.allocator.Allocate(ctx, tx, time.Now())
        if err != nil { return fmt.Errorf("allocate: %w", err) }

        // Step 5: Update donation status + stamp receipt fields
        err = qtx.IssueDonation(ctx, db.IssueDonationParams{
            ID:               donationID,
            ApprovedBy:       approverID,
            ApprovedAt:       time.Now(),
            ReceiptNumberID:  receipt.ID,  // FK to receipt_numbers ledger (D-38)
            ReceiptFormatted: receipt.Formatted,  // frozen snapshot (D-42)
        })
        if err != nil { return fmt.Errorf("issue donation: %w", err) }

        // Step 6: Write audit entry in the same tx (atomic — not best-effort)
        err = s.auditSvc.AppendAuditEntryTx(ctx, tx, audit.AuditEntry{
            ActorID:    claims.Subject,
            ActorEmail: claims.ActorIdentity(),
            Action:     "donation.approve",
            Resource:   "/api/donations/" + donationID.String() + "/approve",
            AfterJSON:  marshalJSON(map[string]any{"receipt": receipt.Formatted}),
        })
        if err != nil { return fmt.Errorf("audit: %w", err) }

        // Step 7: Enqueue outbox job (Phase 4 worker consumes — NEVER render PDF here)
        err = qtx.EnqueueOutboxJob(ctx, db.EnqueueOutboxJobParams{
            JobType: "issue_receipt",
            Payload: marshalJSON(map[string]any{"donation_id": donationID}),
        })
        if err != nil { return fmt.Errorf("enqueue outbox: %w", err) }

        return nil  // commit: all 4 effects or none
    })
}
```

### Pattern 2: SoD DB Check Constraint (Defense-in-Depth)

**What:** DB-level CHECK constraint ที่ป้องกัน approver == creator แม้ application logic ถูก bypass

**When to use:** ใน migration 000005 (ไม่มี exception)

```sql
-- Source: CLAUDE.md §"Auth & RBAC" — SoD requirement
-- เป็น backstop ไม่ใช่ primary guard; primary guard อยู่ใน DonationService.Approve()
ALTER TABLE donations ADD CONSTRAINT chk_sod_approver
    CHECK (approved_by IS NULL OR approved_by != created_by);
```

### Pattern 3: PII Encrypt on Write, Mask on Read

**What:** เข้ารหัส national/tax ID ตอนเขียน, decrypt เฉพาะ authorized reveal, mask ทุกที่อื่น

**When to use:** create/update donation handler (encrypt), GET /donation response (mask), GET /donation/pii endpoint (decrypt + audit)

```go
// Source: internal/crypto/envelope.go (Phase 1)
// Write path — encrypt before INSERT
ciphertext, wrappedDEK, err := crypto.EncryptField(ctx, s.keyProvider, []byte(req.DonorTaxID))
if err != nil { return fmt.Errorf("encrypt tax id: %w", err) }
// Store ciphertext in donor_tax_id_enc column, wrappedDEK in donor_tax_id_dek column

// Read path — always mask (default)
// Source: internal/pii/mask.go (Phase 1)
resp.DonorTaxIDMasked = pii.MaskNationalID(plaintext)  // "x-xxxx-xxxxx-x1234"

// Authorized reveal path (Checker/Admin only) — D-46, D-13
if !pii.CanRevealFull(claims) { return http.StatusForbidden }
plaintext, err := crypto.DecryptField(ctx, s.keyProvider, ciphertext, wrappedDEK)
// MUST write audit entry before returning plaintext
s.auditSvc.AppendAuditEntryTx(ctx, tx, audit.AuditEntry{Action: "pii.reveal", ...})
```

### Pattern 4: Magic-Byte Slip Validation + Object Storage

**What:** ตรวจ content จริงจาก bytes ก่อน upload — ไม่เชื่อ Content-Type header หรือ file extension

**When to use:** POST /api/donations/:id/slip

```go
// Source: github.com/gabriel-vasile/mimetype (already in go.mod)
// + github.com/minio/minio-go/v7 (Context7 verified)

const maxSlipSize = 10 << 20  // 10 MB

func (s *StorageClient) PutSlip(ctx context.Context, r io.Reader, size int64, donationID string) (string, error) {
    // Enforce size limit
    lr := io.LimitReader(r, maxSlipSize+1)

    // Buffer enough bytes for magic-byte detection (mimetype reads first N bytes)
    buf := make([]byte, 512)
    n, err := io.ReadFull(lr, buf)
    if err != nil && err != io.ErrUnexpectedEOF { return "", err }

    mime := mimetype.Detect(buf[:n])

    // Only allow: image/jpeg, image/png, application/pdf
    allowed := map[string]bool{
        "image/jpeg":      true,
        "image/png":       true,
        "application/pdf": true,
    }
    if !allowed[mime.String()] {
        return "", ErrUnsupportedFileType
    }

    // Reassemble reader: prepend consumed bytes
    combined := io.MultiReader(bytes.NewReader(buf[:n]), lr)

    objectKey := "slips/" + donationID + "/" + uuid.NewString() + mime.Extension()
    _, err = s.client.PutObject(ctx, s.bucket, objectKey, combined, size, minio.PutObjectOptions{
        ContentType: mime.String(),
    })
    return objectKey, err
}
```

### Pattern 5: MinIO Client Initialization

**What:** สร้าง MinIO client จาก env config — เหมือน pattern config.go Phase 1

```go
// Source: github.com/minio/minio-go/v7 (Context7 docs verified)
import (
    "github.com/minio/minio-go/v7"
    "github.com/minio/minio-go/v7/pkg/credentials"
)

func NewMinIOClient(endpoint, accessKey, secretKey string, secure bool) (*minio.Client, error) {
    return minio.New(endpoint, &minio.Options{
        Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
        Secure: secure,
    })
}
// Config env vars: MINIO_ENDPOINT, MINIO_ACCESS_KEY, MINIO_SECRET_KEY, MINIO_BUCKET, MINIO_SECURE
```

### Pattern 6: Transactional Outbox Enqueue

**What:** INSERT outbox_jobs row ใน issuance tx — job exists IFF receipt was issued

**When to use:** ใน issuance transaction Step 7 (Pattern 1 ด้านบน)

```sql
-- Source: outbox pattern (CLAUDE.md §"Email Delivery" + §"What NOT to Use")
-- outbox_jobs table (migration 000007)
INSERT INTO outbox_jobs (job_type, payload, status)
VALUES (@job_type, @payload, 'pending')
-- Phase 4 worker polls this table; Phase 3 แค่ INSERT
```

### Anti-Patterns to Avoid

- **ออกเลขในจุดอื่น:** ห้ามเรียก `Allocate()` นอก issuance tx; ห้าม pre-set receipt_number บน draft (D-35)
- **Render PDF ใน issuance tx:** ถือ row lock นานเป็นวินาที; enqueue outbox แล้วปล่อย tx เร็ว (CLAUDE.md)
- **log.Error(err, "national_id:", req.TaxID):** ห้าม log PII ใน error context (Pattern C Phase 1)
- **ใช้ `status='pending_review'` string literal:** ใช้ Go constant หรือ DB enum string จาก sqlc-generated model
- **เช็ค SoD เฉพาะใน service layer:** ต้องมีทั้ง code guard AND DB CHECK constraint (CLAUDE.md defense-in-depth)
- **ลบ record ที่ถูก cancel:** ใช้ status=cancelled เท่านั้น ห้าม DELETE (FR-19, immutability requirement)
- **เชื่อ Content-Type header ของ upload:** ใช้ magic-byte detection เสมอ (CLAUDE.md, D-48)

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Gap-less receipt number | Custom counter logic | `internal/receiptno.Allocator.Allocate()` Phase 2 | สร้างแล้ว, tested under concurrency |
| PII envelope encryption | Custom AES wrapper | `internal/crypto.EncryptField/DecryptField` Phase 1 | DEK rotation, key hygiene, authenticated encryption ทำแล้ว |
| PII masking + reveal gate | Custom mask function | `internal/pii.MaskNationalID` + `CanRevealFull` Phase 1 | Format spec, edge cases, role logic ทำแล้ว |
| Audit hash-chain | Custom logging | `internal/audit.AppendAuditEntryTx` Phase 1 | advisory lock serialization, SHA-256 chain, REVOKE backstop ซับซ้อนมาก |
| DB transaction wrapper | Manual `pool.Begin/Commit` | `internal/db.WithTx()` Phase 1 | deferred rollback pattern, error wrapping ทำแล้ว |
| File type detection | Check file extension / Content-Type header | `github.com/gabriel-vasile/mimetype.Detect()` | extension/header spoofable; magic bytes reliable |
| Object storage client | Raw HTTP to MinIO API | `github.com/minio/minio-go/v7` | multipart upload, retry, S3 signing handled |
| Async email/PDF job queue | Goroutine with channel | DB outbox_jobs table + Phase 4 worker | durability: goroutine state lost on restart; DB outbox survives |

**Key insight:** Phase 3 เขียน code ใหม่น้อยมาก เพราะ crypto/pii/audit/receiptno ทำแล้วใน Phase 1/2 — งานหลักคือ wiring ใน issuance transaction และ donation domain logic

---

## Common Pitfalls

### Pitfall 1: Double-Issuance Race Condition
**What goes wrong:** Checker 2 คนกด approve record เดียวกันพร้อมกัน → 2 receipt numbers ออกสำหรับ donation เดียว
**Why it happens:** `SELECT → check status → UPDATE` ไม่ atomic โดยไม่มี lock; READ COMMITTED ไม่ป้องกัน non-repeatable reads ข้าม statements
**How to avoid:** `SELECT ... FOR UPDATE` บน donation row เป็น Step แรกของ issuance tx (D-52) — หนึ่ง tx ถือ lock อีกคน block จนกว่าจะ commit แล้วเห็น status=issued → reject
**Warning signs:** ถ้าไม่มี `FOR UPDATE` ใน `LockDonationForUpdate` query; ถ้า test TestConcurrentApproval ไม่อยู่ใน test suite

### Pitfall 2: SoD Enforced Only in Code (No DB Backstop)
**What goes wrong:** Direct DB access, future migration, หรือ bug ใน service layer ทำให้ approver = creator
**Why it happens:** ไม่มี DB-level constraint → ข้อมูลผิดในระดับที่ audit ตามจับไม่ได้ทันที
**How to avoid:** `CHECK (approved_by IS NULL OR approved_by != created_by)` ใน migration 000005 (CLAUDE.md requirement) + code guard ใน service
**Warning signs:** migration ไม่มี `chk_sod_approver` constraint

### Pitfall 3: receipt_number_id Set on Non-Issued Records
**What goes wrong:** draft/pending_review record มี receipt_number_id → number "used up" ก่อนออกจริง
**Why it happens:** Set receipt_number_id ก่อน commit issuance tx, หรือ pre-compute บน submit
**How to avoid:** `receipt_number_id` column มีค่าเฉพาะ status=issued (Phase 3 sets on commit only); DB CHECK constraint บังคับ
**Warning signs:** handler submit ที่ call Allocate(); allocator ถูกเรียก outside issuance tx

### Pitfall 4: Audit Entry NOT in Issuance Transaction
**What goes wrong:** audit เขียนหลัง commit (best-effort path) → issuance สำเร็จแต่ audit หายได้ถ้า crash ระหว่างกลาง
**Why it happens:** ใช้ `auditSvc.AppendAuditEntry()` (own-tx) แทน `AppendAuditEntryTx(ctx, tx, ...)` (in-tx)
**How to avoid:** ใช้ `AppendAuditEntryTx` ด้วย `tx` จาก issuance transaction เสมอ (NFR-05 requires atomicity)
**Warning signs:** audit import ใช้ `AppendAuditEntry()` แทน `AppendAuditEntryTx()` ใน approve flow

### Pitfall 5: Cancel Issued Receipt Without Retaining Number
**What goes wrong:** hard-delete donation record หรือ null receipt_number_id เมื่อ cancel → gap ใน receipt sequence
**Why it happens:** "cancel" แปลว่า "remove" — misconception
**How to avoid:** status=cancelled แต่ `receipt_number_id`, `receipt_formatted` ยังคงค่าเดิม; constraint ใน migration: `CHECK ((status IN ('issued','cancelled') AND receipt_number_id IS NOT NULL) OR ...)` (FR-19)
**Warning signs:** UpdateDonationStatus query ที่ set receipt_number_id = NULL เมื่อ cancel

### Pitfall 6: PII Leakage via Log / Error Message
**What goes wrong:** `donor_tax_id` plaintext อยู่ใน error message หรือ zap log field
**Why it happens:** copy-paste error context เข้า logger; validator error messages ที่แสดง input value
**How to avoid:** Pattern C (Phase 1): ห้าม log PII; log donation_id + status เท่านั้น; ใช้ masked value ถ้าต้อง log identifier
**Warning signs:** `zap.String("tax_id", req.DonorTaxID)` ใน log calls

### Pitfall 7: edonation_keyed Cancel Without Warning Bypassed
**What goes wrong:** ระบบยกเลิกใบที่คีย์ RD แล้วโดยไม่บังคับยืนยัน → ผู้บริจาคมีสิทธิลดหย่อนค้างใน e-Donation ทั้งที่ใบถูกยกเลิก
**Why it happens:** cancel handler ไม่เช็ค `edonation_keyed` flag ก่อน
**How to avoid:** cancel service method เช็ค `edonation_keyed` flag → ถ้า true ต้องมี `rd_confirmation_reason` field ใน request body (D-51); validation ที่ handler
**Warning signs:** cancel API รับเพียง `reason` field โดยไม่ handle `edonation_keyed=true` case

---

## Code Examples

### SQL: Donation Table (Migration 000005)

```sql
-- Source: decisions D-43..D-52 + CLAUDE.md §"PII Encryption" + CLAUDE.md §"Auth & RBAC"

CREATE TYPE donation_status AS ENUM (
    'draft', 'pending_review', 'issued', 'rejected', 'cancelled'
);

CREATE TABLE donations (
    id                      UUID        PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Maker & lifecycle
    created_by              UUID        NOT NULL REFERENCES users(id),
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    status                  donation_status NOT NULL DEFAULT 'draft',

    -- Donor snapshot (immutable after issue, D-43)
    donor_name              TEXT        NOT NULL,
    donor_address           TEXT        NOT NULL DEFAULT '',
    donor_email             TEXT,                               -- optional

    -- PII: national/tax ID encrypted at rest (PDPA, NFR-02, D-44)
    donor_tax_id_enc        BYTEA       NOT NULL,               -- ciphertext
    donor_tax_id_dek        BYTEA       NOT NULL,               -- wrapped DEK

    -- Donation detail
    amount                  NUMERIC(15,2) NOT NULL CHECK (amount > 0),
    donated_at              DATE        NOT NULL,
    notes                   TEXT,

    -- Consent capture (D-49, NFR-03)
    consent_given           BOOLEAN     NOT NULL DEFAULT false,
    consent_at              TIMESTAMPTZ,
    consent_text_version    TEXT,
    consent_purpose         TEXT,

    -- Retention (Phase 1 model)
    retain_until            DATE,
    legal_basis             TEXT        NOT NULL DEFAULT 'tax_obligation',

    -- Submit
    submitted_at            TIMESTAMPTZ,

    -- Return / Reject (Checker → Maker loop or terminal)
    reviewed_by             UUID        REFERENCES users(id),
    reviewed_at             TIMESTAMPTZ,
    review_reason           TEXT,   -- mandatory on return/reject

    -- Approval (issuance)
    approved_by             UUID        REFERENCES users(id),
    approved_at             TIMESTAMPTZ,

    -- Receipt number (FK to Phase 2 ledger, D-38)
    receipt_number_id       BIGINT      REFERENCES receipt_numbers(id),
    receipt_formatted       TEXT,       -- frozen snapshot (D-42)

    -- Cancellation (D-47)
    cancelled_by            UUID        REFERENCES users(id),
    cancelled_at            TIMESTAMPTZ,
    cancel_reason           TEXT,
    edonation_keyed         BOOLEAN     NOT NULL DEFAULT false,  -- D-51

    -- Void & Reissue links (D-50, self-FK)
    replaces                UUID        REFERENCES donations(id),  -- this record replaces
    replaced_by             UUID        REFERENCES donations(id),  -- replaced by this record

    -- SoD DB backstop (CLAUDE.md — defense-in-depth)
    CONSTRAINT chk_sod_approver
        CHECK (approved_by IS NULL OR approved_by != created_by),

    -- Receipt number must be set IFF status is issued or cancelled
    CONSTRAINT chk_receipt_only_on_issued_or_cancelled
        CHECK (
            (status IN ('issued','cancelled') AND receipt_number_id IS NOT NULL AND receipt_formatted IS NOT NULL)
            OR (status NOT IN ('issued','cancelled') AND receipt_number_id IS NULL AND receipt_formatted IS NULL)
        )
);

-- Indexes for FR-10 search (D-53: name, date, status, receipt_no)
CREATE INDEX idx_donations_donor_name       ON donations (donor_name);
CREATE INDEX idx_donations_donated_at       ON donations (donated_at);
CREATE INDEX idx_donations_status           ON donations (status);
-- receipt_formatted lookup via receipt_numbers.idx_receipt_numbers_formatted (Phase 2)
CREATE INDEX idx_donations_receipt_number_id ON donations (receipt_number_id) WHERE receipt_number_id IS NOT NULL;
CREATE INDEX idx_donations_created_by       ON donations (created_by);
CREATE INDEX idx_donations_approved_by      ON donations (approved_by) WHERE approved_by IS NOT NULL;

-- Permissions
GRANT SELECT, INSERT, UPDATE ON donations TO donnarec_app;
-- No DELETE — records are never deleted (FR-19, immutability)
REVOKE DELETE ON donations FROM donnarec_app;
GRANT USAGE, SELECT ON SEQUENCE donations_id_seq TO donnarec_app;
```

### SQL: Outbox Jobs Table (Migration 000007)

```sql
-- Source: CLAUDE.md §"Email Delivery" — transactional outbox + worker pattern
CREATE TABLE outbox_jobs (
    id          BIGSERIAL   PRIMARY KEY,
    job_type    TEXT        NOT NULL,   -- 'issue_receipt'
    payload     JSONB       NOT NULL,   -- {"donation_id": "uuid"}
    status      TEXT        NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending','processing','done','failed')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    attempts    INT         NOT NULL DEFAULT 0,
    last_error  TEXT
);

CREATE INDEX idx_outbox_jobs_pending ON outbox_jobs (status, created_at)
    WHERE status IN ('pending','failed');

GRANT SELECT, INSERT, UPDATE ON outbox_jobs TO donnarec_app;
GRANT USAGE, SELECT ON SEQUENCE outbox_jobs_id_seq TO donnarec_app;
```

### sqlc Query: Lock Donation for Update

```sql
-- Source: patterns from internal/db/queries/receiptno.sql (Phase 2 FOR UPDATE pattern)
-- File: internal/db/queries/donations.sql

-- name: LockDonationForUpdate :one
SELECT id, status, created_by, receipt_number_id, edonation_keyed
FROM donations
WHERE id = @id
FOR UPDATE;

-- name: IssueDonation :exec
UPDATE donations
SET status           = 'issued',
    approved_by      = @approved_by,
    approved_at      = @approved_at,
    receipt_number_id = @receipt_number_id,
    receipt_formatted = @receipt_formatted,
    updated_at       = now()
WHERE id = @id
  AND status = 'pending_review';  -- precondition: extra safety

-- name: SearchDonations :many
-- FR-10: ค้นหาด้วย name/date range/status/receipt formatted (D-53)
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

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| ใช้ SEQUENCE/SERIAL สำหรับเลขใบเสร็จ | Counter table + SELECT FOR UPDATE (Phase 2) | Phase 2 decision D-33 | gap-less guarantee; rollback-safe |
| Render PDF synchronously ใน approval request | Enqueue outbox job; worker เป็น Phase 4 | CLAUDE.md design | row lock ถือสั้น; approval fast |
| เก็บ file upload เป็น BLOB ใน DB | Object storage (MinIO/S3) + DB reference | CLAUDE.md design | DB ไม่ bloat; signed URL pattern |
| Blind index สำหรับ encrypted search | Snapshot-only, ไม่ค้นด้วยเลขภาษี (D-53) | Phase 3 D-43 | simple, PDPA-safe; deferred |

**Deprecated/outdated:**
- GORM / ent ORM: ไม่ใช้ในโปรเจกต์นี้ — sqlc + raw FOR UPDATE query (D-23)
- `pgcrypto` สำหรับ PII: ใช้ app-level AES-256-GCM แทน (CLAUDE.md)
- self-signed upload via form: Phase 6 feature; Phase 3 ใช้ staff upload เท่านั้น

---

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | HTTP router คือ Gin v1.12.0 (ไม่ใช่ chi ตาม CLAUDE.md TL;DR) | Standard Stack | handler pattern ต้องเปลี่ยน; CONTEXT.md ยืนยัน "gin ใน go.mod" [ASSUMED from code reading, not external source] |
| A2 | Next.js frontend bootstrap เป็นส่วนหนึ่งของ Phase 3 (UI hint: yes ใน ROADMAP) | Architecture | ถ้าเลื่อน frontend ไป Phase อื่น → planner ปรับ wave structure ได้ |
| A3 | Outbox polling worker ใน Phase 4 จะ poll outbox_jobs table ตรงๆ (ไม่ใช้ Redis) | Don't Hand-Roll | ถ้า Phase 4 เลือก Redis-backed asynq → ต้องเปลี่ยน outbox table structure เล็กน้อย |
| A4 | MinIO bucket name: `donnarec-slips` (planner กำหนดในช่อง discretion) | Architecture | ปรับได้ผ่าน env MINIO_BUCKET |
| A5 | File size limit สำหรับ slip: 10 MB | Pattern 4 | ต้องยืนยัน ops ว่า hospital network รับได้ |

**A1 clarification:** Gin confirmed [VERIFIED: go.mod line `github.com/gin-gonic/gin v1.12.0` + main.go imports + users/handler.go]

---

## Open Questions

1. **Next.js frontend ใน Phase 3 หรือเปล่า?**
   - What we know: ROADMAP §Phase 3 มี "UI hint: yes"; ไม่มี frontend ใน repo; CONTEXT.md ไม่มี UI decisions
   - What's unclear: planner จะ bootstrap Next.js ใน Phase 3 หรือ defer ไป Phase 4?
   - Recommendation: ถ้า MVP = vertical slice end-to-end → bootstrap Next.js + minimal forms ใน Phase 3; ถ้า API-first → defer UI

2. **`issueDate` parameter ใน Allocate() — ใช้ค่าอะไร?**
   - What we know: D-40 กำหนดว่า allocator รับ issueDate จาก caller; Phase 3 approval context คือ "เวลาอนุมัติ"
   - What's unclear: ควร pass `time.Now()` ณ เวลา approve หรือ DB `NOW()` ภายใน tx?
   - Recommendation: ใช้ `time.Now()` ใน Go service layer (consistent กับ approved_at field); ผลต่าง < millisecond ใน single-server scenario; สำคัญกว่าคือ timezone ต้อง UTC → allocator normalize เป็น Asia/Bangkok เอง (D-40)

3. **Slip attachment ใน migration เดี่ยวหรือ รวมกับ donations?**
   - What we know: slip_attachments FK ไป donations; แยกจะง่าย rollback
   - Recommendation: แยก migration 000006 (slip_attachments) — ถ้า MinIO setup ยังไม่พร้อม planner ข้ามได้โดยไม่กระทบ donation table

---

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|-------------|-----------|---------|----------|
| PostgreSQL 17 | Core data layer | ✓ (docker-compose) | 17 (image: postgres:17) | — |
| Keycloak | OIDC auth | ✓ (docker-compose) | 26.6.3 | — |
| Go | Backend | ✓ | 1.26.0 (go.mod: 1.25.1) | — |
| Docker | Local dev | ✓ | 29.6.0 | — |
| Docker Compose | Local dev stack | ✓ | v5.0.1 | — |
| MinIO | Slip file storage | ✗ **NOT in docker-compose.yml** | — | Add service (see below) |
| Node.js | Next.js frontend (if UI in Phase 3) | ✓ | v25.6.1 | — |
| `minio-go/v7` | Object storage client | ✗ (not in go.mod) | v7.2.1 available | `go get github.com/minio/minio-go/v7@v7.2.1` |

**Missing dependencies with no fallback:**
- **MinIO service** — ต้อง add ใน `docker-compose.yml` ก่อน slip upload feature ใช้งานได้; Wave 0 task

**MinIO docker-compose service addition (Wave 0):**
```yaml
minio:
  image: minio/minio:latest
  command: server /data --console-address ":9001"
  environment:
    MINIO_ROOT_USER: ${MINIO_ACCESS_KEY:-minioadmin}
    MINIO_ROOT_PASSWORD: ${MINIO_SECRET_KEY:-minioadmin}
  ports:
    - "9000:9000"
    - "9001:9001"
  volumes:
    - minio_data:/data
  healthcheck:
    test: ["CMD", "curl", "-f", "http://localhost:9000/minio/health/live"]
    interval: 10s
    timeout: 5s
    retries: 5
  restart: unless-stopped
```

---

## Validation Architecture

> `workflow.nyquist_validation: true` ใน .planning/config.json — section นี้ required

### Test Framework

| Property | Value |
|----------|-------|
| Framework | `go test` + `github.com/stretchr/testify` v1.11.1 |
| Config file | `donnarec-api/` (no separate config; `go test ./...` from module root) |
| Quick run command | `go test -count=1 -run TestXxx ./internal/donation/... -timeout 120s` |
| Full suite command | `go test -count=1 -race ./... -timeout 600s` |

### The 7 Hardest Invariants — Must Have Tests

This is the most critical section. Phase 3 has 7 invariants that must be test-verified before the phase closes:

#### INV-1: Atomic Issuance — All 4 Effects or None (SC#3)

**Test:** inject error at each step in the issuance sequence; assert no partial state

```go
// go test -count=1 -run TestIssuanceTransaction_RollbackOnError ./internal/donation/...
// Uses testutil.SetupTestPostgres(t) — requires testcontainers (Docker)

func TestIssuanceTransaction_RollbackOnError(t *testing.T) {
    // Scenario A: error after Allocate, before IssueDonation UPDATE
    // Assert: receipt_numbers ledger has 0 rows, donation.status still 'pending_review'

    // Scenario B: error after IssueDonation, before audit AppendAuditEntryTx
    // Assert: donation.status = 'pending_review', outbox_jobs has 0 rows

    // Scenario C: happy path
    // Assert: status='issued', receipt_number_id set, 1 audit row, 1 outbox_jobs row
}
```

| Req | Behavior | Test Type | Command | File Exists? |
|-----|----------|-----------|---------|--------------|
| SC#3 | All 4 effects commit together or none | integration | `go test -count=1 -run TestIssuanceTransaction ./internal/donation/... -timeout 120s` | ❌ Wave 0 |
| SC#3 | outbox job exists IFF receipt issued | integration | `go test -count=1 -run TestOutboxAtomicity ./internal/donation/... -timeout 120s` | ❌ Wave 0 |

#### INV-2: Segregation of Duties (SC#2)

**Test:** approverID == creatorID must be blocked at both code layer and DB layer

```go
// go test -count=1 -run TestSoDEnforcement ./internal/donation/...

func TestSoD_ApproverCannotBeCreator(t *testing.T) {
    // Create donation with userA, attempt approve with userA claims
    // Assert: service returns ErrSoDViolation; HTTP handler returns 403
}

func TestSoD_DBCheckConstraint(t *testing.T) {
    // Use superuser pool to directly INSERT with approved_by = created_by
    // Assert: pgerrcode.CheckViolation returned from DB
    // (proves DB backstop fires even if code is bypassed)
}
```

| Req | Behavior | Test Type | Command | File Exists? |
|-----|----------|-----------|---------|--------------|
| FR-14 / D-04 | Service rejects approver == creator | unit | `go test -count=1 -run TestSoDEnforcement ./internal/donation/... -timeout 30s` | ❌ Wave 0 |
| CLAUDE.md | DB CHECK fires on direct insert | integration | `go test -count=1 -run TestSoD_DBCheckConstraint ./internal/donation/... -timeout 120s` | ❌ Wave 0 |

#### INV-3: Double-Issuance Under Concurrency (D-52 + NFR-04)

**Test:** N goroutines approve same record simultaneously → exactly 1 succeeds

```go
// go test -count=1 -race -run TestConcurrentApproval ./internal/donation/... -timeout 300s

func TestConcurrentApproval_ExactlyOneSucceeds(t *testing.T) {
    // Setup: 1 donation in status=pending_review
    // N=5 goroutines all call Approve() concurrently
    // Assert: exactly 1 goroutine succeeds (nil error)
    //         exactly 4 return ErrInvalidTransition
    //         receipt_numbers ledger has exactly 1 row for this fiscal year
    //         donation.status = 'issued'
}
```

| Req | Behavior | Test Type | Command | File Exists? |
|-----|----------|-----------|---------|--------------|
| D-52 / NFR-04 | Concurrent approvals issue exactly 1 receipt | integration + -race | `go test -count=1 -race -run TestConcurrentApproval ./internal/donation/... -timeout 300s` | ❌ Wave 0 |

#### INV-4: Gap-Less Number Retained on Cancel (SC#4 + FR-19)

**Test:** issue → cancel → issue new → new number is consecutive (no re-use, no gap at cancel)

```go
// go test -count=1 -run TestCancelRetainsNumber ./internal/donation/...

func TestCancelRetainsReceiptNumber(t *testing.T) {
    // Issue donation A → receipt 2569/000001
    // Cancel donation A → status='cancelled', receipt_number_id still set
    // Issue donation B → receipt 2569/000002 (NOT 2569/000001 re-issued)
    // Assert: donation A has receipt_number_id != nil after cancel
    // Assert: donation B receipt = A receipt running_no + 1
}
```

| Req | Behavior | Test Type | Command | File Exists? |
|-----|----------|-----------|---------|--------------|
| FR-19 | Cancel keeps receipt_number_id, no gap | integration | `go test -count=1 -run TestCancelRetainsNumber ./internal/donation/... -timeout 120s` | ❌ Wave 0 |

#### INV-5: PII Encryption-at-Rest + Role-Gated Reveal (SC#5 + FR-29 + NFR-02)

**Test:** national ID stored as ciphertext; only Checker/Admin can decrypt; Maker gets masked

```go
// go test -count=1 -run TestPII ./internal/donation/...

func TestPII_TaxIDStoredEncrypted(t *testing.T) {
    // Create donation with donor_tax_id = "1234567890123"
    // Read raw DB: SELECT donor_tax_id_enc FROM donations WHERE id = X
    // Assert: raw bytes != "1234567890123" (not plaintext)
    // Assert: donor_tax_id_enc is non-nil BYTEA
}

func TestPII_RevealRequiresCheckerOrAdmin(t *testing.T) {
    // Call reveal endpoint with Maker claims → expect ErrForbidden (403)
    // Call reveal endpoint with Checker claims → expect plaintext "1234567890123"
    // Assert: audit_log has row with action="pii.reveal" for Checker call
}

func TestPII_MaskDefault(t *testing.T) {
    // GET /api/donations/:id with Maker claims
    // Assert: donor_tax_id field in response = "x-xxxx-xxxxx-x0123" (last 4 digits)
}
```

| Req | Behavior | Test Type | Command | File Exists? |
|-----|----------|-----------|---------|--------------|
| FR-29 / NFR-02 | Tax ID stored as ciphertext | integration | `go test -count=1 -run TestPII_TaxIDStoredEncrypted ./internal/donation/... -timeout 120s` | ❌ Wave 0 |
| D-46 | Maker gets 403 on reveal; Checker gets plaintext + audit | integration | `go test -count=1 -run TestPII_RevealRequiresCheckerOrAdmin ./internal/donation/... -timeout 120s` | ❌ Wave 0 |
| D-46 | Default response masks tax ID | unit | `go test -count=1 -run TestPII_MaskDefault ./internal/donation/... -timeout 30s` | ❌ Wave 0 |

#### INV-6: State Machine Transitions (SC#1 + FR-11)

**Test:** invalid transitions return error; valid transitions succeed

```go
// go test -count=1 -run TestStateMachine ./internal/donation/...

func TestStateMachine_InvalidTransitions(t *testing.T) {
    cases := []struct{ from, action string; expectErr bool }{
        {"draft", "approve", true},           // must go through pending_review
        {"rejected", "submit", true},          // terminal state
        {"rejected", "approve", true},         // terminal state
        {"cancelled", "approve", true},        // terminal state
        {"pending_review", "submit", true},    // already submitted
        {"issued", "submit", true},            // already issued
        {"draft", "submit", false},            // valid
        {"pending_review", "approve", false},  // valid (SoD satisfied)
        {"pending_review", "return", false},   // valid (with reason)
        {"pending_review", "reject", false},   // valid (with reason)
        {"issued", "cancel", false},           // valid (Checker/Admin)
    }
}
```

| Req | Behavior | Test Type | Command | File Exists? |
|-----|----------|-----------|---------|--------------|
| FR-11 | State machine blocks invalid transitions | unit | `go test -count=1 -run TestStateMachine ./internal/donation/... -timeout 30s` | ❌ Wave 0 |

#### INV-7: Return/Reject Mandatory Reason (FR-12)

**Test:** return and reject without reason field fail validation

| Req | Behavior | Test Type | Command | File Exists? |
|-----|----------|-----------|---------|--------------|
| FR-12 | return without reason → validation error | unit | `go test -count=1 -run TestMandatoryReason ./internal/donation/... -timeout 30s` | ❌ Wave 0 |

### Phase Requirements → Test Map (complete)

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|--------------|
| FR-07 | Maker creates draft donation | unit+integration | `go test -run TestCreateDonation ./internal/donation/... -timeout 120s` | ❌ Wave 0 |
| FR-09 | Edit draft; view slip | unit+integration | `go test -run TestEditDraft ./internal/donation/... -timeout 120s` | ❌ Wave 0 |
| FR-11 | Full lifecycle state machine | unit | `go test -run TestStateMachine ./internal/donation/... -timeout 30s` | ❌ Wave 0 |
| FR-10 | Search by name/date/status/receipt_no | integration | `go test -run TestSearchDonations ./internal/donation/... -timeout 120s` | ❌ Wave 0 |
| FR-12 | Return/reject with mandatory reason | unit | `go test -run TestMandatoryReason ./internal/donation/... -timeout 30s` | ❌ Wave 0 |
| FR-14 | SoD: approver ≠ creator | unit+integration | `go test -run TestSoDEnforcement ./internal/donation/... -timeout 120s` | ❌ Wave 0 |
| FR-19 | Cancel retains receipt number (no gap) | integration | `go test -run TestCancelRetainsNumber ./internal/donation/... -timeout 120s` | ❌ Wave 0 |
| FR-29 | PII encrypted at rest + masked in response | integration | `go test -run TestPII ./internal/donation/... -timeout 120s` | ❌ Wave 0 |
| NFR-02 | Tax ID ciphertext never plaintext in DB | integration | `go test -run TestPII_TaxIDStoredEncrypted ./internal/donation/... -timeout 120s` | ❌ Wave 0 |
| NFR-03 | Consent fields stored on create | unit | `go test -run TestConsentCapture ./internal/donation/... -timeout 30s` | ❌ Wave 0 |
| NFR-04 | Concurrent approvals: exactly 1 issued | integration+race | `go test -count=1 -race -run TestConcurrentApproval ./internal/donation/... -timeout 300s` | ❌ Wave 0 |
| NFR-05 | Issuance audit row in same tx | integration | `go test -run TestIssuanceTransaction_Audit ./internal/donation/... -timeout 120s` | ❌ Wave 0 |
| D-50 | Void & Reissue creates new record + links | integration | `go test -run TestVoidAndReissue ./internal/donation/... -timeout 120s` | ❌ Wave 0 |
| D-51 | edonation_keyed=true requires RD confirmation | unit | `go test -run TestEDonationKeyedGuard ./internal/donation/... -timeout 30s` | ❌ Wave 0 |

### Sampling Rate

- **Per task commit:** `go test -count=1 -run TestXxx ./internal/donation/... -timeout 120s` (affected package only)
- **Per wave merge:** `go test -count=1 ./internal/donation/... ./internal/storage/... -timeout 300s`
- **Phase gate:** `go test -count=1 -race ./... -timeout 600s` — full suite green before `/gsd:verify-work`

### Wave 0 Gaps

- [ ] `internal/donation/service_test.go` — unit tests (state machine, SoD, reason validation, consent, edonation_keyed)
- [ ] `internal/donation/service_integration_test.go` — testcontainers: issuance tx atomicity, concurrency, PII encryption, cancel retains number
- [ ] `internal/storage/client_test.go` — magic-byte validation tests (mock reader; MinIO integration optional)
- [ ] `internal/db/queries/donations.sql` — sqlc source for all donation queries
- [ ] `internal/db/queries/outbox.sql` — sqlc source for outbox enqueue
- [ ] `migrations/000005_donations.{up,down}.sql` — before any service code
- [ ] `migrations/000006_slip_attachments.{up,down}.sql`
- [ ] `migrations/000007_outbox_jobs.{up,down}.sql`

---

## Project Constraints (from CLAUDE.md)

| Directive | Type | Impact on Phase 3 |
|-----------|------|-------------------|
| ห้ามใช้ PostgreSQL SEQUENCE/SERIAL สำหรับเลขใบเสร็จ | FORBIDDEN | ใช้ Phase 2 allocator เท่านั้น |
| ห้าม pre-compute เลขบน draft | FORBIDDEN | receipt_number_id = NULL จนกว่า status='issued' |
| ห้าม render PDF ใน issuance transaction | FORBIDDEN | enqueue outbox_jobs เท่านั้น; Phase 4 worker |
| ห้ามเก็บ slip BLOB ใน DB | FORBIDDEN | ใช้ MinIO + DB reference |
| ห้ามเชื่อ file extension / MIME header | FORBIDDEN | ใช้ magic-byte detection เสมอ |
| SoD: approver_id != created_by ใน code + DB CHECK | REQUIRED | ทั้ง service guard และ migration constraint |
| Audit ทุก action สำคัญ | REQUIRED | AppendAuditEntryTx ใน issuance tx; AppendAuditEntry สำหรับ non-tx actions |
| AES-256-GCM envelope สำหรับ national/tax ID | REQUIRED | ใช้ `internal/crypto.EncryptField` เท่านั้น |
| ห้าม log PII (Pattern C) | FORBIDDEN | log donation_id + status เท่านั้น |
| ตอบกลับและตั้งคำถามเป็นภาษาไทยเท่านั้น | LANGUAGE | applies to user-facing communication |
| Keycloak roles ใน realm_access.roles | REQUIRED | `claims.HasRole()` pattern จาก Phase 1 |
| ใช้ `go test` + testify (ห้าม third-party test frameworks) | REQUIRED | test suite pattern จาก Phase 1/2 |
| TDD mode active | REQUIRED | tests ก่อน implementation (config: tdd_mode: true) |

---

## Security Domain

> `security_enforcement` not explicitly set to false in config.json → treated as enabled.

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | YES (inherited) | Keycloak OIDC + `auth.RequireAuth()` middleware (Phase 1) |
| V3 Session Management | YES (inherited) | JWT bearer token; stateless; Keycloak session management |
| V4 Access Control | YES (Phase 3 extends) | `auth.RequireRoles()` + SoD `approver_id != created_by` guard |
| V5 Input Validation | YES | `go-playground/validator` on all request structs; donor fields |
| V6 Cryptography | YES | AES-256-GCM envelope (`internal/crypto`); never pgcrypto |
| V12 File Upload | YES (new Phase 3) | magic-byte validation (`mimetype`); size limit; content-type allowlist |

### Known Threat Patterns

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| IDOR: access another user's donation | Elevation of Privilege | RBAC: Maker sees only own drafts; Checker/Admin sees all pending |
| SoD bypass: maker approves own donation | Elevation of Privilege | Code guard (ErrSoDViolation) + DB CHECK constraint (defense-in-depth) |
| Double-issuance race | Tampering (data integrity) | `SELECT ... FOR UPDATE` on donation row (D-52) |
| PII leakage via API response | Information Disclosure | Mask by default; decrypt only after `CanRevealFull()` + audit |
| PII leakage via logs | Information Disclosure | Pattern C: log only IDs and status; never log plaintext PII fields |
| File upload attack (malicious payload) | Tampering | Magic-byte validation; content-type allowlist; size cap |
| Cancelled receipt re-use (gap creation) | Tampering | `cancelled` keeps receipt_number_id; REVOKE DELETE on donations |
| edonation_keyed cancel without RD confirmation | Repudiation | D-51 guard: explicit reason required + audited when keyed=true |
| Mass assignment (bind request → DB model) | Tampering | Use explicit request structs (pattern from users/handler.go) |

---

## Sources

### Primary (HIGH confidence)

- Codebase: `donnarec-api/internal/receiptno/allocator.go` — allocator API (`Allocate(ctx, tx, issueDate) (AllocatedReceipt, error)`), caller-managed tx contract
- Codebase: `donnarec-api/internal/db/helpers.go` — `db.WithTx(ctx, pool, fn func(pgx.Tx) error) error`
- Codebase: `donnarec-api/internal/audit/service.go` — `AppendAuditEntryTx(ctx, tx, AuditEntry)` + advisory lock pattern
- Codebase: `donnarec-api/internal/crypto/envelope.go` — `EncryptField` / `DecryptField` envelope AES-256-GCM
- Codebase: `donnarec-api/internal/pii/mask.go` — `MaskNationalID` + `CanRevealFull` (Admin+Checker=true)
- Codebase: `donnarec-api/internal/auth/rbac.go` — `RequireRoles`, role constants (`RoleMaker/RoleChecker/RoleAdmin`)
- Codebase: `donnarec-api/internal/auth/claims.go` — `KeycloakClaims`, `HasRole()`, `ActorIdentity()`
- Codebase: `donnarec-api/internal/users/handler.go` — HTTP handler pattern (gin, validator, audit_after)
- Codebase: `donnarec-api/go.mod` — confirmed: gin v1.12.0, pgx/v5 v5.10.0, mimetype v1.4.13 (indirect), testcontainers v0.43.0
- Codebase: `donnarec-api/docker-compose.yml` — services: postgres+keycloak+migrate+api; MinIO absent
- Context7: `/minio/minio-go` — `PutObject(ctx, bucket, object, reader, size, opts)`, `PresignedPutObject`, `New(endpoint, &Options{Creds, Secure})`
- Go module proxy: `github.com/minio/minio-go/v7@latest = v7.2.1` (verified)
- `.planning/phases/02-gap-less-receipt-numbering-core/02-CONTEXT.md` — D-33..D-42 allocator contract
- `.planning/phases/03-donation-lifecycle-maker-checker-issuance/03-CONTEXT.md` — D-43..D-54 all locked decisions
- `CLAUDE.md` — stack decisions, forbidden patterns, encryption spec, SoD requirement

### Secondary (MEDIUM confidence)

- `.planning/ROADMAP.md` §Phase 3 — 5 success criteria, UI hint: yes
- `.planning/REQUIREMENTS.md` — FR-07/09/10/11/12/14/19/29, NFR-02/03/04/05 detail

### Tertiary (LOW confidence)

- MinIO docker-compose service config — standard community pattern; not from official MinIO Compose docs directly [ASSUMED]

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all packages verified in go.mod or Go proxy; patterns verified from existing code
- Architecture (issuance tx, SoD, encryption): HIGH — direct from Phase 1/2 code patterns + CLAUDE.md decisions
- MinIO integration: MEDIUM — API verified via Context7; docker-compose config is standard pattern
- Frontend bootstrap: LOW — Next.js 15 + next-auth not yet investigated; marked as ASSUMED/open question

**Research date:** 2026-06-28
**Valid until:** 2026-07-28 (30 days; stable Go ecosystem; minio-go versions move slowly)
