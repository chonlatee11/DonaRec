---
phase: 04-receipt-pdf-email-delivery-outbox-worker
scope: backend (Go+SQL) — BL-/BW-/BI- findings
fixed_at: 2026-07-05T00:00:00Z
review_path: .planning/phases/04-receipt-pdf-email-delivery-outbox-worker/04-REVIEW-PRESHIP.md
iteration: 1
tdd_mode: true
findings_in_scope: 9
fixed: 6
deferred: 1
skipped: 2
status: all_actioned
---

# Phase 04 — Backend Code-Review Fix Report

TDD applied: every behavior-changing fix landed as a RED failing test first,
then the GREEN fix. Config/perms/doc/refactor changes are TDD-exempt.

**Environment caveat (affects verification, not code):** `sqlc` and a running
Docker daemon were both unavailable in this environment. Consequences:
- sqlc-generated Go (`internal/db/generated/*.sql.go`, `querier.go`) was updated
  **by hand** to match sqlc v1.31.1 output conventions; the `.sql` sources were
  edited in lockstep.
- The DB-backed integration RED tests (BW-01/BW-03/BW-04) require a Postgres
  testcontainer, so they were **not executed locally** — they compile, are wired
  through the public API, and encode the invariant. BL-01's fix is a pure
  function fully exercised locally (true RED → GREEN observed).

**Green status:** `go build ./...` OK · `go vet ./...` clean · `go test -short
./...` all pass (the 4 gofmt-flagged files are pre-existing and untouched by this
pass). DB-integration tests are skipped under `-short` pending a Docker daemon.

## Summary

| Finding | Severity | Outcome | Commit(s) |
|---------|----------|---------|-----------|
| BL-01 | Blocker | fixed (requires human verification — compliance wording) | `5d3d8e1` RED, `3acf55a` GREEN |
| BW-01 | Warning | fixed | `6e1ba91` RED, `15c0f43` GREEN |
| BW-03 | Warning | fixed | `2dd79ac` RED, `cf2cbd1` GREEN |
| BW-04 | Warning | fixed | `8e07c3a` RED, `bc9c0ca` GREEN |
| BI-01 | Info | fixed | `823032e` |
| BI-04 | Info | fixed (refactor) | `2c2be1b` |
| BW-02 | Warning | deferred (tracked backlog) | `72b9921` |
| BI-03 | Info | skipped (by-design) | — |
| BI-05 | Info | skipped (scale) | — |

## Fixed

### BL-01 — Receipt issue date UTC/ISO → Asia/Bangkok + Thai BE
**Files:** `internal/receiptfmt/date.go` (new), `internal/worker/issue_receipt.go`,
`internal/settings/service.go`
Extracted one shared `receiptfmt.FormatIssueDate(t, lang)` that normalises the
instant to `Asia/Bangkok` first, then renders Thai Buddhist-Era + Thai month
abbreviation for `th` (matching the existing preview fixture `"15 มี.ค. 2569"`)
and Gregorian `"2 Jan 2006"` for English. Both the worker (real receipt) and the
settings preview fixture now call this helper, so **preview == real**. RED test
proves an `approved_at` of `2026-06-01T18:30:00Z` (Bangkok `2026-06-02 01:30`)
renders the Bangkok date; a year-boundary case is covered too.
**Human verification flag:** exact BE-vs-CE era + month wording is pending the
accounting/legal stakeholder gate per CLAUDE.md — aligned to the existing preview
fixture as current intent.

### BW-01 — Panic+reclaim path never dead-lettered (unbounded retry)
**Files:** `internal/db/queries/outbox.sql`,
`internal/db/generated/outbox.sql.go`, `internal/db/generated/querier.go`,
`internal/worker/worker.go`
`ReclaimStuckOutboxJobs` now increments `attempts` and applies the same terminal
CASE as `MarkOutboxJobFailed` (→ `failed` once `attempts+1 >= max_attempts`), so
a deterministically-panicking job — which `ProcessOnceSafe` leaves `processing`
without incrementing attempts — is bounded and eventually dead-letters. Added the
`max_attempts` param; `ReclaimStuckJobs` passes `cfg.MaxAttempts`. Existing
reclaim tests still hold (attempts 0→1 with MaxAttempts 5 stays `pending`).

### BW-03 — SaveTemplateImage lost-update race
**Files:** `internal/db/queries/settings.sql`,
`internal/db/generated/settings.sql.go`, `internal/db/generated/querier.go`,
`internal/settings/service.go`
Replaced the unlocked read-whole-row / mutate-one-slot / write-whole-row path
with a new `UpdateTemplateImageKey` query: a single atomic UPDATE that sets only
the target slot's column and reads every other slot's own current value in the
same statement, so a concurrent upload to a different slot is never clobbered.
RED test drives four concurrent slot uploads and asserts none is lost.

### BW-04 — SaveSettings overwrote image object keys from client body
**Files:** `internal/db/queries/settings.sql`,
`internal/db/generated/settings.sql.go`, `internal/db/generated/querier.go`,
`internal/settings/service.go`
Added `UpdateReceiptTemplateContent` (text/compliance fields only — no image key
columns) and pointed `SaveSettings` at it. Image object keys are now owned solely
by the upload endpoint and are read-only on the settings PUT, so a "save all
tabs" body with a stale/omitted key can no longer null a freshly-uploaded asset.
RED test asserts persisted image keys survive a SaveSettings that omits them.

### BI-01 — DevSender in prod + world-readable PII PDFs
**Files:** `cmd/server/main.go`, `internal/mailer/dev_sender.go`
Added a startup guard (`mailDevEnabled`) that refuses to boot unless `MAIL_DEV=1`
is explicitly set (fail-fast — no real provider is wired yet). Tightened the dev
mail capture perms from `0755`/`0644` to `0700` dirs / `0600` files. Added a cheap
unit test for the guard.

### BI-04 — Code duplication across worker/settings/donation
**Files:** `internal/pdf/render.go`, `internal/receiptfmt/amount.go` (new),
`internal/worker/issue_receipt.go`, `internal/settings/service.go`,
`internal/donation/service.go`
Extracted `pdf.FetchTemplateImage` (over a new `pdf.ImageStore` seam) replacing
the verbatim-duplicated private `fetchTemplateImage` in worker + settings, and
`receiptfmt.FormatAmount` replacing `worker.formatAmount` + `donation.numericStr`.
Pure refactor; behavior identical; removed now-unused `mimetype` imports.

## Deferred

### BW-02 — Compliance config read at render, not snapshotted at approval
**Not auto-fixed — needs design + stakeholder decision.** The safe fix touches
the `Approve` issuance transaction (gap-less numbering + SoD + audit) plus a
schema change — too risky for an autonomous drive-by. Converted the in-code
"KNOWN LIMITATION" comment into a grep-able `TODO(BW-02)` and recorded it as a
tracked backlog item with suggested scope in
`.planning/phases/04-receipt-pdf-email-delivery-outbox-worker/04-BACKLOG.md`.
Code behavior unchanged (`72b9921`).

## Skipped

### BI-03 — At-least-once email duplication on transient post-send failure
Acknowledged, no change (by-design). Inherent to the transactional-outbox
pattern and harmless — the re-sent PDF is the identical frozen artifact.

### BI-05 — Reclaim not covered by the polling partial index; unbounded growth
Deferred (scale). Fine at hospital volume; no speculative migration added. Note
for a future scale/retention pass (partial index for `status='processing'` +
cleanup of `done` rows).

---

_Fixer: Claude (gsd-code-fixer) · Iteration 1 · backend scope_
