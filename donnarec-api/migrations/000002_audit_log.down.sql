-- migrations/000002_audit_log.down.sql
-- Reversal of 000002_audit_log.up.sql
-- Drops the audit_log table and revokes/drops the app role grants.
-- WARNING: This irreversibly destroys all audit trail data.

-- Restore UPDATE, DELETE to donnarec_app before drop (avoids dependency errors)
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'donnarec_app') THEN
        GRANT UPDATE, DELETE ON audit_log TO donnarec_app;
    END IF;
END
$$;

-- Drop indexes first (CASCADE on table drop handles this, but explicit is safer)
DROP INDEX IF EXISTS idx_audit_actor;
DROP INDEX IF EXISTS idx_audit_created;

-- Drop the audit_log table (CASCADE removes any dependent views/constraints)
DROP TABLE IF EXISTS audit_log;

-- Drop the app role if it has no other dependencies
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'donnarec_app') THEN
        DROP ROLE donnarec_app;
    END IF;
END
$$;
