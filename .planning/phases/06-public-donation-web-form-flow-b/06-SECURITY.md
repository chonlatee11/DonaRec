---
phase: 06-public-donation-web-form-flow-b
audited: 2026-07-17T20:20:00Z
asvs_level: 1
block_on: high
threats_total: 37
threats_mitigate: 30
threats_accept: 7
threats_closed: 37
threats_open: 0
unregistered_flags: 2
unregistered_flags_open: 0
status: SECURED
escalation_note: "RESOLVED 2026-07-17 (quick task 260717-spx). Declared threat register was 37/37 CLOSED; the two escalated High public-surface flags (unauthenticated multipart disk-exhaustion + rate-limiter-defeating IP spoof) are now both fixed with code + threat-model-grade evidence + new E2E regression tests (commits becedfd, 72dca94; TestPublicDonationE2E 7/7 pass). User chose fix-both over accept-risk. See Unregistered Flags → Resolution section."
---

# Phase 06: Security Audit Report

**Audited:** 2026-07-17
**ASVS Level:** 1 (grep-level: mitigation present in the cited file, and re-verified against real code/tests, not SUMMARY.md claims)
**block_on:** high (only high/critical OPEN threats would block ship)

## Methodology

Every threat declared in `06-01..06-08-PLAN.md`'s `<threat_model>` blocks was verified
against the CURRENT implementation — never accepted on documentation or SUMMARY.md's
self-reported "Threat Mitigations" section alone. Each `mitigate` threat was closed
only after locating the actual code path (route wiring in `main.go`, the exact
validation/encryption/audit call, or a passing test asserting the behavior). Where the
plan pointed at unit/E2E tests as evidence, those tests were **re-run live in this
audit** (Docker/testcontainers available) rather than trusted from the SUMMARY:

```
go build ./...                                                          — clean
go test ./internal/captcha/... ./internal/ratelimit/... -count=1        — 9 passed
go test ./internal/donation/... -run TestCreatePublicSubmission -count=1 — 3 passed
go test ./cmd/server/... -run TestPublicDonationE2E -race -count=1       — 5 passed
  (HappyPath_AtomicPendingReviewFlowB, BadMagicByteSlip_Rejected_NoRow,
   MissingSlip_Rejected_NoRow, RateLimit_ExceedingThreshold_429, + parent)
go test ./internal/worker/... -run TestAckEmail -count=1                 — 5 passed
go test ./internal/donation/... -run TestSearchDonations_SourceFilter -count=1 — 4 passed
npm run build (donnarec-web)                                             — clean
npm run test -- PublicDonationForm                                       — 4 passed
```

`accept` threats were checked for a plan-time rationale and are recorded below in the
Accepted Risks Log (this SECURITY.md IS that log — first audit of this phase, so no
prior entries existed; every `accept`-disposition threat is logged here for the first
time and closes on that basis, matching the phase 05 precedent).

## Threat Verification

### 06-01 — Source column + public-web system user + source filter

| Threat ID | Category | Severity | Disposition | Status | Evidence |
|-----------|----------|----------|-------------|--------|----------|
| T-06-01 | Tampering | medium | mitigate | CLOSED | `migrations/000015_donation_source.up.sql:28-30` — `ALTER TABLE donations ADD COLUMN source TEXT NOT NULL DEFAULT 'flow_a' CONSTRAINT chk_donations_source CHECK (source IN ('flow_a','flow_b'))`. DB-layer CHECK backstop confirmed. |
| T-06-02 | Repudiation | high | mitigate | CLOSED | `migrations/000016_seed_public_web_user.up.sql:44-53` — seeds `users.id = users.keycloak_subject = '00000000-0000-4000-8000-000000000006'` (valid UUID literal, never a readable sentinel). Mirrored as `PublicWebUserID` Go constant in `internal/donation/public_submission.go:50`. |
| T-06-03 | Elevation of Privilege | high | mitigate | CLOSED | `migrations/000016_seed_public_web_user.up.sql:59-61` — assigns only the least-privileged `maker` role; no Keycloak credential is ever issued for this identity (comment + design confirmed; the user exists purely as an FK/audit target). |
| T-06-04 | Injection | medium | mitigate | CLOSED | `internal/donation/handler.go:540-545` — `?source=` allow-lists `flow_a`/`flow_b`, 400 `invalid_source` otherwise; `internal/db/queries/donations.sql:265,288` — `sqlc.narg('source')::TEXT` parameterized predicate, zero string concatenation. Live-verified: `TestSearchDonations_SourceFilter` (4 subtests) PASS. |
| T-06-SC (06-01) | Tampering (supply chain) | low | accept | CLOSED-accepted | No new package this plan; `go.mod` unchanged. Logged in Accepted Risks Log below. |

### 06-02 — Anti-automation primitives (Turnstile + rate limit)

| Threat ID | Category | Severity | Disposition | Status | Evidence |
|-----------|----------|----------|-------------|--------|----------|
| T-06-05 | Denial of Service | high | mitigate | CLOSED\* | `cmd/server/main.go:366-369` — `publicGroup.Use(ratelimit.PerIP(...))` registered BEFORE `captchaMW.VerifyTurnstile()`, both before the handler/DB/siteverify call. Live-verified: `TestPublicDonationE2E/RateLimit_ExceedingThreshold_429` PASS. **\*Closed at ASVS L1 (mitigation present exactly as declared) — but see Unregistered Flag "gin ClientIP() trust-all-proxies" below: the per-IP identity key this mitigation keys on is attacker-spoofable in the current deployment config, which materially weakens (does not eliminate — CAPTCHA remains a second, non-bypassable layer) this control's real-world effectiveness.** |
| T-06-06 | Tampering | high | mitigate | CLOSED | `internal/captcha/turnstile.go:60-94` — `Verify()` is the ONLY signal accepted; no client-asserted success field exists anywhere in `PublicDonationRequest` (`internal/donation/model.go:42-54` has no captcha field). |
| T-06-07 | Denial of Service | high | mitigate | CLOSED | `internal/captcha/turnstile.go:76-89` — transport error, decode error, and `success=false` all return `fmt.Errorf("%w: ...", ErrCaptchaFailed)`; nil is returned ONLY on genuine `success=true`. Unit-verified (fake-server success/failure/timeout subtests) PASS. |
| T-06-08 | Information Disclosure | high | mitigate | CLOSED | `internal/config/config.go:125,198` — `TurnstileSecretKey` sourced only from `os.Getenv("TURNSTILE_SECRET_KEY")`, no DB column, no default fallback (empty ⇒ fail-closed per T-06-07); never logged (grep confirms no `zap.String("secret"...)`-style call in `internal/captcha/*.go`). |
| T-06-SC (06-02) | Tampering (supply chain) | medium | mitigate | CLOSED | `go.mod:27` — `golang.org/x/time v0.15.0` pinned exactly, no floating version. |

### 06-03 — CreatePublicSubmission atomic path + public route + E2E gate

| Threat ID | Category | Severity | Disposition | Status | Evidence |
|-----------|----------|----------|-------------|--------|----------|
| T-06-09 | Tampering / EoP | high | mitigate | CLOSED | `internal/storage/client.go:112-131` — `validateSlip` runs `mimetype.Detect` on buffered header bytes BEFORE any DB write; `internal/donation/public_handler.go:150-172` — `PutSlip` called (Step 3) before `CreatePublicSubmission` (Step 4). Live-verified: `TestPublicDonationE2E/BadMagicByteSlip_Rejected_NoRow` PASS. |
| T-06-10 | Information Disclosure | high | mitigate | CLOSED | `internal/donation/public_submission.go:94-99` — `crypto.EncryptField(ctx, s.keyProvider, []byte(req.DonorTaxID))` (AES-256-GCM envelope) runs BEFORE the transaction opens; only `encBytes`/`dekBytes` ciphertext reach `CreateDonationParams`. Live-verified: `TestCreatePublicSubmission` PASS. |
| T-06-11 | Elevation of Privilege | high | mitigate | CLOSED | `cmd/server/main.go:366-369` — `publicGroup` registers EXACTLY one route, `POST /donations`; no update/delete/reveal/approve handler is ever attached to this group (grep confirms only one `publicGroup.` call site). |
| T-06-12 | Repudiation | high | mitigate | CLOSED | `internal/donation/public_submission.go:186-193` — `AppendAuditEntryTx` runs IN-TX with `ActorID: PublicWebUserID` (the fixed UUID, never a readable string). Live-verified: `TestCreatePublicSubmission` asserts exactly one `donation.public_submit` audit row, PASS. |
| T-06-13 | Data integrity | high | mitigate | CLOSED | `internal/donation/public_submission.go:141-211` — single `dbhelpers.WithTx` closure wraps create+submit+slip+audit+outbox; slip already PUT+validated before the tx opens. Live-verified: `TestCreatePublicSubmission` rollback subtest (zero orphan rows) PASS; `TestPublicDonationE2E` bad-magic and missing-slip subtests confirm no DB row on failure, PASS. |
| T-06-14 | Information Disclosure | medium | mitigate | CLOSED | `internal/donation/public_handler.go:98-195` — every error branch returns a fixed, generic error shape (`validation_failed`, `slip_required`, `unsupported_file_type`, `file_too_large`, `missing_tax_id`, `public_submission_failed`); no branch differs based on whether a donor/tax-ID match already exists (no donor-master/dedup lookup exists in this handler at all). |
| T-06-SC (06-03) | Tampering (supply chain) | low | accept | CLOSED-accepted | No new package this plan (`x/time` added in plan 02). Logged in Accepted Risks Log below. |

### 06-04 — ack_email outbox job handler

| Threat ID | Category | Severity | Disposition | Status | Evidence |
|-----------|----------|----------|-------------|--------|----------|
| T-06-15 | Spoofing/Repudiation | high | mitigate | CLOSED | `internal/i18n/locales/th.json:75` / `en.json:75` — `ackEmail.body` explicitly states the submission "ยังไม่ใช่ใบเสร็จรับเงิน" / "This message is not a receipt"; `internal/worker/ack_email.go:146` — reference number derived via `donation.PublicReferenceNumber`, never `internal/receiptno`. |
| T-06-16 | Denial of Service | high | mitigate | CLOSED | `internal/worker/ack_email.go:70-108` — `handleAckEmail` returns a plain `error` on send failure (routed through `ProcessOnce`'s existing `MarkOutboxJobFailed`/backoff, `internal/worker/worker.go:220-222`), never touching the already-committed donation row. |
| T-06-17 | Information Disclosure | medium | mitigate | CLOSED | `internal/worker/ack_email.go:90-94` — the only log call in the no-email path logs `job_id` + `donation_id` only; no donor name/email/tax-ID field anywhere in the file (grep confirms zero `don.DonorName`/`don.DonorEmail` in any `zap.*` call). |
| T-06-SC (06-04) | Tampering (supply chain) | low | accept | CLOSED-accepted | No new package; reuses mailer/i18n/worker verbatim. Logged in Accepted Risks Log below. |

### 06-05 — (app)/(public) route-group split + warm theme scope

| Threat ID | Category | Severity | Disposition | Status | Evidence |
|-----------|----------|----------|-------------|--------|----------|
| T-06-18 | Tampering | high | mitigate | CLOSED | `donnarec-web/middleware.ts:20-25` — `matcher: ["/", "/donations/:path*", "/queue/:path*"]` — `/donate` is absent by omission; `/donations` and `/queue` (the `(app)` surfaces touched by this phase) remain matched. |
| T-06-19 | Elevation of Privilege | medium | mitigate | CLOSED | `donnarec-web/app/(public)/layout.tsx:26-44` — `PublicLayout` calls no `getServerSession`/role-check helper; renders `PublicHeader` + `{children}` unconditionally. |
| T-06-20 | Tampering | low | mitigate | CLOSED | `donnarec-web/app/(public)/public-theme.css:18` — `.theme-public { ... }` is a descendant-scoped class (never `:root`); `app/globals.css` `:root` and `tailwind.config.ts` `colors.primary/secondary` confirmed unmodified by this file. |
| T-06-SC (06-05) | Tampering (supply chain) | medium | mitigate | CLOSED | `donnarec-web/package.json:14` — `@marsidev/react-turnstile: "^1.5.3"`; `package-lock.json:1197-1200` — resolved/locked to exactly `1.5.3` with an `integrity` (sha512) hash, so `npm ci` reproducibly installs the audited version. |

### 06-06 — Public donation form + session-less passthrough + confirmation

| Threat ID | Category | Severity | Disposition | Status | Evidence |
|-----------|----------|----------|-------------|--------|----------|
| T-06-21 | Spoofing | high | mitigate | CLOSED | `donnarec-web/app/api/public/donations/route.ts:26-69` — no `getServerSession`/`getBffToken` import or call; `fetch(...)` sends no `Authorization` header (grep confirms zero matches for all three tokens in this file). |
| T-06-22 | Tampering | medium | mitigate | CLOSED | `components/PublicDonationForm.tsx:130-141` — client `canSubmit` gating is UX-only; server independently re-validates via `validator.Struct` (`public_handler.go:124-130`), magic-byte slip check (T-06-09), and server-side `siteverify` (T-06-06) — none of which trust any client-asserted flag. |
| T-06-23 | Repudiation | high | mitigate | CLOSED | `donnarec-web/messages/th.json:236` — confirmation body: "รายการนี้ยังไม่ใช่ใบเสร็จรับเงิน..."; `components/PublicDonationConfirmation.tsx` renders the reference number in IBM Plex Mono, visually distinct from the receipt-number presentation elsewhere. Live-verified: `PublicDonationForm.test.tsx` (4 subtests) PASS, including the in-page confirmation swap. |
| T-06-24 | Information Disclosure | low | mitigate | CLOSED | `app/api/public/donations/route.ts:5-6` — `API_BASE_URL` is read server-side only (`process.env.NEXT_PUBLIC_API_BASE_URL` is resolved in the Route Handler, never serialized into the response body or exposed to the browser via a client component); no CORS headers introduced. |
| T-06-SC (06-06) | Tampering (supply chain) | low | accept | CLOSED-accepted | Turnstile dep already added + verified in plan 05 (T-06-SC 06-05 above). Logged in Accepted Risks Log below. |

### 06-07 — Staff queue (source separation)

| Threat ID | Category | Severity | Disposition | Status | Evidence |
|-----------|----------|----------|-------------|--------|----------|
| T-06-25 | Elevation of Privilege | medium | mitigate | CLOSED | `app/api/bff/queue/route.ts:1-45` — uses `bffForward` (session-bound Bearer forwarding), never the session-less public passthrough; `cmd/server/main.go:407-408` — `donationGroup.Use(auth.RequireAnyRole(Maker,Checker,Admin))` sits on `api` (which itself requires `authMW.RequireAuth()`, `main.go:373`) — the Go side is the real authority, independently re-checked regardless of the BFF proxy. |
| T-06-26 | Information Disclosure | low | mitigate | CLOSED | `components/DonationDetailView.tsx:480-484` — `donation.source === "flow_b"` branch renders "ผู้บริจาคส่งเอง (ผ่านเว็บไซต์)" instead of the raw creator display name. |
| T-06-27 | Tampering | low | mitigate | CLOSED | `app/api/bff/queue/route.ts:30-37` — UI tokens map to `flow_a`/`flow_b` only (unrecognized values silently omit the filter, never forwarded raw); Go's `handler.go:540-545` independently re-validates with its own allow-list (T-06-04 evidence) — defense-in-depth, client cannot inject an arbitrary value into the SQL predicate. |
| T-06-SC (06-07) | Tampering (supply chain) | low | accept | CLOSED-accepted | No new package this plan. Logged in Accepted Risks Log below. |

### 06-08 — Mobile nav drawer + responsive/bilingual human walkthrough

| Threat ID | Category | Severity | Disposition | Status | Evidence |
|-----------|----------|----------|-------------|--------|----------|
| T-06-28 | Elevation of Privilege | low | mitigate | CLOSED | `components/AppShell.tsx:41-44,154` — `sidebarContent` (the SAME role-gated nav JSX, e.g. `isAdminViewer`/`isCheckerOrAdminViewer` guards) is rendered once and passed as `{sidebarContent}` into BOTH the desktop `<aside>` and `<MobileNavDrawer>` — no separate/weaker nav definition for the mobile variant. |
| T-06-29 | Denial of Service | medium | accept | CLOSED-accepted | Fail-closed verifier (T-06-06/T-06-07 evidence) rejects on egress failure rather than silently admitting submissions; the underlying hosting-egress-availability question is an explicit open stakeholder item (06-RESEARCH Pitfall 5), not silently decided. `06-UAT.md` (`status: complete`, updated 2026-07-17) records Test 2 "Real Cloudflare Turnstile challenge on a live stack" — **result: pass**, confirming the fail-closed behavior and the distinct CAPTCHA-error UX were both exercised, not merely asserted. Logged in Accepted Risks Log below. |
| T-06-SC (06-08) | Tampering (supply chain) | low | accept | CLOSED-accepted | No new package this plan. Logged in Accepted Risks Log below. |

## Accepted Risks Log

| Threat ID | Rationale | Recorded |
|-----------|-----------|----------|
| T-06-SC (06-01, 06-03, 06-04, 06-06, 06-07, 06-08) | Six of the eight plans in this phase add no new third-party package (the only two new deps — `golang.org/x/time` and `@marsidev/react-turnstile` — are separately verified as `mitigate`, see T-06-SC 06-02 / 06-05 above). Six-fold repeated low-severity "no-op" supply-chain entry, one per plan with no dependency change. | `06-01/03/04/06/07/08-PLAN.md` threat registers; confirmed by `go.mod`/`package.json` diffs matching each plan's `files_modified` list — no unexpected dependency additions found. |
| T-06-29 | A real Turnstile challenge requires outbound HTTPS egress from the Go backend to `challenges.cloudflare.com`. If the production hosting environment restricts egress, EVERY public submission fails closed (denial of service by design, not a silent security bypass) until egress is provisioned. This is an infrastructure/hosting decision explicitly deferred to a stakeholder, not a code defect — the code's own behavior (fail-closed) is the correct security posture regardless of how the egress question resolves. | `06-08-PLAN.md` threat register; `06-RESEARCH.md` Pitfall 5; confirmed exercised (not merely asserted) by `06-UAT.md` Test 2, result: pass, on 2026-07-17. |

## Unregistered Flags (new attack surface, no threat-model mapping)

Only `06-07-SUMMARY.md` and `06-08-SUMMARY.md` contain an explicit `## Threat Flags`
section (both state "None"); `06-01/02/03/04/05/06-SUMMARY.md` omit the section
entirely. Per the adversarial-stance requirement, this audit does **not** treat that
omission as "no new attack surface" and independently inspected the newly-introduced
unauthenticated seam for gaps outside the declared register. Two genuine, unmitigated,
and CAUSALLY LINKED findings surfaced (Flag 2 is what turns Flag 1 from a
burst-limited nuisance into a sustained, unbounded disk-exhaustion vector):

| # | Flag | Category | Severity (auditor-assigned) | Found in | Status |
|---|------|----------|-------------------------------|----------|--------|
| 1 | Unbounded multipart request body on the FIRST unauthenticated endpoint (`POST /api/public/donations`), and Go never cleans up the resulting disk temp files — **triggered by the CAPTCHA middleware itself, before any token is validated; no valid Turnstile solve required** | Denial of Service | **High** | `internal/captcha/middleware.go:38` (`c.PostForm(TokenField)`) → gin `initFormCache` → `ParseMultipartForm` | **RESOLVED — commit `72dca94`** |
| 2 | `gin.Engine.trustedProxies` left at its default `["0.0.0.0/0","::/0"]` — `c.ClientIP()` (the sole identity key for `ratelimit.PerIP`, T-06-05, AND the `remoteip` field sent to Cloudflare siteverify) honors an attacker-supplied `X-Forwarded-For`/`X-Real-IP` header on a DIRECT, unproxied connection | Elevation of Privilege / Denial of Service | **High** | `cmd/server/main.go` (no `SetTrustedProxies`/`TrustedPlatform`/`ForwardedByClientIP` call anywhere in `cmd/server` or `internal`) | **RESOLVED — commit `becedfd`** |

### Resolution (quick task 260717-spx, 2026-07-17)

Both High-severity flags were fixed before phase advancement (user chose "fix both"). Verified on the merged phase branch: `go build ./...` clean; `go test -run TestPublicDonationE2E ./cmd/server/` → **7/7 pass**, including the two new adversarial regression tests.

**Flag 2 — RESOLVED (`becedfd`).** Added a config-driven trusted-proxy allowlist. `internal/config/config.go:138` (`TrustedProxies []string`), loaded from env `TRUSTED_PROXIES` at `config.go:215` (comma-split, trimmed, empty→nil). `cmd/server/main.go:345` calls `router.SetTrustedProxies(trustedProxies)` right after `gin.New()` (fatal on error). Default nil = trust no proxy → `c.ClientIP()` returns `RemoteAddr`, ignoring attacker `X-Forwarded-For`; operator sets `TRUSTED_PROXIES=<LB CIDR>` in prod. New E2E test `TestPublicDonationE2E/SpoofedXFF_SharesRemoteAddrBucket_NotBypassed` asserts a rotating spoofed XFF no longer moves the per-IP bucket (burst+1 → 429).

**Flag 1 — RESOLVED (`72dca94`).** Added `bodyLimitMiddleware` (`cmd/server/main.go:514`) registered on `publicGroup` at `main.go:380` — **after `PerIP`, before `VerifyTurnstile`** so it bounds the body before captcha's `PostForm` triggers the parse. It wraps the body in `http.MaxBytesReader(c.Writer, c.Request.Body, publicBodyLimitBytes)` (`main.go:500`, `11<<20` ≈ 11 MB) and `defer`s `c.Request.MultipartForm.RemoveAll()` around `c.Next()`. Production 400 `{"error":"captcha_failed"}` response shape unchanged. New E2E test `TestPublicDonationE2E/OversizedBody_Rejected_NoRow_NoHang` asserts an oversized body is bounded-rejected (4xx), creates no donation row, and returns promptly (the test harness's fake captcha verifier always passes, so the assertion targets the DoS property that holds under it, not `captcha_failed`).

Both flags are now closed with threat-model-grade evidence + code + tests, per the audit's ship-gate requirement (fix, not silent deferral).

**Flag 1 — evidence.** `go doc mime/multipart.Reader.ReadForm`: "File parts which
can't be stored in memory will be stored on disk in temporary files" — no upper bound
on total body size. Gin's `defaultMultipartMemory = 32 << 20` (32 MB, `gin.go:26,221`)
is the in-memory threshold only; `c.FormFile()` (`context.go:698-710`) calls
`ParseMultipartForm(32 MB)` and returns without ever calling
`r.MultipartForm.RemoveAll()` — **confirmed by `grep -rn "RemoveAll" cmd/server
internal/donation internal/storage` = zero matches** anywhere in this codebase. Any
body over 32 MB is therefore written to an OS temp file that is NEVER explicitly
removed (Go's `net/http` server does not auto-remove multipart temp files either) —
this is a genuinely **cumulative** disk-fill vector, not a transient per-request spike.
The app's own 10 MB check (`storage.validateSlip`, `internal/storage/client.go:112-131`)
runs on the reader AFTER `ParseMultipartForm` has already fully consumed and
disk-buffered the oversized body — too late to prevent the write it is meant to bound.
No `http.MaxBytesReader`, no `MaxMultipartMemory` override, no reverse-proxy
`client_max_body_size` exists anywhere in `cmd/server/main.go` or
`donnarec-api/docker-compose*.yml`. `http.Server{ReadTimeout: 15s}` (`main.go:290`)
bounds time, not bytes — a multi-GB body is achievable well within 15s on any
reasonable connection.

**This does NOT require a valid CAPTCHA solve to trigger — verified via the gin
source, not assumed.** Middleware order is `ratelimit.PerIP` → `captcha.VerifyTurnstile`
→ handler (`cmd/server/main.go:367-369`). `VerifyTurnstile()`
(`internal/captcha/middleware.go:38`) reads the token with `token :=
c.PostForm(TokenField)` — and `Context.PostForm` → `GetPostForm` →
`GetPostFormArray` → `initFormCache` (`context.go:601-604,624-629,638-649`), where
`initFormCache` is what actually calls `req.ParseMultipartForm(MaxMultipartMemory)`.
So the ENTIRE multipart body — including an oversized "slip" part — is parsed and
(if >32 MB) disk-spilled **during the CAPTCHA middleware's own token read, before
Cloudflare's siteverify is ever called and before the token's validity is known.** An
attacker can submit a garbage/empty `turnstile_token` alongside a multi-GB file part;
the disk write happens regardless of whether the request is subsequently rejected
with `captcha_failed`. The only gate standing between an anonymous request and this
disk write is the rate limiter — which Flag 2 shows is bypassable.

**Flag 2 — evidence.** `gin@v1.12.0/gin.go:225` — `trustedProxies:
[]string{"0.0.0.0/0", "::/0"}` is the compiled-in default (trust EVERY remote address
as a legitimate proxy) unless `Engine.SetTrustedProxies(...)` is called to narrow it;
`gin.go:214-215` — `ForwardedByClientIP: true` and `RemoteIPHeaders:
["X-Forwarded-For", "X-Real-IP"]` are also both defaulted on. `context.go:975-1015`
(`Context.ClientIP()`) — with the default trust-all config, `isTrustedProxy(remoteIP)`
returns `true` for literally any connecting IP, so the method reads and returns the
attacker-supplied `X-Forwarded-For` header verbatim. **Confirmed: `grep -rn
"SetTrustedProxies|TrustedPlatform|RemoteIPHeaders|ForwardedByClientIP" cmd/server
internal` = zero matches** — this codebase never overrides the default. Practical
effect: `ratelimit.PerIP` (`internal/ratelimit/middleware.go:100-112`) keys its
token-bucket map on `c.ClientIP()` — an attacker who varies the `X-Forwarded-For`
header value on every request gets a fresh, never-throttled bucket each time, fully
bypassing the per-IP 429. Because `ratelimit.PerIP` runs BEFORE `captcha.VerifyTurnstile`
in the middleware chain, and because Flag 1 shows the disk-spilling body parse happens
INSIDE `VerifyTurnstile` regardless of token validity, this bypass is **not** merely
"CAPTCHA remains a second layer, so the practical risk is bounded" — CAPTCHA gates
whether the request is ultimately *accepted*, not whether the oversized body is
*parsed and written to disk*. Combined, Flags 1+2 describe an attacker who (a) rotates
`X-Forwarded-For` to defeat the only per-IP throttle, then (b) sends an arbitrarily
large multipart body with a garbage CAPTCHA token — achieving an **unauthenticated,
effectively unthrottled, cumulative disk-exhaustion DoS that requires no valid
Turnstile solve and no credentials of any kind.**

**Why neither is in the declared register:** T-06-05 (DoS, mitigate) covers request-
*volume* flooding and its own mechanism is correctly implemented exactly as declared;
neither T-06-05 nor T-06-09 (file-type spoofing) anticipated (a) a single-request,
oversized-*body* disk-exhaustion vector, or (b) the identity key the rate limiter
relies on being attacker-controllable by default in this web framework. This is
exactly the class of gap the adversarial stance instructs auditors not to wave through
by treating "a check exists somewhere nearby" as sufficient — the existing controls
are real but (1) run too late in the request lifecycle (Flag 1) and (2) rest on an
identity assumption not actually enforced by the deployment (Flag 2).

**Recommended fix (not applied — implementation is read-only under this audit's
mandate):**
- Flag 1: wrap the public route's request body in `http.MaxBytesReader(w, r.Body,
  11<<20)` (slightly above the 10 MB slip cap) before any multipart parsing occurs,
  and/or call `r.MultipartForm.RemoveAll()` in a `defer` after every parse.
- Flag 2: call `router.SetTrustedProxies([...])` with the actual reverse-proxy/LB CIDR
  (or `SetTrustedProxies(nil)` + accept `RemoteIP()` only, if the Go process is
  directly internet-facing with no fronting proxy) so `c.ClientIP()` reflects a value
  the deployment topology actually guarantees, not an attacker-supplied header.

Neither flag counts toward `threats_open` under this audit's formal gate (per
protocol, only DECLARED-register threats are gate-counted) — **but per this task's
explicit instruction to escalate obvious high-severity gaps in the public attack
surface, this audit's top-line verdict is ESCALATE, not SECURED, on account of these
two findings** (see Gaps Summary / verdict below). Both require a near-term fix (new
threat-model entries + code changes), not silent deferral. Priority: **fix Flag 2
first** — it is a single-line config change (`SetTrustedProxies`) that simultaneously
(a) restores `ratelimit.PerIP`'s real-world effectiveness and (b) closes the only
remaining gate in front of Flag 1's unauthenticated disk-write path, since Flag 1's
exploitation is only bounded by the (currently bypassable) rate limiter.

## Gaps Summary

**No blocking gaps against the declared threat model.** All 30 `mitigate` threats and
7 `accept` threats across `06-01` through `06-08` resolved to CLOSED against the
current implementation, independently verified via direct code inspection (not
SUMMARY.md claims) and live re-execution of every unit/integration/E2E test this audit
could reach (all passed, including the `-race`-enabled `TestPublicDonationE2E` E2E
gate required by CLAUDE.md's Conventions integration-test rule). `threats_open = 0` at
`block_on: high` — **the declared register is fully closed.** Note also that the
declared register's own test evidence for T-06-05
(`TestPublicDonationE2E/RateLimit_ExceedingThreshold_429`) drives requests from a
single fixed test-client IP and never rotates `X-Forwarded-For` — a passing test there
demonstrates the token-bucket mechanism works, not that it is effective against a
real, header-spoofing attacker (that gap is exactly Flag 2).

Two out-of-register **High**-severity, causally-compounding findings were located via
independent inspection of the newly-unauthenticated seam and direct reading of the
`gin-gonic/gin@v1.12.0` stdlib/framework source (no `06-REVIEW.md` code-review
artifact existed for this phase to cross-reference, unlike phase 05):

1. **Flag 1** — the multipart body of `POST /api/public/donations` is fully parsed,
   and large parts (>32 MB) are written to disk temp files that are never cleaned up
   — and this parse/write happens **inside the CAPTCHA middleware's own token read,
   before the token is validated**, so no valid Turnstile solve is needed to trigger
   it.
2. **Flag 2** — `c.ClientIP()`, the sole identity key for the per-IP rate limiter (and
   the value sent to Cloudflare as `remoteip`), is attacker-controlled via a spoofed
   `X-Forwarded-For` header, because this codebase never calls
   `Engine.SetTrustedProxies` and gin defaults to trusting every remote address as a
   legitimate proxy.

Together these describe an **unauthenticated, effectively unthrottled, cumulative
disk-exhaustion DoS requiring no CAPTCHA solve and no credentials** — on the newest
and widest attack surface in a hospital PDPA system. Per this task's own instruction
to escalate exactly this class of finding, this audit does not stamp a clean SECURED
verdict over it.

**Minor, non-blocking observation (not a registered threat, not expanded further):**
`PublicDonationRequest.ConsentGiven` (`internal/donation/model.go:50`) has no
`validate:"required"`/`eq=true` tag — the Go validator does not itself reject
`consent_given=false`, so this specific field's "server re-validates" claim under
T-06-22 is narrower than the other fields in that same struct. A human Checker reviews
every submission before any receipt is issued, which bounds the practical impact; not
raised as a standalone flag.

**Verdict: Phase 06's DECLARED threat register is fully SECURED (37/37 CLOSED,
`threats_open=0`). This audit nonetheless returns ESCALATE at the top level** because
two verified, OPEN, High-severity, causally-compounding gaps exist on the phase's
unauthenticated public surface outside the declared register, and this task's mandate
explicitly directs escalating such findings rather than allowing them to pass silently
as non-blocking WARNINGs. A human ship decision is needed: fix Flag 2 (and ideally
Flag 1) before shipping this phase's public endpoint to the open internet, or
consciously accept the risk and log it in the Accepted Risks Log above with a named
owner and timeline.
