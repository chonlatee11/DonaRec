-- migrations/000002_audit_log.down.sql
-- Reversal of 000002_audit_log.up.sql
--
-- DANGER (WR-06): This migration IRREVERSIBLY DESTROYS ALL AUDIT TRAIL DATA.
-- The audit_log table is append-only and legally significant (NFR-05 / FR-13).
-- This "down" must NEVER be run against a populated staging/production database.
-- It is intended ONLY for local development / disposable test databases.
--
-- Safety guard: this script aborts unless the session explicitly opts in by
-- setting `donnarec.allow_destructive_down = 'on'` for the connection, e.g.:
--     psql "$DATABASE_URL" -c "SET donnarec.allow_destructive_down = 'on';" -f 000002_audit_log.down.sql
-- (or via golang-migrate after issuing the SET on the same session).
-- Without that opt-in, the migration raises an exception and changes nothing.

DO $$
BEGIN
    IF current_setting('donnarec.allow_destructive_down', true) IS DISTINCT FROM 'on' THEN
        RAISE EXCEPTION
            'Refusing to run 000002 down: it destroys all audit trail data. '
            'Set donnarec.allow_destructive_down=''on'' on this session to confirm (dev/test only).';
    END IF;
END
$$;

-- NOTE: We deliberately DO NOT re-grant UPDATE/DELETE on audit_log before the
-- drop. Dropping a table does not require UPDATE/DELETE privileges on it, and
-- momentarily re-enabling them would open a tamper window on a populated table.
-- (The original migration re-granted them "to avoid dependency errors" — that is
-- unnecessary and unsafe; removed per WR-06.)

-- Drop indexes first (CASCADE on table drop handles this, but explicit is safer)
DROP INDEX IF EXISTS idx_audit_actor;
DROP INDEX IF EXISTS idx_audit_created;

-- Drop the audit_log table (CASCADE removes any dependent views/constraints)
DROP TABLE IF EXISTS audit_log;

-- Drop the app role if it has no other dependencies.
-- On a shared cluster donnarec_app may own/grant on other objects (users,
-- user_roles, sequences); DROP ROLE will then fail with a dependency error.
-- That failure is intentional — it signals the role is still in use and must
-- not be dropped here.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'donnarec_app') THEN
        DROP ROLE donnarec_app;
    END IF;
END
$$;
