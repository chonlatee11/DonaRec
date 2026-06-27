// Package receiptno — format.go
//
// formatReceiptNo renders the frozen formatted receipt number string (D-42) from its
// component parts. This string is persisted in the ledger at allocation time and NEVER
// re-rendered from config — changing config does not alter previously issued numbers.
//
// Decisions implemented:
//   - D-28: default format = BE4 year + "/" + zero-padded 6-digit running number
//   - D-29: padding is MINIMUM width; values wider than padding expand naturally (no truncation/error)
//   - D-30: all components (separator, padding, year-format, prefix) are configurable at runtime
//   - D-42: caller persists the returned string; subsequent reads use the stored snapshot
package receiptno

import (
	"fmt"
	"regexp"
)

// configCharAllowlist constrains the admin-configurable prefix/separator components.
// The formatted string is frozen into the immutable ledger (REVOKE UPDATE, DELETE) and
// later rendered into PDFs via an HTML→headless-Chromium pipeline (see CLAUDE.md), so a
// dangerous value such as prefix="<img src=x onerror=...>" would become a permanent stored
// markup/injection payload that no UPDATE can scrub. Defense-in-depth: reject anything
// outside a conservative safe set (alphanumerics, space, and "_./-") at the formatting
// boundary so the tainted value never reaches the ledger. The DB-level CHECK constraint
// suggested in review is deferred — this Go-side guard at the single allocation path
// covers Phase 2 without altering the already-applied 000004 migration.
var configCharAllowlist = regexp.MustCompile(`^[A-Za-z0-9 _./-]*$`)

// formatReceiptNo renders the receipt number string from its primitive components.
//
// Signature uses primitives (not db.GetReceiptNumberConfigRow) so this helper is
// independent of sqlc-generated types and usable in tests without a DB. The allocator
// adapts the sqlc row to these arguments (per interfaces note in 02-02-PLAN.md).
//
// Parameters:
//   - fiscalYear  : Thai BE fiscal year (e.g. 2569)
//   - runningNo   : sequential number within the fiscal year (e.g. 123)
//   - separator   : string between year and running number (e.g. "/")
//   - padding     : minimum digit width for running number (e.g. 6 → "000123")
//   - yearFormat  : "BE4" → %04d of fiscalYear; "CE4" → %04d of (fiscalYear-543); default → BE4
//   - prefix      : prepended before year string (e.g. "HOSP" → "HOSP2569/000123")
//
// D-29 min-width guarantee: fmt.Sprintf("%0*d", padding, runningNo) with the * verb uses
// padding as minimum width. When runningNo has more digits than padding the output expands
// naturally — it is never truncated and never returns an error.
//
// Examples:
//
//	formatReceiptNo(2569, 123,     "/", 6, "BE4",  "")     → "2569/000123"
//	formatReceiptNo(2569, 1000000, "/", 6, "BE4",  "")     → "2569/1000000"  (D-29: expands)
//	formatReceiptNo(2569, 5,       "-", 4, "BE4",  "HOSP") → "HOSP2569-0005"
//	formatReceiptNo(2569, 7,       "/", 6, "CE4",  "")     → "2026/000007"   (CE 2569-543=2026)
//
// It returns an error if separator or prefix contains a character outside the safe
// allowlist (see configCharAllowlist) — the allocator propagates this so the surrounding
// transaction rolls back and no tainted value is frozen into the immutable ledger.
func formatReceiptNo(fiscalYear int, runningNo int, separator string, padding int, yearFormat string, prefix string) (string, error) {
	// Defense-in-depth: validate admin-configurable components before they are frozen.
	if !configCharAllowlist.MatchString(prefix) {
		return "", fmt.Errorf("format receipt no: prefix %q contains disallowed characters (allowed: A-Za-z0-9 _./-)", prefix)
	}
	if !configCharAllowlist.MatchString(separator) {
		return "", fmt.Errorf("format receipt no: separator %q contains disallowed characters (allowed: A-Za-z0-9 _./-)", separator)
	}

	// Render fiscal year string
	var yearStr string
	switch yearFormat {
	case "CE4":
		// Christian Era year: subtract 543 from Buddhist Era year
		yearStr = fmt.Sprintf("%04d", fiscalYear-543)
	default:
		// "BE4" and all other values → Buddhist Era year, 4 digits zero-padded
		yearStr = fmt.Sprintf("%04d", fiscalYear)
	}

	// Render running number with minimum-width padding (D-29).
	// %0*d: '0' = zero-pad, '*' = width from next argument, 'd' = decimal integer.
	// When runningNo > padding digits wide, the output is wider than padding — no truncation.
	runningStr := fmt.Sprintf("%0*d", padding, runningNo)

	// Assemble: prefix + year + separator + running_no
	return prefix + yearStr + separator + runningStr, nil
}
