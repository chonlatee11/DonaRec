-- internal/db/queries/reports.sql
-- sqlc queries for Phase 5: no-PII aggregate reports (FR-32, D-70, D-71).
-- Report period basis is donated_at (วันที่บริจาค — A1 default assumption,
-- 05-RESEARCH.md Assumptions Log); the date-range filter also keys off
-- donated_at. No PII columns are selected on this path — no decrypt/mask
-- step needed anywhere here, matching donated_at's column type (DATE,
-- migration 000005), so no timezone conversion is needed.

-- name: SummaryByMonth :many
-- Aggregates issued donations by calendar month. Excludes non-issued statuses
-- — cancelled/draft/rejected are not "donations received" (D-70 assumption).
SELECT
    date_trunc('month', donated_at)::date AS period,
    COUNT(*)    AS receipt_count,
    SUM(amount) AS total_amount
FROM donations
WHERE status = 'issued'
  AND (sqlc.narg('from_date')::DATE IS NULL OR donated_at >= sqlc.narg('from_date'))
  AND (sqlc.narg('to_date')::DATE   IS NULL OR donated_at <= sqlc.narg('to_date'))
GROUP BY period
ORDER BY period;

-- name: SummaryByDay :many
-- Same shape, daily granularity — donated_at is already a DATE so no
-- truncation is needed.
SELECT
    donated_at  AS period,
    COUNT(*)    AS receipt_count,
    SUM(amount) AS total_amount
FROM donations
WHERE status = 'issued'
  AND (sqlc.narg('from_date')::DATE IS NULL OR donated_at >= sqlc.narg('from_date'))
  AND (sqlc.narg('to_date')::DATE   IS NULL OR donated_at <= sqlc.narg('to_date'))
GROUP BY donated_at
ORDER BY donated_at;
