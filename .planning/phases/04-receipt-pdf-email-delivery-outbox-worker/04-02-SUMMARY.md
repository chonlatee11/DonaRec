---
phase: 04-receipt-pdf-email-delivery-outbox-worker
plan: 02
subsystem: infra
tags: [docker, chromedp, headless-chromium, testcontainers, fonts-thai-tlwg, docker-compose]

# Dependency graph
requires:
  - phase: 04-receipt-pdf-email-delivery-outbox-worker
    provides: "04-01 data-layer foundation — config.Worker.ChromeWSURL default (ws://chrome:9222), MinIO.ReceiptsBucket"
provides:
  - "docker/chrome.Dockerfile: chromedp/headless-shell:stable + fonts-thai-tlwg + fontconfig, verified via fc-list/dpkg"
  - "docker-compose.yml chrome service: internal-network-only sidecar (no host ports:), api depends_on it + CHROME_WS_URL/MINIO_RECEIPTS_BUCKET env"
  - "go.mod: github.com/chromedp/chromedp v0.14.2 pinned (go directive stays 1.25.1)"
  - "internal/testutil/chrome.go: StartChrome(t) testcontainers helper returning a real CDP ws:// URL + idempotent cleanup"
  - "assets/fonts/README.md: TH Sarabun New sourcing/licensing documented as an explicit open assumption"
affects: [04-03, 04-05]

# Tech tracking
tech-stack:
  added:
    - "github.com/chromedp/chromedp v0.14.2 (pinned, not yet imported by non-test code — 04-03 will be the first real caller)"
  patterns:
    - "Chromium runs as a separate docker-compose sidecar, never bundled into the app's own minimal Dockerfile image"
    - "testutil.StartChrome builds the SAME docker/chrome.Dockerfile testcontainers uses for docker-compose, so integration tests exercise the identical Thai-font-equipped image, not a bare upstream one"
    - "sync.Once-guarded cleanup closure so a helper can both register t.Cleanup AND return an explicit cleanup func without a double-terminate warning"

key-files:
  created:
    - donnarec-api/docker/chrome.Dockerfile
    - donnarec-api/internal/testutil/chrome.go
    - donnarec-api/assets/fonts/README.md
  modified:
    - donnarec-api/docker-compose.yml
    - donnarec-api/go.mod
    - donnarec-api/go.sum

key-decisions:
  - "chromedp pin lands as `// indirect` in go.mod (nothing imports it yet — StartChrome only needs an HTTP GET to /json/version, not chromedp itself) — this is expected and will flip to a direct requirement automatically once 04-03's internal/pdf package imports it; do not run `go mod tidy` speculatively before that, or it would drop the indirect entry"
  - "chrome.Dockerfile installs fonts-thai-tlwg as an explicitly-documented INTERIM fallback (Waree/Garuda/Purisa/etc., verified via fc-list — 58 TLWG font files), not a substitute for TH Sarabun New, which assets/fonts/README.md documents as a separate licensing/sourcing task with a commented-out COPY slot ready for when the file arrives"
  - "StartChrome does a fast `docker info` probe (5s timeout, os/exec) to skip cleanly (t.Skip) when Docker is unavailable, rather than reusing the codebase's existing `testing.Short()` convention (receiptno/allocator_test.go) — building this helper's own image via FromDockerfile is a heavier local-dev precondition than the pre-built module images (postgres/keycloak) the other testutil helpers assume, so a Docker-availability check (not a test-mode flag) is the more accurate skip condition"
  - "StartChrome returns an explicit cleanup func (per plan's stated signature) in addition to registering t.Cleanup internally (matching SetupTestPostgres's implicit style) — wrapped both in a sync.Once so calling the returned cleanup() and the t.Cleanup-triggered call are both safe, avoiding the 'No such container' warning observed when both fired unguarded during manual verification"

requirements-completed: [FR-23, NFR-07]

coverage:
  - id: D1
    description: "chrome.Dockerfile builds from chromedp/headless-shell:stable + fonts-thai-tlwg + fontconfig; image contains Thai-shaping-capable fonts"
    requirement: "FR-23"
    verification:
      - kind: other
        ref: "docker build -f docker/chrome.Dockerfile -t donnarec-chrome-test . (exit 0); docker run --entrypoint fc-list donnarec-chrome-test | grep -i tlwg (58 matches); docker run --entrypoint dpkg donnarec-chrome-test -l | grep fonts-thai-tlwg (ii, installed)"
        status: pass
    human_judgment: false
  - id: D2
    description: "chromedp pinned at v0.14.2 without bumping the go.mod `go` directive off 1.25.1"
    requirement: "NFR-07"
    verification:
      - kind: other
        ref: "grep -q 'chromedp v0.14.2' go.mod && grep -qE '^go 1\\.25' go.mod (both pass); go build ./... exits 0"
        status: pass
    human_judgment: false
  - id: D3
    description: "docker-compose.yml chrome service has no host ports: mapping (internal-network-only)"
    verification:
      - kind: other
        ref: "docker compose config --quiet (exit 0); manual grep of the chrome: service block confirms no `ports:` key"
        status: pass
    human_judgment: false
  - id: D4
    description: "internal/testutil/chrome.go: StartChrome builds the sidecar via testcontainers, waits for CDP readiness, and returns a real ws:// URL + idempotent cleanup"
    verification:
      - kind: integration
        ref: "ad-hoc TestStartChrome_Smoke (run this session, not committed — see Verification note below): obtained ws://localhost:<port>/devtools/browser/<uuid> and confirmed GET /json/version returns 200 through it"
        status: pass
    human_judgment: false

# Metrics
duration: 12min
completed: 2026-07-04
status: complete
---

# Phase 4 Plan 2: Chrome Sidecar + chromedp Pin + Test Harness Summary

**docker/chrome.Dockerfile (chromedp/headless-shell:stable + fonts-thai-tlwg, network-isolated compose sidecar) + chromedp v0.14.2 pinned without a Go 1.26 toolchain bump + testutil.StartChrome testcontainers helper**

## Performance

- **Duration:** ~12 min
- **Started:** 2026-07-04T14:12:37+07:00
- **Completed:** 2026-07-04T14:22:32+07:00
- **Tasks:** 2
- **Files modified:** 6 (3 created, 3 modified)

## Accomplishments
- `docker/chrome.Dockerfile` builds `FROM chromedp/headless-shell:stable` + `fonts-thai-tlwg`/`fontconfig`/`fc-cache` — verified this session: `docker build` exits 0, and the resulting image's `fc-list` shows 58 TLWG font files (Waree, Garuda, Purisa, Norasi, Kinnari, etc.) plus `dpkg -l` confirms the `fonts-thai-tlwg` metapackage is installed
- `docker-compose.yml` gained a `chrome` service (builds from the new Dockerfile, `networks: [default]`, **no `ports:` key** — reachable only over the internal compose network); `api` now depends on it and receives `CHROME_WS_URL` (default `ws://chrome:9222`, matching 04-01's config default) and `MINIO_RECEIPTS_BUCKET` env vars
- `chromedp` pinned to `v0.14.2` via `go get github.com/chromedp/chromedp@v0.14.2` — confirmed `go.mod`'s `go` directive stayed at `1.25.1` (v0.15.1/latest would have forced a silent bump to Go 1.26, per 04-RESEARCH Pitfall 2); `go build ./...` and `go vet ./...` both exit 0 afterward
- `internal/testutil/chrome.go` adds `StartChrome(t *testing.T) (wsURL string, cleanup func())`: builds the same `docker/chrome.Dockerfile` image via `testcontainers.GenericContainer` + `FromDockerfile`, waits on `wait.ForHTTP("/json/version").WithPort("9222/tcp")`, resolves the real `webSocketDebuggerUrl` from that endpoint (not just "port is open"), and skips gracefully (`t.Skip`) via a fast `docker info` probe when Docker is unavailable
- `assets/fonts/README.md` documents that TH Sarabun New (Thailand's standard tax/government document font) is **not** included in `fonts-thai-tlwg` and must be sourced/licensed separately, with the exact file path and a commented-out `COPY` slot already wired into the Dockerfile for when it's supplied

## Task Commits

Each task was committed atomically:

1. **Task 1: chrome sidecar Dockerfile + docker-compose service + chromedp v0.14.2 pin** - `72dc04b` (feat)
2. **Task 2: testutil.StartChrome testcontainers helper** - `f0706eb` (feat)

**Plan metadata:** (this commit, docs: complete plan)

_Note: Neither task declared `tdd="true"` in PLAN.md, and both are infra/Dockerfile/dependency-pin/test-helper work with no application-facing `<behavior>` block — per the MVP+TDD gate's Behavior-Adding Task predicate (tdd="true" frontmatter AND a `<behavior>` block AND non-test source files), neither task triggers the RED/GREEN commit sequence. Task 2 produces a test **helper**, not application behavior under test._

## Files Created/Modified
- `donnarec-api/docker/chrome.Dockerfile` - Chromium sidecar image: base + Thai fonts + digest re-pin note + commented TH Sarabun COPY slot
- `donnarec-api/docker-compose.yml` - new `chrome` service (internal-only) + `api` env/depends_on wiring
- `donnarec-api/go.mod` / `go.sum` - `github.com/chromedp/chromedp v0.14.2` pin (and its transitive deps: cdproto, sysutil, gobwas/ws, etc.)
- `donnarec-api/internal/testutil/chrome.go` - `StartChrome` testcontainers helper + `resolveBrowserWSURL` + `dockerAvailable`
- `donnarec-api/assets/fonts/README.md` - TH Sarabun New sourcing/licensing documentation

## Decisions Made
- `chromedp` remains `// indirect` in `go.mod` until 04-03 actually imports it in `internal/pdf` — this is expected sqlc/Go module behavior, not an error; flagged so a future `go mod tidy` run doesn't accidentally strip it before that import lands.
- `assets/fonts/README.md` frames `fonts-thai-tlwg` explicitly as an interim/fallback font set, never production typography, to prevent a future contributor from mistaking Waree-rendered receipts for the correct TH Sarabun New output.
- `StartChrome` uses a `docker info` probe (not the codebase's existing `testing.Short()` convention) to decide when to skip — building a custom image via `FromDockerfile` is a materially heavier precondition than the pre-built module images `SetupTestPostgres`/`NewKeycloakTestServer` rely on.
- `StartChrome`'s cleanup closure is wrapped in `sync.Once` so both the explicit returned `cleanup()` and the internally-registered `t.Cleanup(cleanupFn)` are safe to fire — observed and fixed a "No such container" double-terminate warning during this session's manual verification before finalizing the file.

## Deviations from Plan

None - plan executed exactly as written. The `sync.Once` guard and the `docker info` skip probe are implementation details within Task 2's stated action ("Skip the test (t.Skip) ... matching the existing testutil skip convention" and "mirror postgres.go's constructor/cleanup shape") rather than scope additions.

## Issues Encountered
- The plan's Task 1 acceptance criterion ("`fc-list | grep -i thai` returns matches") does not literally match on this image, because none of the `fonts-thai-tlwg` package's font family names contain the literal substring "Thai" (they're named Waree/Garuda/Purisa/Norasi/Kinnari/etc. — TLWG's font family naming convention). Verified the substantive intent instead via `fc-list | grep -i tlwg` (58 matches) and `dpkg -l | grep fonts-thai-tlwg` (package confirmed installed) — both prove the font package is present and its files are registered with fontconfig, which is what the acceptance criterion is actually checking for.
- A manual smoke test (`TestStartChrome_Smoke`, written and run this session but **not committed** — it was scratch verification, not a plan deliverable) surfaced the double-Terminate warning fixed by the `sync.Once` decision above; the test file was removed after verification since it wasn't part of the plan's `files_modified` scope.

## User Setup Required

None - no external service configuration required. `CHROME_WS_URL` defaults to `ws://chrome:9222` (matches the new compose service's internal DNS name); no new required env vars. TH Sarabun New sourcing (assets/fonts/README.md) is a **future** action item for whoever supplies the licensed font file, not a blocker for this plan or the next.

## Next Phase Readiness

- 04-03 (PDF render core) can dial `config.Worker.ChromeWSURL` in production and use `testutil.StartChrome(t)` in tests to get a real CDP endpoint against the exact Thai-font-equipped image; `chromedp v0.14.2` is already in `go.mod`, ready to import.
- 04-05 (worker integration test) can reuse `testutil.StartChrome` alongside `testutil.SetupTestPostgres` for full-pipeline integration tests.
- No blockers. Remaining open item (non-blocking): TH Sarabun New licensed font file still needs to be sourced by the team per `assets/fonts/README.md` before production-quality PDF typography is final — the render pipeline itself does not depend on this to function (fonts-thai-tlwg renders Thai correctly-shaped, just not in the production font).

## Self-Check: PASSED

Verified files exist on disk: `donnarec-api/docker/chrome.Dockerfile`, `donnarec-api/internal/testutil/chrome.go`, `donnarec-api/assets/fonts/README.md` (all FOUND). Verified commit hashes `72dc04b` and `f0706eb` present in `git log --oneline --all` (both FOUND).

---
*Phase: 04-receipt-pdf-email-delivery-outbox-worker*
*Completed: 2026-07-04*
