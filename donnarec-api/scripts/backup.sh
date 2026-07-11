#!/bin/sh
# scripts/backup.sh — Postgres dump + MinIO mirror + retention sweep (NFR-08, D-72, D-73)
#
# Runs inside the `backup` companion container (docker/backup.Dockerfile,
# docker-compose.yml), scheduled by cron via $BACKUP_CRON. Produces:
#   1. A pg_dump -Fc (custom format — Pitfall 4: NEVER plain/default format, which
#      cannot be pg_restore'd) snapshot of the Postgres database.
#   2. A live mirror (via `mc mirror`) of BOTH MinIO buckets — donation slips AND
#      frozen receipt PDFs (D-72: a DB-only backup is not enough; object storage
#      cannot be recovered from the DB alone).
#   3. A retention sweep that deletes pg_dump artifacts older than
#      $BACKUP_RETENTION_DAYS.
#
# All output lives under /backups (a named Docker volume — NEVER a bind mount into the
# repo; dump artifacts contain ciphertext + KEK-adjacent data, T-05-03-DUMPLEAK).
#
# Required environment (see docker-compose.yml `backup` service + .env.example):
#   DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME       — Postgres connection
#   MINIO_ENDPOINT, MINIO_ACCESS_KEY, MINIO_SECRET_KEY    — MinIO connection
#   MINIO_BUCKET, MINIO_RECEIPTS_BUCKET                   — buckets to mirror (D-72)
#   BACKUP_RETENTION_DAYS                                  — days to keep dump artifacts
#
# See docs/BACKUP_RESTORE_RUNBOOK.md for the restore procedure (scripts/restore.sh) and
# the recorded restore-proof evidence (internal/backupverify/restore_test.go, D-73).
set -eu

DB_HOST="${DB_HOST:-postgres}"
DB_PORT="${DB_PORT:-5432}"
DB_USER="${DB_USER:-donnarec}"
DB_NAME="${DB_NAME:-donnarec_app}"
MINIO_ENDPOINT="${MINIO_ENDPOINT:-minio:9000}"
MINIO_ACCESS_KEY="${MINIO_ACCESS_KEY:-minioadmin}"
MINIO_SECRET_KEY="${MINIO_SECRET_KEY:-minioadmin}"
MINIO_BUCKET="${MINIO_BUCKET:-donnarec-slips}"
MINIO_RECEIPTS_BUCKET="${MINIO_RECEIPTS_BUCKET:-donnarec-receipts}"
BACKUP_RETENTION_DAYS="${BACKUP_RETENTION_DAYS:-14}"

TIMESTAMP=$(date +%Y%m%d_%H%M%S)
DUMP_FILE="/backups/donnarec_${TIMESTAMP}.dump"

echo "[backup] $(date -Iseconds) starting pg_dump -Fc -> ${DUMP_FILE}"
PGPASSWORD="${DB_PASSWORD:?DB_PASSWORD is required}" pg_dump -Fc \
  -h "${DB_HOST}" -p "${DB_PORT}" \
  -U "${DB_USER}" -d "${DB_NAME}" \
  -f "${DUMP_FILE}"
echo "[backup] pg_dump complete: $(du -h "${DUMP_FILE}" | cut -f1)"

echo "[backup] configuring mc alias 'donnarec' -> http://${MINIO_ENDPOINT}"
mc alias set donnarec "http://${MINIO_ENDPOINT}" "${MINIO_ACCESS_KEY}" "${MINIO_SECRET_KEY}" >/dev/null

mkdir -p /backups/minio/slips /backups/minio/receipts

echo "[backup] mirroring bucket '${MINIO_BUCKET}' -> /backups/minio/slips"
mc mirror --overwrite "donnarec/${MINIO_BUCKET}" /backups/minio/slips

echo "[backup] mirroring bucket '${MINIO_RECEIPTS_BUCKET}' -> /backups/minio/receipts"
mc mirror --overwrite "donnarec/${MINIO_RECEIPTS_BUCKET}" /backups/minio/receipts

echo "[backup] applying retention: deleting dump artifacts older than ${BACKUP_RETENTION_DAYS} days"
find /backups -maxdepth 1 -name 'donnarec_*.dump' -mtime "+${BACKUP_RETENTION_DAYS}" -print -delete

echo "[backup] $(date -Iseconds) done"
