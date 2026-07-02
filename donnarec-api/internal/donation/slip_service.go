// Package donation — slip_service.go
//
// SlipService handles donation slip attachment lifecycle (Plan 03-04, D-48, D-54).
//
// Key design invariants:
//
//	D-48: slip is optional — cash/no-slip donations proceed without it
//	D-54: soft-delete only (deleted_at/deleted_by) — files retained in MinIO for audit
//	T-03-15: 10 MB cap + magic-byte validation enforced in storage.PutSlip (not repeated here)
//	T-03-16: presigned URLs with 15-min TTL (slipPresignTTL)
//	Pattern D: AppendAuditEntryTx inside every mutation transaction for immutable audit trail
//	Pattern C: no PII in logs — only donation_id + actor UUID
package donation

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/donnarec/donnarec-api/internal/audit"
	"github.com/donnarec/donnarec-api/internal/auth"
	dbhelpers "github.com/donnarec/donnarec-api/internal/db"
	db "github.com/donnarec/donnarec-api/internal/db/generated"
	"github.com/donnarec/donnarec-api/internal/storage"
)

// slipPresignTTL is the presigned URL lifetime for ViewSlip (T-03-16, UI-SPEC Screen 5).
const slipPresignTTL = 15 * time.Minute

// Sentinel errors for slip attachment operations (Plan 03-04).
var (
	// ErrSlipAlreadyExists is returned when an upload is attempted but an active
	// (non-deleted) slip already exists for the donation. The caller must remove
	// the existing slip first (D-54 audit trail integrity).
	// HTTP mapping: 409 Conflict.
	ErrSlipAlreadyExists = errors.New("donation: slip attachment already exists — remove the existing slip before uploading a replacement")

	// ErrSlipNotFound is returned when ViewSlip or RemoveSlip is called but there is
	// no active slip for the donation. Normal for cash/no-slip donations (D-48).
	// HTTP mapping: 404 Not Found.
	ErrSlipNotFound = errors.New("donation: no active slip attachment found for this donation")
)

// SlipResponse is the API response for a successful UploadSlip.
type SlipResponse struct {
	ID         string `json:"id"`
	DonationID string `json:"donation_id"`
	MimeType   string `json:"mime_type"`
	SizeBytes  int64  `json:"size_bytes"`
	UploadedAt string `json:"uploaded_at"` // RFC3339 UTC
}

// SlipViewResponse is the API response for ViewSlip: a short-lived presigned URL (T-03-16).
type SlipViewResponse struct {
	URL       string `json:"url"`
	ExpiresIn int    `json:"expires_in_seconds"` // always 900 (15 min)
}

// SlipService handles slip attachment business logic for Plan 03-04.
// All DB mutations use dbhelpers.WithTx + audit.AppendAuditEntryTx for atomicity + immutable trail.
type SlipService struct {
	pool     *pgxpool.Pool
	queries  *db.Queries
	storage  *storage.StorageClient
	auditSvc *audit.AuditService
	logger   *zap.Logger
}

// NewSlipService constructs a SlipService with injected dependencies.
func NewSlipService(
	pool *pgxpool.Pool,
	queries *db.Queries,
	storageClient *storage.StorageClient,
	auditSvc *audit.AuditService,
	logger *zap.Logger,
) *SlipService {
	return &SlipService{
		pool:     pool,
		queries:  queries,
		storage:  storageClient,
		auditSvc: auditSvc,
		logger:   logger,
	}
}

// UploadSlip validates, uploads a slip file to MinIO, and records the reference in DB.
//
// D-48: slip is optional, but if one already exists (not soft-deleted) caller must
// remove it first (ErrSlipAlreadyExists → 409 Conflict).
// T-03-15: 10 MB cap and magic-byte validation are enforced inside storage.PutSlip.
// Audit: slip.upload entry appended inside the same DB transaction as InsertSlip (Pattern D).
// Pattern C: logs only donation_id + uploaded_by UUID — no file name, content, or PII.
func (s *SlipService) UploadSlip(
	ctx context.Context,
	donationID string,
	r io.Reader,
	size int64,
	actingUserID pgtype.UUID,
	claims auth.KeycloakClaims,
) (*SlipResponse, error) {
	var pgDonationID pgtype.UUID
	if err := pgDonationID.Scan(donationID); err != nil {
		return nil, fmt.Errorf("invalid donation ID: %w", err)
	}

	// D-48: reject if an active slip already exists — caller must remove first.
	// ErrNoRows means no active slip (safe to proceed).
	existing, err := s.queries.GetActiveSlipByDonation(ctx, pgDonationID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("check existing slip: %w", err)
	}
	if err == nil && existing.ID.Valid {
		return nil, ErrSlipAlreadyExists
	}

	// Upload to MinIO: validates magic bytes + size (T-03-14, T-03-15) then streams.
	// storage.ErrFileTooLarge or storage.ErrUnsupportedFileType bubble up to handler.
	objectKey, mimeType, err := s.storage.PutSlip(ctx, r, size, donationID)
	if err != nil {
		return nil, fmt.Errorf("put slip: %w", err)
	}

	// Insert DB reference + audit in a single transaction (atomicity — if audit fails, rollback).
	var inserted db.SlipAttachment
	if txErr := dbhelpers.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		qtx := s.queries.WithTx(tx)

		var insertErr error
		inserted, insertErr = qtx.InsertSlip(ctx, db.InsertSlipParams{
			DonationID: pgDonationID,
			ObjectKey:  objectKey,
			MimeType:   mimeType,
			SizeBytes:  size,
			UploadedBy: actingUserID,
		})
		if insertErr != nil {
			return fmt.Errorf("insert slip: %w", insertErr)
		}

		// Pattern D: AppendAuditEntryTx inside same tx — audit row committed with DB record.
		return s.auditSvc.AppendAuditEntryTx(ctx, tx, audit.AuditEntry{
			ActorID:    claims.Subject,
			ActorEmail: claims.Email,
			Action:     "slip.upload",
			Resource:   "/api/donations/" + donationID + "/slip",
		})
	}); txErr != nil {
		return nil, txErr
	}

	// Pattern C: log donation_id + uploaded_by only — no file name, PII, or path details.
	s.logger.Info("slip uploaded",
		zap.String("donation_id", donationID),
		zap.String("uploaded_by", claims.Subject),
	)

	return slipToResponse(inserted), nil
}

// ViewSlip returns a short-lived presigned URL for the donation's active slip (T-03-16).
//
// D-48: if no active slip exists (cash/no-slip donation), returns ErrSlipNotFound (404).
// TTL is 15 minutes (slipPresignTTL). The object key contains a UUID so the URL is not guessable.
// No audit entry for read-only view (reads are not individually audited per D-15).
func (s *SlipService) ViewSlip(ctx context.Context, donationID string, claims auth.KeycloakClaims) (*SlipViewResponse, error) {
	var pgDonationID pgtype.UUID
	if err := pgDonationID.Scan(donationID); err != nil {
		return nil, fmt.Errorf("invalid donation ID: %w", err)
	}

	slip, err := s.queries.GetActiveSlipByDonation(ctx, pgDonationID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSlipNotFound
		}
		return nil, fmt.Errorf("get slip: %w", err)
	}

	url, err := s.storage.PresignedGet(ctx, slip.ObjectKey, slipPresignTTL)
	if err != nil {
		return nil, fmt.Errorf("presign slip URL: %w", err)
	}

	return &SlipViewResponse{
		URL:       url,
		ExpiresIn: int(slipPresignTTL.Seconds()),
	}, nil
}

// RemoveSlip soft-deletes the active slip reference for a donation (D-54).
//
// The file is NOT removed from MinIO — it is retained for audit/evidence.
// The DB REVOKE DELETE on slip_attachments ensures the reference is also immutable.
// D-48: if no active slip exists, returns ErrSlipNotFound (404).
// Audit: slip.remove entry appended inside the same DB transaction as SoftDeleteSlip (Pattern D).
func (s *SlipService) RemoveSlip(ctx context.Context, donationID string, actingUserID pgtype.UUID, claims auth.KeycloakClaims) error {
	var pgDonationID pgtype.UUID
	if err := pgDonationID.Scan(donationID); err != nil {
		return fmt.Errorf("invalid donation ID: %w", err)
	}

	// Load active slip before entering tx (avoids holding lock during GetActiveSlipByDonation).
	slip, err := s.queries.GetActiveSlipByDonation(ctx, pgDonationID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrSlipNotFound
		}
		return fmt.Errorf("get slip: %w", err)
	}

	// Soft-delete + audit in a single transaction (atomic).
	return dbhelpers.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		qtx := s.queries.WithTx(tx)

		if delErr := qtx.SoftDeleteSlip(ctx, db.SoftDeleteSlipParams{
			ID:        slip.ID,
			DeletedBy: actingUserID,
		}); delErr != nil {
			return fmt.Errorf("soft delete slip: %w", delErr)
		}

		// Pattern D: audit row committed atomically with the soft-delete.
		return s.auditSvc.AppendAuditEntryTx(ctx, tx, audit.AuditEntry{
			ActorID:    claims.Subject,
			ActorEmail: claims.Email,
			Action:     "slip.remove",
			Resource:   "/api/donations/" + donationID + "/slip",
		})
	})
}

// slipToResponse converts a db.SlipAttachment to a SlipResponse for JSON serialization.
func slipToResponse(s db.SlipAttachment) *SlipResponse {
	uploadedAt := ""
	if s.UploadedAt.Valid {
		uploadedAt = s.UploadedAt.Time.UTC().Format(time.RFC3339)
	}
	return &SlipResponse{
		ID:         s.ID.String(),
		DonationID: s.DonationID.String(),
		MimeType:   s.MimeType,
		SizeBytes:  s.SizeBytes,
		UploadedAt: uploadedAt,
	}
}
