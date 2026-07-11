-- migrations/000013_edonation_keyed_metadata.down.sql
-- Reversal of 000013_edonation_keyed_metadata.up.sql
--
-- Drops the "who/when marked" metadata columns only. donations.edonation_keyed
-- itself is untouched — it was added in 000005 and is owned by that migration.

ALTER TABLE donations
    DROP COLUMN IF EXISTS edonation_keyed_at,
    DROP COLUMN IF EXISTS edonation_keyed_by;
