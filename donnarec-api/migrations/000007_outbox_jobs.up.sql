-- migrations/000007_outbox_jobs.up.sql
-- Phase 3: Transactional outbox for async PDF+email delivery
--
-- Design decisions realized here:
--   DB-backed outbox (no Redis dependency) — durability without additional infrastructure
--   INSERT in issuance tx → job exists IFF receipt was issued (atomic linkage)
--   Worker (Phase 4) polls pending/failed rows and processes them outside the lock path
--   Keeping render+email out of issuance tx satisfies NFR-07 latency requirement
--
-- The application connects as role 'donnarec_app'.
-- This migration:
--   1. Creates outbox_jobs table with status CHECK constraint
--   2. Creates partial index for efficient pending-job polling (Phase 4 worker)
--   3. Grants SELECT, INSERT, UPDATE (Phase 4 worker updates status/attempts/last_error)
--   4. Grants USAGE, SELECT on SEQUENCE for BIGSERIAL id

-- ============================================================
-- 1. Outbox jobs table (transactional outbox pattern, CLAUDE.md §"Email Delivery")
-- ============================================================

CREATE TABLE outbox_jobs (
    id          BIGSERIAL   PRIMARY KEY,
    job_type    TEXT        NOT NULL,   -- e.g. 'issue_receipt'
    payload     JSONB       NOT NULL,   -- e.g. {"donation_id": "uuid"}
    status      TEXT        NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending','processing','done','failed')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    attempts    INT         NOT NULL DEFAULT 0,
    last_error  TEXT
);

-- ============================================================
-- 2. Partial index for efficient pending-job polling (Phase 4 worker)
--    Only pending and failed rows need polling; done/processing excluded.
-- ============================================================

CREATE INDEX idx_outbox_jobs_pending ON outbox_jobs (status, created_at)
    WHERE status IN ('pending', 'failed');

-- ============================================================
-- 3. Permissions for donnarec_app role
-- ============================================================

-- Phase 3: INSERT (enqueue in issuance tx)
-- Phase 4: SELECT + UPDATE (worker polls and updates status/attempts/last_error)
GRANT SELECT, INSERT, UPDATE ON outbox_jobs TO donnarec_app;

-- Sequence for BIGSERIAL id — app needs USAGE + SELECT for nextval/currval
GRANT USAGE, SELECT ON SEQUENCE outbox_jobs_id_seq TO donnarec_app;
