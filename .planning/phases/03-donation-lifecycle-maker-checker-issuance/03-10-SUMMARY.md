---
phase: 03-donation-lifecycle-maker-checker-issuance
plan: 10
subsystem: frontend-data-layer
tags: [tanstack-query, tanstack-table, bff-proxy, nextjs, contract-alignment]
requires:
  - "03-09: GET /api/donations pagination envelope {data:{items,total,page,per_page}}"
  - "getServerSession(authOptions) + session.accessToken (existing NextAuth wiring)"
provides:
  - "BFF proxy pattern: app/api/bff/** Route Handlers + lib/bff.ts bffForward() (D-R1)"
  - "TanStack Query QueryClientProvider mounted app-wide (app/providers.tsx)"
  - "apiFetch unwraps the {data:...} envelope (D-R2) — reusable by all callers"
  - "Donation list screen (Screen 1) driven by TanStack Query + TanStack Table"
affects:
  - "donnarec-web/lib/api.ts (envelope unwrap changes every apiFetch caller)"
  - "donnarec-web/lib/donations.ts (DonationSummary.amount now string; list key items)"
  - "detail + edit screens (amount-as-string adaptation)"
tech-stack:
  added:
    - "@tanstack/react-query ^5.101.2"
    - "@tanstack/react-table ^8.21.3"
  patterns:
    - "BFF proxy: client TanStack Query → same-origin Next Route Handler → getServerSession Bearer → Go API (token never in browser)"
    - "Envelope unwrap at the apiFetch boundary so callers receive inner payload"
key-files:
  created:
    - donnarec-web/app/providers.tsx
    - donnarec-web/lib/bff.ts
    - donnarec-web/app/api/bff/donations/route.ts
    - donnarec-web/components/DonationListView.tsx
  modified:
    - donnarec-web/package.json
    - donnarec-web/package-lock.json
    - donnarec-web/app/layout.tsx
    - donnarec-web/lib/api.ts
    - donnarec-web/lib/donations.ts
    - donnarec-web/components/DonationTable.tsx
    - donnarec-web/app/donations/page.tsx
    - "donnarec-web/app/donations/[id]/page.tsx"
    - "donnarec-web/app/donations/[id]/edit/page.tsx"
decisions:
  - "D-R1 realised: BFF Route Handler (app/api/bff/donations) forwards a server-side Bearer; token never serialized to the browser"
  - "D-R2 realised: apiFetch unwraps {data:...}; DonationListResponse key donations→items; amount is a numeric string"
  - "amount typed as string on DonationSummary (backend serialises money as string); render sites parseFloat/Number"
metrics:
  tasks: 2
  files_created: 4
  files_modified: 9
  completed: 2026-07-03
requirements: [FR-10]
---

# Phase 3 Plan 10: Frontend List Slice — TanStack Query/Table + BFF Proxy Summary

Migrated the donation LIST screen (Screen 1) to TanStack Query + TanStack Table driving a new same-origin BFF proxy (`/api/bff/donations`) that forwards a server-side Keycloak Bearer to the Go API, and fixed `apiFetch` to unwrap the `{data:{items,total,page,per_page}}` envelope — the root fix for bug #5 (`DonationTable` crashed on `undefined.length` reading `result.donations`).

## What Was Built

### Task 1 — TanStack infra + BFF proxy + apiFetch unwrap (commit `e53aca7`)
- Installed the approved official TanStack packages (`@tanstack/react-query`, `@tanstack/react-table`) after the blocking-human legitimacy gate was approved.
- `app/providers.tsx` (`"use client"`): a stable `QueryClient` (via `useState`) inside `QueryClientProvider`; mounted in `app/layout.tsx` inside `AuthSessionProvider`, wrapping `AppShell`.
- `lib/bff.ts` `bffForward(request, goPath)`: calls `getServerSession(authOptions)`, returns 401 JSON when no `accessToken`, otherwise forwards method/body with `Authorization: Bearer` to `${API_BASE_URL}${goPath}` and passes the Go response (status + JSON) through. The token is added server-side only — never serialized into the browser response (T-10-01).
- `app/api/bff/donations/route.ts` `GET`: forwards the incoming query string verbatim to Go `/api/donations`, returning the `{data:{items,total,page,per_page}}` envelope unchanged (D-R2).
- `lib/api.ts` `apiFetch`: unwraps the envelope — when the parsed body has a `data` key, returns `body.data`; 204 still returns undefined; error switch untouched.

### Task 2 — List screen migration (commit `f68b176`)
- `lib/donations.ts`: `DonationListResponse.donations` → `items`; `DonationSummary.amount` → `string`; extracted `buildDonationQuery`; added client `fetchDonations(filter)` that fetches the same-origin BFF route and unwraps `.data`.
- `components/DonationListView.tsx` (`"use client"`): `useQuery({ queryKey: ["donations", filter], queryFn: () => fetchDonations(filter) })`; loading skeleton; UI-SPEC error alert on `isError`; hands `items/total/page/perPage` to `DonationTable`.
- `components/DonationTable.tsx`: rebuilt with `@tanstack/react-table` (`useReactTable` + `getCoreRowModel` + `flexRender`) while preserving the exact UI-SPEC Screen 1 columns/order (วันที่บริจาค / ชื่อผู้บริจาค / จำนวนเงิน right-aligned / สถานะ StatusBadge / เลขที่ใบเสร็จ issued|cancelled-only / ผู้สร้าง / จัดการ), 56px rows, receipt-cell rules, `viewerId` draft-routing, shadcn Pagination; `formatAmount` now `parseFloat`s the string amount.
- `app/donations/page.tsx`: removed the server-side `searchDonations` call and the `result.donations` crash site; renders `<DonationListView filter viewerId />` (server still decodes `viewerId` + renders `DonationFilterBar`).

## Verification Evidence

- `cd donnarec-web && npx tsc --noEmit` → **No errors found**.
- `cd donnarec-web && npm run build` → **Compiled successfully**; `/api/bff/donations` route registered; `/donations` builds with no `undefined.length` type/runtime break.
- `grep -rn "result.donations" app components lib` → only comment references remain (no property access).
- `useReactTable` present in `DonationTable.tsx`; `useQuery` + `/api/bff/donations` present in `DonationListView.tsx`; `getServerSession` present in the BFF path via `lib/bff.ts`.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Adapt detail + edit screens to `amount: string`**
- **Found during:** Task 2
- **Issue:** `DonationDetail extends DonationSummary`, so retyping `amount` to `string` (per the plan) rippled into `app/donations/[id]/page.tsx` (`formatAmount(number)`) and `app/donations/[id]/edit/page.tsx` (`initialData.amount: number`), which would fail `npm run build` — a hard acceptance criterion.
- **Fix:** detail `formatAmount` now accepts a string and `parseFloat`s it; edit page passes `Number(donation.amount)` to the form model.
- **Files modified:** `donnarec-web/app/donations/[id]/page.tsx`, `donnarec-web/app/donations/[id]/edit/page.tsx`
- **Commit:** `f68b176`
- **Scope note:** these two files are outside the plan's declared file list but the change was directly caused by the in-scope type change and required to keep the build green. The detail/edit screens' own contract migration remains 03-12 scope.

## Checkpoints

- **Task 0 (checkpoint:human-verify, gate=blocking-human — package legitimacy):** APPROVED by the user. `@tanstack/react-query` and `@tanstack/react-table` confirmed as the official TanStack packages before install.
- **Final (checkpoint:human-verify, gate=blocking — live UI walkthrough):** auto-advanced under `workflow.auto_advance=true`. Automated evidence (tsc + `npm run build` + route registration) captured above. Per `.planning/CONVENTIONS.md` integration-test gate, the **live human browser walkthrough remains a recommended manual gate** before Phase 3 is formally re-closed: bring the full stack up (Go API + Postgres + Keycloak + web), sign in as staff, and confirm at `/donations`: (a) table renders rows / Thai empty-state with no crash; (b) DevTools → Network shows the list request goes to `/api/bff/donations` same-origin and the response body contains NO `access_token`/Bearer; (c) filter by name/status/date/receipt + pagination update the table; (d) no national IDs in list rows (PII-free per D-53).

## Known Stubs

None — the list screen renders live Go-API data through the BFF proxy; no hardcoded/placeholder data introduced.

## Threat Flags

None — no new security surface beyond the BFF route already modelled in the plan's threat register (T-10-01..T-10-SC).

## Self-Check: PASSED

- Created files verified present: `app/providers.tsx`, `lib/bff.ts`, `app/api/bff/donations/route.ts`, `components/DonationListView.tsx`.
- Commits verified: `e53aca7` (Task 1), `f68b176` (Task 2) exist in git log.
