---
slug: created-by-fk-mismatch
status: resolved
trigger: "donation service sets created_by = claims.Subject (Keycloak sub UUID) but donations.created_by REFERENCES users(id) which is a random gen_random_uuid() independent of keycloak_subject. Real Keycloak login flow will FK-violate on donation INSERT. Integration tests mask this by setting claims.Subject = users.id. GetUserByKeycloakSubject exists but is never called."
created: 2026-07-02
updated: 2026-07-02
phase: 03-donation-lifecycle-maker-checker-issuance
tdd_mode: true
goal: find_and_fix
---

# Debug Session: created-by-fk-mismatch

## Symptoms

- **Expected behavior:** A Maker logged in via Keycloak can create a donation draft (`POST /donations`); `created_by` is persisted and the record is retrievable. The same identity model must hold for `reviewed_by`, `approved_by`, `cancelled_by` (all `REFERENCES users(id)`).
- **Actual behavior (predicted, not yet reproduced live):** With a real Keycloak token, `created_by = claims.Subject` (the Keycloak `sub` UUID) does not match any `users.id` (which is `gen_random_uuid()`), so `INSERT INTO donations` violates the FK `created_by REFERENCES users(id)`. Donation creation fails for every real user.
- **Error messages:** Expected Postgres `23503 foreign_key_violation` on `donations_created_by_fkey` at donation Create. (Not yet captured from a live run — stack bring-up blocked by a host port 5432 conflict; unrelated infra issue.)
- **Timeline:** Introduced in Phase 3 (donation module). Phase 1 defined `users.id UUID PRIMARY KEY DEFAULT gen_random_uuid()` + separate `keycloak_subject TEXT`. Phase 3 wired `created_by = claims.Subject` against `REFERENCES users(id)`.
- **Reproduction (proposed, no live Keycloak needed):** In an integration test that mirrors real provisioning — insert a user with a random `id` and a distinct `keycloak_subject`, then call `svc.Create` with `claims.Subject = <keycloak_subject>` (≠ `users.id`). Expect FK violation. Contrast with existing tests which set `claims.Subject = makerRow.ID.String()`, hiding the defect.

## Evidence (from static investigation)

- migrations/000001_init_schema.up.sql:24 — `users.id UUID PRIMARY KEY DEFAULT gen_random_uuid()`
- migrations/000001_init_schema.up.sql:27 — `keycloak_subject TEXT NOT NULL UNIQUE` (KC 'sub' stored separately)
- migrations/000005_donations.up.sql:44 — `created_by UUID NOT NULL REFERENCES users(id)` (also reviewed_by:78, approved_by:83, cancelled_by:91)
- internal/donation/service.go:117-118,170 — `createdByUUID.Scan(claims.Subject)` → `CreatedBy: createdByUUID`
- internal/auth/middleware.go — sets `c.Set("claims", claims)`; `claims.Subject` = raw KC `sub`; NO subject→users.id resolution
- internal/auth/claims.go:11-12 — `Subject string json:"sub"` = Keycloak user ID
- internal/db/queries/users.sql:9-21 — `CreateUser` inserts with `id` defaulted (random), `keycloak_subject` = KC sub
- scripts/seed-admin.sh — mirrors user with `id` default (random), `keycloak_subject = KC_USER_ID`
- internal/db/generated/querier.go:71 — `GetUserByKeycloakSubject` generated but NEVER called in donation/audit flow (grep confirms)
- internal/audit/middleware.go:77 — `actorID = claims.Subject` (same assumption applied to audit actor)
- internal/donation/service_integration_test.go:122-123 — `Subject: makerRow.ID.String()` fakes `claims.Subject == users.id`, masking the defect

## Evidence (live reproduction — TDD RED confirmation)

- timestamp: 2026-07-02T07:04Z
  checked: Ran `internal/donation/service_fk_repro_test.go::TestCreate_RealKeycloakSubject_ReproducesFKMismatch` via `go test -race -count=1 -run TestCreate_RealKeycloakSubject_ReproducesFKMismatch ./internal/donation/ -v` against testcontainers Postgres 17.
  found: Provisioned a user via `queries.CreateUser` with DB-generated `users.id` (random) and a distinct realistic Keycloak-shaped UUID `keycloak_subject`. Built `auth.KeycloakClaims{Subject: keycloakSub}` — exactly what a real OIDC-validated request carries (Subject = raw KC sub, NOT users.id). Called `svc.Create(...)`. Result: `Create` returned an error; asserted (and confirmed passing) that the error unwraps via `errors.As` to `*pgconn.PgError` with `Code == pgerrcode.ForeignKeyViolation` ("23503") and `ConstraintName == "donations_created_by_fkey"`. Test run: `--- PASS: TestCreate_RealKeycloakSubject_ReproducesFKMismatch (2.50s)` — i.e. the predicted buggy behavior (FK violation) was reproduced exactly, live, against a real Postgres instance (not inferred from reading code).
  implication: Root cause is CONFIRMED with direct, repeatable evidence — not just static inference. Any real Keycloak-authenticated Maker will get a hard 500 (FK violation) on every `POST /donations`, because `claims.Subject` (KC sub) is never resolved to the corresponding `users.id` before being written to `created_by`.

- timestamp: 2026-07-02T07:10Z
  checked: grep for every `REFERENCES users(id)` column across all migrations, plus who writes to each.
  found: Exactly 6 FK columns are affected, all fed directly from `claims.Subject` with zero sub→id resolution: `donations.created_by` (NOT NULL), `donations.reviewed_by`, `donations.approved_by`, `donations.cancelled_by` (migrations/000005_donations.up.sql:44,78,83,91 — internal/donation/service.go) and `slip_attachments.uploaded_by` (NOT NULL), `slip_attachments.deleted_by` (migrations/000006_slip_attachments.up.sql:33,38 — internal/donation/slip_service.go:113,145,214,233). `user_roles.user_id` (migrations/000001_init_schema.up.sql:39) is also `REFERENCES users(id)` but is populated from an app-generated `users.id` at provisioning time, not from `claims.Subject` — NOT affected.
  implication: The bug's blast radius is precisely: 2 tables (`donations`, `slip_attachments`), 6 columns, all populated via `donation` package services from `claims.Subject`. Whichever fix direction is chosen must cover all 6 call sites (service.go: Create/Approve/Return/Reject/Cancel/Reissue; slip_service.go: upload/delete).

- timestamp: 2026-07-02T07:11Z
  checked: migrations/000002_audit_log.up.sql (audit_log table definition) — re-examined the earlier assumption that `audit_log.actor_id` shares this bug.
  found: `audit_log.actor_id` is `UUID NOT NULL` with **no FK constraint** to `users(id)` at all (grep of all `REFERENCES users` confirms audit_log is absent from that list). The column comment explicitly states `-- Keycloak 'sub' claim` — i.e. audit_log is BY DESIGN storing the raw, immutable Keycloak identity, decoupled from the mutable `users` table (a user row could later be deactivated/deleted without invalidating historical audit trail integrity).
  implication: CORRECTION to earlier static evidence — `audit/middleware.go:77` (`actorID = claims.Subject`) is NOT a bug instance; it is working exactly as designed. The root cause is scoped ONLY to genuine `REFERENCES users(id)` FK columns (the 6 identified above), not to `audit_log.actor_id`. Any fix must NOT touch audit_log's actor identity semantics — audit should keep recording the raw KC sub regardless of which fix direction (a) or (b) is chosen for the donation/slip FK columns.

## Evidence (fix implementation + full regression run — TDD GREEN confirmation)

- timestamp: 2026-07-02T07:19Z
  checked: Ran `go test -race -count=1 ./internal/donation/... ./internal/audit/...` immediately after implementing resolveUserID and inverting the repro test — first regression pass.
  found: `TestCreate_RealKeycloakSubject_ReproducesFKMismatch` went GREEN (`--- PASS`, `resp.CreatedBy == userRow.ID.String()`, DB row cross-checked). `internal/audit` suite: 7/7 PASS (unaffected, as predicted — audit_log.actor_id still stores raw claims.Subject). BUT `internal/donation` suite: 19/33 pre-existing tests FAILED, all with the identical error shape `donation: authenticated identity is not a provisioned application user` (ErrUserNotProvisioned) — i.e. these tests set `claims.Subject = makerRow.ID.String()` / `userRow.ID.String()` (the users.id, not the real keycloak_subject literal), which resolveUserID now correctly rejects since GetUserByKeycloakSubject looks up by keycloak_subject, not users.id.
  implication: Exactly matches the checkpoint response's explicit prediction ("existing tests set claims.Subject = makerRow.ID.String() (which masked the bug) — verify these still pass now that resolution is real rather than accidental"). This is NOT a regression in the fix — it is the masking pattern being exposed, confirming the fix's correctness. All 19 failing tests (18 in internal/donation/service_integration_test.go, 2 in internal/donation/service_test.go — TestConsentCapture, TestStateMachine_InvalidTransitions, which use the identical masking pattern in a different file) needed their `claims.Subject` fixture values corrected to the real `KeycloakSubject` literal used at `queries.CreateUser(...)`.

- timestamp: 2026-07-02T07:21Z
  checked: Fixed all 19 masked test fixtures (Subject: XRow.ID.String() -> Subject: "<matching KeycloakSubject literal>"); fixed 2 call sites (TestIssuanceTransaction_RollbackOnError, TestOutboxAtomicity) that bypass DonationService.Approve and scan claims.Subject directly into a raw pgtype.UUID for a low-level qtx.IssueDonation call — changed to use `checkerRow.ID` directly instead of `checkerUUID.Scan(checkerClaims.Subject)`. Re-ran the same two suites.
  found: NEW failure class appeared: `audit approve: audit: invalid actor_id "checker-outbox-kc": cannot parse UUID checker-outbox-kc` (and equivalents for return/reject/concurrent-approve). Root cause: `audit_log.actor_id` IS a genuine Postgres `UUID` column (confirmed again — no FK, but still UUID-typed, not TEXT), and the test literals used as fake KeycloakSubject values (e.g. "checker-outbox-kc", "maker-rollback-kc") are human-readable slugs, NOT valid UUID strings. Previously this was masked because claims.Subject was set to `checkerRow.ID.String()` (a real UUID) instead of the literal. Real Keycloak `sub` claims are ALWAYS valid UUIDs by Keycloak convention (confirmed by TestCreate_RealKeycloakSubject_ReproducesFKMismatch's own `uuid.NewString()` usage) — so this is a test-fixture realism gap, not a production code bug and not a flaw in the fix.
  implication: Fix code (resolveUserID, audit ActorID: claims.Subject passthrough) is correct and requires NO further changes. Test fixtures across internal/donation/service_integration_test.go and internal/donation/service_test.go needed their 30 human-readable KeycloakSubject literals ("maker-rollback-kc" etc.) replaced with valid UUID-format strings (via a scripted 1:1 mapping) so they realistically model what a real Keycloak sub looks like — consistent with the TDD repro test's own approach.

- timestamp: 2026-07-02T07:26Z
  checked: Re-ran `go test -race -count=1 ./internal/donation/... ./internal/audit/... -v` after the UUID-literal fixture fix, plus `go test -race -count=1 ./internal/auth/... -v` for completeness (auth package touches claims.Subject shape but was not modified).
  found: 33/33 tests PASS in internal/donation (including TestCreate_RealKeycloakSubject_ReproducesFKMismatch and the race-sensitive TestConcurrentApproval_ExactlyOneSucceeds under `-race`), 7/7 PASS in internal/audit, 3/3 top-level test groups PASS in internal/auth. Zero FAILs, exit code 0.
  implication: Fix is verified GREEN with zero regressions across all three related packages.

- timestamp: 2026-07-02T07:27Z
  checked: Ran full-repo `go test -race -count=1 ./...` as a final sanity pass.
  found: 2 unrelated failures — `TestCancelRetainsReceiptNumber` (internal/donation) and `TestAllocator_NewFiscalYearStartsAtOne` (internal/receiptno, a package never touched by this fix) both failed with `port "5432/tcp" not found` — a testcontainers/Docker port-contention flake from many packages' Postgres containers starting in parallel (same class of "unrelated infra issue" already flagged in this debug session's original Symptoms.errors entry). Re-ran BOTH failing tests individually in isolation immediately after: both PASS (`--- PASS: TestCancelRetainsReceiptNumber (2.58s)`, `--- PASS: TestAllocator_NewFiscalYearStartsAtOne (2.57s)`).
  implication: Confirmed Docker/testcontainers parallel-execution flakiness, not a code regression — receiptno package source was never modified by this fix, and TestCancelRetainsReceiptNumber already passed cleanly in the dedicated internal/donation + internal/audit run at 07:26Z.

## Current Focus

- hypothesis: The app assumes `claims.Subject` (KC sub) IS `users.id`, but provisioning generates an independent random `users.id`. There is no subject→id resolution layer, so real Keycloak logins FK-violate on any write referencing users(id). Fix is either (a) resolve sub→app-id in auth middleware and pass app id downstream, or (b) make provisioning set `users.id = sub` (align PK with KC sub).
- test: (TDD) added internal/donation/service_fk_repro_test.go::TestCreate_RealKeycloakSubject_ReproducesFKMismatch — provisions user with random users.id + distinct realistic keycloak_subject (UUID), calls svc.Create with claims.Subject = keycloak_subject (not users.id), asserts current failure is a Postgres FK violation on donations_created_by_fkey.
- expecting: FK violation (23503) on donations_created_by_fkey.
- next_action: DONE. Fix implemented, TDD test inverted RED->GREEN, full internal/donation (33/33) + internal/audit (7/7) + internal/auth (3/3 groups) suites PASS under -race with zero regressions (2 unrelated Docker-port-contention flakes in full-repo run confirmed to pass in isolation). Session resolved via automated self-verification (Auto Mode — no interactive human checkpoint was available; see checkpoint_response in the original CHECKPOINT REACHED turn). No further action needed unless a real end-to-end walkthrough (Keycloak login + POST /donations) later surfaces something this test suite could not observe (e.g. actual token shape from a live Keycloak realm).
- fix_implementation:
    approach: "Option (a) — resolve sub -> users.id via GetUserByKeycloakSubject, propagate downstream."
    helper: "resolveUserID(ctx, queries *db.Queries, claims auth.KeycloakClaims) (pgtype.UUID, error) — new private func in internal/donation/service.go. Calls queries.GetUserByKeycloakSubject(ctx, claims.Subject); returns ErrUserNotProvisioned (new sentinel, 403) on pgx.ErrNoRows."
    call_sites_updated:
      - "internal/donation/service.go: Create (createdByUUID), Approve (approverUUID), Return (reviewerUUID), Reject (reviewerUUID), Cancel (cancellerUUID), Reissue (actorUUID — covers both CancelDonation.CancelledBy and CreateDonation.CreatedBy in the same method)"
      - "internal/donation/slip_service.go: UploadSlip (pgUploaderID), RemoveSlip (pgActorID)"
    not_touched: "internal/audit/middleware.go:77 and all AuditEntry{ActorID: claims.Subject} call sites in service.go/slip_service.go — audit_log.actor_id intentionally still stores raw claims.Subject (correct-by-design per Evidence timestamp 07:11Z, no FK constraint)."
    handler_mapping: "internal/donation/handler.go + slip_handler.go: added `case errors.Is(err, ErrUserNotProvisioned): c.JSON(http.StatusForbidden, ...)` to Create/Approve/Return/Reject/Cancel/Reissue and slip Upload/Remove switches — was previously guidance's flagged blind spot (must not silently proceed with zero-value UUID)."
- reasoning_checkpoint:
    hypothesis: "claims.Subject (Keycloak JWT 'sub') is written directly as created_by/reviewed_by/approved_by/cancelled_by (donations) and actor_id (audit_log) — all of which are/should-correspond-to users.id — but users.id is an independently DB-generated gen_random_uuid() that is never reconciled with keycloak_subject at provisioning time, so claims.Subject != users.id for every real user."
    confirming_evidence:
      - "Static: migrations/000001_init_schema.up.sql:24 defines users.id as gen_random_uuid() default; :27 defines keycloak_subject as a separate TEXT UNIQUE column — two independent identifiers by design."
      - "Live: TestCreate_RealKeycloakSubject_ReproducesFKMismatch reproduced the exact predicted failure against real Postgres — Create() with a realistic non-users.id claims.Subject returns *pgconn.PgError{Code: 23503, ConstraintName: donations_created_by_fkey}."
    falsification_test: "If claims.Subject were in fact guaranteed to equal users.id (e.g. if provisioning already set users.id = sub), the same test would return no error. It did not — hypothesis is not falsified."
    fix_rationale: "PENDING — two viable root-cause-addressing directions exist and were intentionally NOT chosen yet (architectural identity decision, see CHECKPOINT): (a) resolve sub -> users.id once in auth middleware / a lookup helper and propagate the resolved app id downstream to donation service + audit middleware, or (b) align users.id = keycloak_subject at provisioning time so no resolution step is ever needed. Both address the root cause (identity mismatch) rather than a symptom (e.g. relaxing/removing the FK, which would NOT be root-cause-addressing and would silently corrupt audit/SoD integrity)."
    blind_spots: "Have not yet checked how audit_log.actor_id is typed/constrained (TEXT vs FK) — if it's an unconstrained TEXT column it currently stores raw claims.Subject without erroring, which is a related but silent (non-FK-enforced) instance of the same defect and must be covered by whichever fix is chosen. Have not yet inspected internal/users/handler.go or the Keycloak realm event/webhook (if any) for how/when a users row is actually provisioned relative to first login in production (may affect fix (a) vs (b) tradeoffs — e.g. does provisioning happen synchronously on first token validation, or via an admin-driven onboarding step?)."
- tdd_checkpoint:
    test_file: "internal/donation/service_fk_repro_test.go"
    test_name: "TestCreate_RealKeycloakSubject_ReproducesFKMismatch"
    status: "green"
    red_evidence: "Historical (see 'Evidence (live reproduction — TDD RED confirmation)' above): --- PASS: TestCreate_RealKeycloakSubject_ReproducesFKMismatch (2.50s) while asserting the buggy FK-violation as ground truth."
    green_evidence: "Test inverted to assert success + correct identity resolution (require.NoError on Create; resp.CreatedBy == userRow.ID.String() != keycloakSub; cross-checked directly against the donations.created_by DB column via queries.GetDonationByID). Result: --- PASS: TestCreate_RealKeycloakSubject_ReproducesFKMismatch (1.91s), confirmed in the final full-suite run (internal/donation: 33/33 PASS, internal/audit: 7/7 PASS, internal/auth: 3/3 top-level groups PASS, all under -race)."

## Refactor (post-resolution, user-directed 2026-07-02)

Fix direction (a) confirmed by user after a detailed (a)-vs-(b) comparison. Per user request, the sub→users.id resolution was hoisted from 8 per-call sites into a single middleware:

- NEW `internal/auth/user_resolver.go` — `ResolveAppUser(resolve UserIDResolver, logger)` gin middleware + `AppUserIDContextKey = "app_user_id"` + `UserIDResolver` type + `ErrUserNotProvisioned` sentinel. Auth pkg stays db-agnostic (injected resolver closure). Runs AFTER RequireAuth; unprovisioned sub → 403 `user_not_provisioned`.
- `cmd/server/main.go` — resolver closure over `queries.GetUserByKeycloakSubject`; `donationGroup.Use(auth.ResolveAppUser(...))` (scope = donationGroup only; admin group deliberately excluded so bootstrap provisioning still works).
- Service methods (Create/Approve/Return/Reject/Cancel/Reissue + slip Upload/Remove) now take explicit `actingUserID pgtype.UUID`; internal `resolveUserID` helper removed; `claims` kept for audit `ActorID` (raw sub) + roles. SoD now compares `locked.CreatedBy == actingUserID` (both users.id).
- Handlers extract `app_user_id` from gin context (Pattern A) and pass it down.
- Tests: repro reshaped to `TestCreate_ActingUserIDWritesCorrectCreatedBy` (service-level); new `TestResolveAppUser` (provisioned→sets id / unprovisioned→403 / missing claims→401).

Verification (independent, by orchestrator): `go build ./...` ✓, `go vet ./...` ✓, `go test -race -count=1 ./internal/donation/... ./internal/audit/... ./internal/auth/...` → all 3 packages `ok`, exit 0. New middleware + repro tests confirmed PASS individually.

## Eliminated

## Resolution

- root_cause: CONFIRMED (live reproduction). `users.id` (PK, `gen_random_uuid()` default) and `users.keycloak_subject` (KC 'sub', TEXT UNIQUE) are two independently-generated identifiers with no reconciliation step at provisioning. `internal/donation/service.go` (Create/Approve/Return/Reject/Cancel/Reissue) and `internal/donation/slip_service.go` (UploadSlip/RemoveSlip) wrote `claims.Subject` (raw KC sub) directly into columns that `REFERENCES users(id)`: `donations.{created_by,reviewed_by,approved_by,cancelled_by}` and `slip_attachments.{uploaded_by,deleted_by}` — 6 FK columns, 8 call sites. `internal/db/generated/querier.go`'s `GetUserByKeycloakSubject` existed (generated from `internal/db/queries/users.sql`) but was never called anywhere in the donation/slip flow. Result: any real Keycloak-authenticated write to these columns raised Postgres `23503 foreign_key_violation`. Confirmed NOT in scope: `audit_log.actor_id` is `UUID NOT NULL` with no FK to `users(id)` and is documented (migration comment) as intentionally storing the raw, immutable KC sub — correct-by-design, unchanged by this fix.
- fix: Implemented direction (a) per the automated session-manager decision (Auto Mode; checkpoint presented twice with no interactive reply). Added a new private helper `resolveUserID(ctx, queries *db.Queries, claims auth.KeycloakClaims) (pgtype.UUID, error)` in `internal/donation/service.go` that calls `queries.GetUserByKeycloakSubject(ctx, claims.Subject)` and returns the resolved `users.id`, or a new sentinel `ErrUserNotProvisioned` (mapped to HTTP 403) on `pgx.ErrNoRows` — explicitly handling the "valid Keycloak token but not provisioned in app DB" case rather than silently writing a zero-value UUID (closes the blind spot flagged in reasoning_checkpoint). Wired into all 8 call sites: `service.go` Create/Approve/Return/Reject/Cancel/Reissue, `slip_service.go` UploadSlip/RemoveSlip. `audit_log.actor_id` writes (`AuditEntry{ActorID: claims.Subject, ...}`) were deliberately left untouched — audit continues to store the raw Keycloak sub by design. Handler error mapping (`internal/donation/handler.go`, `internal/donation/slip_handler.go`) updated to map `ErrUserNotProvisioned` -> 403 in every affected switch statement.
- verification: DONE (automated/self-verified — Auto Mode, no interactive human checkpoint available). `TestCreate_RealKeycloakSubject_ReproducesFKMismatch` inverted to assert success + correct `users.id` resolution (verified both via the service response and a direct DB row read) — GREEN. Fixing the resolveUserID gap exposed 19 pre-existing tests that masked the bug via `claims.Subject = <row>.ID.String()`; all 19 fixture blocks corrected to use the real `KeycloakSubject` literal. That in turn exposed a second, unrelated realism gap — `audit_log.actor_id` is UUID-typed and the test literals were human-readable slugs, not valid UUIDs (never an issue in production, since real Keycloak subs are always UUIDs) — fixed by replacing all 30 KeycloakSubject test literals with valid UUID strings. Final verification: `go test -race -count=1 ./internal/donation/... ./internal/audit/... -v` → 33/33 + 7/7 PASS, 0 FAIL, exit 0. `go test -race -count=1 ./internal/auth/... -v` → 3/3 top-level groups PASS. Full-repo `go test -race -count=1 ./...` showed 2 unrelated Docker-port-contention flakes (`TestCancelRetainsReceiptNumber`, `TestAllocator_NewFiscalYearStartsAtOne`) that both PASS individually in isolation — confirmed infra flakiness, not a regression (receiptno package was never touched by this fix).
- files_changed:
  - internal/donation/service.go (resolveUserID helper added; Create/Approve/Return/Reject/Cancel/Reissue now resolve claims.Subject -> users.id instead of scanning claims.Subject directly)
  - internal/donation/slip_service.go (UploadSlip/RemoveSlip now resolve claims.Subject -> users.id)
  - internal/donation/errors.go (new sentinel ErrUserNotProvisioned)
  - internal/donation/handler.go (ErrUserNotProvisioned -> 403 mapping in Create/Approve/Return/Reject/Cancel/Reissue switches + doc comment)
  - internal/donation/slip_handler.go (ErrUserNotProvisioned -> 403 mapping in Upload/Remove switches + doc comment)
  - internal/donation/service_fk_repro_test.go (TDD reproduction test — inverted from RED to GREEN: now asserts success + correct users.id resolution instead of the FK violation)
  - internal/donation/service_integration_test.go (18 test fixtures fixed: claims.Subject corrected from `<row>.ID.String()` to the real KeycloakSubject literal, which unmasked the bug in tests; 2 low-level call sites (TestIssuanceTransaction_RollbackOnError, TestOutboxAtomicity) changed to use checkerRow.ID directly instead of scanning claims.Subject; all 27 KeycloakSubject literals replaced with valid UUID strings to realistically model real Keycloak subs, fixing a downstream audit_log.actor_id UUID-parse failure exposed by the same fix)
  - internal/donation/service_test.go (2 test fixtures fixed: TestConsentCapture, TestStateMachine_InvalidTransitions — same masking pattern as above; 2 KeycloakSubject literals replaced with valid UUID strings)
