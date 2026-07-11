---
phase: 06-public-donation-web-form-flow-b
plan: 02
subsystem: api
tags: [go, gin, captcha, turnstile, rate-limiting, golang.org/x/time, anti-automation]

# Dependency graph
requires:
  - phase: 06-public-donation-web-form-flow-b
    provides: "06-01 backend data foundation (donations.source column, public-web system user)"
provides:
  - "internal/captcha.Verifier interface + ErrCaptchaFailed sentinel + TurnstileVerifier (fail-closed Cloudflare siteverify)"
  - "internal/captcha.Middleware.VerifyTurnstile() gin.HandlerFunc reading multipart field turnstile_token"
  - "internal/ratelimit.IPRateLimiter + PerIP(r, b) gin.HandlerFunc (per-IP token bucket, 429 on exhaustion)"
  - "config.Config.TurnstileSecretKey (env-only TURNSTILE_SECRET_KEY) + config.Config.RateLimit (SubmissionsPerWindow/Window, 5-per-10-minute default)"
affects: [06-03-public-donation-handler-and-routing, 06-public-donation-web-form-flow-b]

# Tech tracking
tech-stack:
  added: ["golang.org/x/time@v0.15.0 (rate.Limiter, promoted to direct dependency)"]
  patterns:
    - "Narrow-interface anti-automation seam (Verifier) mirroring mailer.EmailSender/worker.PDFRenderer"
    - "Fail-closed CAPTCHA verification: every error path (empty token, network/timeout, decode, provider success=false) returns ErrCaptchaFailed, never nil"
    - "Per-IP in-memory token-bucket rate limiter (sync.Mutex visitor map + background eviction goroutine), no DB write on the request hot path"
    - "Test-seam via a second exported constructor (NewTurnstileVerifierWithClient) instead of unexported-field access, preserving the codebase's black-box `_test` package convention"

key-files:
  created:
    - donnarec-api/internal/captcha/verifier.go
    - donnarec-api/internal/captcha/turnstile.go
    - donnarec-api/internal/captcha/turnstile_test.go
    - donnarec-api/internal/captcha/middleware.go
    - donnarec-api/internal/ratelimit/middleware.go
    - donnarec-api/internal/ratelimit/middleware_test.go
  modified:
    - donnarec-api/internal/config/config.go
    - donnarec-api/go.mod
    - donnarec-api/go.sum

key-decisions:
  - "captcha.Middleware is a struct wrapping Verifier with a VerifyTurnstile() method (not a bare function taking Verifier) — matches 06-PATTERNS.md's documented captchaMW.VerifyTurnstile() wiring call site for plan 03"
  - "TurnstileVerifier exposes two constructors: NewTurnstileVerifier(secretKey) for production (real Cloudflare URL + 5s-timeout client) and NewTurnstileVerifierWithClient(secretKey, verifyURL, client) as the test seam — avoids breaking the codebase's established black-box `_test` package convention while keeping fields unexported"
  - "TURNSTILE_SECRET_KEY is read into config but deliberately NOT added to Config.validate()'s required-env map — wiring/enforcement is plan 03's scope (route registration); this plan only builds the primitives"
  - "Multipart field name for the CAPTCHA token is turnstile_token (internal/captcha.TokenField constant) — the exact contract plan 03's handler and the frontend TurnstileWidget must agree on"

patterns-established:
  - "Anti-automation middleware ordering contract for plan 03: ratelimit.PerIP registered BEFORE captcha.Middleware.VerifyTurnstile() on the public route group — cheap IP-bucket rejection happens before any outbound siteverify network call"

requirements-completed: [FR-04]

coverage:
  - id: D1
    description: "TurnstileVerifier fails closed on every error path (empty token, network/timeout, transport-closed, decode error) and passes only on a real success=true from siteverify"
    requirement: "FR-04"
    verification:
      - kind: unit
        ref: "donnarec-api/internal/captcha/turnstile_test.go#TestTurnstileVerifier_Verify"
        status: pass
    human_judgment: false
  - id: D2
    description: "Per-IP token-bucket rate limiter returns 429 after N=2 burst is exhausted and isolates buckets by client IP"
    requirement: "FR-04"
    verification:
      - kind: unit
        ref: "donnarec-api/internal/ratelimit/middleware_test.go#TestPerIP_TokenBucket"
        status: pass
    human_judgment: false
  - id: D3
    description: "Turnstile secret is read from TURNSTILE_SECRET_KEY env var only, never DB-stored; rate-limit count/window are env-configurable with a conservative default"
    requirement: "FR-04"
    verification:
      - kind: unit
        ref: "go build ./... (donnarec-api/internal/config/config.go compiles + Load() wires both new env vars)"
        status: pass
    human_judgment: false

duration: 8min
completed: 2026-07-11
status: complete
---

# Phase 6 Plan 2: Anti-automation primitives (Turnstile + per-IP rate limiter) Summary

**Fail-closed Cloudflare Turnstile verifier + in-memory per-IP token-bucket rate limiter as the two gin middlewares that will replace RequireAuth on the public donation route group**

## Performance

- **Duration:** 8 min
- **Started:** 2026-07-11T13:37:58+07:00
- **Completed:** 2026-07-11T13:44:44+07:00
- **Tasks:** 1 (single TDD feature, RED -> GREEN)
- **Files modified:** 9

## Accomplishments
- `internal/captcha.Verifier` narrow interface + `ErrCaptchaFailed` sentinel + `TurnstileVerifier` implementation that POSTs form-encoded `secret`/`response`/`remoteip` to Cloudflare's `siteverify` and fails closed on every non-success path (empty token, transport/timeout error, decode error, `success:false`) — proven against a fake `httptest.Server` standing in for `challenges.cloudflare.com`
- `internal/captcha.Middleware.VerifyTurnstile()` gin middleware reading the `turnstile_token` multipart field, calling `Verify` with `c.ClientIP()`, and aborting `400 {"error":"captcha_failed"}` on failure — deliberately a distinct shape from the `422` field-validation error, and the token is never forwarded into a domain request struct
- `internal/ratelimit.IPRateLimiter` + `PerIP(r, b)` gin middleware: one `golang.org/x/time/rate.Limiter` per client IP behind a mutex-guarded map, background goroutine evicting visitors unseen >10min, `429 {"error":"rate_limited"}` on bucket exhaustion, isolated per IP
- `config.Config` gained `TurnstileSecretKey` (env `TURNSTILE_SECRET_KEY`, never DB-stored) and `RateLimit RateLimitConfig{SubmissionsPerWindow, Window}` (env `RATE_LIMIT_SUBMISSIONS_PER_WINDOW`/`RATE_LIMIT_WINDOW`, default 5 per 10 minutes)
- `golang.org/x/time@v0.15.0` promoted from an indirect transitive dependency to a direct one

## Task Commits

Single `type: tdd` feature plan — RED then GREEN (no separate REFACTOR commit needed; the cleanup goroutine was already extracted as its own method, `cleanupVisitors`, from first write):

1. **RED — failing tests** - `a081c01` (test)
2. **GREEN — implementation** - `f6b6cfb` (feat)

**Plan metadata:** (this commit, following this SUMMARY)

## Files Created/Modified
- `donnarec-api/internal/captcha/verifier.go` - `Verifier` interface + `ErrCaptchaFailed` sentinel
- `donnarec-api/internal/captcha/turnstile.go` - `TurnstileVerifier`, `NewTurnstileVerifier` (production), `NewTurnstileVerifierWithClient` (test seam)
- `donnarec-api/internal/captcha/turnstile_test.go` - 5 fake-siteverify-server test cases (empty/success/failure/timeout/closed-server)
- `donnarec-api/internal/captcha/middleware.go` - `Middleware`, `NewMiddleware`, `VerifyTurnstile()`, `TokenField = "turnstile_token"`
- `donnarec-api/internal/ratelimit/middleware.go` - `IPRateLimiter`, `NewIPRateLimiter`, `cleanupVisitors`, `PerIP(r, b)`
- `donnarec-api/internal/ratelimit/middleware_test.go` - burst=2 allow/deny + per-IP isolation tests
- `donnarec-api/internal/config/config.go` - `RateLimitConfig` type, `Config.TurnstileSecretKey`, `Config.RateLimit`, both wired in `Load()`
- `donnarec-api/go.mod` / `go.sum` - `golang.org/x/time@v0.15.0` direct dependency

## Decisions Made
- `Middleware` struct + `VerifyTurnstile()` method (not a bare function) — matches 06-PATTERNS.md's `captchaMW.VerifyTurnstile()` wiring call site plan 03 will use
- Dual constructors on `TurnstileVerifier` (`NewTurnstileVerifier` for prod, `NewTurnstileVerifierWithClient` for tests) instead of exporting the `verifyURL`/`client` fields — keeps DI private (matches `DonationService`'s constructor-injection convention) while still letting the black-box `captcha_test` package point at a fake server
- `TURNSTILE_SECRET_KEY` is loaded into `Config` but NOT added to `validate()`'s required-env map in this plan — enforcing it as hard-required at boot is plan 03's call once the route is actually wired (a `TurnstileVerifier` with an empty secret still fails closed against the real Cloudflare API, it just never succeeds)
- Rate-limit test uses `rate.Limit(0.0001)` (near-zero sustained refill) with `burst=2` instead of sleeping — makes the "N=2 over a long window" behavior deterministic without a real-time wait

## Deviations from Plan

None - plan executed exactly as written. The RESEARCH.md code sketch's inline `Verify` example used unexported fields with no test-injection path; this was extended (dual constructor) purely to fit the codebase's existing black-box test-package convention, not a functional deviation.

## Issues Encountered
None.

## User Setup Required

None - no external service configuration required by this plan. `TURNSTILE_SECRET_KEY` will need to be set in the environment once plan 03/07 wires the real route and the Cloudflare Turnstile site key is provisioned (stakeholder/ops item, out of scope here).

## Next Phase Readiness
- `internal/captcha` and `internal/ratelimit` are both fully self-contained, unit-tested, and ready for plan 03 to wire onto the new `/api/public` gin route group in the exact order documented in 06-PATTERNS.md: `publicGroup.Use(ratelimit.PerIP(...))` then `publicGroup.Use(captchaMW.VerifyTurnstile())`
- No blockers. The one open item (production `TURNSTILE_SECRET_KEY` value / Cloudflare site registration) is a stakeholder/ops task, not a code blocker — the fail-closed design means an unset key simply rejects all submissions rather than silently accepting them.

---
*Phase: 06-public-donation-web-form-flow-b*
*Completed: 2026-07-11*
