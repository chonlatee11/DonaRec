---
phase: 06-public-donation-web-form-flow-b
plan: 08
subsystem: ui
tags: [nextjs, app-router, responsive, a11y, mobile-nav, i18n, nfr-06]

# Dependency graph
requires:
  - phase: 06-public-donation-web-form-flow-b
    provides: "plan 05 (app)/(public) route-group split + AppShell slate/blue sidebar; plan 06 public /donate form; plan 07 /queue + DonationTable/AgingTable list surfaces"
provides:
  - "MobileNavDrawer component — <768px slide-in drawer (role=dialog aria-modal, focus trap, Escape/backdrop/X close, body-scroll lock, focus return) wrapping AppShell's shared brand+nav markup"
  - "AppShell responsive retrofit — desktop 256px sidebar hidden below md, header hamburger trigger (aria-expanded/aria-controls), mobile page-padding override"
  - "overflow-x-auto scroll wrappers on DonationTable + AgingTable (wide tables scroll instead of overflowing below 768px)"
affects: []

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "single shared sidebarContent JSX rendered once and reused by BOTH the desktop fixed <aside> (≥768px) and the mobile slide-in drawer (<768px) so the two variants can never diverge — same markup, same role guards (T-06-28)"
    - "self-contained focus-trap drawer (useId drawer id, Tab-cycle trap, Escape close, focus return to trigger) without a dialog library — reuses the app's existing Button primitive only"

key-files:
  created:
    - donnarec-web/components/MobileNavDrawer.tsx
  modified:
    - donnarec-web/components/AppShell.tsx
    - donnarec-web/components/DonationTable.tsx
    - donnarec-web/components/AgingTable.tsx

key-decisions:
  - "sidebarContent extracted once in AppShell and passed as children to MobileNavDrawer — the desktop <aside> and the mobile drawer render the identical brand + role-gated nav markup, so the responsive variant structurally cannot expose a route the desktop nav does not (T-06-28)"
  - "MobileNavDrawer owns BOTH the hamburger trigger (rendered md:hidden inside the AppShell header) and the fixed slide-in panel + backdrop, sharing one open state — the trigger and overlay can never desync; the whole unit is hidden at md+ so the desktop layout is byte-for-byte unchanged"
  - "drawer focus trap + Escape + backdrop-click + X close + focus-return implemented directly (no Radix Dialog dependency) to keep the drawer a thin wrapper around existing nav markup"
  - "tables use an overflow-x-auto wrapper (horizontal scroll) rather than a column-hiding scheme — per UI-SPEC 'Table/filter responsiveness' (no column-hiding scheme)"

# NFR-06 is delivered in CODE only. Its ACCEPTANCE is gated on the Task 2
# human walkthrough (live stack, bilingual, desktop+mobile, real Turnstile),
# which has NOT been performed. Do not treat NFR-06 as validated.
requirements-completed: []
requirements-pending-human-uat: [NFR-06]

coverage:
  - id: D1
    description: "Below 768px the AppShell sidebar collapses into a hamburger-triggered slide-in drawer (backdrop + Escape/X close + focus return); at ≥768px the fixed 256px sidebar is unchanged"
    requirement: "NFR-06"
    verification:
      - kind: build
        ref: "cd donnarec-web && npm run build — green (verified in Task 1, commit a2f2796)"
        status: pass
      - kind: e2e
        ref: "Task 2 human walkthrough step 6 — staff mobile (~375px): sidebar is a hamburger-triggered drawer with backdrop/Escape/focus behavior; queue table scrolls horizontally"
        status: pending
    human_judgment: true
    rationale: "Responsive/a11y drawer behavior on a real ~375px viewport requires a human viewport walkthrough — build-green proves compilation only, not the interaction/layout contract."
  - id: D2
    description: "The public /donate form and the staff /queue are usable and correctly laid out on a ~375px mobile viewport in both Thai and English; wide tables scroll horizontally instead of overflowing"
    requirement: "NFR-06"
    verification:
      - kind: build
        ref: "npm run build green; DonationTable + AgingTable wrapped in overflow-x-auto"
        status: pass
      - kind: e2e
        ref: "Task 2 human walkthrough steps 1–5 + 7 — desktop/mobile × TH/EN on /donate + /queue, real Turnstile challenge (pass + fail path), ack-email language, source badge/filter"
        status: pending
    human_judgment: true
    rationale: "Bilingual/responsive layout fidelity + the real Turnstile challenge require a live local stack driven by a human — cannot be automated or fabricated."

# Metrics
duration: ~15min (Task 1 only; Task 2 is a pending human checkpoint)
completed: 2026-07-12
status: code-complete
---

# Phase 6 Plan 8: Mobile-Nav Retrofit + Table Scroll + Responsive/Bilingual Walkthrough Summary

**NFR-06 CODE is delivered — AppShell now collapses to a focus-trapped mobile nav drawer below 768px (desktop sidebar unchanged) and wide tables scroll horizontally — but NFR-06 ACCEPTANCE remains PENDING the Task 2 human walkthrough (live stack, bilingual, desktop+mobile, real Turnstile), which has NOT been performed.**

## Status: CODE-COMPLETE — human UAT gate NOT yet satisfied

- **Task 1 (auto):** COMPLETE and committed as `a2f2796`. Build verified green.
- **Task 2 (checkpoint:human-verify, blocking):** **PENDING HUMAN UAT.** This is a live-stack, bilingual/responsive human walkthrough that requires the operator to bring up the stack and supply Cloudflare Turnstile keys. It CANNOT be automated and was NOT performed. NFR-06 is therefore NOT validated. See "Outstanding Human-Verification Items" below — these carry into phase-end UAT.

## Performance
- **Duration:** ~15 min (Task 1 code only)
- **Tasks:** 1 of 2 complete (Task 2 is a pending human checkpoint)
- **Files:** 4 (1 created, 3 modified)

## Accomplishments (Task 1 — committed `a2f2796`)
- **MobileNavDrawer.tsx (created)** — a `<768px` slide-in wrapper around AppShell's existing `<aside>` nav markup. Fixed `inset-y-0 left-0`, `min(280px, 85vw)` width, `z-50`, `translate-x-0` when open / `-translate-x-full` when closed with `transition-transform`; a `bg-slate-900/40` backdrop that closes on click; `role="dialog" aria-modal="true" aria-label="เมนูนำทาง"`; Tab focus-trap while open; Escape to close; a 44px X close button; body-scroll lock; focus returns to the hamburger on close. Renders BOTH the hamburger trigger (in the header, `md:hidden`) and the drawer, sharing one `open` state; the whole unit is `hidden` at `md`+. Slate/blue only — no warm theme.
- **AppShell.tsx (modified)** — the brand + role-gated nav links were extracted into a single shared `sidebarContent` JSX rendered once and reused by BOTH the desktop fixed `<aside>` (now `hidden md:flex`, 256px, byte-for-byte unchanged at md+) and the mobile `MobileNavDrawer` (T-06-28: same markup, same role guards, no new route exposed). Header gains a left-side hamburger trigger (`aria-expanded` reflecting drawer state, `aria-controls` → drawer id, 44px) below md and moves the account controls to the right. `<main>` gets the mobile page-padding override (`px-4 md:px-6`).
- **DonationTable.tsx + AgingTable.tsx (modified)** — each table wrapped in an `overflow-x-auto` container so wide tables scroll horizontally below 768px rather than overflowing (per UI-SPEC — no column-hiding scheme).
- **Build:** `cd donnarec-web && npm run build` green.

## Task Commits
1. **Task 1: Mobile nav drawer + table horizontal-scroll wrappers** — `a2f2796` (feat)
2. **Task 2: Responsive + bilingual human walkthrough** — NO COMMIT (pending human checkpoint; not yet performed).

## Outstanding Human-Verification Items (Task 2 — PENDING, carry into UAT)

**This walkthrough has NOT been run. It requires a live local stack (docker compose + seeded users + `donnarec-web/.env.local`) and Cloudflare Turnstile keys (`NEXT_PUBLIC_TURNSTILE_SITE_KEY` + `TURNSTILE_SECRET_KEY` — test keys acceptable). NFR-06 stays PENDING until every step below passes.**

1. **Desktop, Thai:** open `/donate`, complete the form with a valid slip (jpg/png/pdf), check consent, complete the real Turnstile challenge, submit. Confirm the in-page confirmation shows a reference number + the explicit "not yet a receipt" wording, and a copy button works.
2. **Toggle EN on `/donate`:** confirm all labels/intro/consent/confirmation copy switch to English and the submitted `donor_language` follows.
3. **Mobile viewport (~375px):** repeat the `/donate` submission — confirm the card goes full-bleed, fields reflow to single column, inputs are ≥44px, the sticky header + language toggle stay reachable, no horizontal overflow.
4. **Ack email (dev mailer capture):** an acknowledgement email arrived in the submitted language stating "received, not yet a receipt" with the reference number.
5. **Staff, desktop:** open `/queue`, confirm the new submission appears with a "จากเว็บไซต์" source badge; exercise the all / from-website / staff-entered filter; open the record and confirm the source-aware creator label ("ผู้บริจาคส่งเอง (ผ่านเว็บไซต์)").
6. **Staff, mobile (~375px):** confirm the sidebar is a hamburger-triggered drawer (backdrop, Escape, focus behavior) and the queue table scrolls horizontally without breaking layout.
7. **Negative CAPTCHA path:** an unverified/failed challenge surfaces the distinct CAPTCHA error copy, not a field-validation error.

**Resume signal (from the plan):** Type "approved" or describe any responsive/bilingual/flow issue to fix, then re-run this checkpoint.

## Integration-test / UAT gate (CONVENTIONS)
Per the project's integration-test + human-walkthrough gate, this phase touches the runtime request seam (public submit path, queue BFF) and a user-facing responsive surface. Build-green + unit coverage do NOT satisfy the gate. NFR-06 and the phase are NOT Complete until the Task 2 human walkthrough above passes (and the phase-level E2E gate over the real HTTP path is confirmed by the phase verifier). This is deferred to phase-end `/gsd-verify-work`.

## Deviations from Plan
None — Task 1 executed exactly as written. Task 2 was not executed (it is a human checkpoint requiring a live stack + operator-supplied Turnstile keys; it must not be fabricated).

## Threat Flags
None — the mobile drawer renders the SAME nav markup behind the SAME middleware/role guards as the desktop sidebar (T-06-28); no new route, capability, or network surface introduced.

## Known Stubs
None — no mock/placeholder data. The retrofit wraps the existing live nav and tables.

## Next Phase Readiness
- NFR-06 code is in place; the outstanding blocker is the Task 2 human walkthrough (above), which gates NFR-06 acceptance and the phase's Success Criterion 5 (responsive/usable on desktop + mobile, TH/EN).
- No downstream plan depends on plan 08 (`affects: []`).

---
*Phase: 06-public-donation-web-form-flow-b*
*Completed (code): 2026-07-12*
*Human UAT (Task 2): PENDING*

## Self-Check: PASSED

MobileNavDrawer.tsx + AppShell/DonationTable/AgingTable changes + this SUMMARY confirmed on disk; Task 1 commit `a2f2796` confirmed in git log. Task 2 intentionally has no commit — it is an un-performed human checkpoint, documented as PENDING per honest verification status.
