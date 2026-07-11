---
gsd_state_version: 1.0
milestone: v1.0
milestone_name: milestone
current_phase: 05
status: "Phase 05 shipped — PR #5"
stopped_at: Phase 6 UI-SPEC approved (dual-theme warm public + responsive)
last_updated: "2026-07-11T04:05:39.366Z"
last_activity: 2026-07-11
progress:
  total_phases: 6
  completed_phases: 5
  total_plans: 38
  completed_plans: 39
  percent: 83
current_phase_name: e-Donation Export, Reports & Admin Settings
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-06-22)

**Core value:** ออกใบเสร็จบริจาคที่มีเลขที่รันต่อเนื่องไม่ซ้ำ ห้ามข้ามเลข (gap-less) ตามปีงบประมาณ หลังผ่านการอนุมัติโดยมนุษย์ และส่งถึงผู้บริจาคได้อย่างถูกต้องน่าเชื่อถือ
**Current focus:** Phase 05 — e-Donation Export, Reports & Admin Settings

## Current Position

Phase: 05 — COMPLETE
Plan: 7 of 7
Prior phases: Phase 3 Complete (integration gate met — automated E2E + human walkthrough 7/7, 2026-07-04); Phase 4 Complete + shipped (PR #4). Phase 4 deferred human UI walkthroughs (04-06 Task 4 Screen 3b + 04-08 Task 3 Screen 6) driven live through Chrome and PASSED 2026-07-04 (04-UAT.md 2/2 passed, 04-VERIFICATION.md status: passed) — no outstanding Phase 4 items.
Status: Phase 05 shipped — PR #5
Last activity: 2026-07-11

Context: Phase 3 was marked Complete 2026-07-01 on 5/5 unit-level verification. On 2026-07-02, standing up the real stack (docker compose; postgres remapped to host 5433 via docker-compose.override.yml; 4 users seeded) and driving it with a real Keycloak token surfaced three runtime-integration-seam bugs that unit tests structurally could not catch. New done-criterion added (Conventions → Integration-test gate; ROADMAP Phase 3 criterion 6).

## Performance Metrics

**Velocity:**

- Total plans completed: 14
- Average duration: —
- Total execution time: —

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| 01 | 5 | - | - |
| 04 | 9 | - | - |

**Recent Trend:**

- Last 5 plans: —
- Trend: —

*Updated after each plan completion*
| Phase 02-gap-less-receipt-numbering-core P01 | 262s | 2 tasks | 8 files |
| Phase 02 P02 | 254 | 2 tasks | 4 files |
| Phase 02 P03 | 256 | 1 tasks | 2 files |
| Phase 02-gap-less-receipt-numbering-core P04 | 502 | 2 tasks | 4 files |
| Phase 03 P05 | 120 | 3 tasks | 7 files |
| Phase 03 P09 | 35min | 3 tasks | 7 files |
| Phase 03 P11 | 30min | 3 tasks | 7 files |
| Phase 03 P10 | 18min | 2 tasks | 13 files |
| Phase 03 P12 | 35min | 2 tasks | 12 files |
| Phase 03 P13 | 3min | 3 tasks | 9 files |
| Phase 04 P01 | 15min | 3 tasks | 19 files |
| Phase 04 P02 | 12min | 2 tasks | 6 files |
| Phase 04 P04 | 3min | 1 tasks | 6 files |
| Phase 04 P03 | 18min | 1 tasks | 6 files |
| Phase 04 P05 | 25m | 1 tasks | 8 files |
| Phase 04 P06 | 35min | 3 tasks | 20 files |
| Phase 04 P07 | 20min | 2 tasks | 11 files |
| Phase 04 P08 | 13min | 2 tasks | 24 files |
| Phase 05 P01 | 20min | 2 tasks | 20 files |
| Phase 05 P03 | 20min | 3 tasks | 9 files |
| Phase 05 P02 | 40min | 3 tasks | 10 files |
| Phase 05 P04 | 30min | 3 tasks | 8 files |
| Phase 05 P05 | 8min | 3 tasks | 10 files |
| Phase 05 P06 | 6min | 3 tasks | 19 files |
| Phase 05 P07 | 3min | 2 tasks | 12 files |

## Accumulated Context

### Decisions

Decisions are logged in PROJECT.md Key Decisions table.
Recent decisions affecting current work:

- Roadmap: Correctness-first ordering — gap-less numbering (Phase 2) is built and concurrency-proven BEFORE the approval/issuance flow (Phase 3) depends on it.
- Roadmap: Foundation (audit + RBAC + retention model) first — expensive to retrofit; everything depends on it.
- Roadmap: PDF + email decoupled behind an async outbox worker (Phase 4); kept out of the issuance transaction.
- Roadmap: Public donation web form (Flow B) built LAST (Phase 6) — reuses the entire Flow-A pipeline.
- [Phase ?]: Allocator.queries field is *db.Queries (concrete) not db.Querier — WithTx bind requires concrete type (02-PATTERNS Key Observation #1)
- [Phase ?]: allocator_test.go uses black-box package receiptno_test; Allocator/NewAllocator/AllocatedReceipt are exported
- [Phase 03]: Atomic 7-step Approve tx: lock→SoD→Allocate→Issue→Audit→Outbox in one WithTx closure — any failure rolls back all effects; receipt number exists only on issued records
- [Phase 03]: SoD enforced at code layer (approverID != createdBy) AND DB CHECK (chk_sod_approver) — defense-in-depth; both layers tested by integration tests
- [Phase 03]: Checker route group RequireRoles(Checker, Admin) at HTTP layer in addition to service-layer SoD guard — defense-in-depth over service layer
  - ⚠️ CORRECTION 2026-07-02: this decision encoded the RBAC AND-bug. `RequireRoles(...)` enforces AND (user must hold ALL listed roles), so `RequireRoles(Checker, Admin)` requires checker AND admin — a checker-only user is wrongly blocked. Intent was "checker OR admin". Same bug on donationGroup `RequireRoles(Maker,Checker,Admin)`. Fix: add `RequireAnyRole` (OR) and use it at both sites. (bug #3, OPEN)
- [Phase 03] 2026-07-02: Identity model clarified — `claims.Subject` (KC sub) ≠ `users.id` (surrogate PK). All `REFERENCES users(id)` writes must resolve sub→users.id via `auth.ResolveAppUser` middleware; `audit_log.actor_id` intentionally keeps raw sub (no FK). (bug #1 fix, committed ef7ede6)
- [Phase 03] 2026-07-02: FE↔BE auth requires a Keycloak Audience mapper putting `donnarec-backend` in token `aud`, and `donnarec-frontend` must be a confidential client (NextAuth server-side). (bug #2 fix, uncommitted)
- [Phase 03]: GET /api/donations now returns the D-R2 pagination envelope {data:{items,total,page,per_page}} with a real COUNT (CountDonations mirrors SearchDonations' filter predicate) and creator display-name/UUID per row
- [Phase 03]: donations.sql list filters use sqlc.narg(...) instead of bare @param for nullable params — fixes a hand-edited generated-code fragility (donations.sql.go pointer types were manually patched, violating DO NOT EDIT); sqlc.narg() makes the D-53 nil-skips-filter semantics regeneration-safe
- [Phase 03]: DonationDetailResponse (D-R3): GetByID + all 8 mutations now share one buildDetailResponse builder returning national_id_masked/address/email/note/created_by(name)+created_by_id(UUID)/review_history/replaces+replaced_by({id,receipt_formatted}) plus server-computed viewer_is_creator/can_approve/can_return/can_reject/can_reveal_pii (T-03-31); viewer_is_creator always resolves claims.Subject to users.id via GetUserByKeycloakSubject, never compares claims.Subject directly (T-11-03)
- [Phase 03]: review_history is sourced from audit_log (immutable, full return/reject history) via a new GetDonationReviewHistory sqlc query, not donations.review_reason which only holds the latest review action
- [Phase ?]: [Phase 03] 03-10: BFF proxy pattern (D-R1) - app/api/bff Route Handlers + lib/bff.ts bffForward obtain the Keycloak token via getServerSession and forward a Bearer server-side; access token never reaches the browser. TanStack Query calls the same-origin BFF route only.
- [Phase ?]: [Phase 03] 03-10: apiFetch unwraps the data envelope (D-R2); DonationListResponse key donations to items; DonationSummary.amount is a numeric string (parseFloat at render). Root fix for bug #5 (undefined.length on result.donations). Donation list screen migrated to TanStack Query + TanStack Table.
- [Phase 03]: 03-12: BFF Route Handlers for donation detail (composes slip_url via a second server-side /:id/slip call)/pii (donor_tax_id->national_id mapping)/approve/return/reject; client DonationDetailView (useQuery+useMutation) drives Screen 3+4 — ReviewActionPanel/MaskedIdField needed zero changes since their existing Promise<{error}|null> callback contracts already matched the new mutation wrappers. Cancel/reissue deliberately deferred to 03-13.
- [Phase 03]: Cancel/reissue mutation wiring lives in DonationDetailView (useMutation), not in CancelDialog itself; CancelDialog stays presentational — Matches the existing approve/return/reject pattern established in 03-12; fixed a broken Server Action -> client-BFF-fetcher call path (Rule 1 bug)
- [Phase 04]: [Phase 04] 04-01: ClaimNextOutboxJob's self-referencing subquery requires table aliases (o./j.) to satisfy sqlc's static analyzer, even though Postgres itself has no ambiguity issue with the unaliased query
- [Phase 04]: [Phase 04] 04-01: receipt_template_config seeded with BOTH template_html (Thai) and template_html_en (English) skeletons so donor_language='en' records don't render blank before an admin edits the template; section6_text_th/en left empty pending accounting/legal sign-off
- [Phase 04]: [Phase 04] 04-02: chrome sidecar service builds from chromedp/headless-shell:stable + fonts-thai-tlwg, reachable only over the internal compose network (no host ports:); chromedp pinned v0.14.2 to avoid a Go 1.26 toolchain bump
- [Phase 04]: [Phase 04] 04-02: testutil.StartChrome builds the same docker/chrome.Dockerfile via testcontainers FromDockerfile and resolves the real CDP webSocketDebuggerUrl from /json/version, mirroring StartPostgres/NewKeycloakTestServer's t.Helper/cleanup shape
- [Phase ?]: [Phase 04] 04-04: EmailSender interface + DevSender capture-to-disk built per RESEARCH.md's pre-vetted code (D-60); no self-hosted SMTP/provider SDK import
- [Phase ?]: [Phase 04] 04-04: i18n receipt.*/email.* message IDs added to th.json/en.json for FR-23/26; section6_text stays config-store-driven, not part of the i18n catalog, pending accounting/legal sign-off
- [Phase 04]: 04-03: ReceiptData field names match the 04-01-seeded receipt_template_config placeholders; image/font fields typed template.URL/template.CSS (plain string gets sanitized to #ZgotmplZ by html/template)
- [Phase 04]: 04-03: renderInSandbox is the single CDP action sequence both production RenderPDF and the security regression tests call — proves tests exercise the exact production sandbox (network-isolated sidecar + fetch.Enable/FailRequest-all + emulation.SetScriptExecutionDisabled)
- [Phase ?]: [Phase 04] 04-05: worker.Config.ComputeBackoff is an injected func decoupled from config.WorkerConfig, so tests can use near-instant backoff without touching internal/config's unexported schedule
- [Phase ?]: [Phase 04] 04-05: freeze-then-email ordering — render+freeze commits before email send is attempted, so a failed send never re-triggers a render; retries just re-fetch the frozen PDF from MinIO
- [Phase ?]: [Phase 04] 04-05: template branding assets fetched via the same receipts bucket/ReceiptsStore as frozen PDFs — no dedicated asset bucket yet since 04-07 settings UI doesn't exist; revisit if 04-07 chooses differently
- [Phase 04]: [Phase 04] 04-06: Resend is enqueue-only (D-56/D-57) — inserts a new outbox_jobs row for the same donation_id via the existing EnqueueOutboxJob path; relies entirely on 04-05's freeze-idempotency for the worker to reuse the frozen PDF, never re-numbers/re-renders.
- [Phase 04]: [Phase 04] 04-06: donor_language (D-55) captured on create/edit, defaults 'th', frozen at create-time like other snapshot fields (D-43 precedent); resend route on checkerGroup (Checker/Admin), download route on the broader donationGroup so all staff roles can download.
- [Phase 04]: [Phase 04] 04-06: Task 4 (Screen 3b human UI walkthrough, checkpoint:human-verify) DEFERRED to phase-end /gsd-verify-work by explicit user decision — code complete (Tasks 1-3, E2E-proven over real HTTP path); 04-06-SUMMARY.md documents exact walkthrough steps + credential prerequisites (Keycloak donnarec-frontend client secret, donnarec-web/.env.local, admin-test/maker-test/checker-test passwords).
- [Phase 04]: 04-07: Added UpdateReceiptNumberConfig sqlc query (Rule 2) — the number-format tab had no save path before this plan; Phase 2 only ever built a read-only GetReceiptNumberConfig
- [Phase 04]: 04-07: adminGroup now runs auth.ResolveAppUser (mirrors donationGroup) so settings Save/UploadImage can set updated_by to the acting admin's resolved users.id, never the raw Keycloak subject
- [Phase 04]: 04-07: settings Preview/PreviewPDF reuse the EXACT SAME receiptsStore and pdfRenderer instances the outbox worker (04-05) uses — not new ones — so preview structurally cannot run through a second, less-sandboxed rendering path (D-58/D-61)
- [Phase ?]: 04-08: Admin settings route is /admin/settings (plan's file path); Admin gating is a client UX hint only, Go RequireRoles(RoleAdmin) remains the real authority
- [Phase 04]: 04-08: TemplateEditor.tsx split into TemplateEditorFields + TemplateLivePreview so the live preview persists across all four tabs, not just the template tab
- [Phase 04]: 04-08: TH Sarabun New font remains unsourced (same open licensing item as backend) — preview iframe falls back to Google-Fonts Sarabun until public/fonts/THSarabunNew.woff2 is provided
- [Phase 04]: [Phase 04] Code-review fixes (04-REVIEW.md): CR-01/CR-02 (blockers) and WR-01/02/04/05/06/07 fixed with RED/GREEN tests; WR-03 (deduction_multiplier frozen-at-approval) documented as a known limitation rather than reworking the Approve transaction — see 04-REVIEW-FIXES-SUMMARY.md
- [Phase ?]: [Phase 05] 05-01: GetDonationByID's SELECT list extended to include edonation_keyed_at/edonation_keyed_by (physical column order) so sqlc keeps reusing the Donation model type after migration 000013's ALTER TABLE — required to keep go build green
- [Phase ?]: [Phase 05] 05-01: edonation.Config merges DTO+accessor into one type (NewConfig(*db.Queries)); FieldMapping.RowValues takes a plain map[string]string, not a concrete ExportRow type owned by a later plan
- [Phase 05]: 05-03: pg_restore test-scope uses --no-owner --no-privileges (fresh unmigrated target has no roles); production restore.sh uses --role=donnarec_app and documents the role-provisioning prerequisite
- [Phase 05]: 05-03: TestRestoreProof_MinIO uses minio-go SDK round trip instead of the mc CLI (not installed on test-runner host); functionally equivalent restore-completeness proof
- [Phase ?]: [Phase 05] 05-02: Service.Export mirrors donation.RevealPII's audited-decrypt discipline (Pattern 3) — role gate before any DB call, one WithTx closure for query+decrypt+ONE summary audit row, commit, then return plaintext; never imports internal/exportfile (Pitfall 3: streaming stays outside the tx, in xlsx.go/csv.go/handler).
- [Phase ?]: [Phase 05] 05-02: empty-result 404 check lives in the handler (len(rows)==0), not the service — Service.Export always returns rows plus a committed audit row; HTTP semantics stay out of the service layer.
- [Phase 05]: 05-04: computeBucket's deadline-instant boundary uses a strict !now.Before(deadline) time comparison (not a truncated-integer days>=0 check) so now==deadline classifies as overdue, not near_due
- [Phase 05]: 05-04: SetKeyed's per-donation audit loop is driven by a pre-update raw-SQL SELECT of caller ids WHERE status='issued' inside the same WithTx, not the raw caller input list — a cancelled id in the same bulk request is a silent no-op (no audit row)
- [Phase 05]: 05-04: Service.Aging stays pure/testable (now + near_due_days as explicit params, never reads wall clock/config internally); the handler resolves now (default wall clock, overridable via ?now=RFC3339) and near_due_days (via Config.GetConfig), mirroring Export's handler-owns-config-resolution precedent
- [Phase 05]: 05-05: SUM(amount) in reports.sql needed an explicit ::numeric cast — sqlc v1.31.1 mis-infers SUM() over NUMERIC(15,2) as int64, which cannot losslessly hold a fractional (satang) total; regenerated sqlc after the fix
- [Phase 05]: 05-05: report.Service takes only *db.Queries (no keyProvider/auditSvc) — reportGroup carries NO RequireAnyRole/RequireRoles gate (D-71), and Export writes zero audit_log rows since there is no PII to reveal
- [Phase 05]: Record-count preview (Export tab) derives an exact client-side count from the shared aging query for the default not_keyed filter; hidden (not fabricated) for all/keyed since no backend count endpoint exists for those scopes
- [Phase 05]: AgingTable is the Tab B smart container owning the shared aging query/mutation/selection state; AgingStatCards/BulkActionBar stay presentational
- [Phase ?]: 05-07: added lib/reports.ts as shared typed client-fetcher module (mirrors 05-06 lib/edonation.ts precedent) plus currentFiscalYearDateRange() default for the Screen 8 filter bar
- [Phase ?]: 05-07: EdonationConfigTab is a self-contained 5th SettingsTabs tab with its own save button, independent of the top-level 'save all tabs' button, since it persists edonation_config (not receipt settings)

### Pending Todos

Phase 3 integration-gate remediation — ✅ **RESOLVED / CLOSED 2026-07-04** (Phase 3 is Complete; ROADMAP criterion 6 met — automated E2E + human walkthrough 7/7, `03-UAT.md` / `03-VERIFICATION.md` frontmatter `status: passed`, commit f1f5b0e). Items below retained for history.

1. [x] Bug #1 `created-by-fk-mismatch` — resolve sub→users.id in `auth.ResolveAppUser` middleware. FIXED + committed (ef7ede6, refactor a1e348e).
2. [x] Bug #2 `fe-be-audience-mismatch` — audience mapper + confidential frontend client + web env. FIXED + committed (8604caa; debug doc 369dcce).
3. [x] Bug #3 RBAC AND-bug — added `RequireAnyRole` (OR); switched donationGroup + checkerGroup guards; test added. FIXED + committed (b10fae8).
4. [x] E2E HTTP integration test (real router + real signed token) — `cmd/server/e2e_test.go`: happy path + unprovisioned-403 + RBAC + SoD + audience. 5/5 subtests PASS (-race). COMMITTED (c5b0c4f). **Automated integration gate SATISFIED.**
5. [x] Gap #4 `frontend-auth-gating-missing` — frontend had NO route protection / login gating (root was a placeholder, no middleware, custom signin 404'd). Added middleware.ts (withAuth) + app/auth/signin (auto signIn keycloak) + `/`→`/donations` redirect + SessionProvider + SignOutButton. FIXED + committed (63c7a40; debug doc 71345e5). Verified: unauth /,/donations → 307 to signin; /auth/signin → 200.
6. [x] Human UI browser walkthrough — **DONE 2026-07-04: ran full-stack walkthrough, 7/7 checkpoints passed (03-UAT.md).** 3 issues found+fixed in-session (stale api container 3b3aeda, federated logout 78b04f1, hydration skeleton 88e82ff). Criterion 6b satisfied.
7. [ ] (Optional) wider auth/RBAC/wiring seam audit — not required for phase completion; leave as optional follow-up.

Phase 3 integration gate (ROADMAP criterion 6) is MET → Phase 3 is Complete. ✅

Phase 4 deferred UAT (blocks marking Phase 4 Complete — Conventions integration-test gate):

1. [ ] 04-06 Task 4 — Screen 3b human UI walkthrough (EmailDeliveryPanel status/recipient/attempts, resend re-enqueue with unchanged receipt_no, download PDF renders Thai/English, Maker sees Download but not Resend). DEFERRED by explicit user decision to phase-end `/gsd-verify-work`. Code (Tasks 1-3) complete and E2E-proven over the real HTTP path (commits 6f9ad34, d09419d, 743389c, 7264491, 3659dbf, 2173be9). Full walkthrough steps + credential prerequisites (Keycloak `donnarec-frontend` confidential client secret, `donnarec-web/.env.local`, admin-test/maker-test/checker-test passwords) documented in `.planning/phases/04-receipt-pdf-email-delivery-outbox-worker/04-06-SUMMARY.md` under "Deferred: Task 4 Human UI Walkthrough".

### Blockers/Concerns

Stakeholder confirmations are gated but NON-blocking for the roadmap; resolve at the relevant phase start:

- Phase 1: PDPA retention period (~5y vs erasure); email provider / KMS / hosting choices.
- Phase 4: §6 receipt wording + 1x/2x deduction eligibility (accounting/legal sign-off).
- Phase 5: exact e-Donation field spec + 1 Jan 2026 mandate obligation.

## Deferred Items

Items acknowledged and carried forward from previous milestone close:

| Category | Item | Status | Deferred At |
|----------|------|--------|-------------|
| *(none)* | | | |

## Session Continuity

Last session: 2026-07-11T04:05:39.355Z
Stopped at: Phase 6 UI-SPEC approved (dual-theme warm public + responsive)
Resume file: 
.planning/phases/06-public-donation-web-form-flow-b/06-UI-SPEC.md
