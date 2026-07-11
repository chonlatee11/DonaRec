// Package report — handler.go
//
// Handler provides Gin HTTP handlers for the donation summary report
// endpoints (FR-32, D-70/D-71). Deliberately open to EVERY authenticated
// staff role — the route this handler is wired to (cmd/server/main.go's
// reportGroup) carries NO RequireAnyRole/RequireRoles gate (D-71), because
// the underlying data has no PII column and nothing to reveal.
//
// Sentinel error → HTTP status mapping:
//
//	ErrInvalidGroupBy → 400 Bad Request (also returned directly by the
//	                    handler's own allowlist check before Summary is
//	                    ever called — the service-layer check is
//	                    defense-in-depth)
//	default           → 500 (log operation only)
package report

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/donnarec/donnarec-api/internal/exportfile"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Handler handles HTTP requests for the donation summary report endpoints.
type Handler struct {
	svc    *Service
	logger *zap.Logger
}

// NewHandler creates a Handler with the given dependencies.
func NewHandler(svc *Service, logger *zap.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

// allowedGroupBy is the allowlist for the ?group_by= query param (mirrors
// edonation.Handler's allowedExportFormats discipline — only the two
// granularities Service.Summary actually supports are ever accepted).
var allowedGroupBy = map[string]bool{"month": true, "day": true}

// allowedReportExportFormats is the allowlist for the report export ?format=
// query param.
var allowedReportExportFormats = map[string]bool{"xlsx": true, "csv": true}

// PeriodRowResponse is the JSON representation of one breakdown row.
type PeriodRowResponse struct {
	Period       string  `json:"period"`
	ReceiptCount int     `json:"receipt_count"`
	TotalAmount  float64 `json:"total_amount"`
}

// SummaryResponse is the JSON response body for GET /api/reports/summary.
type SummaryResponse struct {
	TotalAmount       float64             `json:"total_amount"`
	ReceiptCount      int                 `json:"receipt_count"`
	AveragePerReceipt float64             `json:"average_per_receipt"`
	Breakdown         []PeriodRowResponse `json:"breakdown"`
}

func toSummaryResponse(result SummaryResult) SummaryResponse {
	breakdown := make([]PeriodRowResponse, 0, len(result.Breakdown))
	for _, row := range result.Breakdown {
		breakdown = append(breakdown, PeriodRowResponse{
			Period:       row.Period,
			ReceiptCount: row.ReceiptCount,
			TotalAmount:  row.TotalAmount,
		})
	}
	return SummaryResponse{
		TotalAmount:       result.TotalAmount,
		ReceiptCount:      result.ReceiptCount,
		AveragePerReceipt: result.AveragePerReceipt,
		Breakdown:         breakdown,
	}
}

// parseReportDate parses a "YYYY-MM-DD" query param into a time.Time (UTC
// midnight) — mirrors edonation.parseExportDate's convention (unexported
// there, so duplicated here rather than imported).
func parseReportDate(s string) (time.Time, error) {
	return time.ParseInLocation("2006-01-02", s, time.UTC)
}

// parseSummaryFilter binds and validates the shared from/to/group_by query
// params used by both Summary and Export. Returns (filter, ok) — ok=false
// means the handler already wrote an error response and the caller must
// return immediately.
func parseSummaryFilter(c *gin.Context) (SummaryFilter, bool) {
	groupBy := c.DefaultQuery("group_by", "month")
	if !allowedGroupBy[groupBy] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_group_by", "detail": "group_by must be month or day"})
		return SummaryFilter{}, false
	}

	filter := SummaryFilter{GroupBy: groupBy}
	if from := c.Query("from"); from != "" {
		t, parseErr := parseReportDate(from)
		if parseErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_from_date"})
			return SummaryFilter{}, false
		}
		filter.From = &t
	}
	if to := c.Query("to"); to != "" {
		t, parseErr := parseReportDate(to)
		if parseErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_to_date"})
			return SummaryFilter{}, false
		}
		filter.To = &t
	}
	if filter.From != nil && filter.To != nil && filter.From.After(*filter.To) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_date_range", "detail": "from must not be after to"})
		return SummaryFilter{}, false
	}

	return filter, true
}

// Summary returns the PII-free donation summary report (FR-32, D-70/D-71).
// GET /api/reports/summary?from=&to=&group_by=month|day
//
// Open to ALL authenticated staff (Maker/Checker/Admin) — the route this
// handler is wired to carries no role gate (D-71); every caller that passes
// RequireAuth reaches this handler.
func (h *Handler) Summary(c *gin.Context) {
	// Pattern A: auth claims extraction — the report needs no role from the
	// claims (D-71: no RBAC check), but the presence check still guards
	// against a misconfigured route that somehow skipped RequireAuth.
	if _, exists := c.Get("claims"); !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing_auth_context"})
		return
	}

	filter, ok := parseSummaryFilter(c)
	if !ok {
		return
	}

	result, err := h.svc.Summary(c.Request.Context(), filter)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidGroupBy):
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_group_by"})
		default:
			h.logger.Error("failed to compute donation summary report",
				zap.String("operation", "ReportSummary"),
				zap.Error(err),
			)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "report_summary_failed"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": toSummaryResponse(result)})
}

// Export streams the same PII-free summary report as an .xlsx/.csv file via
// the shared exportfile writer (FR-32). GET
// /api/reports/export?from=&to=&group_by=month|day&format=xlsx|csv
//
// NO confirmation gate and NO audit-reveal event — unlike
// internal/edonation.Handler.Export, there is zero PII in this data to warn
// about or to audit a reveal of (D-71 spirit: this report is meant to be
// transparently available to all staff).
func (h *Handler) Export(c *gin.Context) {
	if _, exists := c.Get("claims"); !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing_auth_context"})
		return
	}

	format := c.DefaultQuery("format", "xlsx")
	if !allowedReportExportFormats[format] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_format", "detail": "format must be xlsx or csv"})
		return
	}

	filter, ok := parseSummaryFilter(c)
	if !ok {
		return
	}

	result, err := h.svc.Summary(c.Request.Context(), filter)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidGroupBy):
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_group_by"})
		default:
			h.logger.Error("failed to compute donation report export",
				zap.String("operation", "ReportExport"),
				zap.Error(err),
			)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "report_export_failed"})
		}
		return
	}

	headers := []string{"ช่วงเวลา", "จำนวนใบเสร็จ", "ยอดรวม (บาท)"}
	rows := make([][]string, 0, len(result.Breakdown)+1)
	for _, row := range result.Breakdown {
		rows = append(rows, []string{
			row.Period,
			strconv.Itoa(row.ReceiptCount),
			formatAmount(row.TotalAmount),
		})
	}
	// A trailing summary row — total across every breakdown bucket (not a
	// PII row, just the same top-line SummaryResult.TotalAmount/ReceiptCount
	// already returned by Summary).
	rows = append(rows, []string{"รวมทั้งหมด", strconv.Itoa(result.ReceiptCount), formatAmount(result.TotalAmount)})

	asciiName := "donation-report." + format
	utf8Name := "รายงานสรุปการบริจาค." + format
	contentType := "text/csv; charset=utf-8"
	if format == "xlsx" {
		contentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	}
	exportfile.SetDownloadHeaders(c.Writer, contentType, asciiName, utf8Name)

	var writeErr error
	switch format {
	case "xlsx":
		writeErr = exportfile.StreamXLSX(c.Writer, "รายงานสรุป", headers, rows)
	case "csv":
		writeErr = exportfile.StreamCSV(c.Writer, headers, rows)
	}
	if writeErr != nil {
		h.logger.Error("failed to stream donation report export file",
			zap.String("operation", "ReportExport"),
			zap.Int("row_count", len(rows)),
			zap.Error(writeErr),
		)
		return
	}

	h.logger.Info("donation report export streamed",
		zap.String("operation", "ReportExport"),
		zap.Int("row_count", len(rows)),
		zap.String("format", format),
	)
}

// formatAmount formats a float64 amount with exactly 2 decimal places —
// mirrors donation/service.go's strconv.FormatFloat(amount, 'f', 2, 64)
// convention for money display in export files.
func formatAmount(f float64) string {
	return strconv.FormatFloat(f, 'f', 2, 64)
}
