# docker/backup.Dockerfile — Backup companion service (NFR-08, D-72/D-73, Plan 05-03)
#
# Base image is postgres:17 — the SAME major version as the `postgres` service in
# docker-compose.yml — so pg_dump/pg_restore ship the exact matching client version as
# the server (a version-mismatched pg_dump can silently omit newer object types).
#
# mc (MinIO client) is copied from the official minio/mc image via a multi-stage build
# rather than curl-downloaded at build time — no network egress needed during the
# Docker build itself. This mirrors docker/chrome.Dockerfile's precedent: build a small
# custom image from an official base rather than adopt a third-party ops image
# (05-RESEARCH.md Pattern 8, "Don't Hand-Roll: use `mc mirror`, not a custom Go loop").
FROM minio/mc:latest AS mc

FROM postgres:17

# cron: runs scripts/backup.sh on the schedule given by $BACKUP_CRON (docker-compose.yml
# `backup` service env, .env.example). pg_dump/pg_restore are already on PATH via
# /usr/bin (symlinked by the base postgres:17 image itself).
RUN apt-get update && apt-get install -y --no-install-recommends cron \
    && rm -rf /var/lib/apt/lists/*

COPY --from=mc /usr/bin/mc /usr/local/bin/mc

COPY scripts/backup.sh  /usr/local/bin/backup.sh
COPY scripts/restore.sh /usr/local/bin/restore.sh
RUN chmod +x /usr/local/bin/backup.sh /usr/local/bin/restore.sh

# Render the cron.d entry from $BACKUP_CRON at container start (env vars are not
# available to cron.d files written at image BUILD time), then run cron in the
# foreground as PID 1. Job output is redirected to the container's own stdout/stderr
# (/proc/1/fd/1|2) so `docker compose logs backup` shows real backup.sh output instead
# of cron's default silent mailbox behaviour.
CMD ["/bin/sh", "-c", "printf '%s root /usr/local/bin/backup.sh >> /proc/1/fd/1 2>> /proc/1/fd/2\\n' \"${BACKUP_CRON:-0 2 * * *}\" > /etc/cron.d/donnarec-backup && chmod 0644 /etc/cron.d/donnarec-backup && echo \"[backup] cron schedule: ${BACKUP_CRON:-0 2 * * *}\" && cron -f"]
