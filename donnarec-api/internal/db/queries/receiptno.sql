-- internal/db/queries/receiptno.sql
-- sqlc queries for Phase 2: gap-less receipt number allocator
-- All queries use named @params and explicit column lists (no SELECT * in writes).
-- Parameterized only — no string concatenation (T-02-03 mitigation).
--
-- Query naming follows Path A: SELECT FOR UPDATE + UPDATE RETURNING (D-36, 02-RESEARCH Q1)
-- Anti-patterns explicitly absent (T-02-03/T-02-04):
--   - NO MAX(running_no) — race-prone "read max+1" pattern
--   - NO nextval() / SEQUENCE for running_no — not rollback-safe (CLAUDE.md What NOT to Use)

-- name: LockCounterForUpdate :one
-- Lock the counter row for the given fiscal year using SELECT FOR UPDATE.
-- This serializes concurrent allocations: only one transaction may increment
-- last_running_no at a time; all others block until the lock holder commits/rolls back.
--
-- Returns pgx.ErrNoRows if no counter row exists yet (first allocation of a new fiscal year).
-- Caller (allocator.go) must handle ErrNoRows by calling InitCounterRow first (Pitfall 1).
SELECT last_running_no
FROM receipt_number_counters
WHERE fiscal_year = @fiscal_year
FOR UPDATE;

-- name: InitCounterRow :exec
-- Create a counter row for a new fiscal year if one does not exist yet.
-- ON CONFLICT (fiscal_year) DO NOTHING: safe under concurrent first-allocation of the same
-- fiscal year — both sessions attempt the INSERT; one wins, one silently skips.
-- The losing session then proceeds to LockCounterForUpdate (row now exists) and blocks
-- until the winning session commits, ensuring correct serialization (D-41, Pitfall 1).
INSERT INTO receipt_number_counters (fiscal_year, last_running_no)
VALUES (@fiscal_year, 0)
ON CONFLICT (fiscal_year) DO NOTHING;

-- name: IncrementCounter :one
-- Increment last_running_no by 1 and return the new value.
-- MUST be called only while holding the FOR UPDATE lock from LockCounterForUpdate.
-- updated_at is refreshed here so the counter row has an accurate last-modified timestamp.
UPDATE receipt_number_counters
SET
    last_running_no = last_running_no + 1,
    updated_at      = now()
WHERE fiscal_year = @fiscal_year
RETURNING last_running_no;

-- name: GetReceiptNumberConfig :one
-- Read the number format config row (called within the same allocation transaction — D-32).
-- Reading inside the tx ensures the allocator sees config consistent with its own snapshot;
-- the formatted snapshot is frozen in the ledger at this moment (D-42).
SELECT separator, running_no_padding, year_format, prefix
FROM receipt_number_config
LIMIT 1;

-- name: UpdateReceiptNumberConfig :exec
-- Update the single number-format config row (Admin-only, Phase 4 D-58 settings
-- UI — CONTEXT.md canonical_refs note: consolidate number-format editing into
-- the same Admin settings screen as the template config, rather than a
-- separate page). Frozen ledger entries (D-42) are NEVER affected by this —
-- only the NEXT allocation picks up the new format. Callers MUST validate
-- separator/prefix against the same safe-character allowlist
-- receiptno.formatReceiptNo enforces at allocation time (mirrored in
-- internal/settings/service.go) BEFORE calling this, so a bad save cannot
-- silently corrupt the next issuance instead of failing fast at save-time.
UPDATE receipt_number_config
SET
    separator          = @separator,
    running_no_padding = @running_no_padding,
    year_format        = @year_format,
    prefix             = @prefix,
    updated_at         = now(),
    updated_by         = @updated_by
WHERE id = true;

-- name: InsertReceiptNumberLedger :one
-- Record the allocated receipt number in the append-only ledger (D-37, D-42).
-- allocated_at uses DB-side now() for clock consistency across application instances.
-- The UNIQUE(fiscal_year, running_no) backstop fires here if a logic bug produces a
-- duplicate — the constraint is the last line of defense independent of app logic (D-37).
INSERT INTO receipt_numbers (fiscal_year, running_no, formatted, allocated_at)
VALUES (@fiscal_year, @running_no, @formatted, now())
RETURNING id, fiscal_year, running_no, formatted, allocated_at;
