// Package edonation — service.go
//
// Service.Export implements the audited, stream-only e-Donation export source
// (FR-30/SC#1, D-64/D-66/D-74). It mirrors internal/donation.DonationService's
// RevealPII audited-decrypt discipline exactly (05-RESEARCH.md Pattern 3):
//
//  1. Role gate: Checker/Admin only (service-layer defense-in-depth over the
//     RequireAnyRole route guard).
//  2. Within ONE WithTx: query issued-only rows (SearchIssuedForExport), decrypt
//     each row's donor_tax_id_enc/dek via crypto.DecryptField, append exactly ONE
//     summary audit row (action "edonation.export", count/from/to/keyed_status),
//     then commit.
//  3. Return the decrypted rows to the caller ONLY after commit.
//
// The transaction closure is scoped to query+decrypt+audit ONLY — workbook build/
// stream happens in the HANDLER, after Export returns (05-RESEARCH.md Pitfall 3:
// never hold a DB tx open across a slow workbook build). This file therefore never
// imports internal/exportfile — see xlsx.go/csv.go for the streaming adapters.
package edonation

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/donnarec/donnarec-api/internal/audit"
	"github.com/donnarec/donnarec-api/internal/auth"
	"github.com/donnarec/donnarec-api/internal/crypto"
	dbhelpers "github.com/donnarec/donnarec-api/internal/db"
	db "github.com/donnarec/donnarec-api/internal/db/generated"
)

// Service implements the e-Donation export source (FR-30, plan 05-02).
// All dependencies are constructor-injected (no global state — mirrors
// donation.DonationService's Pattern B).
type Service struct {
	pool        *pgxpool.Pool
	queries     *db.Queries
	auditSvc    *audit.AuditService
	keyProvider crypto.KeyProvider
	logger      *zap.Logger
}

// NewService constructs a Service with injected dependencies.
func NewService(
	pool *pgxpool.Pool,
	queries *db.Queries,
	auditSvc *audit.AuditService,
	keyProvider crypto.KeyProvider,
	logger *zap.Logger,
) *Service {
	return &Service{
		pool:        pool,
		queries:     queries,
		auditSvc:    auditSvc,
		keyProvider: keyProvider,
		logger:      logger,
	}
}

// Export returns the decrypted, issued-only donation rows matching filter,
// auditing exactly ONE summary row (actor/range/count) inside the SAME
// transaction as the decrypt — committed BEFORE plaintext is ever returned
// (D-64, T-05-02-UNAUDITED).
//
// Role gate: Checker/Admin only — ErrForbidden otherwise (T-05-02-RBAC defense-
// in-depth; the real authority is the RequireAnyRole route guard in cmd/server/main.go).
// Cancelled/draft/rejected donations are excluded by SearchIssuedForExport's
// WHERE status='issued' predicate (D-66). date-range + keyed-status filters, if
// set, are applied by the same query.
func (s *Service) Export(ctx context.Context, filter ExportFilter, claims auth.KeycloakClaims) ([]ExportRow, error) {
	// Role gate — reject before any DB call (mirrors RevealPII's D-46 discipline).
	if !claims.HasRole(auth.RoleChecker) && !claims.HasRole(auth.RoleAdmin) {
		return nil, ErrForbidden
	}

	var fromDate, toDate pgtype.Date
	if filter.From != nil {
		fromDate = pgtype.Date{Time: *filter.From, Valid: true}
	}
	if filter.To != nil {
		toDate = pgtype.Date{Time: *filter.To, Valid: true}
	}

	var rows []ExportRow
	err := dbhelpers.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		qtx := s.queries.WithTx(tx)

		dbRows, queryErr := qtx.SearchIssuedForExport(ctx, db.SearchIssuedForExportParams{
			FromDate:    fromDate,
			ToDate:      toDate,
			KeyedStatus: filter.KeyedStatus,
		})
		if queryErr != nil {
			return fmt.Errorf("edonation: search issued for export: %w", queryErr)
		}

		rows = make([]ExportRow, 0, len(dbRows))
		for _, r := range dbRows {
			// D-64: decrypt only inside this audited transaction — never before,
			// never outside a committed summary audit row.
			plaintext, decErr := crypto.DecryptField(ctx, s.keyProvider, r.DonorTaxIDEnc, r.DonorTaxIDDek)
			if decErr != nil {
				return fmt.Errorf("edonation: decrypt donor tax id: %w", decErr)
			}

			receiptFormatted := ""
			if r.ReceiptFormatted != nil {
				receiptFormatted = *r.ReceiptFormatted
			}

			rows = append(rows, ExportRow{
				NationalID:       string(plaintext),
				DonatedAt:        dateStr(r.DonatedAt),
				ReceiptFormatted: receiptFormatted,
				DonorName:        r.DonorName,
			})
		}

		// Exactly ONE summary audit row per export event (T-05-02-UNAUDITED, D-64) —
		// NOT one row per donation. Committed BEFORE plaintext is returned to the
		// caller: if AppendAuditEntryTx fails, the whole export is rolled back and
		// no plaintext is ever handed back.
		afterJSON, _ := json.Marshal(map[string]any{
			"count":        len(rows),
			"from":         optionalDateStr(filter.From),
			"to":           optionalDateStr(filter.To),
			"keyed_status": filter.KeyedStatus,
		})
		if auditErr := s.auditSvc.AppendAuditEntryTx(ctx, tx, audit.AuditEntry{
			ActorID:    claims.Subject,
			ActorEmail: claims.ActorIdentity(),
			Action:     "edonation.export",
			Resource:   "/api/edonation/export",
			AfterJSON:  afterJSON,
		}); auditErr != nil {
			return fmt.Errorf("edonation: audit export: %w", auditErr)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Pattern C (mirrors donation package): log count + actor only — NEVER the
	// plaintext national IDs (T-05-02-LOGPII).
	s.logger.Info("edonation export",
		zap.Int("count", len(rows)),
		zap.String("actor", claims.Subject),
	)

	return rows, nil
}

// dateStr converts a pgtype.Date to a "YYYY-MM-DD" string, or "" if invalid.
// Duplicated (rather than imported) from internal/donation's private helper of the
// same name/behavior — that helper is unexported in a different package.
func dateStr(d pgtype.Date) string {
	if !d.Valid {
		return ""
	}
	return d.Time.Format("2006-01-02")
}

// optionalDateStr formats a *time.Time as "YYYY-MM-DD" for the audit AfterJSON
// snapshot, or "" if nil (no filter was set).
func optionalDateStr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02")
}
