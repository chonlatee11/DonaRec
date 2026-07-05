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
