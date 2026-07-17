# Phase 6: Public Donation Web Form (Flow B) - Research

**Researched:** 2026-07-11
**Domain:** Unauthenticated public web submission seam (Go/Gin backend) + Next.js public route group + CAPTCHA/rate-limiting, reusing the existing Flow A donation/approval/PDF/email pipeline
**Confidence:** HIGH (backend/frontend patterns are 100% grounded in existing codebase; external CAPTCHA/rate-limit findings are MEDIUM-HIGH — official docs + verified packages)

## Summary

Phase 6 does not introduce a new stack — it opens the **first unauthenticated HTTP seam** in a codebase where every prior route lives under `RequireAuth()`. The work is almost entirely about **composition and guarding**, not new technology: a new Gin route group (`/api/public/donations`) that swaps `RequireAuth()` for a CAPTCHA-verify + per-IP rate-limit middleware pair, a new orchestration method on the existing `DonationService` that atomically creates a donation directly into `pending_review` (skipping the `draft` state Flow A always visits) with a mandatory slip, a `source` column to separate the two flows in the UI, a new `ack_email` outbox job type dispatched by the existing worker's job-type switch, and a Next.js `(public)` route group with its own warm CSS-variable-scoped theme sitting outside the app's Keycloak-gated layout.

Three findings are load-bearing for planning and are **not obvious from the CONTEXT/UI-SPEC alone**:

1. **`audit.AuditEntry.ActorID` must parse as a UUID** (`internal/audit/service.go` calls `parseUUID(entry.ActorID)` and errors out — rolling back the whole transaction — if it isn't). The seeded `public-web` system user's `keycloak_subject` (D-76) must therefore be a **UUID-shaped literal**, not a human-readable sentinel string like `"public-web"`.
2. **`DonationService.Create` always produces `status='draft'`**; `Submit` is a separate call that transitions `draft → pending_review`. Flow B's success criterion #1 requires landing directly in `pending_review`. This means Phase 6 needs a **new atomic orchestration path** (Create + Submit, ideally + slip-reference insert, in one `WithTx`), not a reuse of the two public HTTP endpoints Flow A exposes.
3. **The existing BFF pattern (`lib/bff.ts` `bffForward`) is unusable for the public form** — it calls `getServerSession(authOptions)` and 401s if there's no Keycloak session, which a donor never has. The public form needs a **new, session-less Next.js Route Handler family** that proxies to Go's `/api/public/donations` without ever calling `getServerSession`.

**Primary recommendation:** Reuse `DonationService`/`SlipService`/`crypto`/`storage`/`audit`/`worker` as-is; add one new atomic service method for the public create+submit+slip path, one new Gin route group with two new middlewares (Turnstile verify, per-IP token-bucket rate limit via `golang.org/x/time/rate`), one `source` column + one system-user seed migration, one new outbox `job_type`, and a fully separate Next.js `(public)` route group with its own CSS-scoped theme — never touching `(app)`'s slate/blue tokens or the BFF's session-bound proxy helpers.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Public donation form rendering (bilingual, warm theme) | Frontend Server (Next.js SSR/RSC) | Browser (client interactivity — react-hook-form, Turnstile widget) | `(public)` route group, server-rendered shell + client form state, matches existing `DonationForm` pattern |
| CAPTCHA challenge render | Browser | — | Turnstile widget is a third-party script; browser-only |
| CAPTCHA token verification | API / Backend (Go) | — | Never trust a client-asserted "captcha passed" flag — the Go handler must call Cloudflare's `siteverify` server-side before accepting the submission (D-82) |
| Per-IP rate limiting | API / Backend (Go, middleware) | — | Must gate before any DB/storage work; CDN/edge rate limiting is out of scope (self-hosted stack, no CDN commitment yet) |
| Donation creation (pending_review, source=flow_b) | API / Backend (Go) | Database (row insert, CHECK constraints) | Business rule: bypass `draft`, mandatory slip, mandatory tax ID — enforced in `DonationService`, not the frontend |
| Slip magic-byte validation + storage | API / Backend (Go) | Database / Storage (MinIO) | `internal/storage.PutSlip` already does this; content-based, never trust `Content-Type` header or extension |
| PII encryption at rest | API / Backend (Go) | Database (ciphertext column) | `internal/crypto` envelope encryption — plaintext never reaches Postgres, reused verbatim |
| PDPA consent capture | API / Backend (Go) | Database | Snapshot fields on `donations` row, distinct `consent_text_version` string for Flow B (D-81) |
| Ack email dispatch | API / Backend (Go, outbox worker) | Email provider (dev capture only, MVP) | Same outbox/worker seam as `issue_receipt`; new `job_type="ack_email"`, dispatched off the request path (NFR-07 precedent) |
| Pending-review queue (staff) | Frontend Server (Next.js `(app)`, authenticated) | API / Backend (Go, `source` filter) | Existing `(app)` slate/blue surface; reuses `SearchDonations`/`CountDonations` extended with a `source` filter |
| Audit trail (public submit) | API / Backend (Go) | Database (append-only, hash-chained) | Actor = seeded `public-web` system user's UUID-shaped `keycloak_subject`; same `AppendAuditEntryTx` seam |

## User Constraints (from CONTEXT.md)

<user_constraints>

### Locked Decisions

- **D-76:** `created_by` of Flow B = a seeded system user (`public-web`); `users.id` FK stays `NOT NULL` (no schema softening of the Phase 3 invariant).
- **D-77:** New `source` column (`'flow_a'` / `'flow_b'`) on `donations` (migration ≥000015; default `'flow_a'`, backfills existing rows) — explicit separation of the pending-review queue by source, never inferred from `created_by`.
- **D-78:** Public form hits the existing Go API through a **new unauthenticated route group** `/api/public/donations` — no `RequireAuth`, gated instead by CAPTCHA + rate-limit middleware. Reuses the donation service / crypto encrypt / storage (magic-byte) / audit exactly as Flow A. No microservice split; Next.js server actions must never touch the DB directly (would bypass Go's encryption/validation).
- **D-79:** National ID / tax ID (13 digits) is **mandatory** on the public form — `donor_tax_id_enc` stays `NOT NULL` (D-44 unchanged); pipeline is identical to Flow A, e-Donation export works unmodified.
- **D-80:** Slip is **mandatory** in Flow B (unlike Flow A's optional slip, D-48) — no staff member sees the money arrive, so a slip is the only confirmation before review. Reuses magic-byte validation + size limit + object storage seam unchanged.
- **D-81:** Reuse consent snapshot fields (`consent_given`/`consent_at`/`consent_text_version`/`consent_purpose`, D-49) with a **distinct `consent_text_version` string** specific to the public form.
- **D-82:** CAPTCHA = Cloudflare Turnstile, behind a **config-swappable verifier interface** (privacy-first vs. reCAPTCHA; matches PDPA-first project stance). Token verified server-side in Go before accepting a submission. Turnstile needs outbound network egress — a hosting/stakeholder gate; abstract the verifier so a different provider can be swapped in if on-prem egress is later restricted.
- **D-83:** Per-IP rate limiting (Go middleware) **alongside** CAPTCHA (defense-in-depth) — applies to both the submit and (if a separate endpoint exists) upload path.
- **D-84:** Post-submit = on-screen confirmation + a **reference number that is explicitly NOT a receipt number** (receipt numbers are allocated only at Checker approval, Phase 2 allocator — never pre-computed). Reference is for the donor to quote when contacting staff.
- **D-85:** Ack email = new outbox `job_type="ack_email"`, dispatched via the existing worker/outbox (Phase 4 pattern) — decoupled, retryable, does not block the submit response (NFR-07). Copy must explicitly state "received, not yet a receipt." Bilingual via `internal/mailer` + i18n, reusing the receipt email's branding chrome for continuity.
- **D-86:** **No donor status tracking/portal/login** this phase — ack email + on-screen confirmation is the entire MVP donor-facing feedback loop. Explicitly deferred to reduce public attack surface + PII exposure.

### Claude's Discretion

- Schema details: `source` column type (enum vs. `TEXT` + `CHECK`), `public-web` system-user seed value/method, reference-number format, next migration number (≥000015).
- Go package structure: public handler location (`internal/donation/` reuse vs. new `internal/publicform/`), CAPTCHA verifier interface + Turnstile implementation location (e.g. `internal/captcha/`), rate-limit middleware location.
- Rate-limit numeric defaults (submit/upload per IP per window, configurable) and counter storage (in-memory acceptable — no Redis in the stack today).
- Outbox `ack_email` payload shape + dispatch wiring (mirrors `issue_receipt`'s `job_type` switch).
- Next.js public form: route/URL, Turnstile widget integration, language default/detection (reuse `next-intl`).
- Responsive audit scope (NFR-06): which screens across Phases 3–5 plus the new public form need the pass.

### Deferred Ideas (OUT OF SCOPE)

- Donor status tracking / portal / login / status-lookup link (D-86) — new capability, increases public attack surface + PII exposure; next milestone.
- Donor master table / dedup / auto-fill / blind index (D-43, Phase 3) — still snapshot-only; can be added later without migrating existing snapshots.
- Swapping CAPTCHA provider / self-hosting a challenge — the verifier interface (D-82) makes this possible later; not built this phase.
- Direct e-Donation API integration / deep reporting / PKI signatures — separate milestone (REQUIREMENTS.md v2).

</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| FR-01 | Donor fills a donation form (donor details + amount + donated date) | `PublicDonationForm` reuses `DonationForm`'s field set (Screen 9); Go `CreateDonationRequest` struct already has every field except `source`/CAPTCHA token — extend, don't rebuild |
| FR-02 | Donor uploads slip (jpg/png/pdf, size-limited, type-checked) | `internal/storage.PutSlip` + `validateSlip` (magic-byte via `gabriel-vasile/mimetype`) reused verbatim; only the "optional" gate (D-48 in Flow A) is bypassed for Flow B (D-80 mandatory) |
| FR-03 | PDPA consent shown + recorded before submit | `ConsentBlock` component + `consent_given/at/text_version/purpose` snapshot columns reused verbatim (D-81); new `consentTextVersion` string only |
| FR-06 | Thai/English language selection on the public form | `next-intl` + `LocaleSwitcher` reused; new `publicDonation.*` message namespace; `donor_language` submitted field already exists end-to-end (D-55, Phase 4) |
| FR-05 | Acknowledgement (on-screen + email, explicitly "not a receipt") | New outbox `job_type="ack_email"`, worker `switch` extension (`internal/worker/worker.go` `ProcessOnce`); reuses `internal/mailer.EmailSender` + `internal/i18n` |
| FR-04 | Spam/bot protection (CAPTCHA + rate limiting) | Cloudflare Turnstile `siteverify` server-side check (new `internal/captcha` package) + `golang.org/x/time/rate` per-IP token-bucket middleware (new, no new external dependency — `x/time` is an official Go extended-stdlib module) |
| FR-08 | Staff see a pending-review queue from the web, separated from Flow A | New `source` column + extend `SearchDonations`/`CountDonations` sqlc queries with a `source` `sqlc.narg` filter (exact pattern already used for `donor_name`/`status`/date range, D-53) |
| NFR-06 | Responsive + bilingual UI across desktop/mobile | UI-SPEC's Responsive Contract + Mobile Navigation Retrofit sections are fully specified already (06-UI-SPEC.md); this phase's implementation work, not new research |

</phase_requirements>

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `golang.org/x/time/rate` | v0.15.0 (latest, confirmed via Go module proxy) [VERIFIED: proxy.golang.org] | Per-IP token-bucket rate limiting (FR-04, D-83) | Official Go extended-stdlib module (`golang.org/x/*`), zero new third-party trust surface; the de-facto standard building block for this exact "per-visitor limiter map" pattern in idiomatic Go HTTP services [CITED: alexedwards.net/blog/how-to-rate-limit-http-requests] |
| `@marsidev/react-turnstile` | 1.5.3 (latest on npm, published 2026-06-09) [VERIFIED: npm registry — package-legitimacy check verdict OK, 1.59M weekly downloads, MIT, github.com/marsidev/react-turnstile] | React wrapper around the Cloudflare Turnstile widget script | Thin, actively maintained wrapper (45 published versions since 2022) around the official `<script>`; UI-SPEC explicitly allows either this or a raw `<script>` tag — this package removes React lifecycle boilerplate |
| Cloudflare Turnstile `siteverify` API | `POST https://challenges.cloudflare.com/turnstile/v0/siteverify` [CITED: developers.cloudflare.com/turnstile/get-started/server-side-validation] | Server-side CAPTCHA token verification | The only correct way to trust a Turnstile completion — client-side "widget succeeded" state is never sufficient; supports an `idempotency_key` for safe retries on timeout |

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/gabriel-vasile/mimetype` | already in `go.mod` (v1.4.13) | Magic-byte MIME detection for slip uploads | Already the mandatory pattern for every file upload in this codebase (`internal/storage.validateSlip`) — reuse, do not add a second detection library |
| `github.com/gin-gonic/gin` | already in `go.mod` (v1.12.0) | HTTP router/middleware | **Note:** CLAUDE.md's stack table says "`net/http` + chi router (recommended)" — the actual shipped codebase uses **Gin** throughout (`cmd/server/main.go`). This is a real divergence between the stack doc and the code; Phase 6 MUST follow the codebase (Gin), not CLAUDE.md's aspirational recommendation. Flagged for a CLAUDE.md correction, out of this phase's scope to fix the doc. |
| `go-playground/validator/v10` | already in `go.mod` | Struct-tag validation for the public request body | Reuse the exact `validate:"required,len=13,numeric"` etc. tags already on `CreateDonationRequest` |
| `next-intl` | already in `donnarec-web/package.json` (^3.26.5) | Bilingual UI, incl. new `publicDonation.*` namespace | Established pattern, zero new dependency |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `golang.org/x/time/rate` (in-memory, per-process) | Redis-backed limiter (e.g. `ulule/limiter`) | Only worth it once the API runs as >1 replica behind a shared limiter requirement; stack has no Redis today (CONTEXT explicitly notes this) — defer until horizontal scaling is a real requirement |
| `@marsidev/react-turnstile` | Raw `<script src="https://challenges.cloudflare.com/turnstile/v0/api.js">` + manual `window.turnstile.render()` | Zero new npm dependency, but more boilerplate lifecycle code (mount/unmount/reset) to hand-write and test; UI-SPEC explicitly allows either |
| DB-stored CAPTCHA secret (via a `settings`-style config table like `edonation_config`) | Env var `TURNSTILE_SECRET_KEY` (mirrors `DONAREC_KEK`/`MINIO_SECRET_KEY` pattern) | Recommend **env var** for the secret key — every other secret in this codebase (`DONAREC_KEK`, `MINIO_SECRET_KEY`, DB credentials) is env-only, never DB-editable; a DB-stored CAPTCHA secret would be the first exception and adds attack surface (any admin-panel SQLi/RBAC bug would leak it). The **site key** (public by Cloudflare design) is fine as `NEXT_PUBLIC_TURNSTILE_SITE_KEY`. Rate-limit numeric thresholds ARE reasonable to make DB-configurable later (non-secret), but env var is sufficient for MVP per CONTEXT's own "Claude's Discretion" framing. |

**Installation:**
```bash
# Go backend — no new module beyond the extended-stdlib x/time (add to go.mod)
cd donnarec-api && go get golang.org/x/time@v0.15.0

# Next.js frontend
cd donnarec-web && npm install @marsidev/react-turnstile@1.5.3
```

**Version verification:** `golang.org/x/time@v0.15.0` confirmed live via `curl https://proxy.golang.org/golang.org/x/time/@latest` on 2026-07-11 [VERIFIED: proxy.golang.org]. `@marsidev/react-turnstile@1.5.3` confirmed via `npm view` + the package-legitimacy seam (verdict `OK`) on 2026-07-11 [VERIFIED: npm registry].

## Package Legitimacy Audit

| Package | Registry | Age | Downloads | Source Repo | Verdict | Disposition |
|---------|----------|-----|-----------|-------------|---------|-------------|
| `@marsidev/react-turnstile` | npm | 4 yrs (first published 2022-10-17) | 1.59M/week | `github.com/marsidev/react-turnstile` | **OK** | Approved |
| `golang.org/x/time` | Go module proxy | Official Go team extended-stdlib module (`golang.org/x/*` namespace) | N/A (Go modules don't report npm-style download counts) | `go.googlesource.com/time` | **OK** (not run through the npm/pypi legitimacy seam — Go's `golang.org/x/*` namespace is maintained directly by the Go team and is the closest equivalent to a first-party/stdlib package; presence on the official module proxy at a tagged version is itself the authoritative signal) | Approved |

**Packages removed due to `[SLOP]` verdict:** none
**Packages flagged as suspicious `[SUS]`:** none

Cloudflare's own `<script>` tag (if the planner chooses the no-dependency route instead of `@marsidev/react-turnstile`) is loaded directly from `https://challenges.cloudflare.com/turnstile/v0/api.js` and is not an npm package — no registry legitimacy check applies; it is the first-party official asset (UI-SPEC 06-UI-SPEC.md §"Design System" already documents this as the fallback option).

## Architecture Patterns

### System Architecture Diagram

```
                              ┌─────────────────────────────────────────┐
                              │  Donor's Browser (unauthenticated)        │
                              │  (public)/donate  — warm theme            │
                              └───────────────┬───────────────────────────┘
                                              │ 1. GET /donate (SSR shell, PublicHeader,
                                              │    Turnstile widget loads its own script)
                                              │ 2. multipart/form-data POST (fields + slip
                                              │    file + turnstile_token) on submit
                                              ▼
                    ┌───────────────────────────────────────────────────┐
                    │  Next.js (public) route group — NO getServerSession │
                    │  app/(public)/donate/page.tsx  +  NEW              │
                    │  app/api/public/donations/route.ts  (session-less  │
                    │  passthrough — reuses the multipart re-post shape  │
                    │  from app/api/bff/donations/[id]/slip/route.ts,    │
                    │  but WITHOUT the getBffToken()/Authorization step) │
                    └───────────────────────┬───────────────────────────┘
                                            │ 3. multipart POST, no Authorization header
                                            ▼
        ┌───────────────────────────────────────────────────────────────────┐
        │  Go API — NEW unauthenticated route group                          │
        │  api.Group("/public")  →  publicGroup.POST("/donations", ...)      │
        │                                                                    │
        │  Middleware chain (replaces RequireAuth):                         │
        │   [Recovery] → [zapLogger] → [AuditMiddleware] →                  │
        │   [RateLimitByIP] → [VerifyTurnstile] → handler                   │
        └───────────────────────┬───────────────────────────────────────────┘
                                │ 4. handler:
                                │    a. parse multipart fields + file (gin c.PostForm/c.FormFile)
                                │    b. storage.PutSlip FIRST (magic-byte validate,
                                │       fail fast BEFORE any DB row exists)
                                │    c. donationSvc.CreatePublicSubmission(...)
                                │       — ONE WithTx: CreateDonation(status draft-skip
                                │         semantics) → SubmitDonation → InsertSlip →
                                │         AppendAuditEntryTx → EnqueueOutboxJob("ack_email")
                                ▼
        ┌───────────────────────────────────────────────────────────────────┐
        │  Postgres — donations (source='flow_b', status='pending_review'), │
        │  slip_attachments, audit_log (hash-chained), outbox_jobs           │
        └───────────────────────┬───────────────────────────────────────────┘
                                │ 5. donor sees on-screen confirmation +
                                │    reference number (donation.id, NOT receipt no.)
                                │
                                │ 6. outbox worker polls (existing ticker loop,
                                │    unchanged) → claims job_type="ack_email" →
                                │    mailer.Send (bilingual, "not yet a receipt")
                                ▼
                     ┌─────────────────────────┐
                     │  Donor's email inbox     │
                     └─────────────────────────┘

        ┌───────────────────────────────────────────────────────────────────┐
        │  Staff Browser (authenticated, existing (app) slate/blue shell)    │
        │  /queue  →  BFF  →  GET /api/donations?status=pending_review       │
        │  &source=flow_b|flow_a  (extends existing SearchDonations query)   │
        │  → EXACT SAME approve/return/reject pipeline as Flow A (Phase 3-5) │
        └───────────────────────────────────────────────────────────────────┘
```

### Recommended Project Structure

```
donnarec-api/
├── internal/
│   ├── captcha/                # NEW — Turnstile verifier + swappable interface (D-82)
│   │   ├── verifier.go         #   type Verifier interface { Verify(ctx, token, remoteIP) error }
│   │   └── turnstile.go        #   TurnstileVerifier — calls siteverify
│   ├── ratelimit/               # NEW — per-IP token-bucket middleware (D-83)
│   │   └── middleware.go       #   golang.org/x/time/rate + visitor map + cleanup goroutine
│   ├── donation/
│   │   ├── public_handler.go   # NEW — Gin handler for POST /api/public/donations
│   │   └── service.go          # EXTENDED — add CreatePublicSubmission (Create+Submit+Slip+Audit+Outbox in one WithTx)
│   └── worker/
│       └── ack_email.go        # NEW — job_type="ack_email" handler, mirrors issue_receipt.go's shape
├── migrations/
│   ├── 000015_donation_source.up/down.sql       # NEW — source column + backfill
│   └── 000016_seed_public_web_user.up/down.sql  # NEW — fixed-UUID system user (see Pitfall 1)

donnarec-web/
├── app/
│   ├── (app)/                  # NEW route group — existing donations/queue/e-donation/reports/admin MOVE here
│   │   └── layout.tsx          #   renders <AppShell>{children}</AppShell>
│   ├── (public)/                # NEW route group
│   │   ├── layout.tsx          #   NO AppShell — PublicHeader + .theme-public wrapper
│   │   ├── public-theme.css    #   scoped .theme-public CSS variable block
│   │   └── donate/
│   │       └── page.tsx        #   Screen 9/10 — PublicDonationForm / PublicDonationConfirmation
│   ├── queue/
│   │   └── page.tsx            #   NEW — Screen 11, first real implementation of the existing dead nav link
│   └── api/
│       ├── bff/…               #   UNCHANGED — session-bound, existing
│       └── public/              # NEW — session-LESS passthrough family
│           └── donations/route.ts  # multipart re-post, no getServerSession/Authorization
├── components/
│   ├── PublicHeader.tsx        # NEW
│   ├── PublicDonationForm.tsx  # NEW
│   ├── PublicDonationConfirmation.tsx  # NEW
│   ├── TurnstileWidget.tsx     # NEW
│   ├── SourceBadge.tsx         # NEW
│   ├── QueueTable.tsx          # NEW
│   ├── QueueSourceFilter.tsx   # NEW
│   └── MobileNavDrawer.tsx     # NEW
└── middleware.ts               # UNCHANGED matcher list — /donate must stay OUT of it
```

### Pattern 1: Atomic public-submission transaction (new — no direct precedent, but composes two existing ones)

**What:** A single `DonationService` method that performs Create (draft semantics minus the draft state) → Submit → slip DB reference insert → audit → outbox enqueue, all inside **one** `dbhelpers.WithTx` closure — mirroring the shape of `Approve` (7 steps in one transaction, `internal/donation/service.go`), not the shape of `Create`+`Submit`+`UploadSlip` (three separate HTTP calls / three separate transactions used by Flow A).

**When to use:** Any time a single logical business event (here: "a donor submitted a complete request") must be all-or-nothing. Flow A tolerates a "draft with no slip yet" limbo state because a human Maker is iterating; Flow B has no such iteration — a partial submission (donation row created, slip upload failed) would leave a mandatory-slip record with no slip, violating D-80 silently.

**Example (illustrative shape, follows `Approve`'s established structure exactly):**
```go
// Source: internal/donation/service.go Approve() — the closest existing precedent
// for "multiple DB effects that must commit or rollback together, plus an
// outbox enqueue at the end."
func (s *DonationService) CreatePublicSubmission(
    ctx context.Context,
    req PublicDonationRequest,
    slipObjectKey, slipMimeType string,
    slipSizeBytes int64,
    publicWebUserID pgtype.UUID, // resolved once at wiring time (fixed system user)
) (*DonationDetailResponse, error) {
    // 1. Encrypt tax ID BEFORE the transaction (same as Create today).
    encBytes, dekBytes, err := crypto.EncryptField(ctx, s.keyProvider, []byte(req.DonorTaxID))
    // ...

    var fullRow db.Donation
    err = dbhelpers.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
        qtx := s.queries.WithTx(tx)

        // Step 1: create with source='flow_b', created_by=publicWebUserID
        row, txErr := qtx.CreateDonation(ctx, db.CreateDonationParams{ /* ..., Source: "flow_b" */ })
        if txErr != nil { return txErr }

        // Step 2: immediately submit — draft -> pending_review, same tx
        if err := qtx.SubmitDonation(ctx, row.ID); err != nil { return err }

        // Step 3: insert the ALREADY-uploaded (outside-tx) slip reference
        if _, err := qtx.InsertSlip(ctx, db.InsertSlipParams{
            DonationID: row.ID, ObjectKey: slipObjectKey,
            MimeType: slipMimeType, SizeBytes: slipSizeBytes,
            UploadedBy: publicWebUserID,
        }); err != nil { return err }

        // Step 4: audit — actor = system user's UUID-shaped keycloak_subject (Pitfall 1)
        if err := s.auditSvc.AppendAuditEntryTx(ctx, tx, audit.AuditEntry{
            ActorID: publicWebSystemUserKeycloakSubject, // MUST be a valid UUID string
            Action:  "donation.public_submit",
            Resource: "/api/public/donations",
        }); err != nil { return err }

        // Step 5: enqueue ack_email (NOT issue_receipt — no receipt exists yet)
        payload, _ := json.Marshal(map[string]string{"donation_id": row.ID.String()})
        return qtx.EnqueueOutboxJob(ctx, db.EnqueueOutboxJobParams{
            JobType: "ack_email", Payload: payload,
        })
    })
    // ...
}
```

**Ordering rationale (fail-fast, no orphan state):** slip upload to MinIO happens **before** the transaction opens (matching `SlipService.UploadSlip`'s existing sequencing — `storage.PutSlip` runs, then a *separate* tx inserts the DB reference). If Turnstile verification or the slip magic-byte check fails, **no donation row is ever created** — the handler returns 4xx before calling the service at all. If the slip-put to MinIO succeeds but the subsequent DB transaction fails for any other reason, MinIO retains an orphaned object (acceptable — matches D-54's "storage retains, DB is source of truth" philosophy already established for soft-deleted slips) but **no partial `donations` row exists**.

### Pattern 2: Session-less BFF passthrough (new — extends the existing BFF proxy pattern)

**What:** A parallel family to `app/api/bff/**` under `app/api/public/**` that does the identical multipart-reconstruction dance as `app/api/bff/donations/[id]/slip/route.ts` (`request.formData()` → new `FormData` → `fetch` with no `Content-Type` override) but **never calls `getServerSession`/`getBffToken`** and **never sets an `Authorization` header** — because there is no session to have.

**When to use:** Any future unauthenticated-to-Go-API frontend route. This is the only such route in Phase 6, but the pattern generalizes.

**Why a passthrough at all, instead of the browser calling Go directly:** (a) keeps `NEXT_PUBLIC_API_BASE_URL` server-side-only — the browser never needs to know the Go API's origin, avoiding a CORS configuration that the codebase has never needed before (every other browser→Go path today goes through the session-bound BFF); (b) keeps the "Next.js never talks to the DB, only proxies to Go" invariant (D-78) visually consistent with every other route in the app; (c) lets the Next.js layer apply its own defense-in-depth (e.g. Next.js `unstable_after`/edge rate limiting) later without touching Go. **This is flagged as a Claude's-Discretion recommendation** — CONTEXT does not mandate proxy-vs-direct-CORS, but the passthrough is the lower-risk, more-consistent-with-existing-architecture choice. See Open Questions.

### Pattern 3: Per-IP token-bucket rate limiting (new — standard idiomatic Go pattern)

**What:** A `sync.Mutex`-protected `map[string]*rate.Limiter` keyed by `c.ClientIP()` (Gin already resolves `X-Forwarded-For`/`X-Real-IP` correctly — reuse it, do not re-implement IP extraction), with a background goroutine that evicts entries unseen for N minutes to bound memory growth.

**When to use:** `publicGroup.Use(ratelimit.PerIP(rate.Limit(...), burst))` placed before the CAPTCHA-verify middleware (fail cheap before calling out to Cloudflare's `siteverify` on every retry).

**Example:**
```go
// Source: standard idiomatic Go pattern, e.g. alexedwards.net/blog/how-to-rate-limit-http-requests
// (canonical reference for this exact "visitor map + cleanup goroutine" shape)
type visitor struct {
    limiter  *rate.Limiter
    lastSeen time.Time
}

type IPRateLimiter struct {
    mu       sync.Mutex
    visitors map[string]*visitor
    r        rate.Limit
    b        int
}

func (i *IPRateLimiter) getLimiter(ip string) *rate.Limiter {
    i.mu.Lock()
    defer i.mu.Unlock()
    v, exists := i.visitors[ip]
    if !exists {
        limiter := rate.NewLimiter(i.r, i.b)
        i.visitors[ip] = &visitor{limiter, time.Now()}
        return limiter
    }
    v.lastSeen = time.Now()
    return v.limiter
}

func (i *IPRateLimiter) Middleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        if !i.getLimiter(c.ClientIP()).Allow() {
            c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "rate_limited"})
            return
        }
        c.Next()
    }
}
```

### Anti-Patterns to Avoid

- **Calling the two existing public Flow-A HTTP calls (`POST /:id` then `POST /:id/slip`) back-to-back from the public handler:** this reintroduces a `draft`-then-optional-slip window that Flow B's success criteria explicitly forbid (record must land in `pending_review`, slip is mandatory). Build the atomic path instead (Pattern 1).
- **Trusting a client-side `captchaSuccess: true` flag in the request body:** always re-verify the Turnstile token server-side via `siteverify` — a bot can simply omit calling the widget and forge the flag.
- **Reusing `bffForward`/`getBffToken` for the public route:** it will 401 every unauthenticated donor immediately. Build the session-less passthrough (Pattern 2).
- **Storing the Turnstile secret key in a DB-editable settings table:** breaks the "all secrets are env-only" invariant this codebase has held since Phase 1 (`DONAREC_KEK`, `MinIO` credentials, Keycloak client secret are all env vars, never DB rows).
- **Adding the public form's `/donate` route to `middleware.ts`'s auth matcher:** it must stay reachable unauthenticated by design; the matcher already correctly excludes it by omission (UI-SPEC's Architecture Change Required section flags this explicitly).

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| CAPTCHA challenge + verification | A custom image/puzzle CAPTCHA or a bespoke "prove you're human" heuristic | Cloudflare Turnstile (`siteverify`) | D-82 already locks this; hand-rolled bot detection is a well-known losing arms race, and Turnstile is free, privacy-preserving, and has an official verification API |
| Magic-byte file-type detection | Sniffing bytes manually / trusting `Content-Type` header | `gabriel-vasile/mimetype` (already the project standard, `internal/storage.validateSlip`) | Extension/header spoofing is trivial; this library already correctly detects jpg/png/pdf and is proven in Flow A |
| PII envelope encryption | A new encryption scheme for the public path | `internal/crypto` (AES-256-GCM envelope, unchanged) | The exact same field (`donor_tax_id_enc`/`dek`) with the exact same threat model — there is no reason for Flow B to encrypt differently |
| Rate limiting | A custom sliding-window counter in Postgres | `golang.org/x/time/rate` per-IP token bucket (in-memory) | Well-understood, allocation-cheap, no new infrastructure (no Redis needed); a DB-backed counter would add write load to Postgres on every single public request, including rejected/bot ones |
| Gap-less receipt numbering for the "reference number" | Any numbering scheme that resembles or could collide with the receipt-number allocator | `donation.id` (UUID) or a short code derived from it, generated at insert time, NOT via `internal/receiptno` | D-84 is explicit: reference number ≠ receipt number; the receipt allocator (`internal/receiptno.Allocator`) has exactly ONE call site (`Approve`) by design (D-35) — Phase 6 must never add a second |

**Key insight:** almost nothing in this phase is a genuinely new technical problem — the risk is entirely in *composition*: making sure the new unauthenticated seam reuses every existing security control (encryption, magic-byte validation, audit, SoD-adjacent identity resolution) without silently weakening any of them, and that the new atomic transaction doesn't leave the "mandatory slip" invariant only conditionally enforced.

## Common Pitfalls

### Pitfall 1: Audit `ActorID` must be a UUID string — the seeded system user's `keycloak_subject` cannot be a human-readable sentinel
**What goes wrong:** `internal/audit/service.go`'s `AppendAuditEntryTx` calls `parseUUID(entry.ActorID)` and returns an error (which rolls back the entire transaction, including the just-created `pending_review` donation) if `ActorID` is not UUID-shaped. If the `public-web` system user is seeded with `keycloak_subject = 'public-web'` (a readable string, which looks natural given the name in D-76), **every single public submission will fail** at the audit step.
**Why it happens:** the audit table's `actor_id` column and the `parseUUID` guard were designed around real Keycloak `sub` claims, which are always UUIDs — nobody anticipated a synthetic non-Keycloak actor when that constraint was written (Phase 1/3).
**How to avoid:** seed the `public-web` system user's `keycloak_subject` column (and, for consistency, `users.id`) with a **fixed, literal UUID** (e.g. a constant declared in the migration file's comment and mirrored as a Go constant for the handler to reference when building the synthetic audit actor). Migration precedent for seed-via-migration already exists (`000011_receipt_template_config.up.sql`, `000014_edonation_config.up.sql` both do `INSERT INTO … DEFAULT VALUES`); this phase's seed is the first to need an explicit non-default `INSERT`.
**Warning signs:** integration test for the public submit path returns a 500 with an "audit: invalid actor_id" error; or (worse, if only unit-tested with mocks) this passes tests but fails at the real HTTP layer — exactly the class of bug this project's Conventions "Integration-test gate" was added to catch (Phase 3's `created-by-fk-mismatch`/`fe-be-audience-mismatch`/RBAC-AND bugs were all found only when driven with a real HTTP path).

### Pitfall 2: `sqlc` column-order fragility on `ALTER TABLE ADD COLUMN`
**What goes wrong:** adding the `source` column to `donations` and not also updating `GetDonationByID`'s (and any other `SELECT *`/explicit-column-list query touching `donations`) column list in physical order can either break the sqlc-generated `Donation` struct binding or silently produce a struct field that never populates.
**Why it happens:** this codebase already hit this exact class of bug once — STATE.md documents (Phase 05, 05-01): *"GetDonationByID's SELECT list extended to include edonation_keyed_at/edonation_keyed_by (physical column order) so sqlc keeps reusing the Donation model type after migration 000013's ALTER TABLE — required to keep go build green."*
**How to avoid:** after writing the `source` column migration, run `sqlc generate` and manually verify `GetDonationByID`'s SELECT list (and any other query selecting the full `donations` row) includes `source` in the same physical position sqlc expects, exactly mirroring the 000013 fix.
**Warning signs:** `go build` fails after `sqlc generate` with a field-count/type mismatch on `db.Donation`, or (silently worse) `source` always reads back as the zero value in Go even though the DB row is correct.

### Pitfall 3: `CreateDonationRequest`'s validator tags are shared — don't let CAPTCHA/rate-limit concerns leak into the domain request struct
**What goes wrong:** it's tempting to add `TurnstileToken string \`validate:"required"\`` directly onto `CreateDonationRequest` (or a Flow-B variant of it) so `go-playground/validator` enforces its presence "for free." This conflates a **transport/security concern** (CAPTCHA token) with the **domain request** (donor fields), and if `CreateDonationRequest` is ever reused by Flow A's `UpdateDraft` codepath (it currently is NOT the same struct, but `PublicDonationRequest` will likely be a near-copy), a stray required CAPTCHA field breaks unrelated tests.
**How to avoid:** verify the CAPTCHA token in the **middleware layer** (before the handler even parses the domain fields), not as a validated field on the domain request struct. The middleware either aborts with 4xx or calls `c.Next()` — the handler's `PublicDonationRequest` should contain donor fields only, matching the existing separation of concerns (`RequireAuth`/`RequireAnyRole` are middleware, never struct fields).
**Warning signs:** CAPTCHA validation errors surface as `422 validation_failed` with a `details` string mentioning `turnstile_token` instead of a distinct, CAPTCHA-specific error shape the frontend can key off of (UI-SPEC's Copywriting Contract expects a *distinct* message for CAPTCHA failure vs. field validation failure).

### Pitfall 4: Multipart body parsing order — validate BEFORE any DB write, not after
**What goes wrong:** if the handler calls `donationSvc.CreatePublicSubmission` before or in parallel with `storage.PutSlip`, a slip validation failure (wrong file type, too large) after the donation row is already committed leaves a `pending_review` record that violates D-80's "slip mandatory" invariant, with no way to attach the slip later (no authenticated follow-up exists for a donor, D-86).
**How to avoid:** strict ordering in the handler: (1) rate-limit check, (2) Turnstile verify, (3) parse multipart fields, (4) `storage.PutSlip` (magic-byte + size, fails fast, no DB writes yet), (5) `donationSvc.CreatePublicSubmission` (one atomic tx: create+submit+slip-reference+audit+outbox). See Pattern 1's ordering rationale.
**Warning signs:** a `pending_review` Flow-B record in staging with no linked `slip_attachments` row — should be structurally impossible if the ordering above is followed; add an integration test asserting this invariant.

### Pitfall 5: Turnstile requires outbound egress — don't assume it "just works" in every deployment target
**What goes wrong:** if the production host has restricted egress (an on-prem/air-gapped hospital deployment, flagged as an open stakeholder item in REQUIREMENTS.md's "Hosting" row), the Go backend's call to `https://challenges.cloudflare.com/turnstile/v0/siteverify` will time out or fail, and — if the verifier isn't wired with a sane failure mode — could either silently accept all submissions (fail-open, a security regression) or reject 100% of legitimate donors (fail-closed, an availability regression).
**How to avoid:** the `internal/captcha.Verifier` interface (D-82) should default to **fail-closed** (reject the submission on any verification error, including network timeout) with a clear, distinct log line and HTTP error the frontend can render as "ไม่สามารถโหลดระบบยืนยันตัวตนได้" (already specified in UI-SPEC's CAPTCHA/Rate-Limit Copy table). This is a deliberate MVP tradeoff (availability sacrificed for security) that should be called out explicitly to the planner/stakeholder, not silently decided.
**Warning signs:** none observable until a real network-restricted deployment; flag this in Open Questions for stakeholder awareness (matches REQUIREMENTS.md's existing "Hosting (on-prem vs cloud)" stakeholder-confirmation row).

## Code Examples

### Extending `SearchDonations`/`CountDonations` with a `source` filter (FR-08)

```sql
-- Source: internal/db/queries/donations.sql — exact existing D-53 nullable-narg pattern,
-- extended with one new AND clause. No new query needed; Screen 11 (Queue) calls the
-- SAME endpoint Screen 1 (Donation List) uses, with status='pending_review' pinned by
-- the handler and source passed through from the ?source= query param.
SELECT
    d.id, d.status, d.donor_name, d.donated_at, d.amount, d.receipt_formatted,
    d.created_at, d.approved_at, d.created_by, d.edonation_keyed, d.source,
    u.display_name AS created_by_name
FROM donations d
LEFT JOIN users u ON u.id = d.created_by
WHERE
    (sqlc.narg('donor_name')::TEXT           IS NULL OR d.donor_name ILIKE '%' || sqlc.narg('donor_name') || '%')
    AND (sqlc.narg('status')::donation_status IS NULL OR d.status = sqlc.narg('status'))
    AND (sqlc.narg('from_date')::DATE         IS NULL OR d.donated_at >= sqlc.narg('from_date'))
    AND (sqlc.narg('to_date')::DATE           IS NULL OR d.donated_at <= sqlc.narg('to_date'))
    AND (sqlc.narg('receipt_no')::TEXT        IS NULL OR d.receipt_formatted = sqlc.narg('receipt_no'))
    AND (sqlc.narg('source')::TEXT            IS NULL OR d.source = sqlc.narg('source'))
ORDER BY d.created_at DESC
LIMIT @limit_n OFFSET @offset_n;
```

### Turnstile server-side verification (Go)

```go
// Source: developers.cloudflare.com/turnstile/get-started/server-side-validation/
// (official docs) — shape adapted to this project's narrow-interface convention
// (mirrors mailer.EmailSender / worker.PDFRenderer's "define the seam here" style).
package captcha

type Verifier interface {
    Verify(ctx context.Context, token, remoteIP string) error
}

type TurnstileVerifier struct {
    secretKey string // from env TURNSTILE_SECRET_KEY, never DB-stored
    client    *http.Client
}

var ErrCaptchaFailed = errors.New("captcha: verification failed")

func (v *TurnstileVerifier) Verify(ctx context.Context, token, remoteIP string) error {
    if token == "" {
        return ErrCaptchaFailed
    }
    form := url.Values{
        "secret":   {v.secretKey},
        "response": {token},
        "remoteip": {remoteIP},
    }
    req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
        "https://challenges.cloudflare.com/turnstile/v0/siteverify",
        strings.NewReader(form.Encode()))
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

    resp, err := v.client.Do(req)
    if err != nil {
        return fmt.Errorf("%w: %v", ErrCaptchaFailed, err) // fail-closed (Pitfall 5)
    }
    defer resp.Body.Close()

    var result struct {
        Success    bool     `json:"success"`
        ErrorCodes []string `json:"error-codes"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || !result.Success {
        return ErrCaptchaFailed
    }
    return nil
}
```

### Session-less Next.js passthrough route (FE)

```typescript
// Source: pattern adapted from app/api/bff/donations/[id]/slip/route.ts —
// SAME multipart re-post shape, but deliberately WITHOUT getBffToken()/
// getServerSession()/Authorization header, since there is no donor session.
// app/api/public/donations/route.ts
export async function POST(request: NextRequest): Promise<Response> {
  let incoming: FormData;
  try {
    incoming = await request.formData();
  } catch {
    return NextResponse.json({ error: "invalid_request_body" }, { status: 400 });
  }

  // Re-post as a FRESH FormData — no Authorization header (D-78: unauthenticated seam).
  const goRes = await fetch(`${API_BASE_URL}/api/public/donations`, {
    method: "POST",
    body: incoming, // fetch regenerates the multipart boundary
  });

  return passthroughGoResponse(goRes); // reused helper from lib/bff.ts
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|---------------|--------|
| Google reCAPTCHA v2/v3 for bot protection | Cloudflare Turnstile | Turnstile GA'd 2023, now widely adopted as the privacy-preserving alternative (no third-party tracking cookie, no user-facing "select all traffic lights") | Aligns with D-82's explicit PDPA-first rationale; no user profiling behavior to disclose in the consent text |
| Server-restarts-required config for CAPTCHA/rate-limit thresholds | Env-var driven (MVP) with a documented path to DB-configurable later (NFR-09 spirit) | N/A — MVP scoping decision, not an industry shift | Rate-limit numbers can be tuned via redeploy for MVP; a later phase can move non-secret thresholds to a settings table if operational friction justifies it |

**Deprecated/outdated:** none directly relevant — this is a first-time integration, not a migration off an older pattern.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Turnstile secret key should be an env var (not DB-stored) for MVP | Standard Stack (Alternatives Considered) | Low — this is a recommendation following the codebase's existing "all secrets are env-only" pattern, not a hard external fact; if the team wants a DB-editable secret, that's a straightforward deviation, not a rework |
| A2 | The public multipart POST should bundle donor fields + slip file + CAPTCHA token in ONE request (rather than a two-step create-then-upload) | Architecture Patterns (Pattern 1) | Medium — this is the load-bearing design recommendation for FR-01/FR-02/FR-08's "lands directly in pending_review" criterion; if the planner instead chooses a two-step public flow, the atomicity guarantee (Pitfall 4) must be re-derived some other way (e.g. a short-lived "pending upload" status) |
| A3 | The reference number shown to the donor (D-84) should be the `donation.id` UUID (or a short code derived from it) generated at INSERT time, not a separate allocator | Don't Hand-Roll | Low — UI-SPEC's example (`REF-2569-000482`) suggests a formatted short code, not a raw UUID; the exact format is explicitly left to Claude's Discretion in CONTEXT, so this is a starting recommendation, not a locked fact |
| A4 | `golang.org/x/time/rate` in-memory per-process limiting is sufficient for MVP (no Redis) | Standard Stack | Low — CONTEXT explicitly notes "ปัจจุบัน stack ไม่มี Redis → เริ่ม in-memory/DB ได้", confirming this is an accepted MVP tradeoff, not a risk introduced by this research |
| A5 | Cloudflare Turnstile verifier should fail-closed (reject) on network/timeout errors rather than fail-open | Common Pitfalls (Pitfall 5) | Medium — this is a security-vs-availability tradeoff not explicitly locked by CONTEXT; if hospital hosting turns out to have restricted egress (an open REQUIREMENTS.md stakeholder item), fail-closed could make the public form completely unusable until that's resolved — flagged for stakeholder awareness |

## Open Questions

1. **Should the public form's Next.js route proxy through a session-less BFF passthrough, or should the browser call the Go API directly (requiring CORS)?**
   - What we know: every existing browser→Go path in this codebase goes through the session-bound BFF (`bffForward`), which cannot be reused as-is for an unauthenticated donor. The codebase has never configured CORS on the Go API (`cmd/server/main.go` has no CORS middleware today).
   - What's unclear: CONTEXT/D-78 says the public form "hits the Go API" but doesn't specify browser-direct vs. proxied.
   - Recommendation: proxy via a new session-less Next.js route family (Pattern 2) — avoids introducing CORS configuration for the first time in this codebase, keeps the Go API's attack surface limited to same-origin-adjacent traffic (Next.js server), and keeps the architectural invariant "browser never talks to Go directly" intact. Flag for planner confirmation.

2. **Exact rate-limit numeric defaults (requests per IP per window) for submit and CAPTCHA-verify.**
   - What we know: CONTEXT marks this as Claude's Discretion; no numeric target given.
   - What's unclear: hospital-scale traffic expectations (donor volume is likely low — dozens/day, not thousands) aren't specified anywhere in REQUIREMENTS.md/PROJECT.md.
   - Recommendation: start conservative but not punitive — e.g. 5 submissions per IP per 10 minutes, with a `429` response mapped to the UI-SPEC's already-drafted rate-limit copy. Make both the window and count env-configurable from day one (matches `WorkerConfig`'s existing env-driven-tunable pattern) so it can be adjusted without a code change.

3. **Should `ack_email` reuse the `email_delivery` table for delivery-status tracking, or is on-screen confirmation + best-effort email sufficient?**
   - What we know: `email_delivery` (migration 000010) is currently written by the `issue_receipt` job handler only; its schema (`donation_id`, `status`, `provider_message_id`, `attempts`, `last_error`) is generic enough to record an `ack_email` attempt too.
   - What's unclear: whether staff need a "did the ack email actually send" visibility (Screen 3's `EmailDeliveryPanel` currently shows receipt-email delivery status only) — D-85/D-86 don't mention staff-side ack-email visibility as in-scope.
   - Recommendation: reuse `email_delivery` for consistency (avoid a second delivery-tracking table for what is structurally the same "one row per send attempt" concern) but do NOT build new UI to surface it this phase — out of the locked success criteria. Flag for planner: cheap to write, zero UI cost, keeps a future "did the donor actually get their ack" support-debugging path open.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Outbound HTTPS egress to `challenges.cloudflare.com` | Turnstile server-side verification (FR-04) | ✗ not verifiable from this research pass — depends on the deployment target (dev/staging confirmed to have general internet egress via existing `npm`/`go mod`/Google Fonts usage; production hosting is an open REQUIREMENTS.md stakeholder item) | — | None built this phase — D-82's swappable `Verifier` interface is the designed escape hatch if egress turns out to be restricted; a future phase would need to implement an alternative (self-hosted challenge, or an allow-listed egress proxy) |
| `golang.org/x/time` module | Rate limiting (FR-04) | ✓ (confirmed live on the Go module proxy) | v0.15.0 | — |
| `@marsidev/react-turnstile` npm package | Turnstile React wrapper | ✓ (confirmed on npm registry, OK legitimacy verdict) | 1.5.3 | Raw `<script>` tag (UI-SPEC-approved fallback, zero new dependency) |
| MinIO / object storage | Slip upload (FR-02) | ✓ already provisioned and used by Flow A (`internal/storage`) | existing | — |
| Postgres | All new schema/data (FR-08, D-76/77) | ✓ already provisioned | existing (17 / 16+ acceptable) | — |
| Existing outbox worker process | `ack_email` dispatch (FR-05) | ✓ already running (`cmd/server/main.go` `go outboxWorker.Run(ctx)`) | existing | — |

**Missing dependencies with no fallback:**
- Confirmed production egress to Cloudflare's `siteverify` endpoint — this is an existing REQUIREMENTS.md stakeholder-confirmation item ("Hosting — on-prem vs cloud", Phase 1/4 row) that directly affects Phase 6's CAPTCHA choice; not newly introduced by this research, but Phase 6 is the first phase where it becomes load-bearing.

**Missing dependencies with fallback:**
- `@marsidev/react-turnstile` → raw official `<script>` tag if the team prefers zero new npm dependencies.

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework (backend) | `go test` + `testify` + `testcontainers-go` (Postgres + Keycloak + MinIO modules) — established since Phase 1/3 |
| Framework (frontend) | `vitest` (`npm run test` → `vitest run`) — established since Phase 3 |
| Config file (backend) | none dedicated — standard `go test ./...`; testcontainers spin up per-package in `internal/testutil/` |
| Config file (frontend) | `donnarec-web/vitest.config.ts` (existing) |
| Quick run command (backend) | `go test ./internal/donation/... ./internal/captcha/... ./internal/ratelimit/... -run TestPublic -v` |
| Quick run command (frontend) | `npm run test -- PublicDonationForm` (from `donnarec-web/`) |
| Full suite command (backend) | `go test ./... -race` (matches the project's existing concurrency-sensitive test discipline — see `internal/receiptno/allocator_concurrency_test.go` precedent) |
| Full suite command (frontend) | `npm run test` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| FR-01 | Public form submit creates a `pending_review` record with `source='flow_b'` | integration (real HTTP path — Conventions gate) | `go test ./cmd/server/... -run TestPublicSubmit_E2E -v` | ❌ Wave 0 |
| FR-02 | Slip upload rejected on bad magic bytes / oversized file, accepted on valid jpg/png/pdf | unit + integration | `go test ./internal/donation/... -run TestCreatePublicSubmission_SlipValidation -v` | ❌ Wave 0 |
| FR-03 | Consent fields captured with the Flow-B-specific `consent_text_version` | unit | `go test ./internal/donation/... -run TestCreatePublicSubmission_Consent -v` | ❌ Wave 0 |
| FR-04 | CAPTCHA-verify middleware rejects missing/invalid token; rate-limit middleware 429s after threshold | unit (middleware, injectable fake `Verifier`) | `go test ./internal/captcha/... ./internal/ratelimit/... -v` | ❌ Wave 0 |
| FR-05 | `ack_email` outbox job dispatched, worker sends bilingual "not a receipt" email, does not block the submit HTTP response | integration (worker `ProcessOnce` driven synchronously, matches `issue_receipt`'s existing test shape) | `go test ./internal/worker/... -run TestAckEmail -v` | ❌ Wave 0 |
| FR-06 | Language toggle sets both UI locale and `donor_language` sent on submit | frontend unit | `npm run test -- PublicDonationForm.locale` | ❌ Wave 0 |
| FR-08 | Queue endpoint filters by `source`; Flow A record never appears under `source=flow_b` filter and vice versa | integration | `go test ./internal/donation/... -run TestSearchDonations_SourceFilter -v` | ❌ Wave 0 |
| NFR-06 | Responsive breakpoint behavior (card full-bleed on mobile, 2-col reflow on desktop) | manual (visual, no automated snapshot infra in this codebase today) + `checkpoint:human-verify` | — | manual-only, matches existing project convention (no visual regression tooling installed) |
| **Cross-cutting** | Full unauthenticated HTTP path (`router → RateLimitByIP → VerifyTurnstile → handler → service → DB`) driven by a real multipart request, per Conventions' Integration-test gate | integration (E2E) | `go test ./cmd/server/... -run TestPublicDonationE2E -race -v` | ❌ Wave 0 — **this is the load-bearing gate**; per CLAUDE.md Conventions, Phase 6 is NOT "done" without it, since it touches the runtime request seam for the first time as an unauthenticated route |

### Sampling Rate
- **Per task commit:** targeted `go test ./internal/<changed-package>/... -v` and/or `npm run test -- <ChangedComponent>`
- **Per wave merge:** `go test ./... -race` (backend) + `npm run test` (frontend)
- **Phase gate:** full suite green AND the E2E integration test (real HTTP path, real Turnstile-fake/injected verifier, real rate-limit middleware) passing, per CLAUDE.md's Integration-test gate — this phase is explicitly named in that convention's example list ("HTTP routing, auth middleware... RBAC/route guards... DB writes behind those layers") since it adds an entirely new middleware chain shape (CAPTCHA+rate-limit replacing RequireAuth)

### Wave 0 Gaps
- [ ] `internal/captcha/turnstile_test.go` — fake HTTP server standing in for Cloudflare's `siteverify`, covers success/failure/timeout(fail-closed) cases
- [ ] `internal/ratelimit/middleware_test.go` — covers allow/deny/burst/cleanup
- [ ] `internal/donation/create_public_submission_test.go` — the atomic-transaction unit tests (mirrors `service_test.go`'s existing shape for `Create`/`Approve`)
- [ ] `cmd/server/e2e_public_test.go` — the load-bearing full-HTTP-path integration test (extends the existing `cmd/server/e2e_test.go` pattern, but for the unauthenticated group)
- [ ] `donnarec-web/components/__tests__/PublicDonationForm.test.tsx` — form validation + slip-required + consent-required + Turnstile-token-required gating on the submit button
- [ ] Framework install: none — `go test`/`vitest` already fully wired; no new test framework needed

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | No (this route is deliberately unauthenticated by design, D-78) | N/A — the control here is CAPTCHA + rate limiting, not authentication |
| V3 Session Management | No | N/A — stateless public submission, no session created |
| V4 Access Control | Partial | The public route must be provably unreachable for any mutating action beyond "create one pending_review record" — no update/delete/reveal-PII/approve capability exists on this route group; verify by code review that `publicGroup` only ever registers the single `POST /donations` handler |
| V5 Input Validation | Yes | `go-playground/validator` struct tags (existing pattern) for donor fields; magic-byte detection (`gabriel-vasile/mimetype`, existing) for the slip file; explicit allow-list of MIME types, never an extension/header check |
| V6 Cryptography | Yes | `internal/crypto` AES-256-GCM envelope encryption, unchanged, applied identically to Flow A's `donor_tax_id_enc`/`dek` |
| V11 Business Logic (relevant addition — ASVS "anti-automation") | Yes | CAPTCHA (Turnstile) + per-IP rate limiting is exactly ASVS's anti-automation control family; this is the primary new threat surface this phase introduces |

### Known Threat Patterns for this stack

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Automated mass-submission (spam/DoS via the new unauthenticated endpoint) | Denial of Service | CAPTCHA (Turnstile server-verified) + per-IP rate limiting (D-83, defense-in-depth pair) |
| CAPTCHA bypass via forged client-side "success" flag | Tampering | Server-side `siteverify` call is mandatory and authoritative — client state is never trusted (Common Pitfalls, Anti-Patterns) |
| Slip file-type spoofing (rename a script/executable to `.jpg`) | Tampering / Elevation of Privilege | Magic-byte detection (`mimetype.Detect`), never `Content-Type` header or filename extension — reused verbatim from Flow A |
| PII exposure via the new unauthenticated write path (e.g. verbose error messages leaking whether a tax ID already exists elsewhere) | Information Disclosure | The public handler must return generic validation errors, never differential responses that could be used to enumerate existing donor records (this codebase has no donor master/dedup yet — D-43 — so this risk is currently low, but flag for the next phase that adds donor dedup) |
| Audit-trail bypass or forgery on the public path | Repudiation | Every public submission is audited via `AppendAuditEntryTx` with a fixed, well-known system-user actor (Pitfall 1) — auditable even though there's no real authenticated identity behind the submission |
| Turnstile secret key leakage | Information Disclosure | Env-var only (never DB-stored, never logged — `zapRequestLogger` already never logs request bodies/headers, Pattern C) |
| Resource exhaustion via oversized/many slip uploads before rate-limit kicks in | Denial of Service | Rate-limit middleware runs BEFORE the multipart body is even fully parsed where possible (Gin's `MaxMultipartMemory` + the existing 10MB `maxSlipSize` cap in `internal/storage` bound worst-case memory per request) |

## Sources

### Primary (HIGH confidence)
- `donnarec-api/cmd/server/main.go`, `internal/donation/service.go`, `internal/donation/slip_service.go`, `internal/storage/client.go`, `internal/auth/middleware.go`, `internal/auth/rbac.go`, `internal/worker/worker.go`, `internal/worker/issue_receipt.go`, `internal/audit/service.go`, `internal/config/config.go`, `internal/mailer/sender.go`, `internal/users/service.go`, `internal/db/queries/donations.sql`, `migrations/000001…000014` — direct codebase inspection, 2026-07-11 [VERIFIED: donnarec-api codebase]
- `donnarec-web/middleware.ts`, `app/layout.tsx`, `lib/bff.ts`, `app/api/bff/donations/[id]/slip/route.ts`, `components/ConsentBlock.tsx`, `package.json`, `messages/th.json` — direct codebase inspection, 2026-07-11 [VERIFIED: donnarec-web codebase]
- `.planning/phases/06-public-donation-web-form-flow-b/06-CONTEXT.md`, `06-UI-SPEC.md`, `.planning/REQUIREMENTS.md`, `.planning/STATE.md`, `.planning/config.json` — direct file read, 2026-07-11
- Cloudflare Turnstile server-side validation docs — https://developers.cloudflare.com/turnstile/get-started/server-side-validation/ [CITED]
- Go module proxy (`proxy.golang.org/golang.org/x/time/@latest`) — confirmed v0.15.0 live, 2026-07-11 [VERIFIED: Go module proxy]
- npm registry (`npm view @marsidev/react-turnstile`) + `gsd-tools query package-legitimacy check` — verdict `OK`, 2026-07-11 [VERIFIED: npm registry]

### Secondary (MEDIUM confidence)
- Alex Edwards, "How to Rate Limit HTTP Requests in Go" — https://www.alexedwards.net/blog/how-to-rate-limit-http-requests — canonical per-IP `golang.org/x/time/rate` middleware pattern reference [CITED]

### Tertiary (LOW confidence)
- None used as load-bearing claims — all external findings were cross-checked against either official docs or an authoritative registry/proxy.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — every backend dependency is either already in `go.mod` or a single, officially-sourced addition (`golang.org/x/time`) confirmed live on the module proxy; frontend addition confirmed OK via the package-legitimacy seam.
- Architecture: HIGH — every pattern is either a direct extension of an existing, tested codebase pattern (`Approve`'s atomic-tx shape, `SearchDonations`'s nullable-narg filter, the BFF multipart-passthrough shape) or a well-documented standard Go idiom (per-IP rate limiting) with an official-docs-backed external API (Turnstile `siteverify`).
- Pitfalls: HIGH — Pitfalls 1 and 2 are drawn directly from this codebase's own documented history (STATE.md's Phase 3/5 postmortems on audit ActorID and sqlc column-order fragility), not generic advice; Pitfall 5 is a genuine open risk correctly flagged as MEDIUM in the Assumptions Log, not overstated as fact.

**Research date:** 2026-07-11
**Valid until:** ~30 days for the internal architecture findings (codebase-derived, stable); ~90 days for the Cloudflare Turnstile API shape (stable, versioned public API); re-verify `golang.org/x/time` and `@marsidev/react-turnstile` versions at plan time if execution is delayed more than a few weeks.
