---
phase: 06-public-donation-web-form-flow-b
plan: 04
subsystem: api
tags: [go, worker, outbox, i18n, email, bilingual, pdpa-safe-logging]

# Dependency graph
requires:
  - phase: 06-public-donation-web-form-flow-b
    provides: "06-03 (outbox job_type 'ack_email' enqueued in CreatePublicSubmission's tx with {\"donation_id\": ...} payload; donation.PublicReferenceNumber D-84 helper; PublicWebUserID seeded system user)"
  - phase: 04-receipt-pdf-email-delivery-outbox-worker
    provides: "Worker.ProcessOnce dispatch switch + MarkOutboxJobFailed/backoff (D-57); mailer.EmailSender seam; go-i18n bundle + donor_language locale selection; issue_receipt handler shape (payload decode → fetch → send)"
provides:
  - "Worker.handleAckEmail — decodes ack_email payload, fetches the donation, selects donor_language locale, sends a bilingual 'received, not yet a receipt' email carrying the REF- reference number; returns a plain error so ProcessOnce's uniform retry/backoff applies"
  - "ProcessOnce switch case 'ack_email' alongside 'issue_receipt' (default error arm left intact)"
  - "i18n keys ackEmail.subject / ackEmail.greeting / ackEmail.body / ackEmail.reference_label / ackEmail.footer (th + en) — body carries the non-negotiable not-yet-a-receipt statement (FR-05/D-84)"
affects: [06-06 (public form donor receives this ack after POSTing to /api/public/donations), 06-08 (any phase verification of the donor feedback loop)]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "ack_email handler mirrors issue_receipt's error contract verbatim (plain error → ProcessOnce MarkOutboxJobFailed/backoff), but sends NO attachment and records NO email_delivery row — an ack is not a receipt delivery, keeping the two flows uncoupled from the receipt-scoped email_delivery table (status CHECK: sent/failed/no_email)"
    - "no-donor-email is a terminal no-op (job → done), not a retry loop — mirrors issue_receipt's FR-28 no-email semantics without borrowing its email_delivery record"
    - "reference number rendered with a system-monospace fallback stack in email HTML (email web-font support unreliable) — same approach as the Phase 4 receipt email"

key-files:
  created:
    - donnarec-api/internal/worker/ack_email.go
    - donnarec-api/internal/worker/ack_email_test.go
  modified:
    - donnarec-api/internal/worker/worker.go
    - donnarec-api/internal/i18n/locales/th.json
    - donnarec-api/internal/i18n/locales/en.json

key-decisions:
  - "handleAckEmail records NO email_delivery row (unlike issue_receipt). The email_delivery table's status CHECK and the 04-06 staff resend UI are scoped to RECEIPT deliveries; recording ack rows there (only 'sent'/'no_email' would even satisfy the CHECK) would pollute the staff resend view with non-receipt sends and couple the two flows. The ack's observability is Pattern-C structured logs (donation_id + job_id only)."
  - "The donor-facing reference number is computed via donation.PublicReferenceNumber(payload.DonationID) — the SAME D-84 helper plan 03's response used — so the ack quotes exactly the REF- code the donor saw on-screen, and NEVER a receipt number (none exists pre-approval, T-06-15). worker importing internal/donation introduces no cycle (donation does not import worker)."
  - "No-donor-email is a terminal success (job marked done), not an error and not a retry loop — a public donor may legitimately omit an email. This mirrors issue_receipt's no-email contract behaviorally without the email_delivery record."
  - "ackEmail.subject/body carry the explicit '(ยังไม่ใช่ใบเสร็จ)' / 'ยังไม่ใช่ใบเสร็จรับเงิน' statement (and English 'not yet a receipt' / 'not a receipt'), matching 06-UI-SPEC's Ack Email Template Copy — the non-negotiable FR-05/D-84 mitigation for T-06-15. The dynamic {hospital_name} suffix from the UI-SPEC subject is omitted for parity with the existing receipt email (which likewise uses a static institutional subject, no hospital-name interpolation wired)."

patterns-established:
  - "A new outbox job_type is added by (1) a sibling handler file mirroring issue_receipt's decode→fetch→send→plain-error shape, (2) one ProcessOnce switch arm, (3) new i18n keys — the retry/backoff/dead-letter machinery is inherited unchanged"

requirements-completed: []

coverage:
  - id: D1
    description: "A th-language flow_b pending_review donation with a donor email produces exactly one bilingual ack email to that address: subject + body resolved from the ackEmail.* i18n keys, the explicit not-yet-a-receipt statement present, and the REF- reference number in the body; no receipt number appears"
    requirement: "FR-05"
    verification:
      - kind: integration
        ref: "donnarec-api/internal/worker/ack_email_test.go#TestAckEmail/Thai_BilingualNotAReceiptWithReference"
        status: pass
    human_judgment: false
  - id: D2
    description: "donor_language 'en' resolves the English catalog (subject/body in English, still not-a-receipt, still carrying the REF- reference)"
    requirement: "FR-06"
    verification:
      - kind: integration
        ref: "donnarec-api/internal/worker/ack_email_test.go#TestAckEmail/English_ResolvesEnglishCatalog"
        status: pass
    human_judgment: false
  - id: D3
    description: "A donation with no donor email sends nothing and terminates the job as 'done' (expected terminal state, not a fatal error or retry loop); an unknown job_type still hits ProcessOnce's default error arm and sends no email"
    requirement: "FR-05"
    verification:
      - kind: integration
        ref: "donnarec-api/internal/worker/ack_email_test.go#TestAckEmail/NoDonorEmail_TerminalSuccessNotRetryLoop + UnknownJobType_HitsDefaultErrorArm"
        status: pass
    human_judgment: false

# Metrics
duration: ~12min
completed: 2026-07-12
status: complete
---

# Phase 6 Plan 4: ack_email Outbox Handler (bilingual "not a receipt" acknowledgement) Summary

**The `ack_email` outbox job enqueued atomically in plan 03 is now consumed by the Phase 4 worker: `ProcessOnce` dispatches it to `handleAckEmail`, which fetches the donation, selects the donor_language locale, and sends a bilingual "received — not yet a receipt" email carrying the donor's REF- reference number — entirely off the submit request path, retryable like `issue_receipt`, closing the donor feedback loop (FR-05/FR-06, D-84/D-85).**

## Performance
- **Duration:** ~12 min
- **Completed:** 2026-07-12
- **Tasks:** 1 (TDD feature — RED → GREEN)
- **Files:** 5 (2 created, 3 modified)

## Accomplishments
- `Worker.handleAckEmail` (`internal/worker/ack_email.go`): decode `{"donation_id": ...}` → `GetDonationByID` → no-email terminal check → `composeAckEmail` → `mailer.EmailSender.Send`. Returns a plain error on send failure so `ProcessOnce`'s existing `MarkOutboxJobFailed`/backoff (D-57) applies uniformly — a failing ack never rolls back the already-committed submission (T-06-16).
- `composeAckEmail`: bilingual message from the new `ackEmail.*` i18n keys via a `donor_language` localizer, with the REF- reference number (from `donation.PublicReferenceNumber`, D-84 — never a receipt number) rendered in a system-monospace fallback stack in the HTML body. No attachment (an ack is not a receipt).
- `ProcessOnce` switch: new `case "ack_email"` alongside `issue_receipt`; the `default` unknown-job_type error arm is left intact (proven by a regression subtest).
- i18n: `ackEmail.subject/greeting/body/reference_label/footer` in `th.json` + `en.json`. The TH subject `"...(ยังไม่ใช่ใบเสร็จ)"` and body `"...ยังไม่ใช่ใบเสร็จรับเงิน..."` carry the non-negotiable not-yet-a-receipt statement matching 06-UI-SPEC (FR-05/D-84/T-06-15).
- `TestAckEmail` (integration, Postgres testcontainer only — no MinIO/Chrome; `Worker` built with nil receiptsStore/renderer to prove the ack path never reaches them): Thai bilingual send with REF- in body, English catalog resolution, no-email terminal (job → done), and unknown-job_type default arm.

## Task Commits
Plan `type: tdd` — RED → GREEN gate:
1. **RED** — `d3ec95d` (`test(06-04)`): `ack_email_test.go` fails (4/4 behavior subtests) because no `ack_email` case/handler/i18n keys exist yet. The unknown-job_type subtest already passed (the default arm pre-existed) — a correct partial RED.
2. **GREEN** — `1ca5a78` (`feat(06-04)`): handler + switch case + i18n keys; `TestAckEmail` passes 5/5.

_No REFACTOR commit — the GREEN implementation mirrors the existing `issue_receipt` handler shape exactly._

## TDD Gate Compliance
Both gate commits present in git log: RED `test(06-04)` (`d3ec95d`) then GREEN `feat(06-04)` (`1ca5a78`). Compliant.

## Verification Results
- `go test ./internal/worker/... -run TestAckEmail -count=1` — **pass** (5/5: Thai / English / NoEmail-terminal / UnknownJobType-default)
- `go build ./...` — **green**
- `go vet ./internal/worker/...` — **clean**

## Threat Mitigations (from plan threat_model)
- **T-06-15** (ack misrepresents status): `ackEmail.body` (th+en) explicitly states this is not yet a receipt; no receipt number is present (none exists pre-approval — the ack quotes only the REF- reference). Asserted by the Thai subtest (`NotContains` receipt-number label + `Contains` not-a-receipt statement). **mitigated.**
- **T-06-16** (ack send failure blocks submit): the handler runs in the worker off the request path; a send error returns a plain error → `ProcessOnce` retry/backoff, never a rollback of the committed submission. **mitigated** (inherited from the Phase 4 outbox architecture).
- **T-06-17** (donor PII in logs): only `donation_id` + `job_id` + operation name are logged (Pattern C) — the no-email log line and all error paths carry no donor name/email/tax id or body. **mitigated.**
- **T-06-SC** (package installs): none — reuses mailer/i18n/worker verbatim. **accept (no new package).**

## Deviations from Plan

### Auto-adjusted (within plan discretion)

**1. [Rule 2 - Design] `handleAckEmail` records NO `email_delivery` row**
- **Found during:** implementation (plan left this to "Claude's discretion" — 06-RESEARCH Open Question 3)
- **Issue:** `issue_receipt` records an `email_delivery` row per send. Reusing that for acks would (a) be constrained by the table's status CHECK (`sent`/`failed`/`no_email` only) and (b) pollute the 04-06 staff *receipt* resend UI with non-receipt sends.
- **Decision:** ack sends are observed via Pattern-C structured logs only; the receipt-delivery table stays receipt-scoped. Behavior the plan cares about (terminal no-op on no-email, retry on send failure) is preserved via the return-error contract, not the delivery record.
- **Files:** `internal/worker/ack_email.go`

**2. [Rule 3 - Blocking→clarify] Subject omits the UI-SPEC's `{hospital_name}` suffix**
- **Found during:** i18n authoring
- **Issue:** 06-UI-SPEC's TH subject is `"...(ยังไม่ใช่ใบเสร็จ) — {hospital_name}"`, but there is no hospital-name value wired into the mailer/i18n path (the existing Phase 4 receipt email likewise uses a static institutional subject with no such interpolation).
- **Fix:** kept the subject static through the not-a-receipt statement (the load-bearing, FR-05 part); dropped the dynamic hospital-name suffix for parity with the receipt email. No template-data plumbing introduced. If hospital-name interpolation is later wired for the receipt email, the ack subject can adopt it the same way.
- **Files:** `internal/i18n/locales/{th,en}.json`

**Total deviations:** 2, both within the plan's stated discretion; neither changes the plan's intended behavior or the T-06-15 mitigation.

## Known Stubs
None — the handler is wired end-to-end and exercised by the integration test against a real Postgres + real i18n bundle + a capturing `EmailSender`. The real email provider (SES vs Postmark) remains the Phase-4 `DevSender`/`EmailSender`-seam stakeholder gate, unchanged by this plan.

## Issues Encountered
None blocking. The four `TestAckEmail` subtests share one Postgres container and run sequentially; the deliberately-failed unknown-job_type job uses an hour-long injected backoff so it is never re-claimed by a later subtest.

## Next Phase Readiness
- **06-06** (public form UI): a donor who submits will now receive the bilingual ack (in their selected language) after the worker processes the enqueued job — the donor-facing feedback loop is closed backend-to-email.
- The worker must be running (its poll loop) in any environment/verification that asserts the donor actually receives the ack; in the hermetic E2E harness (06-03) no worker runs, so `ack_email` jobs still accumulate harmlessly as before.

---
*Phase: 06-public-donation-web-form-flow-b*
*Completed: 2026-07-12*

## Self-Check: PASSED

Both created source files + the SUMMARY confirmed present on disk; both commit hashes (`d3ec95d`, `1ca5a78`) confirmed in `git log --oneline --all`.
