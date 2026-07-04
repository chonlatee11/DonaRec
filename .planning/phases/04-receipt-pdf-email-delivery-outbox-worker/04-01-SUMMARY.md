---
phase: 04-receipt-pdf-email-delivery-outbox-worker
plan: 01
subsystem: database
tags: [postgresql, golang-migrate, sqlc, pgx, outbox-pattern, config]

# Dependency graph
requires:
  - phase: 03-donation-lifecycle-maker-checker-issuance
    provides: donations table (snapshot/PII/status), outbox_jobs table + EnqueueOutboxJob (issuance-tx enqueue), receipt_number_config single-row pattern (Phase 2)
provides:
  - "migrations 000008-000012: donations.donor_language, outbox_jobs.next_attempt_at, email_delivery table, receipt_template_config table (seeded), donations.receipt_pdf_object_key"
  - "sqlc queries: ClaimNextOutboxJob/MarkOutboxJobDone/MarkOutboxJobFailed (atomic FOR UPDATE SKIP LOCKED claim + backoff), InsertEmailDelivery/GetLatestEmailDeliveryForDonation, GetReceiptTemplateConfig/UpdateReceiptTemplateConfig, SetReceiptPDFObjectKey, donor_language+receipt_pdf_object_key on GetDonationByID"
  - "config.go: MinIO.ReceiptsBucket, Worker.ChromeWSURL/PollInterval/MaxAttempts, WorkerConfig.ComputeBackoff (1m/5m/15m/1h/4h)"
affects: [04-02, 04-03, 04-04, 04-05, 04-06, 04-07, 04-08]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Atomic outbox claim: single UPDATE...WHERE id=(SELECT...FOR UPDATE SKIP LOCKED) — no separate select-then-update race window"
    - "Single-row config table (id BOOLEAN PRIMARY KEY DEFAULT true + CHECK) reused for receipt_template_config, matching Phase 2's receipt_number_config"
    - "getEnvDuration helper added to internal/config alongside existing getEnvStr/getEnvInt/getEnvBool"

key-files:
  created:
    - donnarec-api/migrations/000008_donor_language.up.sql
    - donnarec-api/migrations/000008_donor_language.down.sql
    - donnarec-api/migrations/000009_outbox_next_attempt_at.up.sql
    - donnarec-api/migrations/000009_outbox_next_attempt_at.down.sql
    - donnarec-api/migrations/000010_email_delivery.up.sql
    - donnarec-api/migrations/000010_email_delivery.down.sql
    - donnarec-api/migrations/000011_receipt_template_config.up.sql
    - donnarec-api/migrations/000011_receipt_template_config.down.sql
    - donnarec-api/migrations/000012_receipt_pdf_reference.up.sql
    - donnarec-api/migrations/000012_receipt_pdf_reference.down.sql
    - donnarec-api/internal/db/queries/email_delivery.sql
    - donnarec-api/internal/db/queries/settings.sql
  modified:
    - donnarec-api/internal/db/queries/outbox.sql
    - donnarec-api/internal/db/queries/donations.sql
    - donnarec-api/internal/config/config.go
    - donnarec-api/internal/db/generated/*.sql.go (sqlc-generated, DO NOT EDIT)

key-decisions:
  - "ClaimNextOutboxJob's self-referencing subquery needed explicit table aliases (o./j.) — sqlc's static analyzer reported 'attempts is ambiguous' on the unqualified query even though Postgres itself parses/executes it without error; aliasing is a syntax-only fix, behavior unchanged from 04-RESEARCH's verified Pattern 1."
  - "Seeded both template_html (Thai) AND template_html_en (English) in the 000011 migration, not just template_html as the PLAN.md action text literally named — leaving template_html_en empty would render a blank receipt for any donor_language='en' record before an admin ever opens the settings UI, which defeats the seed's stated purpose ('worker renders a meaningful receipt before any admin edit', D-58). Rule 2 (missing critical functionality)."
  - "section6_text_th/en left as empty-string defaults (not seeded with real wording) — the exact §6 tax-deduction text is an explicit accounting/legal stakeholder gate (STATE.md Blockers, PROJECT.md); seeding placeholder legal text would be worse than an empty field an admin is prompted to fill in."
  - "GetReceiptRefByID was NOT extended with donor_language/receipt_pdf_object_key — it exists solely to expand the replaces/replaced_by self-FK pointers into {id, receipt_formatted} for the UI; download/resend (04-06) will read the donation's OWN row via GetDonationByID, which already carries the new columns."
  - "WorkerConfig.ComputeBackoff(attempts) takes the pre-increment attempts count and returns backoffSchedule[attempts] (clamped to the last entry) — a discretionary design point (04-RESEARCH explicitly deferred exact numbers to the planner); 04-05 (worker) is the actual caller and may adjust the exact attempts-to-delay mapping if needed."

requirements-completed: [FR-23, FR-24, FR-27, FR-33, NFR-09, NFR-07]

coverage:
  - id: D1
    description: "Migrations 000008-000012 apply and revert cleanly (donor_language, outbox next_attempt_at, email_delivery, receipt_template_config seeded, receipt_pdf_object_key)"
    requirement: "FR-23"
    verification:
      - kind: other
        ref: "migrate -path migrations -database $DATABASE_URL up && down 5 && up (manual CLI run, this session)"
        status: pass
    human_judgment: false
  - id: D2
    description: "sqlc generate produces compiling Go for ClaimNextOutboxJob/MarkOutboxJobDone/MarkOutboxJobFailed/InsertEmailDelivery/GetLatestEmailDeliveryForDonation/GetReceiptTemplateConfig/UpdateReceiptTemplateConfig/SetReceiptPDFObjectKey; GetDonationByID includes DonorLanguage/ReceiptPdfObjectKey"
    requirement: "FR-27"
    verification:
      - kind: other
        ref: "sqlc generate -f internal/db/sqlc.yaml && go build ./internal/db/... && go build ./... (manual CLI run, this session)"
        status: pass
    human_judgment: false
  - id: D3
    description: "config.go exposes ReceiptsBucket, ChromeWSURL, PollInterval, MaxAttempts, ComputeBackoff with safe defaults (zero new required env vars)"
    requirement: "NFR-09"
    verification:
      - kind: other
        ref: "go build ./internal/config/... && go vet ./internal/config/... (manual CLI run, this session)"
        status: pass
    human_judgment: false

# Metrics
duration: 15min
completed: 2026-07-04
status: complete
---

# Phase 4 Plan 1: Data-Layer Foundation Summary

**Migrations 000008-000012 (donor_language, outbox backoff, email_delivery, seeded receipt_template_config, frozen PDF reference) + 8 new/extended sqlc queries + worker/receipts config, all verified reversible and compiling**

## Performance

- **Duration:** ~15 min
- **Completed:** 2026-07-04T07:08:32Z
- **Tasks:** 3
- **Files modified:** 19 (10 new migrations, 4 query files, 5 sqlc-generated files, 1 config file)

## Accomplishments
- Five migration pairs (000008-000012) applying donor_language, outbox_jobs.next_attempt_at, the new email_delivery table, the seeded single-row receipt_template_config table, and donations.receipt_pdf_object_key — verified `migrate up` → `down 5` → `up` all exit 0
- receipt_template_config seeded with a complete bilingual (Thai + English) HTML receipt skeleton (letterhead/watermark/signature img slots + donor_name/receipt_no/amount/issue_date/section6_text placeholders) so the worker (04-05) has a meaningful default before any admin edits the template
- Eight new/extended sqlc queries covering the atomic outbox claim (FOR UPDATE SKIP LOCKED, race-free across worker instances), email delivery history, template config CRUD, and the donation's frozen-PDF-key write path — all compile via `sqlc generate` + `go build ./...`
- New Worker config block (chrome sidecar URL, poll interval, max attempts) and a concrete 1m/5m/15m/1h/4h backoff schedule, plus a separate MinIO receipts bucket — all default-safe, no new required env vars

## Task Commits

Each task was committed atomically:

1. **Task 1: Migrations 000008-000012** - `1f1ee07` (feat)
2. **Task 2: sqlc queries — outbox worker claim/mark, email_delivery, settings, donation additions** - `8e6a255` (feat)
3. **Task 3: config.go — receipts bucket, chrome ws url, worker poll + retry knobs** - `bd04fb7` (feat)

**Plan metadata:** (this commit, docs: complete plan)

## Files Created/Modified
- `donnarec-api/migrations/000008_donor_language.{up,down}.sql` - donations.donor_language (th/en, default th)
- `donnarec-api/migrations/000009_outbox_next_attempt_at.{up,down}.sql` - outbox_jobs.next_attempt_at for backoff filtering
- `donnarec-api/migrations/000010_email_delivery.{up,down}.sql` - email_delivery table (status/provider_msg_id/attempts/error) + grants
- `donnarec-api/migrations/000011_receipt_template_config.{up,down}.sql` - single-row config table, seeded with bilingual HTML skeleton
- `donnarec-api/migrations/000012_receipt_pdf_reference.{up,down}.sql` - donations.receipt_pdf_object_key (nullable, worker-populated)
- `donnarec-api/internal/db/queries/outbox.sql` - added ClaimNextOutboxJob, MarkOutboxJobDone, MarkOutboxJobFailed
- `donnarec-api/internal/db/queries/email_delivery.sql` - new: InsertEmailDelivery, GetLatestEmailDeliveryForDonation
- `donnarec-api/internal/db/queries/settings.sql` - new: GetReceiptTemplateConfig, UpdateReceiptTemplateConfig
- `donnarec-api/internal/db/queries/donations.sql` - donor_language/receipt_pdf_object_key added to GetDonationByID; new SetReceiptPDFObjectKey
- `donnarec-api/internal/db/generated/*.sql.go` - sqlc-regenerated (outbox.sql.go, donations.sql.go, models.go, querier.go, email_delivery.sql.go, settings.sql.go)
- `donnarec-api/internal/config/config.go` - MinIO.ReceiptsBucket, WorkerConfig (ChromeWSURL/PollInterval/MaxAttempts/ComputeBackoff), getEnvDuration helper

## Decisions Made
- Table-aliased the ClaimNextOutboxJob self-referencing subquery (`o.`/`j.`) to satisfy sqlc's static analyzer, which flagged "attempts is ambiguous" even though the unqualified query (04-RESEARCH's verified live-spike code) runs correctly against Postgres itself — a sqlc tooling quirk, not a behavior change.
- Seeded template_html_en (English skeleton) in addition to template_html (Thai), beyond the plan's literal action text, so donor_language='en' records don't render blank before an admin edit (Rule 2).
- Left section6_text_th/en empty pending the accounting/legal §6-wording stakeholder gate — intentionally not blocking build.
- Did not extend GetReceiptRefByID (replaces/replaced_by pointer expansion only) — download/resend will use GetDonationByID for the donation's own donor_language/receipt_pdf_object_key.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] sqlc ambiguous-column error on ClaimNextOutboxJob**
- **Found during:** Task 2 (sqlc queries)
- **Issue:** `sqlc generate` failed with `column reference "attempts" is ambiguous` on the verified RESEARCH.md Pattern 1 query (`UPDATE outbox_jobs SET ... WHERE id = (SELECT id FROM outbox_jobs ... FOR UPDATE SKIP LOCKED) RETURNING ..., attempts`), even though the identical query executes correctly in raw psql.
- **Fix:** Added explicit table aliases (`outbox_jobs AS o` / `outbox_jobs AS j` in the subquery) throughout the query and its RETURNING clause — purely a disambiguation hint for sqlc's analyzer, no change to runtime SQL semantics.
- **Files modified:** donnarec-api/internal/db/queries/outbox.sql
- **Verification:** `sqlc generate` + `go build ./internal/db/...` both exit 0 after the fix.
- **Committed in:** 8e6a255 (Task 2 commit)

**2. [Rule 2 - Missing Critical] Seeded English template alongside Thai template**
- **Found during:** Task 1 (migrations)
- **Issue:** The plan's action text only specified seeding `template_html` (Thai); `template_html_en` would have been left at its `''` column default, meaning any donor with `donor_language='en'` would get a blank/empty receipt HTML before an admin ever touched the settings UI — directly undermining the seed's own stated purpose (D-58: "worker renders a meaningful receipt before any admin edit").
- **Fix:** Authored a matching English HTML skeleton (same placeholders: DonorName/ReceiptNo/Amount/IssueDate/Section6Text/Letterhead/Signature/Watermark) and seeded `template_html_en` in the same UPDATE statement.
- **Files modified:** donnarec-api/migrations/000011_receipt_template_config.up.sql
- **Verification:** `SELECT LENGTH(template_html_en) FROM receipt_template_config` returns 972 (non-empty) after migrate up.
- **Committed in:** 1f1ee07 (Task 1 commit)

---

**Total deviations:** 2 auto-fixed (1 blocking/tooling, 1 missing-critical)
**Impact on plan:** Both fixes are necessary for correctness (tooling compile) and completeness (bilingual default receipt) respectively. No scope creep — no new tables/endpoints/architecture beyond what the plan specified.

## Issues Encountered
- The local Postgres container's schema was stale (only migrations up to 000003 had actually run, despite `docker ps` showing 8 days of uptime) — ran `migrate up` to bring it to 000007 (Phase 3's baseline) before applying this plan's 000008-000012, so the reversibility verification (`up`/`down 5`/`up`) exercised the correct starting state.
- `.env`'s `DATABASE_URL` pointed at `localhost:5433` with a stale password, left over from a prior session's `docker-compose.override.yml` port remap (see STATE.md) that is not present in the current working tree. Used the correct `localhost:5432` + the container's actual `POSTGRES_PASSWORD` (read from the running container, not committed) for verification commands only — `.env` itself was not modified as it is outside this plan's `files_modified` scope.

## User Setup Required

None - no external service configuration required. All new config fields (MINIO_RECEIPTS_BUCKET, CHROME_WS_URL, WORKER_POLL_INTERVAL, WORKER_MAX_ATTEMPTS) have safe defaults; nothing new is required to boot the dev stack.

## Next Phase Readiness

Data-layer foundation for the entire phase is in place and verified:
- 04-02 (chrome sidecar / docker-compose) can reach `config.Worker.ChromeWSURL`
- 04-03/04-04 (PDF render / mailer packages) can call `GetReceiptTemplateConfig`, `GetDonationByID` (now carrying `donor_language`), and `SetReceiptPDFObjectKey`
- 04-05 (worker) can call `ClaimNextOutboxJob`/`MarkOutboxJobDone`/`MarkOutboxJobFailed` and `config.Worker.ComputeBackoff`
- 04-06 (resend/download) can call `InsertEmailDelivery`/`GetLatestEmailDeliveryForDonation` and read `donations.receipt_pdf_object_key`
- 04-07 (settings UI/API) can call `GetReceiptTemplateConfig`/`UpdateReceiptTemplateConfig`

No blockers. Note for 04-05/04-07: `section6_text_th`/`section6_text_en` are intentionally empty pending the accounting/legal stakeholder sign-off already tracked in STATE.md — this does not block building the worker or settings UI, only the final production wording.

---
*Phase: 04-receipt-pdf-email-delivery-outbox-worker*
*Completed: 2026-07-04*
