package report

import "errors"

// ErrInvalidGroupBy is returned by Service.Summary when GroupBy is anything
// other than "month" or "day" — service-layer defense-in-depth beyond the
// handler's own allowlist validation (mirrors edonation.Handler.Export's
// format-allowlist discipline).
// HTTP mapping: 400 Bad Request.
var ErrInvalidGroupBy = errors.New("report: group_by must be \"month\" or \"day\"")
