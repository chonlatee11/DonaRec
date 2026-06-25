-- migrations/000004_receipt_number_tables.up.sql
-- Phase 2: Gap-less receipt number allocator tables (D-28..D-42, FR-15..FR-17, NFR-04)
--
-- Design decisions realized here:
--   D-30/D-31: receipt_number_config — configurable format stored in DB (no-deploy)
--   D-39:      receipt_number_counters — one row per fiscal year; SELECT FOR UPDATE path
--   D-37/D-42: receipt_numbers (ledger) — append-only; UNIQUE(fiscal_year, running_no) backstop
--
-- The application connects as role 'donnarec_app'.
-- This migration:
--   1. Creates receipt_number_config (single-row enforced) and seeds defaults (D-28)
--   2. Creates receipt_number_counters (counter table, one row per fiscal year)
--   3. Creates receipt_numbers ledger (append-only) with UNIQUE backstop
--   4. Grants minimal privileges to donnarec_app; REVOKEs UPDATE/DELETE on ledger
--
-- NEVER use SEQUENCE/SERIAL for running_no — BIGSERIAL is only the ledger surrogate PK.
-- The running_no column is a plain INT populated by the counter table (D-37, CLAUDE.md).

-- ============================================================
-- 1. receipt_number_config — format settings (D-30/D-31)
--    Single-row table: id BOOLEAN DEFAULT true + CHECK enforces max 1 row.
--    Phase 4 UI connects here to edit the single config row (no schema change needed).
-- ============================================================

CREATE TABLE receipt_number_config (
    id                  BOOLEAN     PRIMARY KEY DEFAULT true,
    CONSTRAINT          single_row CHECK (id = true),

    -- Number format components (D-28/D-29/D-30)
    separator           TEXT        NOT NULL DEFAULT '/',        -- D-28: "/"
    running_no_padding  INT         NOT NULL DEFAULT 6           -- D-29: minimum width (not hard cap)
                            CHECK (running_no_padding >= 1),
    year_format         TEXT        NOT NULL DEFAULT 'BE4'       -- 'BE4' = พ.ศ. 4 digits; 'CE4' = ค.ศ.
                            CHECK (year_format IN ('BE4', 'CE4')),  -- reject unknown formats; no silent fallback (IN-02)
    prefix              TEXT        NOT NULL DEFAULT '',         -- D-28: empty prefix

    -- Audit fields
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by          UUID        NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000'
);

-- Seed default config row (D-28: sep '/', pad 6, BE4 year format, empty prefix)
-- ON CONFLICT makes the seed idempotent — the single-row id=true PK means a
-- re-run conflicts on the existing row and is skipped instead of raising (WR-02).
INSERT INTO receipt_number_config DEFAULT VALUES
ON CONFLICT (id) DO NOTHING;

-- ============================================================
-- 2. receipt_number_counters — one row per fiscal year (D-39)
--    Holds last_running_no; incremented via SELECT FOR UPDATE + UPDATE in one tx.
--    Auto-reset: new fiscal year = new row created on first allocation (D-41).
-- ============================================================

CREATE TABLE receipt_number_counters (
    fiscal_year         INT         NOT NULL,
    last_running_no     INT         NOT NULL DEFAULT 0
                            CHECK (last_running_no >= 0),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT pk_receipt_number_counters PRIMARY KEY (fiscal_year)
);

-- ============================================================
-- 3. receipt_numbers — ledger + UNIQUE backstop (D-37, D-42)
--    Each allocated receipt number = one row (append-only at DB level via REVOKE).
--    BIGSERIAL id is surrogate PK for FK from Phase 3 receipts (D-38).
--    running_no is plain INT (counter table provides value) — NOT SERIAL/BIGSERIAL.
--    formatted is frozen snapshot at allocate time (D-42): config changes after
--    allocation must NOT alter the displayed number on previously issued receipts.
-- ============================================================

CREATE TABLE receipt_numbers (
    id              BIGSERIAL   PRIMARY KEY,
    fiscal_year     INT         NOT NULL,
    running_no      INT         NOT NULL
                        CHECK (running_no >= 1),          -- minimum 1; first allocation = 1
    formatted       TEXT        NOT NULL,                 -- frozen snapshot (D-42)
    allocated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- Backstop: DB-level guarantee — no duplicate numbers within a fiscal year (D-37, NFR-04)
    CONSTRAINT uq_receipt_numbers_fy_no UNIQUE (fiscal_year, running_no)
);

-- NOTE: no separate index on (fiscal_year, running_no) — the UNIQUE constraint
-- uq_receipt_numbers_fy_no already creates a B-tree index covering Phase 3 lookups (WR-01).

-- Index for lookup / search by formatted receipt number (Phase 3/5)
CREATE INDEX idx_receipt_numbers_formatted  ON receipt_numbers (formatted);

-- ============================================================
-- 4. Permissions for donnarec_app role
-- ============================================================

-- Config: app may read and update the single config row (Phase 4 UI writes via app role)
GRANT SELECT, INSERT, UPDATE ON receipt_number_config    TO donnarec_app;

-- Counters: app may read (SELECT FOR UPDATE) and update (INCREMENT) counter rows
GRANT SELECT, INSERT, UPDATE ON receipt_number_counters  TO donnarec_app;

-- Ledger: app may SELECT and INSERT only — ledger is append-only (D-42)
GRANT SELECT, INSERT         ON receipt_numbers          TO donnarec_app;

-- Immutable ledger enforcement (T-02-01): no UPDATE or DELETE allowed at DB level.
-- NOTE: this REVOKE is not permanent armor — a later explicit GRANT UPDATE/DELETE
-- (or GRANT ALL) would restore the privilege. It is defense-in-depth against the
-- app role's normal grants, not a substitute for not issuing such a grant (WR-03).
REVOKE UPDATE, DELETE        ON receipt_numbers          FROM donnarec_app;

-- Sequence for surrogate PK — app needs USAGE + SELECT for nextval/currval
GRANT USAGE, SELECT ON SEQUENCE receipt_numbers_id_seq  TO donnarec_app;
