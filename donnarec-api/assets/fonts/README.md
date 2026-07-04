# Fonts — TH Sarabun New sourcing (Phase 4, D-58 / Assumption A3)

## Why this file exists

Thai receipt PDFs (FR-20/21/22/23/24) are rendered server-side through
headless Chromium (`docker/chrome.Dockerfile`, `internal/pdf` — 04-03), the
only mechanism proven to correctly shape Thai stacked tone marks/vowels
(CLAUDE.md "The Two Load-Bearing Decisions" #2; `04-RESEARCH.md`, verified
live this session).

Chromium's font matching needs an actual font file on disk (or embedded as a
`@font-face` base64 data URI at render time) to draw **TH Sarabun New** —
the standard font for Thai government/tax documents.

## Assumption A3 — TH Sarabun New is NOT in any apt package

`apt-get install fonts-thai-tlwg` (already applied in
`docker/chrome.Dockerfile`) installs a set of Thai-shaping-capable TrueType
fonts maintained by TLWG:

- Garuda, Kinnari, Laksaman, Loma, Norasi, Purisa, Sawasdee, Tlwg Mono,
  Tlwg Typewriter, Tlwg Typist, Tlwg Typo, Umpush, Waree

**None of these is TH Sarabun New.** They exist as an **interim shaping
fallback / safety net** so the render pipeline never silently produces tofu
boxes (`□□□`) for Thai glyphs (04-RESEARCH.md Pitfall 1) — they are
**never** an acceptable substitute for production receipt typography. Thai
tax/government documents are conventionally set in TH Sarabun New
specifically, and using a TLWG font in a real, customer-facing receipt would
be a visually-wrong (if legible) document.

## What the team must source and provide

1. Obtain a **licensed** copy of `THSarabunNew.ttf` (and, if available,
   its Bold/Italic/BoldItalic siblings) from an authorized distribution
   channel. This is explicitly **not** bundled with this repository — it
   must be sourced/licensed separately by the hospital or the implementing
   team before this phase's PDF output is used in production.
2. Place the file(s) at:
   ```
   donnarec-api/assets/fonts/THSarabunNew.ttf
   donnarec-api/assets/fonts/THSarabunNew-Bold.ttf        (optional)
   donnarec-api/assets/fonts/THSarabunNew-Italic.ttf      (optional)
   donnarec-api/assets/fonts/THSarabunNew-BoldItalic.ttf  (optional)
   ```
3. Two places need the font bytes, and BOTH must reference the **identical**
   file so PDF output and the admin live-preview iframe (D-61) match:
   - `docker/chrome.Dockerfile` has a commented-out `COPY` line that installs
     the file into the chrome sidecar image's font path
     (`/usr/share/fonts/truetype/th-sarabun/`) as a fontconfig-level
     fallback/match target. Uncomment it once the file is present.
   - `internal/pdf` (04-03) is expected to additionally embed the same file
     as a base64 `@font-face` data URI directly in the self-contained HTML
     handed to Chromium (`page.SetDocumentContent`) — this is the primary,
     guaranteed-available path (it does not depend on the container's
     installed fonts at all), with the Dockerfile `COPY` as a defense-in-depth
     fallback.
4. Rebuild the `chrome` image (`docker compose build chrome`) after adding
   the file so the fontconfig cache picks it up (`fc-cache -f` reruns as
   part of that Dockerfile layer).

## Until the licensed file is supplied

The `fonts-thai-tlwg` fonts (Waree in particular, which has broad Thai
Unicode coverage) render Thai text correctly-shaped but with different,
non-production typography. This is acceptable for development/local testing
of the render pipeline (correctness of tone-mark stacking, watermark/
signature/letterhead placement, i18n text swapping) but **must not** be
treated as the final production font — track this as an open item until
TH Sarabun New is sourced and wired per the steps above.
