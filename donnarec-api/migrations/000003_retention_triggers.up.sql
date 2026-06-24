-- migrations/000003_retention_triggers.up.sql
-- Retention: prevent_legal_hold_delete trigger function + per-table attachment
--
-- Design (D-19, NFR-03):
--   This migration adds a database-level BEFORE DELETE trigger as a defense-in-depth
--   backstop against hard-deleting records under legal hold. The application-level
--   GuardHardDelete() check (internal/retention/service.go) is the primary guard;
--   this trigger is the secondary safety net that catches direct SQL bypass.
--
--   Trust boundary: even if a bug allows a DELETE to reach the DB, the trigger
--   blocks it before any row is removed.
--
-- Trigger attachment strategy:
--   Phase 1: attached to `users` (the only legal_hold-bearing table this phase).
--   Phase 3: when donor/donation tables are added, the same trigger function is
--             reused by attaching it to those tables (one CREATE TRIGGER per table).
--   No modification to this function is needed for Phase 3 extension.

-- ============================================================
-- 1. prevent_legal_hold_delete trigger function
-- ============================================================

CREATE OR REPLACE FUNCTION prevent_legal_hold_delete()
RETURNS TRIGGER AS $$
BEGIN
    -- Block hard DELETE when legal_hold = true.
    -- This guard operates at the row level (BEFORE DELETE per-row trigger).
    -- Soft delete (UPDATE is_active=false) is NOT blocked — only hard DELETE.
    IF OLD.legal_hold = true THEN
        RAISE EXCEPTION 'cannot delete record under legal hold (id=%, table=%)',
            OLD.id, TG_TABLE_NAME
            USING ERRCODE = 'P0001';  -- RAISE_EXCEPTION
    END IF;
    RETURN OLD;  -- return OLD to allow the DELETE to proceed
END;
$$ LANGUAGE plpgsql;

-- ============================================================
-- 2. Attach trigger to `users` table (Phase 1 scope)
-- ============================================================

-- BEFORE DELETE: fires before the row is removed, allowing us to abort.
-- FOR EACH ROW: fires once per deleted row (not once per statement).
CREATE TRIGGER trg_prevent_legal_hold_delete_users
    BEFORE DELETE ON users
    FOR EACH ROW
    EXECUTE FUNCTION prevent_legal_hold_delete();

-- NOTE: Phase 3 extends this by adding:
--   CREATE TRIGGER trg_prevent_legal_hold_delete_donors
--       BEFORE DELETE ON donors FOR EACH ROW EXECUTE FUNCTION prevent_legal_hold_delete();
--   CREATE TRIGGER trg_prevent_legal_hold_delete_donations
--       BEFORE DELETE ON donations FOR EACH ROW EXECUTE FUNCTION prevent_legal_hold_delete();
-- The function itself does not need modification — it reads OLD.legal_hold generically.
