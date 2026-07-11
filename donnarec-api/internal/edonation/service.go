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

// SetKeyed marks or unmarks "คีย์เข้า e-Donation แล้ว" for a bulk or per-row
// selection of donations (FR-31/SC#2, D-67, T-05-04-AUDITGAP). Within ONE
// WithTx:
//
//  1. Role gate: Checker/Admin only (service-layer defense-in-depth, mirrors
//     Export's discipline).
//  2. Determine which of req.DonationIDs are CURRENTLY status='issued' — the
//     status='issued' scope guard lives on this pre-update SELECT, not just
//     the UPDATE's WHERE clause, so the audit-write loop below can write
//     exactly one row per donation THAT ACTUALLY MATCHED the guard (a
//     cancelled/draft id passed in the same request is silently excluded
//     from both the UPDATE and the audit trail — T-05-04-IDOR).
//  3. SetKeyedBulk (plain boolean UPDATE, no allocator/locking machinery —
//     05-RESEARCH.md Anti-Patterns) scoped to status='issued'.
//  4. Append ONE audit row PER matched donation_id (action
//     "edonation.mark_keyed" or "edonation.unmark_keyed", D-67 — distinct
//     from Export's single-summary-row rationale, Pattern 4) — if any
//     AppendAuditEntryTx call fails, the whole mutation rolls back (D-67
//     "ทุกการติ๊ก/เอาออก audit" — never a silent partial write).
//  5. Commit.
func (s *Service) SetKeyed(ctx context.Context, req KeyedRequest, claims auth.KeycloakClaims, actorUserID pgtype.UUID) error {
	// Role gate — reject before any DB call (mirrors Export's D-46 discipline).
	if !claims.HasRole(auth.RoleChecker) && !claims.HasRole(auth.RoleAdmin) {
		return ErrForbidden
	}

	if len(req.DonationIDs) == 0 {
		return nil
	}

	action := "edonation.mark_keyed"
	if !req.Keyed {
		action = "edonation.unmark_keyed"
	}

	var keyedAt pgtype.Timestamptz
	var keyedBy pgtype.UUID
	if req.Keyed {
		keyedAt = pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
		keyedBy = actorUserID
	}

	return dbhelpers.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		qtx := s.queries.WithTx(tx)

		// Step 2: pre-update scope guard (T-05-04-IDOR + WR-01) — determines
		// exactly which selected ids will ACTUALLY transition BEFORE the bulk
		// UPDATE runs, so the per-donation audit loop below matches the UPDATE's
		// real blast radius precisely. Two exclusions apply here:
		//   - status <> 'issued' (a cancelled id in the same request)
		//   - edonation_keyed already equals the requested target
		// The second guard (WR-02) means marking a mixed selection of
		// already-keyed + not-yet-keyed rows only rewrites the not-yet-keyed
		// subset: already-keyed rows keep their original keyed_at/keyed_by
		// provenance and produce no misleading "new keying event" audit row.
		rows, queryErr := tx.Query(ctx,
			`SELECT id FROM donations WHERE id = ANY($1::uuid[]) AND status = 'issued' AND edonation_keyed <> $2`,
			req.DonationIDs, req.Keyed)
		if queryErr != nil {
			return fmt.Errorf("edonation: select issued scope for keyed mutation: %w", queryErr)
		}
		var issuedIDs []pgtype.UUID
		for rows.Next() {
			var id pgtype.UUID
			if scanErr := rows.Scan(&id); scanErr != nil {
				rows.Close()
				return fmt.Errorf("edonation: scan issued scope row: %w", scanErr)
			}
			issuedIDs = append(issuedIDs, id)
		}
		rows.Close()
		if rowsErr := rows.Err(); rowsErr != nil {
			return fmt.Errorf("edonation: iterate issued scope rows: %w", rowsErr)
		}

		if len(issuedIDs) == 0 {
			// Nothing in the selection actually transitions — every id is either
			// not issued or already at the requested keyed value — so this is a
			// no-op (T-05-04-IDOR / WR-02): no UPDATE, no audit rows, no error (a
			// legitimate empty result, not a failure).
			return nil
		}

		// Step 3: bulk UPDATE — status='issued' guard duplicated here as
		// defense-in-depth (mirrors donations.sql's other lifecycle UPDATEs);
		// the pre-update SELECT above is what the audit loop actually trusts.
		if setErr := qtx.SetKeyedBulk(ctx, db.SetKeyedBulkParams{
			Keyed:       req.Keyed,
			KeyedAt:     keyedAt,
			KeyedBy:     keyedBy,
			DonationIds: issuedIDs,
		}); setErr != nil {
			return fmt.Errorf("edonation: set keyed bulk: %w", setErr)
		}

		// Step 4: exactly ONE audit row PER matched donation (D-67) — NOT one
		// summary row (Pattern 4, distinct from Export's Pattern 3 rationale).
		for _, id := range issuedIDs {
			afterJSON, _ := json.Marshal(map[string]any{
				"donation_id": id.String(),
				"keyed":       req.Keyed,
			})
			if auditErr := s.auditSvc.AppendAuditEntryTx(ctx, tx, audit.AuditEntry{
				ActorID:    claims.Subject,
				ActorEmail: claims.ActorIdentity(),
				Action:     action,
				Resource:   "/api/edonation/keyed",
				AfterJSON:  afterJSON,
			}); auditErr != nil {
				return fmt.Errorf("edonation: audit %s for donation %s: %w", action, id.String(), auditErr)
			}
		}

		return nil
	})
}

// Aging returns the 3-bucket (not_due/near_due/overdue) aging view of all
// unkeyed issued donations (FR-31/SC#2, D-68). now and nearDueDays are always
// caller-supplied (handler resolves now from an optional query param or the
// wall clock, and nearDueDays from edonation_config via Config.GetConfig) —
// Service.Aging itself never reads the wall clock or the config table
// directly, keeping computeBucket's pure/testable discipline all the way up
// to this method (mirrors aging.go's own "caller passes now" contract).
//
// Role gate: Checker/Admin only — ErrForbidden otherwise (T-05-04-RBAC
// defense-in-depth; the real authority is the RequireAnyRole route guard).
func (s *Service) Aging(ctx context.Context, claims auth.KeycloakClaims, now time.Time, nearDueDays int) (AgingResult, error) {
	if !claims.HasRole(auth.RoleChecker) && !claims.HasRole(auth.RoleAdmin) {
		return AgingResult{}, ErrForbidden
	}

	dbRows, err := s.queries.SearchUnkeyedIssued(ctx)
	if err != nil {
		return AgingResult{}, fmt.Errorf("edonation: search unkeyed issued: %w", err)
	}

	result := AgingResult{
		Rows: make([]AgingRow, 0, len(dbRows)),
		Counts: map[AgingBucket]int{
			BucketNotDue:  0,
			BucketNearDue: 0,
			BucketOverdue: 0,
		},
	}
	for _, r := range dbRows {
		receiptFormatted := ""
		if r.ReceiptFormatted != nil {
			receiptFormatted = *r.ReceiptFormatted
		}

		approvedAt := r.ApprovedAt.Time
		bucket := computeBucket(approvedAt, now, nearDueDays)
		result.Counts[bucket]++

		result.Rows = append(result.Rows, AgingRow{
			ID:               r.ID.String(),
			DonorName:        r.DonorName,
			ReceiptFormatted: receiptFormatted,
			DonatedAt:        dateStr(r.DonatedAt),
			ApprovedAt:       approvedAt,
			Deadline:         computeDeadline(approvedAt),
			Bucket:           bucket,
			Keyed:            r.EdonationKeyed,
		})
	}

	return result, nil
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
