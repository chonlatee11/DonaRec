# Phase 4: Receipt PDF + Email Delivery (Outbox Worker) - Pattern Map

**Mapped:** 2026-07-04
**Files analyzed:** ~24 new/modified files (Go backend + migrations + Next.js Admin/BFF)
**Analogs found:** 20 / 24 (rest are genuinely new capability — see "No Analog Found")

---

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|---|---|---|---|---|
| `internal/worker/worker.go` | worker/service | event-driven (poll loop) | `cmd/server/main.go` (goroutine + `signal.NotifyContext` shutdown pattern) | role-match |
| `internal/worker/issue_receipt.go` | service | pipeline/transform | `internal/donation/service.go` (`Approve`, atomic tx steps) | role-match |
| `internal/worker/worker_test.go` | test | integration | `internal/donation/*_test.go` + `internal/testutil/postgres.go` | role-match |
| `internal/db/queries/outbox.sql` (additions) | query file | CRUD + claim | `internal/db/queries/outbox.sql` (existing, Phase 3 `EnqueueOutboxJob`) | exact |
| `internal/pdf/render.go` | service/transform | transform (HTML→string) | `internal/receiptno/format.go` (pure-function style, `internal/receiptno/allocator.go` doc-comment conventions) | partial |
| `internal/pdf/chromium.go` | service (external process/CDP) | request-response (RPC over WS) | `internal/storage/client.go` (external-client wrapper: constructor + panic/err-guard style) | role-match |
| `internal/pdf/render_golden_test.go` | test | golden-file | none existing (new capability) | no analog |
| `internal/mailer/sender.go` (interface) | service (interface seam) | request-response | `internal/storage/client.go` (constructed client + narrow interface boundary) | partial |
| `internal/mailer/dev_sender.go` | service (impl) | file-I/O | `internal/storage/client.go` `PutSlip` (validate → write pattern, minus MinIO) | partial |
| `internal/settings/service.go` | service | CRUD | `internal/receiptno/allocator.go` + `receiptno/format.go` (config-row read/format pattern) | role-match |
| `internal/settings/model.go` | model | CRUD | `internal/donation/model.go` | role-match |
| `internal/settings/handler.go` | controller | request-response | `internal/donation/handler.go` (Pattern A/C/D: claims extraction, sentinel-error switch, audit_after) | exact |
| `internal/donation/model.go` (add `donor_language`) | model | CRUD | itself (existing file, additive change) | exact |
| `internal/donation/handler.go` (add resend/download handlers) | controller | request-response | itself — `RevealPII`/`GetByID` handlers (Pattern A/C/D) | exact |
| `internal/donation/service.go` (add donor_language capture; resend/download methods) | service | CRUD + event-driven (re-enqueue) | itself — `Approve` (outbox enqueue, Step 7) | exact |
| `internal/db/queries/donations.sql` (donor_language, receipt_pdf_object_key) | query file | CRUD | `internal/db/queries/donations.sql` (existing) | exact |
| `internal/db/queries/email_delivery.sql` (new) | query file | CRUD | `internal/db/queries/outbox.sql` (INSERT/UPDATE shape) | role-match |
| `internal/db/queries/settings.sql` (new) | query file | CRUD (single-row) | `internal/db/queries/receiptno.sql` (single-row config CRUD, `GetReceiptNumberConfig`) | exact |
| `migrations/000008_donor_language.up/down.sql` | migration | schema | `migrations/000005_donations.up.sql` (ALTER-style additive column + default backfill) | exact |
| `migrations/000009_email_delivery.up/down.sql` | migration | schema | `migrations/000007_outbox_jobs.up.sql` (status CHECK, attempts, grants) | exact |
| `migrations/000010_receipt_template_config.up/down.sql` | migration | schema | `migrations/000004_receipt_number_tables.up.sql` (single-row `id BOOLEAN PRIMARY KEY DEFAULT true` config table) | exact |
| `migrations/000011_receipt_pdf_reference.up/down.sql` | migration | schema | `migrations/000005_donations.up.sql` (nullable column ALTER on `donations`) | exact |
| `internal/config/config.go` (chrome sidecar URL, receipts bucket) | config | config | itself — `MinIOConfig` block | exact |
| `docker-compose.yml` (chrome sidecar) | config | — | existing `docker-compose.yml` service blocks (minio/postgres/keycloak) | role-match |
| `donnarec-web/app/api/bff/donations/[id]/resend/route.ts` | route (BFF proxy) | request-response | `app/api/bff/donations/[id]/approve/route.ts` | exact |
| `donnarec-web/app/api/bff/donations/[id]/receipt-pdf/route.ts` (download) | route (BFF proxy) | file-I/O (redirect to presigned URL) | `app/api/bff/donations/[id]/slip/route.ts` | exact |
| `donnarec-web/app/api/bff/settings/route.ts` + subpaths (preview, preview/pdf) | route (BFF proxy) | request-response | `app/api/bff/donations/route.ts` (list/create) + `app/api/bff/donations/[id]/route.ts` | role-match |
| `donnarec-web/app/admin/settings/page.tsx` | page (Next.js server component) | request-response | `donnarec-web/app/donations/page.tsx` (server component, `getServerSession`, i18n) | role-match |
| `donnarec-web/components/SettingsTemplateEditor.tsx` (+ preview iframe) | component | streaming/debounced preview | `donnarec-web/components/DonationListView.tsx` (TanStack Query client component pattern) | partial |

---

## Pattern Assignments

### `internal/worker/worker.go` (worker, event-driven poll loop)

**Analog:** `cmd/server/main.go` (goroutine lifecycle + graceful shutdown) and `internal/receiptno/allocator.go` (caller-managed-tx discipline)

**Shutdown pattern to copy** (`cmd/server/main.go` lines 57-59, 168-187):
```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()
...
go func() {
    logger.Info("donnarec-api starting", zap.String("addr", addr))
    if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
        logger.Fatal("server error", zap.Error(err))
    }
}()
<-ctx.Done()
logger.Info("shutdown signal received; draining connections...")
```
Worker should be started as its own goroutine from `main.go` using the SAME `ctx` (shared `signal.NotifyContext`), with its own `Run(ctx)` loop that returns when `ctx.Done()` fires — this mirrors the existing single-process, no-global-state wiring style ("pool → queries → services → handlers → router → server" in `main.go`'s package doc comment). Add `worker := worker.New(pool, queries, pdfRenderer, mailer, logger, cfg.WorkerPollInterval)` then `go worker.Run(ctx)` right after `go func() { srv.ListenAndServe() ... }()`.

**Poll loop pattern (from RESEARCH.md Pattern 1, already verified against this project's schema):**
```go
func (w *Worker) Run(ctx context.Context) {
    ticker := time.NewTicker(w.pollInterval) // e.g. 5s
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            w.logger.Info("worker: shutting down")
            return
        case <-ticker.C:
            w.processOnce(ctx)
        }
    }
}
```

**Transaction-boundary discipline to copy from `internal/receiptno/allocator.go`:**
- Doc-comment style listing numbered steps (1..N) before the function body — `Allocator.Allocate`'s comment block (lines 75-97) is the template for `processOnce`'s doc comment: claim → load donation snapshot → render → store PDF → send email → mark done, each step numbered, explicitly stating what happens on error at each step and that render/email must stay OUTSIDE the numbering transaction (NFR-07, already true because outbox job is Phase-3-committed independently of the render pipeline).
- Anti-patterns section style ("Anti-patterns explicitly absent (threat register ...)" — allocator.go lines 18-22) should be mirrored in `worker.go`'s package doc: explicitly state NO re-render on resend (D-56), NO email send inside the receipt-numbering tx (already true), NO double-claim (SKIP LOCKED).

---

### `internal/db/queries/outbox.sql` (additions: claim/mark-done/mark-failed)

**Analog:** itself (existing file) — extend, do not replace

**Existing header/style to copy** (`internal/db/queries/outbox.sql` lines 1-16):
```sql
-- internal/db/queries/outbox.sql
-- sqlc queries for the transactional outbox_jobs table (Phase 3 enqueue only).
-- Phase 4 adds worker queries (poll, update status, mark done/failed).
```
Add below the existing `EnqueueOutboxJob`:
```sql
-- name: ClaimNextOutboxJob :one
UPDATE outbox_jobs
SET status = 'processing', updated_at = now()
WHERE id = (
    SELECT id FROM outbox_jobs
    WHERE status IN ('pending', 'failed')
      AND next_attempt_at <= now()
      AND attempts < @max_attempts
    ORDER BY created_at
    FOR UPDATE SKIP LOCKED
    LIMIT 1
)
RETURNING id, job_type, payload, attempts;

-- name: MarkOutboxJobDone :exec
UPDATE outbox_jobs SET status = 'done', updated_at = now() WHERE id = @id;

-- name: MarkOutboxJobFailed :exec
UPDATE outbox_jobs
SET status = CASE WHEN attempts + 1 >= @max_attempts THEN 'failed' ELSE 'pending' END,
    attempts = attempts + 1,
    last_error = @last_error,
    next_attempt_at = @next_attempt_at,
    updated_at = now()
WHERE id = @id;
```
This is exactly RESEARCH.md's verified Pattern 1 — copy it verbatim into the sqlc source file, requires a new `next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now()` column added in migration 000009 (or 000008) alongside `outbox_jobs`.

**sqlc naming convention observed:** `-- name: <VerbNoun> :one|:exec|:many` — every query in this codebase (`outbox.sql`, `receiptno.sql`, `donations.sql`) follows PascalCase Go-method-name-as-comment; keep this exactly.

---

### `internal/pdf/chromium.go` (service, CDP request-response)

**Analog:** `internal/storage/client.go` (external-client wrapper: constructor with explicit error wrapping, narrow public API surface)

**Constructor pattern to copy** (`internal/storage/client.go` lines 55-72):
```go
func NewStorageClient(endpoint, accessKey, secretKey, bucket string, secure bool) (*StorageClient, error) {
    client, err := minio.New(endpoint, &minio.Options{...})
    if err != nil {
        return nil, fmt.Errorf("storage: minio client init: %w", err)
    }
    return &StorageClient{client: client, bucket: bucket}, nil
}
```
Mirror this exactly for `pdf.NewRenderer(chromeWSURL string) (*Renderer, error)` — same error-wrap-with-package-prefix idiom (`"pdf: chromedp remote allocator init: %w"`), same "package comment documents which design decisions are realized here" doc-comment style seen at the top of `client.go` (lines 1-13, referencing D-48/D-54/T-03-14 etc.) — for `pdf/chromium.go` reference D-58 (security sandbox mitigations) and the specific CDP calls implementing each.

**Render pipeline itself: use RESEARCH.md's verified live-spike code verbatim** (already tested in this session against Thai worst-case text and the SSRF/XSS probes) — see RESEARCH.md "Pattern 2: Sandboxed Chromium Render" for the exact `chromedp.Run(...)` sequence (`fetch.Enable` + `FailRequest`-all, `emulation.SetScriptExecutionDisabled(true)`, `page.SetDocumentContent`, `page.PrintToPDF`). This is pre-verified code, not a new pattern to derive from an analog — treat it as canonical source.

**Magic-byte validation for template image uploads (letterhead/seal/signature/watermark):** reuse `internal/storage/client.go`'s `validateSlip` function verbatim (lines 74-103) — same `mimetype.Detect` + `allowedMIMETypes` map approach, just change the allowed set to `{image/jpeg, image/png}` (no PDF for template assets) and the error variable names (`ErrUnsupportedFileType`, `ErrFileTooLarge` — copy names, change package).

---

### `internal/mailer/sender.go` (service interface seam, D-60)

**Analog:** `internal/storage/client.go` (interface = narrow struct + explicit constructor; but this is a genuinely new interface-based seam — no existing Go `interface` type exists in this codebase to copy verbatim, so structure comes from RESEARCH.md Code Examples, which is pre-vetted for this project)

**Interface + dev implementation — use RESEARCH.md's verified code directly:**
```go
// internal/mailer/sender.go
type EmailSender interface {
    Send(ctx context.Context, msg Message) (SendResult, error)
}
```
```go
// internal/mailer/dev_sender.go
type DevSender struct{ OutDir string }
func (d *DevSender) Send(ctx context.Context, msg Message) (SendResult, error) {
    dir := filepath.Join(d.OutDir, uuid.NewString())
    os.MkdirAll(dir, 0o755)
    os.WriteFile(filepath.Join(dir, "body.html"), []byte(msg.BodyHTML), 0o644)
    os.WriteFile(filepath.Join(dir, msg.Attachment.Filename), msg.Attachment.Data, 0o644)
    return SendResult{SentAt: time.Now()}, nil
}
```
**Package doc-comment convention to copy:** `internal/storage/client.go`'s header (lines 1-13) lists "Design decisions realized here" with D-numbers and a plain-English one-liner each — `mailer/sender.go` should open the same way, referencing D-60 (interface + dev capture, real provider deferred) and CLAUDE.md's "no self-hosted SMTP in production" rule.

---

### `internal/settings/handler.go` (controller, request-response, Admin-only)

**Analog:** `internal/donation/handler.go` — this is the strongest, most literal analog in the whole phase (same package doc pattern, same Pattern A/C/D conventions already codified in that file's own header comment)

**Pattern A — claims extraction block, copy verbatim per handler** (`internal/donation/handler.go` lines 58-69, reused identically in every handler in that file):
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

**Sentinel-error → HTTP status mapping convention** (`internal/donation/handler.go` lines 9-20, the doc-comment table at the top of the file) — settings handler should declare its own table the same way, e.g.:
```
ErrInvalidTemplate    → 422 Unprocessable Entity (template.Parse failed)
ErrForbidden          → 403 Forbidden
ErrNotFound           → 404 Not Found
default               → 500 (log operation only — Pattern C, no template HTML/PII in logs)
```

**Pattern C (no PII in logs) + Pattern D (audit_after) — copy verbatim** (`internal/donation/handler.go` `GetByID`, lines 122-160):
```go
h.logger.Error("failed to get donation", zap.String("operation", "GetDonationByID"), zap.String("donation_id", id), zap.Error(err))
...
c.Set("audit_after", resp)
c.JSON(http.StatusOK, gin.H{"data": resp})
```
For settings: `zap.String("operation", "SaveReceiptTemplateConfig")` — never log the template HTML body itself (analogous to "never log plaintext tax ID", T-03-10).

**RBAC wiring — copy from `main.go`'s `adminGroup`** (`cmd/server/main.go` lines 225-230):
```go
adminGroup := api.Group("/admin")
adminGroup.Use(auth.RequireRoles(auth.RoleAdmin))
adminGroup.POST("/users", userHandler.CreateUser)
```
Add `adminGroup.GET("/settings", settingsHandler.Get)`, `adminGroup.PUT("/settings", settingsHandler.Save)`, `adminGroup.POST("/settings/preview", settingsHandler.Preview)`, `adminGroup.POST("/settings/preview/pdf", settingsHandler.PreviewPDF)` — all under the EXISTING `adminGroup` (RequireRoles(RoleAdmin) already enforced there, no new middleware needed).

---

### `internal/donation/handler.go` (add: `Resend`, `DownloadReceipt`)

**Analog:** itself — `RevealPII` (lines 726-769) is the closest existing handler shape: GET-by-id, role-gated inside the service, audited.

**Resend pattern — copy the `Approve` handler's app_user_id extraction block** (lines 289-300) since resend is a Checker/Admin-gated mutating action:
```go
rawUserID, userExists := c.Get(auth.AppUserIDContextKey)
if !userExists {
    c.JSON(http.StatusInternalServerError, gin.H{"error": "missing_auth_context"})
    return
}
appUserID, userOK := rawUserID.(pgtype.UUID)
```
Route placement: add to `checkerGroup` in `main.go` (line ~271, alongside `approve`/`return`/`reject`/`cancel`/`reissue`):
```go
checkerGroup.POST("/:id/resend", donationHandler.Resend)
```

**Download pattern — copy `slip_handler.go`'s `View`** (presigned URL, 15-min TTL — see `internal/storage/client.go` `PresignedGet`, lines 147-156) but scoped to `donationGroup` (any of Maker/Checker/Admin per D-57 "staff ดาวน์โหลด PDF เองได้เสมอ"):
```go
donationGroup.GET("/:id/receipt-pdf", donationHandler.DownloadReceipt)
```

---

### Migrations

**Analog for `000008_donor_language`:** `migrations/000005_donations.up.sql` (additive nullable/defaulted column with backfill-safe default) — pattern: `ALTER TABLE donations ADD COLUMN donor_language TEXT NOT NULL DEFAULT 'th' CHECK (donor_language IN ('th','en'));` (default applies to existing rows automatically per D-55).

**Analog for `000009_email_delivery`:** `migrations/000007_outbox_jobs.up.sql` — copy its exact structure: status CHECK enum, `attempts INT NOT NULL DEFAULT 0`, `last_error TEXT`, `created_at`/`updated_at TIMESTAMPTZ NOT NULL DEFAULT now()`, then a `GRANT SELECT, INSERT, UPDATE ON email_delivery TO donnarec_app;` + `GRANT USAGE, SELECT ON SEQUENCE email_delivery_id_seq TO donnarec_app;` footer — this table also needs `donation_id UUID NOT NULL REFERENCES donations(id)`, `provider_message_id TEXT`, `sent_to TEXT`.

**Analog for `000010_receipt_template_config`:** `migrations/000004_receipt_number_tables.up.sql` section 1 (`receipt_number_config`, lines 19-46) — copy the exact single-row-enforced shape:
```sql
CREATE TABLE receipt_template_config (
    id              BOOLEAN PRIMARY KEY DEFAULT true,
    CONSTRAINT      single_row CHECK (id = true),
    template_html   TEXT NOT NULL DEFAULT '',
    section6_text   TEXT NOT NULL DEFAULT '',
    deduction_multiplier TEXT NOT NULL DEFAULT '1x' CHECK (deduction_multiplier IN ('1x','2x')),
    letterhead_object_key TEXT,
    seal_object_key TEXT,
    signature_object_key TEXT,
    watermark_object_key TEXT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000'
);
INSERT INTO receipt_template_config DEFAULT VALUES ON CONFLICT (id) DO NOTHING;
GRANT SELECT, INSERT, UPDATE ON receipt_template_config TO donnarec_app;
```
Per RESEARCH.md Open Question 3: do NOT alter `receipt_number_config` — add this as a sibling table, same single-row pattern.

**Analog for `000011_receipt_pdf_reference`:** `migrations/000005_donations.up.sql` nullable-column-ALTER style: `ALTER TABLE donations ADD COLUMN receipt_pdf_object_key TEXT;` (nullable — populated by worker post-render, per RESEARCH.md Open Question 2 recommendation of a column, not a separate table).

---

### `internal/config/config.go` (extend)

**Analog:** itself — `MinIOConfig` block (lines 13-25, ~60-61, ~101-105)
```go
type MinIOConfig struct {
    Endpoint string
    ...
    Bucket string // default: "donnarec-slips"
    Secure bool
}
...
MinIO: MinIOConfig{
    ...
    Bucket: getEnvStr("MINIO_BUCKET", "donnarec-slips"),
},
```
Copy this exact shape for a new `ReceiptsBucket` field (default `"donnarec-receipts"`, per D-56) and a `ChromeWSURL` field (e.g. `getEnvStr("CHROME_WS_URL", "ws://chrome:9222")`), plus `WorkerPollInterval` (`getEnvDuration`-style helper if one exists, else `getEnvStr` + `time.ParseDuration`).

---

## Shared Patterns

### Auth claims extraction (Pattern A)
**Source:** `internal/donation/handler.go` lines 58-69 (repeated in every handler in that file)
**Apply to:** every new Go HTTP handler in `internal/settings/handler.go` and the new `Resend`/`DownloadReceipt` methods on `internal/donation/handler.go`.
```go
raw, exists := c.Get("claims")
if !exists { c.JSON(http.StatusUnauthorized, gin.H{"error": "missing_auth_context"}); return }
claims, ok := raw.(auth.KeycloakClaims)
if !ok { c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid_claims_type"}); return }
```

### RBAC guard (role-level)
**Source:** `internal/auth/rbac.go` — `RequireRoles` (AND) vs `RequireAnyRole` (OR)
**Apply to:** settings routes → `RequireRoles(auth.RoleAdmin)` (already the `adminGroup` default in `main.go`); resend routes → `RequireAnyRole(auth.RoleChecker, auth.RoleAdmin)` (already the `checkerGroup` default); download route → inherits `donationGroup`'s `RequireAnyRole(RoleMaker, RoleChecker, RoleAdmin)` (no new group needed — mirrors how `RevealPII` is placed on `donationGroup` not `checkerGroup` per its own comment, lines 255-258, "so a Maker receives 403, not 401/404").

### Audit trail (Pattern D)
**Source:** `internal/audit/middleware.go` (`AuditMiddleware`, wired globally in `main.go` line 210) + `c.Set("audit_after", resp)` convention used throughout `internal/donation/handler.go`
**Apply to:** every mutating settings/resend endpoint — set `c.Set("audit_after", resp)` before the final `c.JSON`; template/config saves and resend actions are exactly the kind of "significant action" CLAUDE.md's Audit constraint requires (immutable audit trail).
For worker-triggered actions (auto-retry, worker-side status transitions) that do NOT go through an HTTP request, use `AuditService.AppendAuditEntryTx` directly (the tx-scoped variant, `internal/audit/service.go` line 138) inside the worker's own DB transaction — mirrors how `internal/donation/service.go`'s `Approve` writes audit entries atomically with the state transition rather than relying on the HTTP middleware (which only fires per-request).

### Error logging without PII (Pattern C)
**Source:** every handler in `internal/donation/handler.go` (e.g. lines 107-112, 207-213)
**Apply to:** all new handlers/services — log `zap.String("operation", "<Name>")` + the non-PII ID field only (`donation_id`, or a new `job_id`/`settings` marker), never the template HTML, tax ID, or email body.

### External-client wrapper constructor style
**Source:** `internal/storage/client.go` (`NewStorageClient`)
**Apply to:** `internal/pdf.NewRenderer(...)`, `internal/mailer.NewDevSender(...)` (though the latter needs no error return) — package-prefixed `fmt.Errorf("<pkg>: <step>: %w", err)` wrapping, doc-comment block at file top listing "Design decisions realized here" with D-numbers.

### sqlc query file conventions
**Source:** `internal/db/queries/outbox.sql`, `internal/db/queries/receiptno.sql` (single-row config CRUD)
**Apply to:** `internal/db/queries/email_delivery.sql`, `internal/db/queries/settings.sql` — `-- name: <PascalCaseVerbNoun> :one|:many|:exec` header per query, file-top comment block explaining the design decision(s) realized (D-numbers), migration-number cross-reference in the comment.

### Migration file conventions
**Source:** `migrations/000004_receipt_number_tables.up.sql`, `migrations/000007_outbox_jobs.up.sql`
**Apply to:** all four new migrations (000008-000011) — numbered section comments (`-- === N. <Thing> ===`), explicit `GRANT`/`REVOKE` footer scoped to `donnarec_app` role, `ON CONFLICT (id) DO NOTHING` idempotent seed for single-row config tables, explicit doc comment at top listing which D-decisions/FRs the migration realizes.

### Next.js BFF proxy shape
**Source:** `donnarec-web/app/api/bff/donations/[id]/approve/route.ts` (and sibling `cancel`, `reject`, `return`, `reissue`, `slip` routes — all structurally identical)
**Apply to:** `app/api/bff/donations/[id]/resend/route.ts`, `app/api/bff/donations/[id]/receipt-pdf/route.ts`, `app/api/bff/settings/route.ts` (+ `preview`, `preview/pdf` subroutes)
```typescript
import type { NextRequest } from "next/server";
import { bffForward } from "@/lib/bff";
export async function POST(request: NextRequest, { params }: { params: Promise<{ id: string }> }): Promise<Response> {
  const { id } = await params;
  return bffForward(request, `/api/donations/${id}/resend`);
}
```
Every BFF route is a thin proxy only — Go re-enforces RBAC/SoD; the BFF is never the authorization authority (T-12-02 convention, documented in every existing route file's comment).

### Admin/list page composition (Server Component + TanStack Query client view)
**Source:** `donnarec-web/app/donations/page.tsx` (server component: `getTranslations`, `getServerSession`, decode JWT `sub` for viewer-based routing) + `donnarec-web/components/DonationListView.tsx` (client component, TanStack Query against the BFF)
**Apply to:** `app/admin/settings/page.tsx` (server component wrapper: i18n + session) + a new `components/SettingsTemplateEditor.tsx` (client component: debounced (400ms) `POST /api/bff/settings/preview` calls, sandboxed iframe rendering the returned HTML, a separate button triggering `POST /api/bff/settings/preview/pdf` for the real-Chromium accurate preview — per D-61's recommended hybrid (a)+(b) strategy).

---

## No Analog Found

Files with no close match in the codebase — planner should ground these in RESEARCH.md's verified Code Examples/Patterns instead of an existing file:

| File | Role | Data Flow | Reason |
|------|------|-----------|--------|
| `internal/pdf/render_golden_test.go` + `testdata/*.golden.png` | test | golden-file (visual regression) | No golden-file/visual-regression test exists anywhere in Phase 1-3; RESEARCH.md's "Code Examples: Golden-file test shape" (verified this session) is the canonical source — exact PNG-byte comparison via `pdftoppm`, no fuzzy-diff dependency |
| `internal/pdf/render_sandbox_security_test.go` | test | security regression | New capability — port RESEARCH.md's live-verified JS-disable/network-block spike code directly as a permanent regression test |
| `internal/testutil/chrome.go` | test helper | testcontainers fixture | Closest structural analog is `internal/testutil/postgres.go`/`keycloak.go` (same testcontainers-go wiring idiom) — role-match, listed here because it's a new sidecar type (Chromium), not literally the same service |
| `docker/chrome.Dockerfile` | config | container build | No existing custom Dockerfile beyond the app's own `Dockerfile`; RESEARCH.md Pattern 4 provides the verified content (`FROM chromedp/headless-shell:stable` + `fonts-thai-tlwg` + `fc-cache`) |
| `internal/mailer/sender.go` (interface itself) | interface definition | — | This codebase has no prior `interface`-based swappable-adapter seam (storage/audit/receiptno are all concrete structs) — RESEARCH.md's Code Examples section is pre-vetted canonical source, not derived from an in-repo analog |

---

## Metadata

**Analog search scope:** `donnarec-api/internal/**`, `donnarec-api/migrations/**`, `donnarec-api/cmd/server/main.go`, `donnarec-web/app/**`, `donnarec-web/components/**`
**Files scanned:** ~35 Go files, 14 migration files, ~20 Next.js route/page/component files (targeted reads, not exhaustive)
**Pattern extraction date:** 2026-07-04
