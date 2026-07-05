---
phase: 04-receipt-pdf-email-delivery-outbox-worker
reviewed: 2026-07-05T00:00:00Z
depth: standard
kind: pre-ship re-review (after 04-REVIEW.md fixes; supersedes nothing — additive)
supersedes: none
related: 04-REVIEW.md, 04-REVIEW-FIXES-SUMMARY.md
components: [backend (Go+SQL), frontend (Next.js 15 + TS)]
files_reviewed: 58
findings:
  blocker: 1
  critical: 1
  warning: 8
  info: 8
  total: 17
status: fixed
resolution: "13 fixed, 1 deferred (BW-02 → 04-BACKLOG.md), 2 skipped (by-design/scale). See 04-REVIEW-FIX.md."
---

> **Resolved 2026-07-05** via `/gsd-code-review 04 --fix --all` — 13/17 findings fixed
> (incl. blocker BL-01), BW-02 deferred to `04-BACKLOG.md`, BI-03/BI-05 skipped. Fix
> details + pre-merge caveats in `04-REVIEW-FIX.md`.

# Phase 04 — Pre-Ship Code Review (Consolidated)

**Trigger:** `/gsd-code-review 04` after ship (PR #4), a re-review of the current
post-fix code. Reviewed in two parallel passes — backend (Go+SQL, 31 files) and
frontend (Next.js/TS, 27 files). The earlier `04-REVIEW.md` (2026-07-04) and its
applied fixes (`04-REVIEW-FIXES-SUMMARY.md`, CR-01/CR-02 + WR-01..07) remain the
historical record; finding IDs here are namespaced (`BL-`/`BW-`/`BI-` backend,
`FW-`/`FI-` frontend) and do **not** map to that document's IDs.

## Verdict

**1 blocker** must be resolved before merge. It is a correctness defect on the
legal receipt artifact itself (wrong date). Everything the phase exists to protect
is intact — gap-less numbering, freeze-idempotency (no re-number/re-render on
resend), the 3-layer PDF sandbox, RBAC OR-semantics (no repeat of the Phase-3
AND bug), SoD, PII masking, in-tx audit, and up/down-symmetric migrations. No SQL
injection or auth-bypass surface was found.

| Severity | Count |
|----------|-------|
| Blocker  | 1 |
| Warning  | 8 |
| Info     | 8 |
| **Total**| **17** |

---

## 🔴 Blocker

### BL-01 — Receipt issue date rendered in UTC + ISO-Gregorian, not Asia/Bangkok / Thai BE
**File:** `donnarec-api/internal/worker/issue_receipt.go:402-407` (`formatIssueDate`), used at `:243`

`approved_at` is stored as `time.Now().UTC()` (`service.go:559`) and pgx returns
`timestamptz` as a UTC `time.Time`, so `formatIssueDate` prints the **UTC calendar
date**. Any donation approved during Bangkok 00:00–07:00 (~29% of the day, UTC+7)
prints the **previous day's date** on a legally-binding tax receipt — and at a
year boundary this shifts the receipt into the wrong tax year. The value is also
ISO-Gregorian (`2026-06-01`), while the admin preview fixture shows a Thai BE date
(`15 มี.ค. 2569`), so the preview misrepresents real output. CLAUDE.md makes
Asia/Bangkok + BE fiscal-year handling load-bearing.

**Fix:** Convert to `Asia/Bangkok` and render in the era/locale the compliance
template requires (BE + Thai month names when `lang=="th"`). Confirm BE-vs-CE with
the accounting/legal stakeholder gate. This should also reconcile the preview
fixture so preview == real output.

---

## 🟠 Warnings

### BW-01 — Panic path defeats the "truly terminal dead-letter" invariant
**File:** `internal/worker/worker.go:243-271` (`ProcessOnceSafe`), `internal/db/queries/outbox.sql:69-73` (`ReclaimStuckOutboxJobs`)

`ProcessOnceSafe` recovers a panic and leaves the job `processing` **without
incrementing `attempts`**; `ReclaimStuckOutboxJobs` resets it to `pending`, also
without incrementing. A deterministically-panicking job is reclaimed and retried
forever and **never dead-letters** — contradicting the bounded-retry design claim.
Returned-error failures are bounded correctly; only the panic+reclaim path is
unbounded. **Fix:** increment `attempts` (and apply the terminal CASE) on reclaim.

### BW-02 — Compliance config read fresh at render, not snapshotted at approval
**File:** `internal/worker/issue_receipt.go:208-219` (self-documented "KNOWN LIMITATION")

`deduction_multiplier` / `section6_text_*` / `template_html` are read when the
worker renders, not frozen at `Approve`. An admin editing them in the window
between approval and render produces a receipt that doesn't reflect what the
checker approved — unlike every other frozen field. Deduction multiplier is
compliance-critical. **Fix:** snapshot at least `deduction_multiplier` + resolved
§6 text onto the donation/outbox payload inside the `Approve` tx; if deferred,
make it a tracked backlog item, not a code comment.

### BW-03 — `SaveTemplateImage` read-modify-write of the config row with no tx/lock
**File:** `internal/settings/service.go:198-241`

Reads the whole config, mutates one slot in memory, writes the whole row back —
no transaction or row lock. Two near-simultaneous slot uploads (or a `SaveSettings`
PUT interleaved) race; the later writer clobbers the other slot's fresh
`*_object_key`. `SaveSettings` was fixed to `WithTx` (WR-07) but this sibling was
not. **Fix:** `WithTx` + `SELECT … FOR UPDATE`, or a single-column
`UpdateTemplateImageKey` query.

### BW-04 — Full-settings PUT overwrites image object keys from the client body
**File:** `internal/settings/service.go:161-174`, `model.go:43-46`

`SaveSettings` writes image object keys straight from the request body. Since
uploads persist keys out-of-band, a "Save all tabs" PUT carrying a stale/omitted
key silently nulls/reverts a just-uploaded asset — the server-side half of BW-03,
undefended server-side. **Fix:** exclude image keys from the `SaveSettings` write
path (owned solely by the upload endpoint), or validate echoed keys against the
persisted row.

### FW-01 — Unmount cleanup revokes the wrong (initial) object URLs — memory leak
**File:** `donnarec-web/components/SettingsTabs.tsx:112-119`

Cleanup effect has an empty dep array, so its closure captures `localPreviewUrls`
from the first render (all `null`) and revokes nothing at unmount. Live brand-image
blob URLs from the session leak. **Fix:** track URLs in a ref the cleanup reads at
unmount.

### FW-02 — Preview iframe blocks scripts but not passive network ("no network" unmet)
**File:** `donnarec-web/components/TemplateEditor.tsx:307-312`

`sandbox="allow-same-origin"` (no `allow-scripts`) correctly disables JS — the
primary XSS vector — but `<img>`/`<link>`/CSS `url()` can still issue GETs, and
`allow-same-origin` means same-origin sub-requests carry the admin's cookies. The
D-58 "no network" half is unenforced. Low risk (no scripts, Admin-authored), but
should add a restrictive CSP `<meta>` to the sandboxed document (keep `font-src
'self' data:` so THSarabun still loads).

### FW-03 — `window.open` after an await is popup-blocker-prone (silent download failure)
**File:** `donnarec-web/components/DonationDetailView.tsx:117-125`

`window.open` runs in the mutation `onSuccess` after the `await`, outside the
click's user-gesture stack — popup blockers commonly suppress it, so the receipt
PDF silently fails to open. **Fix:** synthesized anchor `.click()` (with
`rel="noopener noreferrer"`), or open the window synchronously on click.

### FW-04 — Receipt-number example uses calendar BE year, not Thai fiscal year
**File:** `donnarec-web/lib/receipt-number-format.ts:35` (used by `NumberFormatEditor.tsx:56-61`)

Defaults year to `getFullYear()+543` (calendar BE). The counter is keyed per Thai
fiscal year (Oct 1 start), so during Oct–Dec the live example shows a year one
behind what the backend freezes. Display-only, but on the most correctness-
sensitive field. **Fix:** derive the fiscal BE year the same way the backend does
(month ≥ Oct → +1); verify against `format.go` and document. **Note:** this is the
same fiscal-year/timezone theme as BL-01 — resolve them together.

---

## 🔵 Info

### BI-01 — DevSender is the only wired EmailSender; donor-PII PDFs written world-readable to /tmp
**File:** `cmd/server/main.go:182-186`, `internal/mailer/dev_sender.go:28-45`
By-design for the MVP provider gate (D-60), but no guard prevents DevSender in prod
and the on-disk PII is unencrypted (mode `0644`). Add a startup guard (require
`MAIL_DEV=1`) and tighten perms before go-live.

### BI-02 — Preview endpoint returns raw admin HTML for client-side rendering
**File:** `internal/settings/handler.go:234-274`
`/settings/preview` returns admin HTML verbatim; the sandbox is only in the PDF
pipeline. Relies on the FE rendering it in a `sandbox` iframe (it does — see FW-02).
Admin-only; low severity.

### BI-03 — At-least-once delivery can duplicate a receipt email on transient post-send failure
**File:** `internal/worker/issue_receipt.go:127-160`, `worker.go:237-240`
If `Send` succeeds but `InsertEmailDelivery('sent')`/`MarkOutboxJobDone` fails, the
frozen PDF is emailed again on retry. Inherent to the outbox pattern, generally
harmless (identical PDF). Recording delivery in the same tx as the done-mark would
narrow the window.

### BI-04 — Code duplication across worker/settings
**File:** `worker/issue_receipt.go:275-285,370-396` vs `settings/service.go:292-302`, `donation/service.go:1616-1645`
`fetchTemplateImage` duplicated verbatim; `formatAmount` duplicates `numericStr`.
Extract a shared helper to prevent drift.

### BI-05 — `ReclaimStuckOutboxJobs` not covered by the polling partial index; table grows unbounded
**File:** `migrations/000007_outbox_jobs.up.sql:38-39`, `outbox.sql:69-73`
`idx_outbox_jobs_pending` is `WHERE status IN ('pending','failed')`; reclaim filters
`status='processing'` → sequential scan each tick. Fine at hospital volume; no
cleanup/DELETE of `done` rows either. Note for scale.

### FI-01 — Concurrent image uploads to the same slot can race
**File:** `donnarec-web/components/SettingsTabs.tsx:146-167`, `ImageUploadSlot.tsx:112`
The tile's `onClick` checks `disabled` but not `uploading`, so a second upload can
start; the two in-flight mutations resolve last-wins and the first `finally` clears
`uploadingSlot` early. **Fix:** gate the click on `uploading` too. (FE mirror of
BW-03.)

### FI-02 — Real-PDF preview render has no stale-response guard
**File:** `donnarec-web/components/TemplateEditor.tsx:213-228`
Unlike the HTML preview (`createLatestGuard`), `handleRenderRealPdf` has no ordering
guard; rapid clicks can display an older render. **Fix:** reuse `createLatestGuard()`.

### FI-03 — Preview-PII test asserts client payload shape, not BFF enforcement
**File:** `donnarec-web/app/api/bff/settings/__tests__/bff-routes.test.ts:149-165`
The "never sends donor field on preview (D-61)" test builds a payload already
omitting PII, then asserts the pass-through proxy omits it — it would pass even if a
caller *did* include PII. Mild false confidence. **Fix:** retarget at
`buildPreviewRequest`, or send a PII-bearing body and document the BFF as an
intentional transparent proxy (guarantee enforced upstream).

---

## Correct-by-construction (verified, do not re-flag)
- RBAC uses `RequireAnyRole` (OR) on donation/checker groups; `RequireRoles(Admin)`
  is single-role — **no AND-vs-OR regression**. Resend checkerGroup-gated; download
  all-staff. BFF attaches the Keycloak bearer server-side via `getServerSession`;
  token never reaches the browser; no client secret referenced.
- Worker never imports `receiptno` and never re-renders a frozen receipt (D-56);
  `ClaimNextOutboxJob` is a single atomic `FOR UPDATE SKIP LOCKED`; `pending`-only
  claim prevents dead-letter resurrection.
- PDF sandbox: internal-only chrome network + fetch-block-all + JS-disabled +
  `SetDocumentContent`; regression tests assert JS-disabled + network-blocked
  against a real sidecar. `html/template` autoescaping protects donor fields.
- Preview iframe disables JavaScript (no `allow-scripts`) — primary XSS vector shut.
  `TemplateEditor` HTML-preview stale-response guard present and correct.
- Migrations 000008–000012 up/down symmetric; CHECK constraints/indexes correct.
  All queries parameterized (incl. `||`-built audit resource strings) — no injection.

---

_Reviewers: Claude (gsd-code-reviewer ×2, backend + frontend) · Depth: standard · Pre-ship re-review of PR #4_
