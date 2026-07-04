// Package settings — handler.go
//
// SettingsHandler provides Gin HTTP handlers for the Admin-only receipt template/
// compliance settings API (D-58/D-59/D-61, NFR-09). All handlers follow the same
// conventions as internal/donation/handler.go:
//   - Pattern A: claims extraction block (verbatim copy from donation/handler.go)
//   - Pattern C: no PII / template HTML in logs — only operation name
//   - audit_after marker set on every successful mutating response (Pattern D)
//
// Every route is registered under the EXISTING adminGroup (cmd/server/main.go), which
// already enforces RequireRoles(RoleAdmin) — no new role-guard middleware needed. This
// package additionally requires auth.ResolveAppUser on adminGroup (added by this plan) so
// updated_by can be set to the acting admin's resolved users.id, never the raw Keycloak
// subject (mirrors the created-by-fk-mismatch fix from Phase 3).
//
// Sentinel error → HTTP status mapping:
//
//	ErrInvalidTemplate                     → 422 Unprocessable Entity (template.Parse failed)
//	ErrInvalidNumberFormat                 → 422 Unprocessable Entity (bad separator/prefix)
//	ErrInvalidImageSlot                    → 400 Bad Request (unknown image slot)
//	storage.ErrUnsupportedTemplateImageType → 415 Unsupported Media Type
//	storage.ErrTemplateImageTooLarge        → 413 Payload Too Large
//	default                                 → 500 (log operation only — Pattern C)
package settings

import (
	"context"
	"errors"
	"net/http"

	"github.com/donnarec/donnarec-api/internal/auth"
	"github.com/donnarec/donnarec-api/internal/storage"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v5/pgtype"
	"go.uber.org/zap"
)

// PDFRenderer is the subset of *pdf.Renderer the settings handler depends on for the
// real-PDF preview endpoint — declared as an interface so tests can substitute a fake
// where a live chrome sidecar is unavailable. Production wiring passes the EXACT SAME
// *pdf.Renderer instance the outbox worker uses (D-58/D-61: preview must go through the
// identical sandboxed pipeline as production, never a second/less-locked path).
type PDFRenderer interface {
	RenderPDF(ctx context.Context, selfContainedHTML string) ([]byte, error)
}

// Handler handles HTTP requests for the Admin settings endpoints.
type Handler struct {
	svc      *SettingsService
	renderer PDFRenderer
	validate *validator.Validate
	logger   *zap.Logger
}

// NewHandler creates a Handler with the given dependencies.
func NewHandler(svc *SettingsService, renderer PDFRenderer, logger *zap.Logger) *Handler {
	return &Handler{
		svc:      svc,
		renderer: renderer,
		validate: validator.New(),
		logger:   logger,
	}
}

// claimsAndAppUserID extracts both the Keycloak claims and the resolved app_user_id from
// the Gin context (Pattern A, extended with the auth.ResolveAppUser lookup every mutating
// settings handler needs for updated_by). Returns ok=false after already writing the
// appropriate error response — callers must return immediately when ok is false.
func claimsAndAppUserID(c *gin.Context) (auth.KeycloakClaims, pgtype.UUID, bool) {
	raw, exists := c.Get("claims")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing_auth_context"})
		return auth.KeycloakClaims{}, pgtype.UUID{}, false
	}
	claims, ok := raw.(auth.KeycloakClaims)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid_claims_type"})
		return auth.KeycloakClaims{}, pgtype.UUID{}, false
	}

	rawUserID, userExists := c.Get(auth.AppUserIDContextKey)
	if !userExists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "missing_auth_context"})
		return auth.KeycloakClaims{}, pgtype.UUID{}, false
	}
	appUserID, userOK := rawUserID.(pgtype.UUID)
	if !userOK {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid_user_id"})
		return auth.KeycloakClaims{}, pgtype.UUID{}, false
	}

	return claims, appUserID, true
}

// Get returns the merged receipt template + number-format config (Admin-only — enforced by
// adminGroup's RequireRoles(RoleAdmin)).
// GET /api/admin/settings
func (h *Handler) Get(c *gin.Context) {
	// Pattern A: auth claims extraction
	raw, exists := c.Get("claims")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing_auth_context"})
		return
	}
	if _, ok := raw.(auth.KeycloakClaims); !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid_claims_type"})
		return
	}

	resp, err := h.svc.GetSettings(c.Request.Context())
	if err != nil {
		// Pattern C: log operation only — never the template HTML body
		h.logger.Error("failed to get settings",
			zap.String("operation", "GetSettings"),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "get_settings_failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// Save validates then persists ALL settings fields in one request (Admin-only, audited).
// PUT /api/admin/settings
//
// ErrInvalidTemplate     → 422 (template.Parse failed for template_html/template_html_en)
// ErrInvalidNumberFormat → 422 (separator/prefix contains disallowed characters)
func (h *Handler) Save(c *gin.Context) {
	_, appUserID, ok := claimsAndAppUserID(c)
	if !ok {
		return
	}

	var req ReceiptSettings
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

	if err := h.svc.SaveSettings(c.Request.Context(), req, appUserID); err != nil {
		switch {
		case errors.Is(err, ErrInvalidTemplate):
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "invalid_template"})
		case errors.Is(err, ErrInvalidNumberFormat):
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "invalid_number_format"})
		default:
			// Pattern C: log operation only — NEVER the template HTML body (T-03-10 analog)
			h.logger.Error("failed to save settings",
				zap.String("operation", "SaveReceiptTemplateConfig"),
				zap.Error(err),
			)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "save_settings_failed"})
		}
		return
	}

	resp := gin.H{"saved": true}
	// audit_after: captured by AuditMiddleware for the immutable audit trail (Pattern D,
	// D-58 "every settings mutation is Admin-gated and append-only audited").
	c.Set("audit_after", resp)
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// UploadImage validates (magic-byte + 2 MB cap) and stores a brand-image file for the
// given slot (letterhead/seal/signature/watermark), persisting the new object key
// immediately (Admin-only, audited).
// POST /api/admin/settings/images/:slot  (multipart/form-data, field "file")
func (h *Handler) UploadImage(c *gin.Context) {
	_, appUserID, ok := claimsAndAppUserID(c)
	if !ok {
		return
	}

	slot := c.Param("slot")

	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing_file_field", "detail": "multipart field 'file' is required"})
		return
	}

	file, err := fileHeader.Open()
	if err != nil {
		h.logger.Error("failed to open uploaded template image",
			zap.String("operation", "UploadTemplateImage"),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "file_open_failed"})
		return
	}
	defer file.Close()

	objectKey, err := h.svc.SaveTemplateImage(c.Request.Context(), slot, file, fileHeader.Size, appUserID)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidImageSlot):
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_image_slot"})
		case errors.Is(err, storage.ErrTemplateImageTooLarge):
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "file_too_large", "detail": "maximum template image size is 2 MB"})
		case errors.Is(err, storage.ErrUnsupportedTemplateImageType):
			c.JSON(http.StatusUnsupportedMediaType, gin.H{"error": "unsupported_file_type", "detail": "only JPEG and PNG files are accepted"})
		default:
			// Pattern C: log operation + slot only — never the file bytes
			h.logger.Error("failed to upload template image",
				zap.String("operation", "UploadTemplateImage"),
				zap.String("slot", slot),
				zap.Error(err),
			)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "upload_failed"})
		}
		return
	}

	resp := gin.H{"slot": slot, "object_key": objectKey}
	c.Set("audit_after", resp)
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// Preview returns assembled HTML from the CURRENT, UNSAVED template + sample/mock data —
// no Chromium involved (fast path for the 400ms-debounced live preview, D-61).
// POST /api/admin/settings/preview
//
// Never audited (read-only, no persisted side effect) and never logs the template HTML
// body (Pattern C).
func (h *Handler) Preview(c *gin.Context) {
	raw, exists := c.Get("claims")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing_auth_context"})
		return
	}
	if _, ok := raw.(auth.KeycloakClaims); !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid_claims_type"})
		return
	}

	var req PreviewRequest
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

	html, err := h.svc.BuildPreviewHTML(c.Request.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidTemplate):
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "invalid_template"})
		default:
			h.logger.Error("failed to build settings preview",
				zap.String("operation", "PreviewSettings"),
				zap.Error(err),
			)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "preview_failed"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": gin.H{"html": html}})
}

// PreviewPDF renders the CURRENT, UNSAVED template + sample/mock data through the SAME
// sandboxed Chromium pipeline production rendering uses (internal/pdf.RenderPDF, 04-03) —
// the accurate "real PDF" fidelity check (D-61 hybrid preview strategy).
// POST /api/admin/settings/preview/pdf
//
// Returns raw PDF bytes (Content-Type: application/pdf), not the usual JSON envelope —
// the response body IS the artifact being previewed.
func (h *Handler) PreviewPDF(c *gin.Context) {
	raw, exists := c.Get("claims")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing_auth_context"})
		return
	}
	if _, ok := raw.(auth.KeycloakClaims); !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid_claims_type"})
		return
	}

	var req PreviewRequest
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

	html, err := h.svc.BuildPreviewHTML(c.Request.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidTemplate):
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "invalid_template"})
		default:
			h.logger.Error("failed to build settings preview for PDF render",
				zap.String("operation", "PreviewPDFSettings"),
				zap.Error(err),
			)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "preview_failed"})
		}
		return
	}

	pdfBytes, err := h.renderer.RenderPDF(c.Request.Context(), html)
	if err != nil {
		h.logger.Error("failed to render settings preview pdf",
			zap.String("operation", "PreviewPDFSettings"),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "pdf_render_failed"})
		return
	}

	c.Data(http.StatusOK, "application/pdf", pdfBytes)
}
