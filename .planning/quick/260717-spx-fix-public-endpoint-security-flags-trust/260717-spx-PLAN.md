---
phase: quick-260717-spx
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - donnarec-api/internal/config/config.go
  - donnarec-api/cmd/server/main.go
  - donnarec-api/cmd/server/e2e_test.go
  - donnarec-api/cmd/server/e2e_public_test.go
  - donnarec-api/.env.example
autonomous: true
requirements: [SEC-06-FLAG1, SEC-06-FLAG2]
must_haves:
  truths:
    - "An attacker-supplied X-Forwarded-For header on a direct connection does NOT move the per-IP rate-limit bucket (spoofed XFF is ignored; RemoteAddr governs)."
    - "An oversized multipart POST to /api/public/donations is rejected (4xx, no donation row) without spilling unbounded temp files to disk, and the temp files that do form are removed. In production the reject is 400 captcha_failed; the E2E harness (always-pass fake captcha) reaches the handler's slip rejection instead — both are bounded, non-hanging 4xx."
    - "Existing public E2E suite still passes: nil TrustedProxies default keeps RemoteAddr-driven IP resolution, and the accepted captcha_failed shape is unchanged."
  artifacts:
    - "donnarec-api/internal/config/config.go — Config.TrustedProxies field loaded from TRUSTED_PROXIES"
    - "donnarec-api/cmd/server/main.go — router.SetTrustedProxies(...) + body-limit middleware on publicGroup"
    - "donnarec-api/cmd/server/e2e_public_test.go — two new E2E sub-tests (XFF-spoof, oversized-body)"
  key_links:
    - "config.TrustedProxies → setupRouter → router.SetTrustedProxies → gin c.ClientIP() → ratelimit.PerIP bucket key"
    - "publicGroup body-limit middleware runs BEFORE captcha.VerifyTurnstile (parse happens inside captcha via c.PostForm)"
---

<objective>
Fix two confirmed HIGH-severity security findings on the Phase 06 public donation endpoint (`POST /api/public/donations`), both verified against real code in `.planning/phases/06-public-donation-web-form-flow-b/06-SECURITY.md`:

- Flag 2 (Task 1): rate-limit IP-spoof bypass — gin trusts all proxies by default, so a direct-connection attacker's `X-Forwarded-For` defeats `ratelimit.PerIP`.
- Flag 1 (Task 2): unauthenticated multipart disk-exhaustion DoS — gin parses the multipart body inside `captcha.VerifyTurnstile`'s `c.PostForm` call before any valid CAPTCHA is required, and large parts spill to unbounded, uncleaned disk temp files.

Purpose: close both public-surface DoS/bypass vectors before the phase is secured.
Output: a directly-exposed service that ignores spoofed proxy headers and bounds+cleans unauthenticated request bodies.
Scope: work ONLY in `donnarec-api` (Go). Two atomic commits, Task 1 first.
</objective>

<execution_context>
@$HOME/.claude/gsd-core/workflows/execute-plan.md
</execution_context>

<context>
@.planning/STATE.md
@./CLAUDE.md
@.planning/phases/06-public-donation-web-form-flow-b/06-SECURITY.md
@donnarec-api/internal/config/config.go
@donnarec-api/internal/captcha/middleware.go
@donnarec-api/internal/ratelimit/middleware.go
@donnarec-api/cmd/server/main.go
@donnarec-api/cmd/server/e2e_test.go
@donnarec-api/cmd/server/e2e_public_test.go

Confirmed facts (already read):
- `setupRouter` has TWO callers: `cmd/server/main.go:281` (prod) and `cmd/server/e2e_test.go:270` (harness). Any signature change must update both.
- `router := gin.New()` is at `cmd/server/main.go:337`; `logger *zap.Logger` is already a `setupRouter` param.
- `publicGroup` is built at `main.go:366-369`: `PerIP` (367) then `VerifyTurnstile` (368) then `POST /donations` (369).
- `captcha.VerifyTurnstile` (`internal/captcha/middleware.go:38`) calls `c.PostForm(TokenField)` — THIS triggers the multipart parse; it runs before the token is validated.
- The public handler already enforces a 10 MB per-slip cap via `storage.PutSlip` (`internal/donation/public_handler.go:156-160`, `ErrFileTooLarge`). An 11<<20 body cap comfortably exceeds one legitimate slip + form fields → consistent with (>=) that limit.
- `.env.example` exists in `donnarec-api/`.
- E2E harness config: `e2ePublicRateBurst = 5` (`e2e_test.go:126`); `doPublicSubmission(t, fields, slipFilename, slipBytes, remoteAddr)` sets `req.RemoteAddr` (NOT XFF) at `e2e_public_test.go:69-71`.
</context>

<tasks>

<task type="auto">
  <name>Task 1: Set trusted proxies so spoofed X-Forwarded-For cannot bypass per-IP rate limiting (Flag 2)</name>
  <files>donnarec-api/internal/config/config.go, donnarec-api/cmd/server/main.go, donnarec-api/cmd/server/e2e_test.go, donnarec-api/cmd/server/e2e_public_test.go, donnarec-api/.env.example</files>
  <action>
Root cause: donnarec-api never calls gin's SetTrustedProxies, so gin trusts all proxies (default 0.0.0.0/0) and c.ClientIP() honors an attacker-supplied X-Forwarded-For on a direct connection, defeating ratelimit.PerIP.

1. config.go: add a `TrustedProxies []string` field to the `Config` struct (place near the RateLimit field, documented as the CIDRs/IPs of trusted upstream proxies/load balancers; nil = trust no proxy = c.ClientIP() returns the direct RemoteAddr). In `Load`, read env var `TRUSTED_PROXIES` (comma-separated CIDRs/IPs): split on comma, `strings.TrimSpace` each element, drop empty elements; if the env var is unset/empty, leave the slice nil (do NOT default to a non-empty list). Assign into the Config literal. Do not add it to `validate()` required — it is optional with a safe nil default.

2. main.go: add a `trustedProxies []string` parameter to `setupRouter` (append to the existing signature at line 336). Immediately after `router := gin.New()` (line 337), call `router.SetTrustedProxies(trustedProxies)` and handle the returned error — on error, `logger.Fatal(...)` (logger is already in scope). Passing nil/empty to SetTrustedProxies(nil) is the intended safe default: it trusts no proxy, so c.ClientIP() returns RemoteAddr and ignores XFF. Thread the real value at the prod call site (line 281): pass `cfg.TrustedProxies` as the new final argument.

3. e2e_test.go: update the harness `setupRouter(...)` call (line 270) to pass `nil` for the new trustedProxies argument. nil default keeps the harness's existing RemoteAddr-based IP resolution working, so all current sub-tests still pass.

4. .env.example: add a `TRUSTED_PROXIES` entry with a short comment documenting it as comma-separated CIDRs/IPs of trusted upstream proxies (empty = trust none = safe default for a directly-exposed service; set to the load-balancer CIDR in prod). Leave the value empty.

5. e2e_public_test.go: add a new sub-test to `TestPublicDonationE2E` proving the spoof is closed. Use a dedicated RemoteAddr distinct from the other sub-tests' IPs to get a fresh token bucket. Extend/parameterize the multipart POST so the request can carry a caller-supplied `X-Forwarded-For` header (add an optional header arg or a small variant helper — do not break existing `doPublicSubmission` callers). Send `e2ePublicRateBurst` requests that share ONE RemoteAddr but each carry a DIFFERENT spoofed X-Forwarded-For value; assert none returns 429 (they share the RemoteAddr bucket). Then send one more request (same RemoteAddr, another distinct spoofed XFF) and assert it IS 429 rate_limited — proving the spoofed header did not create per-header buckets and RemoteAddr governs. Keep the slip nil (400 slip_required still consumes a rate token since PerIP runs first).
  </action>
  <verify>
    <automated>cd donnarec-api && rtk go build ./... && rtk go test ./... && rtk go test -race -run TestPublicDonationE2E ./cmd/server/</automated>
  </verify>
  <done>
Config exposes TrustedProxies loaded from TRUSTED_PROXIES (nil when unset). setupRouter calls router.SetTrustedProxies with the threaded value and fatals on error; both callers (main.go, e2e_test.go) compile. New E2E sub-test proves requests sharing one RemoteAddr but varying X-Forwarded-For share a single rate-limit bucket (spoofed XFF ignored) and the burst+1 request returns 429 rate_limited. `go build ./...` and `go test ./...` pass; existing public E2E sub-tests still green.
  </done>
</task>

<task type="auto">
  <name>Task 2: Bound + clean unauthenticated multipart body to stop disk-exhaustion DoS (Flag 1)</name>
  <files>donnarec-api/cmd/server/main.go, donnarec-api/cmd/server/e2e_public_test.go</files>
  <action>
Root cause: gin parses the multipart body inside captcha.VerifyTurnstile's own c.PostForm(TokenField) call BEFORE the token is validated, so no valid CAPTCHA is required to force a parse; parts larger than gin's 32MB memory threshold spill to disk temp files that are never cleaned. There is no MaxBytesReader/RemoveAll anywhere in cmd/server or the donation storage path.

1. main.go: add a small body-limit gin middleware (a local `gin.HandlerFunc`-returning helper is fine — keep it in cmd/server/main.go near setupRouter). It must:
   - set `c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 11<<20)` at the top (11<<20 ≈ 11 MB — comfortably exceeds the handler's existing 10 MB slip cap so legitimate submissions are unaffected, while bounding abuse),
   - register a `defer` (wrapping `c.Next()`) that, after the handler chain runs, calls `c.Request.MultipartForm.RemoveAll()` when `c.Request.MultipartForm != nil` — removing any temp files the parse created.
   Register this middleware on `publicGroup` (main.go ~line 366-368) AFTER `ratelimit.PerIP` (keep the cheapest rejection first) and BEFORE `captchaMW.VerifyTurnstile()`, because the parse happens inside captcha. Do NOT alter the PerIP or VerifyTurnstile registrations otherwise.

   Do NOT change the existing 400 `{"error":"captcha_failed"}` response shape and do NOT add a distinct 413 path. In PRODUCTION, an oversized body → ParseMultipartForm errors → c.PostForm returns "" → the REAL Turnstile verifier rejects the empty token with 400 captcha_failed. That is the accepted, intended behavior (DoS stopped; temp files bounded by MaxBytesReader and removed by the defer).

   IMPORTANT harness divergence (confirmed): the E2E harness wires `fakeCaptchaVerifier` (`cmd/server/e2e_test.go:134-136`) whose `Verify` ALWAYS returns nil (passes) regardless of token. So in-harness an oversized body → parse fails → empty token → fake verifier PASSES → the handler runs and rejects on the missing/unreadable slip (NOT captcha_failed). Therefore the in-harness test must NOT assert `captcha_failed`.

2. e2e_public_test.go: add a new sub-test to `TestPublicDonationE2E` that POSTs an oversized multipart body (a slip part larger than the 11 MB cap, e.g. a valid PNG magic-byte prefix padded past 11<<20) to /api/public/donations. Use a fresh dedicated RemoteAddr so the rate limiter does not interfere. Assert the DoS property that actually holds under the fake verifier (mirror the existing `MissingSlip_Rejected_NoRow` / `BadMagicByteSlip_Rejected_NoRow` sub-tests): (a) the response is a 4xx rejection (recorder returns, code >= 400 — do NOT pin `captcha_failed`), (b) NO donation row is created (count-unchanged check before/after), and (c) the request completes promptly (does not hang). Add a comment noting that production yields 400 captcha_failed via the real Turnstile verifier and the harness diverges only because CAPTCHA is stubbed to always-pass. This still exercises the real HTTP path: PerIP → body-limit → captcha parse over the production setupRouter (Conventions integration-test gate for the public request seam) — the body-limit + RemoveAll fix is what makes the oversized request bounded rather than an unbounded disk spill.
  </action>
  <verify>
    <automated>cd donnarec-api && rtk go build ./... && rtk go test ./... && rtk go test -race -run TestPublicDonationE2E ./cmd/server/</automated>
  </verify>
  <done>
publicGroup registers a body-limit middleware (MaxBytesReader 11<<20 + deferred MultipartForm.RemoveAll) AFTER PerIP and BEFORE VerifyTurnstile. In production an oversized multipart POST is rejected 400 captcha_failed (unchanged shape) via the real verifier; the fix bounds the disk spill and cleans temp files regardless. New E2E sub-test asserts the property that holds under the always-pass fake verifier: oversized body → 4xx rejection, NO donation row created, request returns promptly (does not hang), over the real router. `go build ./...` and `go test ./...` pass; all prior public E2E sub-tests still green.
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| Internet → public route group | Unauthenticated donor traffic crosses here into `/api/public/donations` — the only unauthenticated write surface. Both attacker-controlled headers (XFF) and attacker-controlled body size arrive untrusted. |

## STRIDE Threat Register

| Threat ID | Category | Component | Severity | Disposition | Mitigation Plan |
|-----------|----------|-----------|----------|-------------|-----------------|
| T-06-SPX-01 | Spoofing / DoS | `ratelimit.PerIP` via gin `c.ClientIP()` | high | mitigate | Task 1: `router.SetTrustedProxies(cfg.TrustedProxies)` (nil default) so a direct-connection attacker's `X-Forwarded-For` cannot create per-header buckets; RemoteAddr governs the bucket. E2E test asserts spoofed XFF shares one bucket. |
| T-06-SPX-02 | Denial of Service | multipart parse inside `captcha.VerifyTurnstile` (`cmd/server`, no MaxBytesReader/RemoveAll) | high | mitigate | Task 2: body-limit middleware (`http.MaxBytesReader` 11<<20 + deferred `MultipartForm.RemoveAll`) on `publicGroup` before captcha parse — bounds disk spill and cleans temp files; oversized body → 400 captcha_failed. E2E test asserts bounded rejection. |
| T-06-SPX-SC | Tampering | package installs | low | accept | No new third-party dependencies added — both fixes use gin/stdlib (`SetTrustedProxies`, `net/http.MaxBytesReader`) already in the module. |
</threat_model>

<verification>
- `cd donnarec-api && rtk go build ./...` — compiles with the new setupRouter signature threaded through both callers.
- `cd donnarec-api && rtk go test ./...` — full suite green, including the two new E2E sub-tests.
- `cd donnarec-api && rtk go test -race -run TestPublicDonationE2E ./cmd/server/` — public seam sub-tests (happy path, bad magic byte, missing slip, existing rate limit, new XFF-spoof, new oversized-body) all pass under the race detector over the real HTTP path (Conventions integration-test gate).
</verification>

<success_criteria>
- Spoofed `X-Forwarded-For` on a direct connection does not move the per-IP rate-limit bucket (proven by E2E sub-test).
- Oversized unauthenticated multipart body is rejected 400 `captcha_failed` without unbounded/uncleaned temp files and without hanging (proven by E2E sub-test).
- Existing behavior and response shapes unchanged; nil `TrustedProxies` default keeps the current suite passing.
- Two atomic commits, Task 1 (Flag 2) committed before Task 2 (Flag 1).
</success_criteria>

<output>
Create `.planning/quick/260717-spx-fix-public-endpoint-security-flags-trust/260717-spx-SUMMARY.md` when done.
</output>
