# Fonts — TH Sarabun New sourcing for the Admin settings live preview (plan 04-08)

## Why this file exists

`04-UI-SPEC.md` (Design System, "New font requirement (Phase 4)") requires the
Admin settings template live-preview iframe (`TemplateEditor`'s `TemplateLivePreview`,
D-61) to embed **the exact same TH Sarabun New font file** the server-side PDF
render pipeline uses (`donnarec-api/internal/pdf/render.go`'s `FontFaceCSS`),
so the fast in-browser preview visually matches the real PDF as closely as
possible.

## Status: same open item as `donnarec-api/assets/fonts/README.md`

As of this plan, **TH Sarabun New has not been sourced/licensed** anywhere in
this repository (see `donnarec-api/assets/fonts/README.md`'s "Assumption A3").
Placing an unlicensed copy of this font here would be the same problem on the
frontend side.

## What the team must source and provide

1. Obtain the **same licensed** `THSarabunNew.ttf`/`.woff2` file used (or
   planned to be used) by `donnarec-api/assets/fonts/THSarabunNew.ttf`.
2. Place a web-optimised copy at:
   ```
   donnarec-web/public/fonts/THSarabunNew.woff2
   ```
3. `components/TemplateEditor.tsx`'s injected preview `<style>` block already
   references this exact path via `@font-face` (`url('/fonts/THSarabunNew.woff2')`)
   with a graceful fallback to the Google-Fonts "Sarabun" family already used
   for app chrome (`next/font/google`, see `app/layout.tsx`) — the preview
   renders correctly (visually close, not glyph-identical) even before this
   file is sourced; no code change is needed once the file is added here.

## Until the licensed file is supplied

The preview iframe falls back to the Google-Fonts "Sarabun" family (the same
font already used for the rest of the back-office UI). This is visually
similar to TH Sarabun New but not glyph-identical — acceptable for
development/local use, but the info banner above the preview already tells
admins the preview is an approximation of the real PDF, and the "เรนเดอร์ PDF
จริง" (Render Real PDF) button always renders through the actual
production Chromium pipeline (`internal/pdf`), which is authoritative for
font-accuracy sign-off regardless of what this preview iframe uses.
