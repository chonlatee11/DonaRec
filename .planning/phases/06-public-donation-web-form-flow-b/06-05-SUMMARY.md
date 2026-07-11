---
phase: 06-public-donation-web-form-flow-b
plan: 05
subsystem: ui
tags: [nextjs, app-router, route-groups, tailwind, css-variables, shadcn, next-font, turnstile]

# Dependency graph
requires:
  - phase: 06-public-donation-web-form-flow-b
    provides: 06-UI-SPEC.md dual-theme architecture contract, 06-RESEARCH.md package legitimacy audit
provides:
  - "app/(app)/ route group wrapping every existing authenticated page in AppShell, URLs unchanged"
  - "app/(public)/ route group (layout + public-theme.css) rendering unauthenticated pages with a scoped .theme-public warm token layer, zero leakage into (app)"
  - "tailwind.config.ts colors.public.* additive namespace (pine/pine-2/sage/gold/gold-wash/ink/line)"
  - "app/fonts.ts — Trirong/IBM Plex Sans Thai/IBM Plex Mono next/font instances, applied only in (public)"
  - "@marsidev/react-turnstile@1.5.3 npm dependency"
affects: [06-06, 06-07, 06-08, public-donation-form, post-submit-confirmation, pending-review-queue]

# Tech tracking
tech-stack:
  added: ["@marsidev/react-turnstile@1.5.3", "next/font/google: Trirong, IBM_Plex_Sans_Thai, IBM_Plex_Mono"]
  patterns:
    - "scoped CSS variable layer (.theme-public) reusing shadcn semantic var names, same mechanism as the existing .dark class"
    - "next/font instances shared across route-group layout files via a plain module (app/fonts.ts), never exported from a layout.tsx file"

key-files:
  created:
    - donnarec-web/app/(app)/layout.tsx
    - donnarec-web/app/(public)/layout.tsx
    - donnarec-web/app/(public)/public-theme.css
    - donnarec-web/app/fonts.ts
  modified:
    - donnarec-web/app/layout.tsx
    - donnarec-web/tailwind.config.ts
    - donnarec-web/package.json

key-decisions:
  - "app/(public)-only next/font instances live in a new app/fonts.ts module, not directly in app/layout.tsx as the plan literally said — Next.js validates layout.tsx exports against a fixed shape and rejects arbitrary named exports (Rule 3 blocking-issue fix)"
  - "HSL triplets for the warm palette were recomputed from the locked hex values with a converter rather than copied from 06-UI-SPEC.md's own HSL column, which it explicitly flagged as needing re-verification — found and corrected one transposed-digit typo (destructive text: 32 -> 3.2)"
  - "Added --public-destructive-wash (light rose #FBEAE7) as an extra token beyond the plan's literal --public-* list, since the UI-SPEC destructive Color row pairs a text color with this bg wash for CAPTCHA/rate-limit/validation banners built in later plans (Rule 2)"
  - "Also redefined --card-foreground/--popover-foreground to ink inside .theme-public even though the plan/UI-SPEC's literal shadcn-var list omitted them — without it, Card/Popover text in (public) would silently fall back to (app)'s slate-900 ink instead of the locked warm ink (Rule 1 rendering-fidelity bug)"
  - "tailwind colors.public.pine maps to a dedicated --public-pine var (not var(--primary)) for literal consistency with the plan's 'mapping to the var(--public-*) tokens' instruction, even though its value duplicates --primary"

patterns-established:
  - "Any future route needing the warm theme wraps its content inside app/(public)/ — the layout already provides .theme-public + font variables with no further wiring"

requirements-completed: [NFR-06, FR-06]

coverage:
  - id: D1
    description: "app/(app)/ route group renders every existing authenticated page (/donations, /e-donation, /reports, /admin, /) inside AppShell with unchanged URLs and unchanged slate/blue :root tokens"
    requirement: "FR-06"
    verification:
      - kind: e2e
        ref: "cd donnarec-web && npm run build (route manifest lists /, /admin/settings, /donations, /donations/[id], /donations/[id]/edit, /donations/new, /e-donation, /reports unchanged)"
        status: pass
      - kind: other
        ref: "git diff donnarec-web/app/globals.css (empty — :root untouched)"
        status: pass
    human_judgment: false
  - id: D2
    description: "app/(public)/ route group renders with no AppShell/SignOutButton/role checks, wrapped in a .theme-public scope carrying the locked warm palette, 18px radius, WCAG-passing pine focus ring, and the three public fonts"
    requirement: "NFR-06"
    verification:
      - kind: other
        ref: "cd donnarec-web && npm run build (compiles clean; no page exists under (public) yet so no route is rendered this plan — visual verification deferred to the plan that adds app/(public)/donate/page.tsx)"
        status: pass
    human_judgment: true
    rationale: "No (public) page exists yet in this plan (Screen 9 ships in a later plan of this phase) — there is nothing on-screen to visually inspect; the scoped-CSS-variable correctness (HSL values, isolation from :root) needs a human/checker pass once a real page renders through .theme-public."
  - id: D3
    description: "tailwind.config.ts colors.public.* namespace and @marsidev/react-turnstile@1.5.3 dependency added additively"
    requirement: "FR-06"
    verification:
      - kind: other
        ref: "git diff donnarec-web/tailwind.config.ts (only an added block, no line changes to colors.primary/secondary/etc.); grep '@marsidev/react-turnstile' donnarec-web/package.json"
        status: pass
    human_judgment: false

duration: 15min
completed: 2026-07-11
status: complete
---

# Phase 06 Plan 05: Dual-theme route-group split Summary

**Split `app/layout.tsx` into `(app)` (AppShell, slate/blue, unchanged) and `(public)` (scoped `.theme-public` warm cream/pine/gold token layer) route groups, with the three public fonts and `@marsidev/react-turnstile` wired in but not yet consumed by any page.**

## Performance

- **Duration:** ~15 min
- **Tasks:** 2 completed
- **Files modified:** 9 (4 created, 5 modified)

## Accomplishments
- `app/(app)/layout.tsx` now renders `AppShell`; `donations/`, `e-donation/`, `reports/`, `admin/`, and the root landing redirect moved into `app/(app)/` via `git mv` (URLs unchanged — verified in the build's route manifest)
- `app/layout.tsx` reduced to html/body + Sarabun/Inter font vars + `NextIntlClientProvider` + `AuthSessionProvider` + `Providers` — no more unconditional `AppShell` wrap
- `app/(public)/layout.tsx` renders a minimal placeholder header (to be replaced by `PublicHeader` in a later plan) wrapped in a `.theme-public` div, with zero session/role logic
- `app/(public)/public-theme.css` defines `.theme-public` overriding every listed shadcn semantic variable with the locked warm palette (HSL recomputed and verified against the hex source of truth), `--radius: 18px`, a WCAG-1.4.11-passing pine (not gold) `:focus-visible` outline, 44px input/select/textarea min-height, and the `--public-*` extra tokens + `--shadow-public-card`
- `tailwind.config.ts` gains an additive `colors.public.{pine,pine-2,sage,gold,gold-wash,ink,line}` namespace; `colors.primary/secondary/muted/accent/destructive/etc.` are byte-identical to before
- `app/fonts.ts` instantiates Trirong (500+600 normal, 400 italic), IBM Plex Sans Thai (400+600), and IBM Plex Mono (400+500) once; only `app/(public)/layout.tsx` applies their `.variable` classes
- `@marsidev/react-turnstile@1.5.3` added to `package.json`/`package-lock.json` (legitimacy pre-vetted OK in `06-RESEARCH.md`, no human checkpoint needed per plan)

## Task Commits

Each task was committed atomically:

1. **Task 1: Split root layout into (app) and (public) route groups** - `972a2a8` (feat)
2. **Task 2: Warm theme scope (public-theme.css) + additive tailwind namespace + fonts + Turnstile dep** - `f71968a` (feat)

## Files Created/Modified
- `donnarec-web/app/(app)/layout.tsx` - renders `AppShell` around authenticated children (new)
- `donnarec-web/app/(public)/layout.tsx` - renders unauthenticated children wrapped in `.theme-public` + placeholder header + public font variables (new)
- `donnarec-web/app/(public)/public-theme.css` - the `.theme-public` scoped warm-token CSS variable block (new)
- `donnarec-web/app/fonts.ts` - Trirong/IBM Plex Sans Thai/IBM Plex Mono `next/font/google` instances (new)
- `donnarec-web/app/layout.tsx` - reduced to theme-neutral root shell, no `AppShell`
- `donnarec-web/tailwind.config.ts` - additive `colors.public.*` namespace
- `donnarec-web/package.json` / `package-lock.json` - `@marsidev/react-turnstile@1.5.3`
- `donnarec-web/app/(app)/{admin,donations,e-donation,reports}/**`, `donnarec-web/app/(app)/page.tsx` - moved via `git mv`, no content changes

## Decisions Made
- Public fonts moved to a dedicated `app/fonts.ts` module instead of living directly in `app/layout.tsx` — Next.js's layout-export-shape validation rejects arbitrary named exports from a `layout.tsx` file (build error: `"trirong" is not a valid Layout export field`), discovered empirically while implementing Task 2.
- Recomputed every HSL triplet from the locked hex palette with a converter rather than trusting `06-UI-SPEC.md`'s own HSL column, per its own explicit "re-verify before pasting" flag — this caught a real typo (destructive text row: spec listed `32 71% 41%`, correct value is `3.2 71.3% 41%`).
- Added `--public-destructive-wash` and re-defined `--card-foreground`/`--popover-foreground` inside `.theme-public` beyond the plan's literal enumerated variable list — both are Rule 1/2 fixes for rendering-fidelity gaps that would otherwise surface once a real (public) page renders (see Deviations below).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Moved public font instantiation out of app/layout.tsx into app/fonts.ts**
- **Found during:** Task 2 (adding Trirong/IBM Plex Sans Thai/IBM Plex Mono)
- **Issue:** The plan's action text said to add the three fonts "via next/font/google" in `app/layout.tsx`. Exporting them as named consts from that file failed Next.js's build-time layout validation: `Type error: Layout "app/layout.tsx" does not match the required types of a Next.js Layout. "trirong" is not a valid Layout export field.`
- **Fix:** Created `app/fonts.ts` (a plain module, not a Next.js route file) holding the three `next/font/google` calls; `app/(public)/layout.tsx` imports from it and applies `.variable` classNames on its wrapper div, matching `06-UI-SPEC.md`'s literal code example (which already showed the variable classes applied on the `(public)` div, not `<html>`).
- **Files modified:** `donnarec-web/app/fonts.ts` (new), `donnarec-web/app/layout.tsx`, `donnarec-web/app/(public)/layout.tsx`
- **Verification:** `npm run build` compiles clean (previously failed with the type error above)
- **Committed in:** `f71968a` (Task 2 commit)

**2. [Rule 2 - Missing Critical] Added --public-destructive-wash token**
- **Found during:** Task 2 (writing public-theme.css from the UI-SPEC Color table)
- **Issue:** The UI-SPEC's destructive Color row locks TWO hex values (`#B3261E` text on `#FBEAE7` bg) for error/blocking banners (CAPTCHA failure, rate-limit banner, slip-required error, field validation), but the plan's literal `--public-*` token enumeration only listed pine-2/sage/gold/gold-wash/ink/line — no destructive wash. Without it, a later plan building those banners would have no locked token for the light error background and would have to hand-derive or re-guess it.
- **Fix:** Added `--public-destructive-wash: 9 71.4% 94.5%` (from `#FBEAE7`) alongside the other extra tokens.
- **Files modified:** `donnarec-web/app/(public)/public-theme.css`
- **Verification:** `npm run build` passes; value matches the UI-SPEC-locked hex via the same recompute script used for all other tokens
- **Committed in:** `f71968a` (Task 2 commit)

**3. [Rule 1 - Bug] Defined --card-foreground/--popover-foreground inside .theme-public**
- **Found during:** Task 2 (reviewing which shadcn primitives read which vars)
- **Issue:** `components/ui/card.tsx` and `components/ui/popover.tsx` explicitly use `text-card-foreground`/`text-popover-foreground` Tailwind utilities. The plan's/UI-SPEC's literal shadcn-var enumeration for `.theme-public` omitted `--card-foreground`/`--popover-foreground`, which would have left them unset in the scope, silently falling back to `:root`'s `(app)` dark-slate value (`222.2 47.4% 11.2%`) instead of the locked warm `ink` (`156.9 22% 11.6%`) — a rendering-fidelity bug on every `(public)` Card/Popover, not a leakage risk but a visible off-palette text color.
- **Fix:** Added `--card-foreground: 156.9 22% 11.6%` and `--popover-foreground: 156.9 22% 11.6%` (both = ink) inside `.theme-public`.
- **Files modified:** `donnarec-web/app/(public)/public-theme.css`
- **Verification:** `npm run build` passes; values match `--foreground`/`ink` per the locked palette
- **Committed in:** `f71968a` (Task 2 commit)

---

**Total deviations:** 3 auto-fixed (1 blocking, 2 missing-critical/rendering-fidelity)
**Impact on plan:** All three are required for the plan's own stated correctness bar (build passing, warm palette rendering exactly as locked) — no scope creep beyond what `06-UI-SPEC.md`'s own Color table and component inventory already implied.

## Issues Encountered
- Empirically confirmed that `app/(public)/layout.tsx`, having no `page.tsx` under it yet, is not included in the webpack bundle graph at all this plan — `npm run build` succeeded even with a syntactically-broken CSS import during an intermediate step of Task 1, before `public-theme.css` existed. TypeScript's own type-check step (separate from webpack bundling) still validates the file's export shape regardless of route reachability, which is what surfaced the Rule 3 deviation above.

## User Setup Required
None - no external service configuration required. (Turnstile site/secret keys are needed by a later plan that wires the actual `TurnstileWidget`/verification route, not this plan.)

## Next Phase Readiness
- The structural prerequisite for every `(public)` screen is in place: route groups, scoped warm theme, additive Tailwind namespace, public fonts, and the Turnstile npm dependency.
- No `(public)` page exists yet — the next plan(s) in this phase (`PublicHeader`, Screen 9 donation form, Screen 10 confirmation) can now build directly under `app/(public)/` and get the warm theme with zero additional CSS wiring.
- Visual verification of the `.theme-public` palette (D2 above) is deferred to whichever later plan first renders a real page through this scope — flagged as `human_judgment: true` in this SUMMARY's coverage block, not a blocker for this plan.

---
*Phase: 06-public-donation-web-form-flow-b*
*Completed: 2026-07-11*

## Self-Check: PASSED

All created files verified present on disk; both task commits (`972a2a8`, `f71968a`) verified present in git log.
