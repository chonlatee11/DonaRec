---
phase: 06-public-donation-web-form-flow-b
verified: 2026-07-12T03:24:06Z
status: human_needed
score: 8/9 must-haves verified
behavior_unverified: 1
overrides_applied: 0
mode: mvp
behavior_unverified_items:
  - truth: "The mobile nav drawer + bilingual public form/queue are usable and correctly laid out on a mobile viewport in Thai and English (NFR-06)"
    test: "Bring up the local stack; on a <768px viewport open /donate and /queue in both Thai and English"
    expected: "Sidebar collapses to hamburger drawer (backdrop + Escape + focus return); form and queue lay out correctly, no overflow; wide tables scroll horizontally; both languages render without truncation/tofu"
    why_human: "Responsive layout and visual bilingual rendering are runtime/visual properties grep and unit tests cannot observe; recorded as PENDING HUMAN UAT (plan 06-08 Task 2)"
human_verification:
  - test: "Responsive + bilingual walkthrough (NFR-06, plan 06-08 Task 2)"
    expected: "Public form + staff queue usable and correctly laid out on desktop AND mobile in both Thai and English; AppShell hamburger drawer works (backdrop, Escape, focus return); wide tables scroll horizontally below 768px"
    why_human: "Visual/responsive behavior; NFR-06 intentionally left Pending in REQUIREMENTS.md until this walkthrough passes"
  - test: "Real Cloudflare Turnstile challenge on a live stack"
    expected: "With real NEXT_PUBLIC_TURNSTILE_SITE_KEY + TURNSTILE_SECRET_KEY, the donor completes an actual Turnstile challenge and submission succeeds; a missing/failed challenge is rejected"
    why_human: "Automated E2E injects a fake verifier; the real challenge widget + siteverify round-trip can only be exercised against a live Cloudflare-keyed stack (plan 06-08 user_setup)"
  - test: "Authenticated staff queue end-to-end walkthrough (FR-08)"
    expected: "A logged-in staff user opens /queue, sees both Flow A and Flow B pending_review rows with source badges, the 3-chip source filter narrows correctly, and a Flow B record shows the source-aware creator label"
    why_human: "Plan 06-07 shipped the authenticated /queue BFF path with build-green + manual only — no NEW automated E2E drives a real Keycloak token through /queue -> BFF -> Go?source=. See Gaps Summary: this is a flagged decision, not a hard failure (the Go source filter and the authed /api/donations HTTP path already carry integration/E2E coverage)."
---

# Phase 6: Public Donation Web Form (Flow B) Verification Report

**Phase Goal:** A donor can submit a bilingual donation request with a slip and PDPA consent through a public, bot-protected web form, which lands in a pending-review queue and flows through the exact same back-office approval and issuance pipeline.
**Verified:** 2026-07-12T03:24:06Z
**Status:** human_needed
**Re-verification:** No — initial verification
**Mode:** MVP (goal narrowed to the donor outcome; roadmap Success Criteria are the contract)

## Goal Achievement

### Observable Truths

| #   | Truth | Status | Evidence |
| --- | ----- | ------ | -------- |
| 1 | A donor multipart POST to /api/public/donations creates ONE `donations` row `status=pending_review`, `source=flow_b`, `created_by=public-web`, plus exactly one linked slip — all committed atomically (SC1) | ✓ VERIFIED | `TestPublicDonationE2E/HappyPath` passes under -race (real testcontainer, 23s): asserts status/source/created_by, 1 slip row, 1 ack_email job, 0 issue_receipt, 1 audit row. `public_submission.go:141-211` single `WithTx` (create→submit→slip→audit→outbox). `TestCreatePublicSubmission` passes. |
| 2 | Donor tax ID is envelope-encrypted at rest as in Flow A; consent snapshot (given/at/text_version/purpose) captured, tied to Phase 1 retention (SC3) | ✓ VERIFIED | `public_submission.go:96` `crypto.EncryptField` BEFORE tx (plaintext never reaches PG); consent fields threaded into `CreateDonationParams`; `consent_at=now()` when given; `retain_until = donated_at + 10y`, `legal_basis="consent"` |
| 3 | Slip validated server-side by real magic bytes + size, stored outside webroot; a bad/missing slip creates NO donation row (SC2) | ✓ VERIFIED | `TestPublicDonationE2E/BadMagicByteSlip_Rejected_NoRow` → 415 + row count unchanged; `MissingSlip_Rejected_NoRow` → 400 `slip_required`; `public_handler.go:150` `PutSlip` validates before any DB write; MinIO/S3 storage (not webroot) |
| 4 | Public form is bot-protected: Turnstile verified server-side fail-closed + per-IP rate limit returns 429 (SC4, FR-04) | ✓ VERIFIED (automated) | `TestPublicDonationE2E/RateLimit...429`; `TestTurnstileVerifier_Verify` (empty/false/network → error, never nil); `TestPerIP_TokenBucket`; `main.go:366-369` publicGroup uses `ratelimit.PerIP` + `captcha.VerifyTurnstile`. Real live-challenge round-trip is a human item. |
| 5 | Donor receives a bilingual acknowledgement stating "received, NOT yet a receipt" with the reference number, sent off the request path (SC4, FR-05/06) | ✓ VERIFIED | `TestAckEmail` asserts TH "ยังไม่ใช่ใบเสร็จ" subject/body, EN catalog resolves, REF- reference present, receipt number ABSENT; `worker.go:222` `case "ack_email" → handleAckEmail`; enqueued in-tx, consumed in worker (off request path) |
| 6 | The public route is genuinely session-less — no RequireAuth on Go, no Authorization/getServerSession on the Next passthrough | ✓ VERIFIED | `main.go` publicGroup hangs off ROOT router (not `/api`), the FIRST group without `RequireAuth`, exactly ONE handler exposed; `route.ts` sends NO bearer, NO `getServerSession`, re-posts fresh multipart FormData |
| 7 | At /donate the submit button is gated until fields+slip+consent+Turnstile all satisfied; language toggle drives both UI and `donor_language`; success swaps to in-page confirmation with a reference (not a receipt) + copy button (FR-01/02/03/06) | ✓ VERIFIED | vitest 48/48 pass; `PublicDonationForm.tsx:130-160` gating on `isValid && consentChecked && turnstileToken && slip`; `useLocale` single source for render + `donor_language`; `PublicDonationConfirmation.tsx` renders REF-, "not yet a receipt", clipboard copy |
| 8 | Staff open /queue and see pending_review Flow A + Flow B rows with a source badge, a 3-chip source filter that narrows by source, and a source-aware creator label on Flow B records (FR-08) | ✓ VERIFIED (code + wired; authed E2E is a human item) | `queue/page.tsx` renders `QueueTable`+`QueueSourceFilter`, filters server-side on `status=pending_review`; `bff/queue/route.ts` session-bound `bffForward`, maps from-website→flow_b / staff-entered→flow_a; `TestSearchDonations_SourceFilter` (DB integration) proves the Go filter; `SourceBadge` + `DonationDetailView` creator-label branch present |
| 9 | Mobile nav drawer + bilingual public form/queue are usable and correctly laid out on a mobile viewport in Thai and English (SC5, NFR-06) | ⚠️ PRESENT_BEHAVIOR_UNVERIFIED | `MobileNavDrawer.tsx` code-complete: hamburger (`md:hidden`), backdrop, Escape, focus-trap + focus-return, `role="dialog" aria-modal`; table scroll wrappers added. But the responsive + bilingual HUMAN walkthrough (plan 06-08 Task 2) was NOT performed — recorded PENDING HUMAN UAT. Visual/responsive runtime property; not observable by grep/unit test. → human verification |

**Score:** 8/9 truths verified (1 present, behavior-unverified)

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | -------- | ------ | ------- |
| `donnarec-api/migrations/000015_donation_source.up.sql` | source column + backfill | ✓ VERIFIED | 30 lines; migrations apply/rollback test green |
| `donnarec-api/migrations/000016_seed_public_web_user.up.sql` | fixed-UUID public-web user | ✓ VERIFIED | 61 lines; UUID `00000000-0000-4000-8000-000000000006` |
| `donnarec-api/internal/captcha/turnstile.go` + `verifier.go` + `middleware.go` | fail-closed Turnstile | ✓ VERIFIED | Wired into publicGroup; `TestTurnstileVerifier_Verify` green |
| `donnarec-api/internal/ratelimit/middleware.go` | per-IP token bucket | ✓ VERIFIED | 112 lines; `TestPerIP_TokenBucket` green; wired into publicGroup |
| `donnarec-api/internal/donation/public_submission.go` / `public_handler.go` | atomic public submission | ✓ VERIFIED | Substantive; encryption + atomic tx + audit + outbox |
| `donnarec-api/cmd/server/e2e_public_test.go` | real-path E2E gate | ✓ VERIFIED | 220 lines; passes under -race; satisfies CONVENTIONS integration gate for the unauthenticated seam |
| `donnarec-api/internal/worker/ack_email.go` | bilingual ack handler | ✓ VERIFIED | 165 lines; `TestAckEmail` green |
| `donnarec-web/app/(public)/layout.tsx` + `public-theme.css` + `(app)/layout.tsx` | dual-theme route split | ✓ VERIFIED | Warm `.theme-public` scope; (app) slate/blue unchanged |
| `donnarec-web/components/PublicDonationForm.tsx` + `PublicDonationConfirmation.tsx` + `TurnstileWidget.tsx` | donor surface | ✓ VERIFIED | 494/134/72 lines; vitest form-gating green |
| `donnarec-web/app/api/public/donations/route.ts` | session-less passthrough | ✓ VERIFIED | No auth; fresh multipart forward |
| `donnarec-web/app/(app)/queue/page.tsx` + `SourceBadge.tsx` + `QueueSourceFilter.tsx` + `bff/queue/route.ts` | staff queue (FR-08) | ✓ VERIFIED (code+wired) | Authed BFF path lacks a NEW real-token E2E — see Gaps Summary |
| `donnarec-web/components/MobileNavDrawer.tsx` | mobile drawer (NFR-06) | ⚠️ ORPHANED-CLEAR / behavior-unverified | Code-complete + wired to AppShell hamburger; visual/responsive behavior human-pending |

### Key Link Verification

| From | To | Via | Status |
| ---- | -- | --- | ------ |
| publicGroup (no RequireAuth) | public_handler.CreatePublic | `ratelimit.PerIP` + `captcha.VerifyTurnstile` | ✓ WIRED (main.go:366-369) |
| storage.PutSlip (before tx) | CreatePublicSubmission one WithTx | Create+Submit+InsertSlip+AppendAuditEntryTx+EnqueueOutboxJob | ✓ WIRED (public_submission.go) |
| worker ProcessOnce `ack_email` | handleAckEmail | mailer.EmailSender + i18n catalog | ✓ WIRED (worker.go:222) |
| PublicDonationForm (token+slip+consent gating) | /api/public/donations route.ts | session-less passthrough to Go | ✓ WIRED |
| LocaleSwitcher selection | rendered locale AND donor_language | `useLocale` single source | ✓ WIRED |
| QueueSourceFilter chip | Go /api/donations?source= | queue BFF `?source=` mapping | ✓ WIRED (unit-tested mapping; authed HTTP path E2E is a human item) |
| hamburger button (AppShell) | MobileNavDrawer open state | shared open state | ✓ WIRED (visual behavior human-pending) |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| Unauthenticated public seam end-to-end | `go test -race -run TestPublicDonationE2E ./cmd/server/` | ok 23.3s | ✓ PASS |
| Turnstile fail-closed + rate limit units | `go test -race ./internal/captcha ./internal/ratelimit` | ok | ✓ PASS |
| Atomic public submission (service) | `go test -run TestCreatePublicSubmission ./internal/donation/` | ok | ✓ PASS |
| Ack "not a receipt" bilingual | `go test -run TestAckEmail ./internal/worker/` | ok | ✓ PASS |
| Frontend suite (form gating, BFF mapping, etc.) | `npx vitest run` | 48/48 pass | ✓ PASS |
| Real Turnstile challenge on live stack | (requires live Cloudflare keys) | — | ? SKIP → human |
| Responsive/bilingual mobile layout | (visual) | — | ? SKIP → human |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| ----------- | ----------- | ----------- | ------ | -------- |
| FR-01 | 06-01, 06-03, 06-06 | Donor fills form (donor + amount + date) | ✓ SATISFIED | Truths 1, 7 |
| FR-02 | 06-03, 06-06 | Slip upload (jpg/png/pdf, size, type) | ✓ SATISFIED | Truth 3 (magic-byte E2E) |
| FR-03 | 06-03, 06-06 | Show + record PDPA consent before submit | ✓ SATISFIED | Truths 2, 7 |
| FR-04 | 06-02, 06-03 | Spam/bot protection (CAPTCHA / rate limit) | ✓ SATISFIED (automated) | Truth 4; live challenge = human item |
| FR-05 | 06-04, 06-06 | Ack email "received, not yet a receipt" | ✓ SATISFIED | Truth 5 |
| FR-06 | 06-04, 06-05, 06-06 | Thai/English on the web form | ✓ SATISFIED (code) | Truths 5, 7; visual bilingual = human item |
| FR-08 | 06-01, 06-07 | Pending-review queue of web submissions | ✓ SATISFIED (code+wired) | Truth 8; authed E2E flagged (Gaps Summary) |
| NFR-06 | 06-05, 06-08 | Responsive + bilingual desktop/mobile | ⚠️ NEEDS HUMAN | Truth 9; intentionally Pending in REQUIREMENTS.md |

All 8 phase requirement IDs are accounted for. No orphaned requirements: REQUIREMENTS.md maps exactly FR-01/02/03/04/05/06/08 + NFR-06 to Phase 6, matching the union of plan `requirements` fields.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| (none) | — | No TODO/FIXME/XXX/TBD/placeholder/not-implemented markers in any phase-modified source file | — | Clean |

### Human Verification Required

1. **Responsive + bilingual walkthrough (NFR-06, plan 06-08 Task 2)** — open `/donate` and `/queue` on desktop and mobile in Thai and English; confirm the hamburger drawer (backdrop/Escape/focus-return), correct layouts, and horizontal table scroll below 768px.
2. **Real Turnstile challenge on a live stack** — with real Cloudflare keys, complete an actual challenge and confirm submission succeeds while a failed/absent challenge is rejected.
3. **Authenticated staff queue walkthrough (FR-08)** — logged-in staff open `/queue`, see Flow A + Flow B pending rows with source badges, exercise the 3-chip filter, and confirm the Flow B source-aware creator label.

### Gaps Summary

No FAILED truths, MISSING/STUB artifacts, unwired key links, or blocker anti-patterns were found. The entire automated must-have surface is green:

- The **load-bearing unauthenticated seam** (donor → real HTTP path → atomic pending_review Flow B) is proven end-to-end under `-race` against a Postgres testcontainer — this satisfies the CLAUDE.md/CONVENTIONS integration-test gate for the phase's first unauthenticated route group.
- Full backend suite, `npx vitest` (48/48), and the E2E all pass.

Two acceptance items remain outstanding and are surfaced for human decision (escalation gate):

1. **NFR-06 responsive + bilingual walkthrough + real Turnstile (plan 06-08 Task 2)** — NOT performed; recorded as PENDING HUMAN UAT and intentionally left Pending in REQUIREMENTS.md. This is the primary reason the phase status is `human_needed`, not `passed`. It is a required human checkpoint, not an automatable one.

2. **FR-08 authenticated queue path has no NEW automated E2E** — plan 06-07 shipped build-green + manual only. Per CONVENTIONS this is a runtime-request-seam surface that would normally warrant an E2E over a real Keycloak token. Assessed severity: **WARNING / acceptable-gap, flagged for human decision**, because the residual uncovered surface is thin — (a) the Go `source` filter is covered by `TestSearchDonations_SourceFilter` (DB integration), (b) the authenticated `/api/donations` HTTP path already carries prior-phase E2E (`cmd/server/e2e_test.go`), and (c) the BFF source-token → flow_a/flow_b mapping is unit-tested. The only unproven link is the UI-to-BFF wiring, which the manual queue walkthrough (human item 3) covers. If the team wants the CONVENTIONS gate satisfied strictly, add a real-token E2E driving `/queue → BFF → Go?source=`; otherwise accept via the manual walkthrough.

**Recommendation:** Do not mark the phase Complete until the three human_verification items pass. The automated foundation is solid; the outstanding work is genuinely human (visual/responsive + live-CAPTCHA + staff UI walkthrough), consistent with the honest baseline provided.

---

_Verified: 2026-07-12T03:24:06Z_
_Verifier: Claude (gsd-verifier)_
