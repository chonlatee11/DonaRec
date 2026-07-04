// Package donation — slip_handler.go
//
// SlipHandler provides Gin HTTP handlers for donation slip attachment endpoints (Plan 03-04).
//
// Endpoints:
//
//	POST   /api/donations/:id/slip  — upload a slip file (multipart/form-data, field "file")
//	GET    /api/donations/:id/slip  — view slip via presigned URL (15-min TTL, T-03-16)
//	DELETE /api/donations/:id/slip  — soft-delete the active slip (D-54)
//
// Error mapping:
//
//	ErrSlipAlreadyExists     → 409 Conflict
//	ErrSlipNotFound          → 404 Not Found
//	ErrUserNotProvisioned    → 403 Forbidden (defensive: identity resolution now happens in
//	                           auth.ResolveAppUser middleware, which 403s before the handler runs)
//	storage.ErrFileTooLarge  → 413 Request Entity Too Large
//	storage.ErrUnsupportedFileType → 415 Unsupported Media Type
//	default                  → 500 (log donation_id + operation — Pattern C)
//
// All handlers follow established patterns from internal/donation/handler.go:
//   - Pattern A: claims extraction block
//   - Pattern C: no PII in logs
//   - audit_after marker set on successful mutations (Pattern D via SlipService)
package donation

import (
	"errors"
	"net/http"

	"github.com/donnarec/donnarec-api/internal/auth"
	"github.com/donnarec/donnarec-api/internal/storage"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"
	"go.uber.org/zap"
)

// SlipHandler handles HTTP requests for donation slip attachment endpoints.
// All endpoints require authentication (handled by middleware before reaching here).
type SlipHandler struct {
	svc    *SlipService
	logger *zap.Logger
}

// NewSlipHandler constructs a SlipHandler with the given dependencies.
func NewSlipHandler(svc *SlipService, logger *zap.Logger) *SlipHandler {
	return &SlipHandler{svc: svc, logger: logger}
}

// Upload handles POST /api/donations/:id/slip
//
// Expects a multipart/form-data request with a "file" field.
// Returns 201 Created with SlipResponse on success.
// Returns 409 if an active slip already exists (remove it first).
// Returns 413 if the file exceeds 10 MB (T-03-15).
// Returns 415 if the file type is not JPEG, PNG, or PDF (T-03-14 magic-byte check).
func (h *SlipHandler) Upload(c *gin.Context) {
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

	donationID := c.Param("id")

	// Parse multipart form: get the uploaded file.
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing_file_field", "detail": "multipart field 'file' is required"})
		return
	}

	file, err := fileHeader.Open()
	if err != nil {
		h.logger.Error("failed to open uploaded file",
			zap.String("operation", "UploadSlip"),
			zap.String("donation_id", donationID),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "file_open_failed"})
		return
	}
	defer file.Close()

	// app_user_id: caller's resolved users.id, set by auth.ResolveAppUser middleware
	// (created-by-fk-mismatch). Passed explicitly to the service (Pattern A).
	rawUserID, userExists := c.Get(auth.AppUserIDContextKey)
	if !userExists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "missing_auth_context"})
		return
	}
	appUserID, userOK := rawUserID.(pgtype.UUID)
	if !userOK {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid_user_id"})
		return
	}

	resp, err := h.svc.UploadSlip(c.Request.Context(), donationID, file, fileHeader.Size, appUserID, claims)
	if err != nil {
		switch {
		case errors.Is(err, ErrSlipAlreadyExists):
			c.JSON(http.StatusConflict, gin.H{"error": "slip_already_exists", "detail": "remove the existing slip before uploading a replacement"})
		case errors.Is(err, ErrNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
		case errors.Is(err, ErrUserNotProvisioned):
			c.JSON(http.StatusForbidden, gin.H{"error": "user_not_provisioned"})
		case errors.Is(err, storage.ErrFileTooLarge):
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "file_too_large", "detail": "maximum slip size is 10 MB"})
		case errors.Is(err, storage.ErrUnsupportedFileType):
			c.JSON(http.StatusUnsupportedMediaType, gin.H{"error": "unsupported_file_type", "detail": "only JPEG, PNG, and PDF files are accepted"})
		default:
			// Pattern C: log donation_id + operation only — no PII
			h.logger.Error("failed to upload slip",
				zap.String("operation", "UploadSlip"),
				zap.String("donation_id", donationID),
				zap.Error(err),
			)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "slip_upload_failed"})
		}
		return
	}

	// audit_after set by SlipService via AppendAuditEntryTx inside the tx (Pattern D).
	c.JSON(http.StatusCreated, gin.H{"data": resp})
}

// View handles GET /api/donations/:id/slip
//
// Returns 200 with a SlipViewResponse containing a presigned URL (15-min TTL, T-03-16).
// Returns 404 if no active slip exists for the donation (normal for cash/no-slip — D-48).
func (h *SlipHandler) View(c *gin.Context) {
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

	donationID := c.Param("id")

	resp, err := h.svc.ViewSlip(c.Request.Context(), donationID, claims)
	if err != nil {
		switch {
		case errors.Is(err, ErrSlipNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "slip_not_found", "detail": "no active slip attachment for this donation"})
		default:
			// Pattern C: log donation_id only — no PII
			h.logger.Error("failed to view slip",
				zap.String("operation", "ViewSlip"),
				zap.String("donation_id", donationID),
				zap.Error(err),
			)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "slip_view_failed"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// Remove handles DELETE /api/donations/:id/slip
//
// Soft-deletes the active slip reference (D-54 — file retained in MinIO for audit).
// Returns 204 No Content on success.
// Returns 404 if no active slip exists.
func (h *SlipHandler) Remove(c *gin.Context) {
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

	donationID := c.Param("id")

	// app_user_id: caller's resolved users.id, set by auth.ResolveAppUser middleware
	// (created-by-fk-mismatch). Passed explicitly to the service (Pattern A).
	rawUserID, userExists := c.Get(auth.AppUserIDContextKey)
	if !userExists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "missing_auth_context"})
		return
	}
	appUserID, userOK := rawUserID.(pgtype.UUID)
	if !userOK {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid_user_id"})
		return
	}

	if err := h.svc.RemoveSlip(c.Request.Context(), donationID, appUserID, claims); err != nil {
		switch {
		case errors.Is(err, ErrSlipNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "slip_not_found", "detail": "no active slip attachment for this donation"})
		case errors.Is(err, ErrUserNotProvisioned):
			c.JSON(http.StatusForbidden, gin.H{"error": "user_not_provisioned"})
		default:
			// Pattern C: log donation_id + operation only — no PII
			h.logger.Error("failed to remove slip",
				zap.String("operation", "RemoveSlip"),
				zap.String("donation_id", donationID),
				zap.Error(err),
			)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "slip_remove_failed"})
		}
		return
	}

	c.Status(http.StatusNoContent)
}
