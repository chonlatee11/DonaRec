---
phase: 06-public-donation-web-form-flow-b
plan: 03
subsystem: api
tags: [go, gin, sqlc, pgx, multipart, captcha, rate-limiting, audit, outbox, envelope-encryption]

# Dependency graph
requires:
  - phase: 06-public-donation-web-form-flow-b
    provides: "06-01 (donations.source column, seeded public-web system user 00000000-0000-4000-8000-000000000006); 06-02 (captcha.Verifier/Middleware, ratelimit.PerIP, config.TurnstileSecretKey/RateLimit)"
  - phase: 03-approval-issuance-audit-rbac
    provides: "DonationService atomic WithTx shape (Approve), AppendAuditEntryTx (UUID actor requirement), CreateDonation/SubmitDonation/InsertSlip/EnqueueOutboxJob queries, crypto.EncryptField, storage.PutSlip magic-byte validation"
provides:
  - "DonationService.CreatePublicSubmission — one WithTx: create(flow_b)+submit+slip-ref+audit+ack_email enqueue, tax ID envelope-encrypted, rollback leaves no orphan row"
  - "PublicDonationRequest (donor-fields-only request struct; no captcha field — Pitfall 3)"
  - "PublicWebUserID Go constant (00000000-0000-4000-8000-000000000006, mirrors migration 000016) used as created_by AND the synthetic audit actor"
  - "PublicReferenceNumber helper (REF-<8 hex> derived from donation id, D-84 — never internal/receiptno)"
  - "PublicDonationHandler.CreatePublic + unauthenticated POST /api/public/donations route group (ratelimit.PerIP -> captcha.VerifyTurnstile -> handler; NO RequireAuth)"
  - "SlipPutter narrow interface (fakeable slip-store seam)"
  - "outbox job_type 'ack_email' (enqueued here; consumed by plan 06-04)"
  - "audit action 'donation.public_submit'"
  - "CreateDonation sqlc query now carries an explicit source param (Flow A passes flow_a, Flow B passes flow_b)"
affects: [06-04 (ack_email worker handler consumes the enqueued job), 06-06 (public form UI POSTs to /api/public/donations with the slip + turnstile_token contract), 06-07 (staff pending-review queue lists source=flow_b rows)]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Atomic public-submission tx (Approve's WithTx shape reused for create+submit+slip+audit+outbox — never Flow A's three separate calls)"
    - "Unauthenticated gin route group off the ROOT router with ratelimit+captcha substituting RequireAuth (first of its kind, ASVS V4: exactly one POST handler)"
    - "Narrow SlipPutter interface (mirrors ReceiptsStore) so the E2E injects a fake that runs REAL magic-byte validation while faking the MinIO PUT"
    - "Explicit source param on CreateDonation set at INSERT time (no post-insert UPDATE); every caller passes flow_a/flow_b explicitly"

key-files:
  created:
    - donnarec-api/internal/donation/create_public_submission_test.go
    - donnarec-api/internal/donation/public_submission.go
    - donnarec-api/internal/donation/public_handler.go
    - donnarec-api/cmd/server/e2e_public_test.go
  modified:
    - donnarec-api/internal/db/queries/donations.sql
    - donnarec-api/internal/db/generated/donations.sql.go
    - donnarec-api/internal/db/generated/querier.go
    - donnarec-api/internal/donation/service.go
    - donnarec-api/internal/donation/model.go
    - donnarec-api/cmd/server/main.go
    - donnarec-api/cmd/server/e2e_test.go

key-decisions:
  - "CreateDonation carries an explicit source param (sqlc regen) rather than a post-insert UPDATE or a separate query — the row is born with the correct source; Flow A's Create + Reissue call sites pass 'flow_a', CreatePublicSubmission passes 'flow_b'. Only 2 internal Flow A call sites touched, both fully test-covered."
  - "PublicWebUserID is an exported Go constant mirroring the fixed literal UUID seeded in migration 000016 as BOTH users.id and keycloak_subject — used for created_by (FK) AND the in-tx audit actor_id, so audit.parseUUID never rolls back the submission (Pitfall 1)."
  - "Reference number is PublicReferenceNumber(donation.id) = 'REF-'+first-8-uppercase-hex — derived from the UUID, generated at read time, never via internal/receiptno (whose sole call site stays Approve, D-35/D-84)."
  - "PublicDonationHandler depends on a narrow SlipPutter interface (not *storage.StorageClient) so the E2E can inject a fake that runs the REAL storage.ValidateSlip magic-byte check while faking only the network PUT — keeps the Conventions E2E hermetic (no MinIO testcontainer needed) yet genuinely exercises the 415 rejection path."
  - "publicGroup hangs off the ROOT router (not the /api group that applies RequireAuth) and registers EXACTLY ONE handler (POST /donations); ordering is ratelimit.PerIP (cheap reject) THEN captcha.VerifyTurnstile (outbound siteverify), matching 06-02's documented contract."
  - "The router-level AuditMiddleware still fires on the public POST with an empty actor (best-effort own-tx, post-commit) — its parseUUID('') fails cleanly BEFORE any INSERT (no partial row, no hash-chain corruption) and is only logged, never aborting the 201. The AUTHORITATIVE audit is the in-tx AppendAuditEntryTx under the public-web UUID."

patterns-established:
  - "Atomic unauthenticated create+submit+slip+audit+outbox in one WithTx (CreatePublicSubmission) — the template for any future single-shot public mutation"
  - "Fakeable narrow store interface at the handler boundary so the Conventions E2E can drive the real HTTP seam without external network dependencies"

requirements-completed: []

coverage:
  - id: D1
    description: "CreatePublicSubmission atomically creates one pending_review flow_b donation (encrypted tax ID, consent snapshot with Flow-B consent version, created_by=public-web), one slip reference, one ack_email outbox job (no issue_receipt), one in-tx audit row under the public-web UUID; a forced in-tx error rolls everything back (no orphan row)"
    requirement: "FR-02"
    verification:
      - kind: integration
        ref: "donnarec-api/internal/donation/create_public_submission_test.go#TestCreatePublicSubmission (HappyPath_AtomicPendingReviewFlowB + Rollback_ForcedError_NoOrphanRow)"
        status: pass
    human_judgment: false
  - id: D2
    description: "Unauthenticated POST /api/public/donations over the REAL router chain (ratelimit -> captcha -> handler -> service -> DB): 201 + REF- reference number; full record set created; bad-magic-byte slip -> 415 with no row; missing slip -> 400 with no row; per-IP burst exceeded -> 429"
    requirement: "FR-04"
    verification:
      - kind: e2e
        ref: "donnarec-api/cmd/server/e2e_public_test.go#TestPublicDonationE2E (HappyPath + BadMagicByteSlip + MissingSlip + RateLimit subtests, -race)"
        status: pass
    human_judgment: false
  - id: D3
    description: "PDPA consent snapshot (consent_given + consent_at + consent_text_version=public-form-v1 + consent_purpose) captured on the flow_b row via the reused D-49 fields (D-81)"
    requirement: "FR-03"
    verification:
      - kind: integration
        ref: "donnarec-api/internal/donation/create_public_submission_test.go#TestCreatePublicSubmission (asserts consent_given + consent_text_version on the response and row)"
        status: pass
    human_judgment: true
    rationale: "The BACKEND consent capture is proven, but FR-03's 'แสดง...ก่อนส่ง' (display consent before submit) is a donor-facing UI concern delivered by the public form (plan 06-06); marking FR-03 fully complete now would misrepresent the donor-facing loop."

# Metrics
duration: ~14min
completed: 2026-07-11
status: complete
---

# Phase 6 Plan 3: Public Submission Atomic Path (Flow B) Summary

**A donor's complete request lands atomically in pending_review as source=flow_b — tax ID envelope-encrypted, consent snapshotted, slip stored+referenced, submit audited under the seeded public-web UUID, and an ack_email outbox job enqueued — over the codebase's first unauthenticated HTTP route, proven end-to-end under -race.**

## Performance

- **Duration:** ~14 min
- **Started:** 2026-07-11T12:23:26Z
- **Completed:** 2026-07-11T12:37:00Z (approx.)
- **Tasks:** 3
- **Files modified:** 11 (4 created, 7 modified)

## Accomplishments
- `DonationService.CreatePublicSubmission` — the load-bearing vertical slice: one `dbhelpers.WithTx` closure does (1) `CreateDonation` with `source='flow_b'` + `created_by`=public-web, (2) `SubmitDonation` to land directly in `pending_review` in the SAME tx (never Flow A's two HTTP calls), (3) `InsertSlip` referencing the already-uploaded object, (4) `AppendAuditEntryTx` under the public-web UUID actor, (5) `EnqueueOutboxJob('ack_email')` — with the tax ID `crypto.EncryptField`-encrypted BEFORE the tx (plaintext never reaches Postgres) and a full rollback (zero orphan rows) on any in-tx error.
- `PublicDonationRequest` (donor-fields-only) + `PublicWebUserID` constant (mirrors migration 000016's fixed UUID — Pitfall 1) + `PublicReferenceNumber` helper (`REF-`+8 hex from the donation id, D-84 — never `internal/receiptno`).
- `CreateDonation` sqlc query extended with an explicit `source` param (regenerated); Flow A's `Create` and Void-&-Reissue call sites pass `'flow_a'`, so the shared path stays correct and the full Flow A suite still passes.
- `PublicDonationHandler.CreatePublic` + the unauthenticated `POST /api/public/donations` route group wired off the ROOT router with `ratelimit.PerIP` → `captcha.VerifyTurnstile` substituting `RequireAuth` (ASVS V4: exactly one handler exposed). Fail-fast ordering (validate fields → require slip (D-80) → `PutSlip` magic-byte/size FIRST → service) with distinct error shapes.
- `SlipPutter` narrow interface at the handler boundary so the Conventions E2E injects a fake that runs the REAL `storage.ValidateSlip` magic-byte check while faking only the MinIO PUT.
- `TestPublicDonationE2E` (under `-race`) drives the production router over multipart `POST /api/public/donations`: 201 + `REF-` reference; one `pending_review` `flow_b` row with `created_by`=public-web; one slip row; one `ack_email` job (no `issue_receipt`); one audit row; bad-magic slip → 415 (no row); missing slip → 400 (no row); per-IP burst exceeded → 429.

## Task Commits

Each task was committed atomically (plan `type: tdd` — Task 1 followed the RED→GREEN gate):

1. **Task 1: CreatePublicSubmission atomic tx (TDD)**
   - RED: `f330b6e` (test) — `create_public_submission_test.go` referencing `CreatePublicSubmission`/`PublicDonationRequest`/`PublicWebUserID` (compile-level RED, symbol-adding change)
   - GREEN: `e158a73` (feat) — sqlc `source` param + Flow A call-site updates + request struct + const + helper + the atomic method; test passes
2. **Task 2: public_handler + unauthenticated route** - `0c765e8` (feat)
3. **Task 3: E2E over the real HTTP path** - `3e8c398` (test)

_No REFACTOR commit needed — the GREEN implementation mirrored the existing `Approve`/`Create`/`SlipService` patterns exactly._

## TDD Gate Compliance

The plan's `type: tdd` RED→GREEN sequence is present in git log:
- RED (`f330b6e`, `test(06-03)`): the new test failed to compile (`undefined: donation.PublicDonationRequest`) — a schema/symbol-adding change, the same compile-level RED gate 06-01 used.
- GREEN (`e158a73`, `feat(06-03)`): implementation added; `TestCreatePublicSubmission` passes all four behaviors; the full `internal/donation` suite stays green (Flow A unaffected).

Both gate commits present — compliant.

## Verification Results
- `go test ./internal/donation/... -run TestCreatePublicSubmission -count=1` — **pass** (3.9s)
- `go test ./internal/donation/... -count=1` (full package, Flow A regression) — **pass** (66s)
- `go test ./cmd/server/... -run TestPublicDonationE2E -race -count=1` — **pass** (25s)
- `go build ./...` — **green**
- `go vet ./cmd/server/... ./internal/donation/...` — **clean**

## Threat Mitigations (from plan threat_model)
- **T-06-09** (slip file-type spoofing): `storage.PutSlip` magic-byte detection runs BEFORE any DB write; the E2E's bad-magic subtest proves 415 + no row. **mitigated.**
- **T-06-10** (tax ID at rest): `crypto.EncryptField` AES-256-GCM envelope before the tx; test asserts ciphertext present and plaintext never in the column. **mitigated.**
- **T-06-11** (unauthenticated reachable mutations): `publicGroup` registers ONLY `POST /donations`; no update/delete/reveal/approve exposed. **mitigated.**
- **T-06-12** (public submit without an actor): in-tx `AppendAuditEntryTx` with the fixed public-web UUID; test asserts exactly one `donation.public_submit` audit row. **mitigated.**
- **T-06-13** (partial submission / orphan row): single `WithTx`; slip validated+PUT before the tx; rollback subtest + E2E negative subtests prove no orphan `donations` row. **mitigated.**
- **T-06-14** (verbose enumeration): handler returns generic validation shapes; no donor-master/dedup exists to enumerate (D-43). **mitigated.**

## Decisions Made
- **Explicit `source` on `CreateDonation`** rather than a post-insert `UPDATE` or a duplicate query — the honest, single-write design the plan intended ("ensure CreateDonation carries source … regen if needed"). Blast radius: 2 internal Flow A call sites (`Create` line ~202, Reissue line ~1049), both set `'flow_a'`, both covered by the (passing) full `internal/donation` suite.
- **`PublicWebUserID` as an exported Go constant** mirroring the migration-000016 literal — one source of truth for the FK target and the synthetic audit actor; a readable sentinel would have tripped `audit.parseUUID` and rolled back every submission (Pitfall 1).
- **`SlipPutter` interface** at the handler boundary — lets the Conventions E2E stay hermetic (no MinIO testcontainer) while still exercising the real magic-byte rejection path, mirroring the codebase's `ReceiptsStore`/`fakeSettingsStore` convention.
- **`ack_email` enqueued but not consumed here** — the worker's `ProcessOnce` switch has no `ack_email` case yet (plan 06-04 adds it). Enqueued jobs sit `pending`; no worker runs in the E2E harness, so they simply accumulate harmlessly. `job_type` has no CHECK constraint, so `'ack_email'` inserts cleanly.

## Deviations from Plan

### Auto-fixed / adjusted

**1. [Rule 3 - Blocking] `setupRouter` signature change forced a same-commit update to the shared E2E harness**
- **Found during:** Task 2
- **Issue:** Adding the public route group required 4 new `setupRouter` params (`publicDonationHandler`, `captchaMW`, `rlRate`, `rlBurst`). The shared `newE2EHarness` (used by `TestE2E_MakerChecker*`, `TestE2E_AdminSettings`, `TestE2E_EdonationExport`) calls `setupRouter`, so `go vet ./cmd/server/...` (which compiles tests) would break unless the harness was updated in the SAME commit.
- **Fix:** Task 2's commit also updated `newE2EHarness` to construct a fake captcha verifier, a fake `SlipPutter`, and a deterministic rate limiter — additive, no behavior change to the existing E2E tests. The actual public E2E test cases landed in Task 3.
- **Files modified:** `cmd/server/e2e_test.go`
- **Committed in:** `0c765e8`

**2. [Rule 2 - Design] Introduced the `SlipPutter` narrow interface (not in the plan's literal wording)**
- **Found during:** Task 2/3
- **Issue:** The plan said the handler calls `storage.PutSlip` (concrete). But the Conventions E2E must drive the real handler→service→DB seam without a MinIO round-trip, and a concrete `*storage.StorageClient` pointed at a dummy endpoint would fail the real network PUT (or force a MinIO testcontainer into the shared harness, slowing every E2E test).
- **Fix:** Depend on a narrow `SlipPutter` interface (satisfied by `*storage.StorageClient` in production; a fake in the E2E that runs REAL `storage.ValidateSlip` and fakes only the PUT). This is the codebase's established narrow-interface convention (`ReceiptsStore`), and it makes the 415 bad-magic path genuinely testable.
- **Files modified:** `internal/donation/public_handler.go`, `cmd/server/main.go`, `cmd/server/e2e_test.go`
- **Committed in:** `0c765e8`

**3. [Rule 1 - Bug] Did NOT mark FR-01/FR-02/FR-03 complete despite the plan frontmatter listing them**
- **Found during:** state/requirements update step
- **Issue:** This plan's frontmatter lists `requirements: [FR-01, FR-02, FR-03, FR-04]`. FR-04 was already completed by plan 06-02 (the captcha/rate-limit primitives). FR-01 ("ผู้บริจาคกรอกแบบฟอร์ม"), FR-02 ("ผู้บริจาคอัปโหลดสลิป"), FR-03 ("แสดงและบันทึก consent … ก่อนส่ง") are donor-FACING capabilities: this plan built only the BACKEND submission path — the donor-facing public form is plan 06-06. Mechanically checking these boxes would misrepresent phase progress (the same trap 06-01 documented for FR-01/FR-08).
- **Fix:** Left FR-01/FR-02/FR-03 as `[ ]` in REQUIREMENTS.md; did not run `requirements mark-complete`. They will be completed by plan 06-06 (the public form UI) which closes the donor-facing loop. The backend deliverables are recorded in this SUMMARY's coverage block (D1/D2/D3) so the verifier can still route to them.
- **Files modified:** none (REQUIREMENTS.md deliberately unchanged)

**Total deviations:** 3 (1 blocking harness update, 1 design interface addition, 1 deliberate requirement-completion deferral). None changed the plan's intended behavior or scope.

## Known Stubs
None — every deliverable is wired end-to-end. The one deferred consumer (`ack_email` worker handler) is an explicit, planned dependency (06-04), not a stub: the job is genuinely enqueued and will be processed once 06-04 adds the switch case.

## Issues Encountered
None blocking. Note: the router-level `AuditMiddleware` best-effort own-tx write fires on the public POST with an empty actor and fails `parseUUID('')` (logged, never aborts, no partial row) — this is pre-existing before-auth middleware behavior; the authoritative audit is the in-tx entry under the public-web UUID. Not fixed here (out of scope; harmless).

## User Setup Required
None for this plan. Production deployment will need `TURNSTILE_SECRET_KEY` set once the Cloudflare Turnstile site is provisioned (stakeholder/ops item, per 06-02) — the fail-closed verifier rejects all submissions if unset rather than accepting bots.

## Next Phase Readiness
- **06-04** can add a `case "ack_email":` arm to `worker.ProcessOnce` and a `handleAckEmail` handler; the outbox rows are already being enqueued with `{"donation_id": "..."}` payloads.
- **06-06** (public form UI) POSTs `multipart/form-data` to `/api/public/donations` with donor fields, the slip under form field **`slip`** (`donation.PublicSlipField`), and the CAPTCHA token under **`turnstile_token`** (`captcha.TokenField`); on success it reads `data.reference_number` (the `REF-` code) and `data.status`.
- **06-07** (staff queue) lists these via the existing `GET /api/donations?source=flow_b&status=pending_review`.

---
*Phase: 06-public-donation-web-form-flow-b*
*Completed: 2026-07-11*

## Self-Check: PASSED

All 4 created files confirmed present on disk; all 4 commit hashes (`f330b6e`, `e158a73`, `0c765e8`, `3e8c398`) confirmed present in `git log --oneline --all`.
