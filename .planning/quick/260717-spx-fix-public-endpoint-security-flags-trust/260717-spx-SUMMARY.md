---
phase: quick-260717-spx
plan: 01
subsystem: api
tags: [gin, security, rate-limit, captcha, multipart, dos]

requires:
  - phase: 06-public-donation-web-form-flow-b
    provides: "publicGroup route (ratelimit.PerIP -> captcha.VerifyTurnstile -> handler) and 06-SECURITY.md's two escalated, unmitigated High findings (Flag 1, Flag 2)"
provides:
  - "router.SetTrustedProxies(cfg.TrustedProxies) closing the spoofed X-Forwarded-For rate-limit bypass (SEC-06-FLAG2)"
  - "bodyLimitMiddleware (http.MaxBytesReader + deferred MultipartForm.RemoveAll) closing the unauthenticated multipart disk-exhaustion DoS (SEC-06-FLAG1)"
affects: ["06-public-donation-web-form-flow-b security resolution / re-audit"]

tech-stack:
  added: []
  patterns:
    - "gin.Engine.SetTrustedProxies(nil) as the safe default for a directly-exposed service — no third-party dependency, stdlib gin API"
    - "http.MaxBytesReader + deferred MultipartForm.RemoveAll wraps a public multipart route BEFORE any body-parsing middleware runs"

key-files:
  created: []
  modified:
    - donnarec-api/internal/config/config.go
    - donnarec-api/cmd/server/main.go
    - donnarec-api/cmd/server/e2e_test.go
    - donnarec-api/cmd/server/e2e_public_test.go

key-decisions:
  - "TrustedProxies defaults to nil (unset TRUSTED_PROXIES env) — trust NO proxy, so c.ClientIP() always returns RemoteAddr, ignoring any attacker-supplied X-Forwarded-For/X-Real-IP on a directly-exposed service"
  - "bodyLimitMiddleware registered AFTER ratelimit.PerIP and BEFORE captchaMW.VerifyTurnstile() — the multipart parse happens inside VerifyTurnstile's own c.PostForm call, before any CAPTCHA token is validated, so the cap must be in effect before that middleware runs"
  - "11<<20 (11 MB) body cap chosen to sit just above the handler's existing 10 MB slip cap (storage.validateSlip) so legitimate submissions are unaffected"
  - ".env.example NOT modified — Read/Edit/Write on .env.* paths is globally denied by this session's permission settings (~/.claude/settings.json deny list); TRUSTED_PROXIES is documented in config.go's field doc instead"

requirements-completed: [SEC-06-FLAG1, SEC-06-FLAG2]

coverage:
  - id: D1
    description: "Spoofed X-Forwarded-For on a direct connection no longer moves the per-IP rate-limit bucket (SEC-06-FLAG2)"
    requirement: "SEC-06-FLAG2"
    verification:
      - kind: e2e
        ref: "donnarec-api/cmd/server/e2e_public_test.go#TestPublicDonationE2E/SpoofedXFF_SharesRemoteAddrBucket_NotBypassed"
        status: pass
    human_judgment: false
  - id: D2
    description: "Oversized unauthenticated multipart body is rejected with a bounded 4xx (no hang) and creates no donation row; production still returns 400 captcha_failed unchanged; temp files are bounded/cleaned (SEC-06-FLAG1)"
    requirement: "SEC-06-FLAG1"
    verification:
      - kind: e2e
        ref: "donnarec-api/cmd/server/e2e_public_test.go#TestPublicDonationE2E/OversizedBody_Rejected_NoRow_NoHang"
        status: pass
    human_judgment: false

duration: ~35min
completed: 2026-07-17
status: complete
---

# Quick Task 260717-spx: Fix Public-Endpoint Security Flags (Trusted Proxies + Multipart Body Limit) Summary

**Closed both HIGH-severity, causally-compounding findings from 06-SECURITY.md's audit escalation — `gin.SetTrustedProxies(cfg.TrustedProxies)` stops spoofed X-Forwarded-For from bypassing `ratelimit.PerIP`, and a new `bodyLimitMiddleware` (`http.MaxBytesReader` + deferred `MultipartForm.RemoveAll`) bounds and cleans the unauthenticated multipart body before CAPTCHA's own token read can trigger an unbounded disk-spill parse.**

## Performance

- **Duration:** ~35 min
- **Tasks:** 2 (both `type="auto"`)
- **Files modified:** 4 (`config.go`, `main.go`, `e2e_test.go`, `e2e_public_test.go`)

## Setup Note: Worktree Branch Realignment (not a plan task)

Before any code edits, this worktree's per-agent branch
(`worktree-agent-a5691b310349db81b`) was discovered to be based on `4a98c9f`
(the Phase 05 PR merge on `main`) — a commit that predates every Phase 06
commit and does not contain `internal/captcha`, `internal/ratelimit`,
`cmd/server/e2e_public_test.go`, `public_handler.go`, or the
`RateLimit`/`TurnstileSecretKey` fields in `config.go` that this plan's
tasks target. `git status --short` was clean (zero commits authored yet) and
`gsd/phase-06-public-donation-web-form-flow-b` (tip `cd1947d`, which itself
contains this task's own pre-dispatch commit) was the only branch containing
the required surface. Ran `git reset --hard
gsd/phase-06-public-donation-web-form-flow-b` to fast-align the per-agent
branch pointer to `cd1947d` before starting Task 1. No commits were lost —
the worktree was branch-pointer-only, zero unique history. Flagging so the
orchestrator's wave-cleanup uses `cd1947d` (not `4a98c9f`) as `expected_base`.

## Accomplishments

- **Task 1 (Flag 2 — rate-limit IP-spoof bypass):** `Config.TrustedProxies
  []string` loaded from comma-separated `TRUSTED_PROXIES` (nil when unset —
  safe "trust no proxy" default). `setupRouter` now takes a `trustedProxies
  []string` param and calls `router.SetTrustedProxies(trustedProxies)`
  immediately after `gin.New()`, fataling on error. Prod call site passes
  `cfg.TrustedProxies`; the E2E harness passes `nil` (preserves its
  RemoteAddr-based IP resolution).
- **Task 2 (Flag 1 — unauthenticated multipart disk-exhaustion DoS):** new
  `bodyLimitMiddleware(maxBytes int64) gin.HandlerFunc` wraps
  `c.Request.Body` in `http.MaxBytesReader(c.Writer, c.Request.Body,
  11<<20)` and, in a `defer` around `c.Next()`, calls
  `c.Request.MultipartForm.RemoveAll()` when non-nil. Registered on
  `publicGroup` AFTER `ratelimit.PerIP` and BEFORE
  `captchaMW.VerifyTurnstile()` — the multipart parse happens inside
  `VerifyTurnstile`'s own `c.PostForm(TokenField)` call, before any CAPTCHA
  token is validated, so the size cap must already be in effect when that
  parse runs. Production response shape is UNCHANGED (still 400
  `{"error":"captcha_failed"}` on an oversized body, via the real
  Turnstile verifier rejecting the now-empty parsed token).
- Two new E2E sub-tests added to `TestPublicDonationE2E`
  (`cmd/server/e2e_public_test.go`), both passing under `-race` alongside
  all 5 pre-existing subtests (7/7 total).

## Task Commits

Each task was committed atomically (code only — SUMMARY/STATE/ROADMAP are the
orchestrator's responsibility per the execute-plan workflow):

1. **Task 1: Set trusted proxies (Flag 2)** — `becedfd` (fix)
   `donnarec-api/internal/config/config.go`,
   `donnarec-api/cmd/server/main.go`, `donnarec-api/cmd/server/e2e_test.go`,
   `donnarec-api/cmd/server/e2e_public_test.go`
2. **Task 2: Bound + clean multipart body (Flag 1)** — `72dca94` (fix)
   `donnarec-api/cmd/server/main.go`,
   `donnarec-api/cmd/server/e2e_public_test.go`

Both commits are on top of `cd1947d` (the pre-dispatch plan commit, now the
worktree branch's realigned base — see Setup Note above).

## Files Created/Modified

- `donnarec-api/internal/config/config.go` — `Config.TrustedProxies []string`
  field (line ~138) + `getEnvStrSlice` helper (parses `TRUSTED_PROXIES`,
  comma-split, trimmed, nil when unset) + wired into `Load()` (line 215)
- `donnarec-api/cmd/server/main.go` — `setupRouter` gained a
  `trustedProxies []string` param; `router.SetTrustedProxies(trustedProxies)`
  call at **main.go:345** (Flag 2 fix); new `bodyLimitMiddleware` function at
  **main.go:514** (definition) with `publicBodyLimitBytes = 11 << 20`
  constant at **main.go:500**, registered on `publicGroup` at **main.go:380**
  (Flag 1 fix), between `ratelimit.PerIP` (line 378) and
  `captchaMW.VerifyTurnstile()` (line 381); prod call site threading
  `cfg.TrustedProxies` at **main.go:281**
- `donnarec-api/cmd/server/e2e_test.go` — harness `setupRouter(...)` call
  site updated to pass `nil` for `trustedProxies` (line ~272, preserves
  RemoteAddr-based IP resolution)
- `donnarec-api/cmd/server/e2e_public_test.go` — added
  `doPublicSubmissionXFF` helper (sets a caller-supplied `X-Forwarded-For`
  header, slip always nil), `oversizedSlipBytes()` helper (PNG-prefixed, 12
  MB > 11 MB cap), and two new `t.Run` sub-tests inside
  `TestPublicDonationE2E`:
  - `SpoofedXFF_SharesRemoteAddrBucket_NotBypassed` — sends
    `e2ePublicRateBurst` requests sharing one RemoteAddr but each carrying a
    DIFFERENT spoofed XFF; asserts none is 429, then one more (same
    RemoteAddr, new spoofed XFF) IS 429 `rate_limited`
  - `OversizedBody_Rejected_NoRow_NoHang` — POSTs a 12 MB slip past the 11
    MB cap; asserts elapsed time < 10s (no hang), response code in
    `[400,500)` (bounded 4xx, not the specific `captcha_failed` shape since
    the harness's fake CAPTCHA verifier always passes — see the sub-test's
    inline comment for the production-vs-harness divergence), no donation
    row created (before/after count + named-row check)

- `.planning/phases/06-public-donation-web-form-flow-b/deferred-items.md`
  (created, untracked — planning doc, left for the orchestrator's docs
  commit) — logs the pre-existing, out-of-scope
  `TestE2E_MakerCheckerIssuancePipeline` chrome-sidecar Docker build failure
  (see Deviations below)

## Decisions Made

- `TrustedProxies` nil default (no `TRUSTED_PROXIES` env set) — matches the
  plan's explicit instruction not to default to a non-empty list; a
  directly-exposed service with no fronting reverse proxy should trust
  nothing.
- Body cap set to `11<<20` (11 MB), matching the plan's stated rationale
  (comfortably above the handler's existing 10 MB slip cap in
  `storage.validateSlip`).
- Oversized-body E2E sub-test asserts `code >= 400 && code < 500` rather than
  a specific error body/status, per the plan's explicit instruction (the
  harness's `fakeCaptchaVerifier` always returns nil, so the in-harness
  rejection path differs from production's real-verifier `captcha_failed`
  path — asserting the shared DoS-bounding property, not the divergent error
  shape).
- `doPublicSubmissionXFF` and `oversizedSlipBytes` added as new, additive
  helpers rather than modifying `doPublicSubmission`'s signature, per the
  plan's explicit instruction not to break existing callers.

## Deviations from Plan

### Auto-fixed / Necessary Adjustments

**1. [Setup — worktree branch realignment] Reset per-agent branch to the correct phase-06 base**
- **Found during:** Pre-Task-1 orientation (files_to_read step)
- **Issue:** Worktree branch was cut from `4a98c9f` (Phase 05 PR merge on
  `main`), predating all Phase 06 work this plan's tasks depend on
  (`internal/captcha`, `internal/ratelimit`, `e2e_public_test.go`,
  `RateLimit`/`TurnstileSecretKey` config fields all absent)
- **Fix:** `git reset --hard gsd/phase-06-public-donation-web-form-flow-b`
  (tip `cd1947d`) — safe because `git status --short` was clean and zero
  commits had been authored on the per-agent branch yet (moving only the
  branch pointer, not discarding any of my own work)
- **Verification:** confirmed `internal/captcha`, `internal/ratelimit`,
  `cmd/server/e2e_public_test.go` present and
  `grep RateLimit\|TurnstileSecretKey internal/config/config.go` matched,
  before proceeding
- **Not part of either task commit** (setup-only; see Setup Note above)

**2. [Tooling constraint — not a deviation rule, a session permission limit] `.env.example` left unmodified**
- **Found during:** Task 1, action item 4 (plan asked to document
  `TRUSTED_PROXIES` in `.env.example`)
- **Issue:** This session's global permission settings
  (`~/.claude/settings.json` → `permissions.deny: ["Read(.env.*)", ...]`)
  block Read on any `.env.*` path, which transitively blocks Edit (requires
  a prior successful Read) and Write (same contract) on `.env.example`
- **Fix:** none applied to `.env.example` itself; `TRUSTED_PROXIES`'s
  purpose/format/safe-default is documented in `Config.TrustedProxies`'s Go
  doc comment (`internal/config/config.go` line ~131) instead, which is the
  authoritative source `.env.example` would have mirrored
- **Recommended follow-up:** a human (or an agent without the `.env.*` deny
  rule) should append a `TRUSTED_PROXIES=` entry with a short comment to
  `donnarec-api/.env.example`, mirroring the existing var-documentation
  style in that file. Not blocking — `Config.Load()`'s `getEnvStrSlice`
  already handles the unset case safely (nil, trust no proxy).

---

**Total deviations:** 1 setup realignment (not a Rule 1-4 code deviation) +
1 tooling-permission limitation (no code impact — safe default already
correct without the `.env.example` entry).
**Impact on plan:** No scope creep; both fixes implemented exactly as
specified. Neither deviation altered the security posture of the shipped
fix.

## Issues Encountered

- `go test ./...` (full suite) shows one pre-existing, unrelated failure:
  `cmd/server.TestE2E_MakerCheckerIssuancePipeline` fails to start its Chrome
  sidecar testcontainer (`create container: build image: NotFound: content
  digest ...: not found`). Root-caused to a stale local Docker
  layer-cache/registry-resolution issue in `testutil.StartChrome`
  (`internal/testutil/chrome.go:74`), unrelated to the rate-limit/CAPTCHA/
  trusted-proxy files this task touched. `TestPublicDonationE2E` (the
  test this task's verification actually gates on) passes fully, including
  under `-race` (7/7 subtests, 328/329 total package tests green). Logged
  per the SCOPE BOUNDARY rule at
  `.planning/phases/06-public-donation-web-form-flow-b/deferred-items.md`
  (not fixed — out of scope for this task).

## User Setup Required

None for the code fix itself. Optional follow-up (not blocking): a human
with `.env.*` read access should add a `TRUSTED_PROXIES=` line +
short comment to `donnarec-api/.env.example` (see Deviations #2 above) for
documentation completeness — the safe nil-default behavior is already
correct without it.

## Next Phase Readiness

- Both `06-SECURITY.md`-escalated High findings (Flag 1, Flag 2) are now
  code-fixed and E2E-proven over the real HTTP path (Conventions
  integration-test gate satisfied — `TestPublicDonationE2E`, 7/7 subtests,
  `-race` clean).
- The orchestrator's next step (per this task's constraints) is to update
  `06-SECURITY.md`'s Unregistered Flags section to CLOSED, referencing the
  file:line evidence above (`main.go:345` for Flag 2,
  `main.go:380`/`main.go:500`/`main.go:514` for Flag 1) and these two commit
  hashes (`becedfd`, `72dca94`).
- Optional, non-blocking follow-up: append `TRUSTED_PROXIES=` documentation
  to `donnarec-api/.env.example` (blocked by this session's `.env.*` deny
  rule — see Deviations #2).
- Optional, non-blocking follow-up: investigate the pre-existing chrome
  sidecar Docker build failure logged in `deferred-items.md` (unrelated to
  this task, does not block either Flag's fix).

---
*Quick task: 260717-spx*
*Completed: 2026-07-17*

## Self-Check: PASSED

- FOUND: donnarec-api/internal/config/config.go
- FOUND: donnarec-api/cmd/server/main.go
- FOUND: donnarec-api/cmd/server/e2e_test.go
- FOUND: donnarec-api/cmd/server/e2e_public_test.go
- FOUND: .planning/quick/260717-spx-fix-public-endpoint-security-flags-trust/260717-spx-SUMMARY.md
- FOUND commit: becedfd
- FOUND commit: 72dca94
