---
phase: 01
slug: foundation-db-auth-rbac-audit-retention-model
status: verified
threats_open: 0
asvs_level: 2
created: 2026-06-24
---

# Phase 01 — Security

> Per-phase security contract: threat register, accepted risks, and audit trail.
> Register authored at plan time (01-01/01-02/01-03 `<threat_model>` blocks); this audit **verified each mitigation exists in implemented code** (State B → SECURED).

---

## Trust Boundaries

| Boundary | Description | Data Crossing |
|----------|-------------|---------------|
| Browser/Next.js → Go API | Untrusted Bearer JWT; validated against Keycloak JWKS | Access token (bearer) |
| Go API → Keycloak | API trusts only tokens signed by configured realm | OIDC token / JWKS |
| Go API → PostgreSQL | Parameterized queries only; app role privileges constrained | Queries + ciphertext PII |
| Operator → Keycloak realm config | Password policy / lockout applied via realm import | Realm config (data) |
| App role → audit_log | App may INSERT/SELECT, never UPDATE/DELETE (DB-enforced) | Audit rows |
| DBA → audit_log | DBA tampering detectable via hash-chain verification | Audit rows |
| Concurrent requests → audit_log | Serialized hash-chain link via advisory lock + FOR UPDATE | prev_hash linkage |
| App → PostgreSQL (PII fields) | Only ciphertext crosses; plaintext never persisted (PDPA) | AES-256-GCM ciphertext + wrapped DEK |
| KEK source (env/secrets) → app | KEK loaded at startup; never in source or DB | KEK (DONAREC_KEK) |
| Role → PII visibility | Maker sees mask; Admin/Checker may reveal (gated + audited) | National/tax ID |
| App/DBA → legal-hold record | Neither can hard-delete under legal_hold | Donor records |

---

## Threat Register

| Threat ID | Category | Component | Disposition | Mitigation | Status |
|-----------|----------|-----------|-------------|------------|--------|
| T-1-auth-01 | Spoofing | auth/middleware.go token validation | mitigate | go-oidc Verifier enforces sig+iss+aud; 401 otherwise (`middleware.go:41,49`); `TestOIDCMiddleware_Integration` wrong-aud→401 | closed |
| T-1-auth-02 | Spoofing | Keycloak login | mitigate | `bruteForceProtected:true` + `failureFactor:5` + `passwordPolicy length(8)` (`realm-donnarec.json:15,17,22`) | closed |
| T-1-auth-03 | Spoofing | session lifetime | mitigate | accessTokenLifespan 300s / ssoSessionIdleTimeout 1800s / maxLifespan 28800s (`realm-donnarec.json:24-26`) | closed |
| T-1-rbac-01 | Elevation of privilege | rbac.go RequireRoles | mitigate | server-side guard; missing role→403, no claims→401 (`rbac.go:33-62`); `TestRequireRoles_Unit` | closed |
| T-1-rbac-02 | Elevation of privilege | claims parsing | mitigate | roles read ONLY from `realm_access.roles` (`claims.go:19`); no top-level roles field; `rbac_test.go:133` | closed |
| T-1-tamper-01 | Tampering | SQL access | mitigate | sqlc `@param` / `$1..$n` placeholders; no string concat (`db/queries/audit.sql`, `audit/service.go`) | closed |
| T-1-config-01 | Tampering | Keycloak deploy mode | mitigate | `command: start --import-realm`; no `start-dev` (`docker-compose.yml:54`) | closed |
| T-1-info-01 | Information disclosure | logs | mitigate | logs only error reason; "Never log raw token" (`middleware.go:77-80`); audit logs action+resource only (`audit/middleware.go:96-100`) | closed |
| T-1-SC | Tampering (supply chain) | go get / go install | accept | Packages pinned in go.mod, verified via GOPROXY/GOSUM checksum DB | closed |
| T-1-audit-01 | Tampering | audit_log row mutation | mitigate | `REVOKE UPDATE, DELETE ... FROM donnarec_app` (`000002_audit_log.up.sql:95`); `TestAuditImmutability` | closed |
| T-1-audit-02 | Tampering | DBA-level row edit | mitigate | SHA-256 hash-chain `computeRowHash` (`audit/service.go:101-112`); `VerifyChain` returns brokenID; `TestHashChainVerification` | closed |
| T-1-audit-conc | Tampering | concurrent hash-chain link | mitigate | `pg_advisory_xact_lock` + `FOR UPDATE` (`audit/service.go:140`, `audit.sql:41`); `TestConcurrentAuditInserts` 50 goroutines, zero dup prev_hash | closed |
| T-1-audit-03 | Repudiation | missing audit on action | mitigate | generic `AuditMiddleware`, synchronous same-tx, no goroutine (`audit/middleware.go:41-105`); `TestAuditMiddlewareCoverage` | closed |
| T-1-audit-04 | Repudiation | PII reveal not logged | mitigate | `isPIIRevealEndpoint` forces audit on `/reveal` GET (`audit/middleware.go:113-116`); `TestPIIRevealAudit` | closed |
| T-1-audit-05 | Information disclosure | before/after JSON in DB audit | accept | Stored intentionally, Admin-only (D-16); zap logs never include payload | closed |
| T-1-pii-01 | Information disclosure | PII at rest | mitigate | AES-256-GCM envelope, fresh 32-byte DEK/call, DB stores ciphertext+wrappedDEK (`crypto/envelope.go:29-51`); `TestEnvelopeRoundTrip` | closed |
| T-1-pii-02 | Information disclosure | KEK leakage | mitigate | KEK from `DONAREC_KEK` env only, errors if absent/short (`crypto/envprovider.go:42-55`); `${DONAREC_KEK:?}` (`docker-compose.yml:110`); `TestEnvKeyProvider` | closed |
| T-1-pii-mask | Information disclosure | over-broad PII visibility | mitigate | mask last-4 default; `CanRevealFull` gates Admin/Checker (`pii/mask.go:59-127`); reveal audited via 01-02; `TestCanRevealFull` | closed |
| T-1-pii-03 | Tampering | ciphertext tamper | mitigate | GCM `Open` errors on auth-tag failure (`crypto/aes_gcm.go:65-69`); `TestAESGCMRoundTrip` tamper case | closed |
| T-1-retention-01 | Tampering/Destruction | hard-delete under legal_hold | mitigate | app `GuardHardDelete` (`retention/service.go:75-85`) + DB BEFORE DELETE trigger (`000003_retention_triggers.up.sql:23-36`); `TestLegalHoldDeleteBlocked` | closed |
| T-1-retention-02 | Repudiation | wrong/absent retain_until | accept | config-driven default, no hardcoded constants (`retention/service.go:44-57`); exact PDPA period pending DPO | closed |
| T-1-pii-04 | Information disclosure | blind index reverses PII | accept | HMAC-SHA256 with separate index key, one-way (`crypto/aes_gcm.go:91-95`); search wiring deferred to Phase 3 (D-14) | closed |

*Status: open · closed*
*Disposition: mitigate (implementation required) · accept (documented risk) · transfer (third-party)*

---

## Accepted Risks Log

| Risk ID | Threat Ref | Rationale | Accepted By | Date |
|---------|------------|-----------|-------------|------|
| AR-01 | T-1-SC | Go module supply chain bounded by GOSUM checksum DB; versions pinned in go.mod; no non-stdlib crypto/auth lib | gsd-security-auditor (plan-time disposition) | 2026-06-24 |
| AR-02 | T-1-audit-05 | before/after JSON in DB audit is intended & Admin-restricted (D-16); never emitted to application logs | gsd-security-auditor (plan-time disposition) | 2026-06-24 |
| AR-03 | T-1-retention-02 | retain_until config-driven; conservative defaults (1825/3650 days); exact PDPA period pending DPO (stakeholder gate, non-blocking) | gsd-security-auditor (plan-time disposition) | 2026-06-24 |
| AR-04 | T-1-pii-04 | Blind index HMAC-SHA256 one-way with separate key; full search usage deferred to Phase 3 | gsd-security-auditor (plan-time disposition) | 2026-06-24 |

*Accepted risks do not resurface in future audit runs.*

---

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open | Run By |
|------------|---------------|--------|------|--------|
| 2026-06-24 | 22 | 22 | 0 | gsd-security-auditor (sonnet) |

Notes:
- Register authored at plan time across 01-01/01-02/01-03; auditor verified each declared mitigation exists in code/SQL/config (not a retroactive scan).
- 01-04 (infra gap-closure) introduced no new threat register; its changes touch only T-1-config-01 (`start --import-realm`) and T-1-auth-02 (`failureFactor:5` replaced the unsupported `maxLoginFailures` key — semantically identical lockout mitigation) and T-1-pii-02 (`${DONAREC_KEK:?}` quoting).
- No unregistered threat surfaces — all four SUMMARY threat-surface scans reported clean.

---

## Sign-Off

- [x] All threats have a disposition (mitigate / accept / transfer)
- [x] Accepted risks documented in Accepted Risks Log
- [x] `threats_open: 0` confirmed
- [x] `status: verified` set in frontmatter

**Approval:** verified 2026-06-24
