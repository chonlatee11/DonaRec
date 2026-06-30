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
//
// NOTE: This file contains the skeleton (RED phase — 03-03 TDD).
// Full implementation is in the GREEN commit that follows.
package donation

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/donnarec/donnarec-api/internal/audit"
	"github.com/donnarec/donnarec-api/internal/auth"
	"github.com/donnarec/donnarec-api/internal/crypto"
	db "github.com/donnarec/donnarec-api/internal/db/generated"
	"github.com/donnarec/donnarec-api/internal/receiptno"
)

// DonationService implements donation lifecycle business logic.
// All dependencies are constructor-injected (no global state — Pattern B).
type DonationService struct {
	pool        *pgxpool.Pool
	queries     *db.Queries
	allocator   *receiptno.Allocator  // used in Approve (03-05)
	auditSvc    *audit.AuditService   // used in Approve (03-05)
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
// This function is the single source of truth for the D-45 state machine subset owned by
// plan 03-03 (draft/submit/update). Approve/return/reject/cancel transitions wired in 03-05/06.
func canTransition(from db.DonationStatus, action string) bool {
	switch action {
	case "submit":
		return from == db.DonationStatusDraft
	case "update":
		return from == db.DonationStatusDraft
	default:
		return false
	}
}

// Create inserts a new donation record in 'draft' status. (RED stub — fails intentionally)
func (s *DonationService) Create(ctx context.Context, req CreateDonationRequest, claims auth.KeycloakClaims) (*DonationResponse, error) {
	return nil, fmt.Errorf("Create: not implemented — RED phase (03-03)")
}

// GetByID retrieves a donation by ID with the tax ID masked. (RED stub)
func (s *DonationService) GetByID(ctx context.Context, id string, claims auth.KeycloakClaims) (*DonationResponse, error) {
	return nil, fmt.Errorf("GetByID: not implemented — RED phase (03-03)")
}

// UpdateDraft updates Maker-editable fields on a draft donation. (RED stub)
func (s *DonationService) UpdateDraft(ctx context.Context, id string, req UpdateDraftRequest, claims auth.KeycloakClaims) (*DonationResponse, error) {
	return nil, fmt.Errorf("UpdateDraft: not implemented — RED phase (03-03)")
}

// Submit transitions a draft donation to pending_review status. (RED stub)
func (s *DonationService) Submit(ctx context.Context, id string, claims auth.KeycloakClaims) (*DonationResponse, error) {
	return nil, fmt.Errorf("Submit: not implemented — RED phase (03-03)")
}

// List returns a paginated list of donations with masked PII. (RED stub)
func (s *DonationService) List(ctx context.Context, filter ListFilter, claims auth.KeycloakClaims) ([]DonationResponse, error) {
	return nil, fmt.Errorf("List: not implemented — RED phase (03-03)")
}
