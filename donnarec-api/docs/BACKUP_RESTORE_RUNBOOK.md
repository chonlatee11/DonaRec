# Backup & Restore Runbook

**Scope:** NFR-08 (backups) / D-72 (what is backed up) / D-73 (proof of restore).
**Owner:** hospital IT / on-call admin.
**Last verified:** 2026-07-07 (see [Verification Evidence](#verification-evidence)).

---

## 1. What is backed up (D-72)

Two independent things, both required — **the database alone is not enough**:

| Component | Tool | Format | Why |
|---|---|---|---|
| PostgreSQL (`donnarec_app`) | `pg_dump` | **`-Fc` custom format** | Contains donations, users, audit log, receipt numbers, settings — the transactional record of everything. |
| MinIO — `donnarec-slips` bucket | `mc mirror` | live object mirror | Uploaded donation slip images. **Not stored in Postgres** — a DB-only backup loses every slip. |
| MinIO — `donnarec-receipts` bucket | `mc mirror` | live object mirror | Frozen, already-issued receipt PDFs (the legally significant artifact handed to donors). **Also not stored in Postgres.** |

Both MinIO buckets are mirrored on **every** backup run — object storage cannot be
reconstructed from the database, and the database's `donations` table only records
*metadata* about a receipt (`receipt_formatted`, `receipt_number_id`), not the
rendered PDF bytes themselves.

**Never plain-format `pg_dump`.** The default (plain SQL) format cannot be
`pg_restore`'d and is far more fragile to reconstruct from than a custom-format
archive (Pitfall 4). `scripts/backup.sh` always passes `-Fc`.

## 2. Schedule, retention, and where artifacts live

- **Schedule:** `$BACKUP_CRON` (default `0 2 * * *` — daily at 02:00), configured via
  `.env` / `.env.example`. Runs inside the `backup` docker-compose service
  (`docker/backup.Dockerfile`, a `postgres:17` base + the official MinIO `mc` client +
  `cron`).
- **Retention:** `$BACKUP_RETENTION_DAYS` (default 14 days) — `scripts/backup.sh`
  deletes `pg_dump` artifacts (`donnarec_<timestamp>.dump`) older than this. The MinIO
  mirror directories (`/backups/minio/slips`, `/backups/minio/receipts`) are **not**
  time-pruned — each run re-mirrors the buckets' *current* state, so they always
  reflect "now", not a point-in-time snapshot history (the timestamped `pg_dump`
  files are the point-in-time snapshots).
- **Location:** everything lives under `/backups` inside the `backup` container,
  backed by the **named Docker volume `backups`** (declared in `docker-compose.yml`).
  This is deliberately **not** a bind mount into the repository — `pg_dump` artifacts
  contain ciphertext (`donor_tax_id_enc`) and KEK-adjacent data, and must never reach
  VCS (T-05-03-DUMPLEAK). Both `.gitignore` (repo root) and `donnarec-api/.gitignore`
  ignore `backups/` as defense-in-depth in case of a manual `docker cp` extraction.

## 3. Restore procedure

`scripts/restore.sh` automates the two restore steps below; run it inside (or with
access to) a container built from `docker/backup.Dockerfile` (it needs `pg_restore`
and `mc`), pointed at the target environment via the same env vars as the `backup`
service (`DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `MINIO_ENDPOINT`,
`MINIO_ACCESS_KEY`, `MINIO_SECRET_KEY`, `MINIO_BUCKET`, `MINIO_RECEIPTS_BUCKET`).

```
DB_PASSWORD=... MINIO_ACCESS_KEY=... MINIO_SECRET_KEY=... \
  restore.sh /backups/donnarec_20260707_020000.dump
```

### Step 1 — restore the database

```
pg_restore --no-owner --role=donnarec_app \
  -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" \
  <dump-file>
```

**Prerequisite:** the target Postgres cluster must already have the `donnarec_app`
role provisioned (created by `migrations/000002_audit_log.up.sql`) *before* running
this. `pg_dump` captures `GRANT`/`REVOKE` statements that *reference* `donnarec_app`,
but it never emits a `CREATE ROLE` statement — roles are cluster-level objects,
outside any single-database dump. If you are restoring into a genuinely empty/roleless
cluster (no migrations have ever run against it), drop `--role=donnarec_app` and use
`--no-privileges` instead; re-run migrations afterward to reapply grants/roles.

**Troubleshooting — "input file does not appear to be a valid archive":** this means
the dump you're restoring was **not** taken with `-Fc` (Pitfall 4). Plain-format
`pg_dump` output is a `.sql` text file and must be restored with `psql -f`, not
`pg_restore` — but `scripts/backup.sh` never produces plain-format dumps, so seeing
this error means either the dump file is corrupted/truncated, or you are pointing
`pg_restore` at the wrong file.

### Step 2 — restore object storage

```
mc alias set donnarec "http://$MINIO_ENDPOINT" "$MINIO_ACCESS_KEY" "$MINIO_SECRET_KEY"
mc mirror --overwrite /backups/minio/slips    "donnarec/$MINIO_BUCKET"
mc mirror --overwrite /backups/minio/receipts "donnarec/$MINIO_RECEIPTS_BUCKET"
```

### Step 3 — verify before resuming traffic

Do not point the `api` service at a restored target until you have confirmed:

- Row counts in `users`, `donations`, `audit_log` are non-zero and plausible for the
  expected point-in-time.
- Spot-check a handful of `donations.receipt_formatted` values against
  `mc ls donnarec/donnarec-receipts` — every issued donation should have a
  corresponding object.
- `donnarec_app` can connect and `SELECT`/`INSERT` against `donations` and
  `audit_log` (it should NOT be able to `UPDATE`/`DELETE` `audit_log` — that REVOKE is
  part of the schema, D-17).

## 4. Repeatable proof harness

`internal/backupverify/restore_test.go` (`TestRestoreProof`,
`TestRestoreProof_MinIO`) is the automated, repeatable version of this entire
procedure: a real `pg_dump -Fc` from a migrated source Postgres testcontainer,
restored into a **second, genuinely fresh/unmigrated** Postgres testcontainer, with
exact row-count assertions; and a real object mirror round trip between two
independent MinIO testcontainers covering both buckets, with byte-exact content
assertions. Run it any time schema or backup tooling changes:

```
cd donnarec-api && go test -count=1 -v ./internal/backupverify/... -run TestRestoreProof
```

If either test starts failing, treat it as a **backup-integrity incident** — it means
a real restore of production data would not have worked either.

## Verification Evidence

Recorded run — 2026-07-07, this plan (05-03):

### 4a. Restore-proof integration tests (real, fresh-target restore)

```
$ go test -count=1 -v ./internal/backupverify/... -run TestRestoreProof
=== RUN   TestRestoreProof
    restore_test.go:240: RESTORE-PROOF: pg_dump -Fc artifact size = 45757 bytes
    restore_test.go:280: RESTORE-PROOF: PASS — users=3 donations=5 audit_log=4 restored
        into a fresh (unmigrated) Postgres 17 container, exactly matching the seeded
        source fixture (pg_dump -Fc artifact = 45757 bytes).
--- PASS: TestRestoreProof (6.77s)
=== RUN   TestRestoreProof_MinIO
    restore_test.go:396: RESTORE-PROOF: mirrored 4 objects out of source MinIO to
        /tmp/TestRestoreProof_MinIO.../001
    restore_test.go:432: RESTORE-PROOF: PASS — 4 objects across both buckets
        (donnarec-slips, donnarec-receipts) restored into a fresh MinIO instance via
        mirror-out/mirror-in round trip, every key present with byte-exact content.
--- PASS: TestRestoreProof_MinIO (3.07s)
PASS
ok  	github.com/donnarec/donnarec-api/internal/backupverify	9.926s
```

**Asserted results:** 3/3 users, 5/5 donations, 4/4 audit_log rows restored into a
Postgres cluster that had *never run a migration* — schema and data both came from
the dump alone. 4/4 MinIO objects across both buckets restored into an empty MinIO
instance with byte-exact content and size.

### 4b. Live backup run against the real local stack (`scripts/backup.sh`)

Run via `docker compose run --rm backup /usr/local/bin/backup.sh` against the actual
running `postgres` + `minio` services (not testcontainers) — proves the production
script path, not just the test harness:

```
[backup] 2026-07-07T11:59:11+00:00 starting pg_dump -Fc -> /backups/donnarec_20260707_115911.dump
[backup] pg_dump complete: 48K
[backup] configuring mc alias 'donnarec' -> http://minio:9000
[backup] mirroring bucket 'donnarec-slips' -> /backups/minio/slips
  (bucket empty at time of run — 0 objects)
[backup] mirroring bucket 'donnarec-receipts' -> /backups/minio/receipts
  `donnarec/donnarec-receipts/receipts/1a46914e-5cd3-48ba-807e-e25ef16a7759.pdf` -> ...
  Total 15.07 KiB transferred
[backup] applying retention: deleting dump artifacts older than 14 days
[backup] 2026-07-07T11:59:11+00:00 done
```

**Asserted result:** a real 48K `pg_dump -Fc` artifact was produced against the live
`donnarec_app` database, and a real, already-issued receipt PDF
(`1a46914e-5cd3-48ba-807e-e25ef16a7759.pdf`) was mirrored out of the live
`donnarec-receipts` bucket — confirming the exact same script path exercised by
`TestRestoreProof`/`TestRestoreProof_MinIO` above also works against the real
docker-compose stack, not only in an isolated testcontainers sandbox.
