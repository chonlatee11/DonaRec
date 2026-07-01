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
	"fmt"
	"net/http"
	"time"

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

// Approve issues a receipt for a pending_review donation (FR-08, D-45).
// POST /api/donations/:id/approve
//
// Only Checker and Admin may approve (enforced by route group middleware + SoD guard).
// Returns 200 with the issued DonationResponse on success.
// ErrSoDViolation  → 403 (approver == creator)
// ErrInvalidTransition → 409 (status not pending_review)
// ErrNotFound      → 404
func (h *DonationHandler) Approve(c *gin.Context) {
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

	resp, err := h.svc.Approve(c.Request.Context(), id, claims)
	if err != nil {
		switch {
		case errors.Is(err, ErrSoDViolation):
			c.JSON(http.StatusForbidden, gin.H{"error": "sod_violation"})
		case errors.Is(err, ErrInvalidTransition):
			c.JSON(http.StatusConflict, gin.H{"error": "status_conflict"})
		case errors.Is(err, ErrForbidden):
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		case errors.Is(err, ErrNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
		default:
			// Pattern C: log donation_id only — no PII
			h.logger.Error("failed to approve donation",
				zap.String("operation", "ApproveDonation"),
				zap.String("donation_id", id),
				zap.Error(err),
			)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "approve_donation_failed"})
		}
		return
	}

	c.Set("audit_after", resp)
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// ReturnToDraft returns a pending_review donation to draft with a mandatory reason (FR-12, D-45).
// POST /api/donations/:id/return
//
// Only Checker and Admin may return (enforced by route group middleware).
// ErrMissingReason → 422 (reason empty/whitespace)
// ErrInvalidTransition → 409 (status not pending_review)
// ErrNotFound → 404
func (h *DonationHandler) ReturnToDraft(c *gin.Context) {
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

	var req ReviewRequest
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

	resp, err := h.svc.Return(c.Request.Context(), id, req.Reason, claims)
	if err != nil {
		switch {
		case errors.Is(err, ErrMissingReason):
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "missing_reason"})
		case errors.Is(err, ErrInvalidTransition):
			c.JSON(http.StatusConflict, gin.H{"error": "status_conflict"})
		case errors.Is(err, ErrForbidden):
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		case errors.Is(err, ErrNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
		default:
			// Pattern C: log donation_id only — no PII
			h.logger.Error("failed to return donation",
				zap.String("operation", "ReturnDonation"),
				zap.String("donation_id", id),
				zap.Error(err),
			)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "return_donation_failed"})
		}
		return
	}

	c.Set("audit_after", resp)
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// Reject permanently rejects a pending_review donation with a mandatory reason (FR-12, D-45).
// POST /api/donations/:id/reject
//
// Rejected is a terminal state — no further transitions are possible.
// Only Checker and Admin may reject (enforced by route group middleware).
// ErrMissingReason → 422 (reason empty/whitespace)
// ErrInvalidTransition → 409 (status not pending_review)
// ErrNotFound → 404
func (h *DonationHandler) Reject(c *gin.Context) {
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

	var req ReviewRequest
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

	resp, err := h.svc.Reject(c.Request.Context(), id, req.Reason, claims)
	if err != nil {
		switch {
		case errors.Is(err, ErrMissingReason):
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "missing_reason"})
		case errors.Is(err, ErrInvalidTransition):
			c.JSON(http.StatusConflict, gin.H{"error": "status_conflict"})
		case errors.Is(err, ErrForbidden):
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		case errors.Is(err, ErrNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
		default:
			// Pattern C: log donation_id only — no PII
			h.logger.Error("failed to reject donation",
				zap.String("operation", "RejectDonation"),
				zap.String("donation_id", id),
				zap.Error(err),
			)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "reject_donation_failed"})
		}
		return
	}

	c.Set("audit_after", resp)
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// List returns a paginated, PII-masked list of donations with optional search filters (FR-10, D-53).
// GET /api/donations[?name=...&status=...&from=...&to=...&receipt_no=...&page=...]
//
// Supported query params (all optional):
//
//	name       — donor name ILIKE filter
//	status     — donation status filter (draft/pending_review/issued/cancelled/rejected)
//	from       — from date (YYYY-MM-DD), inclusive
//	to         — to date (YYYY-MM-DD), inclusive
//	receipt_no — exact receipt formatted string
//	page       — 1-based page number (default 1, page size 20)
//
// Tax ID is NOT accepted as a filter (D-53, T-03-29).
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

	// Parse optional search filter query params (D-53).
	filter := ListFilter{
		Limit:  20, // default page size per UI-SPEC
		Offset: 0,
	}

	if name := c.Query("name"); name != "" {
		filter.DonorName = &name
	}
	if status := c.Query("status"); status != "" {
		filter.Status = &status
	}
	if from := c.Query("from"); from != "" {
		if t, parseErr := parseDate(from); parseErr == nil {
			filter.FromDate = &t
		}
	}
	if to := c.Query("to"); to != "" {
		if t, parseErr := parseDate(to); parseErr == nil {
			filter.ToDate = &t
		}
	}
	if receiptNo := c.Query("receipt_no"); receiptNo != "" {
		filter.ReceiptNo = &receiptNo
	}
	if pageStr := c.Query("page"); pageStr != "" {
		if page := parsePositiveInt32(pageStr); page > 0 {
			filter.Offset = (page - 1) * filter.Limit
		}
	}

	resp, err := h.svc.Search(c.Request.Context(), filter, claims)
	if err != nil {
		// Pattern C: log operation only — no PII
		h.logger.Error("failed to search donations",
			zap.String("operation", "SearchDonations"),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list_donations_failed"})
		return
	}

	c.Set("audit_after", resp)
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// Cancel voids an issued receipt (FR-19, D-47).
// POST /api/donations/:id/cancel
//
// Checker and Admin only (enforced by route group + service layer).
// Reason is mandatory (ErrMissingReason → 422).
// RDConfirmationReason is required when edonation_keyed=true (ErrEDonationKeyedCancel → 409).
// The receipt number is retained on the cancelled record (no gap — load-bearing invariant).
func (h *DonationHandler) Cancel(c *gin.Context) {
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

	var req CancelDonationRequest
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

	resp, err := h.svc.Cancel(c.Request.Context(), id, req, claims)
	if err != nil {
		switch {
		case errors.Is(err, ErrForbidden):
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		case errors.Is(err, ErrMissingReason):
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "reason_required"})
		case errors.Is(err, ErrEDonationKeyedCancel):
			c.JSON(http.StatusConflict, gin.H{"error": "edonation_keyed_confirmation_required"})
		case errors.Is(err, ErrInvalidTransition):
			c.JSON(http.StatusConflict, gin.H{"error": "status_conflict"})
		case errors.Is(err, ErrNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
		default:
			// Pattern C: log donation_id only — no PII
			h.logger.Error("failed to cancel donation",
				zap.String("operation", "CancelDonation"),
				zap.String("donation_id", id),
				zap.Error(err),
			)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "cancel_donation_failed"})
		}
		return
	}

	c.Set("audit_after", resp)
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// Reissue performs Void & Reissue (D-50): cancels an issued receipt and creates a corrected draft.
// POST /api/donations/:id/reissue
//
// Checker and Admin only (enforced by route group + service layer).
// The replacement draft earns a fresh receipt number only via the normal Submit → Approve path.
// Reason is mandatory; RDConfirmationReason required when edonation_keyed=true.
func (h *DonationHandler) Reissue(c *gin.Context) {
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

	var req ReissueDonationRequest
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

	resp, err := h.svc.Reissue(c.Request.Context(), id, req, claims)
	if err != nil {
		switch {
		case errors.Is(err, ErrForbidden):
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		case errors.Is(err, ErrMissingReason):
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "reason_required"})
		case errors.Is(err, ErrEDonationKeyedCancel):
			c.JSON(http.StatusConflict, gin.H{"error": "edonation_keyed_confirmation_required"})
		case errors.Is(err, ErrInvalidTransition):
			c.JSON(http.StatusConflict, gin.H{"error": "status_conflict"})
		case errors.Is(err, ErrMissingTaxID):
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "missing_tax_id"})
		case errors.Is(err, ErrNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
		default:
			// Pattern C: log donation_id only — no PII
			h.logger.Error("failed to reissue donation",
				zap.String("operation", "ReissueDonation"),
				zap.String("original_id", id),
				zap.Error(err),
			)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "reissue_donation_failed"})
		}
		return
	}

	c.Set("audit_after", resp)
	c.JSON(http.StatusCreated, gin.H{"data": resp})
}

// RevealPII returns the full plaintext donor tax/national ID (D-46, T-03-26).
// GET /api/donations/:id/pii
//
// Checker and Admin only — the service performs the role check (ErrForbidden → 403).
// Every authorized reveal is audited (action="pii.reveal") atomically in the service.
// Pattern C: donation_id is logged; plaintext is NOT logged (T-03-10).
func (h *DonationHandler) RevealPII(c *gin.Context) {
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

	resp, err := h.svc.RevealPII(c.Request.Context(), id, claims)
	if err != nil {
		switch {
		case errors.Is(err, ErrForbidden):
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		case errors.Is(err, ErrNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
		default:
			// Pattern C: log donation_id only — NEVER log plaintext tax ID (T-03-10)
			h.logger.Error("failed to reveal PII",
				zap.String("operation", "RevealPII"),
				zap.String("donation_id", id),
				zap.Error(err),
			)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "reveal_pii_failed"})
		}
		return
	}

	// audit_after: AuditMiddleware captures the response for the immutable trail (Pattern D).
	c.Set("audit_after", resp)
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// --- Handler helper functions ---

// parseDate parses a "YYYY-MM-DD" string into a time.Time (UTC midnight).
func parseDate(s string) (time.Time, error) {
	return time.ParseInLocation("2006-01-02", s, time.UTC)
}

// parsePositiveInt32 parses a decimal string to int32. Returns 0 on error or non-positive input.
func parsePositiveInt32(s string) int32 {
	var v int
	if _, err := fmt.Sscanf(s, "%d", &v); err != nil || v <= 0 {
		return 0
	}
	return int32(v)
}
