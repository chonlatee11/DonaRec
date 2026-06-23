---
phase: 1
slug: foundation-db-auth-rbac-audit-retention-model
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-06-23
---

# Phase 1 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | `go test` (standard library + testify) + testcontainers-go for PostgreSQL/Keycloak integration |
| **Config file** | none — Wave 0 installs (go.mod test deps + docker-compose for local) |
| **Quick run command** | `go test ./... -short` |
| **Full suite command** | `go test ./... -count=1` (includes integration tests against testcontainers) |
| **Estimated runtime** | ~60 seconds (quick ~10s; full with containers ~60s) |

---

## Sampling Rate

- **After every task commit:** Run `go test ./... -short`
- **After every plan wave:** Run `go test ./... -count=1`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 60 seconds

---

## Per-Task Verification Map

> Populated by the planner against the final task IDs. Each Phase 1 success criterion maps to at least one automated test below.

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| TBD | TBD | TBD | NFR-01 | T-1-auth | argon2id/Keycloak password not stored plaintext; login issues valid OIDC session | integration | `go test ./internal/auth/... -run TestLogin` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | FR-34 | T-1-rbac | server-side RBAC rejects unpermitted role; multi-role honored; admin assigns roles | integration | `go test ./internal/authz/... -run TestRBAC` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | NFR-02 | T-1-pii | sensitive-ID column stored AES-256-GCM ciphertext, never plaintext; TLS served | unit+integration | `go test ./internal/crypto/... ./internal/transport/...` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | NFR-05 / FR-13 | T-1-audit | every mutation + auth event writes immutable audit row; hash-chain verifies; UPDATE/DELETE denied at DB | integration | `go test ./internal/audit/... -run 'TestAppendOnly|TestHashChain|TestDBDenyMutation'` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | NFR-05 | T-1-audit-conc | concurrent audit inserts keep hash-chain intact (advisory lock / FOR UPDATE) | integration | `go test ./internal/audit/... -run TestConcurrentAppend -race` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | NFR-03 | T-1-retention | retain_until + legal_basis + legal_hold present; hard-delete denied under legal_hold (app + DB) | integration | `go test ./internal/retention/... -run 'TestLegalHold|TestRetainUntil'` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | NFR-02 | T-1-pii-mask | PII masked by default (last-4); reveal is just-in-time AND writes an audit row | integration | `go test ./internal/pii/... -run 'TestMaskDefault|TestRevealAudited'` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `go.mod` test dependencies installed (testify, testcontainers-go)
- [ ] `docker-compose.yml` with PostgreSQL + Keycloak for local/integration tests
- [ ] Shared test helpers: DB migration runner + truncate-between-tests, Keycloak realm import fixture
- [ ] Test DB seeding for users/roles (Maker/Checker/Admin)

*Greenfield project — Wave 0 establishes all test infrastructure.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| TLS/HTTPS served end-to-end in deployed env | NFR-02 | Local dev may use plain HTTP via proxy; cert termination is deployment-time | Deploy via docker-compose, `curl -vkI https://localhost` confirms TLS handshake |
| Keycloak realm password policy / lockout values | NFR-01 | Realm config is data, not code; confirm policy applied in admin console | Import realm, verify password policy ≥8 alnum + lockout=N in Keycloak admin |

*Remaining behaviors have automated verification.*

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 60s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
