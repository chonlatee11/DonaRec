---
phase: 04-receipt-pdf-email-delivery-outbox-worker
plan: 08
subsystem: ui
tags: [nextjs, react, tanstack-query, next-intl, shadcn-tabs, sandboxed-iframe, vitest]

# Dependency graph
requires:
  - phase: 04-receipt-pdf-email-delivery-outbox-worker
    plan: 07
    provides: "Admin API: GET/PUT /api/admin/settings, POST /api/admin/settings/images/:slot, POST /api/admin/settings/preview, POST /api/admin/settings/preview/pdf — all Admin-gated, audited, E2E-proven"
  - phase: 04-receipt-pdf-email-delivery-outbox-worker
    plan: 06
    provides: "FE patterns: BFF route shape, next-intl namespace conventions, TanStack Query client-component pattern, SlipUploadZone drag-drop visual language"
provides:
  - "Admin-only /admin/settings screen (Screen 6): four tabs (template HTML, brand images, tax-deduction text, number format) sharing one local form-state object, saved via a single PUT"
  - "Sandboxed (sandbox=\"allow-same-origin\" only) 400ms-debounced HTML live preview + a real-Chromium-backed PDF preview mode, both using server-side sample/mock data only — never a donation id or donor field"
  - "BFF proxies to the 04-07 Admin API: GET/PUT /api/bff/settings, POST /api/bff/settings/preview, POST /api/bff/settings/preview/pdf (binary passthrough), POST /api/bff/settings/images/:slot (multipart passthrough)"
  - "lib/session-role.ts: decodeAccessTokenRoles/isAdminViewer — Admin-only nav gate (AppShell) + route guard (page.tsx)"
  - "lib/receipt-number-format.ts + lib/debounce.ts: pure, unit-tested logic backing NumberFormatEditor's live example and TemplateEditor's live-preview debounce"
affects: []

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Compound-component split for a persistent cross-tab UI region: TemplateEditor.tsx exports TemplateEditorFields (left-column textareas, rendered only inside the 'template' TabsContent) and TemplateLivePreview (right-column preview pane, rendered by SettingsTabs OUTSIDE the Tabs' conditional content so it stays visible across all four tabs, per UI-SPEC's 'preview always renders the full receipt regardless of active tab')"
    - "One shared local form-state object (SettingsFormValues) owns all four tabs' values in SettingsTabs — 'Save' always issues a single PUT with everything, matching settings/model.go's 'no partial/inconsistent config states' contract"
    - "Debounce implemented as a plain testable function (lib/debounce.ts), not a React hook — keeps the 400ms live-preview timing logic unit-testable in a hermetic node environment (no jsdom/testing-library in this repo) instead of only reachable via component rendering"
    - "formatReceiptNumberExample (lib/receipt-number-format.ts) is a literal TS port of internal/receiptno/format.go's algorithm, tested against that file's own doc-comment examples verbatim, so the FE live example can never silently drift from what the backend actually freezes"

key-files:
  created:
    - donnarec-web/lib/session-role.ts
    - donnarec-web/lib/settings.ts
    - donnarec-web/lib/receipt-number-format.ts
    - donnarec-web/lib/debounce.ts
    - donnarec-web/lib/__tests__/session-role.test.ts
    - donnarec-web/lib/__tests__/receipt-number-format.test.ts
    - donnarec-web/lib/__tests__/debounce.test.ts
    - donnarec-web/app/admin/settings/page.tsx
    - donnarec-web/app/api/bff/settings/route.ts
    - donnarec-web/app/api/bff/settings/preview/route.ts
    - donnarec-web/app/api/bff/settings/preview/pdf/route.ts
    - donnarec-web/app/api/bff/settings/images/[slot]/route.ts
    - donnarec-web/app/api/bff/settings/__tests__/bff-routes.test.ts
    - donnarec-web/components/SettingsTabs.tsx
    - donnarec-web/components/TemplateEditor.tsx
    - donnarec-web/components/ImageUploadSlot.tsx
    - donnarec-web/components/NumberFormatEditor.tsx
    - donnarec-web/components/ui/tabs.tsx
    - donnarec-web/public/fonts/README.md
  modified:
    - donnarec-web/components/AppShell.tsx
    - donnarec-web/messages/th.json
    - donnarec-web/messages/en.json
    - donnarec-web/package.json
    - donnarec-web/package-lock.json

key-decisions:
  - "Route path is /admin/settings (matches the plan's explicit files_modified path app/admin/settings/page.tsx), not the bare /settings mentioned in 04-UI-SPEC.md's Screen 6 prose — the plan's file path is authoritative; AppShell's nav link points at /admin/settings."
  - "Admin-only gating implemented via a new lib/session-role.ts (decodeAccessTokenRoles reading realm_access.roles, mirroring donnarec-api/internal/auth/claims.go's documented Pitfall 1) — this is a UX-layer hint only (nav visibility + a page.tsx redirect); Go's adminGroup.Use(RequireRoles(RoleAdmin)) (04-07) remains the sole authorization authority, re-checked on every BFF call regardless of what this client-side hint decides."
  - "TemplateEditor.tsx is split into two exports (TemplateEditorFields, TemplateLivePreview) rather than one component, so the live-preview pane can be rendered by SettingsTabs OUTSIDE the shadcn Tabs' conditional panels and stay visible on all four tabs — satisfying UI-SPEC's 'persistent across tab switches' requirement while keeping both pieces in the single TemplateEditor.tsx file the plan names."
  - "PreviewPDF's raw application/pdf byte response required bypassing bffForward/passthroughGoResponse (which assume a JSON envelope) — app/api/bff/settings/preview/pdf/route.ts reads goRes.arrayBuffer() and forwards Content-Type: application/pdf verbatim, falling back to JSON passthrough only for non-PDF (error) responses."
  - "TH Sarabun New is NOT bundled (same open licensing item as donnarec-api/assets/fonts/README.md's Assumption A3) — the preview iframe's injected @font-face references public/fonts/THSarabunNew.woff2 with a graceful fallback to the Google-Fonts 'Sarabun' family already used for app chrome, so the preview works correctly today and picks up the licensed font automatically once sourced, with no code change."

requirements-completed: [FR-33, NFR-09, FR-24]

coverage:
  - id: D1
    description: "An Admin can view and edit the full receipt template config (HTML th/en, brand images, tax-deduction text + 1x/2x, number format) across four tabs and save all of it in one request"
    requirement: "NFR-09"
    verification:
      - kind: unit
        ref: "donnarec-web/lib/__tests__/receipt-number-format.test.ts (5 tests, pass) — number-format tab's live example logic"
        status: pass
      - kind: unit
        ref: "donnarec-web/lib/__tests__/debounce.test.ts (3 tests, pass) — live-preview debounce logic"
        status: pass
      - kind: other
        ref: "cd donnarec-web && npm run build (pass) — /admin/settings route compiles, SettingsTabs/TemplateEditor/ImageUploadSlot/NumberFormatEditor type-check against settings/model.go's exact JSON contract"
        status: pass
    human_judgment: true
    rationale: "This plan's Task 3 (checkpoint:human-verify, gate=blocking) is a human browser walkthrough of the four tabs, live preview, image upload, and save-all-reflects-on-new-receipts behavior against the running stack — deferred to phase-end /gsd-verify-work per explicit user decision (see 'Deferred: Task 3 Human UI Walkthrough' below). Automated coverage proves the logic and type contracts; it cannot observe the browser DOM, iframe rendering, or toast copy."
  - id: D2
    description: "BFF proxy layer for the Admin settings API forwards requests/responses correctly, including binary PDF passthrough and multipart image upload, and never sends live donor PII on preview"
    requirement: "FR-24"
    verification:
      - kind: unit
        ref: "donnarec-web/app/api/bff/settings/__tests__/bff-routes.test.ts (11 tests, pass) — Bearer forwarding, 401 gating, 422/415 error passthrough, binary application/pdf byte-for-byte passthrough, multipart forward, and an explicit assertion that no donation_id/national_id is ever sent on preview"
        status: pass
    human_judgment: false
  - id: D3
    description: "Settings screen and nav link are visible only to Admin"
    requirement: "FR-33"
    verification:
      - kind: unit
        ref: "donnarec-web/lib/__tests__/session-role.test.ts (4 tests, pass) — realm_access.roles extraction, never a top-level roles claim"
        status: pass
    human_judgment: true
    rationale: "The actual visual nav-gating and /admin/settings redirect behavior (isAdminViewer wired into AppShell + page.tsx) requires a real Keycloak-issued token across three roles to observe end-to-end — covered by this plan's deferred Task 3 walkthrough, not by the hermetic decode-only unit test."

# Metrics
duration: ~13min
completed: 2026-07-04
status: complete
---

# Phase 4 Plan 8: Admin Settings UI (Screen 6) Summary

**Admin-only /admin/settings screen — four shadcn Tabs panels (template HTML, brand images, tax text, number format) sharing one save-all form state, a 400ms-debounced sandboxed HTML live preview plus a real-Chromium PDF preview mode, all proxied through new BFF routes to the 04-07 Admin API; Screen 6's manual UI walkthrough deferred to phase-end verify-work**

## Performance

- **Duration:** ~13 min (Tasks 1-2; Task 3 is a deferred checkpoint, not executed)
- **Started:** 2026-07-04T18:42:13+07:00 (first RED commit)
- **Completed:** 2026-07-04T18:55:31+07:00 (Task 2 GREEN commit)
- **Tasks:** 2 of 3 executed (Task 3 is a `checkpoint:human-verify` gate deferred by user decision, not executed)
- **Files modified:** 24 (19 created, 5 modified)

## Accomplishments

- `lib/session-role.ts`: `decodeAccessTokenRoles`/`getViewerRoles`/`isAdminViewer` decode the Keycloak `realm_access.roles` claim (mirroring `donnarec-api/internal/auth/claims.go`'s documented "roles are nested under realm_access.roles, never top-level" pitfall) to drive Admin-only nav visibility (`AppShell`) and a route redirect (`app/admin/settings/page.tsx`) — UX-layer only; Go's `adminGroup.Use(RequireRoles(RoleAdmin))` (04-07) remains the real authority and is re-checked on every BFF call.
- Four new BFF proxy routes consuming the 04-07 Admin API: `GET/PUT /api/bff/settings`, `POST /api/bff/settings/preview` (JSON), `POST /api/bff/settings/preview/pdf` (raw `application/pdf` byte passthrough — bypasses `bffForward`'s JSON-only envelope assumption), `POST /api/bff/settings/images/:slot` (multipart passthrough, mirroring the existing slip-upload route's rationale for bypassing `bffForward` on the request side).
- `components/SettingsTabs.tsx`: the Screen 6 orchestrator — one local `SettingsFormValues` object backs all four tabs (template / images / tax text / number format), so "บันทึกการตั้งค่า" always issues a single `PUT` with everything (no partial-save states, matching `settings/model.go`'s contract). Fetches its seed via TanStack Query against the BFF (mirrors `DonationListView`'s established client-fetch pattern).
- `components/TemplateEditor.tsx`: split into `TemplateEditorFields` (the two mono `<textarea>`s, th/en, rendered only in the "template" tab) and `TemplateLivePreview` (the persistent right-column preview pane, rendered by `SettingsTabs` outside the `Tabs`' conditional content so it stays visible across all four tabs per the UI-SPEC). The preview iframe uses `sandbox="allow-same-origin"` only (no `allow-scripts`), content injected via `srcDoc` (no script execution needed), a non-dismissible info banner stating no-JS/no-network, a 400ms debounce (`lib/debounce.ts`) driving `POST /api/bff/settings/preview`, and a segmented HTML/real-PDF toggle where "เรนเดอร์ PDF จริง" calls `POST /api/bff/settings/preview/pdf` and renders the returned PDF via `<embed>`.
- `components/ImageUploadSlot.tsx`: 96×96 drag-drop upload tile (`SlipUploadZone`'s visual language at thumbnail scale), 2 MB/JPG-PNG client pre-check, live pixel thumbnail for a file just selected this session (via `URL.createObjectURL`), and a "has image" populated-state indicator for a previously-saved key (see Known Limitation below).
- `components/NumberFormatEditor.tsx`: separator/padding/year-format/prefix inputs plus a client-computed, `aria-live` live example via `lib/receipt-number-format.ts`'s `formatReceiptNumberExample` — a literal TS port of `internal/receiptno/format.go`'s algorithm, tested against that file's own doc-comment examples so the preview can never drift from what the backend actually freezes at allocation time.
- `messages/th.json`/`en.json`: new `settings.*` namespace (tabs, template editor, images, tax text, number format copy per `04-UI-SPEC.md`'s Copywriting Contract); `nav.settings` already existed from Phase 3 bootstrap.
- Two hermetic Vitest suites (mirroring the existing `bff-routes.test.ts`/pure-unit conventions in this repo, which has no jsdom/testing-library installed): BFF proxy trust-boundary tests (11 cases) and pure-logic tests for role decode, debounce, and number-format formatting (12 cases across 3 files) — all RED-then-GREEN per task.

## Task Commits

Each task was committed atomically (RED then GREEN, this repo's established TDD convention for behavior-adding logic):

1. **Task 1: BFF settings routes + Admin nav gate + settings.\* messages + install tabs**
   - RED — `cd2337f` (test): failing `decodeAccessTokenRoles` + settings BFF route tests (modules did not exist)
   - GREEN — `4ec6159` (feat): `lib/session-role.ts`, BFF routes, `AppShell` nav gate, `settings.*` i18n, shadcn `tabs` installed — all tests pass, `npm run lint`/`build` pass
2. **Task 2: Settings page — four tabs, sandboxed live preview + real-PDF button, save-all**
   - RED — `a73ba9b` (test): failing `formatReceiptNumberExample` + `debounce` tests (modules did not exist)
   - GREEN — `802350d` (feat): `lib/receipt-number-format.ts`, `lib/debounce.ts`, `lib/settings.ts`, `app/admin/settings/page.tsx`, `SettingsTabs`/`TemplateEditor`/`ImageUploadSlot`/`NumberFormatEditor` — all tests pass, `npm run lint`/`build` pass

**Task 3 (checkpoint:human-verify, gate=blocking): NOT EXECUTED — deferred by explicit user decision.** See "Deferred: Task 3 Human UI Walkthrough" below.

**Plan metadata:** (this commit, docs: complete plan)

## Files Created/Modified

- `donnarec-web/lib/session-role.ts` - `decodeAccessTokenRoles`/`getViewerRoles`/`isAdminViewer`
- `donnarec-web/lib/settings.ts` - `ReceiptSettings`/`PreviewRequest` types + server/client fetchers
- `donnarec-web/lib/receipt-number-format.ts` - `formatReceiptNumberExample` (TS port of format.go)
- `donnarec-web/lib/debounce.ts` - generic trailing-edge debounce utility
- `donnarec-web/lib/__tests__/session-role.test.ts`, `receipt-number-format.test.ts`, `debounce.test.ts` - hermetic unit tests
- `donnarec-web/app/admin/settings/page.tsx` - Admin-guarded server component
- `donnarec-web/app/api/bff/settings/route.ts` - GET/PUT proxy to `/api/admin/settings`
- `donnarec-web/app/api/bff/settings/preview/route.ts` - POST proxy to `/api/admin/settings/preview`
- `donnarec-web/app/api/bff/settings/preview/pdf/route.ts` - POST proxy, binary PDF passthrough
- `donnarec-web/app/api/bff/settings/images/[slot]/route.ts` - POST multipart proxy to image upload
- `donnarec-web/app/api/bff/settings/__tests__/bff-routes.test.ts` - BFF trust-boundary tests
- `donnarec-web/components/SettingsTabs.tsx` - Screen 6 orchestrator (four tabs + save-all)
- `donnarec-web/components/TemplateEditor.tsx` - `TemplateEditorFields` + `TemplateLivePreview`
- `donnarec-web/components/ImageUploadSlot.tsx` - 96×96 brand-image upload tile
- `donnarec-web/components/NumberFormatEditor.tsx` - number-format fields + live example
- `donnarec-web/components/ui/tabs.tsx` - shadcn `tabs` component (installed)
- `donnarec-web/public/fonts/README.md` - TH Sarabun New sourcing note (mirrors backend's)
- `donnarec-web/components/AppShell.tsx` - Admin-only "ตั้งค่า" nav link
- `donnarec-web/messages/th.json`, `en.json` - `settings.*` namespace

## Decisions Made

- Route path is `/admin/settings` (the plan's explicit `files_modified` path), not the bare `/settings` mentioned in 04-UI-SPEC.md's prose — `AppShell`'s nav link and the redirect target both point at `/admin/settings`.
- Admin gating is a client-side UX hint only (`lib/session-role.ts`); Go's `RequireRoles(RoleAdmin)` remains the sole authority, independently re-checked on every request.
- `TemplateEditor.tsx` exports two components so the live-preview pane can render outside the `Tabs`' conditional panels and stay visible across all four tabs (UI-SPEC "persistent across tab switches"), while both pieces still live in the one file the plan names.
- `preview/pdf`'s raw PDF bytes required a custom BFF route (not `bffForward`) that reads `arrayBuffer()` and forwards `Content-Type: application/pdf` verbatim, with JSON-error fallback for non-PDF responses.
- TH Sarabun New is not bundled yet (same pending licensing item as the backend's `assets/fonts/README.md`) — the preview iframe falls back to the Google-Fonts "Sarabun" family already used for app chrome until the licensed file is placed at `public/fonts/THSarabunNew.woff2`.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical] Added `POST /api/bff/settings/images/:slot` BFF proxy**
- **Found during:** Task 1 (BFF settings routes)
- **Issue:** Not named in the plan's `files_modified` list, but 04-07 built the corresponding Go endpoint (`POST /api/admin/settings/images/:slot`) specifically for brand-image uploads. Without a BFF proxy, `ImageUploadSlot`'s upload flow (required by the plan's own must_haves truth "lets admins edit ... brand images") would be unreachable from the browser — the exact situation 04-07-SUMMARY.md documents for the same endpoint's existence.
- **Fix:** Added `app/api/bff/settings/images/[slot]/route.ts`, a multipart-passthrough proxy mirroring the existing slip-upload route's pattern (bypasses `bffForward` for the request body, which is UTF-8-unsafe for binary bytes).
- **Files modified:** `app/api/bff/settings/images/[slot]/route.ts` (new), `lib/settings.ts` (`uploadTemplateImage`)
- **Verification:** `app/api/bff/settings/__tests__/bff-routes.test.ts` (3 subtests: successful multipart forward, 415 passthrough, 401 gating)
- **Committed in:** `4ec6159` (Task 1 GREEN commit)

**2. [Rule 2 - Missing Critical] `NumberFormatEditor` exposes `year_format` + `prefix`, not just separator/padding**
- **Found during:** Task 2 (Settings page components)
- **Issue:** 04-UI-SPEC.md's Tab 4 prose only describes a separator input, a zero-pad digit input, and a live example. `settings/model.go`'s `ReceiptSettings.YearFormat` is validated `required,oneof=BE4 CE4` on the "save all tabs" `PUT` — omitting a year-format control from the UI would make every save fail 422 regardless of which tab the admin actually intended to change.
- **Fix:** Added a `Select` for `year_format` (BE4/CE4) and an `Input` for `prefix` to `NumberFormatEditor`, both wired into the shared form state.
- **Files modified:** `components/NumberFormatEditor.tsx`, `messages/th.json`/`en.json` (`numberFormat.yearFormatLabel`/`yearFormatBE`/`yearFormatCE`/`prefixLabel`)
- **Verification:** `lib/__tests__/receipt-number-format.test.ts` covers the BE4/CE4 rendering the Select drives; `npm run build` confirms the full `ReceiptSettings` shape round-trips without validation gaps.
- **Committed in:** `802350d` (Task 2 GREEN commit)

---

**Total deviations:** 2 auto-fixed (2 missing-critical). No architectural scope creep — no new backend endpoints, tables, or packages; both fixes make an already-planned capability (image upload, save-all) actually reachable/valid.

## Known Limitation (not a stub)

`ImageUploadSlot` cannot render a pixel-accurate thumbnail for a brand image that was already saved in a *previous* session — 04-07 built no GET-by-key/presigned-view endpoint for template images (only upload). The component instead shows a "มีรูปภาพแล้ว" (has image) populated-state indicator with a generic icon for that case. A file selected/uploaded in the **current** session renders as a real pixel thumbnail immediately via a local `URL.createObjectURL` object URL, since the browser already holds those bytes — this covers the primary upload/replace workflow. This is a genuine capability gap in the existing API surface, not a hardcoded/placeholder value — `hasImage` reflects the real object-key state, and adding a presigned-view endpoint (mirroring `DownloadReceipt`'s pattern from 04-06) would be a small, isolated follow-up if pixel-preview-of-already-saved-images becomes a requirement.

## Issues Encountered

None beyond the two Rule-2 fixes above.

## Deferred: Task 3 Human UI Walkthrough

**Status: DEFERRED to phase-end verification (`/gsd-verify-work`)** — by explicit user decision (this plan was executed with instructions to complete all code/automated-test tasks and defer the manual UI walkthrough). Code for Tasks 1-2 is complete, committed, and proven via automated unit/build gates (`npm run lint`, `npm run build`, `npm test` — all green, 39/39 tests passing across the whole `donnarec-web` suite). What remains unverified is the **visual/manual** confirmation of Screen 6's behavior against the live running stack, which these automated gates do not exercise (no browser, no real Keycloak-issued token across three roles, no visual confirmation of nav-link gating, no real chromedp/rod PDF render observed by a human, no confirmation that "Save" actually changes what a newly issued receipt looks like).

**Do NOT perform this walkthrough now.** This section exists so `/gsd-verify-work` (or whichever agent picks up phase-end UAT) has everything needed to execute it without re-deriving context.

### Credential / environment prerequisites (must be satisfied before the walkthrough can run)

1. **Keycloak `donnarec-frontend` confidential client secret** + **`donnarec-web/.env.local`** populated (issuer URL, client id, client secret, `NEXTAUTH_URL`, `NEXTAUTH_SECRET`) — same prerequisite documented in 04-06-SUMMARY.md's deferred walkthrough section.
2. **An Admin-role test account** (e.g. `admin`/`DonaRec123` per STATE.md, or the current seed data's admin user) plus at least one non-Admin account (Maker or Checker) to confirm the nav link is Admin-only.
3. **Running stack**: API on `:8000` (with the 04-07 Admin settings routes and `04-05`'s chrome sidecar for `PreviewPDF`), outbox worker process running, web on `:3000`, MinIO up, Keycloak on `:8080`. `docker compose up` should bring all of this up (per Phase 3/04-06 notes on the Postgres port-remap override).
4. **TH Sarabun New font file** is NOT required for this walkthrough to proceed — the preview gracefully falls back to Google-Fonts "Sarabun" (see `public/fonts/README.md`). If the licensed font has since been sourced and placed at `public/fonts/THSarabunNew.woff2`, the walkthrough should additionally confirm the preview visually matches the "Render Real PDF" output more closely than before.

### Exact walkthrough steps (per this plan's Task 3 `how-to-verify`)

1. Log in as Admin; confirm a "ตั้งค่า" nav item appears (and does NOT appear for Maker/Checker).
2. Open `/admin/settings`; edit the receipt template HTML — the right-hand preview updates ~400ms after you stop typing, in a sandboxed iframe, with sample (non-PII) data. Confirm the blue info banner states no-JS/no-network.
3. Upload a signature/letterhead image (jpg/png) — the 96×96 thumbnail updates (live, since this is a fresh upload this session); try a non-image or >2MB file and confirm the rejection copy.
4. Edit the §6 text and switch the 1x/2x select; edit the number-format separator/pad/year-format/prefix and confirm the live example updates.
5. Click "เรนเดอร์ PDF จริง" — a real Chromium-rendered sample PDF appears (Thai renders correctly via `fonts-thai-tlwg` fallback or TH Sarabun New if sourced), matching the HTML preview closely.
6. Click "บันทึกการตั้งค่า" — success toast confirms the change is live for new receipts; issue a fresh receipt (via the 04-06 flow) and confirm it reflects the new template. Confirm an already-issued receipt is unchanged (D-56 freeze).
7. Reload `/admin/settings` after saving with a previously-uploaded image slot populated (no fresh upload this session) — confirm the "มีรูปภาพแล้ว" populated-state indicator shows (not a broken image), consistent with the documented Known Limitation above.

### What automated coverage already proves (so the walkthrough is confirming presentation, not correctness)

- BFF proxy trust boundary (Bearer forwarding, 401 gating, error passthrough including binary PDF bytes, D-61 sample-data-only assertion) is proven by `app/api/bff/settings/__tests__/bff-routes.test.ts` (11/11 pass).
- Role-decode logic (`realm_access.roles`, never top-level `roles`) is proven by `lib/__tests__/session-role.test.ts` (4/4 pass).
- The number-format live example and the 400ms debounce timing logic are proven by `lib/__tests__/receipt-number-format.test.ts` (5/5 pass) and `lib/__tests__/debounce.test.ts` (3/3 pass).
- What is NOT proven by automation: the visual tab layout/45-55 split, the sandboxed iframe actually rendering in a real browser, the real-PDF `<embed>` displaying correctly, toast copy, and whether "Save" genuinely changes a subsequently-issued receipt's rendered PDF.

## User Setup Required

None new — this plan reuses infrastructure (Keycloak, the 04-07 Admin API, the 04-02/04-03 chrome sidecar) already wired in prior 04-* plans. The credential prerequisites listed above are existing environment setup, not new configuration introduced by this plan. The TH Sarabun New font file remains an open sourcing/licensing item shared with the backend (not a blocker for this plan's code-completeness).

## Next Phase Readiness

- Phase 4's eighth and final plan is code-complete. All 8 plans (04-01 through 04-08) have now been executed; the phase's remaining gate is the deferred human UI walkthroughs from 04-06 (Task 4, Screen 3b) and 04-08 (Task 3, Screen 6), both explicitly deferred to `/gsd-verify-work` per user decision.
- No blockers to phase-end verification. `/gsd-verify-work` should run both deferred walkthroughs (04-06's Screen 3b and 04-08's Screen 6) in the same session against one running stack, since both need the same Keycloak/API/worker/MinIO/chrome-sidecar environment up.

## Self-Check: PASSED

Verified files exist on disk: `donnarec-web/lib/session-role.ts`, `lib/settings.ts`, `lib/receipt-number-format.ts`, `lib/debounce.ts`, `app/admin/settings/page.tsx`, `components/SettingsTabs.tsx`, `components/TemplateEditor.tsx`, `components/ImageUploadSlot.tsx`, `components/NumberFormatEditor.tsx` (all FOUND). Verified commit hashes present in `git log --oneline`: `cd2337f`, `4ec6159`, `a73ba9b`, `802350d` (all FOUND). `npm run lint`, `npm run build`, and `npm test` (39/39 tests passing across the whole suite) all re-verified green as of this SUMMARY.

---
*Phase: 04-receipt-pdf-email-delivery-outbox-worker*
*Completed: 2026-07-04*
