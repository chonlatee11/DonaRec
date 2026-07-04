---
phase: 04-receipt-pdf-email-delivery-outbox-worker
plan: 04
subsystem: mailer + i18n
tags: [email, interface-seam, go-i18n, tdd]

# Dependency graph
requires:
  - phase: 04-receipt-pdf-email-delivery-outbox-worker
    plan: 01
    provides: email_delivery table + queries, receipt_template_config (bilingual seed) — consumed by the future worker (04-05), not directly by this plan
provides:
  - "internal/mailer.EmailSender interface (Message/Attachment/SendResult) — D-60 swappable seam"
  - "internal/mailer.DevSender — capture-to-disk implementation, zero network/provider imports"
  - "i18n message IDs receipt.* (header/donor_label/amount_label/receipt_no_label/date_label) and email.* (subject/greeting/body/footer) in th.json + en.json"
affects: [04-05]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "External-client wrapper doc-comment convention (storage/client.go 'Design decisions realized here' header) mirrored in sender.go, referencing D-60"
    - "DevSender.Send writes to a uuid-named per-message subdirectory under OutDir — same shape as RESEARCH.md's verified Code Examples"

key-files:
  created:
    - donnarec-api/internal/mailer/sender.go
    - donnarec-api/internal/mailer/dev_sender.go
    - donnarec-api/internal/mailer/dev_sender_test.go
    - donnarec-api/internal/i18n/receipt_messages_test.go
  modified:
    - donnarec-api/internal/i18n/locales/th.json
    - donnarec-api/internal/i18n/locales/en.json

key-decisions:
  - "sender.go and dev_sender.go split exactly as RESEARCH.md's Code Examples specify (interface file vs. implementation file) — no deviation from the pre-vetted shape."
  - "DevSender skips writing an attachment file when Attachment.Filename is empty (defensive; not exercised by the current test but avoids writing a stray unnamed file if a caller ever sends a message with no attachment)."
  - "i18n keys chosen to cover exactly what 04-05 (worker, PDF render) and the email body will need for FR-23/FR-26: 5 receipt.* labels + 4 email.* strings. section6_text (§6 tax wording) is intentionally NOT part of this key set — it is config-store driven (receipt_template_config, seeded in 04-01) pending an accounting/legal stakeholder gate, not an i18n catalog concern."

requirements-completed: [FR-25, FR-26, FR-23]

coverage:
  - id: D1
    description: "DevSender.Send writes body.html + attachment to OutDir, returns SendResult{SentAt: non-zero, ProviderMessageID: \"\"}, no network call"
    requirement: "FR-25"
    verification:
      - kind: test
        ref: "donnarec-api/internal/mailer/dev_sender_test.go::TestDevSender_Send_CapturesToDisk"
        status: pass
    human_judgment: false
  - id: D2
    description: "go-i18n bundle resolves email.subject and receipt.header for both th and en localizers, non-empty and differing"
    requirement: "FR-23"
    verification:
      - kind: test
        ref: "donnarec-api/internal/i18n/receipt_messages_test.go::TestBundle_ReceiptAndEmailMessageIDs_DifferByLocale"
        status: pass
    human_judgment: false
  - id: D3
    description: "DevSender has no net/smtp or provider SDK import (D-60 / CLAUDE.md no-self-hosted-SMTP)"
    requirement: "FR-26"
    verification:
      - kind: other
        ref: "grep -rn 'net/smtp|aws-sdk|postmark' internal/mailer/ — only matches the doc-comment prohibition text, no actual import"
        status: pass
    human_judgment: false

# Metrics
duration: 3min
completed: 2026-07-04
status: complete
---

# Phase 4 Plan 4: Swappable EmailSender Seam + Bilingual Receipt/Email i18n Summary

**EmailSender interface (D-60) + DevSender capture-to-disk implementation, plus 9 new go-i18n message IDs (receipt.* + email.*) driving bilingual receipt/email text — both delivered RED→GREEN via TDD**

## Performance

- **Duration:** ~3 min (commit-to-commit)
- **Completed:** 2026-07-04
- **Tasks:** 1 (single TDD task, two RED→GREEN cycles)
- **Files modified:** 6 (2 new mailer .go, 1 new mailer test, 1 new i18n test, 2 modified locale JSON)

## Accomplishments

- `internal/mailer.EmailSender` — a one-method interface (`Send(ctx, Message) (SendResult, error)`) that lets the worker (04-05) send bilingual receipt emails without depending on any concrete provider (D-60 swappable seam)
- `internal/mailer.DevSender` — dev/local capture implementation: writes `body.html` + the PDF attachment to a uuid-named subdirectory under a configured `OutDir`; zero network calls, zero provider SDK imports, verified by test and by grep
- Bilingual i18n coverage: 5 `receipt.*` message IDs and 4 `email.*` message IDs added to both `th.json` and `en.json`, verified to resolve to different non-empty strings per locale via the existing `go-i18n` bundle (no changes to `bundle.go` setup)

## Task Commits

Each RED/GREEN step was committed atomically:

1. **RED — DevSender capture-to-disk test** - `50f5dd0` (test) — also introduces `sender.go` (interface/types, non-behavior-adding, needed for the test to reference `mailer.Message`/`mailer.EmailSender`)
2. **GREEN — DevSender implementation** - `e4b15fb` (feat)
3. **RED — bilingual receipt/email message ID test** - `223e271` (test)
4. **GREEN — locale catalog additions** - `e4b20f6` (feat)

**Plan metadata:** (this commit, docs: complete plan)

## Files Created/Modified

- `donnarec-api/internal/mailer/sender.go` - `Message`, `Attachment`, `SendResult`, `EmailSender` interface (D-60); file-top doc comment lists D-60 + the no-self-hosted-SMTP rule, mirroring `internal/storage/client.go`'s "Design decisions realized here" convention
- `donnarec-api/internal/mailer/dev_sender.go` - `DevSender{OutDir}` + `Send` — writes body + attachment to a per-message uuid dir, returns `SendResult{SentAt: time.Now()}`
- `donnarec-api/internal/mailer/dev_sender_test.go` - RED→GREEN test asserting capture-to-disk, non-zero `SentAt`, empty `ProviderMessageID`
- `donnarec-api/internal/i18n/locales/th.json` - added `receipt.header/donor_label/amount_label/receipt_no_label/date_label` + `email.subject/greeting/body/footer` (Thai strings)
- `donnarec-api/internal/i18n/locales/en.json` - same 9 keys, English strings
- `donnarec-api/internal/i18n/receipt_messages_test.go` - RED→GREEN test asserting `email.subject`/`receipt.header` resolve to non-empty, differing strings under th vs en localizers

## Decisions Made

- Followed RESEARCH.md's pre-vetted `EmailSender`/`DevSender` code shape verbatim (no existing in-repo interface analog to compare against — this codebase's other external-client wrappers, e.g. `internal/storage`, are all concrete structs, not interfaces).
- `DevSender.Send` guards against writing an attachment when `Filename` is empty (defensive; not the primary tested path, but avoids an unnamed file appearing under `OutDir`).
- Limited i18n additions to exactly the `receipt.*`/`email.*` keys 04-05 (worker/PDF-render) will need; deliberately did NOT add §6 tax-deduction wording here — that text lives in `receipt_template_config` (DB config store, seeded empty in 04-01) pending an accounting/legal stakeholder sign-off, not the static i18n catalog.

## Deviations from Plan

None — plan executed exactly as written. Both RED tests failed for the expected reason (undefined `DevSender`; missing message IDs) before their corresponding GREEN commits made them pass.

## TDD Gate Compliance

Gate sequence verified in git log for this plan:
1. `test(04-04): add failing test for DevSender capture-to-disk` (50f5dd0) — RED
2. `feat(04-04): implement DevSender capture-to-disk EmailSender (D-60)` (e4b15fb) — GREEN
3. `test(04-04): add failing test for bilingual receipt/email i18n message IDs` (223e271) — RED
4. `feat(04-04): add bilingual receipt/email message IDs to i18n catalog` (e4b20f6) — GREEN

Both RED commits were confirmed failing (`go test` build failure / `message ... not found`) before their GREEN counterparts landed. No missing gates.

## Issues Encountered

None.

## User Setup Required

None — no external service configuration required. `DevSender.OutDir` will be wired from config by the worker (04-05); no new env vars introduced by this plan itself.

## Next Phase Readiness

- 04-05 (worker) can construct `&mailer.DevSender{OutDir: cfg.Something}` and call `.Send(ctx, mailer.Message{...})` to satisfy FR-25/26 without any provider dependency
- 04-05's PDF-render + email-body assembly can call `i18n.SetupBundle(...)` + `NewLocalizer(bundle, donorLanguage)` and localize `receipt.header`, `receipt.donor_label`, `receipt.amount_label`, `receipt.receipt_no_label`, `receipt.date_label`, `email.subject`, `email.greeting`, `email.body`, `email.footer` for both `th` and `en`
- No blockers for 04-05.

## Self-Check: PASSED

All created files verified present on disk (sender.go, dev_sender.go, dev_sender_test.go, receipt_messages_test.go); locale JSON modifications verified valid JSON and present. All 4 commit hashes (50f5dd0, e4b15fb, 223e271, e4b20f6) verified present in git log.

---
*Phase: 04-receipt-pdf-email-delivery-outbox-worker*
*Completed: 2026-07-04*
