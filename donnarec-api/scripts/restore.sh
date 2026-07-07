#!/bin/sh
# scripts/restore.sh — Disaster-recovery restore procedure (NFR-08, D-72, D-73)
#
# NOT run automatically — invoked manually during disaster recovery, inside (or with
# access to) the `backup` companion container's tooling (pg_restore + mc). See
# docs/BACKUP_RESTORE_RUNBOOK.md for the full step-by-step procedure and the recorded
# restore-proof evidence (internal/backupverify/restore_test.go, D-73).
#
# Usage:
#   restore.sh <path-to-dump-file>
#
# Prerequisite: the target Postgres cluster must already have the `donnarec_app` role
# provisioned (created by migrations/000002_audit_log.up.sql) BEFORE running this —
# pg_restore --role=donnarec_app requires that role to already exist (pg_dump captures
# GRANT/REVOKE statements referencing the role, but never a CREATE ROLE statement,
# since roles are cluster-level objects outside any single database dump). If restoring
# into a genuinely empty/roleless cluster, drop --role=donnarec_app and use
# --no-privileges instead — ACLs are then reapplied by re-running migrations normally.
set -eu

DUMP_FILE="${1:?usage: restore.sh <path-to-dump-file>}"

DB_HOST="${DB_HOST:-postgres}"
DB_PORT="${DB_PORT:-5432}"
DB_USER="${DB_USER:-donnarec}"
DB_NAME="${DB_NAME:-donnarec_app}"
MINIO_ENDPOINT="${MINIO_ENDPOINT:-minio:9000}"
MINIO_ACCESS_KEY="${MINIO_ACCESS_KEY:-minioadmin}"
MINIO_SECRET_KEY="${MINIO_SECRET_KEY:-minioadmin}"
MINIO_BUCKET="${MINIO_BUCKET:-donnarec-slips}"
MINIO_RECEIPTS_BUCKET="${MINIO_RECEIPTS_BUCKET:-donnarec-receipts}"

echo "[restore] $(date -Iseconds) restoring ${DUMP_FILE} into ${DB_NAME}"
# NOTE: if pg_restore fails with "input file does not appear to be a valid archive" —
# the dump was NOT taken with -Fc (Pitfall 4). Re-check scripts/backup.sh.
PGPASSWORD="${DB_PASSWORD:?DB_PASSWORD is required}" pg_restore --no-owner --role=donnarec_app \
  -h "${DB_HOST}" -p "${DB_PORT}" \
  -U "${DB_USER}" -d "${DB_NAME}" \
  "${DUMP_FILE}"
echo "[restore] pg_restore complete"

echo "[restore] configuring mc alias 'donnarec' -> http://${MINIO_ENDPOINT}"
mc alias set donnarec "http://${MINIO_ENDPOINT}" "${MINIO_ACCESS_KEY}" "${MINIO_SECRET_KEY}" >/dev/null

echo "[restore] mirroring /backups/minio/slips -> bucket '${MINIO_BUCKET}'"
mc mirror --overwrite /backups/minio/slips "donnarec/${MINIO_BUCKET}"

echo "[restore] mirroring /backups/minio/receipts -> bucket '${MINIO_RECEIPTS_BUCKET}'"
mc mirror --overwrite /backups/minio/receipts "donnarec/${MINIO_RECEIPTS_BUCKET}"

echo "[restore] $(date -Iseconds) done — verify row counts / object listings before resuming traffic"
