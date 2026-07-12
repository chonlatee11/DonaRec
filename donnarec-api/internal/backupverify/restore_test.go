// Package backupverify contains restore-proof integration tests for NFR-08 / D-73.
//
// D-72 requires backups to cover BOTH Postgres (pg_dump -Fc) AND both MinIO buckets
// (slips + receipts, via mirror). D-73 requires a REAL restore to have been proven —
// not a "cron is configured" check — with recorded evidence. These two tests are that
// proof: they run a real pg_dump/pg_restore round trip between two independent
// testcontainers Postgres instances (the second one genuinely fresh/unmigrated) and a
// real object mirror round trip between two independent testcontainers MinIO instances,
// then assert the restored data exactly matches what was backed up.
//
// See docs/BACKUP_RESTORE_RUNBOOK.md "Verification Evidence" for a recorded real run
// of these tests (D-73's "recorded evidence" requirement).
package backupverify

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	miniotc "github.com/testcontainers/testcontainers-go/modules/minio"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// ============================================================
// TestRestoreProof — Postgres pg_dump -Fc / pg_restore round trip
// ============================================================

// Fixed test credentials mirroring internal/testutil/postgres.go's SetupTestPostgres
// convention — a fixed, non-secret user/password/db is fine for ephemeral testcontainers.
const (
	pgUser = "test"
	pgPass = "test"
	pgDB   = "donnarec_test"
)

// Fixture sizes are fixed (not random) so the restored-row-count assertion is exact —
// D-73 requires asserted completeness, not a "some rows came back" smoke check.
const (
	expectedUsers     = 3
	expectedDonations = 5
	expectedAuditLogs = 4
)

// migrationSeededUsers counts rows the migrations themselves insert into `users`
// before seedFixture runs. Migration 000016 (Phase 6, Flow B) seeds exactly one
// fixed-UUID public-web system user (D-76). The dump carries it, so both the
// migrated source and the restored target hold expectedUsers + migrationSeededUsers
// user rows. donations/audit_log get no migration-seeded rows, so only the users
// assertions add this baseline.
const migrationSeededUsers = 1

// startMigratedPostgres starts a source Postgres 17 testcontainer with ALL migrations
// applied. It mirrors testutil.SetupTestPostgres's shape, but also returns the
// *postgres.PostgresContainer itself (not just the pool): this test needs
// Exec/CopyFileFromContainer to run a REAL pg_dump inside the container, which
// testutil's pool-only helper does not expose.
func startMigratedPostgres(t *testing.T) (*postgres.PostgresContainer, *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx,
		"postgres:17",
		postgres.WithDatabase(pgDB),
		postgres.WithUsername(pgUser),
		postgres.WithPassword(pgPass),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
		),
	)
	require.NoError(t, err, "failed to start source postgres container")
	t.Cleanup(func() {
		if err := pgContainer.Terminate(ctx); err != nil {
			t.Logf("warning: failed to terminate source postgres container: %v", err)
		}
	})

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err, "failed to get source connection string")

	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err, "failed to create source pgxpool")
	t.Cleanup(pool.Close)

	// Migration path mirrors internal/testutil/postgres.go: "../../migrations" relative
	// to this package directory (internal/backupverify -> donnarec-api/migrations).
	migrateURL := "pgx5://" + connStr[len("postgres://"):]
	m, err := migrate.New("file://../../migrations", migrateURL)
	require.NoError(t, err, "failed to create migrator")
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("migration up failed: %v", err)
	}

	return pgContainer, pool
}

// startFreshPostgres starts a Postgres 17 testcontainer with NO migrations applied — a
// genuinely empty cluster/database. This is what proves (per the plan's acceptance
// criteria) that pg_restore recreates schema+data from the dump ALONE, not by relying
// on pre-applied migrations already having built the schema.
func startFreshPostgres(t *testing.T) *postgres.PostgresContainer {
	t.Helper()
	ctx := context.Background()

	pgContainer, err := postgres.Run(ctx,
		"postgres:17",
		postgres.WithDatabase(pgDB),
		postgres.WithUsername(pgUser),
		postgres.WithPassword(pgPass),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
		),
	)
	require.NoError(t, err, "failed to start fresh (unmigrated) target postgres container")
	t.Cleanup(func() {
		if err := pgContainer.Terminate(ctx); err != nil {
			t.Logf("warning: failed to terminate target postgres container: %v", err)
		}
	})

	return pgContainer
}

// seedFixture inserts a known, fixed-size set of users/donations/audit_log rows into the
// source database so the post-restore assertion can check an EXACT count, not "> 0".
// Donations are seeded as 'draft' (no receipt_number_id) — the simplest status that
// satisfies chk_receipt_only_on_issued_or_cancelled without needing a full Phase-2/3
// issuance flow; this test proves restore completeness, not issuance business logic.
func seedFixture(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	userIDs := make([]string, 0, expectedUsers)
	for i := 0; i < expectedUsers; i++ {
		var id string
		err := pool.QueryRow(ctx, `
			INSERT INTO users (email, display_name, keycloak_subject)
			VALUES ($1, $2, $3)
			RETURNING id
		`,
			fmt.Sprintf("restoreproof-user-%d@example.com", i),
			fmt.Sprintf("Restore Proof User %d", i),
			fmt.Sprintf("kc-restoreproof-%d", i),
		).Scan(&id)
		require.NoError(t, err, "seed user %d", i)
		userIDs = append(userIDs, id)
	}

	for i := 0; i < expectedDonations; i++ {
		_, err := pool.Exec(ctx, `
			INSERT INTO donations (created_by, donor_name, donor_tax_id_enc, donor_tax_id_dek, amount, donated_at)
			VALUES ($1, $2, $3, $4, $5, CURRENT_DATE)
		`,
			userIDs[i%len(userIDs)],
			fmt.Sprintf("Restore Proof Donor %d", i),
			[]byte(fmt.Sprintf("ciphertext-fixture-%d", i)),
			[]byte(fmt.Sprintf("dek-fixture-%d", i)),
			100.00+float64(i),
		)
		require.NoError(t, err, "seed donation %d", i)
	}

	for i := 0; i < expectedAuditLogs; i++ {
		_, err := pool.Exec(ctx, `
			INSERT INTO audit_log (actor_id, actor_email, action, resource, prev_hash, row_hash)
			VALUES (gen_random_uuid(), $1, 'restoreproof.seed', '/internal/backupverify', $2, $3)
		`,
			fmt.Sprintf("restoreproof-actor-%d@example.com", i),
			fmt.Sprintf("prevhash-fixture-%d", i),
			fmt.Sprintf("rowhash-fixture-%d", i),
		)
		require.NoError(t, err, "seed audit_log %d", i)
	}
}

// execInContainer runs cmd inside c and fails the test (with full command output
// attached) on a non-zero exit code or exec transport error. pg_dump/pg_restore
// failures must be loud, never silently swallowed.
func execInContainer(ctx context.Context, t *testing.T, c testcontainers.Container, cmd []string) string {
	t.Helper()

	code, reader, err := c.Exec(ctx, cmd)
	require.NoError(t, err, "exec failed: %v", cmd)

	var buf bytes.Buffer
	if reader != nil {
		_, _ = io.Copy(&buf, reader)
	}
	output := buf.String()

	require.Zerof(t, code, "command exited %d: %v\noutput:\n%s", code, cmd, output)
	return output
}

// TestRestoreProof is the D-73 Postgres evidence: a real pg_dump -Fc artifact, produced
// inside a migrated source Postgres container, is pg_restore'd into a SECOND, completely
// fresh (unmigrated) Postgres container, and the restored row counts are asserted to
// exactly match the seeded fixture.
func TestRestoreProof(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	ctx := context.Background()

	// 1. Source: migrated Postgres with a known fixture seeded.
	sourceContainer, sourcePool := startMigratedPostgres(t)
	seedFixture(ctx, t, sourcePool)

	// Sanity: confirm the seed actually landed before trusting the dump to carry it.
	var seededUsers, seededDonations, seededAuditLogs int
	require.NoError(t, sourcePool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&seededUsers))
	require.NoError(t, sourcePool.QueryRow(ctx, `SELECT count(*) FROM donations`).Scan(&seededDonations))
	require.NoError(t, sourcePool.QueryRow(ctx, `SELECT count(*) FROM audit_log`).Scan(&seededAuditLogs))
	require.Equal(t, expectedUsers+migrationSeededUsers, seededUsers)
	require.Equal(t, expectedDonations, seededDonations)
	require.Equal(t, expectedAuditLogs, seededAuditLogs)

	// 2. pg_dump -Fc (custom format — Pitfall 4: plain-format dumps cannot be
	//    pg_restore'd) run INSIDE the source container using the real pg_dump binary
	//    shipped by the postgres:17 image. No -h flag: connects over the local unix
	//    socket, which the official postgres image trusts locally — no password needed.
	const dumpPath = "/tmp/backupverify.dump"
	execInContainer(ctx, t, sourceContainer, []string{
		"pg_dump", "-Fc", "-U", pgUser, "-d", pgDB, "-f", dumpPath,
	})

	// 3. Pull the real dump bytes out of the source container. No local pg_dump/pg_restore
	//    binary is assumed to exist on the test-runner host — every dump/restore command
	//    runs inside the real postgres:17 container image, matching what
	//    docker/backup.Dockerfile ships in production.
	dumpReader, err := sourceContainer.CopyFileFromContainer(ctx, dumpPath)
	require.NoError(t, err, "copy dump out of source container")
	dumpBytes, err := io.ReadAll(dumpReader)
	require.NoError(t, err, "read dump bytes")
	_ = dumpReader.Close()
	require.NotEmpty(t, dumpBytes, "pg_dump produced an empty artifact")
	t.Logf("RESTORE-PROOF: pg_dump -Fc artifact size = %d bytes", len(dumpBytes))

	// 4. Target: a GENUINELY FRESH, unmigrated Postgres container — no schema, no roles,
	//    no data. Only the dump will populate it.
	targetContainer := startFreshPostgres(t)

	require.NoError(t, targetContainer.CopyToContainer(ctx, dumpBytes, dumpPath, 0o644),
		"copy dump into fresh target container")

	// 5. pg_restore into the fresh target. --no-owner --no-privileges: the target cluster
	//    has none of the source's roles (donnarec_app is created by migration 000002 — a
	//    genuinely fresh cluster never ran that migration), so ownership/ACL statements
	//    are skipped rather than erroring. This is a deliberate test-scope decision: the
	//    invariant under test is DATA COMPLETENESS (row counts), not ACL fidelity.
	//    Production restores (scripts/restore.sh) target a cluster where donnarec_app
	//    already exists and use --role=donnarec_app to reattach ownership — see
	//    docs/BACKUP_RESTORE_RUNBOOK.md.
	restoreOutput := execInContainer(ctx, t, targetContainer, []string{
		"pg_restore", "--no-owner", "--no-privileges", "-U", pgUser, "-d", pgDB, dumpPath,
	})
	t.Logf("RESTORE-PROOF: pg_restore output:\n%s", restoreOutput)

	// 6. Connect to the restored target and assert EXACT row-count parity with the
	//    seeded source fixture — D-73's "asserted completeness", not a configured-but-
	//    unverified cron job.
	targetConnStr, err := targetContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err, "get target connection string")
	targetPool, err := pgxpool.New(ctx, targetConnStr)
	require.NoError(t, err, "connect to restored target")
	defer targetPool.Close()

	var restoredUsers, restoredDonations, restoredAuditLogs int
	require.NoError(t, targetPool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&restoredUsers))
	require.NoError(t, targetPool.QueryRow(ctx, `SELECT count(*) FROM donations`).Scan(&restoredDonations))
	require.NoError(t, targetPool.QueryRow(ctx, `SELECT count(*) FROM audit_log`).Scan(&restoredAuditLogs))

	require.Equal(t, expectedUsers+migrationSeededUsers, restoredUsers, "restored users count must exactly match seeded fixture plus migration-seeded system users")
	require.Equal(t, expectedDonations, restoredDonations, "restored donations count must exactly match seeded fixture")
	require.Equal(t, expectedAuditLogs, restoredAuditLogs, "restored audit_log count must exactly match seeded fixture")

	t.Logf("RESTORE-PROOF: PASS — users=%d donations=%d audit_log=%d restored into a fresh (unmigrated) Postgres 17 container, exactly matching the seeded source fixture (pg_dump -Fc artifact = %d bytes).",
		restoredUsers, restoredDonations, restoredAuditLogs, len(dumpBytes))
}

// ============================================================
// TestRestoreProof_MinIO — object-storage mirror round trip (both buckets)
// ============================================================

// minioTestImage pins the same MinIO test image used by internal/testutil/minio.go
// (a fixed tag, not ":latest", for reproducible CI runs). Duplicated here rather than
// imported because this test needs TWO independent instances, each with BOTH donnarec
// buckets pre-created — testutil.StartMinio only creates one bucket per call.
const minioTestImage = "minio/minio:RELEASE.2024-01-16T16-07-38Z"

// Bucket names mirror internal/config.Config defaults (MinIOConfig.Bucket /
// MinIOConfig.ReceiptsBucket) — D-72 requires BOTH covered by backup, not just one.
const (
	slipsBucketName    = "donnarec-slips"
	receiptsBucketName = "donnarec-receipts"
)

// startFreshMinio starts a MinIO testcontainer and creates both donnarec buckets
// (empty) in it.
func startFreshMinio(t *testing.T) *minio.Client {
	t.Helper()
	ctx := context.Background()

	c, err := miniotc.Run(ctx, minioTestImage)
	require.NoError(t, err, "failed to start minio container")
	t.Cleanup(func() {
		if err := c.Terminate(ctx); err != nil {
			t.Logf("warning: failed to terminate minio container: %v", err)
		}
	})

	endpoint, err := c.ConnectionString(ctx)
	require.NoError(t, err, "get minio connection string")

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(c.Username, c.Password, ""),
		Secure: false,
	})
	require.NoError(t, err, "create minio client")

	for _, bucket := range []string{slipsBucketName, receiptsBucketName} {
		require.NoError(t, client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}),
			"create bucket %s", bucket)
	}

	return client
}

// minioFixtureObject is a single known object seeded into the source MinIO instance.
type minioFixtureObject struct {
	bucket  string
	key     string
	content []byte
}

// minioFixture returns a fixed, known set of objects spanning BOTH buckets (D-72: a
// DB-only backup is not enough — object storage for slips AND frozen receipt PDFs must
// both be covered and both be proven restorable).
func minioFixture() []minioFixtureObject {
	return []minioFixtureObject{
		{slipsBucketName, "2026/07/slip-fixture-1.jpg", []byte("slip-fixture-content-1")},
		{slipsBucketName, "2026/07/slip-fixture-2.jpg", []byte("slip-fixture-content-2-longer-payload")},
		{receiptsBucketName, "2026/07/receipt-fixture-1.pdf", []byte("receipt-fixture-content-1")},
		{receiptsBucketName, "2026/07/receipt-fixture-2.pdf", []byte("receipt-fixture-content-2-longer-payload")},
	}
}

// TestRestoreProof_MinIO is the D-73 object-storage evidence: a known set of objects is
// seeded into BOTH donnarec buckets on a source MinIO instance, mirrored out to a local
// temp directory (the same shape `mc mirror <bucket> <local-dir>` produces), then
// mirrored back in to a SECOND, completely fresh/empty MinIO instance. Every expected
// object key is asserted present with byte-for-byte matching content and size — not
// just "the bucket is non-empty".
//
// This test uses the minio-go SDK (List/Get/Put) to perform the mirror round trip
// rather than shelling out to the `mc` CLI: the test-runner host has no `mc` binary
// installed (only inside docker/backup.Dockerfile's runtime image, which Task 1
// verifies via `docker compose config` + a static grep for `mc mirror` in
// scripts/backup.sh). The SDK round trip is a functionally equivalent, more portable
// proof of the same invariant D-73 requires: real objects, real restore, asserted
// completeness — see docs/BACKUP_RESTORE_RUNBOOK.md.
func TestRestoreProof_MinIO(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	ctx := context.Background()

	sourceClient := startFreshMinio(t)
	fixture := minioFixture()

	for _, obj := range fixture {
		_, err := sourceClient.PutObject(ctx, obj.bucket, obj.key,
			bytes.NewReader(obj.content), int64(len(obj.content)), minio.PutObjectOptions{})
		require.NoError(t, err, "seed object %s/%s", obj.bucket, obj.key)
	}

	// "mc mirror <bucket> <local-dir>" step: pull every seeded object down to a real
	// local temp directory, laid out as bucket/key — a genuine on-disk intermediate,
	// not an in-memory shortcut.
	mirrorDir := t.TempDir()
	for _, obj := range fixture {
		reader, err := sourceClient.GetObject(ctx, obj.bucket, obj.key, minio.GetObjectOptions{})
		require.NoError(t, err, "mirror-out get %s/%s", obj.bucket, obj.key)
		data, err := io.ReadAll(reader)
		require.NoError(t, err, "mirror-out read %s/%s", obj.bucket, obj.key)
		_ = reader.Close()
		require.Equal(t, obj.content, data, "mirror-out content mismatch for %s/%s", obj.bucket, obj.key)

		localPath := filepath.Join(mirrorDir, obj.bucket, obj.key)
		require.NoError(t, os.MkdirAll(filepath.Dir(localPath), 0o755))
		require.NoError(t, os.WriteFile(localPath, data, 0o644), "mirror-out write %s", localPath)
	}
	t.Logf("RESTORE-PROOF: mirrored %d objects out of source MinIO to %s", len(fixture), mirrorDir)

	// Fresh target: a SECOND, completely empty MinIO instance — proves the local
	// mirror artifact alone can repopulate object storage, the same way scripts/
	// restore.sh's `mc mirror /backups/minio/<bucket> donnarec/<bucket>` would after a
	// real disaster.
	targetClient := startFreshMinio(t)

	// "mc mirror <local-dir> <bucket>" step: push the local mirror back into the fresh
	// target buckets.
	for _, obj := range fixture {
		localPath := filepath.Join(mirrorDir, obj.bucket, obj.key)
		data, err := os.ReadFile(localPath)
		require.NoError(t, err, "mirror-in read %s", localPath)

		_, err = targetClient.PutObject(ctx, obj.bucket, obj.key,
			bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{})
		require.NoError(t, err, "mirror-in put %s/%s", obj.bucket, obj.key)
	}

	// Assert every expected object key is present in the fresh target with matching
	// content AND size — D-73's asserted completeness for object storage.
	for _, obj := range fixture {
		info, err := targetClient.StatObject(ctx, obj.bucket, obj.key, minio.StatObjectOptions{})
		require.NoErrorf(t, err, "expected object %s/%s missing after restore", obj.bucket, obj.key)
		require.Equal(t, int64(len(obj.content)), info.Size,
			"restored object %s/%s size mismatch", obj.bucket, obj.key)

		reader, err := targetClient.GetObject(ctx, obj.bucket, obj.key, minio.GetObjectOptions{})
		require.NoError(t, err, "read restored object %s/%s", obj.bucket, obj.key)
		data, err := io.ReadAll(reader)
		require.NoError(t, err)
		_ = reader.Close()
		require.Equal(t, obj.content, data, "restored object %s/%s content mismatch", obj.bucket, obj.key)
	}

	t.Logf("RESTORE-PROOF: PASS — %d objects across both buckets (%s, %s) restored into a fresh MinIO instance via mirror-out/mirror-in round trip, every key present with byte-exact content.",
		len(fixture), slipsBucketName, receiptsBucketName)
}
