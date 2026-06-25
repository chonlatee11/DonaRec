-- migrations/000001_init_schema.up.sql
-- Phase 1 foundation tables (ordered by dependency)
-- NOTE: audit_log table is owned by plan 01-02 (migration 000002)
--       retention triggers are owned by plan 01-03 (migration 000003)
--       Do NOT create them here.

-- ============================================================
-- 1. Enums (must be created before tables that reference them)
-- ============================================================

CREATE TYPE legal_basis_enum AS ENUM (
    'tax_obligation',
    'consent',
    'legitimate_interest'
);

CREATE TYPE user_role_enum AS ENUM ('maker', 'checker', 'admin');

-- ============================================================
-- 2. Core tables
-- ============================================================

CREATE TABLE users (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    email             TEXT        NOT NULL UNIQUE,
    display_name      TEXT        NOT NULL,
    keycloak_subject  TEXT        NOT NULL UNIQUE,  -- Keycloak 'sub' JWT claim
    is_active         BOOLEAN     NOT NULL DEFAULT true,
    -- Retention / PDPA fields (D-18)
    -- legal_hold: when true, no code path may hard-delete this row
    legal_hold        BOOLEAN     NOT NULL DEFAULT false,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- user_roles: junction table enabling multi-role per user (D-02)
-- PRIMARY KEY (user_id, role) enforces uniqueness while allowing multiple roles per user
CREATE TABLE user_roles (
    user_id  UUID           NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role     user_role_enum NOT NULL,
    PRIMARY KEY (user_id, role)
);

-- retention_config: configurable retention policies (D-18, NFR-03)
-- updated_by is intentionally NOT a FK to users(id) (IN-02): this table is
-- seeded by THIS migration, which runs before any user (and before seed-admin.sh)
-- exists, so a FK would make the seed impossible at migration time. Instead we
-- seed updated_by with the all-zero sentinel UUID below to mean "seeded by
-- migration, not yet attributed to a real admin". seed-admin.sh / admin UI may
-- later overwrite updated_by with a real users.id. The sentinel is an accepted,
-- documented design choice, not an integrity bug.
CREATE TABLE retention_config (
    id                  SERIAL          PRIMARY KEY,
    entity_type         TEXT            NOT NULL UNIQUE,
    default_retain_days INT             NOT NULL,
    legal_basis         legal_basis_enum NOT NULL,
    updated_at          TIMESTAMPTZ     NOT NULL DEFAULT now(),
    -- updated_by: see note above — sentinel zero-UUID until an admin updates it.
    updated_by          UUID            NOT NULL
);

-- ============================================================
-- 3. Indexes
-- ============================================================

CREATE INDEX idx_users_email       ON users(email);
CREATE INDEX idx_users_keycloak    ON users(keycloak_subject);

-- ============================================================
-- 4. Seed retention_config defaults
-- NOTE: updated_by uses the all-zero UUID as a documented sentinel meaning
--       "seeded by migration" (IN-02). There is intentionally no FK to users(id)
--       because no user exists at migration time. seed-admin.sh / admin UI may
--       overwrite updated_by with a real users.id later.
-- ============================================================

INSERT INTO retention_config (entity_type, default_retain_days, legal_basis, updated_by)
VALUES
    ('donation', 1825, 'tax_obligation',    '00000000-0000-0000-0000-000000000000'),
    ('audit_log', 3650, 'tax_obligation',   '00000000-0000-0000-0000-000000000000');
