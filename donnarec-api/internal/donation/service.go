// Package donation — service.go
//
// DonationService implements the donation lifecycle business logic (FR-07, FR-09, FR-11).
// Dependency injection via constructor; no global state.
//
// State machine (D-45, FR-11):
//
//	draft ──submit──► pending_review ──approve──► issued ──cancel──► cancelled
//	             ◄──return──┘         ──reject──► rejected (terminal)
//
// canTransition is the single source of truth for allowed transitions.
// Approve/return/reject/cancel arms are wired in plans 03-05/03-06.
//
// PII rules (PDPA, NFR-02):
//   - Donor tax/national ID is AES-256-GCM encrypted before any DB write (T-03-08).
//   - Responses always expose DonorTaxIDMasked (last-4 reveal) — never plaintext (T-03-09).
//   - Logs contain donation_id + created_by UUID only — no PII fields (T-03-10, Pattern C).
package donation

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/donnarec/donnarec-api/internal/audit"
	"github.com/donnarec/donnarec-api/internal/auth"
	"github.com/donnarec/donnarec-api/internal/crypto"
	dbhelpers "github.com/donnarec/donnarec-api/internal/db"
	db "github.com/donnarec/donnarec-api/internal/db/generated"
	"github.com/donnarec/donnarec-api/internal/pii"
	"github.com/donnarec/donnarec-api/internal/receiptno"
)

// DonationService implements donation lifecycle business logic.
// All dependencies are constructor-injected (no global state — Pattern B).
type DonationService struct {
	pool        *pgxpool.Pool
	queries     *db.Queries
	allocator   *receiptno.Allocator // used in Approve (03-05)
	auditSvc    *audit.AuditService  // used in Approve (03-05)
	keyProvider crypto.KeyProvider
	logger      *zap.Logger
}

// NewDonationService constructs a DonationService with injected dependencies.
// allocator and auditSvc may be nil for tests that only exercise Task 1/2 methods.
func NewDonationService(
	pool *pgxpool.Pool,
	queries *db.Queries,
	allocator *receiptno.Allocator,
	auditSvc *audit.AuditService,
	keyProvider crypto.KeyProvider,
	logger *zap.Logger,
) *DonationService {
	return &DonationService{
		pool:        pool,
		queries:     queries,
		allocator:   allocator,
		auditSvc:    auditSvc,
		keyProvider: keyProvider,
		logger:      logger,
	}
}

// canTransition returns true if the given action is permitted from the current donation status.
//
// This function is the single source of truth for the D-45 state machine (plan 03-03).
// Submit and update are the two transitions owned by this plan.
// Approve/return/reject/cancel arms are wired in plans 03-05/03-06.
func canTransition(from db.DonationStatus, action string) bool {
	switch action {
	case "submit":
		// Only draft can be submitted.
		return from == db.DonationStatusDraft
	case "update":
		// Only draft can be edited (FR-09).
		return from == db.DonationStatusDraft
	default:
		return false
	}
}

// Create inserts a new donation record in 'draft' status with encrypted PII + consent snapshot.
//
// D-44: DonorTaxID is mandatory — ErrMissingTaxID is returned before any DB call if empty.
// T-03-08: EncryptField is called before any DB write — plaintext never reaches Postgres.
// D-49: consent fields (consent_given/at/text_version/purpose) are captured per-snapshot.
// Pattern C: logs only donation_id + created_by UUID — no PII fields ever logged.
func (s *DonationService) Create(ctx context.Context, req CreateDonationRequest, claims auth.KeycloakClaims) (*DonationResponse, error) {
	// D-44: mandatory tax ID check — fail fast before any DB call.
	if req.DonorTaxID == "" {
		return nil, ErrMissingTaxID
	}

	donatedAtTime, err := time.ParseInLocation("2006-01-02", req.DonatedAt, time.UTC)
	if err != nil {
		return nil, fmt.Errorf("invalid donated_at %q: %w", req.DonatedAt, err)
	}

	var createdByUUID pgtype.UUID
	if err := createdByUUID.Scan(claims.Subject); err != nil {
		return nil, fmt.Errorf("invalid creator UUID: %w", err)
	}

	// T-03-08: AES-256-GCM envelope encryption — plaintext never reaches Postgres.
	encBytes, dekBytes, err := crypto.EncryptField(ctx, s.keyProvider, []byte(req.DonorTaxID))
	if err != nil {
		return nil, fmt.Errorf("encrypt donor tax ID: %w", err)
	}

	var amount pgtype.Numeric
	if err := amount.Scan(strconv.FormatFloat(req.Amount, 'f', 2, 64)); err != nil {
		return nil, fmt.Errorf("invalid amount: %w", err)
	}

	donatedAt := pgtype.Date{Time: donatedAtTime, Valid: true}
	// PDPA default retention: 10 years from donation date (retain_until NOT NULL constraint).
	retainUntil := pgtype.Date{Time: donatedAtTime.AddDate(10, 0, 0), Valid: true}

	// D-49: consent_at is set to now() when consent_given=true.
	var consentAt pgtype.Timestamptz
	if req.ConsentGiven {
		consentAt = pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	}

	// Optional pointer fields — nil means absent in DB, omitted in JSON.
	var donorEmail *string
	if req.DonorEmail != "" {
		v := req.DonorEmail
		donorEmail = &v
	}
	var notes *string
	if req.Notes != "" {
		v := req.Notes
		notes = &v
	}
	var consentTextVersion *string
	if req.ConsentTextVersion != "" {
		v := req.ConsentTextVersion
		consentTextVersion = &v
	}
	var consentPurpose *string
	if req.ConsentPurpose != "" {
		v := req.ConsentPurpose
		consentPurpose = &v
	}

	var row db.CreateDonationRow
	err = dbhelpers.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		qtx := s.queries.WithTx(tx)
		var txErr error
		row, txErr = qtx.CreateDonation(ctx, db.CreateDonationParams{
			CreatedBy:          createdByUUID,
			DonorName:          req.DonorName,
			DonorAddress:       req.DonorAddress,
			DonorEmail:         donorEmail,
			DonorTaxIDEnc:      encBytes,
			DonorTaxIDDek:      dekBytes,
			Amount:             amount,
			DonatedAt:          donatedAt,
			Notes:              notes,
			ConsentGiven:       req.ConsentGiven,
			ConsentAt:          consentAt,
			ConsentTextVersion: consentTextVersion,
			ConsentPurpose:     consentPurpose,
			RetainUntil:        retainUntil,
			LegalBasis:         "consent",
		})
		return txErr
	})
	if err != nil {
		return nil, fmt.Errorf("create donation: %w", err)
	}

	// Pattern C: log only donation_id + created_by — no donor name, tax ID, or email.
	s.logger.Info("donation created",
		zap.String("donation_id", row.ID.String()),
		zap.String("created_by", row.CreatedBy.String()),
	)

	resp := &DonationResponse{
		ID:                 row.ID.String(),
		Status:             string(row.Status),
		DonorName:          req.DonorName,
		DonorTaxIDMasked:   pii.MaskNationalID(req.DonorTaxID), // T-03-09: masked, never plaintext
		DonorAddress:       req.DonorAddress,
		DonorEmail:         donorEmail,
		Amount:             strconv.FormatFloat(req.Amount, 'f', 2, 64),
		DonatedAt:          req.DonatedAt,
		Notes:              notes,
		ConsentGiven:       req.ConsentGiven,
		ConsentTextVersion: consentTextVersion,
		ConsentPurpose:     consentPurpose,
		CreatedBy:          row.CreatedBy.String(),
		CreatedAt:          row.CreatedAt.Time,
		UpdatedAt:          row.UpdatedAt.Time,
	}
	if req.ConsentGiven && consentAt.Valid {
		t := consentAt.Time
		resp.ConsentAt = &t
	}
	return resp, nil
}

// GetByID retrieves a donation by ID, decrypting the tax ID and returning the masked value.
//
// T-03-09: Response exposes DonorTaxIDMasked only (last-4 reveal via pii.MaskNationalID).
// The plaintext tax ID is decrypted in memory solely to compute the mask and is never returned.
// For authorised full-PII reveal (Checker/Admin), use the /pii endpoint (plan 03-05).
func (s *DonationService) GetByID(ctx context.Context, id string, claims auth.KeycloakClaims) (*DonationResponse, error) {
	var pgUUID pgtype.UUID
	if err := pgUUID.Scan(id); err != nil {
		return nil, fmt.Errorf("invalid donation ID: %w", err)
	}

	row, err := s.queries.GetDonationByID(ctx, pgUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get donation: %w", err)
	}

	// Decrypt only to produce the masked value — plaintext is zeroed by EncryptField's defer.
	plaintext, err := crypto.DecryptField(ctx, s.keyProvider, row.DonorTaxIDEnc, row.DonorTaxIDDek)
	if err != nil {
		return nil, fmt.Errorf("decrypt donor tax ID: %w", err)
	}
	maskedTaxID := pii.MaskNationalID(string(plaintext))

	return donationRowToResponse(row, maskedTaxID), nil
}

// UpdateDraft updates Maker-editable fields on a draft donation.
// Returns ErrInvalidTransition if the donation is not in draft status (FR-09).
// Implemented in Task 2 GREEN (03-03).
func (s *DonationService) UpdateDraft(ctx context.Context, id string, req UpdateDraftRequest, claims auth.KeycloakClaims) (*DonationResponse, error) {
	return nil, fmt.Errorf("UpdateDraft: not implemented — Task 2 GREEN (03-03)")
}

// Submit transitions a draft donation to pending_review status.
// Returns ErrInvalidTransition if the donation is not in draft status (FR-11, D-45).
// Implemented in Task 2 GREEN (03-03).
func (s *DonationService) Submit(ctx context.Context, id string, claims auth.KeycloakClaims) (*DonationResponse, error) {
	return nil, fmt.Errorf("Submit: not implemented — Task 2 GREEN (03-03)")
}

// List returns a paginated list of donations with masked PII.
// Full filter wiring (donor name, date range, status, receipt) implemented in plan 03-06.
// Implemented in Task 2 GREEN (03-03).
func (s *DonationService) List(ctx context.Context, filter ListFilter, claims auth.KeycloakClaims) ([]DonationResponse, error) {
	return nil, fmt.Errorf("List: not implemented — Task 2 GREEN (03-03)")
}

// --- Private helpers ---

// donationRowToResponse converts a full db.Donation row to a DonationResponse.
// maskedTaxID must be the result of pii.MaskNationalID — never pass ciphertext or plaintext (T-03-09).
func donationRowToResponse(row db.Donation, maskedTaxID string) *DonationResponse {
	resp := &DonationResponse{
		ID:                 row.ID.String(),
		Status:             string(row.Status),
		DonorName:          row.DonorName,
		DonorTaxIDMasked:   maskedTaxID,
		DonorAddress:       row.DonorAddress,
		DonorEmail:         row.DonorEmail,
		Amount:             numericStr(row.Amount),
		DonatedAt:          dateStr(row.DonatedAt),
		Notes:              row.Notes,
		ConsentGiven:       row.ConsentGiven,
		ConsentTextVersion: row.ConsentTextVersion,
		ConsentPurpose:     row.ConsentPurpose,
		CreatedBy:          row.CreatedBy.String(),
		CreatedAt:          row.CreatedAt.Time,
		UpdatedAt:          row.UpdatedAt.Time,
	}
	if row.ConsentAt.Valid {
		t := row.ConsentAt.Time
		resp.ConsentAt = &t
	}
	if row.SubmittedAt.Valid {
		t := row.SubmittedAt.Time
		resp.SubmittedAt = &t
	}
	return resp
}

// numericStr converts a pgtype.Numeric (big.Int + Exp) to a decimal string.
// Used for Amount fields in DonationResponse.
// Handles positive and negative amounts; treats invalid/nil as "0".
func numericStr(n pgtype.Numeric) string {
	if !n.Valid || n.Int == nil {
		return "0"
	}
	// *big.Int.Text(base) returns the string representation; no math/big import needed
	// since we only call a method on the existing *big.Int value (not constructing one).
	intStr := n.Int.Text(10)
	negative := false
	if len(intStr) > 0 && intStr[0] == '-' {
		negative = true
		intStr = intStr[1:]
	}
	var result string
	if n.Exp >= 0 {
		// Positive exponent: append trailing zeros.
		result = intStr + strings.Repeat("0", int(n.Exp))
	} else {
		// Negative exponent: insert decimal point.
		decPlaces := int(-n.Exp)
		for len(intStr) <= decPlaces {
			intStr = "0" + intStr // left-pad to accommodate the decimal
		}
		pos := len(intStr) - decPlaces
		result = intStr[:pos] + "." + intStr[pos:]
	}
	if negative {
		return "-" + result
	}
	return result
}

// dateStr converts a pgtype.Date to a "YYYY-MM-DD" string, or "" if invalid.
func dateStr(d pgtype.Date) string {
	if !d.Valid {
		return ""
	}
	return d.Time.Format("2006-01-02")
}
