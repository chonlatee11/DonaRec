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
	"encoding/json"
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
// This function is the single source of truth for the D-45 state machine (plan 03-03/03-05/03-06).
// All arms must be kept in sync with the DB CHECK constraints and the state diagram in CLAUDE.md.
//
//	draft ──submit──► pending_review ──approve──► issued ──cancel──► cancelled
//	             ◄──return──┘         ──reject──► rejected (terminal)
func canTransition(from db.DonationStatus, action string) bool {
	switch action {
	case "submit":
		// Only draft can be submitted.
		return from == db.DonationStatusDraft
	case "update":
		// Only draft can be edited (FR-09).
		return from == db.DonationStatusDraft
	case "approve", "return", "reject":
		// Checker actions: only pending_review is actionable (D-45).
		return from == db.DonationStatusPendingReview
	case "cancel":
		// Only issued records can be cancelled (FR-19, D-47 — wired in plan 03-06).
		return from == db.DonationStatusIssued
	default:
		return false
	}
}

// Identity resolution (bug: created-by-fk-mismatch): the Keycloak "sub" (claims.Subject)
// is NEVER equal to users.id (an independently DB-generated gen_random_uuid()). Columns
// that REFERENCES users(id) — donations.{created_by,reviewed_by,approved_by,cancelled_by}
// and slip_attachments.{uploaded_by,deleted_by} — must therefore be populated with the
// caller's already-resolved users.id, passed in as the explicit actingUserID parameter.
// Resolution now happens ONCE in auth.ResolveAppUser middleware (see cmd/server/main.go),
// not inside each service method.
//
// NOTE: audit_log.actor_id is intentionally NOT the resolved users.id — it has no FK to
// users(id) and stores the raw, immutable Keycloak sub (claims.Subject) so audit history
// survives user deactivation/deletion. Keep audit ActorID = claims.Subject.

// Create inserts a new donation record in 'draft' status with encrypted PII + consent snapshot.
//
// D-44: DonorTaxID is mandatory — ErrMissingTaxID is returned before any DB call if empty.
// T-03-08: EncryptField is called before any DB write — plaintext never reaches Postgres.
// D-49: consent fields (consent_given/at/text_version/purpose) are captured per-snapshot.
// Pattern C: logs only donation_id + created_by UUID — no PII fields ever logged.
func (s *DonationService) Create(ctx context.Context, req CreateDonationRequest, actingUserID pgtype.UUID, claims auth.KeycloakClaims) (*DonationDetailResponse, error) {
	// D-44: mandatory tax ID check — fail fast before any DB call.
	if req.DonorTaxID == "" {
		return nil, ErrMissingTaxID
	}

	donatedAtTime, err := time.ParseInLocation("2006-01-02", req.DonatedAt, time.UTC)
	if err != nil {
		return nil, fmt.Errorf("invalid donated_at %q: %w", req.DonatedAt, err)
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

	var fullRow db.Donation
	err = dbhelpers.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		qtx := s.queries.WithTx(tx)
		row, txErr := qtx.CreateDonation(ctx, db.CreateDonationParams{
			CreatedBy:          actingUserID,
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
		if txErr != nil {
			return txErr
		}
		// Re-fetch the full row (buildDetailResponse needs the full db.Donation shape —
		// CreateDonation's RETURNING only carries id/created_by/status/created_at/updated_at).
		var getErr error
		fullRow, getErr = qtx.GetDonationByID(ctx, row.ID)
		return getErr
	})
	if err != nil {
		return nil, fmt.Errorf("create donation: %w", err)
	}

	// Pattern C: log only donation_id + created_by — no donor name, tax ID, or email.
	s.logger.Info("donation created",
		zap.String("donation_id", fullRow.ID.String()),
		zap.String("created_by", fullRow.CreatedBy.String()),
	)

	return s.buildDetailResponse(ctx, fullRow, pii.MaskNationalID(req.DonorTaxID), claims)
}

// GetByID retrieves a donation by ID, decrypting the tax ID and returning the masked value.
//
// T-03-09: Response exposes DonorTaxIDMasked only (last-4 reveal via pii.MaskNationalID).
// The plaintext tax ID is decrypted in memory solely to compute the mask and is never returned.
// For authorised full-PII reveal (Checker/Admin), use the /pii endpoint (plan 03-05).
func (s *DonationService) GetByID(ctx context.Context, id string, claims auth.KeycloakClaims) (*DonationDetailResponse, error) {
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

	return s.buildDetailResponse(ctx, row, maskedTaxID, claims)
}

// UpdateDraft updates Maker-editable fields on a draft donation (FR-09).
//
// Uses LockDonationForUpdate within a transaction to atomically check the current
// status and apply the update. Returns ErrInvalidTransition if the donation is not
// in 'draft' status (state machine guard — D-45, T-03-13).
// Re-encrypts the tax ID whenever it is provided (T-03-08: always fresh EncryptField).
func (s *DonationService) UpdateDraft(ctx context.Context, id string, req UpdateDraftRequest, claims auth.KeycloakClaims) (*DonationDetailResponse, error) {
	if req.DonorTaxID == "" {
		return nil, ErrMissingTaxID
	}

	var pgUUID pgtype.UUID
	if err := pgUUID.Scan(id); err != nil {
		return nil, fmt.Errorf("invalid donation ID: %w", err)
	}

	donatedAtTime, err := time.ParseInLocation("2006-01-02", req.DonatedAt, time.UTC)
	if err != nil {
		return nil, fmt.Errorf("invalid donated_at %q: %w", req.DonatedAt, err)
	}

	// T-03-08: always re-encrypt — plaintext never persisted.
	encBytes, dekBytes, err := crypto.EncryptField(ctx, s.keyProvider, []byte(req.DonorTaxID))
	if err != nil {
		return nil, fmt.Errorf("encrypt donor tax ID: %w", err)
	}

	var amount pgtype.Numeric
	if err := amount.Scan(strconv.FormatFloat(req.Amount, 'f', 2, 64)); err != nil {
		return nil, fmt.Errorf("invalid amount: %w", err)
	}

	donatedAt := pgtype.Date{Time: donatedAtTime, Valid: true}
	retainUntil := pgtype.Date{Time: donatedAtTime.AddDate(10, 0, 0), Valid: true}

	var consentAt pgtype.Timestamptz
	if req.ConsentGiven {
		consentAt = pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	}

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

	var updatedRow db.Donation
	err = dbhelpers.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		qtx := s.queries.WithTx(tx)

		// Acquire row lock; check current status inside the same transaction.
		locked, lockErr := qtx.LockDonationForUpdate(ctx, pgUUID)
		if lockErr != nil {
			if errors.Is(lockErr, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return fmt.Errorf("lock donation: %w", lockErr)
		}
		// D-45 / T-03-13: only draft records can be edited.
		if !canTransition(locked.Status, "update") {
			return ErrInvalidTransition
		}

		if updateErr := qtx.UpdateDraftDonation(ctx, db.UpdateDraftDonationParams{
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
			ID:                 pgUUID,
		}); updateErr != nil {
			return fmt.Errorf("update draft: %w", updateErr)
		}

		var getErr error
		updatedRow, getErr = qtx.GetDonationByID(ctx, pgUUID)
		return getErr
	})
	if err != nil {
		return nil, err
	}

	// Pattern C: no PII in logs.
	s.logger.Info("donation draft updated",
		zap.String("donation_id", id),
		zap.String("created_by", updatedRow.CreatedBy.String()),
	)

	return s.buildDetailResponse(ctx, updatedRow, pii.MaskNationalID(req.DonorTaxID), claims)
}

// Submit transitions a draft donation to pending_review status (FR-11, D-45).
//
// Uses LockDonationForUpdate within a transaction to atomically check the current
// status and apply the transition. Returns ErrInvalidTransition if not in 'draft'.
// submitted_at is set by the SubmitDonation query (DEFAULT now()).
func (s *DonationService) Submit(ctx context.Context, id string, claims auth.KeycloakClaims) (*DonationDetailResponse, error) {
	var pgUUID pgtype.UUID
	if err := pgUUID.Scan(id); err != nil {
		return nil, fmt.Errorf("invalid donation ID: %w", err)
	}

	var submittedRow db.Donation
	err := dbhelpers.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		qtx := s.queries.WithTx(tx)

		// Acquire row lock to serialize concurrent submit attempts.
		locked, lockErr := qtx.LockDonationForUpdate(ctx, pgUUID)
		if lockErr != nil {
			if errors.Is(lockErr, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return fmt.Errorf("lock donation: %w", lockErr)
		}
		// D-45: only draft can be submitted.
		if !canTransition(locked.Status, "submit") {
			return ErrInvalidTransition
		}

		if submitErr := qtx.SubmitDonation(ctx, pgUUID); submitErr != nil {
			return fmt.Errorf("submit donation: %w", submitErr)
		}

		var getErr error
		submittedRow, getErr = qtx.GetDonationByID(ctx, pgUUID)
		return getErr
	})
	if err != nil {
		return nil, err
	}

	// Pattern C: log submitted_by (actor) + donation_id only — no donor details.
	s.logger.Info("donation submitted",
		zap.String("donation_id", id),
		zap.String("submitted_by", claims.Subject),
	)

	// Decrypt to build the masked value for the response (T-03-09).
	plaintext, decErr := crypto.DecryptField(ctx, s.keyProvider, submittedRow.DonorTaxIDEnc, submittedRow.DonorTaxIDDek)
	if decErr != nil {
		return nil, fmt.Errorf("decrypt donor tax ID: %w", decErr)
	}

	return s.buildDetailResponse(ctx, submittedRow, pii.MaskNationalID(string(plaintext)), claims)
}

// List returns a paginated, masked list of donations ordered by created_at DESC.
//
// Basic implementation for plan 03-03: returns unfiltered results with PII masked.
// Full filter wiring (donor name ILIKE, date range, status, receipt number) is
// implemented in plan 03-06 using the SearchDonations sqlc query.
func (s *DonationService) List(ctx context.Context, filter ListFilter, claims auth.KeycloakClaims) ([]DonationResponse, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	// Raw query: includes PII columns for decrypt+mask per row (03-06 will use SearchDonations).
	rows, queryErr := s.pool.Query(ctx, `
		SELECT id, status, donor_name, donor_address, donor_email,
		       donor_tax_id_enc, donor_tax_id_dek, amount, donated_at,
		       notes, consent_given, consent_at, consent_text_version, consent_purpose,
		       created_by, created_at, updated_at, submitted_at
		FROM donations
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2`, limit, offset)
	if queryErr != nil {
		return nil, fmt.Errorf("list donations: %w", queryErr)
	}
	defer rows.Close()

	var result []DonationResponse
	for rows.Next() {
		var (
			id                 pgtype.UUID
			status             db.DonationStatus
			donorName          string
			donorAddress       string
			donorEmail         *string
			encBytes, dekBytes []byte
			amount             pgtype.Numeric
			donatedAt          pgtype.Date
			notes              *string
			consentGiven       bool
			consentAt          pgtype.Timestamptz
			consentTextVersion *string
			consentPurpose     *string
			createdBy          pgtype.UUID
			createdAt          pgtype.Timestamptz
			updatedAt          pgtype.Timestamptz
			submittedAt        pgtype.Timestamptz
		)
		if scanErr := rows.Scan(
			&id, &status, &donorName, &donorAddress, &donorEmail,
			&encBytes, &dekBytes, &amount, &donatedAt, &notes,
			&consentGiven, &consentAt, &consentTextVersion, &consentPurpose,
			&createdBy, &createdAt, &updatedAt, &submittedAt,
		); scanErr != nil {
			return nil, fmt.Errorf("scan donation row: %w", scanErr)
		}

		// T-03-09: decrypt only to produce the mask — never return plaintext.
		plaintext, decErr := crypto.DecryptField(ctx, s.keyProvider, encBytes, dekBytes)
		if decErr != nil {
			return nil, fmt.Errorf("decrypt donor tax ID: %w", decErr)
		}

		resp := DonationResponse{
			ID:                 id.String(),
			Status:             string(status),
			DonorName:          donorName,
			DonorTaxIDMasked:   pii.MaskNationalID(string(plaintext)),
			DonorAddress:       donorAddress,
			DonorEmail:         donorEmail,
			Amount:             numericStr(amount),
			DonatedAt:          dateStr(donatedAt),
			Notes:              notes,
			ConsentGiven:       consentGiven,
			ConsentTextVersion: consentTextVersion,
			ConsentPurpose:     consentPurpose,
			CreatedBy:          createdBy.String(),
			CreatedAt:          createdAt.Time,
			UpdatedAt:          updatedAt.Time,
		}
		if consentAt.Valid {
			t := consentAt.Time
			resp.ConsentAt = &t
		}
		if submittedAt.Valid {
			t := submittedAt.Time
			resp.SubmittedAt = &t
		}
		result = append(result, resp)
	}
	return result, rows.Err()
}

// Approve is the load-bearing issuance transaction (FR-14, D-52, plan 03-05).
//
// Inside ONE db.WithTx closure the method:
//  1. Locks the donation row FOR UPDATE (D-52) to serialize concurrent approvals.
//  2. Checks status == pending_review — ErrInvalidTransition otherwise.
//  3. Enforces SoD: approverID != donation.CreatedBy — ErrSoDViolation otherwise.
//  4. Calls s.allocator.Allocate(ctx, tx, ...) — THE ONLY call site (D-35/D-33).
//  5. Calls qtx.IssueDonation — stamps status=issued + receipt fields.
//  6. Calls s.auditSvc.AppendAuditEntryTx — audit in-tx, NOT best-effort (NFR-05/Pitfall 4).
//  7. Calls qtx.EnqueueOutboxJob — outbox INSERT atomically linked (Phase 4 consumes).
//
// Any error causes WithTx to roll back ALL seven effects.
// PDF render and email send are NOT performed here — only the outbox job is enqueued.
func (s *DonationService) Approve(ctx context.Context, id string, actingUserID pgtype.UUID, claims auth.KeycloakClaims) (*DonationDetailResponse, error) {
	var pgUUID pgtype.UUID
	if err := pgUUID.Scan(id); err != nil {
		return nil, fmt.Errorf("invalid donation ID: %w", err)
	}

	approvedAt := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}

	var issuedRow db.Donation
	err := dbhelpers.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		qtx := s.queries.WithTx(tx)

		// Step 1: Lock the donation row FOR UPDATE — serializes concurrent approvals (D-52).
		// LockDonationForUpdate returns ErrNoRows if the donation does not exist.
		locked, lockErr := qtx.LockDonationForUpdate(ctx, pgUUID)
		if lockErr != nil {
			if errors.Is(lockErr, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return fmt.Errorf("lock donation: %w", lockErr)
		}

		// Step 2: Status precondition — only pending_review may be approved (D-45).
		// Primary guard; IssueDonation SQL also has WHERE status='pending_review' as backstop.
		if !canTransition(locked.Status, "approve") {
			return ErrInvalidTransition
		}

		// Step 3: Segregation of Duties — approver must not be the record's creator (FR-14).
		// Both the UUID bytes and the Valid flag are compared so a NULL created_by never
		// accidentally passes the check.
		if locked.CreatedBy == actingUserID {
			return ErrSoDViolation
		}

		// Step 4: Allocate gap-less receipt number within this transaction.
		// IMPORTANT: pass the closure's tx, NOT the pool (D-33/D-35).
		// This is the SOLE call site of Allocate in the entire codebase (D-35).
		receipt, allocErr := s.allocator.Allocate(ctx, tx, approvedAt.Time)
		if allocErr != nil {
			return fmt.Errorf("allocate receipt number: %w", allocErr)
		}

		// Step 5: Stamp status=issued + receipt fields on the donation row.
		// receipt.ID  = receipt_numbers ledger PK → donations.receipt_number_id FK (D-38).
		// receipt.Formatted = frozen formatted string — never recomputed after this (D-42).
		receiptID := receipt.ID
		formatted := receipt.Formatted
		if issueErr := qtx.IssueDonation(ctx, db.IssueDonationParams{
			ApprovedBy:       actingUserID,
			ApprovedAt:       approvedAt,
			ReceiptNumberID:  &receiptID,
			ReceiptFormatted: &formatted,
			ID:               pgUUID,
		}); issueErr != nil {
			return fmt.Errorf("issue donation: %w", issueErr)
		}

		// Step 6: Append audit entry inside this transaction (NFR-05).
		// Must NOT be best-effort (Pitfall 4): failure here rolls back the entire issuance.
		// AppendAuditEntryTx acquires pg_advisory_xact_lock internally — no extra locking needed.
		afterJSON, _ := json.Marshal(map[string]any{
			"receipt_formatted": receipt.Formatted,
			"status":            "issued",
		})
		if auditErr := s.auditSvc.AppendAuditEntryTx(ctx, tx, audit.AuditEntry{
			ActorID:    claims.Subject,
			ActorEmail: claims.ActorIdentity(),
			Action:     "donation.approve",
			Resource:   "/api/donations/" + id + "/approve",
			AfterJSON:  afterJSON,
		}); auditErr != nil {
			return fmt.Errorf("audit approve: %w", auditErr)
		}

		// Step 7: Enqueue outbox job — atomically linked with the issuance (Phase 4 consumes).
		// Do NOT render PDF or send email here; that would hold the row lock too long (NFR-07).
		payload, _ := json.Marshal(map[string]string{"donation_id": id})
		if outboxErr := qtx.EnqueueOutboxJob(ctx, db.EnqueueOutboxJobParams{
			JobType: "issue_receipt",
			Payload: payload,
		}); outboxErr != nil {
			return fmt.Errorf("enqueue outbox: %w", outboxErr)
		}

		// Fetch the updated row inside the transaction so the response reflects committed state.
		var getErr error
		issuedRow, getErr = qtx.GetDonationByID(ctx, pgUUID)
		return getErr
	})
	if err != nil {
		return nil, err
	}

	// Decrypt only to produce the masked value for the response (T-03-09).
	plaintext, decErr := crypto.DecryptField(ctx, s.keyProvider, issuedRow.DonorTaxIDEnc, issuedRow.DonorTaxIDDek)
	if decErr != nil {
		return nil, fmt.Errorf("decrypt donor tax ID: %w", decErr)
	}

	// Pattern C: log operation + IDs only — no PII in logs (T-03-10).
	s.logger.Info("donation approved",
		zap.String("donation_id", id),
		zap.String("approved_by", claims.Subject),
	)

	return s.buildDetailResponse(ctx, issuedRow, pii.MaskNationalID(string(plaintext)), claims)
}

// Return transitions a pending_review donation back to draft so the Maker can correct it (D-45, FR-12).
//
// reason is mandatory — returns ErrMissingReason before any DB call if empty/whitespace.
// Uses LockDonationForUpdate + status precondition to serialize concurrent reviewer attempts.
// AppendAuditEntryTx records the action in the same transaction (NFR-05).
func (s *DonationService) Return(ctx context.Context, id string, reason string, actingUserID pgtype.UUID, claims auth.KeycloakClaims) (*DonationDetailResponse, error) {
	// Mandatory reason check — early exit before any DB call.
	if strings.TrimSpace(reason) == "" {
		return nil, ErrMissingReason
	}

	var pgUUID pgtype.UUID
	if err := pgUUID.Scan(id); err != nil {
		return nil, fmt.Errorf("invalid donation ID: %w", err)
	}

	reviewedAt := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}

	var returnedRow db.Donation
	err := dbhelpers.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		qtx := s.queries.WithTx(tx)

		locked, lockErr := qtx.LockDonationForUpdate(ctx, pgUUID)
		if lockErr != nil {
			if errors.Is(lockErr, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return fmt.Errorf("lock donation: %w", lockErr)
		}

		// Only pending_review can be returned to draft (D-45).
		if !canTransition(locked.Status, "return") {
			return ErrInvalidTransition
		}

		if err := qtx.ReturnDonation(ctx, db.ReturnDonationParams{
			ReviewedBy:   actingUserID,
			ReviewedAt:   reviewedAt,
			ReviewReason: &reason,
			ID:           pgUUID,
		}); err != nil {
			return fmt.Errorf("return donation: %w", err)
		}

		afterJSON, _ := json.Marshal(map[string]any{
			"status":        "draft",
			"review_reason": reason,
		})
		if auditErr := s.auditSvc.AppendAuditEntryTx(ctx, tx, audit.AuditEntry{
			ActorID:    claims.Subject,
			ActorEmail: claims.ActorIdentity(),
			Action:     "donation.return",
			Resource:   "/api/donations/" + id + "/return",
			AfterJSON:  afterJSON,
		}); auditErr != nil {
			return fmt.Errorf("audit return: %w", auditErr)
		}

		var getErr error
		returnedRow, getErr = qtx.GetDonationByID(ctx, pgUUID)
		return getErr
	})
	if err != nil {
		return nil, err
	}

	plaintext, decErr := crypto.DecryptField(ctx, s.keyProvider, returnedRow.DonorTaxIDEnc, returnedRow.DonorTaxIDDek)
	if decErr != nil {
		return nil, fmt.Errorf("decrypt donor tax ID: %w", decErr)
	}

	// Pattern C: no PII in logs.
	s.logger.Info("donation returned to draft",
		zap.String("donation_id", id),
		zap.String("reviewed_by", claims.Subject),
	)

	return s.buildDetailResponse(ctx, returnedRow, pii.MaskNationalID(string(plaintext)), claims)
}

// Reject permanently rejects a pending_review donation (D-45, FR-12).
// 'rejected' is a terminal state — no further transitions are allowed.
//
// reason is mandatory — returns ErrMissingReason before any DB call if empty/whitespace.
func (s *DonationService) Reject(ctx context.Context, id string, reason string, actingUserID pgtype.UUID, claims auth.KeycloakClaims) (*DonationDetailResponse, error) {
	// Mandatory reason check — early exit before any DB call.
	if strings.TrimSpace(reason) == "" {
		return nil, ErrMissingReason
	}

	var pgUUID pgtype.UUID
	if err := pgUUID.Scan(id); err != nil {
		return nil, fmt.Errorf("invalid donation ID: %w", err)
	}

	reviewedAt := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}

	var rejectedRow db.Donation
	err := dbhelpers.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		qtx := s.queries.WithTx(tx)

		locked, lockErr := qtx.LockDonationForUpdate(ctx, pgUUID)
		if lockErr != nil {
			if errors.Is(lockErr, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return fmt.Errorf("lock donation: %w", lockErr)
		}

		// Only pending_review can be rejected (D-45, terminal state).
		if !canTransition(locked.Status, "reject") {
			return ErrInvalidTransition
		}

		if err := qtx.RejectDonation(ctx, db.RejectDonationParams{
			ReviewedBy:   actingUserID,
			ReviewedAt:   reviewedAt,
			ReviewReason: &reason,
			ID:           pgUUID,
		}); err != nil {
			return fmt.Errorf("reject donation: %w", err)
		}

		afterJSON, _ := json.Marshal(map[string]any{
			"status":        "rejected",
			"review_reason": reason,
		})
		if auditErr := s.auditSvc.AppendAuditEntryTx(ctx, tx, audit.AuditEntry{
			ActorID:    claims.Subject,
			ActorEmail: claims.ActorIdentity(),
			Action:     "donation.reject",
			Resource:   "/api/donations/" + id + "/reject",
			AfterJSON:  afterJSON,
		}); auditErr != nil {
			return fmt.Errorf("audit reject: %w", auditErr)
		}

		var getErr error
		rejectedRow, getErr = qtx.GetDonationByID(ctx, pgUUID)
		return getErr
	})
	if err != nil {
		return nil, err
	}

	plaintext, decErr := crypto.DecryptField(ctx, s.keyProvider, rejectedRow.DonorTaxIDEnc, rejectedRow.DonorTaxIDDek)
	if decErr != nil {
		return nil, fmt.Errorf("decrypt donor tax ID: %w", decErr)
	}

	// Pattern C: no PII in logs.
	s.logger.Info("donation rejected",
		zap.String("donation_id", id),
		zap.String("reviewed_by", claims.Subject),
	)

	return s.buildDetailResponse(ctx, rejectedRow, pii.MaskNationalID(string(plaintext)), claims)
}

// Cancel voids an issued receipt: issued → cancelled (FR-19, D-47, plan 03-06).
//
// Authorization (D-47): Checker and Admin only — Maker is forbidden.
// Reason is mandatory (ErrMissingReason if empty/whitespace).
// If edonation_keyed=true on the record, RDConfirmationReason must be non-empty (D-51, T-03-25).
// The receipt_number_id and receipt_formatted are NEVER nulled out — the number is retained
// to avoid gaps in the sequential series (FR-19, load-bearing invariant Cancel#1).
//
// All effects (status update + audit) are committed atomically inside WithTx.
// Pattern C: only donation_id + cancelled_by logged — no PII in logs.
func (s *DonationService) Cancel(ctx context.Context, id string, req CancelDonationRequest, actingUserID pgtype.UUID, claims auth.KeycloakClaims) (*DonationDetailResponse, error) {
	// D-47: Checker/Admin only — Maker cannot cancel.
	if !claims.HasRole(auth.RoleChecker) && !claims.HasRole(auth.RoleAdmin) {
		return nil, ErrForbidden
	}

	// Mandatory reason check — early exit before any DB call.
	if strings.TrimSpace(req.Reason) == "" {
		return nil, ErrMissingReason
	}

	var pgUUID pgtype.UUID
	if err := pgUUID.Scan(id); err != nil {
		return nil, fmt.Errorf("invalid donation ID: %w", err)
	}

	cancelledAt := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}

	var cancelledRow db.Donation
	err := dbhelpers.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		qtx := s.queries.WithTx(tx)

		// Lock the row to serialize concurrent cancel attempts.
		locked, lockErr := qtx.LockDonationForUpdate(ctx, pgUUID)
		if lockErr != nil {
			if errors.Is(lockErr, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return fmt.Errorf("lock donation: %w", lockErr)
		}

		// D-45: only issued records can be cancelled (FR-19).
		if !canTransition(locked.Status, "cancel") {
			return ErrInvalidTransition
		}

		// D-51: if already keyed into e-Donation, require an explicit RD reconciliation reason.
		// This prevents accidental gaps in the RD-system records without a documented explanation.
		if locked.EdonationKeyed && strings.TrimSpace(req.RDConfirmationReason) == "" {
			return ErrEDonationKeyedCancel
		}

		reason := req.Reason
		if err := qtx.CancelDonation(ctx, db.CancelDonationParams{
			CancelledBy:  actingUserID,
			CancelledAt:  cancelledAt,
			CancelReason: &reason,
			ID:           pgUUID,
		}); err != nil {
			return fmt.Errorf("cancel donation: %w", err)
		}

		// Audit inside tx — failure rolls back cancel (NFR-05, Pitfall 4).
		afterMap := map[string]any{
			"status":        "cancelled",
			"cancel_reason": reason,
		}
		if req.RDConfirmationReason != "" {
			afterMap["rd_confirmation_reason"] = req.RDConfirmationReason
		}
		afterJSON, _ := json.Marshal(afterMap)
		if auditErr := s.auditSvc.AppendAuditEntryTx(ctx, tx, audit.AuditEntry{
			ActorID:    claims.Subject,
			ActorEmail: claims.ActorIdentity(),
			Action:     "donation.cancel",
			Resource:   "/api/donations/" + id + "/cancel",
			AfterJSON:  afterJSON,
		}); auditErr != nil {
			return fmt.Errorf("audit cancel: %w", auditErr)
		}

		var getErr error
		cancelledRow, getErr = qtx.GetDonationByID(ctx, pgUUID)
		return getErr
	})
	if err != nil {
		return nil, err
	}

	// Decrypt only to produce the masked value for the response (T-03-09).
	plaintext, decErr := crypto.DecryptField(ctx, s.keyProvider, cancelledRow.DonorTaxIDEnc, cancelledRow.DonorTaxIDDek)
	if decErr != nil {
		return nil, fmt.Errorf("decrypt donor tax ID: %w", decErr)
	}

	// Pattern C: log donation_id + cancelled_by only — no PII.
	s.logger.Info("donation cancelled",
		zap.String("donation_id", id),
		zap.String("cancelled_by", claims.Subject),
	)

	return s.buildDetailResponse(ctx, cancelledRow, pii.MaskNationalID(string(plaintext)), claims)
}

// Reissue performs Void & Reissue (D-50): cancels an issued receipt and creates a corrected draft.
//
// Inside ONE WithTx closure:
//  1. Performs Cancel guards (Checker/Admin, reason, e-Donation keyed confirmation).
//  2. CancelDonation on the original (sets status=cancelled, retains receipt_number_id — no gap).
//  3. Creates a NEW donation at status='draft' with corrected data (re-encrypt tax ID).
//  4. Sets original.replaced_by = newID (SetReplacedBy).
//  5. Sets new.replaces = originalID (SetReplaces).
//  6. Appends audit entry "donation.reissue".
//
// CRITICAL (D-50): the new draft does NOT get a receipt number here.
// It must go through the normal Submit → Approve path (plan 03-05) to earn a fresh number.
// This preserves Maker-Checker SoD and gap-less numbering — no bypass is allowed.
func (s *DonationService) Reissue(ctx context.Context, originalID string, req ReissueDonationRequest, actingUserID pgtype.UUID, claims auth.KeycloakClaims) (*DonationDetailResponse, error) {
	// D-47: Checker/Admin only.
	if !claims.HasRole(auth.RoleChecker) && !claims.HasRole(auth.RoleAdmin) {
		return nil, ErrForbidden
	}

	// Mandatory reason check.
	if strings.TrimSpace(req.Reason) == "" {
		return nil, ErrMissingReason
	}

	if req.DonorTaxID == "" {
		return nil, ErrMissingTaxID
	}

	var origUUID pgtype.UUID
	if err := origUUID.Scan(originalID); err != nil {
		return nil, fmt.Errorf("invalid original donation ID: %w", err)
	}

	cancelledAt := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}

	// Encrypt the corrected tax ID before the transaction begins.
	encBytes, dekBytes, encErr := crypto.EncryptField(ctx, s.keyProvider, []byte(req.DonorTaxID))
	if encErr != nil {
		return nil, fmt.Errorf("encrypt donor tax ID: %w", encErr)
	}

	donatedAtTime, err := time.ParseInLocation("2006-01-02", req.DonatedAt, time.UTC)
	if err != nil {
		return nil, fmt.Errorf("invalid donated_at %q: %w", req.DonatedAt, err)
	}

	var amount pgtype.Numeric
	if err := amount.Scan(strconv.FormatFloat(req.Amount, 'f', 2, 64)); err != nil {
		return nil, fmt.Errorf("invalid amount: %w", err)
	}
	donatedAt := pgtype.Date{Time: donatedAtTime, Valid: true}
	retainUntil := pgtype.Date{Time: donatedAtTime.AddDate(10, 0, 0), Valid: true}

	var consentAt pgtype.Timestamptz
	if req.ConsentGiven {
		consentAt = pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	}

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

	var newRow db.CreateDonationRow
	var newFullRow db.Donation
	err = dbhelpers.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		qtx := s.queries.WithTx(tx)

		// Step 1: Lock and validate the original record.
		locked, lockErr := qtx.LockDonationForUpdate(ctx, origUUID)
		if lockErr != nil {
			if errors.Is(lockErr, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return fmt.Errorf("lock original donation: %w", lockErr)
		}

		// Only issued records can be voided-and-reissued.
		if !canTransition(locked.Status, "cancel") {
			return ErrInvalidTransition
		}

		// D-51: e-Donation keyed confirmation required if flagged.
		if locked.EdonationKeyed && strings.TrimSpace(req.RDConfirmationReason) == "" {
			return ErrEDonationKeyedCancel
		}

		// Step 2: Cancel the original (keeps receipt_number_id — no gap).
		reason := req.Reason
		if err := qtx.CancelDonation(ctx, db.CancelDonationParams{
			CancelledBy:  actingUserID,
			CancelledAt:  cancelledAt,
			CancelReason: &reason,
			ID:           origUUID,
		}); err != nil {
			return fmt.Errorf("cancel original donation: %w", err)
		}

		// Step 3: Create replacement draft at status='draft' with corrected data.
		var txErr error
		newRow, txErr = qtx.CreateDonation(ctx, db.CreateDonationParams{
			CreatedBy:          actingUserID,
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
		if txErr != nil {
			return fmt.Errorf("create replacement draft: %w", txErr)
		}

		// Step 4: Set original.replaced_by = newID.
		if err := qtx.SetReplacedBy(ctx, db.SetReplacedByParams{
			ReplacedBy: newRow.ID,
			ID:         origUUID,
		}); err != nil {
			return fmt.Errorf("set replaced_by on original: %w", err)
		}

		// Step 5: Set new.replaces = originalID.
		if err := qtx.SetReplaces(ctx, db.SetReplacesParams{
			Replaces: origUUID,
			ID:       newRow.ID,
		}); err != nil {
			return fmt.Errorf("set replaces on new draft: %w", err)
		}

		// Re-fetch the full replacement row (buildDetailResponse needs the full db.Donation
		// shape, including the replaces pointer just set above).
		var getErr error
		newFullRow, getErr = qtx.GetDonationByID(ctx, newRow.ID)
		if getErr != nil {
			return fmt.Errorf("get replacement donation: %w", getErr)
		}

		// Step 6: Audit the reissue action.
		afterMap := map[string]any{
			"action":         "donation.reissue",
			"original_id":    originalID,
			"replacement_id": newRow.ID.String(),
			"cancel_reason":  reason,
		}
		if req.RDConfirmationReason != "" {
			afterMap["rd_confirmation_reason"] = req.RDConfirmationReason
		}
		afterJSON, _ := json.Marshal(afterMap)
		if auditErr := s.auditSvc.AppendAuditEntryTx(ctx, tx, audit.AuditEntry{
			ActorID:    claims.Subject,
			ActorEmail: claims.ActorIdentity(),
			Action:     "donation.reissue",
			Resource:   "/api/donations/" + originalID + "/reissue",
			AfterJSON:  afterJSON,
		}); auditErr != nil {
			return fmt.Errorf("audit reissue: %w", auditErr)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Pattern C: log IDs only — no PII.
	s.logger.Info("donation reissued",
		zap.String("original_id", originalID),
		zap.String("replacement_id", newRow.ID.String()),
		zap.String("reissued_by", claims.Subject),
	)

	// Tax ID masked for the response — plaintext never returned from service (T-03-09).
	return s.buildDetailResponse(ctx, newFullRow, pii.MaskNationalID(req.DonorTaxID), claims)
}

// RevealPII decrypts and returns the full plaintext donor tax/national ID (D-46, T-03-26).
//
// Authorization gate: Checker and Admin only (CanRevealFull). Maker → ErrForbidden (403).
// Every authorized reveal MUST be audited (action="pii.reveal") BEFORE returning plaintext (D-13).
// The audit write is inside WithTx so a failure rolls back — plaintext is never returned
// without a committed audit entry (NFR-05).
//
// Pattern C: donor_id is logged, plaintext tax ID is NOT logged (T-03-10).
func (s *DonationService) RevealPII(ctx context.Context, id string, claims auth.KeycloakClaims) (*PIIRevealResponse, error) {
	// D-46: Checker/Admin gate — reject before any DB call.
	if !pii.CanRevealFull(claims) {
		return nil, ErrForbidden
	}

	var pgUUID pgtype.UUID
	if err := pgUUID.Scan(id); err != nil {
		return nil, fmt.Errorf("invalid donation ID: %w", err)
	}

	var plaintext []byte
	err := dbhelpers.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		qtx := s.queries.WithTx(tx)

		row, rowErr := qtx.GetDonationByID(ctx, pgUUID)
		if rowErr != nil {
			if errors.Is(rowErr, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return fmt.Errorf("get donation: %w", rowErr)
		}

		var decErr error
		plaintext, decErr = crypto.DecryptField(ctx, s.keyProvider, row.DonorTaxIDEnc, row.DonorTaxIDDek)
		if decErr != nil {
			return fmt.Errorf("decrypt donor tax ID: %w", decErr)
		}

		// D-13: audit BEFORE returning plaintext — failure rolls back (audit integrity).
		afterJSON, _ := json.Marshal(map[string]any{
			"action":      "pii.reveal",
			"donation_id": id,
		})
		if auditErr := s.auditSvc.AppendAuditEntryTx(ctx, tx, audit.AuditEntry{
			ActorID:    claims.Subject,
			ActorEmail: claims.ActorIdentity(),
			Action:     "pii.reveal",
			Resource:   "/api/donations/" + id + "/pii",
			AfterJSON:  afterJSON,
		}); auditErr != nil {
			return fmt.Errorf("audit pii reveal: %w", auditErr)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Pattern C: log donation_id + actor only — never log plaintext tax ID.
	s.logger.Info("pii revealed",
		zap.String("donation_id", id),
		zap.String("revealed_by", claims.Subject),
	)

	return &PIIRevealResponse{
		DonationID:          id,
		DonorTaxIDPlaintext: string(plaintext),
	}, nil
}

// Search returns a paginated, PII-free list of donations filtered by optional criteria
// (FR-10, D-53, D-R2). total is a real COUNT over the SAME filter predicate as the page
// query (via CountDonations) — it is NEVER derived from len(items), since a page only
// contains up to filter.Limit rows (T-09-03 mitigation).
//
// Supported filters: donor_name (ILIKE), status, from_date, to_date, receipt_no.
// Tax ID is intentionally excluded as a filter parameter (D-53, T-03-29).
// Results use SearchDonations which excludes PII ciphertext columns (least-privilege);
// each row is enriched with the creator's display name via a LEFT JOIN to users.
func (s *DonationService) Search(ctx context.Context, filter ListFilter, claims auth.KeycloakClaims) ([]DonationListItem, int64, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	// Build nullable filter values shared by SearchDonations and CountDonations — nil
	// means "skip this filter" (D-53, Pattern F). Both queries MUST see identical filter
	// values so total always matches the same predicate as the page (D-R2).
	var donorName *string
	var status *db.DonationStatus
	var fromDate, toDate pgtype.Date
	var receiptNo *string

	if filter.DonorName != nil {
		donorName = filter.DonorName
	}
	if filter.Status != nil {
		st := db.DonationStatus(*filter.Status)
		status = &st
	}
	if filter.FromDate != nil {
		fromDate = pgtype.Date{Time: *filter.FromDate, Valid: true}
	}
	if filter.ToDate != nil {
		toDate = pgtype.Date{Time: *filter.ToDate, Valid: true}
	}
	if filter.ReceiptNo != nil {
		receiptNo = filter.ReceiptNo
	}

	rows, queryErr := s.queries.SearchDonations(ctx, db.SearchDonationsParams{
		LimitN:    limit,
		OffsetN:   offset,
		DonorName: donorName,
		Status:    status,
		FromDate:  fromDate,
		ToDate:    toDate,
		ReceiptNo: receiptNo,
	})
	if queryErr != nil {
		return nil, 0, fmt.Errorf("search donations: %w", queryErr)
	}

	total, countErr := s.queries.CountDonations(ctx, db.CountDonationsParams{
		DonorName: donorName,
		Status:    status,
		FromDate:  fromDate,
		ToDate:    toDate,
		ReceiptNo: receiptNo,
	})
	if countErr != nil {
		return nil, 0, fmt.Errorf("count donations: %w", countErr)
	}

	result := make([]DonationListItem, 0, len(rows))
	for _, row := range rows {
		createdByName := ""
		if row.CreatedByName != nil {
			createdByName = *row.CreatedByName
		}
		result = append(result, DonationListItem{
			ID:               row.ID.String(),
			Status:           string(row.Status),
			DonorName:        row.DonorName,
			Amount:           numericStr(row.Amount),
			DonatedAt:        dateStr(row.DonatedAt),
			ReceiptFormatted: row.ReceiptFormatted,
			CreatedBy:        createdByName,
			CreatedByID:      row.CreatedBy.String(),
		})
	}
	return result, total, nil
}

// --- Private helpers ---

// reviewActionLabel normalizes an audit_log action string ("donation.return" /
// "donation.reject") to the short label the FE contract expects ("return" / "reject").
// Any other value passes through unchanged (defensive — should not occur given the
// action IN (...) filter in GetDonationReviewHistory).
func reviewActionLabel(auditAction string) string {
	switch auditAction {
	case "donation.return":
		return "return"
	case "donation.reject":
		return "reject"
	default:
		return auditAction
	}
}

// buildDetailResponse converts a full db.Donation row into the FE-aligned
// DonationDetailResponse (D-R3 detail contract) — the single shared builder used by
// GetByID AND every mutation (Create/UpdateDraft/Submit/Approve/Return/Reject/Cancel/
// Reissue), so enriching this one function aligns every screen at once (T-03-31).
//
// maskedTaxID must be the result of pii.MaskNationalID — never pass ciphertext or
// plaintext (T-03-09, T-11-02).
//
// Server-computed auth flags (T-03-31, T-11-01, T-11-03): the viewer's identity is
// resolved from claims.Subject to their internal users.id via GetUserByKeycloakSubject
// — NEVER compared directly against claims.Subject (created-by-fk-mismatch class of bug).
// These flags are UI hints only; every mutation independently re-enforces SoD/RBAC.
func (s *DonationService) buildDetailResponse(ctx context.Context, row db.Donation, maskedTaxID string, claims auth.KeycloakClaims) (*DonationDetailResponse, error) {
	// Resolve the viewer's internal users.id from their Keycloak subject. A caller whose
	// subject has no provisioned users row (should not happen on the real HTTP path —
	// auth.ResolveAppUser middleware already 403s unprovisioned subjects before the
	// handler runs) is defensively treated as "not the creator" rather than erroring.
	viewerIsCreator := false
	if viewerRow, err := s.queries.GetUserByKeycloakSubject(ctx, claims.Subject); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("resolve viewer user: %w", err)
		}
	} else {
		viewerIsCreator = viewerRow.ID == row.CreatedBy
	}

	isReviewer := claims.HasRole(auth.RoleChecker) || claims.HasRole(auth.RoleAdmin)
	isPendingReview := row.Status == db.DonationStatusPendingReview
	// SoD-aware UI hint (T-11-01): a reviewer may not approve/return/reject their own record.
	canReview := isReviewer && isPendingReview && !viewerIsCreator

	createdByName := ""
	if name, err := s.queries.GetUserDisplayName(ctx, row.CreatedBy); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("resolve creator display name: %w", err)
		}
	} else {
		createdByName = name
	}

	replaces, err := s.expandReceiptRef(ctx, row.Replaces)
	if err != nil {
		return nil, fmt.Errorf("resolve replaces receipt ref: %w", err)
	}
	replacedBy, err := s.expandReceiptRef(ctx, row.ReplacedBy)
	if err != nil {
		return nil, fmt.Errorf("resolve replaced_by receipt ref: %w", err)
	}

	historyRows, err := s.queries.GetDonationReviewHistory(ctx, row.ID.String())
	if err != nil {
		return nil, fmt.Errorf("get donation review history: %w", err)
	}
	reviewHistory := make([]ReviewHistoryEntry, 0, len(historyRows))
	for _, h := range historyRows {
		reviewHistory = append(reviewHistory, ReviewHistoryEntry{
			ID:        h.ID,
			Action:    reviewActionLabel(h.Action),
			Reason:    h.Reason,
			ActorName: h.ActorName,
			ActedAt:   h.ActedAt.Time,
		})
	}

	resp := &DonationDetailResponse{
		ID:                  row.ID.String(),
		Status:              string(row.Status),
		DonorName:           row.DonorName,
		NationalIDMasked:    maskedTaxID,
		Address:             row.DonorAddress,
		Email:               row.DonorEmail,
		Amount:              numericStr(row.Amount),
		DonatedAt:           dateStr(row.DonatedAt),
		Note:                row.Notes,
		ConsentGiven:        row.ConsentGiven,
		ConsentTextVersion:  row.ConsentTextVersion,
		ConsentPurpose:      row.ConsentPurpose,
		ReviewReason:        row.ReviewReason,
		ReceiptFormatted:    row.ReceiptFormatted,
		CreatedBy:           createdByName,
		CreatedByID:         row.CreatedBy.String(),
		CreatedAt:           row.CreatedAt.Time,
		UpdatedAt:           row.UpdatedAt.Time,
		EdonationKeyed:      row.EdonationKeyed,
		Replaces:            replaces,
		ReplacedBy:          replacedBy,
		ReviewHistory:       reviewHistory,
		DonorLanguage:       row.DonorLanguage,
		ReceiptPDFObjectKey: row.ReceiptPdfObjectKey,
		ViewerIsCreator:     viewerIsCreator,
		CanApprove:          canReview,
		CanReturn:           canReview,
		CanReject:           canReview,
		CanRevealPII:        pii.CanRevealFull(claims),
	}
	if row.ConsentAt.Valid {
		t := row.ConsentAt.Time
		resp.ConsentAt = &t
	}
	if row.SubmittedAt.Valid {
		t := row.SubmittedAt.Time
		resp.SubmittedAt = &t
	}
	if row.ApprovedAt.Valid {
		t := row.ApprovedAt.Time
		resp.ApprovedAt = &t
	}
	if row.ReviewedBy.Valid {
		s := row.ReviewedBy.String()
		resp.ReviewedBy = &s
	}
	if row.ReviewedAt.Valid {
		t := row.ReviewedAt.Time
		resp.ReviewedAt = &t
	}
	if row.CancelledBy.Valid {
		s := row.CancelledBy.String()
		resp.CancelledBy = &s
	}
	if row.CancelledAt.Valid {
		t := row.CancelledAt.Time
		resp.CancelledAt = &t
	}
	resp.CancelReason = row.CancelReason
	return resp, nil
}

// expandReceiptRef expands a nullable self-FK donation-id pointer (row.Replaces or
// row.ReplacedBy, D-50) into a nested {id, receipt_formatted} object. Returns nil if
// the pointer is not set. A missing referenced row (should not happen — the pointer
// only ever targets a real donation) is treated as "no ref" rather than an error.
func (s *DonationService) expandReceiptRef(ctx context.Context, id pgtype.UUID) (*ReceiptRef, error) {
	if !id.Valid {
		return nil, nil
	}
	ref, err := s.queries.GetReceiptRefByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	formatted := ""
	if ref.ReceiptFormatted != nil {
		formatted = *ref.ReceiptFormatted
	}
	return &ReceiptRef{ID: ref.ID.String(), ReceiptFormatted: formatted}, nil
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
