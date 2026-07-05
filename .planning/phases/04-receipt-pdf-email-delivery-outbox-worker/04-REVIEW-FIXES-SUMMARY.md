---
phase: 04-receipt-pdf-email-delivery-outbox-worker
type: review-fixes
reviewed_source: 04-REVIEW.md
status: complete
---

# Phase 4: Code Review Fixes Summary

Fixes for `04-REVIEW.md`'s 2 BLOCKERs (CR-01, CR-02) and 7 WARNINGs (WR-01..WR-07). INFO findings (IN-01..IN-05) were out of scope per the fix directive and were not touched.

Under MVP+TDD, every behavior-adding fix committed a failing RED test first (`test(04): ...`), then the GREEN fix (`fix(04): ...`). Config/docker-only fixes (WR-01) and comment-only documentation (WR-03) were exempt from TDD.

## Findings and Resolutions

| ID | Severity | Resolution | Commits |
|----|----------|------------|---------|
| CR-01 | BLOCKER | **Fixed.** Added `ReclaimStuckOutboxJobs` sqlc query + `Worker.ReclaimStuckJobs`, called once per `Run` tick before `ProcessOnce`. A job stuck in `processing` past a configurable timeout (`WORKER_STUCK_JOB_TIMEOUT`, default 10m) is reset to `pending` and becomes claimable again. | `7330fd0` (RED), `4e3e45c` (GREEN) |
| CR-02 | BLOCKER | **Fixed.** Added `Worker.ProcessOnceSafe`, a `recover()`-guarded wrapper around `ProcessOnce` that `Run` now calls every tick. A panic while processing one job is logged and swallowed instead of crashing the entire `donnarec-api` process (which would take the HTTP API down with it). A panicking job is left claimed (`processing`); CR-01's reclaim eventually recovers it for a normal retry. | `809c429` (RED), `7710941` (GREEN) |
| WR-01 | WARNING | **Fixed.** Added a top-level `chrome-internal` network (`internal: true`) in `docker-compose.yml` and attached the `chrome` service to it exclusively (in place of `default`) — Docker now genuinely never routes egress traffic from that container, closing the gap between the D-58 comments' claim ("network-isolated sidecar") and what the compose file actually configured (previously only `ports:` was omitted, which blocks inbound host access, not outbound/egress). `api` joins `chrome-internal` alongside its existing `default` network so it can still reach `ws://chrome:9222`. Verified with `docker compose config` (parses cleanly, exit 0) and a live `docker compose up --build chrome api` + `docker network inspect` showing `Internal: true` with both containers correctly attached. Config-only fix — exempt from TDD. | `570fb66` |
| WR-02 | WARNING | **Fixed.** `renderInSandbox`'s fetch-block handler now logs (via the new `failRequestAndLog` helper) instead of silently discarding `FailRequest` errors, and binds directly to the existing CDP context's `Target` executor (`cdp.WithExecutor` + `chromedp.FromContext`) instead of spawning a fresh `chromedp.Run` per paused-request event — avoiding the protocol-interleaving risk chromedp's own examples warn about. `Renderer` gained an optional variadic `*zap.Logger` param (defaults to `zap.NewNop()`, so every existing single-arg call site kept compiling unchanged); `cmd/server/main.go` now passes the real app logger. | `7998f19` (RED), `fb4c866` (GREEN) |
| WR-03 | WARNING | **Documented as known limitation** (not fixed). `deduction_multiplier`/`section6_text`/`template_html` are read fresh from `receipt_template_config` at first-render time, not snapshotted at Approve — an admin config edit in the narrow window between approval and worker pickup could affect the rendered receipt. The safe fix (snapshotting `deduction_multiplier` onto the `donations` row or outbox payload) requires touching `internal/donation/service.go`'s `Approve` method — the single most load-bearing, compliance-critical transaction in the codebase (D-52: gap-less receipt numbering + SoD + audit, all inside one lock). Per the fix directive's explicit scope guidance, this was deliberately NOT fixed here to avoid a drive-by change to that path; deferred to a future phase with its own plan and tests against `Approve`. Documented in detail (not a buried one-line comment) in `internal/worker/issue_receipt.go`'s package doc comment and at the `renderReceiptPDF` read site. | `233ef7a` |
| WR-04 | WARNING | **Fixed.** Added `lib/latest-response.ts`'s `createLatestGuard` (a request-sequence-number guard) and wired it into `TemplateLivePreview`'s debounced preview fetch. An older in-flight `fetchPreviewHTML` call whose response resolves *after* a newer one (network gives no ordering guarantee) is now ignored instead of silently overwriting the current preview with stale HTML. | `6271568` (RED), `d1749de` (GREEN) |
| WR-05 | WARNING | **Fixed** (evaluated carefully per the "nuanced" directive — safe to fix directly, not just document). `ClaimNextOutboxJob`'s `WHERE` clause now matches `status = 'pending'` only — `'failed'` is never claimable. `MarkOutboxJobFailed`'s own `CASE` logic already treats `'failed'` as terminal (a retriable failure with attempts remaining stays `'pending'`; only a truly exhausted job becomes `'failed'`), so removing `'failed'` from the claim predicate does not affect the retry+backoff path at all — it only closes the resurrection gap where raising `WORKER_MAX_ATTEMPTS` after the fact could silently reclaim an already dead-lettered job. Verified the existing `TestEmailRetryBackoff` still passes unchanged (jobs with attempts remaining stay `'pending'` throughout that test and are unaffected by this change). | `fb0ab6c` (RED), `4e85be4` (GREEN) |
| WR-06 | WARNING | **Fixed.** Added `Worker.fetchTemplateImageSoft`, which fails OPEN (logs a warning, returns an empty image) instead of propagating an error. `renderReceiptPDF` now uses it for all four decorative branding images (letterhead/seal/signature/watermark) — none of which are legally-required receipt content (donor name/amount/receipt number/section6 text always come from the donation row/config text fields). A transient object-storage blip on one branding asset no longer fails the entire render or burns a D-57 retry/backoff attempt. | `ea99c67` (RED), `e754808` (GREEN) |
| WR-07 | WARNING | **Fixed.** `SaveSettings`'s two writes (`UpdateReceiptTemplateConfig`, `UpdateReceiptNumberConfig`) now run inside a single `dbhelpers.WithTx` (Pattern B — the same helper every other atomic mutation in the codebase uses) — a failure on either write rolls back both, matching what the method's own doc comment already promised ("no partial save") but the code didn't enforce. `NewSettingsService` gained a `pool *pgxpool.Pool` parameter; all call sites updated. | `114a4e0` (RED), `177c69a` (GREEN) |

## Verification

- `sqlc generate` re-run after each query change (CR-01, WR-05).
- `go build ./...` and `go vet ./...` — clean, no warnings.
- `go test ./...` (full suite, no `-short`, real Postgres/MinIO/Chrome via testcontainers — Docker was available in this environment) — **all packages pass**, including the full `internal/worker` suite (reclaim, panic-recovery, branding soft-fetch, dead-letter, plus the pre-existing `TestProcessJob_RenderFreezeEmailRecordAndIdempotency`, `TestProcessJobLatency`, and `TestEmailRetryBackoff` — no regression in retry/backoff or freeze-idempotency).
- `cd donnarec-web && npm run lint && npm test` — clean, all 42 existing tests + 3 new `latest-response` tests pass.
- `docker compose config` (WR-01) — parses cleanly, exit 0; live `docker compose up --build chrome api` + `docker network inspect donnarec-api_chrome-internal` confirmed `Internal: true` with correct container membership.

## Deviations from Plan

None beyond what is documented above (WR-03's deliberate non-fix, WR-05's decision to fix directly rather than defer). No Rule 1-3 auto-fixes were needed beyond the review findings themselves.
