---
phase: 04-receipt-pdf-email-delivery-outbox-worker
plan: 03
subsystem: infra
tags: [chromedp, headless-chromium, html-template, pdf, thai-shaping, xss, ssrf, golden-file-test]

# Dependency graph
requires:
  - phase: 04-receipt-pdf-email-delivery-outbox-worker
    provides: "04-02 chrome sidecar (docker/chrome.Dockerfile, chromedp v0.14.2 pin) + testutil.StartChrome testcontainers helper"
provides:
  - "internal/pdf/render.go: ReceiptData struct + Render (html/template contextual autoescaping) + DataURI/FontFaceCSS server-controlled asset helpers"
  - "internal/pdf/chromium.go: Renderer/NewRenderer/RenderPDF + renderInSandbox — the single code path wiring all three D-58 sandbox layers (network-isolated sidecar, CDP fetch-block, JS-disable)"
  - "internal/pdf/render_sandbox_security_test.go: permanent regression tests proving JS-disable, network-block, and donor-field autoescape"
  - "internal/pdf/render_golden_test.go + testdata/*.golden.png: byte-exact golden-file tests for Thai worst-case (stacked tone marks) + English receipts, plus §6/1x2x text extraction"
affects: [04-05, 04-07]

# Tech tracking
tech-stack:
  added:
    - "github.com/chromedp/chromedp + cdproto now DIRECT imports (promoted from // indirect in go.mod — internal/pdf is the first real caller)"
  patterns:
    - "renderInSandbox (unexported) is the single CDP action sequence both production RenderPDF and the security regression tests call, with different trailing actions (PrintToPDF vs Title/Evaluate) — proves the tests exercise the EXACT sandbox code production uses, not a reimplementation"
    - "ReceiptData's image/CSS fields (LetterheadData, SealData, SignatureData, WatermarkData, FontFaceCSS) are typed template.URL/template.CSS, not plain string — required so html/template treats server-assembled data: URIs as pre-vetted safe content instead of rejecting them as '#ZgotmplZ'"
    - "Exact-byte PNG golden-file comparison (pdftoppm -singlefile, no pixel-diff library) — verified deterministic across repeated runs in this environment, matching 04-RESEARCH.md's live-spike finding"

key-files:
  created:
    - donnarec-api/internal/pdf/render.go
    - donnarec-api/internal/pdf/chromium.go
    - donnarec-api/internal/pdf/render_sandbox_security_test.go
    - donnarec-api/internal/pdf/render_golden_test.go
    - donnarec-api/internal/pdf/testdata/thai_worst_case.golden.png
    - donnarec-api/internal/pdf/testdata/english.golden.png
  modified:
    - donnarec-api/go.mod
    - donnarec-api/go.sum

key-decisions:
  - "ReceiptData field names (DonorName, ReceiptNo, Amount, IssueDate, Section6Text, DeductionMultiplier, Language, LetterheadData, SealData, SignatureData, WatermarkData, FontFaceCSS) match the placeholders already used by the 04-01-seeded receipt_template_config.template_html/template_html_en, so the worker (04-05) can pass this struct straight into Render() against the existing DB-seeded templates without a schema/placeholder mismatch"
  - "Image/font-asset ReceiptData fields are typed template.URL/template.CSS (verified via a standalone spike: plain string fields get rejected by html/template's URL/CSS sanitizer and render as '#ZgotmplZ' / 'ZgotmplZ') — safe ONLY because DataURI/FontFaceCSS assemble them from server-controlled bytes, never donor or admin free-text"
  - "Golden-file fixtures use their own self-contained template (not the migration-seeded one) so the test suite exercises the full ReceiptData surface (including SealData and FontFaceCSS, which the current DB seed template does not yet reference) — the DB seed and the render engine's field contract are independently free to diverge; only the field NAMES need to match, not every seed template's markup"
  - "FontFaceCSS is exercised via a lightweight unit test (TestDataURIAndFontFaceCSS) but NOT used to embed real font bytes in the golden fixtures — TH Sarabun New is not yet sourced (assets/fonts/README.md, Assumption A3), so golden fixtures rely on the fonts-thai-tlwg fallback (Waree) already baked into docker/chrome.Dockerfile; this is the same font any current production render would actually use today"
  - "assertContainsExtractedText strips ALL whitespace (not just collapses runs) from both extracted pdftotext output and the wanted substring before comparison — poppler hard-wraps Thai text (no inter-word spaces) mid-word with a bare newline and no inserted space, so single-space normalization was insufficient; full whitespace-stripping is the robust fix for both languages"

requirements-completed: [FR-20, FR-21, FR-22, FR-24, FR-23]

coverage:
  - id: D1
    description: "internal/pdf/chromium.go: Renderer/RenderPDF renders self-contained HTML to PDF via a sandboxed remote Chromium with JavaScript disabled and all outbound network requests failed"
    requirement: "FR-20"
    verification:
      - kind: integration
        ref: "go test ./internal/pdf/... -run TestRenderSandboxSecurity_JSDisabled (pass); -run TestRenderSandboxSecurity_NetworkBlocked (pass)"
        status: pass
    human_judgment: false
  - id: D2
    description: "internal/pdf/render.go: Render assembles admin template HTML + ReceiptData via html/template contextual autoescaping — donor-supplied markup cannot break out of the admin's HTML structure"
    requirement: "FR-24"
    verification:
      - kind: unit
        ref: "go test ./internal/pdf/... -run TestRenderSandboxSecurity_DonorFieldEscaped (pass)"
        status: pass
    human_judgment: false
  - id: D3
    description: "Thai worst-case fixture (stacked tone marks ก๊วยเตี๋ยว/ปั๊ม/ตั้งชื่อ + Latin-leading mixed text) renders correctly through the full Render->RenderPDF pipeline, proven by an exact-byte golden PNG"
    requirement: "FR-23"
    verification:
      - kind: integration
        ref: "go test ./internal/pdf/... -run TestRenderGolden_ThaiWorstCase (pass); testdata/thai_worst_case.golden.png committed and visually inspected — no tofu boxes, tone marks stack correctly"
        status: pass
    human_judgment: false
  - id: D4
    description: "English fixture renders correctly; §6 tax-deduction text (with 1x/2x statement) is present in the rendered PDF, extracted via pdftotext"
    requirement: "FR-24"
    verification:
      - kind: integration
        ref: "go test ./internal/pdf/... -run TestRenderGolden_English (pass); testdata/english.golden.png committed"
        status: pass
    human_judgment: false
  - id: D5
    description: "letterhead/seal/signature/watermark images and TH-Sarabun-New @font-face CSS are embeddable as base64 data: URIs assembled server-side (DataURI/FontFaceCSS helpers), never fetched by Chromium"
    verification:
      - kind: unit
        ref: "go test ./internal/pdf/... -run TestDataURIAndFontFaceCSS (pass)"
        status: pass
    human_judgment: false

# Metrics
duration: ~18min
completed: 2026-07-04
status: complete
---

# Phase 4 Plan 3: Sandboxed Thai/English Receipt PDF Renderer Summary

**internal/pdf renders admin-templated, donor-safe Thai/English receipt PDFs via chromedp against a sandboxed chrome sidecar with JavaScript disabled and network blocked — proven with byte-exact golden PNGs for a Thai stacked-tone-mark worst case and a live SSRF/XSS regression test**

## Performance

- **Duration:** ~18 min
- **Started:** 2026-07-04T14:41:52+07:00 (RED commit)
- **Completed:** 2026-07-04T14:51:06+07:00 (GREEN commit)
- **Tasks:** 1 (RED + GREEN, per plan's single TDD task)
- **Files modified:** 7 (6 created, 1 modified go.mod/go.sum pair counted once above but tracked separately below)

## Accomplishments
- `internal/pdf/render.go`: `ReceiptData` struct + `Render(templateHTML, data)` executing the admin's stored template via Go's stdlib `html/template` — contextual autoescaping neutralizes donor-field injection (T-04-09) without ever wrapping the admin template as trusted raw HTML; `DataURI`/`FontFaceCSS` helpers assemble server-controlled base64 image/font assets
- `internal/pdf/chromium.go`: `Renderer`/`NewRenderer`/`RenderPDF`, backed by the unexported `renderInSandbox` — the single CDP action sequence wiring all three D-58 mitigation layers (network-isolated `chrome` sidecar from 04-02, `fetch.Enable`+`FailRequest`-all, `emulation.SetScriptExecutionDisabled(true)`) that both production rendering and the security regression tests go through
- `render_sandbox_security_test.go`: three permanent regression tests, all passing live against the real chrome sidecar — inline `<script>` never executes (`document.title` stays empty), an external image probe never loads (`naturalWidth` stays 0, render still succeeds), and donor-supplied markup is HTML-entity-escaped in the assembled HTML
- `render_golden_test.go` + committed goldens: a Thai worst-case fixture (`iPhone ก๊วยเตี๋ยว ปั๊ม ตั้งชื่อ` — Latin-leading mixed with stacked tone marks) and an English fixture both render correctly (visually verified: no tofu boxes, tone marks stack properly) and byte-exact match on rerun, proving 04-RESEARCH.md's determinism claim holds in this environment too; §6 + 1x/2x text is extracted from the rendered PDF via `pdftotext` and asserted present
- `go.mod`/`go.sum`: `chromedp`/`cdproto` promoted from `// indirect` to direct requirements now that `internal/pdf` is their first real importer

## Task Commits

Each task was committed atomically (RED then GREEN, per this plan's single `tdd="true"` task):

1. **Task 1 RED: failing sandbox security + golden-file tests** - `b2fddbf` (test) — compile-fails against not-yet-implemented `ReceiptData`/`Render`/`NewRenderer`/`RenderPDF`/`renderInSandbox`
2. **Task 1 GREEN: render.go + chromium.go implementation, goldens generated** - `f8e016c` (feat) — all 6 tests pass; goldens committed and verified byte-identical on rerun

**Plan metadata:** (this commit, docs: complete plan)

## Files Created/Modified
- `donnarec-api/internal/pdf/render.go` - `ReceiptData`, `Render`, `DataURI`, `FontFaceCSS`
- `donnarec-api/internal/pdf/chromium.go` - `Renderer`, `NewRenderer`, `RenderPDF`, `renderInSandbox`
- `donnarec-api/internal/pdf/render_sandbox_security_test.go` - JS-disable / network-block / autoescape regression tests + a `DataURI`/`FontFaceCSS` unit test
- `donnarec-api/internal/pdf/render_golden_test.go` - Thai worst-case + English golden-PNG tests, §6-text extraction, `solidPNG` fixture-asset generator
- `donnarec-api/internal/pdf/testdata/thai_worst_case.golden.png` / `english.golden.png` - committed golden fixtures
- `donnarec-api/go.mod` / `go.sum` - chromedp/cdproto promoted to direct requirements

## Decisions Made
- `ReceiptData` field names deliberately match the placeholders already present in the 04-01-seeded `receipt_template_config.template_html`/`template_html_en` rows, so the worker (04-05) can render the current DB-seeded templates with zero placeholder mismatch.
- Image/font `ReceiptData` fields are typed `template.URL`/`template.CSS` rather than plain `string` — confirmed via a standalone Go spike that plain-string data: URIs get sanitized into `#ZgotmplZ`/`ZgotmplZ` by `html/template`'s content-type filters. This typing is safe only because `DataURI`/`FontFaceCSS` build these values from server-controlled bytes, never donor or admin free-text.
- Golden-file fixtures use a locally-defined template (not the DB-seeded one) so the test suite exercises the full `ReceiptData` field surface, including `SealData` and `FontFaceCSS`, which the current seed template doesn't yet reference — the DB seed template and the render engine's Go-level field contract are independently free to evolve; only the field *names* need to stay aligned.
- Golden fixtures deliberately do NOT embed real font bytes via `FontFaceCSS` — TH Sarabun New is not yet sourced (tracked in `assets/fonts/README.md`, Assumption A3) — and instead rely on the `fonts-thai-tlwg` fallback (`Waree`) already baked into `docker/chrome.Dockerfile`, matching what any current production render would actually use today. `FontFaceCSS`'s output format is still verified by a dedicated unit test.
- `assertContainsExtractedText` strips all whitespace (not just single-space-collapses) from both `pdftotext` output and the wanted substring before comparison, because poppler hard-wraps unspaced Thai text mid-word with a bare newline (no inserted space) — single-space normalization alone failed the Thai fixture during GREEN implementation; this is now robust for both languages.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] §6-text assertion needed whitespace-normalization for Thai line-wraps**
- **Found during:** Task 1 GREEN, first `-update` run of `TestRenderGolden_ThaiWorstCase`
- **Issue:** `pdftotext` hard-wraps the Section6Text fixture at the page width; for the Thai fixture (no inter-word spaces) the wrap landed mid-word with a bare newline and no inserted space, so a naive `require.Contains` (and even a single-space-collapse normalization) failed even though the text was correctly rendered and extracted.
- **Fix:** `assertContainsExtractedText` now strips ALL whitespace from both the extracted text and the wanted substring before comparing.
- **Files modified:** `internal/pdf/render_golden_test.go`
- **Verification:** Both `TestRenderGolden_ThaiWorstCase` and `TestRenderGolden_English` pass; visually confirmed the §6 text renders correctly and completely in both golden PNGs.
- **Committed in:** `f8e016c` (GREEN commit)

---

**Total deviations:** 1 auto-fixed (test-assertion bug, not a production-code or security issue)
**Impact on plan:** No scope creep — this is a test-harness correctness fix uncovered while proving the plan's own acceptance criterion (§6 text extraction) actually holds.

## Issues Encountered
None beyond the deviation above. Docker and `poppler-utils` (`pdftoppm`/`pdftotext`) were both already available in this environment, and the `chromedp/headless-shell`-based chrome sidecar image from 04-02 was already built locally, so all six tests (three security regressions + two golden-file + one unit test) were run live against the real sandboxed Chromium rather than merely reviewed for correctness.

## User Setup Required

None - no external service configuration required. The chrome sidecar (docker-compose `chrome` service, 04-02) is the only runtime dependency, and it is already wired.

## Next Phase Readiness

- 04-05 (outbox worker) can call `pdf.Render` (against the donation's `donor_language`-selected template + settings-store data) and `pdf.NewRenderer(cfg.Worker.ChromeWSURL).RenderPDF(ctx, html)` to freeze the receipt PDF (D-56) — the `ReceiptData` field contract already matches the 04-01-seeded `receipt_template_config` templates.
- 04-07 (admin settings UI + live preview) can reuse the same `Render`/`RenderPDF` pipeline for the "real PDF" accurate-preview path (D-61), and `DataURI`/`FontFaceCSS` for assembling preview HTML from uploaded template images.
- Tracked, non-blocking open item (carried from 04-02): TH Sarabun New must still be sourced by the team (`assets/fonts/README.md`) before production receipts use the correct typography; the render pipeline itself is fully functional today with the `fonts-thai-tlwg` fallback font.
- No blockers.

## Self-Check: PASSED

Verified files exist on disk: `donnarec-api/internal/pdf/render.go`, `donnarec-api/internal/pdf/chromium.go`, `donnarec-api/internal/pdf/render_sandbox_security_test.go`, `donnarec-api/internal/pdf/render_golden_test.go`, `donnarec-api/internal/pdf/testdata/thai_worst_case.golden.png`, `donnarec-api/internal/pdf/testdata/english.golden.png` (all FOUND). Verified commit hashes `b2fddbf` and `f8e016c` present in `git log --oneline --all` (both FOUND).

---
*Phase: 04-receipt-pdf-email-delivery-outbox-worker*
*Completed: 2026-07-04*
