-- migrations/000003_retention_triggers.down.sql
-- Reverses 000003_retention_triggers.up.sql
--
-- Drop order: triggers first (they depend on the function), then the function.

-- ============================================================
-- 1. Drop triggers from `users` table
-- ============================================================

DROP TRIGGER IF EXISTS trg_prevent_legal_hold_delete_users ON users;

-- ============================================================
-- 2. Drop the trigger function
-- ============================================================

DROP FUNCTION IF EXISTS prevent_legal_hold_delete();
