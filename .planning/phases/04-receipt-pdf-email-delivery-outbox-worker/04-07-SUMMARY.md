---
phase: 04-receipt-pdf-email-delivery-outbox-worker
plan: 07
subsystem: api
tags: [go, gin, html-template, sqlc, chromedp, rbac, audit, magic-byte]

# Dependency graph
requires:
  - phase: 04-receipt-pdf-email-delivery-outbox-worker
    plan: 01
    provides: "receipt_template_config table + GetReceiptTemplateConfig/UpdateReceiptTemplateConfig, receipt_number_config (Phase 2)"
  - phase: 04-receipt-pdf-email-delivery-outbox-worker
    plan: 03
    provides: "internal/pdf.Render (html/template autoescaping) + pdf.NewRenderer/RenderPDF (sandboxed chrome sidecar) + pdf.ReceiptData/DataURI"
  - phase: 04-receipt-pdf-email-delivery-outbox-worker
    plan: 06
    provides: "donation/handler.go Pattern A/C/D conventions reused verbatim for settings.Handler"
provides:
  - "internal/settings package: SettingsService (Get/Save/SaveTemplateImage/BuildPreviewHTML) + Handler (Get/Save/UploadImage/Preview/PreviewPDF)"
  - "Admin API: GET/PUT /api/admin/settings, POST /api/admin/settings/images/:slot, POST /api/admin/settings/preview, POST /api/admin/settings/preview/pdf"
  - "UpdateReceiptNumberConfig sqlc query — the Phase 2 number-format config now has a save path (previously read-only)"
  - "storage.PutTemplateImage/ValidateTemplateImage + ErrUnsupportedTemplateImageType/ErrTemplateImageTooLarge (2 MB, image/jpeg+png only)"
  - "adminGroup now runs auth.ResolveAppUser (previously only donationGroup did) so admin mutations carry a resolved users.id for updated_by/audit"
affects: [04-08]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "SettingsService.ReceiptsStore is a narrow interface (GetObject + PutTemplateImage) satisfied by *storage.StorageClient in production and a hermetic in-memory fake in tests — mirrors donation.ReceiptsStore's seam from plan 04-06"
    - "settings.Handler.PDFRenderer is an interface wrapping pdf.Renderer.RenderPDF so preview/pdf can be tested against a real chrome sidecar without a concrete-type test dependency"
    - "Read-modify-write for single-field image-slot updates: SaveTemplateImage re-reads the current template config row, overlays just the changed object key, then calls the same UpdateReceiptTemplateConfig used by the full Save — no separate per-column sqlc queries needed"

key-files:
  created:
    - donnarec-api/internal/settings/model.go
    - donnarec-api/internal/settings/service.go
    - donnarec-api/internal/settings/service_test.go
    - donnarec-api/internal/settings/handler.go
    - donnarec-api/internal/storage/template_image_test.go
  modified:
    - donnarec-api/internal/storage/client.go
    - donnarec-api/internal/db/queries/receiptno.sql
    - donnarec-api/internal/db/generated/receiptno.sql.go
    - donnarec-api/internal/db/generated/querier.go
    - donnarec-api/cmd/server/main.go
    - donnarec-api/cmd/server/e2e_test.go

key-decisions:
  - "[Rule 2] Added UpdateReceiptNumberConfig to internal/db/queries/receiptno.sql — the plan's must_haves truth requires an Admin to SAVE the number-format tab (separator/padding/year-format/prefix), but only a Get query existed before this plan (Phase 2 built read-only). Without this, SaveSettings could not persist half of what the plan calls a single-request 'save all tabs' operation."
  - "[Rule 1] storage.PutTemplateImage uses its OWN sentinel errors (ErrUnsupportedTemplateImageType, ErrTemplateImageTooLarge) rather than reusing the existing slip sentinels (ErrUnsupportedFileType/ErrFileTooLarge) — those existing error messages literally say '10 MB' and 'application/pdf allowed', both factually wrong for the 2 MB image-only template-asset cap. Reusing them would have shipped a misleading error message."
  - "[Rule 2] Added a dedicated image-upload endpoint (POST /api/admin/settings/images/:slot) not explicitly named in the plan's Task 2 action text — the plan's own must_haves truth ('brand image uploads ... are magic-byte validated ... and Admin-only') and Task 1's PutTemplateImage would otherwise be unreachable dead code. Uploads persist their object key immediately (read-modify-write), independent of the 'save all tabs' PUT, matching 04-UI-SPEC.md's per-slot upload-then-thumbnail-reflects behavior."
  - "adminGroup (cmd/server/main.go) now also runs auth.ResolveAppUser, mirroring donationGroup's wiring — settings Save/UploadImage need the acting admin's resolved users.id for updated_by (UpdateReceiptTemplateConfigParams.UpdatedBy is pgtype.UUID, not a raw Keycloak subject string). POST /api/admin/users does not consume app_user_id but is unaffected: the calling admin must simply already be a provisioned users row, consistent with every other *_by-writing route in this API."
  - "Settings service + handler reuse the EXACT SAME receiptsStore and pdfRenderer instances main.go already constructs for the outbox worker (04-05) — not new instances — directly satisfying D-58/D-61's 'preview must use the same sandboxed pipeline as production, not a second/less-locked path'."
  - "Preview/PreviewPDF request bodies (PreviewRequest) never accept a donation id or any donor field — only the admin's in-progress template/section6/image-key/language values plus a fixed server-side sample fixture (Thai-name + English-name), enforcing D-61's 'never live donor PII' mandate structurally, not just by convention."

requirements-completed: [FR-33, NFR-09, FR-24, FR-20, FR-21, FR-22]

coverage:
  - id: D1
    description: "An Admin can read and save the full receipt template config (HTML th/en, §6 th/en text, 1x/2x, image object keys, number format) in one PUT request with no deploy"
    requirement: "NFR-09"
    verification:
      - kind: e2e
        ref: "donnarec-api/cmd/server/e2e_test.go#TestE2E_AdminSettings/GetSettings_SeededDefaults (pass)"
        status: pass
      - kind: e2e
        ref: "donnarec-api/cmd/server/e2e_test.go#TestE2E_AdminSettings/Save_ValidRoundTrip_AuditedAndPersisted (pass)"
        status: pass
      - kind: integration
        ref: "donnarec-api/internal/settings/service_test.go#TestSettingsService_SaveAndGet_RoundTrip (pass)"
        status: pass
    human_judgment: false
  - id: D2
    description: "Template save validates parse-ability via html/template.Parse and rejects invalid templates with 422; separator/prefix are rejected if they contain characters unsafe for the immutable receipt-number ledger"
    requirement: "FR-33"
    verification:
      - kind: e2e
        ref: "donnarec-api/cmd/server/e2e_test.go#TestE2E_AdminSettings/Save_InvalidTemplate_422 (pass); .../Save_InvalidNumberFormat_422 (pass)"
        status: pass
      - kind: unit
        ref: "donnarec-api/internal/settings/service_test.go#TestSaveSettings_InvalidTemplate_Rejected, #TestSaveSettings_InvalidNumberFormat_Rejected (pass)"
        status: pass
    human_judgment: false
  - id: D3
    description: "Brand image uploads (letterhead/seal/signature/watermark) are magic-byte validated (image/jpeg+png only, 2 MB cap) and Admin-only"
    requirement: "FR-20"
    verification:
      - kind: unit
        ref: "donnarec-api/internal/storage/template_image_test.go (TestTemplateImageAllowedTypes, TestTemplateImageRejectsPDF, TestTemplateImageRejectsSpoofed, TestTemplateImageSizeLimit — all pass)"
        status: pass
      - kind: e2e
        ref: "donnarec-api/cmd/server/e2e_test.go#TestE2E_AdminSettings/UploadImage_MagicByteValidatedAdminOnly (pass)"
        status: pass
    human_judgment: false
  - id: D4
    description: "An HTML preview endpoint executes the template with sample (non-PII) data and escapes admin-configured text substituted into it; a real-PDF preview endpoint renders via the SAME sandboxed Chromium pipeline as production"
    requirement: "FR-24"
    verification:
      - kind: e2e
        ref: "donnarec-api/cmd/server/e2e_test.go#TestE2E_AdminSettings/Preview_EscapesInjectedSection6Text (pass)"
        status: pass
      - kind: e2e
        ref: "donnarec-api/cmd/server/e2e_test.go#TestE2E_AdminSettings/PreviewPDF_ReturnsRealPDFBytesViaSandboxedPipeline (pass, real chrome sidecar via testutil.StartChrome)"
        status: pass
    human_judgment: false
  - id: D5
    description: "Every settings mutation (save, image upload) is Admin-gated (RequireRoles(RoleAdmin)) and non-Admin callers are rejected with 403"
    requirement: "FR-33"
    verification:
      - kind: e2e
        ref: "donnarec-api/cmd/server/e2e_test.go#TestE2E_AdminSettings/NonAdmin_Forbidden, .../Preview_NonAdmin_Forbidden, .../UploadImage_MagicByteValidatedAdminOnly (maker-403 subcase) — all pass"
        status: pass
    human_judgment: false
  - id: D6
    description: "Every settings save writes an append-only audit row (D-58)"
    requirement: "FR-33"
    verification:
      - kind: e2e
        ref: "donnarec-api/cmd/server/e2e_test.go#TestE2E_AdminSettings/Save_ValidRoundTrip_AuditedAndPersisted (asserts a matching audit_log row, pass)"
        status: pass
    human_judgment: false

# Metrics
duration: ~20min
completed: 2026-07-04
status: complete
---

# Phase 4 Plan 7: Admin Receipt Settings API (Config Store + Preview) Summary

**Admin-only settings API (`internal/settings`) merging receipt_template_config + Phase 2's receipt_number_config into one read/save surface, with html/template-validated saves, magic-byte-validated brand-image uploads, and HTML/real-PDF preview endpoints that reuse the exact same sandboxed Chromium pipeline (internal/pdf) as production — all proven over the real HTTP → auth → RBAC(Admin) → handler → service → DB path, including a real chrome-sidecar-backed PreviewPDF test**

## Performance

- **Duration:** ~20 min
- **Started:** 2026-07-04T18:10:43+07:00 (first RED commit)
- **Completed:** 2026-07-04T18:25:51+07:00 (GREEN commit)
- **Tasks:** 2 (both TDD: RED then GREEN)
- **Files modified:** 11 (5 created, 6 modified)

## Accomplishments

- `internal/settings/service.go`: `SettingsService` merges `receipt_template_config` (template HTML th/en, §6 text th/en, deduction multiplier, four image object keys) with `receipt_number_config` (separator/padding/year-format/prefix, Phase 2) into a single `ReceiptSettings` DTO — `GetSettings`/`SaveSettings` round-trip both tables in one call, validating template parse-ability (`html/template.Parse`) and a number-format character allowlist BEFORE any DB write, so a rejected save leaves both rows untouched.
- `internal/settings/handler.go` + `cmd/server/main.go`: five Admin-only routes (`GET`/`PUT /settings`, `POST /settings/images/:slot`, `POST /settings/preview`, `POST /settings/preview/pdf`) under the existing `adminGroup`, now also running `auth.ResolveAppUser` so `updated_by` carries the acting admin's resolved `users.id`. Save/UploadImage set `audit_after` for the append-only audit trail (D-58).
- `internal/storage/client.go`: `PutTemplateImage`/`ValidateTemplateImage` reuse the proven `validateSlip` magic-byte pattern but with a narrower allowlist (image/jpeg + image/png, NO pdf) and a 2 MB cap — with their OWN sentinel errors so the error message text is factually correct for this smaller cap (see Deviations).
- Preview/PreviewPDF assemble the admin's CURRENT UNSAVED template against a fixed, server-side sample fixture (Thai-name + English-name, never a donation id or donor field) via `pdf.Render`/`pdf.RenderPDF` — the identical code path production rendering uses, so donor-field-shaped injection is provably escaped and the "real PDF" preview is genuinely the same sandbox (JS-disabled, network-blocked chrome sidecar), not a second, less-locked path.
- `TestE2E_AdminSettings` (9 subtests) drives every endpoint over the real HTTP → `RequireAuth` (real Keycloak-shaped token) → `RequireRoles(Admin)` → `ResolveAppUser` → handler → service → DB seam, including a genuinely real-Chromium-backed PreviewPDF subtest via `testutil.StartChrome`, satisfying the CONVENTIONS.md integration-test gate for this runtime-seam-touching plan.

## Task Commits

Each task was committed atomically (RED then GREEN per TDD):

1. **Task 1: settings config-store service + template-image validation**
   - RED — `c795bcf` (test): failing storage template-image validation tests
   - RED — `6b09ca9` (test): failing SettingsService tests (model.go DTOs included as data-shape-only support)
   - GREEN — `0080839` (feat): `internal/settings/service.go` + `storage.PutTemplateImage` + `UpdateReceiptNumberConfig` sqlc query — all Task 1 tests pass, including a real-Postgres round-trip
2. **Task 2: Admin settings handler + routes + HTML/real-PDF preview + E2E**
   - RED — `1f6ec8d` (test): failing `TestE2E_AdminSettings` (verified via a temporary handler.go/main.go rollback + `go vet` compile failure, then restored)
   - GREEN — `96a5050` (feat): `internal/settings/handler.go` + `cmd/server/main.go` wiring — all 9 E2E subtests pass, plus the full pre-existing `TestE2E_MakerCheckerIssuancePipeline` suite re-verified green after the `adminGroup` middleware change

**Plan metadata:** (this commit, docs: complete plan)

## Files Created/Modified

- `donnarec-api/internal/settings/model.go` - `ReceiptSettings`/`PreviewRequest` DTOs
- `donnarec-api/internal/settings/service.go` - `SettingsService` (Get/Save/SaveTemplateImage/BuildPreviewHTML), `ReceiptsStore` interface
- `donnarec-api/internal/settings/service_test.go` - invalid-template/invalid-number-format/invalid-slot unit tests + a real-Postgres round-trip integration test
- `donnarec-api/internal/settings/handler.go` - `Handler` (Get/Save/UploadImage/Preview/PreviewPDF), `PDFRenderer` interface
- `donnarec-api/internal/storage/client.go` - `PutTemplateImage`/`ValidateTemplateImage` + new sentinel errors
- `donnarec-api/internal/storage/template_image_test.go` - magic-byte/size-cap unit tests for template images
- `donnarec-api/internal/db/queries/receiptno.sql` + generated code - new `UpdateReceiptNumberConfig` query
- `donnarec-api/cmd/server/main.go` - settings service/handler wiring, `adminGroup` routes + `ResolveAppUser`
- `donnarec-api/cmd/server/e2e_test.go` - `TestE2E_AdminSettings` (9 subtests), `fakeSettingsStore`, real chrome sidecar wiring in `newE2EHarness`

## Decisions Made

- Added `UpdateReceiptNumberConfig` (Rule 2) — the number-format tab had no save path before this plan; without it, "save all tabs in one request" would silently drop half the settings screen.
- New, distinctly-worded sentinel errors for template-image validation (Rule 1) rather than reusing the slip upload's `ErrUnsupportedFileType`/`ErrFileTooLarge` — those messages name "application/pdf" and "10 MB", both wrong for the 2 MB image-only cap.
- Added a dedicated image-upload endpoint (Rule 2) since the plan's Task 1 built `PutTemplateImage` but Task 2's action text never named a route for it — without one, it would be unreachable dead code and the plan's own must_haves truth about image uploads would be unmet.
- `adminGroup` now runs `auth.ResolveAppUser` (previously only `donationGroup` did) so settings mutations can set `updated_by` to a real `users.id`, never the raw Keycloak subject — mirrors the Phase 3 `created-by-fk-mismatch` fix.
- Settings preview/PDF-preview reuse the exact same `receiptsStore`/`pdfRenderer` instances the outbox worker already uses (not new ones) — a structural guarantee, not just a convention, that preview never runs through a second, less-locked rendering path.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical] Added `UpdateReceiptNumberConfig` sqlc query**
- **Found during:** Task 1 (settings config-store service)
- **Issue:** The plan's must_haves truth requires saving the full config "including number format," but Phase 2 only ever built a `GetReceiptNumberConfig` read query — no save path existed anywhere in the codebase.
- **Fix:** Added `UpdateReceiptNumberConfig` to `internal/db/queries/receiptno.sql` (mirrors `UpdateReceiptTemplateConfig`'s shape) and regenerated sqlc code.
- **Files modified:** `internal/db/queries/receiptno.sql`, `internal/db/generated/receiptno.sql.go`, `internal/db/generated/querier.go`
- **Verification:** `TestSettingsService_SaveAndGet_RoundTrip` proves both tables round-trip together; `go build ./internal/db/...` passes.
- **Committed in:** `0080839` (Task 1 GREEN commit)

**2. [Rule 1 - Bug] Dedicated sentinel errors for template-image validation**
- **Found during:** Task 1 (storage template-image validation)
- **Issue:** The plan's action text suggested reusing `ErrUnsupportedFileType`/`ErrFileTooLarge`, but those error strings literally say "only image/jpeg, image/png, application/pdf are allowed" and "exceeds ... 10 MB" — both factually incorrect for the 2 MB, image-only template-asset cap.
- **Fix:** Added `ErrUnsupportedTemplateImageType`/`ErrTemplateImageTooLarge` with correctly-scoped message text; `PutTemplateImage`/`ValidateTemplateImage` return these instead.
- **Files modified:** `internal/storage/client.go`
- **Verification:** `internal/storage/template_image_test.go` (4 tests, all pass); `TestE2E_AdminSettings/UploadImage_MagicByteValidatedAdminOnly` confirms the correct HTTP error codes/bodies.
- **Committed in:** `0080839` (Task 1 GREEN commit)

**3. [Rule 2 - Missing Critical] Added `POST /api/admin/settings/images/:slot` upload endpoint**
- **Found during:** Task 2 (Admin settings handler + routes)
- **Issue:** Task 1 built `storage.PutTemplateImage` and the plan's must_haves truth requires Admin-only, magic-byte-validated brand-image uploads, but Task 2's action text never named a route to invoke it — without one, the upload path would be dead code and the truth would be structurally unmet.
- **Fix:** Added `Handler.UploadImage` + `SettingsService.SaveTemplateImage` (read-modify-write: fetch current template config, overlay the new object key for the given slot, persist via the existing `UpdateReceiptTemplateConfig`) and registered `adminGroup.POST("/settings/images/:slot", ...)`.
- **Files modified:** `internal/settings/service.go`, `internal/settings/handler.go`, `cmd/server/main.go`
- **Verification:** `TestE2E_AdminSettings/UploadImage_MagicByteValidatedAdminOnly` (valid PNG accepted + persisted, PDF rejected 415, unknown slot rejected 400, non-Admin rejected 403).
- **Committed in:** `96a5050` (Task 2 GREEN commit)

**4. [Rule 2 - Missing Critical] `adminGroup` now runs `auth.ResolveAppUser`**
- **Found during:** Task 2 (Admin settings handler + routes)
- **Issue:** `UpdateReceiptTemplateConfigParams.UpdatedBy`/`UpdateReceiptNumberConfigParams.UpdatedBy` are `pgtype.UUID` (a `users.id`), but `adminGroup` had no middleware resolving the Keycloak subject to a `users.id` — only `donationGroup` did. Without this, `Save`/`UploadImage` could not obtain a value to pass as `updatedBy` at all.
- **Fix:** Added `adminGroup.Use(auth.ResolveAppUser(appUserResolver, logger))`, mirroring `donationGroup`'s existing wiring exactly.
- **Files modified:** `cmd/server/main.go`
- **Verification:** Full pre-existing `TestE2E_MakerCheckerIssuancePipeline` suite re-run green after this change (no regression to `POST /api/admin/users`, which does not consume `app_user_id`); `TestE2E_AdminSettings/Save_ValidRoundTrip_AuditedAndPersisted` confirms `updated_by` round-trips as the admin's real `users.id`.
- **Committed in:** `96a5050` (Task 2 GREEN commit)

---

**Total deviations:** 4 auto-fixed (2 missing-critical infrastructure, 1 missing-critical endpoint, 1 bug/message-correctness)
**Impact on plan:** All four are necessary for the plan's own stated `must_haves` truths to be achievable and for existing code (`PutTemplateImage`) not to be dead. No architectural scope creep — no new tables, no new packages beyond `internal/settings` which the plan itself specifies, no new external dependencies.

## Issues Encountered

- Confirming the Task 2 RED gate required a temporary rollback: `settings/handler.go` was moved aside and `cmd/server/main.go`'s changes were `git stash`ed so `go vet` could prove `cmd/server/e2e_test.go` genuinely failed to compile (`undefined: settings.NewHandler`) before restoring both files for GREEN — necessary because `main.go`'s wiring changes and `handler.go`'s new type are tightly coupled (the harness references both).

## User Setup Required

None — no external service configuration required. The settings API reuses the existing chrome sidecar (04-02) and MinIO receipts bucket (04-01/04-05) already wired for the outbox worker; no new environment variables.

## Next Phase Readiness

- 04-08 (Admin Settings UI) can proceed: `GET/PUT /api/admin/settings`, `POST /api/admin/settings/images/:slot`, `POST /api/admin/settings/preview`, `POST /api/admin/settings/preview/pdf` are all built, Admin-gated, audited, and E2E-proven over the real HTTP path.
- `ReceiptSettings`'s JSON field names (`template_html`, `template_html_en`, `section6_text_th`, `section6_text_en`, `deduction_multiplier`, `letterhead_object_key`/`seal_object_key`/`signature_object_key`/`watermark_object_key`, `separator`/`running_no_padding`/`year_format`/`prefix`) are the exact contract 04-08's `SettingsTabs`/`NumberFormatEditor` components will bind to.
- `PreviewRequest`'s `language` field (`"th"`/`"en"`, default `"th"`) is the hook 04-08's segmented preview-mode toggle can use to show both donor_language branches per 04-UI-SPEC.md's "Sample preview data" note.
- No blockers.

## Self-Check: PASSED

Verified files exist on disk: `donnarec-api/internal/settings/model.go`, `service.go`, `service_test.go`, `handler.go`, `donnarec-api/internal/storage/template_image_test.go` (all FOUND). Verified commit hashes present in `git log --oneline --all`: `c795bcf`, `6b09ca9`, `0080839`, `1f6ec8d`, `96a5050` (all FOUND).

---
*Phase: 04-receipt-pdf-email-delivery-outbox-worker*
*Completed: 2026-07-04*
