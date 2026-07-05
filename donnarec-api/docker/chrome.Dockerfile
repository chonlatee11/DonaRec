# docker/chrome.Dockerfile — Chromium render sidecar (Phase 4, D-58)
#
# Design decisions realized here:
#   - D-58: PDF rendering runs in a SEPARATE container from the app, reached
#     only via the internal compose network (no host `ports:` mapping — see
#     docker-compose.yml `chrome` service). Network isolation is layer 1 of 3
#     defense-in-depth against the admin-authored-HTML SSRF/XSS surface;
#     layers 2/3 (CDP Fetch-block + Emulation.SetScriptExecutionDisabled) are
#     enforced in Go render code (04-03, internal/pdf/chromium.go).
#   - 04-RESEARCH.md Pattern 4 (verified live this session): the official
#     chromedp/headless-shell image ships with ZERO Thai font support by
#     default (Pitfall 1 — silent tofu-box rendering, no error). Installing
#     `fonts-thai-tlwg` fixes Thai glyph shaping as an interim/fallback set
#     (Waree/Purisa/Garuda etc.) — NOT a substitute for TH Sarabun New.
#
# Base image digest (RESEARCH.md "Version verification", recorded 2026-07-04):
#   sha256:313ed7255ae1e155fb157631a6d4c0eb8b65bbe06de9e704ed834399bdf678ff
# ASSUMPTION A4 (RESEARCH.md): this digest was current as of the research
# session date above. Before locking this Dockerfile into CI, re-pull
# `chromedp/headless-shell:stable` and update the digest comment (and,
# ideally, pin `FROM chromedp/headless-shell:stable@sha256:<digest>` instead
# of the floating `stable` tag) so rendering stays deterministic across CI
# runs. Not pinned to a digest yet because doing so blind (without a fresh
# pull immediately before the CI lock) would just freeze last session's
# digest without actually verifying it's still current.
FROM chromedp/headless-shell:stable

# fonts-thai-tlwg: interim/fallback Thai shaping fonts (Waree/Purisa/Garuda).
# fontconfig + fc-cache: required so Chromium's font matching picks up the
# newly installed (and later COPY'd) font files.
RUN apt-get update && apt-get install -y --no-install-recommends \
      fonts-thai-tlwg fontconfig \
    && rm -rf /var/lib/apt/lists/* \
    && fc-cache -f

# TH Sarabun New itself is NOT included in fonts-thai-tlwg (Assumption A3,
# 04-RESEARCH.md / assets/fonts/README.md) — it is Thailand's standard
# tax/government document font and must be sourced/licensed separately by
# the hospital/team. When the licensed .ttf file is supplied, drop it at
# donnarec-api/assets/fonts/THSarabunNew.ttf and uncomment the COPY below,
# then re-run `fc-cache -f` (or rebuild the image, which reruns this layer).
#
# COPY assets/fonts/THSarabunNew.ttf /usr/share/fonts/truetype/th-sarabun/THSarabunNew.ttf
# RUN fc-cache -f
#
# NOTE: internal/pdf (04-03) additionally embeds the same font file as a
# base64 @font-face data URI directly in the rendered HTML — this container
# install is a backstop for fontconfig-based fallback/matching, not the sole
# source of the font at render time (RESEARCH.md Pattern 4 note).
