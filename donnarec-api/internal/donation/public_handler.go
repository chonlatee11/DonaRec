// Package donation — public_handler.go
//
// PublicDonationHandler serves the single UNAUTHENTICATED Flow B endpoint
// (plan 06-03, D-78):
//
//	POST /api/public/donations — a donor's complete submission (multipart/form-data:
//	                             donor fields + mandatory slip file + turnstile_token)
//
// This is the FIRST unauthenticated route in the codebase. RequireAuth is replaced
// on the route group by ratelimit.PerIP + captcha.VerifyTurnstile (wired in
// cmd/server/main.go). By the time CreatePublic runs, the rate-limit and CAPTCHA
// middleware have ALREADY passed — the CAPTCHA token is never read here or bound
// into PublicDonationRequest (Pitfall 3).
//
// Strict fail-fast ordering (Pitfall 4 — never a DB row without a slip):
//  1. parse + validate donor fields
//  2. require the slip file (D-80 mandatory) — missing → 4xx, no service call
//  3. storage.PutSlip FIRST (magic-byte + size validation) — no DB write yet
//  4. donationSvc.CreatePublicSubmission — the one atomic tx
//  5. return the reference number (D-84) — explicitly NOT a receipt number
//
// Error shapes are deliberately distinct so the frontend can key off them:
//
//	slip_required          → 400 (D-80: slip is mandatory)
//	unsupported_file_type  → 415 (storage magic-byte allowlist)
//	file_too_large         → 413 (10 MB cap)
//	validation_failed      → 422 (donor field validation — distinct from captcha_failed 400)
//	missing_tax_id         → 422 (D-79)
//	default                → 500 (Pattern C: log operation only, no PII)
package donation

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/donnarec/donnarec-api/internal/storage"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v5/pgtype"
	"go.uber.org/zap"
)

// PublicSlipField is the multipart/form field name the public form submits the
// slip file under. The frontend (plan 06-06) and this handler must agree on it —
// it is deliberately distinct from Flow A's "file" field so the two upload paths
// never share a contract by accident.
const PublicSlipField = "slip"

// SlipPutter is the narrow interface PublicDonationHandler needs to validate
// (magic-byte + size) and store a slip file before opening the DB transaction.
// Satisfied implicitly by *storage.StorageClient (the SAME slips-bucket client
// Flow A's SlipService uses) — this package never imports internal/storage for the
// concrete type, matching the codebase's narrow-interface convention (ReceiptsStore,
// mailer.EmailSender). The seam also lets the E2E harness inject a fake that runs
// the REAL storage.ValidateSlip magic-byte check while faking only the network PUT.
type SlipPutter interface {
	PutSlip(ctx context.Context, r io.Reader, size int64, donationID string) (objectKey, mimeType string, err error)
}

// PublicDonationHandler handles the unauthenticated public donation endpoint.
// It holds its OWN slip store and the resolved public-web system users.id
// (injected once at wiring time) — there is no per-request auth identity to derive.
type PublicDonationHandler struct {
	svc             *DonationService
	slipStore       SlipPutter
	publicWebUserID pgtype.UUID
	validate        *validator.Validate
	logger          *zap.Logger
}

// NewPublicDonationHandler constructs a PublicDonationHandler. publicWebUserID is
// the resolved users.id of the seeded public-web system user (PublicWebUserID),
// used as created_by for every public submission (D-76). slipStore is satisfied
// in production by *storage.StorageClient.
func NewPublicDonationHandler(
	svc *DonationService,
	slipStore SlipPutter,
	publicWebUserID pgtype.UUID,
	logger *zap.Logger,
) *PublicDonationHandler {
	return &PublicDonationHandler{
		svc:             svc,
		slipStore:       slipStore,
		publicWebUserID: publicWebUserID,
		validate:        validator.New(),
		logger:          logger,
	}
}

// CreatePublic handles POST /api/public/donations.
//
// Returns 201 with {"data": {"reference_number": "...", "status": "pending_review"}}
// on success. The reference number (D-84) is derived from the donation id and is
// explicitly NOT a receipt number.
func (h *PublicDonationHandler) CreatePublic(c *gin.Context) {
	// --- Step 1: parse + validate donor fields (multipart form values) ---
	// amount arrives as a string in multipart form; parse before struct validation
	// so a non-numeric amount is a field-validation error, not a 500.
	amount, amtErr := strconv.ParseFloat(c.PostForm("amount"), 64)
	if amtErr != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error":   "validation_failed",
			"details": "amount must be a number",
		})
		return
	}

	req := PublicDonationRequest{
		DonorName:          c.PostForm("donor_name"),
		DonorTaxID:         c.PostForm("donor_tax_id"),
		DonorAddress:       c.PostForm("donor_address"),
		DonorEmail:         c.PostForm("donor_email"),
		Amount:             amount,
		DonatedAt:          c.PostForm("donated_at"),
		Notes:              c.PostForm("notes"),
		ConsentGiven:       c.PostForm("consent_given") == "true",
		ConsentTextVersion: c.PostForm("consent_text_version"),
		ConsentPurpose:     c.PostForm("consent_purpose"),
		DonorLanguage:      c.PostForm("donor_language"),
	}
	if err := h.validate.Struct(req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error":   "validation_failed",
			"details": err.Error(),
		})
		return
	}

	// --- Step 2: slip is MANDATORY (D-80) — missing file is 4xx before any DB row ---
	fileHeader, err := c.FormFile(PublicSlipField)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":  "slip_required",
			"detail": "a donation slip file is required",
		})
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		h.logger.Error("failed to open uploaded public slip",
			zap.String("operation", "CreatePublic"), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "file_open_failed"})
		return
	}
	defer file.Close()

	// --- Step 3: storage.PutSlip FIRST — magic-byte + size validation, no DB write ---
	// "public" is the object-key grouping prefix (the donation id does not exist yet;
	// the object key is a plain reference, the UUID in it prevents guessing, T-03-16).
	objectKey, mimeType, err := h.slipStore.PutSlip(c.Request.Context(), file, fileHeader.Size, "public")
	if err != nil {
		switch {
		case errors.Is(err, storage.ErrFileTooLarge):
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"error":  "file_too_large",
				"detail": "maximum slip size is 10 MB",
			})
		case errors.Is(err, storage.ErrUnsupportedFileType):
			c.JSON(http.StatusUnsupportedMediaType, gin.H{
				"error":  "unsupported_file_type",
				"detail": "only JPEG, PNG, and PDF files are accepted",
			})
		default:
			h.logger.Error("failed to store public slip",
				zap.String("operation", "CreatePublic"), zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "slip_upload_failed"})
		}
		return
	}

	// --- Step 4: atomic create+submit+slip+audit+ack_email ---
	resp, err := h.svc.CreatePublicSubmission(
		c.Request.Context(), req, objectKey, mimeType, fileHeader.Size, h.publicWebUserID)
	if err != nil {
		switch {
		case errors.Is(err, ErrMissingTaxID):
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "missing_tax_id"})
		default:
			// Pattern C: log operation only — no PII fields in error logs.
			h.logger.Error("failed to create public submission",
				zap.String("operation", "CreatePublicSubmission"), zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "public_submission_failed"})
		}
		return
	}

	// --- Step 5: return the donor-facing reference number (D-84) ---
	c.JSON(http.StatusCreated, gin.H{"data": gin.H{
		"reference_number": PublicReferenceNumber(resp.ID),
		"status":           resp.Status,
	}})
}
