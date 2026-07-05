---
phase: 04-receipt-pdf-email-delivery-outbox-worker
fixed_at: 2026-07-05T00:00:00Z
review_path: .planning/phases/04-receipt-pdf-email-delivery-outbox-worker/04-REVIEW-PRESHIP.md
iteration: 1
scope: frontend-only (FW-*, FI-*)
findings_in_scope: 7
fixed: 7
skipped: 0
status: all_fixed
---

# Phase 04 — Frontend Code Review Fix Report

**Fixed at:** 2026-07-05
**Source review:** 04-REVIEW-PRESHIP.md
**Iteration:** 1
**Scope:** Frontend only (`donnarec-web/`, Next.js 15 + TS/React). Backend
findings (BL-*, BW-*, BI-*) landed separately by the backend fixer — untouched.
**TDD mode:** ON. No DOM/testing-library or jsdom is wired in this repo
(`vitest` runs `environment: "node"`), so DOM/component-level fixes are
`fix(04)` commits verified via `next build` typecheck + `next lint`; the two
findings with a node-testable seam (FW-04, FI-03) got test-first / retargeted
tests.

**Summary:**
- Findings in scope: 7
- Fixed: 7
- Skipped: 0

**Verification (final state, run in an isolated worktree):**
- `npm test` — 7 files, **44 passed**
- `npm run lint` — **No ESLint warnings or errors**
- `npm run build` — **succeeded** (all routes compiled)

## Fixed Issues

### FW-04: Receipt-number example used calendar BE year, not Thai fiscal year
**Files modified:** `donnarec-web/lib/receipt-number-format.ts`, `donnarec-web/lib/__tests__/receipt-number-format.test.ts`
**Commits:** `6da3163` (RED test), `6c6d7bc` (GREEN fix)
**Applied fix:** Added `currentFiscalYearBE()` implementing the Oct-rollover
rule (Oct–Dec CE year Y → BE Y+544; Jan–Sep → Y+543) and defaulted
`fiscalYear` to it instead of the flat `getFullYear()+543`. This matches the
backend canonical rule in `donnarec-api/internal/receiptno/fiscal_year.go`
exactly (`month >= time.October → ceYear+544`). `getMonth()` is 0-indexed, so
October is index 9. TDD: added fake-timer tests for an Oct–Dec date
(expect +544) and a Jan–Sep date (expect +543); the Oct case failed RED
(yielded 2568) then passed GREEN (2569). Display-only; browser local time is
acceptable for the example — matching the rule is what matters.

### FW-01: Unmount cleanup revoked the wrong (initial) object URLs — memory leak
**Files modified:** `donnarec-web/components/SettingsTabs.tsx`
**Commit:** `43e8585`
**Applied fix:** Added a `localPreviewUrlsRef` that is assigned the latest
`localPreviewUrls` on every render; the empty-dep unmount cleanup now revokes
`localPreviewUrlsRef.current` so live brand-image blob URLs are actually
released (the old closure captured the first-render all-null snapshot).

### FW-02: Preview iframe blocked scripts but not passive network
**Files modified:** `donnarec-web/components/TemplateEditor.tsx`
**Commit:** `912648d`
**Applied fix:** Inject a CSP `<meta http-equiv="Content-Security-Policy">`
(`default-src 'none'; img-src data:; style-src 'unsafe-inline' 'self';
font-src 'self' data:`) at the top of the sandboxed preview `<head>`, before
the font-face `<style>`. Passive `<img>`/`<link>`/CSS `url()` GETs are blocked
(closing the D-58 "no network" half) while local TH Sarabun `/fonts/*.woff2`
still resolves via `font-src 'self'`. `allow-same-origin` retained (needed for
the local font).

### FW-03: `window.open` after an await was popup-blocker-prone
**Files modified:** `donnarec-web/components/DonationDetailView.tsx`
**Commit:** `791ddb0`
**Applied fix:** Replaced `window.open(url, "_blank", ...)` in the
`downloadMutation` `onSuccess` with a synthesized anchor click
(`document.createElement("a")` + `href` + `target="_blank"` +
`rel="noopener noreferrer"` + append + `.click()` + `remove()`), which
browsers treat as navigation rather than a popup, so the receipt PDF is not
silently suppressed.

### FI-01: Concurrent same-slot image upload race
**Files modified:** `donnarec-web/components/ImageUploadSlot.tsx`
**Commit:** `3750710`
**Applied fix:** Gated the upload tile's `onClick`, `onKeyDown`, and
`tabIndex` on `uploading` in addition to `disabled` (and set `aria-disabled`),
so a second file cannot be selected for a slot while its upload is in flight
(the tile is driven by `uploading={uploadingSlot === slot}`). FE mirror of
BW-03.

### FI-02: Real-PDF preview render had no stale-response guard
**Files modified:** `donnarec-web/components/TemplateEditor.tsx`
**Commit:** `cb95448`
**Applied fix:** Added a `pdfGuardRef = useRef(createLatestGuard())` and wrapped
`handleRenderRealPdf` with the same latest-guard pattern the HTML preview uses:
capture a `requestId` before the fetch and ignore the resolved blob (and its
error/loading state updates) when `isCurrent(requestId)` is false, so a stale
render can't overwrite a newer one.

### FI-03: Preview-PII test asserted client payload shape, not enforcement
**Files modified:** `donnarec-web/app/api/bff/settings/__tests__/bff-routes.test.ts`
**Commit:** `fb38eaf`
**Applied fix:** Removed the false-confidence assertion (which built a
PII-free payload then asserted the transparent proxy kept it PII-free) and
retargeted the guarantee at the real client-side guard: assert that
`buildPreviewRequest`, given a settings object polluted with
`donation_id`/`national_id`/`donor_name`, emits none of them and only the
whitelisted `PreviewRequest` contract keys. Documented the BFF
`/settings/preview` route as an intentional transparent proxy whose D-61
guarantee is enforced upstream.

## Skipped Issues

None — all 7 in-scope findings were fixed.

---

_Fixed: 2026-07-05_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 1_
