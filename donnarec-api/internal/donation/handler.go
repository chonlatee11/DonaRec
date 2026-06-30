// Package donation — handler.go
//
// DonationHandler provides Gin HTTP handlers for the donation lifecycle endpoints.
// All handlers follow established patterns from internal/users/handler.go:
//   - Pattern A: claims extraction block (verbatim copy from users/handler.go)
//   - Pattern C: no PII in logs — only donation_id or operation name
//   - audit_after marker set on every successful response
//
// Sentinel error → HTTP status mapping (PATTERNS.md §"internal/donation/handler.go"):
//
//	ErrInvalidTransition → 409 Conflict
//	ErrMissingTaxID      → 422 Unprocessable Entity
//	ErrForbidden         → 403 Forbidden
//	ErrSoDViolation      → 403 Forbidden
//	ErrMissingReason     → 422 Unprocessable Entity
//	ErrNotFound          → 404 Not Found
//	default              → 500 (log donation_id + operation only — Pattern C)
package donation

import (
	"errors"
	"net/http"

	"github.com/donnarec/donnarec-api/internal/auth"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"go.uber.org/zap"
)

// DonationHandler handles HTTP requests for donation lifecycle endpoints.
// All endpoints require authentication (handled by middleware before reaching here).
type DonationHandler struct {
	svc      *DonationService
	validate *validator.Validate
	logger   *zap.Logger
}

// NewDonationHandler creates a DonationHandler with the given dependencies.
func NewDonationHandler(svc *DonationService, logger *zap.Logger) *DonationHandler {
	return &DonationHandler{
		svc:      svc,
		validate: validator.New(),
		logger:   logger,
	}
}

// Create creates a new donation record in 'draft' status (FR-07).
// POST /api/donations
//
// Returns 201 with the created DonationResponse on success.
// Tax ID is mandatory (D-44); consent fields are captured per snapshot (D-49).
func (h *DonationHandler) Create(c *gin.Context) {
	// Pattern A: auth claims extraction — copy verbatim for every handler
	raw, exists := c.Get("claims")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing_auth_context"})
		return
	}
	claims, ok := raw.(auth.KeycloakClaims)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid_claims_type"})
		return
	}

	var req CreateDonationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request_body"})
		return
	}
	if err := h.validate.Struct(req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error":   "validation_failed",
			"details": err.Error(),
		})
		return
	}

	resp, err := h.svc.Create(c.Request.Context(), req, claims)
	if err != nil {
		switch {
		case errors.Is(err, ErrMissingTaxID):
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "missing_tax_id"})
		case errors.Is(err, ErrForbidden):
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		default:
			// Pattern C: log operation only — no PII fields in error logs (T-03-10)
			h.logger.Error("failed to create donation",
				zap.String("operation", "CreateDonation"),
				zap.Error(err),
			)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "donation_creation_failed"})
		}
		return
	}

	// audit_after: captured by AuditMiddleware for the immutable audit trail (Pattern D)
	c.Set("audit_after", resp)
	c.JSON(http.StatusCreated, gin.H{"data": resp})
}

// GetByID retrieves a single donation record by UUID (PII masked by default — T-03-09).
// GET /api/donations/:id
func (h *DonationHandler) GetByID(c *gin.Context) {
	// Pattern A: auth claims extraction
	raw, exists := c.Get("claims")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing_auth_context"})
		return
	}
	claims, ok := raw.(auth.KeycloakClaims)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid_claims_type"})
		return
	}

	id := c.Param("id")

	resp, err := h.svc.GetByID(c.Request.Context(), id, claims)
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
		case errors.Is(err, ErrForbidden):
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		default:
			// Pattern C: log donation_id only — no PII
			h.logger.Error("failed to get donation",
				zap.String("operation", "GetDonationByID"),
				zap.String("donation_id", id),
				zap.Error(err),
			)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "get_donation_failed"})
		}
		return
	}

	c.Set("audit_after", resp)
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// Update edits a donation that is still in 'draft' status (FR-09).
// PUT /api/donations/:id
//
// Returns ErrInvalidTransition (409) if the record is no longer in draft.
// Tax ID in the request body is re-encrypted on every update (T-03-08).
func (h *DonationHandler) Update(c *gin.Context) {
	// Pattern A: auth claims extraction
	raw, exists := c.Get("claims")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing_auth_context"})
		return
	}
	claims, ok := raw.(auth.KeycloakClaims)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid_claims_type"})
		return
	}

	id := c.Param("id")

	var req UpdateDraftRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request_body"})
		return
	}
	if err := h.validate.Struct(req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error":   "validation_failed",
			"details": err.Error(),
		})
		return
	}

	resp, err := h.svc.UpdateDraft(c.Request.Context(), id, req, claims)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidTransition):
			c.JSON(http.StatusConflict, gin.H{"error": "status_conflict"})
		case errors.Is(err, ErrMissingTaxID):
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "missing_tax_id"})
		case errors.Is(err, ErrForbidden):
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		case errors.Is(err, ErrNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
		default:
			// Pattern C: log donation_id only — no PII
			h.logger.Error("failed to update donation",
				zap.String("operation", "UpdateDraftDonation"),
				zap.String("donation_id", id),
				zap.Error(err),
			)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "update_donation_failed"})
		}
		return
	}

	c.Set("audit_after", resp)
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// Submit transitions a draft donation to pending_review status (FR-11, D-45).
// POST /api/donations/:id/submit
//
// Returns ErrInvalidTransition (409) if the record is not currently in draft.
func (h *DonationHandler) Submit(c *gin.Context) {
	// Pattern A: auth claims extraction
	raw, exists := c.Get("claims")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing_auth_context"})
		return
	}
	claims, ok := raw.(auth.KeycloakClaims)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid_claims_type"})
		return
	}

	id := c.Param("id")

	resp, err := h.svc.Submit(c.Request.Context(), id, claims)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidTransition):
			c.JSON(http.StatusConflict, gin.H{"error": "status_conflict"})
		case errors.Is(err, ErrForbidden):
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		case errors.Is(err, ErrNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
		default:
			// Pattern C: log donation_id only — no PII
			h.logger.Error("failed to submit donation",
				zap.String("operation", "SubmitDonation"),
				zap.String("donation_id", id),
				zap.Error(err),
			)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "submit_donation_failed"})
		}
		return
	}

	c.Set("audit_after", resp)
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// List returns a paginated, PII-masked list of donations ordered by created_at DESC.
// GET /api/donations
//
// Basic implementation for plan 03-03; full filter wiring (date range, status, receipt no)
// is added in plan 03-06 using the SearchDonations sqlc query.
func (h *DonationHandler) List(c *gin.Context) {
	// Pattern A: auth claims extraction
	raw, exists := c.Get("claims")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing_auth_context"})
		return
	}
	claims, ok := raw.(auth.KeycloakClaims)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid_claims_type"})
		return
	}

	// Basic defaults — full filter param parsing wired in 03-06.
	filter := ListFilter{
		Limit:  50,
		Offset: 0,
	}

	resp, err := h.svc.List(c.Request.Context(), filter, claims)
	if err != nil {
		// Pattern C: log operation only — no PII
		h.logger.Error("failed to list donations",
			zap.String("operation", "ListDonations"),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list_donations_failed"})
		return
	}

	c.Set("audit_after", resp)
	c.JSON(http.StatusOK, gin.H{"data": resp})
}
