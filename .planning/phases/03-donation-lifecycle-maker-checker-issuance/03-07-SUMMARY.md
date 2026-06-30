---
phase: 03-donation-lifecycle-maker-checker-issuance
plan: "07"
subsystem: frontend-donations
tags: [next-js, donations, maker-checker, sod, masked-id, i18n, server-components]
dependency_graph:
  requires: ["03-02"]
  provides: ["03-08"]
  affects: []
tech_stack:
  added: []
  patterns:
    - "Server Components with inline server actions passed as props to Client Components"
    - "Server-computed auth flags in API response (viewer_is_creator, can_approve, can_return, can_reject) — avoids JWT role decode in frontend"
    - "JWT sub claim decoded in page.tsx via base64url without library (server-side only)"
    - "Dialog (reversible) vs AlertDialog (terminal) for return vs reject reason collection"
    - "SoD enforcement: controls removed from DOM entirely (not disabled) when viewer is creator on pending_review"
    - "D-53: search scope is name/date/status/receipt_no ONLY — no tax/national ID search"
key_files:
  created:
    - donnarec-web/lib/donations.ts
    - donnarec-web/components/DonationFilterBar.tsx
    - donnarec-web/components/DonationTable.tsx
    - donnarec-web/components/ReviewReasonDialog.tsx
    - donnarec-web/components/SoDBlockedAlert.tsx
    - donnarec-web/components/ReviewActionPanel.tsx
    - donnarec-web/components/MaskedIdField.tsx
    - donnarec-web/app/donations/page.tsx
    - donnarec-web/app/donations/[id]/page.tsx
  modified:
    - donnarec-web/components/AppShell.tsx
    - donnarec-web/messages/th.json
    - donnarec-web/messages/en.json
    - donnarec-web/package-lock.json
decisions:
  - "ReviewActionPanel extracted as unplanned Client Component to manage dialog open state and useToast — keeping [id]/page.tsx a clean Server Component (Rule 2)"
  - "Server actions defined inline in [id]/page.tsx with 'use server' and passed as props; no separate actions.ts file needed at this scope"
  - "initialFocus prop removed from Calendar (react-day-picker v9 dropped it — Rule 1 fix)"
  - "<a> elements in AppShell.tsx replaced with Next.js Link after ESLint no-html-link-for-pages fired when /donations route was created (Rule 3)"
  - "Toaster added to AppShell.tsx (Rule 2 — ReviewActionPanel needs Toaster in DOM for success/error feedback)"
metrics:
  duration_minutes: 90
  tasks_completed: 3
  tasks_total: 3
  files_created: 9
  files_modified: 4
  completed_date: "2026-06-30"
---

# Phase 03 Plan 07: Donation List & Detail Frontend Summary

One-liner: Next.js 15 server-component donation list (Screen 1) and detail/review page (Screen 3) with SoD enforcement, masked national ID, and mandatory-reason dialogs for approve/return/reject.

## What Was Built

### Task 1 — Donations list + search screen (Screen 1)

**`donnarec-web/lib/donations.ts`** — API wrapper with full type definitions:
- `DonationStatus` enum, `DonationSummary`, `DonationListResponse`, `ReviewHistoryEntry`
- `DonationDetail` extending `DonationSummary` with server-computed auth flags: `viewer_is_creator`, `can_approve`, `can_return`, `can_reject`, `can_reveal_pii`
- Functions: `searchDonations(filter)`, `getDonation(id)`, `approve(id)`, `returnForEdit(id, reason)`, `reject(id, reason)`, `revealPII(id)`

**`donnarec-web/components/DonationFilterBar.tsx`** — D-53 compliant filter bar:
- Fields: donor name Input, date-range two Calendar Popovers, status Select with "all" sentinel, receipt_no Input
- 44px min-h touch targets throughout (WCAG)
- "ค้นหา" updates URL params; "ล้างการกรอง" navigates to /donations bare

**`donnarec-web/components/DonationTable.tsx`** — Shadcn Table per UI-SPEC Screen 1:
- Columns: วันที่บริจาค / ชื่อผู้บริจาค / จำนวนเงิน (right-aligned, Intl.NumberFormat th-TH) / สถานะ (StatusBadge) / เลขที่ใบเสร็จ (issued/cancelled only, em-dash otherwise, cancelled shows strikethrough + "(ยกเลิก)") / ผู้สร้าง / จัดการ
- Row routing: Maker own draft → `/donations/${id}/edit`, else → `/donations/${id}`
- Shadcn Pagination 20/page; empty states for no-records vs no-search-results

**`donnarec-web/app/donations/page.tsx`** — Server Component:
- Async `searchParams` (Next.js 15 pattern)
- JWT `sub` decoded server-side via Buffer.from base64url — no auth.ts changes
- Error boundary state, CTA "สร้างรายการบริจาค" links to `/donations/new`

### Task 2 — Detail view + review action panel + SoD blocked state (Screen 3)

**`donnarec-web/app/donations/[id]/page.tsx`** — Server Component:
- Fetches `getDonation(id)`, maps 404 to `notFound()`
- Inline server actions with `"use server"` directive: `handleApprove()`, `handleReturn(reason)`, `handleReject(reason)` — each calls API then `revalidatePath`
- Two-column layout: left 60% (StatusBadge, receipt block visible only on issued/cancelled, donor dl, MaskedIdField, slip link, consent row, `<details>` review history accordion, replace-chain links), right 40% ReviewActionPanel
- Buddhist Era dates via `Intl.DateTimeFormat("th-TH-u-ca-buddhist", ...)`

**`donnarec-web/components/ReviewReasonDialog.tsx`** — Reusable confirmation dialog:
- `variant: "return"` → Dialog (reversible); `variant: "reject"` → AlertDialog (terminal)
- Mandatory reason textarea — confirm blocked until non-empty; `errors.reasonRequired` shown
- "return" → outline confirm button; "reject" → destructive confirm button

**`donnarec-web/components/SoDBlockedAlert.tsx`** — Server Component:
- Amber alert (bg-amber-50, AlertTriangle, text-amber-700)
- Message: "คุณเป็นผู้สร้างรายการนี้ — ผู้อนุมัติต้องเป็นบุคคลอื่นตามหลักการแยกหน้าที่ (Segregation of Duties)"

**`donnarec-web/components/ReviewActionPanel.tsx`** (UNPLANNED — Rule 2):
- Client Component managing dialog open state and useTransition
- Three cases: draft-owner → Edit button; pending_review creator → SoDBlockedAlert only (controls absent from DOM per T-03-31); pending_review non-creator with permissions → approve/return/reject buttons
- `router.refresh()` after successful action; `useToast` for success/error feedback

### Task 3 — MaskedIdField + i18n strings

**`donnarec-web/components/MaskedIdField.tsx`**:
- Displays `maskedValue` in `font-mono text-[14px]`
- `aria-label="เลขประจำตัว (ซ่อน)"`
- Shows "เปิดเผยข้อมูล" (outline blue) when `canReveal=true`; `onReveal` prop exposed for 03-08

**`donnarec-web/messages/th.json` and `en.json`**:
- Added `donations.title`, full `detail.*` namespace (receiptLabel, cancelledSuffix, viewSlip, consentGiven with `{date}`, reviewHistory, replacesLink, replacedByLink, actionPanel, editButton, informationPanel, backToList)
- Added `errors.reasonRequired`
- Verified `errors.sod`, `errors.statusConflict` already present

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] `initialFocus` prop removed from Calendar components**
- **Found during:** Task 1 — build failure
- **Issue:** `react-day-picker` v9 (shadcn default) dropped the `initialFocus` prop; TypeScript error blocked build
- **Fix:** Removed `initialFocus` from both Calendar Popover instances in DonationFilterBar.tsx
- **Files modified:** `donnarec-web/components/DonationFilterBar.tsx`
- **Commit:** 2e01789

**2. [Rule 3 - Blocking] `npm install` run in worktree**
- **Found during:** Task 1 — `next: not found` (exit 127) when running build verification
- **Issue:** Fresh git worktree has no `node_modules`
- **Fix:** `npm install` in `donnarec-web/` — packages sourced from existing `package.json`, no new packages added
- **Files modified:** `donnarec-web/package-lock.json` (committed)
- **Commit:** 2e01789

**3. [Rule 3 - Blocking] `<a>` → `<Link>` in AppShell.tsx**
- **Found during:** Task 1 — ESLint `@next/next/no-html-link-for-pages` error fired after `/donations` route was created
- **Issue:** AppShell.tsx used raw `<a href="/donations">` which violates Next.js lint rule once the route exists
- **Fix:** Added `import Link from "next/link"` and replaced both `<a>` elements with `<Link>`
- **Files modified:** `donnarec-web/components/AppShell.tsx`
- **Commit:** 2e01789

**4. [Rule 2 - Missing critical functionality] `ReviewActionPanel.tsx` extracted as separate Client Component**
- **Found during:** Task 2 — implementation planning
- **Issue:** `[id]/page.tsx` is a Server Component; dialog open state, `useTransition`, and `useToast` require a Client Component boundary
- **Fix:** Extracted `ReviewActionPanel.tsx` as a dedicated Client Component that receives server actions as props
- **Files modified:** `donnarec-web/components/ReviewActionPanel.tsx` (new)
- **Commit:** 9941dc9

**5. [Rule 2 - Missing critical functionality] `<Toaster />` added to AppShell.tsx**
- **Found during:** Task 2 — implementation of ReviewActionPanel
- **Issue:** `useToast` requires `<Toaster />` rendered in the layout DOM; without it, toasts are silently swallowed
- **Fix:** Added `import { Toaster } from "@/components/ui/toaster"` and `<Toaster />` to AppShell.tsx
- **Files modified:** `donnarec-web/components/AppShell.tsx`
- **Commit:** 2e01789

## Threat Mitigations Applied

| Threat ID | Mitigation Applied |
|-----------|-------------------|
| T-03-31 | SoD: `viewer_is_creator && status === 'pending_review'` → approve/return/reject absent from DOM (not disabled); 403 from API mapped to error toast |
| T-03-32 | MaskedIdField renders masked by default; `onReveal` prop is a no-op stub here — full audited reveal flow is 03-08 |
| T-03-33 | 409 from API surfaced as "reload page" toast via `useToast` in ReviewActionPanel |

## Known Stubs

| Stub | File | Reason |
|------|------|--------|
| `onReveal` prop is a no-op | `MaskedIdField.tsx` | Full PII reveal AlertDialog + `/pii` API call is 03-08 scope; placeholder wired here |
| Row routing to `/donations/new` | `app/donations/page.tsx` | Donation creation form is a future plan; link renders but route does not exist yet |
| Row routing to `/donations/${id}/edit` | `DonationTable.tsx` | Edit form route is a future plan |

## Self-Check: PASSED

Files exist:
- `donnarec-web/lib/donations.ts` — FOUND
- `donnarec-web/components/DonationFilterBar.tsx` — FOUND
- `donnarec-web/components/DonationTable.tsx` — FOUND
- `donnarec-web/components/ReviewReasonDialog.tsx` — FOUND
- `donnarec-web/components/SoDBlockedAlert.tsx` — FOUND
- `donnarec-web/components/ReviewActionPanel.tsx` — FOUND
- `donnarec-web/components/MaskedIdField.tsx` — FOUND
- `donnarec-web/app/donations/page.tsx` — FOUND
- `donnarec-web/app/donations/[id]/page.tsx` — FOUND

Commits exist:
- `2e01789` feat(03-07): donations list page — FOUND
- `9941dc9` feat(03-07): donation detail page — FOUND
- `8caec6b` feat(03-07): MaskedIdField component and i18n keys — FOUND

Build verification: `npm run build` exits 0 (all 5 static pages generated, /donations and /donations/[id] dynamic routes compiled successfully).
