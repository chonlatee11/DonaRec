package receiptfmt

import (
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
)

// FormatAmount converts a pgtype.Numeric (big.Int mantissa + Exp) to a plain
// decimal string. Handles positive and negative amounts; treats invalid/nil as
// "0".
//
// BI-04 (04-REVIEW-PRESHIP.md): this is the single shared implementation
// previously duplicated verbatim as internal/worker.formatAmount and
// internal/donation.numericStr.
func FormatAmount(n pgtype.Numeric) string {
	if !n.Valid || n.Int == nil {
		return "0"
	}
	// *big.Int.Text(base) returns the string representation; no math/big import
	// needed since we only call a method on the existing *big.Int value.
	intStr := n.Int.Text(10)
	negative := strings.HasPrefix(intStr, "-")
	if negative {
		intStr = intStr[1:]
	}

	var result string
	if n.Exp >= 0 {
		// Positive exponent: append trailing zeros.
		result = intStr + strings.Repeat("0", int(n.Exp))
	} else {
		// Negative exponent: insert decimal point.
		decPlaces := int(-n.Exp)
		for len(intStr) <= decPlaces {
			intStr = "0" + intStr // left-pad to accommodate the decimal
		}
		pos := len(intStr) - decPlaces
		result = intStr[:pos] + "." + intStr[pos:]
	}

	if negative {
		result = "-" + result
	}
	return result
}
