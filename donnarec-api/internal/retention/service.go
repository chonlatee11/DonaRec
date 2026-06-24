// Package retention provides data-retention policy enforcement for donnarec-api.
//
// Design (D-18, D-19, NFR-03):
//
//	Retention = config-driven: retain_until is computed from RetentionConfig values.
//	No hardcoded day/year constants in service logic — config is the single source of truth.
//
//	Legal hold = defense-in-depth:
//	  - App layer: GuardHardDelete checks the in-memory legal_hold flag.
//	  - DB layer:  prevent_legal_hold_delete trigger (migration 000003) blocks DELETE.
//	Both layers must agree; the DB trigger is a backstop against app-level bugs or
//	direct SQL access.
//
//	Soft delete is ALWAYS allowed regardless of legal_hold. Soft delete sets
//	is_active=false (or a status field) without issuing a SQL DELETE statement.
//	Only hard DELETE is blocked by legal_hold.
//
// Phase scope:
//   - Phase 1 (01-03): service boundary + unit tests + integration test against users table.
//   - Phase 3: wire retain_until/legal_hold onto donor/donation records at insert time.
package retention

import (
	"context"
	"fmt"
	"time"

	"github.com/donnarec/donnarec-api/internal/config"
)

// ComputeRetainUntil calculates the retain_until timestamp for a given entity type.
//
// The calculation is driven entirely by RetentionConfig — no literal day counts
// appear in this function (D-18 config-driven requirement).
//
// Supported entity types:
//   - "donation": uses cfg.DonationRetainDays (default: 1825 = 5 years, pending DPO)
//   - "audit_log": uses cfg.AuditLogRetainDays (default: 3650 = 10 years, tax audit)
//   - anything else: falls back to DonationRetainDays (safe default)
//
// The `from` parameter is typically the record's created_at timestamp.
// Asia/Bangkok timezone considerations are the caller's responsibility;
// this function operates on UTC time.Time values.
func ComputeRetainUntil(entityType string, from time.Time, cfg config.RetentionConfig) time.Time {
	var days int
	switch entityType {
	case "audit_log":
		days = cfg.AuditLogRetainDays
	case "donation":
		days = cfg.DonationRetainDays
	default:
		// Unknown entity type — use the donation default as a conservative fallback.
		// Phase 3 adds "donor" entity type; this fallback keeps the service stable.
		days = cfg.DonationRetainDays
	}
	return from.Add(time.Duration(days) * 24 * time.Hour)
}

// GuardHardDelete is the application-level guard against hard-deleting a record
// that is under legal hold (D-19).
//
// This function must be called BEFORE issuing any SQL DELETE statement on a
// record that has a legal_hold field. If it returns an error, the DELETE must
// be aborted — no SQL must reach the database.
//
// Defense-in-depth: even if this guard is bypassed (e.g., by a code path bug),
// the database trigger `prevent_legal_hold_delete` (migration 000003) will block
// the DELETE at the PostgreSQL level and return a RAISE EXCEPTION error.
//
// Error contract: returns a typed AppError with i18n key
// "retention.legal_hold_delete_blocked" so that callers can localize the message.
//
// Soft delete bypass: this function is NOT called for soft deletes (is_active=false
// updates). Soft deletes are always permitted regardless of legal_hold.
func GuardHardDelete(ctx context.Context, recordLegalHold bool) error {
	if !recordLegalHold {
		return nil
	}
	// Return an AppError carrying the i18n message key.
	// The HTTP handler translates this key into a localized error message.
	return &AppError{
		Code:    "retention.legal_hold_delete_blocked",
		Message: "retention.legal_hold_delete_blocked: cannot hard-delete a record under legal hold",
	}
}

// SoftDelete simulates a soft-delete operation (sets is_active=false or status=inactive).
// It does NOT issue a SQL DELETE and therefore is NOT subject to legal_hold restrictions.
//
// In a real implementation, this would update the record's is_active field in the DB.
// This service-layer function returns nil for all inputs to document the invariant:
// soft delete is always allowed regardless of legal_hold status (D-19).
//
// Phase 3 will implement actual DB update via sqlc/pgx when donor/donation tables exist.
// For Phase 1 (users only), the UserService.DeactivateUser method performs the actual
// UPDATE is_active=false.
func SoftDelete(_ context.Context, _ bool) error {
	// Soft delete is always permitted regardless of legal_hold.
	// The caller updates is_active=false (or equivalent status field) via SQL UPDATE.
	// No SQL DELETE is issued, so the legal_hold trigger never fires.
	return nil
}

// AppError is a typed error carrying an i18n message key.
// HTTP handlers use the Code field to look up a localized error message in the
// go-i18n bundle. The Message field is for internal logging (never sent to client).
type AppError struct {
	// Code is the i18n message key (e.g., "retention.legal_hold_delete_blocked").
	Code string
	// Message is an internal human-readable description (not localized, not sent to client).
	Message string
}

func (e *AppError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}
