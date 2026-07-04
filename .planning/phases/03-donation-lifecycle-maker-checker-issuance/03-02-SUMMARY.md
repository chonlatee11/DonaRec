---
phase: 03-donation-lifecycle-maker-checker-issuance
plan: "02"
subsystem: frontend
tags: [next.js, shadcn, tailwind, next-intl, next-auth, keycloak, i18n, design-system]
dependency_graph:
  requires: []
  provides:
    - donnarec-web Next.js 15 app shell
    - Tailwind CSS v3 + shadcn v4 design system (slate base, blue-600 primary)
    - Sarabun + Inter fonts wired via next/font/google
    - next-intl th/en i18n (cookie-based locale, no route segments)
    - Keycloak bearer token API client (lib/api.ts + lib/auth.ts)
    - StatusBadge with all 5 locked UI-SPEC color tokens
    - AppShell + LocaleSwitcher layout shell
  affects:
    - 03-07 (donation form builds on this scaffold)
    - 03-08 (donation list/detail builds on this scaffold)
tech_stack:
  added:
    - "Next.js 15.5.19 (App Router)"
    - "React 19"
    - "Tailwind CSS v3.4.17"
    - "shadcn v4.12 (slate base, cssVariables=true)"
    - "next-intl v3.26+ (cookie-based locale, non-routing)"
    - "next-auth v4.24 + Keycloak provider"
    - "lucide-react (icons)"
    - "react-hook-form + zod (installed by shadcn for form component)"
    - "date-fns + react-day-picker (installed by shadcn for calendar)"
  patterns:
    - "Server Component default; 'use client' only at interaction boundary (LocaleSwitcher)"
    - "getTranslations() in server components; useTranslations() in client components"
    - "apiFetch() server-side only; token from getServerSession(authOptions)"
    - "setLocale server action → router.refresh() for locale switch"
key_files:
  created:
    - donnarec-web/package.json
    - donnarec-web/next.config.ts
    - donnarec-web/tsconfig.json
    - donnarec-web/tailwind.config.ts
    - donnarec-web/postcss.config.mjs
    - donnarec-web/components.json
    - donnarec-web/eslint.config.mjs
    - donnarec-web/app/globals.css
    - donnarec-web/app/layout.tsx
    - donnarec-web/app/page.tsx
    - donnarec-web/app/api/auth/[...nextauth]/route.ts
    - donnarec-web/lib/utils.ts
    - donnarec-web/lib/auth.ts
    - donnarec-web/lib/api.ts
    - donnarec-web/lib/locale-action.ts
    - donnarec-web/i18n/request.ts
    - donnarec-web/messages/th.json
    - donnarec-web/messages/en.json
    - donnarec-web/types/next-auth.d.ts
    - donnarec-web/types/css.d.ts
    - donnarec-web/components/AppShell.tsx
    - donnarec-web/components/LocaleSwitcher.tsx
    - donnarec-web/components/StatusBadge.tsx
    - "donnarec-web/components/ui/ (19 shadcn components)"
    - donnarec-web/hooks/use-toast.ts
    - donnarec-web/.gitignore
decisions:
  - "Used Tailwind v3 (not v4) for full shadcn v4 compatibility; shadcn install added Radix UI + RHF + zod + date-fns automatically"
  - "Cookie-based locale (i18n/request.ts reads 'locale' cookie) instead of [locale] route segments — matches plan's app/layout.tsx file structure"
  - "next-auth v4 chosen over Auth.js v5 — stable, well-documented Keycloak provider, no breaking changes risk"
  - "apiFetch is server-side only (uses getServerSession); client-side fetch will use session hook in future plans"
  - "Built project manually (not create-next-app) due to npm 11 flag-interception incompatibility"
metrics:
  duration: "~45 minutes"
  completed_date: "2026-06-30"
  tasks_completed: 3
  tasks_total: 3
  files_created: 46
---

# Phase 03 Plan 02: donnarec-web Frontend Bootstrap Summary

Next.js 15 App Router app bootstrapped from scratch with Tailwind v3 + shadcn v4, next-intl (th/en cookie locale), Keycloak bearer API client, and locked UI-SPEC design tokens — StatusBadge + AppShell + LocaleSwitcher.

## Tasks Completed

| Task | Name | Commit | Key Files |
|------|------|--------|-----------|
| 1 | Scaffold Next.js 15 + Tailwind + shadcn + fonts | e56973c | package.json, next.config.ts, tailwind.config.ts, globals.css, layout.tsx, components.json, components/ui/ (19 files) |
| 2 | i18n + authenticated API client + Keycloak bearer | 92f20a3 | i18n/request.ts, messages/th.json, messages/en.json, lib/auth.ts, lib/api.ts, lib/locale-action.ts, app/api/auth/[...nextauth]/route.ts |
| 3 | AppShell + LocaleSwitcher + StatusBadge | c0ae536 | components/AppShell.tsx, components/LocaleSwitcher.tsx, components/StatusBadge.tsx, app/layout.tsx (updated) |

## Verification Evidence

- `npm run build` exits 0 — Next.js 15.5.19, 3 routes compiled
- `npx tsc --noEmit` exits 0 — TypeScript strict mode, no errors
- `npm run lint` exits 0 — ESLint no issues found

## Key Decisions

1. **Manual project scaffolding** — `create-next-app` fails with npm 11 (flag-interception regression). Project created manually with identical output; all config matches what create-next-app produces.

2. **Tailwind v3 (not v4)** — used to ensure full shadcn v4 compatibility. shadcn's component files reference Tailwind v3 class patterns. v4 migration is a separate concern.

3. **Cookie-based i18n locale** — `i18n/request.ts` reads a `locale` cookie. No `[locale]` route segments needed for the back-office bootstrap. LocaleSwitcher calls `setLocale` server action and `router.refresh()`.

4. **next-auth v4 stable** over Auth.js v5 beta — v4 Keycloak provider is production-proven; v5 is still evolving.

5. **apiFetch server-side only** — `getServerSession` requires server context. Client-side data fetching in 03-07/03-08 will use `useSession` hook or route handlers.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] npm 11 incompatibility with create-next-app**
- **Found during:** Task 1
- **Issue:** `npx create-next-app@15 donnarec-web --typescript --tailwind ...` — npm 11 intercepts all flags as npm config options before forwarding to create-next-app, causing ENOENT on package.json
- **Fix:** Built the project structure manually with equivalent config files
- **Files modified:** All donnarec-web/ config files created manually

**2. [Rule 1 - Bug] Type error in i18n/request.ts — AbstractIntlMessages**
- **Found during:** Task 2 (first build attempt)
- **Issue:** `messages: (await import(...)).default as Record<string, unknown>` — next-intl's `RequestConfig.messages` requires `AbstractIntlMessages` not `Record<string, unknown>`
- **Fix:** Added `import type { AbstractIntlMessages } from "next-intl"` and cast with the correct type
- **Commit:** Fixed before Task 2 commit

**3. [Rule 1 - Bug] Typo in lib/api.ts — `error.error`**
- **Found during:** Task 2 (second build attempt)
- **Issue:** `super(error.error ?? error.message)` — `ApiError` has no `.error` field
- **Fix:** Changed to `super(error.message)`
- **Commit:** Fixed before Task 2 commit

**4. [Rule 3 - Blocking] ESLint scanning .next/ compiled output**
- **Found during:** Task 3 lint verification
- **Issue:** lint ran against 5000+ issues in compiled JS files in .next/
- **Fix:** Added `{ ignores: [".next/**", "node_modules/**", "next-env.d.ts", "components/ui/**", "hooks/**"] }` to eslint.config.mjs
- **Files modified:** donnarec-web/eslint.config.mjs

**5. [Rule 2 - Missing] CSS import tsc compatibility**
- **Found during:** Task 3 tsc check
- **Issue:** `tsc --noEmit` does not have Next.js CSS handling; `import "./globals.css"` causes TS2882
- **Fix:** Created `types/css.d.ts` with `declare module "*.css" {}`
- **Files created:** donnarec-web/types/css.d.ts

**6. [Rule 1 - Bug] tailwind.config.ts used require() instead of ESM import**
- **Found during:** Task 3 lint verification
- **Issue:** `plugins: [require("tailwindcss-animate")]` triggers `@typescript-eslint/no-require-imports`
- **Fix:** Added `import tailwindcssAnimate from "tailwindcss-animate"` at top; used `plugins: [tailwindcssAnimate]`
- **Files modified:** donnarec-web/tailwind.config.ts

**7. [Rule 2 - Missing] NextAuth API route**
- **Found during:** Task 2 implementation
- **Issue:** `lib/auth.ts` alone does not make NextAuth work; the App Router route handler `app/api/auth/[...nextauth]/route.ts` is required for `getServerSession` to function
- **Fix:** Created `app/api/auth/[...nextauth]/route.ts`

**8. [Rule 2 - Missing] Locale server action**
- **Found during:** Task 2 implementation
- **Issue:** LocaleSwitcher (Task 3) needs a server action to write the cookie; not in the plan's file list
- **Fix:** Created `lib/locale-action.ts` with `setLocale(locale)` server action

**9. [Rule 2 - Missing] donnarec-web/.gitignore**
- **Found during:** Task 1 commit
- **Issue:** `.next/` and `node_modules/` would be tracked without a gitignore
- **Fix:** Created `donnarec-web/.gitignore`

## Known Stubs

None — this plan creates the app shell and design system infrastructure only. No donor data, forms, or lists are rendered. All UI surfaces in this plan output static/structural content.

## Threat Flags

| Flag | File | Description |
|------|------|-------------|
| T-03-05 mitigated | lib/api.ts | Bearer token from `getServerSession(authOptions)` — never hardcoded; env for base URL only |
| T-03-06 mitigated | components.json | Official shadcn registry only; no third-party registries added |

## Self-Check: PASSED

- `/donnarec-web/components.json` exists and contains "slate": PASS
- `components/ui/` includes button.tsx, badge.tsx, table.tsx, dialog.tsx, alert-dialog.tsx, calendar.tsx: PASS
- `app/layout.tsx` references `Sarabun` and `Inter`: PASS
- `messages/th.json` contains "รอตรวจสอบ" and "สร้างรายการบริจาค": PASS
- `lib/api.ts` contains "Authorization" and "Bearer": PASS
- `lib/api.ts` maps HTTP 403 and 409: PASS
- `StatusBadge.tsx` references all 5 statuses: PASS
- `StatusBadge.tsx` contains aria-label: PASS
- Commits e56973c, 92f20a3, c0ae536 exist: PASS
- `npm run build` exits 0: PASS
- `npx tsc --noEmit` exits 0: PASS
- `npm run lint` exits 0: PASS
