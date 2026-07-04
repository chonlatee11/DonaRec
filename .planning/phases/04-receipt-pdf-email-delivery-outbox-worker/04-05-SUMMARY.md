---
phase: 04-receipt-pdf-email-delivery-outbox-worker
plan: 05
subsystem: worker
tags: [outbox-pattern, chromedp, minio, go-i18n, tdd, retry-backoff]

# Dependency graph
requires:
  - phase: 04-receipt-pdf-email-delivery-outbox-worker
    plan: 01
    provides: "ClaimNextOutboxJob/MarkOutboxJobDone/MarkOutboxJobFailed (atomic FOR UPDATE SKIP LOCKED + backoff), email_delivery table + queries, receipt_template_config (bilingual seed), donations.donor_language/receipt_pdf_object_key, config.Worker (ChromeWSURL/PollInterval/MaxAttempts/ComputeBackoff)"
  - phase: 04-receipt-pdf-email-delivery-outbox-worker
    plan: 03
    provides: "internal/pdf.Render/DataURI + Renderer/NewRenderer/RenderPDF — sandboxed Thai/English HTML->PDF pipeline"
  - phase: 04-receipt-pdf-email-delivery-outbox-worker
    plan: 04
    provides: "internal/mailer.EmailSender interface + DevSender, receipt.*/email.* i18n message IDs"
provides:
  - "internal/worker.Worker/Config/New/Run/ProcessOnce — the outbox poll loop + atomic claim + dispatch, with an injectable ComputeBackoff (test-controllable, decoupled from internal/config)"
  - "internal/worker's job_type=\"issue_receipt\" handler (issue_receipt.go): render-once + freeze-to-MinIO + bilingual email + email_delivery recording + done/retry/dead-letter"
  - "internal/storage.StorageClient.PutObject/GetObject — generic object read/write alongside the existing slip-specific PutSlip/PresignedGet"
  - "internal/testutil.StartMinio — MinIO testcontainers helper for real (non-mocked) object-storage integration tests"
  - "cmd/server/main.go wiring: receipts StorageClient, pdf.Renderer, i18n bundle (LOCALES_DIR), mailer.DevSender (MAIL_DEV_OUTDIR), and `go outboxWorker.Run(ctx)` on the shared signal.NotifyContext"
affects: [04-06, 04-07]

# Tech tracking
tech-stack:
  added:
    - "github.com/testcontainers/testcontainers-go/modules/minio v0.43.0 (test-only dependency)"
  patterns:
    - "worker.PDFRenderer / worker.ReceiptsStore are narrow interfaces satisfied implicitly by *pdf.Renderer / *storage.StorageClient — lets tests wrap the REAL renderer in a call-counting decorator to prove freeze-idempotency (D-56) without reimplementing the render pipeline, and lets the real chrome/MinIO sidecars still be exercised end-to-end"
    - "worker.Config.ComputeBackoff is an injected func(attempts int32) time.Duration, not a call-through to config.WorkerConfig.ComputeBackoff — decouples internal/worker from internal/config and lets tests use near-zero backoff instead of the real 1m/5m/15m/1h/4h schedule while still exercising the real MarkOutboxJobFailed/next_attempt_at DB path"
    - "Freeze-then-email ordering: getOrRenderReceiptPDF (render+freeze) runs and commits BEFORE the email send is attempted, so an email failure never re-triggers a render — retries on a failed send just re-fetch the already-frozen PDF bytes from MinIO"

key-files:
  created:
    - donnarec-api/internal/worker/worker.go
    - donnarec-api/internal/worker/issue_receipt.go
    - donnarec-api/internal/worker/worker_test.go
    - donnarec-api/internal/testutil/minio.go
  modified:
    - donnarec-api/internal/storage/client.go
    - donnarec-api/cmd/server/main.go
    - donnarec-api/go.mod
    - donnarec-api/go.sum

key-decisions:
  - "worker.Config (PollInterval/MaxAttempts/ComputeBackoff) is defined in internal/worker, not reused as config.WorkerConfig directly — main.go adapts cfg.Worker.ComputeBackoff into a plain func at wiring time. This let worker_test.go inject a near-instant backoff (1-3ms) for TestEmailRetryBackoff instead of waiting out the real 1m/5m/15m/1h/4h schedule, without needing to mutate internal/config's unexported package-level backoffSchedule var."
  - "Template branding assets (letterhead/seal/signature/watermark object keys on receipt_template_config) are fetched via the SAME ReceiptsStore/bucket as frozen receipt PDFs, not a separate bucket — no admin settings UI (04-07) exists yet to upload these assets, so this is a low-risk placeholder scoping decision; all current tests exercise the nil-object-key path (04-01's seeded template has no images configured yet), so this choice is not yet exercised by real asset bytes. Revisit if 04-07 chooses a distinct bucket/prefix convention."
  - "Skipped calling GetReceiptNumberConfig inside the render path (04-PATTERNS.md's action text mentioned it) — donations.receipt_formatted is already the frozen, final formatted string (D-42); re-reading the number-format config would be a redundant DB round-trip with no effect on what gets rendered."
  - "FontFaceCSS is left empty in ReceiptData for now (consistent with 04-03's decision) — TH Sarabun New is not yet sourced; the render pipeline falls back to fonts-thai-tlwg (Waree) already baked into docker/chrome.Dockerfile."
  - "IssueDate is formatted from donations.approved_at (the actual issuance timestamp), not donated_at, since the receipt's issue date is when it was approved/issued, not when the donation was received."
  - "LOCALES_DIR (default \"/locales\") and MAIL_DEV_OUTDIR (default \"/tmp/donnarec-mail-dev\") are read directly via os.Getenv in main.go rather than added to internal/config.Config — this plan's files_modified scope does not include config.go, and a two-line inline default is sufficient; LOCALES_DIR's default matches the Dockerfile's existing `COPY --from=builder /app/internal/i18n/locales /locales` step."
  - "No internal/audit entries are written by the worker's automated processing — the plan's must_haves/acceptance_criteria and threat_model do not require it, and the authorizing action (Approve) is already audited in Phase 3; 04-PATTERNS.md's audit-trail suggestion for 'worker-triggered actions' is advisory, not a stated requirement of this plan."

requirements-completed: [FR-25, FR-26, FR-27, NFR-07, FR-23, FR-24]

coverage:
  - id: D1
    description: "Outbox worker claims a pending issue_receipt job (atomic FOR UPDATE SKIP LOCKED), renders the receipt PDF exactly once, freezes it to the MinIO receipts bucket (donations.receipt_pdf_object_key set), emails it to the donor bilingually, records an email_delivery row (status=sent), and marks the job done"
    requirement: "FR-24"
    verification:
      - kind: integration
        ref: "go test ./internal/worker/... -run TestProcessJob_RenderFreezeEmailRecordAndIdempotency (pass)"
        status: pass
    human_judgment: false
  - id: D2
    description: "A second outbox job for the same (already-frozen) donation reuses the stored PDF — the renderer is never invoked again — and still sends a fresh email + records a second, independent email_delivery row (D-56 freeze idempotency)"
    requirement: "FR-24"
    verification:
      - kind: integration
        ref: "go test ./internal/worker/... -run TestProcessJob_RenderFreezeEmailRecordAndIdempotency (pass — same test, second half)"
        status: pass
    human_judgment: false
  - id: D3
    description: "One job (render+store+email, real chrome sidecar + real dev/local EmailSender) completes within the ~2-3s NFR-07 budget, measured off the issuance lock path"
    requirement: "NFR-07"
    verification:
      - kind: integration
        ref: "go test ./internal/worker/... -run TestProcessJobLatency (pass)"
        status: pass
    human_judgment: false
  - id: D4
    description: "A send failure increments outbox_jobs.attempts and pushes next_attempt_at forward per the injected backoff function, records a 'failed' email_delivery row per attempt, and the job becomes terminally 'failed' (dead-letter) once attempts reaches max_attempts — after which no further job is claimable"
    requirement: "FR-27"
    verification:
      - kind: integration
        ref: "go test ./internal/worker/... -run TestEmailRetryBackoff (pass)"
        status: pass
    human_judgment: false
  - id: D5
    description: "A donor with no email on file gets an email_delivery status='no_email' record (not treated as a failure) and the job is marked done — staff can still download the PDF manually (FR-28, covered by 04-06)"
    requirement: "FR-27"
    verification:
      - kind: unit
        ref: "internal/worker/issue_receipt.go handleIssueReceipt no-email branch — exercised implicitly by go build + go vet; no dedicated no-email integration test was added this plan (all worker_test.go donors have an email) — flagged for 04-06's resend/download plan to add explicit coverage when the download path is built"
        status: unknown
    human_judgment: true
    rationale: "The no-email code path (InsertEmailDelivery status='no_email', skip Send, mark done) compiles and follows the same pattern as the sent/failed branches, but was not exercised by its own integration test in this plan — a human or the 04-06 executor should add/verify a donor-has-no-email test case before considering FR-28's full delivery-status behavior proven end-to-end."
  - id: D6
    description: "go build ./cmd/server/... succeeds with the outbox worker wired to the shared signal.NotifyContext (receipts StorageClient, pdf.Renderer, i18n bundle, mailer.DevSender all constructed and passed to worker.New; go outboxWorker.Run(ctx) started alongside the HTTP server goroutine)"
    requirement: "NFR-07"
    verification:
      - kind: other
        ref: "go build ./cmd/server/... (exit 0, this session)"
        status: pass
    human_judgment: false

# Metrics
duration: ~25min
completed: 2026-07-04
status: complete
---

# Phase 4 Plan 5: Outbox Worker — Render, Freeze, Email, Retry Summary

**internal/worker polls outbox_jobs via an atomic FOR UPDATE SKIP LOCKED claim, renders each donation's receipt PDF exactly once through the sandboxed Chromium pipeline, freezes it to a MinIO receipts bucket, emails it bilingually, and records every send attempt — with bounded backoff to a terminal dead-letter, all measured under the NFR-07 latency budget**

## Performance

- **Duration:** ~25 min
- **Completed:** 2026-07-04T09:55Z
- **Tasks:** 1 (single `tdd="true"` task, RED then GREEN)
- **Files modified:** 8 (4 created, 4 modified)

## Accomplishments

- `internal/worker/worker.go`: `Worker`/`Config`/`New`/`Run`/`ProcessOnce` — the poll loop shares the same `signal.NotifyContext` shutdown pattern as `cmd/server/main.go`'s HTTP server goroutine; `ProcessOnce` claims exactly one due job via `ClaimNextOutboxJob` (race-free across worker instances, T-04-11) and dispatches by `job_type`
- `internal/worker/issue_receipt.go`: the `job_type="issue_receipt"` handler — loads the donation snapshot, renders the PDF exactly once against `receipt_template_config` (donor_language-selected template + §6 text), freezes it to the receipts MinIO bucket and sets `receipt_pdf_object_key` (D-56), then composes a bilingual email (go-i18n) with the PDF attached and records an `email_delivery` row per send attempt (sent/failed/no_email)
- Freeze idempotency (D-56, T-04-14) proven live: a second job for an already-frozen donation reuses the stored PDF bytes from MinIO — the renderer is never invoked again — while still sending a fresh email and recording an independent `email_delivery` row (supports resend, 04-06)
- Bounded retry + dead-letter (D-57): a failing send increments `attempts` and pushes `next_attempt_at` forward per an injectable backoff function; once `attempts` reaches `max_attempts` the job transitions to terminal `'failed'` and is excluded from further claims — staff-visible dead-letter, no infinite auto-retry (T-04-12)
- NFR-07 latency proven: one full job (render via the real chrome sidecar + store to real MinIO + send via the real `mailer.DevSender`) completes in ~23s of *test setup* but the `ProcessOnce` call itself is asserted under the 2-3s budget, entirely off the issuance lock path
- `internal/storage/client.go`: added `PutObject`/`GetObject` (generic byte read/write) alongside the existing slip-specific `PutSlip`/`PresignedGet`
- `internal/testutil/minio.go`: new MinIO testcontainers helper, mirroring `SetupTestPostgres`/`StartChrome`'s `t.Helper`/`t.Cleanup` shape
- `cmd/server/main.go`: wired the receipts `StorageClient`, `pdf.Renderer`, i18n bundle, `mailer.DevSender`, and `go outboxWorker.Run(ctx)` on the same shutdown context as the HTTP server

## Task Commits

Each RED/GREEN step was committed atomically:

1. **RED — failing test for outbox worker render/freeze/email/retry pipeline** - `8448889` (test) — `internal/worker` did not exist yet (confirmed compile failure: "no non-test Go files in .../internal/worker")
2. **GREEN — outbox worker implementation** - `2c8d44d` (feat)

**Plan metadata:** (this commit, docs: complete plan)

## Files Created/Modified

- `donnarec-api/internal/worker/worker.go` - `Worker`, `Config`, `New`, `Run`, `ProcessOnce`, `ReceiptsStore`/`PDFRenderer` interfaces, `ErrNoJob`
- `donnarec-api/internal/worker/issue_receipt.go` - `handleIssueReceipt`, `getOrRenderReceiptPDF`, `renderReceiptPDF`, `fetchTemplateImage`, `composeReceiptEmail`, `formatAmount`, `formatIssueDate`, `sanitizeFilename`
- `donnarec-api/internal/worker/worker_test.go` - `TestProcessJob_RenderFreezeEmailRecordAndIdempotency`, `TestProcessJobLatency`, `TestEmailRetryBackoff` + `seedIssuedDonation`/`fakeSender`/`countingRenderer` test helpers
- `donnarec-api/internal/storage/client.go` - `PutObject`, `GetObject`
- `donnarec-api/internal/testutil/minio.go` - `StartMinio` testcontainers helper
- `donnarec-api/cmd/server/main.go` - receipts `StorageClient` + `pdf.Renderer` + i18n bundle + `mailer.DevSender` + `worker.New`/`go outboxWorker.Run(ctx)` wiring
- `donnarec-api/go.mod` / `go.sum` - `github.com/testcontainers/testcontainers-go/modules/minio` added (test-only)

## Decisions Made

- `worker.Config.ComputeBackoff` is an injected function rather than a direct call to `config.WorkerConfig.ComputeBackoff`, decoupling `internal/worker` from `internal/config` and letting `worker_test.go` use a near-instant backoff for `TestEmailRetryBackoff` without touching the production 1m/5m/15m/1h/4h schedule or its unexported package variable.
- `worker.PDFRenderer`/`worker.ReceiptsStore` are narrow interfaces satisfied implicitly by `*pdf.Renderer`/`*storage.StorageClient` — this let the test wrap the REAL renderer in a call-counting decorator to prove freeze-idempotency while still exercising the genuine chrome-sidecar render pipeline for the first render.
- Template branding assets (letterhead/seal/signature/watermark) are fetched via the same receipts bucket/`ReceiptsStore` as frozen PDFs — no dedicated asset bucket exists yet since 04-07 (settings UI) hasn't been built; low-risk since all current object keys are nil.
- Skipped an unnecessary `GetReceiptNumberConfig` call in the render path — `donations.receipt_formatted` is already the frozen final string (D-42); re-deriving the format would be a no-op DB round-trip.
- `LOCALES_DIR`/`MAIL_DEV_OUTDIR` env vars read inline in `main.go` (defaults `/locales`, `/tmp/donnarec-mail-dev`) rather than added to `internal/config.Config`, staying within this plan's `files_modified` scope while still making the wiring actually work at runtime (the Dockerfile already stages locales at `/locales`).
- No worker-triggered `audit_log` entries added — not required by this plan's `must_haves`/`threat_model`; the authorizing action (Approve) is already audited in Phase 3.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical] Added `storage.StorageClient.GetObject`, not just `PutObject`**
- **Found during:** Task 1 (implementation) — the plan's action text explicitly named only `PutObject`, but D-56 freeze-idempotency and template-asset fetching both require reading bytes back from MinIO.
- **Issue:** Without a `GetObject` method, a second job for an already-frozen donation (resend) could never re-attach the stored PDF to a new email, and template branding images (letterhead/seal/signature/watermark) could never be inlined as data URIs.
- **Fix:** Added `GetObject(ctx, objectKey) ([]byte, error)` alongside `PutObject`, using the same package-prefixed error-wrap convention as the rest of `storage/client.go`.
- **Files modified:** `donnarec-api/internal/storage/client.go`
- **Verification:** Exercised directly by `TestProcessJob_RenderFreezeEmailRecordAndIdempotency`'s second-job assertions (renderer not re-invoked, PDF bytes reused).
- **Committed in:** `8448889` (RED commit, since the test references it) — implementation logic lives in `2c8d44d`.

**2. [Rule 3 - Blocking] `worker.Config` decoupled from `config.WorkerConfig`**
- **Found during:** Task 1 (implementation) — a direct `config.WorkerConfig.ComputeBackoff` call in the worker would make `TestEmailRetryBackoff` either wait out real 1-minute-plus backoff windows or require mutating `internal/config`'s unexported `backoffSchedule` package variable from a different package (impossible).
- **Fix:** Defined `worker.Config` with a `ComputeBackoff func(attempts int32) time.Duration` field; `main.go` adapts `cfg.Worker.ComputeBackoff` into this shape at construction time, while `worker_test.go` injects a millisecond-scale function.
- **Files modified:** `donnarec-api/internal/worker/worker.go`, `donnarec-api/cmd/server/main.go`
- **Verification:** `go test ./internal/worker/... -run TestEmailRetryBackoff` completes in ~23s (dominated by container startup, not backoff waiting) and asserts `next_attempt_at` advances per the injected schedule.
- **Committed in:** `2c8d44d`

**3. [Rule 3 - Blocking] `LOCALES_DIR`/`MAIL_DEV_OUTDIR` wiring added to `main.go`**
- **Found during:** Task 1 (implementation) — `internal/i18n.SetupBundle` and `mailer.DevSender` both require a filesystem path, but no env var or default existed anywhere in the codebase for either, and `main.go` had never called `i18n.SetupBundle` before this plan.
- **Fix:** Added inline `os.Getenv`-with-default resolution in `main.go` (`LOCALES_DIR` default `/locales`, matching the existing `Dockerfile`'s `COPY ... /locales` step; `MAIL_DEV_OUTDIR` default `/tmp/donnarec-mail-dev`) rather than extending `internal/config.Config` (out of this plan's `files_modified` scope).
- **Files modified:** `donnarec-api/cmd/server/main.go`
- **Verification:** `go build ./cmd/server/...` (exit 0).
- **Committed in:** `2c8d44d`

---

**Total deviations:** 3 auto-fixed (1 missing-critical, 2 blocking)
**Impact on plan:** All three were necessary for the worker to actually function end-to-end (resend/freeze reuse, a testable retry path, and a bootable `main.go`). No architectural scope creep — no new tables, endpoints, or services beyond what the plan specified.

## Issues Encountered

- A stray `server` binary was accidentally produced in the repo root by an intermediate `go build ./cmd/server/...` verification run (Go names the output after the last path segment when no `-o` is given); deleted before staging, never committed.
- No other issues — Docker, the pre-built `donnarec-chrome-test`/`chromedp/headless-shell:stable` images, and `minio/minio:RELEASE.2024-01-16T16-07-38Z` were all already available locally, so all three integration tests ran against real infrastructure (no mocked Postgres/chrome/MinIO) on the first attempt after implementation.

## User Setup Required

None — no external service configuration required. `LOCALES_DIR`/`MAIL_DEV_OUTDIR`/`CHROME_WS_URL`/`MINIO_RECEIPTS_BUCKET` all have safe defaults; nothing new is required to boot the dev stack beyond what 04-01/04-02 already introduced.

## Next Phase Readiness

- 04-06 (resend/download) can re-enqueue an `issue_receipt` outbox job for a donation whose `receipt_pdf_object_key` is already set — the worker will reuse the frozen PDF and send/record a fresh `email_delivery` row (proven this plan). The download endpoint can read the same `receipt_pdf_object_key` via `storage.StorageClient.PresignedGet` (existing) or `GetObject` (new this plan).
- 04-06 should add an explicit "donor has no email" integration test (coverage item D5 above) — the code path exists and compiles but was not exercised by its own test in this plan.
- 04-07 (settings UI) should confirm/decide which bucket template branding assets (letterhead/seal/signature/watermark) live in — this plan defaulted to the same receipts bucket via `ReceiptsStore.GetObject`, exercised only via the nil-object-key path so far.
- No blockers.

## Self-Check: PASSED

Verified files exist on disk: `donnarec-api/internal/worker/worker.go`, `donnarec-api/internal/worker/issue_receipt.go`, `donnarec-api/internal/worker/worker_test.go`, `donnarec-api/internal/testutil/minio.go` (all FOUND). Verified commit hashes `8448889` and `2c8d44d` present in `git log --oneline --all` (both FOUND).

---
*Phase: 04-receipt-pdf-email-delivery-outbox-worker*
*Completed: 2026-07-04*
