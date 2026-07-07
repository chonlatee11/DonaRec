// Package edonation — handler.go
//
// Handler provides Gin HTTP handlers for the e-Donation export endpoint (FR-30,
// D-63/D-74/D-75) and the Admin-only edonation_config accessor route (D-75/NFR-09).
// Follows the same conventions as internal/donation/handler.go and
// internal/settings/handler.go:
//   - Pattern A: claims extraction block
//   - Pattern C: no PII in logs — only donation count, never plaintext IDs
//
// Sentinel error → HTTP status mapping:
//
//	ErrForbidden  → 403 Forbidden (service-layer defense-in-depth; the real
//	                authority is the RequireAnyRole route guard)
//	ErrNoRecords  → 404 Not Found (D-74 spirit: never stream an empty file)
//	default       → 500 (log operation + count only — Pattern C)
package edonation

import (
	"errors"
	"net/http"
	"time"

	"github.com/donnarec/donnarec-api/internal/auth"
	"github.com/donnarec/donnarec-api/internal/exportfile"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/jackc/pgx/v5/pgtype"
	"go.uber.org/zap"
)

// Handler handles HTTP requests for the e-Donation export + config endpoints.
type Handler struct {
	svc      *Service
	cfg      *Config
	validate *validator.Validate
	logger   *zap.Logger
}

// NewHandler creates a Handler with the given dependencies.
func NewHandler(svc *Service, cfg *Config, logger *zap.Logger) *Handler {
	return &Handler{
		svc:      svc,
		cfg:      cfg,
		validate: validator.New(),
		logger:   logger,
	}
}

// allowedExportFormats is the allowlist for the ?format= query param (D-74 spirit —
// only the two formats this package actually streams are ever accepted).
var allowedExportFormats = map[string]bool{"xlsx": true, "csv": true}

// Export streams an audited, RBAC-gated .xlsx/.csv export of issued donations
// mapped to e-Donation fields (FR-30, D-63/D-64/D-74/D-75).
// GET /api/edonation/export?from=&to=&keyed_status=&format=xlsx|csv&locale=th|en
//
// RBAC: enforced by the route guard (RequireAnyRole(Checker,Admin), D-63) AND
// service-layer defense-in-depth (Service.Export's role gate).
// Empty result set → 404 (no empty-file round trip).
// Never holds a DB transaction open across the workbook build/stream (Service.Export
// already committed before this handler ever calls WriteXLSX/WriteCSV — Pitfall 3).
func (h *Handler) Export(c *gin.Context) {
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

	format := c.DefaultQuery("format", "xlsx")
	if !allowedExportFormats[format] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_format", "detail": "format must be xlsx or csv"})
		return
	}

	locale := c.DefaultQuery("locale", "th")

	filter := ExportFilter{Format: format}
	if from := c.Query("from"); from != "" {
		t, parseErr := parseExportDate(from)
		if parseErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_from_date"})
			return
		}
		filter.From = &t
	}
	if to := c.Query("to"); to != "" {
		t, parseErr := parseExportDate(to)
		if parseErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_to_date"})
			return
		}
		filter.To = &t
	}
	if keyedStatus := c.Query("keyed_status"); keyedStatus != "" {
		switch keyedStatus {
		case "true":
			v := true
			filter.KeyedStatus = &v
		case "false":
			v := false
			filter.KeyedStatus = &v
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_keyed_status", "detail": "keyed_status must be true or false"})
			return
		}
	}

	rows, err := h.svc.Export(c.Request.Context(), filter, claims)
	if err != nil {
		switch {
		case errors.Is(err, ErrForbidden):
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		default:
			// Pattern C: log operation only — never PII
			h.logger.Error("failed to export e-donation data",
				zap.String("operation", "EdonationExport"),
				zap.Error(err),
			)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "export_failed"})
		}
		return
	}

	// D-74 spirit: never stream a zero-row file — 404 before any workbook build.
	if len(rows) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "no_records"})
		return
	}

	cfg, err := h.cfg.GetConfig(c.Request.Context())
	if err != nil {
		h.logger.Error("failed to load edonation config for export",
			zap.String("operation", "EdonationExport"),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "export_failed"})
		return
	}

	asciiName := "edonation-export." + format
	utf8Name := "e-Donation-ส่งออก." + format
	contentType := "text/csv; charset=utf-8"
	if format == "xlsx" {
		contentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	}
	exportfile.SetDownloadHeaders(c.Writer, contentType, asciiName, utf8Name)

	var writeErr error
	switch format {
	case "xlsx":
		writeErr = WriteXLSX(c.Writer, cfg.FieldMapping, cfg.CashTypeLabel, locale, rows)
	case "csv":
		writeErr = WriteCSV(c.Writer, cfg.FieldMapping, cfg.CashTypeLabel, locale, rows)
	}
	if writeErr != nil {
		// Pattern C: log count only — headers/body may already be partially flushed,
		// but never log PII.
		h.logger.Error("failed to stream e-donation export file",
			zap.String("operation", "EdonationExport"),
			zap.Int("count", len(rows)),
			zap.Error(writeErr),
		)
		return
	}

	// Pattern C: log count only — never PII (T-05-02-LOGPII).
	h.logger.Info("e-donation export streamed",
		zap.String("operation", "EdonationExport"),
		zap.Int("count", len(rows)),
		zap.String("format", format),
	)
}

// ConfigRequest is the JSON request body for PUT /api/admin/edonation-config
// (D-75/NFR-09).
type ConfigRequest struct {
	FieldMapping  []FieldMappingColumn `json:"field_mapping" validate:"required,min=1,dive"`
	CashTypeLabel string               `json:"cash_type_label" validate:"required,max=255"`
	NearDueDays   int                  `json:"near_due_days" validate:"required,gt=0"`
}

// ConfigResponse is the JSON response body for GET/PUT /api/admin/edonation-config —
// a clean snake_case DTO (edonation.Config itself carries no json tags, since it
// doubles as the accessor).
type ConfigResponse struct {
	FieldMapping  []FieldMappingColumn `json:"field_mapping"`
	CashTypeLabel string               `json:"cash_type_label"`
	NearDueDays   int                  `json:"near_due_days"`
	UpdatedAt     time.Time            `json:"updated_at"`
	UpdatedBy     string               `json:"updated_by"`
}

func toConfigResponse(cfg Config) ConfigResponse {
	return ConfigResponse{
		FieldMapping:  cfg.FieldMapping.Columns,
		CashTypeLabel: cfg.CashTypeLabel,
		NearDueDays:   cfg.NearDueDays,
		UpdatedAt:     cfg.UpdatedAt,
		UpdatedBy:     cfg.UpdatedBy,
	}
}

// GetConfig returns the current e-Donation field-mapping/cash-type-label/near-due-
// days config (Admin-only — enforced by adminGroup's RequireRoles(RoleAdmin)).
// GET /api/admin/edonation-config
func (h *Handler) GetConfig(c *gin.Context) {
	raw, exists := c.Get("claims")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing_auth_context"})
		return
	}
	if _, ok := raw.(auth.KeycloakClaims); !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid_claims_type"})
		return
	}

	cfg, err := h.cfg.GetConfig(c.Request.Context())
	if err != nil {
		h.logger.Error("failed to get edonation config",
			zap.String("operation", "GetEdonationConfig"),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "get_config_failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": toConfigResponse(cfg)})
}

// UpdateConfig persists the e-Donation field mapping / cash-type label / near-due-
// days threshold (Admin-only, audited, D-75/NFR-09 — configurable without a deploy).
// PUT /api/admin/edonation-config
func (h *Handler) UpdateConfig(c *gin.Context) {
	raw, exists := c.Get("claims")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing_auth_context"})
		return
	}
	if _, ok := raw.(auth.KeycloakClaims); !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid_claims_type"})
		return
	}

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

	var req ConfigRequest
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

	cfg := Config{
		FieldMapping:  FieldMapping{Columns: req.FieldMapping},
		CashTypeLabel: req.CashTypeLabel,
		NearDueDays:   req.NearDueDays,
	}
	if err := h.cfg.UpdateConfig(c.Request.Context(), cfg, appUserID); err != nil {
		h.logger.Error("failed to update edonation config",
			zap.String("operation", "UpdateEdonationConfig"),
			zap.Error(err),
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "update_config_failed"})
		return
	}

	resp := gin.H{"saved": true}
	c.Set("audit_after", resp)
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// parseExportDate parses a "YYYY-MM-DD" query param into a time.Time (UTC midnight) —
// mirrors internal/donation/handler.go's parseDate helper (unexported there, so
// duplicated here rather than imported).
func parseExportDate(s string) (time.Time, error) {
	return time.ParseInLocation("2006-01-02", s, time.UTC)
}
