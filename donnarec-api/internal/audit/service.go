// Package audit provides the hash-chained audit log service for donnarec-api.
//
// Design decisions realized here:
//
//	D-15: All mutations + auth events are audited via a generic interceptor.
//	D-17: Tamper evidence via SHA-256 hash-chain (prev_hash → row_hash per row).
//	NFR-05: Audit trail is append-only; UPDATE/DELETE denied at DB level for donnarec_app.
//
// Concurrency model:
//
//	AppendAuditEntryTx calls GetLastAuditRowForUpdate (SELECT … FOR UPDATE) to serialize
//	concurrent hash-chain appends. Without this lock, two goroutines could both read the
//	same prev_hash and create a forked/duplicate chain (Pitfall 2, T-1-audit-conc).
//
// Hash strategy (no-UPDATE design):
//
//	Because REVOKE UPDATE ON audit_log prevents the app role from updating rows,
//	we use a two-step approach that avoids any UPDATE:
//	  1. Reserve the next sequence value via nextval('audit_log_id_seq').
//	  2. Capture DB NOW() (stable within the transaction) for the created_at field.
//	  3. Compute SHA-256(id||actor_id||action||resource||created_at||prev_hash).
//	  4. INSERT the row with the pre-computed hash, specifying the reserved id and created_at.
//	This produces a complete, correct hash-chain row in a single INSERT statement.
//
// ANTI-PATTERN PROHIBITION (Foundational Rule 2):
//
//	ห้ามเขียน audit entry ใน goroutine แยก — เขียนแบบ synchronous เสมอ.
//	อุดมคติคือเขียนใน transaction เดียวกับ data mutation ผ่าน AppendAuditEntryTx.
//	ข้อจำกัด Phase 1 (WR-01): audit middleware ใช้ AppendAuditEntry (own-tx, post-commit)
//	ซึ่งเป็น best-effort — ยังไม่ได้ wire in-transaction audit ให้ mutating handler ใด
//	ใน phase นี้. ใช้ AppendAuditEntryTx เมื่อต้องการ atomicity จริง.
package audit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/netip"
	"time"

	dbhelpers "github.com/donnarec/donnarec-api/internal/db"
	db "github.com/donnarec/donnarec-api/internal/db/generated"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// genesisHash is the sentinel prev_hash for the very first audit row.
const genesisHash = "GENESIS"

// AuditEntry holds all data required to create one audit log row.
// Internal fields (PrevHash, RowHash, CreatedAt) are set by the service.
type AuditEntry struct {
	// ActorID is the Keycloak user ID ('sub' claim). Must be a valid UUID string.
	ActorID string
	// ActorEmail is the actor's email from the JWT 'email' claim.
	ActorEmail string
	// Action is the derived action string, e.g. "user.create", "pii.reveal".
	Action string
	// Resource is the Gin route path, e.g. "/api/admin/users".
	Resource string
	// BeforeJSON is the serialized state before a mutation (optional; nil if N/A).
	BeforeJSON []byte
	// AfterJSON is the serialized state after a mutation (optional; nil if N/A).
	AfterJSON []byte
	// IPAddress is the client's IP address string (from c.ClientIP()).
	IPAddress string

	// Internal fields populated by AppendAuditEntryTx — callers should not set these.
	CreatedAt time.Time
	PrevHash  string
	RowHash   string
}

// AuditService manages the append-only hash-chained audit_log.
//
// All public methods are goroutine-safe: each transaction acquires a
// FOR UPDATE lock on the last audit row to serialize concurrent inserts.
type AuditService struct {
	pool    *pgxpool.Pool
	queries *db.Queries
	logger  *zap.Logger
}

// NewAuditService constructs an AuditService with the given dependencies.
func NewAuditService(pool *pgxpool.Pool, queries *db.Queries, logger *zap.Logger) *AuditService {
	if pool == nil {
		panic("audit.NewAuditService: pool must not be nil")
	}
	if queries == nil {
		panic("audit.NewAuditService: queries must not be nil")
	}
	if logger == nil {
		panic("audit.NewAuditService: logger must not be nil")
	}
	return &AuditService{pool: pool, queries: queries, logger: logger}
}

// computeRowHash computes SHA-256(id|actor_id|action|resource|created_at|prev_hash).
// All fields are pipe-delimited to avoid prefix-collision.
// created_at is formatted as RFC3339Nano for a deterministic string representation.
func computeRowHash(id int64, actorID, action, resource string, createdAt time.Time, prevHash string) string {
	h := sha256.New()
	fmt.Fprintf(h, "%d|%s|%s|%s|%s|%s",
		id,
		actorID,
		action,
		resource,
		createdAt.UTC().Format(time.RFC3339Nano),
		prevHash,
	)
	return hex.EncodeToString(h.Sum(nil))
}

// auditChainLockKey is the PostgreSQL advisory lock key used to serialize all
// concurrent audit chain appends. This advisory lock is acquired at the transaction
// level (pg_advisory_xact_lock) so it is released automatically on commit/rollback.
//
// Advisory lock approach is preferred over FOR UPDATE on the last row because:
//   - FOR UPDATE on an empty table returns no rows (no lock) — concurrent txs
//     can both see ErrNoRows, both use "GENESIS" as prev_hash, and produce
//     duplicate prev_hash values (violating chain integrity).
//   - pg_advisory_xact_lock(key) serializes even when the table is empty.
const auditChainLockKey int64 = 0x444F4E415245430A // "DONAREC\n" as int64

// AppendAuditEntryTx appends one audit entry within the caller's existing transaction.
//
// Serialization: acquires pg_advisory_xact_lock(auditChainLockKey) to prevent
// concurrent goroutines from interleaving their prev_hash reads and inserts
// (T-1-audit-conc, Pitfall 2). The lock is released when the transaction ends.
//
// Hash strategy: reserves the next sequence id, captures DB NOW(), computes the hash,
// then INSERTs in a single statement — no UPDATE to audit_log is required.
// This design is compatible with the REVOKE UPDATE restriction on the app role.
func (s *AuditService) AppendAuditEntryTx(ctx context.Context, tx pgx.Tx, entry AuditEntry) error {
	qtx := s.queries.WithTx(tx)

	// Step 1: Acquire a transaction-level advisory lock to serialize concurrent appends.
	// This is the correct serialization primitive for an empty table (Pitfall 2):
	// FOR UPDATE on the last row provides no lock when the table is empty.
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", auditChainLockKey); err != nil {
		return fmt.Errorf("audit: acquire chain lock: %w", err)
	}

	// Step 2: Read the last row's hash (now serialized by the advisory lock).
	prevHash := genesisHash
	lastRow, err := qtx.GetLastAuditRowForUpdate(ctx)
	if err != nil && err != pgx.ErrNoRows {
		return fmt.Errorf("audit: lock last row: %w", err)
	}
	if err == nil {
		prevHash = lastRow.RowHash
	}

	// Step 3: Parse actor UUID
	actorUUID, err := parseUUID(entry.ActorID)
	if err != nil {
		return fmt.Errorf("audit: invalid actor_id %q: %w", entry.ActorID, err)
	}

	// Step 4: Parse IP address (optional)
	var ipAddr *netip.Addr
	if entry.IPAddress != "" {
		if ip := net.ParseIP(entry.IPAddress); ip != nil {
			if parsed, ok := netip.AddrFromSlice(ip); ok {
				addr := parsed.Unmap()
				ipAddr = &addr
			}
		}
	}

	// Step 5: Reserve the next sequence id — allows pre-computing the hash before INSERT.
	// audit_log_id_seq is the implicit BIGSERIAL sequence created by the migration.
	var nextID int64
	if err := tx.QueryRow(ctx, "SELECT nextval('audit_log_id_seq')").Scan(&nextID); err != nil {
		return fmt.Errorf("audit: reserve sequence id: %w", err)
	}

	// Step 6: Capture DB-side current timestamp (stable within the transaction).
	// Using NOW() here and also in the INSERT ensures created_at matches what we hash.
	var now time.Time
	if err := tx.QueryRow(ctx, "SELECT NOW()").Scan(&now); err != nil {
		return fmt.Errorf("audit: get db now: %w", err)
	}

	// Step 7: Compute the full row hash with the reserved id and captured timestamp.
	rowHash := computeRowHash(nextID, entry.ActorID, entry.Action, entry.Resource, now, prevHash)

	// Step 8: INSERT with the pre-computed hash and reserved id.
	// Specifying created_at = NOW() deterministically — same value as Step 5 within the tx.
	// No UPDATE is needed, so this is compatible with REVOKE UPDATE for donnarec_app.
	_, err = tx.Exec(ctx, `
		INSERT INTO audit_log (
			id, actor_id, actor_email, action, resource,
			before_json, after_json, ip_address,
			created_at, prev_hash, row_hash
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8,
			$9, $10, $11
		)`,
		nextID,
		actorUUID,
		entry.ActorEmail,
		entry.Action,
		entry.Resource,
		nullableBytes(entry.BeforeJSON),
		nullableBytes(entry.AfterJSON),
		ipAddr,
		now,
		prevHash,
		rowHash,
	)
	if err != nil {
		return fmt.Errorf("audit: insert: %w", err)
	}

	s.logger.Info("audit entry appended",
		zap.Int64("audit_id", nextID),
		zap.String("action", entry.Action),
		zap.String("resource", entry.Resource),
	)
	return nil
}

// AppendAuditEntry appends one audit entry in its own transaction.
//
// This is the "own-tx" / best-effort path used by the audit middleware AFTER the
// handler's mutation has already committed (WR-01). Because it runs in a separate
// transaction, the data mutation can succeed while this audit write fails — the
// middleware logs that failure but does not roll the mutation back.
//
// For true atomicity (audit + data mutation committed together), callers MUST use
// AppendAuditEntryTx inside their own transaction instead of this method.
func (s *AuditService) AppendAuditEntry(ctx context.Context, entry AuditEntry) error {
	return dbhelpers.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		return s.AppendAuditEntryTx(ctx, tx, entry)
	})
}

// VerifyChain reads all audit rows in insertion order and recomputes each row_hash.
// Returns:
//   - (true, 0, nil)   — chain is intact.
//   - (false, id, nil) — row with `id` is the first broken/tampered row.
//   - (false, 0, err)  — DB error prevented verification.
func (s *AuditService) VerifyChain(ctx context.Context) (bool, int64, error) {
	rows, err := s.queries.ListAllAuditForVerify(ctx)
	if err != nil {
		return false, 0, fmt.Errorf("audit: verify chain read: %w", err)
	}

	prevHash := genesisHash
	for _, row := range rows {
		expected := computeRowHash(
			row.ID,
			row.ActorID.String(),
			row.Action,
			row.Resource,
			row.CreatedAt.Time,
			prevHash,
		)
		if row.RowHash != expected {
			s.logger.Warn("audit chain broken",
				zap.Int64("broken_id", row.ID),
				zap.String("stored_hash", row.RowHash),
				zap.String("expected_hash", expected),
			)
			return false, row.ID, nil
		}
		prevHash = row.RowHash
	}
	return true, 0, nil
}

// --- private helpers ---

// parseUUID converts a UUID string to pgtype.UUID (pgx driver type).
func parseUUID(s string) (pgtype.UUID, error) {
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		return pgtype.UUID{}, err
	}
	return u, nil
}

// nullableBytes returns nil if b is empty, otherwise b.
func nullableBytes(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}
	return b
}
