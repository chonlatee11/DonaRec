# Phase 04 — Tracked Backlog

Deferred items surfaced by review that are intentionally NOT auto-fixed and
need their own planned phase (design + stakeholder decision + tests).

---

## BW-02 — Snapshot compliance config at approval (not at render)

**Source:** `04-REVIEW-PRESHIP.md` → BW-02 (Warning)
**Status:** deferred — needs design + stakeholder decision, not auto-fixed
**Code:** `donnarec-api/internal/worker/issue_receipt.go` (`renderReceiptPDF`, see the
`TODO(BW-02)` note)

### Problem

`deduction_multiplier`, `section6_text_*`, and `template_html` are read fresh
from `receipt_template_config` when the worker RENDERS the receipt, not frozen at
`Approve` time. An admin editing these in the window between approval (which
enqueues the outbox job) and the worker actually rendering it produces a receipt
that does not reflect what the checker approved — unlike every other frozen
field (receipt number, donor snapshot). The deduction multiplier is
compliance-critical.

### Why it is deferred (not auto-fixed)

The safe fix (snapshotting at least `deduction_multiplier` + the resolved §6 text
onto the donation row or the outbox job payload) requires changing the `Approve`
issuance transaction — the single most load-bearing, security/compliance-critical
path in the codebase (D-52: gap-less receipt numbering + SoD + audit, all inside
one advisory-locked tx) — plus a schema change. CLAUDE.md warns this path must be
changed with extreme care; a drive-by code-review fix risks a regression in the
one invariant the system cannot get wrong. It needs its own plan and tests
targeting `Approve`.

### Suggested scope for the future phase

1. Add snapshot columns (or an outbox-payload field) captured inside the
   `Approve` tx: at minimum `deduction_multiplier` + resolved §6 text (consider
   `template_html` too).
2. Have `renderReceiptPDF` read the snapshot instead of the live config.
3. Concurrency test: edit config in the approval→render window and assert the
   rendered receipt reflects the value in effect AT APPROVAL.

---

## BL-01 (follow-up) — Confirm exact Thai-RD-compliant receipt issue-date format

**Source:** `04-REVIEW-PRESHIP.md` → BL-01 (Blocker) — **code fixed & verified**; this is
the residual compliance sign-off, not a code bug.
**Status:** awaiting stakeholder (accounting/legal) — non-blocking for the code, blocking
for go-live correctness.
**Code:** `donnarec-api/internal/receiptfmt/date.go` (`FormatIssueDate`)

### What is already done (closed)

The timezone/era defect is fixed: `FormatIssueDate(t, lang)` converts `approved_at` to
`Asia/Bangkok` and renders Thai Buddhist-Era + Thai month abbrev for `th` (e.g.
`"15 มี.ค. 2569"`), Gregorian for `en`. Worker and settings-preview both call the one
helper, so preview == real receipt. RED→GREEN unit tests + full `go test ./...` (live
Postgres) pass. **Completeness-checked:** the receipt PDF `ReceiptData` carries only
`IssueDate` (no other donor-facing date); the remaining `dateStr` (`donation/service.go`)
is a pure `pgtype.Date` for back-office DTOs only — no timezone bug, not on the legal
artifact.

### What still needs a human decision

Confirm with hospital accounting/legal (per CLAUDE.md compliance gate) the EXACT format the
Thai Revenue Department requires on the tax receipt:
1. Era — **พ.ศ. (BE)** is assumed correct; confirm CE is not required.
2. Month — abbreviation (`มี.ค.`) vs full name (`มีนาคม`).
3. Day/month/year ordering and any leading-zero/separator convention.
4. English-locale receipts: confirm the `2 Jan 2006` Gregorian form is acceptable.

If the confirmed format differs, it is a one-function change in `receiptfmt/date.go` +
update the unit test expectations + the preview fixture — no schema/tx impact.
