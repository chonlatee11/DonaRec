---
phase: 03-donation-lifecycle-maker-checker-issuance
plan: "08"
subsystem: frontend-donations
tags: [next-js, donations, maker-checker, create-edit-form, consent, slip-upload, pii-reveal, cancel-reissue, i18n]
dependency_graph:
  requires: ["03-02", "03-03", "03-04", "03-06", "03-07"]
  provides: ["donation-create-edit-ui", "slip-upload-ui", "pii-reveal-dialog", "cancel-reissue-dialogs"]
  affects: []
tech_stack:
  added: []
  patterns:
    - "react-hook-form + zodResolver (zod v4) for client form validation"
    - "Server actions defined inline in page.tsx and passed as props to Client Components"
    - "national_id typed as required string (D-44); '' in edit mode = keep existing encrypted value"
    - "Multipart upload via apiFetchFormData (no Content-Type — browser sets boundary)"
    - "Session-only PII plaintext held in client component state; reload re-masks (T-03-34)"
    - "CancelDialog keeps dialog open on 409 ErrEDonationKeyedConfirmation for inline correction"
key_files:
  created:
    - donnarec-web/components/DonationForm.tsx
    - donnarec-web/components/ConsentBlock.tsx
    - donnarec-web/components/SlipUploadZone.tsx
    - donnarec-web/components/RevealPIIDialog.tsx
    - donnarec-web/components/CancelDialog.tsx
    - donnarec-web/app/donations/new/page.tsx
    - donnarec-web/app/donations/[id]/edit/page.tsx
  modified:
    - donnarec-web/lib/api.ts
    - donnarec-web/lib/donations.ts
    - donnarec-web/components/MaskedIdField.tsx
    - donnarec-web/components/ReviewActionPanel.tsx
    - donnarec-web/app/donations/[id]/page.tsx
    - donnarec-web/messages/th.json
    - donnarec-web/messages/en.json
decisions:
  - "national_id modelled as required `string` in FormValues + schema; edit mode accepts '' to preserve the existing encrypted value — resolves the react-hook-form Resolver type mismatch while honoring D-44"
  - "Consent version read from NEXT_PUBLIC_CONSENT_TEXT_VERSION env (default 1.0); backend config endpoint wiring deferred to a future plan"
  - "Cancel/reissue visibility inferred from can_reveal_pii (Checker/Admin indicator) + status=issued; server re-enforces RBAC on every call"
  - "Slip uploaded AFTER draft save so a donation id exists for the multipart POST target"
metrics:
  duration_minutes: 120
  tasks_completed: 3
  tasks_total: 4
  files_created: 7
  files_modified: 7
  completed_date: "2026-07-01"
---

# Phase 03 Plan 08: Maker Create/Edit Form + Consent + Slip + PII Reveal Summary

One-liner: The closing frontend slice — Maker create/edit donation form (Screen 2) with zod validation, PDPA consent-required-to-submit, optional slip upload, plus the audited PII-reveal dialog and cancel/void-and-reissue dialogs (with the e-Donation-keyed HIGH-RISK two-textarea warning), all wired to the Go backend from 03-03/03-04/03-06.

## What Was Built

### Task 1 — Donation create/edit form (Screen 2)

**`components/DonationForm.tsx`** — 4-section single-column form (max-680px):
- Section 1 Donor Info: ชื่อ-นามสกุล* , เลขประจำตัว/ภาษี* (13-digit numeric, locked Thai error), ที่อยู่* (Textarea), อีเมล (optional, email format)
- Section 2 Donation Details: จำนวนเงิน* (>0, locked error), วันที่บริจาค* (Calendar Popover, not-future, BE display), หมายเหตุ
- Section 3 slip via `SlipUploadZone`
- Section 4 `ConsentBlock`
- Form actions: "บันทึกร่าง" (outline, no consent validation) + "ส่งรอตรวจสอบ"/"ส่งรอตรวจสอบอีกครั้ง" (accent, full validation, **disabled until consent checked**)
- `beforeunload` dirty guard; return-from-checker amber Alert with `{review_reason}` (extracted from review_history "return" entries) and resubmit CTA
- Uses shadcn Form + react-hook-form + zodResolver; aria-required, FormMessage per Accessibility Contract

**`components/ConsentBlock.tsx`** — PDPA consent checkbox with the locked label and `{consent_text_version}` interpolated; required-to-submit, not required to save draft (D-49).

**`app/donations/new/page.tsx`** + **`app/donations/[id]/edit/page.tsx`** — Server Components defining inline `"use server"` actions (create/update/submit/slip/remove-slip) and wrapping `DonationForm`. Edit page fetches the draft, redirects non-draft records to detail (notFound), and passes `initialData` incl `review_history` for the return alert.

### Task 2 — SlipUploadZone

**`components/SlipUploadZone.tsx`** — dashed-border drop zone (56px min, full width), JPG/PNG/PDF ≤10MB, drag-drop + click-to-browse. Client-side size/type pre-check (UX only). On existing slip: PDF/file icon + "ดูสลิป" (presigned new-tab URL, Screen 5) + "เปลี่ยนสลิป" + "ลบสลิป" (soft-delete D-54). Maps server 422/413 to the locked Thai file-type/size error copy — server magic-byte validation (03-04) is authority (T-03-35).

### Task 3 — PII reveal + cancel/void&reissue dialogs

**`components/RevealPIIDialog.tsx`** — AlertDialog (Screen 4): title, audit-log body, "ยืนยันการเปิดเผย" accent confirm, skeleton loader during fetch.

**`components/MaskedIdField.tsx`** (rewired) — masked by default; Checker/Admin see "เปิดเผยข้อมูล" → RevealPIIDialog → audited `/pii` server action → **session-only** plaintext (client state; reload re-masks, T-03-34) + "ซ่อน" (EyeOff) + audit tooltip; 403 → toast "ไม่มีสิทธิ์เปิดเผยข้อมูลนี้".

**`components/CancelDialog.tsx`** — AlertDialog supporting void (single reason), void when `edonation_keyed=true` (red-50 HIGH-RISK banner + TWO mandatory textareas: reason + RD confirmation), and void&reissue (replacement-record body + same keyed warning when applicable). Confirm blocked until required textareas non-empty; keeps dialog open on 409 `ErrEDonationKeyedConfirmation` with inline error (T-03-36).

**`components/ReviewActionPanel.tsx`** (extended) — added Case 4: issued receipt → "ยกเลิกใบเสร็จ" + "ยกเลิกและออกใบแทน" for Checker/Admin, wiring CancelDialog for both modes.

**`app/donations/[id]/page.tsx`** (extended) — inline server actions `handleRevealPII`, `handleCancel`, `handleReissue`; passes `onRevealAction` to MaskedIdField and cancel/reissue actions to ReviewActionPanel.

**`lib/api.ts`** — `apiFetchFormData` multipart wrapper (no Content-Type; maps 413/415/422/403) + 204 No Content handling.

**`lib/donations.ts`** — `createDonation`, `updateDraft`, `submitDonation`, `uploadSlip`, `viewSlip`, `removeSlip`, `cancelDonation`, `reissueDonation` + request types.

## Verification

- `cd donnarec-web && npm install` — dependencies present (no new packages)
- `npm run build` — **exits 0**; all 6 routes compiled incl `/donations/new` and `/donations/[id]/edit`
- `npx tsc --noEmit` — **No errors found**
- `npm run lint` — **No issues found**

The build/typecheck/lint all ran and passed clean.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] react-hook-form Resolver type mismatch on `national_id`**
- **Found during:** Task 1 build verification
- **Issue:** `national_id` was optional in `FormValues` but the create-mode zod schema made it required, so `zodResolver`'s inferred output type (`national_id: string`) was not assignable to `Resolver<FormValues>`.
- **Fix:** Modelled `national_id` as a required `string` in both `FormValues` and the schema. Edit mode uses `z.string().refine(v => v === "" || 13-digit)` so an empty string legitimately means "keep the existing encrypted value" — this both fixes the type and honors D-44 (tax ID is mandatory).
- **Files modified:** `components/DonationForm.tsx`
- **Commit:** 842e05a

**2. [Rule 3 - Blocking] zod v4 error-map API**
- **Found during:** Task 1 build
- **Issue:** `z.number({ invalid_type_error })` / `z.date({ required_error })` are zod v3 APIs; the project uses zod v4 which uses `{ error }`.
- **Fix:** Switched to `{ error: "..." }` for both.
- **Files modified:** `components/DonationForm.tsx`
- **Commit:** 842e05a

**3. [Rule 3 - Blocking] `npm install` in fresh tree**
- **Found during:** build verification (`next: not found`, exit 127)
- **Issue:** `node_modules` absent. Ran `npm install` from existing `package.json` — no new packages added.

**4. [Rule 1 - Bug] Unused `_donationId` params tripped lint**
- **Found during:** Task 1 lint
- **Issue:** edit-page server actions took an unused `_donationId` while using the closure `id`.
- **Fix:** Actions now use the passed `donationId` argument directly (cleaner + no unused-var).
- **Files modified:** `app/donations/[id]/edit/page.tsx`
- **Commit:** 842e05a

## Threat Mitigations Applied

| Threat ID | Mitigation Applied |
|-----------|-------------------|
| T-03-34 | PII reveal held in client component state only; reload re-masks; server audits every `/pii` call (03-06) |
| T-03-35 | SlipUploadZone client size/type pre-check is UX-only; server magic-byte allowlist (03-04) is authority; 422/413 surfaced with locked copy |
| T-03-36 | CancelDialog forces the RD-confirmation textarea when `edonation_keyed`; dialog stays open on 409 for inline correction; server re-checks |
| T-03-37 | consent_text_version displayed from config (env, default 1.0) and recorded server-side (03-03) |

## Known Stubs

| Stub | File | Reason |
|------|------|--------|
| `consent_text_version` from `NEXT_PUBLIC_CONSENT_TEXT_VERSION` env (default "1.0") | `DonationForm.tsx` / `ConsentBlock.tsx` | Backend config endpoint for the active consent version is a future plan; the value is displayed and sent to the server which records the authoritative snapshot (D-49) |

## TDD Gate Compliance

This is a UI/wiring plan (`type: execute`, not `type: tdd`); no RED/GREEN test gates required. Verification is via build + tsc + lint (all clean).

## Human-Verify Checkpoint (Task 4)

**Status: PENDING HUMAN VERIFICATION.** All code (Tasks 1–3) is committed and the frontend builds/typechecks/lints clean. Task 4 is a `checkpoint:human-verify` gate — the executor does not self-approve. The end-to-end Flow A walkthrough must be run by a human against the live stack (steps returned in the `## CHECKPOINT REACHED` message to the orchestrator).

## Self-Check: PASSED

Files exist:
- `donnarec-web/components/DonationForm.tsx` — FOUND
- `donnarec-web/components/ConsentBlock.tsx` — FOUND
- `donnarec-web/components/SlipUploadZone.tsx` — FOUND
- `donnarec-web/components/RevealPIIDialog.tsx` — FOUND
- `donnarec-web/components/CancelDialog.tsx` — FOUND
- `donnarec-web/app/donations/new/page.tsx` — FOUND
- `donnarec-web/app/donations/[id]/edit/page.tsx` — FOUND

Commits exist:
- `dc31f41` feat(03-08): API client + i18n — FOUND
- `842e05a` feat(03-08): create/edit form + consent — FOUND
- `cf00cfd` feat(03-08): SlipUploadZone — FOUND
- `49e1b5f` feat(03-08): PII reveal + cancel/reissue dialogs — FOUND

Build: `npm run build` exits 0; `npx tsc --noEmit` clean; `npm run lint` clean.
