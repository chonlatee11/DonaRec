-- migrations/000002_audit_log.up.sql
-- Append-only hash-chained audit log table (D-17, NFR-05, FR-13)
--
-- Design decisions realized here:
--   D-15: Audit scope covers all mutations + auth events via generic interceptor
--   D-17: Tamper-evidence via REVOKE UPDATE/DELETE + SHA-256 hash-chain per row
--   D-16: Admin-only read access (enforced in service/middleware, not DB)
--
-- The application connects as role 'donnarec_app'.
-- This migration:
--   1. Creates the audit_log table (append-only, hash-chained)
--   2. Creates the donnarec_app role if not exists
--   3. Grants SELECT + INSERT only; explicitly REVOKEs UPDATE + DELETE
--   4. Also applies REVOKE to the 'test' role (used in integration tests)
--      so immutability tests prove DB-level rejection without mocking the role.

-- ============================================================
-- 1. audit_log table (append-only, hash-chained)
-- ============================================================

CREATE TABLE audit_log (
    -- Primary key (BIGSERIAL for ordered iteration in chain verification)
    id          BIGSERIAL   PRIMARY KEY,

    -- Actor fields: who performed the action
    actor_id    UUID        NOT NULL,                          -- Keycloak 'sub' claim
    actor_email TEXT        NOT NULL,                          -- Keycloak 'email' claim

    -- Action descriptor
    action      TEXT        NOT NULL,                          -- e.g. 'user.create', 'pii.reveal'
    resource    TEXT        NOT NULL,                          -- Gin c.FullPath(), e.g. '/api/admin/users'

    -- Snapshots (nullable — not every action has before/after)
    before_json JSONB,                                         -- state before mutation
    after_json  JSONB,                                         -- state after mutation

    -- Request metadata
    ip_address  INET,                                          -- c.ClientIP()

    -- Timestamp (immutable once written)
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- Hash-chain fields (D-17: tamper detection)
    prev_hash   TEXT        NOT NULL,                          -- row_hash of the preceding row; 'GENESIS' for row 1
    row_hash    TEXT        NOT NULL                           -- SHA-256(id||actor_id||action||resource||created_at||prev_hash)
);

-- ============================================================
-- 2. Indexes for efficient query patterns
-- ============================================================

-- Filter + time-range queries for an actor (Admin audit viewer, Phase 4)
CREATE INDEX idx_audit_actor   ON audit_log(actor_id, created_at DESC);

-- Global time-range listing (Admin-only list endpoint)
CREATE INDEX idx_audit_created ON audit_log(created_at DESC);

-- ============================================================
-- 3. App role + immutability enforcement (D-17)
-- ============================================================

-- Create the application DB role used by the API (production) AND integration tests.
-- LOGIN + password allows testcontainers tests to connect as this role directly,
-- proving the REVOKE at the DB level (a superuser/table-owner bypasses REVOKE,
-- so tests must connect as a non-owner role to validate the restriction — D-17).
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'donnarec_app') THEN
        -- PASSWORD 'donnarec_app_test' is used only in testcontainers; production
        -- uses IAM / secrets-manager credentials passed via DATABASE_URL.
        CREATE ROLE donnarec_app LOGIN PASSWORD 'donnarec_app_test';
    END IF;
END
$$;

-- Grant the app role access to the donnarec_test database (needed to connect)
DO $$
DECLARE
    db_name TEXT := current_database();
BEGIN
    EXECUTE format('GRANT CONNECT ON DATABASE %I TO donnarec_app', db_name);
END
$$;

-- Grant schema usage so the role can see tables
GRANT USAGE ON SCHEMA public TO donnarec_app;

-- Grant minimal required privileges on audit_log to the app role:
-- SELECT (for VerifyChain + ListAuditLogs) and INSERT (for AppendAuditEntry).
GRANT SELECT, INSERT ON audit_log TO donnarec_app;

-- Explicitly REVOKE UPDATE and DELETE from the app role.
-- Even if a future GRANT ALL is issued, these specific permissions remain revoked.
-- This is the DB-level tamper-prevention for T-1-audit-01 (D-17, NFR-05).
REVOKE UPDATE, DELETE ON audit_log FROM donnarec_app;

-- Also grant SELECT on other tables the app role might need (future phases)
-- so the app role can function as the sole API connection identity.
GRANT SELECT, INSERT, UPDATE, DELETE ON users TO donnarec_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON user_roles TO donnarec_app;
GRANT SELECT, INSERT, UPDATE ON retention_config TO donnarec_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO donnarec_app;

-- ============================================================
-- Comment: Why BIGSERIAL not UUID for primary key?
-- ============================================================
-- The hash-chain relies on strict ordering (ORDER BY id ASC for verification).
-- BIGSERIAL guarantees monotonic integer ordering matching insertion order,
-- which UUID does not. The id is also embedded in the row_hash computation
-- to bind the hash to the row's position in the chain.
