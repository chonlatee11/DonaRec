---
phase: 04-receipt-pdf-email-delivery-outbox-worker
type: review-fixes
reviewed_source: 04-REVIEW-PRESHIP.md
fix_scope: all (Critical + Warning + Info, per --fix --all)
status: complete
sub_reports: [04-REVIEW-FIX-backend.md, 04-REVIEW-FIX-frontend.md]
outcome:
  fixed: 13
  deferred: 1
  skipped: 2
  total: 16
verification:
  frontend: "npm test 44 passed, lint clean, next build succeeded"
  backend: "go build ./... OK, go vet clean, go test -short ./... all pass"
  caveats:
    - "RESOLVED 2026-07-05: ran sqlc generate v1.31.1 — hand-edited generated code did NOT byte-match; fixed by adding ::text casts to UpdateTemplateImageKey (@slot/@object_key) so sqlc infers string not interface{}/*string (had broken the build). Regenerated + committed (5945d3a). go build/vet/tests green."
    - "OPEN: Docker not running — DB-backed RED tests (BW-01/BW-03/BW-04) compile but were not executed against live Postgres; run `go test ./...` with Docker up"
---

# Phase 04 — Code Review Fixes (Consolidated)

Fixes for the 17 findings in `04-REVIEW-PRESHIP.md` (the pre-ship re-review of PR #4),
applied via `/gsd-code-review 04 --fix --all`. Two fixers ran sequentially in isolated
worktrees (backend then frontend — sequential because fixers commit, and FW-04 had to
match the backend's canonical fiscal-year rule established by BL-01). TDD RED→GREEN was
followed for every behavior-changing fix that had a practical test seam.

Per-finding detail is in `04-REVIEW-FIX-backend.md` and `04-REVIEW-FIX-frontend.md`.

## Outcome — 13 fixed, 1 deferred, 2 skipped

### 🔴 Blocker — fixed
- **BL-01** — receipt issue date now renders in `Asia/Bangkok` + Thai BE via a shared
  `receiptfmt.FormatIssueDate(t, lang)` called by BOTH the worker and the settings preview
  (preview == real). `test`→`fix` (`5d3d8e1`, `3acf55a`).
  **Human sign-off still required:** exact BE-vs-CE era + Thai month wording pends
  accounting/legal confirmation (CLAUDE.md compliance gate). Fix aligned to the existing
  preview fixture (`"15 มี.ค. 2569"`) as the codebase's current intent.

### 🟠 Warnings — fixed (6)
- **BW-01** panic+reclaim path now increments `attempts` + applies terminal CASE → deterministic
  panicker dead-letters instead of looping (`6e1ba91`, `15c0f43`).
- **BW-03** atomic single-column `UpdateTemplateImageKey` replaces the unlocked read-modify-write
  (`2dd79ac`, `cf2cbd1`).
- **BW-04** `SaveSettings` (via `UpdateReceiptTemplateContent`) makes image keys read-only on the
  PUT — a "save all" body can't null an uploaded asset (`8e07c3a`, `bc9c0ca`).
- **FW-01** live brand-image blob URLs revoked at unmount via a ref (`43e8585`).
- **FW-02** restrictive CSP `<meta>` injected into the sandboxed preview doc — blocks passive
  network while keeping `font-src 'self'` for TH Sarabun (`912648d`).
- **FW-03** receipt download uses a synthesized anchor `.click()` instead of popup-blocker-prone
  `window.open`-after-await (`791ddb0`).
- **FW-04** receipt-number example defaults to the Thai FISCAL BE year (Oct–Dec → CE+544,
  Jan–Sep → CE+543) matching backend `fiscal_year.go`; `test`→`fix` (`6da3163`, `6c6d7bc`).

### 🔵 Info — fixed (6)
- **BI-01** DevSender gated behind `MAIL_DEV=1` (fail-fast otherwise); dev PII files tightened
  to `0700`/`0600` (`823032e`).
- **BI-04** deduped `fetchTemplateImage` → `pdf.FetchTemplateImage`, `formatAmount`/`numericStr`
  → `receiptfmt.FormatAmount` (`2c2be1b`).
- **FI-01** upload tile click/keydown gated on `uploading` — no same-slot race (`3750710`).
- **FI-02** `createLatestGuard()` stale-response guard added to real-PDF preview (`cb95448`).
- **FI-03** preview-PII test retargeted at `buildPreviewRequest` (real client guard), false-
  confidence assertion removed (`fb38eaf`).

### Deferred (1)
- **BW-02** — snapshotting compliance config (deduction multiplier / §6 text / template) at
  approval touches the gap-less-numbering `Approve` transaction + a schema change. Too risky
  for an autonomous fix; **not implemented**. In-code comment converted to `TODO(BW-02)` and
  tracked in new `04-BACKLOG.md` for a dedicated phase.

### Skipped — no change (2)
- **BI-03** at-least-once email duplication — inherent to the outbox pattern, harmless (identical
  frozen PDF). Acknowledged, by-design.
- **BI-05** reclaim index / unbounded table growth — fine at hospital volume; no speculative
  migration added. Deferred (scale).

## Pre-merge verification checklist
- [x] `sqlc generate` reconciled — ran v1.31.1, fixed type inference via `::text` casts, regenerated + committed (`5945d3a`); build/vet/tests green
- [ ] `go test ./...` (Docker up) exercises the BW-01/BW-03/BW-04 DB-backed RED→GREEN tests
- [ ] Human/legal sign-off on BL-01 receipt date era/format (BE vs CE, month wording)
