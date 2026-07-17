// Package donation — public_submission.go
//
// CreatePublicSubmission is the Flow B (public web form) load-bearing path (plan
// 06-03, D-76/78/79/80/81, FR-01/02/03/04). Unlike Flow A — which visits `draft`
// first (Create) and only later transitions to pending_review (Submit), possibly
// with an optional slip attached in a separate call — a public donor has no
// iteration loop and no authenticated follow-up (D-86). Their single multipart
// POST must therefore land ATOMICALLY as a fully-formed pending_review record:
// create + submit + slip-reference + audit + ack_email outbox enqueue, all inside
// ONE dbhelpers.WithTx closure. This mirrors Approve's atomic multi-step shape
// (service.go), NOT Create+Submit+UploadSlip's three-transaction shape.
//
// Ordering (fail-fast, no orphan state — Pattern 1 / Pitfall 4, 06-RESEARCH.md):
// the handler PUTs the slip to MinIO and magic-byte-validates it BEFORE calling
// this method; if that fails, no donation row is ever created. Here we only
// insert the already-uploaded object's REFERENCE. If the MinIO PUT succeeds but
// this tx rolls back, MinIO retains an orphaned object (acceptable, D-54's
// "storage retains, DB is source of truth") — but no partial donations row exists.
package donation

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"go.uber.org/zap"

	"github.com/donnarec/donnarec-api/internal/audit"
	"github.com/donnarec/donnarec-api/internal/auth"
	"github.com/donnarec/donnarec-api/internal/crypto"
	dbhelpers "github.com/donnarec/donnarec-api/internal/db"
	db "github.com/donnarec/donnarec-api/internal/db/generated"
	"github.com/donnarec/donnarec-api/internal/pii"
)

// PublicWebUserID is the fixed, literal UUID of the seeded public-web system user
// (migration 000016, D-76). It is used BOTH as donations.created_by (FK to
// users.id) AND as the synthetic audit actor_id for public submissions.
//
// Pitfall 1 (06-RESEARCH.md): audit.AppendAuditEntryTx calls parseUUID(ActorID)
// and rolls back the whole transaction if it is not UUID-shaped. This value is a
// valid UUID (never a human-readable sentinel like "public-web"), so every public
// submission's in-tx audit row succeeds. It mirrors the exact literal seeded as
// BOTH users.id and users.keycloak_subject in 000016_seed_public_web_user.up.sql.
const PublicWebUserID = "00000000-0000-4000-8000-000000000006"

// PublicReferenceNumber derives the donor-facing reference code (D-84) from the
// donation UUID. This is EXPLICITLY NOT a receipt number — receipt numbers are
// gap-less, allocated only at Checker approval by internal/receiptno (which has
// exactly one call site, Approve, by design D-35). The reference is only for the
// donor to quote when contacting staff; it never touches the allocator.
func PublicReferenceNumber(donationID string) string {
	compact := strings.ToUpper(strings.ReplaceAll(donationID, "-", ""))
	if len(compact) < 8 {
		return "REF-" + compact
	}
	return "REF-" + compact[:8]
}

// CreatePublicSubmission atomically creates a submitted (pending_review) Flow B
// donation with encrypted PII + consent snapshot, links the already-uploaded slip,
// audits the submit under the public-web system actor, and enqueues an ack_email
// outbox job — all in one transaction (D-76/78/79/80/81).
//
// slipObjectKey/slipMimeType/slipSizeBytes describe the object the handler already
// PUT to MinIO (magic-byte-validated) before calling this method. publicWebUserID
// is the resolved users.id of the seeded public-web system user (PublicWebUserID).
//
// D-79: DonorTaxID is mandatory — ErrMissingTaxID before any DB call if empty.
// D-84: no receipt number is allocated here; internal/receiptno is never called.
// Pattern C: logs only donation_id + created_by UUID — no PII fields ever logged.
func (s *DonationService) CreatePublicSubmission(
	ctx context.Context,
	req PublicDonationRequest,
	slipObjectKey, slipMimeType string,
	slipSizeBytes int64,
	publicWebUserID pgtype.UUID,
) (*DonationDetailResponse, error) {
	// D-79: mandatory tax ID — fail fast before any DB call.
	if req.DonorTaxID == "" {
		return nil, ErrMissingTaxID
	}

	donatedAtTime, err := time.ParseInLocation("2006-01-02", req.DonatedAt, time.UTC)
	if err != nil {
		return nil, fmt.Errorf("invalid donated_at %q: %w", req.DonatedAt, err)
	}

	// T-06-10: AES-256-GCM envelope encryption BEFORE the transaction — identical to
	// Flow A's Create (service.go); plaintext never reaches Postgres.
	encBytes, dekBytes, err := crypto.EncryptField(ctx, s.keyProvider, []byte(req.DonorTaxID))
	if err != nil {
		return nil, fmt.Errorf("encrypt donor tax ID: %w", err)
	}

	var amount pgtype.Numeric
	if err := amount.Scan(strconv.FormatFloat(req.Amount, 'f', 2, 64)); err != nil {
		return nil, fmt.Errorf("invalid amount: %w", err)
	}

	donatedAt := pgtype.Date{Time: donatedAtTime, Valid: true}
	// PDPA default retention: 10 years from donation date (retain_until NOT NULL).
	retainUntil := pgtype.Date{Time: donatedAtTime.AddDate(10, 0, 0), Valid: true}

	// D-81: consent_at is set to now() when consent_given=true.
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

	donorLanguage := resolveDonorLanguage(req.DonorLanguage)

	var fullRow db.Donation
	err = dbhelpers.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		qtx := s.queries.WithTx(tx)

		// Step 1: create the donation with source='flow_b', created_by=public-web user.
		row, txErr := qtx.CreateDonation(ctx, db.CreateDonationParams{
			CreatedBy:          publicWebUserID,
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
			DonorLanguage:      donorLanguage,
			Source:             "flow_b",
		})
		if txErr != nil {
			return txErr
		}

		// Step 2: immediately transition draft -> pending_review in the SAME tx.
		// Flow B never makes two separate HTTP calls (Anti-Pattern, 06-RESEARCH.md).
		if submitErr := qtx.SubmitDonation(ctx, row.ID); submitErr != nil {
			return fmt.Errorf("submit donation: %w", submitErr)
		}

		// Step 3: insert the ALREADY-uploaded slip's DB reference (D-80 mandatory slip).
		if _, slipErr := qtx.InsertSlip(ctx, db.InsertSlipParams{
			DonationID: row.ID,
			ObjectKey:  slipObjectKey,
			MimeType:   slipMimeType,
			SizeBytes:  slipSizeBytes,
			UploadedBy: publicWebUserID,
		}); slipErr != nil {
			return fmt.Errorf("insert slip: %w", slipErr)
		}

		// Step 4: audit the submit in-tx (NFR-05). ActorID is the public-web UUID
		// constant (Pitfall 1) — NOT a readable string, so parseUUID never rolls back.
		if auditErr := s.auditSvc.AppendAuditEntryTx(ctx, tx, audit.AuditEntry{
			ActorID:  PublicWebUserID,
			Action:   "donation.public_submit",
			Resource: "/api/public/donations",
		}); auditErr != nil {
			return fmt.Errorf("audit public submit: %w", auditErr)
		}

		// Step 5: enqueue the ack_email outbox job (D-85) — NOT issue_receipt (no
		// receipt exists yet; receipts are allocated only at approval). Consumed by
		// the worker in plan 06-04.
		payload, _ := json.Marshal(map[string]string{"donation_id": row.ID.String()})
		if outboxErr := qtx.EnqueueOutboxJob(ctx, db.EnqueueOutboxJobParams{
			JobType: "ack_email",
			Payload: payload,
		}); outboxErr != nil {
			return fmt.Errorf("enqueue ack_email: %w", outboxErr)
		}

		// Fetch the full committed-state row for the response (buildDetailResponse
		// needs the full db.Donation shape — CreateDonation's RETURNING is partial).
		var getErr error
		fullRow, getErr = qtx.GetDonationByID(ctx, row.ID)
		return getErr
	})
	if err != nil {
		return nil, fmt.Errorf("create public submission: %w", err)
	}

	// Pattern C: log donation_id + created_by only — never donor name, tax ID, email.
	s.logger.Info("public donation submitted",
		zap.String("donation_id", fullRow.ID.String()),
		zap.String("created_by", fullRow.CreatedBy.String()),
	)

	// No claims for the unauthenticated donor — empty KeycloakClaims yields all-false
	// viewer/authorization flags (the donor never consumes them). buildDetailResponse
	// treats an unresolvable subject as "not the creator" without erroring.
	return s.buildDetailResponse(ctx, fullRow, pii.MaskNationalID(req.DonorTaxID), auth.KeycloakClaims{})
}
