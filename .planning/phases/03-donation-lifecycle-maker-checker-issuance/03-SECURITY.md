---
phase: 3
slug: donation-lifecycle-maker-checker-issuance
status: verified
threats_open: 0
asvs_level: 1
created: 2026-07-04
---

# Phase 3 — Security: Donation Lifecycle, Maker-Checker, Issuance

> Per-phase security contract: threat register, accepted risks, and audit trail.
>
> **Disposition: SECURED — 62/62 threats closed (56 mitigate + 6 accept). threats_open: 0.**
>
> Register authored at plan time (`03-01`..`03-13`). This audit verifies declared
> dispositions against **implemented code** (file:line evidence) — it does not scan
> for new threats. Verified by `gsd-security-auditor` (sonnet), 2026-07-04.

---

## Trust Boundaries

| Boundary | Description | Data Crossing |
|----------|-------------|---------------|
| browser → Next BFF (server) | Untrusted client request; access token must stay server-side, never returned to browser | Bearer session, donor write payloads, file uploads |
| Next BFF → Go API | Server-side Bearer forward; Go re-verifies token + re-enforces RBAC/SoD/audit (BFF is not the authz authority) | Bearer token, donation data, national/tax ID (TLS) |
| client → Go API endpoints | Untrusted donor data, tax ID, search filters, file bytes cross here | PII, query params, slip bytes |
| Go service → PostgreSQL | PII must be ciphertext before it reaches Postgres; DB CHECK/REVOKE are last line of defense | AES-GCM ciphertext, receipt numbers, audit rows |
| Go service → logs | PII must never enter log/error output | donation_id / user UUID only |
| Go service → MinIO | Object storage; presigned URLs grant time-bound read | slip images, 15-min presigned URLs |
| checker → /approve, /cancel, /pii | Privileged transitions that mint a legal receipt number / decrypt PII | receipt number allocation, plaintext PII |
| concurrent approvers → same row | Race that could double-issue a receipt number | `SELECT … FOR UPDATE` lock |
| third-party registries → build | Supply chain: only official npm / shadcn / Go module registries permitted | package installs |

---

## Threat Register

*Status: all `closed`. Disposition: mitigate (implementation verified) · accept (documented risk).*

### 03-01 — DB schema / constraints

| Threat ID | Category | Component | Disposition | Mitigation (verified evidence) | Status |
|-----------|----------|-----------|-------------|--------------------------------|--------|
| T-03-01 | Elevation of Privilege | SoD bypass at DB | mitigate | `migrations/000005_donations.up.sql`: `CONSTRAINT chk_sod_approver CHECK (approved_by IS NULL OR approved_by != created_by)` | closed |
| T-03-02 | Tampering | receipt number on non-issued record | mitigate | `000005`: `chk_receipt_only_on_issued_or_cancelled` ties receipt presence to status IN ('issued','cancelled') | closed |
| T-03-03 | Tampering | DELETE of donation/receipt row | mitigate | `000005`: `REVOKE DELETE ON donations FROM donnarec_app` | closed |
| T-03-04 | Information Disclosure | national/tax ID plaintext | mitigate | `000005`: `donor_tax_id_enc BYTEA`, `donor_tax_id_dek BYTEA` — no plaintext column in schema | closed |
| T-03-SC-01 | Tampering (supply chain) | minio-go | accept | `go.mod:15` `minio-go/v7 v7.2.1` official MinIO org — see Accepted Risks | closed |

### 03-02 — FE auth client

| Threat ID | Category | Component | Disposition | Mitigation (verified evidence) | Status |
|-----------|----------|-----------|-------------|--------------------------------|--------|
| T-03-05 | Spoofing | API client auth | mitigate | `donnarec-web/lib/api.ts:45-53,76,84-86`: bearer from `getServerSession(authOptions)`; no hardcoded token | closed |
| T-03-06 | Tampering (supply chain) | shadcn registry | mitigate | `components.json`: default `ui.shadcn.com/schema.json`; no third-party registry | closed |
| T-03-07 | Information Disclosure | PII rendered client-side | accept | No donor data reaches this layer in 03-02 scope — see Accepted Risks | closed |

### 03-03 — donation service create/PII

| Threat ID | Category | Component | Disposition | Mitigation (verified evidence) | Status |
|-----------|----------|-----------|-------------|--------------------------------|--------|
| T-03-08 | Information Disclosure | donor_tax_id in DB | mitigate | `service.go:130`: `crypto.EncryptField(...)` (DEK/KEK envelope) before INSERT; only `*_enc/*_dek` written | closed |
| T-03-09 | Information Disclosure | PII in API response | mitigate | `model.go:48-52`: `DonationResponse.DonorTaxIDMasked` only; plaintext field explicitly excluded | closed |
| T-03-10 | Information Disclosure | PII in logs/errors | mitigate | `service.go` (multiple): logs only `donation_id`/`created_by`/actor subject; no donor fields | closed |
| T-03-11 | Tampering | mass assignment | mitigate | `handler.go` (6 sites): `ShouldBindJSON` into explicit request structs, never into `db.Donation` | closed |
| T-03-12 | Elevation of Privilege | non-staff hits endpoints | mitigate | `cmd/server/main.go:236`: `donationGroup.Use(RequireAnyRole(Maker,Checker,Admin))`; E2E regression-tested | closed |
| T-03-13 | Tampering | edit non-draft record | mitigate | `service.go:305-319`: `LockDonationForUpdate` + `canTransition` in tx; `donations.sql:104-126` `WHERE status='draft'` backstop | closed |

### 03-04 — slip upload

| Threat ID | Category | Component | Disposition | Mitigation (verified evidence) | Status |
|-----------|----------|-----------|-------------|--------------------------------|--------|
| T-03-14 | Tampering | spoofed upload | mitigate | `storage/client.go:80-98`: `mimetype.Detect` allowlist (jpg/png/pdf), not caller Content-Type | closed |
| T-03-15 | Denial of Service | oversized upload | mitigate | `storage/client.go:83-85` + `:131` `io.LimitReader` dual-layer 10 MB cap | closed |
| T-03-16 | Information Disclosure | guessable slip URL | mitigate | `storage/client.go:147-151` `PresignedGet` 15-min TTL; `objectKey` includes `uuid.NewString()` | closed |
| T-03-17 | Repudiation | silent slip deletion | mitigate | `000006` `deleted_at` + `REVOKE DELETE`; `slip_service.go:203-230` `SoftDeleteSlip` + `AppendAuditEntryTx` in tx | closed |
| T-03-SC-04 | Tampering (supply chain) | minio-go | mitigate | `go.mod:15` v7.2.1 official; `go.sum:135` checksum verified | closed |

### 03-05 — approval / issuance

| Threat ID | Category | Component | Disposition | Mitigation (verified evidence) | Status |
|-----------|----------|-----------|-------------|--------------------------------|--------|
| T-03-18 | Elevation of Privilege | Maker approves own record | mitigate | `service.go:552-554` `if locked.CreatedBy == actingUserID { return ErrSoDViolation }` + DB `chk_sod_approver`; E2E `SoD_SelfApprove_403` | closed |
| T-03-19 | Tampering (concurrency) | double-issuance | mitigate | `service.go:535` `LockDonationForUpdate` (`FOR UPDATE`); `service_integration_test.go:510` `TestConcurrentApproval_ExactlyOneSucceeds` (5 goroutines → 1 success) | closed |
| T-03-20 | Tampering | partial issuance | mitigate | `service.go:530-610`: all 7 issuance effects inside one `WithTx` closure — any error rolls back all | closed |
| T-03-21 | Repudiation | issuance without audit | mitigate | `service.go:586-594` `AppendAuditEntryTx(ctx, tx, ...)` inside the tx, not best-effort | closed |
| T-03-22 | Tampering | receipt pre-computed on draft | mitigate | `service.go:514,559` `Allocate` sole call site (grep-verified) inside issuance tx + `chk_receipt_only_on_issued_or_cancelled` | closed |
| T-03-23 | Elevation of Privilege | non-checker hits approve | mitigate | `cmd/server/main.go:269-271` `checkerGroup.Use(RequireAnyRole(Checker,Admin))`; E2E `RBAC_MakerRejectedFromCheckerOnlyRoute` | closed |

### 03-06 — cancel / reveal / search

| Threat ID | Category | Component | Disposition | Mitigation (verified evidence) | Status |
|-----------|----------|-----------|-------------|--------------------------------|--------|
| T-03-24 | Tampering | cancel creates gap | mitigate | `donations.sql:183-197` `CancelDonation` never writes receipt fields; `REVOKE DELETE`; E2E `Cancel_RetainsReceiptNumber_RealPath` | closed |
| T-03-25 | Repudiation | keyed receipt cancelled w/o RD reconciliation | mitigate | `service.go:833-836` requires `RDConfirmationReason` when `EdonationKeyed`; recorded in audit | closed |
| T-03-26 | Information Disclosure | unauthorized PII reveal | mitigate | `service.go:1102` `if !pii.CanRevealFull(claims) { return ErrForbidden }` before DB access | closed |
| T-03-27 | Repudiation | reveal not audited | mitigate | `service.go:1123-1144` `AppendAuditEntryTx` (pii.reveal) before plaintext returned (`:1156-1159`), same tx | closed |
| T-03-28 | Injection | SQL injection via search | mitigate | `donations.sql:219-253` `SearchDonations`/`CountDonations` use `sqlc.narg(...)` named params only; no concat | closed |
| T-03-29 | Information Disclosure | enumerate by national ID | mitigate | `donations.sql` filter params exclude tax/national ID — not a filter param | closed |
| T-03-30 | Elevation of Privilege | Maker cancels a receipt | mitigate | `service.go:800-803` Checker/Admin gate in `Cancel` + `checkerGroup` route guard | closed |

### 03-07 — FE lifecycle UI

| Threat ID | Category | Component | Disposition | Mitigation (verified evidence) | Status |
|-----------|----------|-----------|-------------|--------------------------------|--------|
| T-03-31 | Elevation of Privilege | client hides SoD controls | mitigate | `ReviewActionPanel.tsx:100-126` branches on server flags; SoD controls absent from DOM; server authoritative, 403 mapped | closed |
| T-03-32 | Information Disclosure | donor ID unmasked | mitigate | `MaskedIdField.tsx:96-124` masked by default; plaintext only via audited reveal | closed |
| T-03-33 | Tampering | stale state 2nd approver | mitigate | `lib/donations.ts:224-232` 409 → `statusConflict` reload toast; server precondition authoritative | closed |

### 03-08 — FE upload / reveal / cancel dialogs

| Threat ID | Category | Component | Disposition | Mitigation (verified evidence) | Status |
|-----------|----------|-----------|-------------|--------------------------------|--------|
| T-03-34 | Information Disclosure | revealed PII persists client-side | mitigate | `MaskedIdField.tsx:59-61,90-92` `revealedId` in component state only; cleared on hide/unmount; reload re-masks | closed |
| T-03-35 | Tampering | client bypasses file validation | mitigate | `SlipUploadZone.tsx:69-70` client size check "UX-only"; server magic-byte is authority | closed |
| T-03-36 | Repudiation | keyed receipt cancelled w/o RD confirm | mitigate | `CancelDialog.tsx:77-78,198-223` forces RD reason when keyed; server re-checks (`ErrEDonationKeyedCancel` → 409) | closed |
| T-03-37 | Information Disclosure | consent text version mismatch | accept | version from config + recorded server-side (`consent_text_version`) — see Accepted Risks | closed |

### 03-09 — list search backend

| Threat ID | Category | Component | Disposition | Mitigation (verified evidence) | Status |
|-----------|----------|-----------|-------------|--------------------------------|--------|
| T-09-01 | Tampering/Injection | SearchDonations/CountDonations | mitigate | `donations.sql:219-271` sqlc-generated named params (as T-03-28) | closed |
| T-09-02 | Information Disclosure | DonationListItem | mitigate | `model.go:189-198` no tax/national ID field | closed |
| T-09-03 | DoS | page/limit params | mitigate | `service.go:1172-1179` limit clamped to 20 if ≤0 or >200; per_page fixed 20 | closed |
| T-09-SC | Tampering (supply chain) | go/sqlc toolchain | accept | no new Go modules at 03-09 — see Accepted Risks | closed |

### 03-10 — BFF list route

| Threat ID | Category | Component | Disposition | Mitigation (verified evidence) | Status |
|-----------|----------|-----------|-------------|--------------------------------|--------|
| T-10-01 | Information Disclosure | BFF route / lib/bff.ts | mitigate | `lib/bff.ts:113-150` `bffForward`: token via `getServerSession` server-side; never serialized to browser | closed |
| T-10-02 | Spoofing | /api/bff/donations | mitigate | `lib/bff.ts:119-121` 401 when no session; Go `RequireAuth` re-verifies Bearer independently | closed |
| T-10-03 | Information Disclosure | DonationListView | mitigate | `app/api/bff/donations/route.ts:27-30` PII never a search key; list carries no PII | closed |
| T-10-SC | Tampering (supply chain) | @tanstack/react-query,-table | mitigate | commit `e53aca7`; official `@tanstack/*` packages verified in `package.json` | closed |

### 03-11 — detail endpoint backend

| Threat ID | Category | Component | Disposition | Mitigation (verified evidence) | Status |
|-----------|----------|-----------|-------------|--------------------------------|--------|
| T-11-01 | Elevation of Privilege | can_* auth flags | mitigate | `service.go:1279-1358` `buildDetailResponse` computes flags server-side; mutations re-enforce SoD/RBAC independently | closed |
| T-11-02 | Information Disclosure | national_id in detail | mitigate | `model.go:121` `NationalIDMasked` only; E2E asserts `national_id_masked` never matches `\d{13}` | closed |
| T-11-03 | Spoofing/Identity | viewer_is_creator | mitigate | `service.go:1285-1291` `GetUserByKeycloakSubject` → compares resolved `users.id`, never raw `claims.Subject`; E2E regression | closed |
| T-11-04 | Information Disclosure | review_history/created_by | mitigate | `donations.sql:290-310` `actor_email AS actor_name` + reason text only; no PII column joined | closed |
| T-11-SC | Tampering (supply chain) | go/sqlc toolchain | accept | no new Go modules at 03-11 — see Accepted Risks | closed |

### 03-12 — BFF detail / pii / approve routes

| Threat ID | Category | Component | Disposition | Mitigation (verified evidence) | Status |
|-----------|----------|-----------|-------------|--------------------------------|--------|
| T-12-01 | Information Disclosure | [id]/pii BFF route | mitigate | `app/api/bff/donations/[id]/pii/route.ts:19-33` forwards to Go checker/admin-gated audited `/pii`; no bypass | closed |
| T-12-02 | Elevation of Privilege | approve/return/reject via BFF | mitigate | `[id]/approve/route.ts:7-18` Go re-enforces `RequireAnyRole` + SoD; BFF passes 403/409 through | closed |
| T-12-03 | Information Disclosure | token handling | mitigate | `lib/bff.ts:22-25` `getBffToken`/`bffForward` use `getServerSession` — token server-side only | closed |
| T-12-04 | Tampering | slip_url composition | mitigate | `[id]/route.ts:19-46` slip_url composed server-side via Go `GET /:id/slip` (15-min TTL owned by Go) | closed |
| T-12-SC | Tampering (supply chain) | vitest devDep | mitigate | commit `0de0f2f`; official `vitest` in `package.json` | closed |

### 03-13 — BFF create / update / slip / cancel routes

| Threat ID | Category | Component | Disposition | Mitigation (verified evidence) | Status |
|-----------|----------|-----------|-------------|--------------------------------|--------|
| T-13-01 | Tampering | slip upload via BFF | mitigate | `[id]/slip/route.ts:26-28,57-66` streams FormData without inspecting/trusting extension; Go is authority (T-03-14/15) | closed |
| T-13-02 | Information Disclosure | create/update national ID | mitigate | `app/api/bff/donations/route.ts` POST + `lib/bff.ts` forward over TLS; Go `EncryptField` at rest; no PII logged; token never in browser | closed |
| T-13-03 | Elevation of Privilege | cancel/reissue via BFF | mitigate | `[id]/cancel/route.ts:8-14` Go re-enforces Checker/Admin + `edonation_keyed` RD-confirmation guard | closed |
| T-13-04 | Repudiation | cancel/void-reissue | mitigate | `service.go` `Cancel`/`Reissue` write `AppendAuditEntryTx` inside same `WithTx` — UI/BFF cannot bypass | closed |
| T-13-SC | Tampering (supply chain) | npm | accept | no new npm packages at 03-13 — see Accepted Risks | closed |

---

## Accepted Risks Log

| Risk ID | Threat Ref | Rationale | Accepted By | Date |
|---------|------------|-----------|-------------|------|
| AR-03-01 | T-03-SC-01 | `minio-go/v7 v7.2.1` — official MinIO GitHub org release, checksum-verified via go.sum. Supply-chain risk accepted at plan time given package maturity and no alternative for the chosen object-storage backend. | Phase 3 plan 03-01 | 2026-07-04 |
| AR-03-02 | T-03-07 | PII is not rendered client-side within the 03-02 FE auth-client scope. Risk re-assessed and re-mitigated in 03-07/03-08 (T-03-32/T-03-34: masked-by-default + audited reveal). | Phase 3 plan 03-02 | 2026-07-04 |
| AR-03-03 | T-03-37 | Consent text version shown from server config and the served version recorded per record (`consent_text_version`) at capture time — accepted as sufficient without a separate version-integrity check in Phase 3 scope. | Phase 3 plan 03-08 | 2026-07-04 |
| AR-03-04 | T-09-SC | No new Go/sqlc toolchain packages installed in plan 03-09 (schema/query work only). | Phase 3 plan 03-09 | 2026-07-04 |
| AR-03-05 | T-11-SC | No new Go/sqlc toolchain packages installed in plan 03-11 (detail endpoint work only). | Phase 3 plan 03-11 | 2026-07-04 |
| AR-03-06 | T-13-SC | No new npm packages installed in plan 03-13 (BFF route composition reuses 03-10/03-12 dependencies). | Phase 3 plan 03-13 | 2026-07-04 |

---

## Unregistered Flags

None. All `## Threat Flags` sections across the 13 Phase 3 SUMMARY files (03-01..03-13) either report "no new surface" or map explicitly to threat IDs already present in the register (e.g. 03-05-SUMMARY's `threat_flag: auth_boundary` → T-03-12/T-03-18/T-03-23). No new, unmapped attack surface was found.

---

## Load-Bearing Control Cross-Check

| Control | Status | Evidence |
|---|---|---|
| Gap-less receipt numbering (DB row-lock in issuance tx) | VERIFIED | `LockDonationForUpdate` (`SELECT … FOR UPDATE`) + `Allocate` sole call site inside `WithTx`; `TestConcurrentApproval_ExactlyOneSucceeds` (5 goroutines → exactly 1 success) |
| SoD code-guard + DB CHECK | VERIFIED | `service.go:552-554` code guard; `chk_sod_approver` DB CHECK; E2E `SoD_SelfApprove_403` |
| PII AES-GCM encryption + masking-by-default + audited reveal | VERIFIED | `internal/crypto/aes_gcm.go` + `envelope.go` (DEK/KEK); `pii.MaskNationalID`/`CanRevealFull`; `RevealPII` audits before returning plaintext |
| RBAC route guards | VERIFIED | `cmd/server/main.go` donationGroup/checkerGroup/adminGroup guards; E2E RBAC subtests (reject + accept paths) |
| Magic-byte upload validation | VERIFIED | `internal/storage/client.go` `mimetype.Detect` allowlist + dual-layer 10 MB cap |
| Append-only audit + REVOKE DELETE | VERIFIED | `migrations/000002_audit_log.up.sql` `REVOKE UPDATE, DELETE ON audit_log`; every mutation calls `AppendAuditEntryTx` in same tx |
| BFF server-side token handling | VERIFIED | `lib/bff.ts` `bffForward`/`getBffToken` — `getServerSession` only; Vitest "no-token-leak" test |
| Identity resolution: resolved `users.id` vs raw `claims.Subject` | VERIFIED | `auth.ResolveAppUser` middleware; `buildDetailResponse` uses `GetUserByKeycloakSubject`; E2E regression-tests `created-by-fk-mismatch` class |

---

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open | Run By |
|------------|---------------|--------|------|--------|
| 2026-07-04 | 62 | 62 | 0 | gsd-security-auditor (sonnet) via /gsd:secure-phase 3 |

The Phase 3 integration-test gate (per `CLAUDE.md` CONVENTIONS) is independently satisfied:
`cmd/server/e2e_test.go:TestE2E_MakerCheckerIssuancePipeline` drives the real
HTTP → RequireAuth (real signed JWT) → RequireAnyRole → ResolveAppUser → handler → service → DB
seam with 7 subtests covering SoD, RBAC (accept + reject), audience validation, identity
resolution, and the gap-less-cancel invariant — the regression guard for the three previously
shipped seam bugs (`created-by-fk-mismatch`, `fe-be-audience-mismatch`, RBAC AND-bug), now closed
per `03-VERIFICATION.md`.

---

## Sign-Off

- [x] All threats have a disposition (mitigate / accept / transfer)
- [x] Accepted risks documented in Accepted Risks Log
- [x] `threats_open: 0` confirmed
- [x] `status: verified` set in frontmatter

**Approval:** verified 2026-07-04
