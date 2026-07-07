package edonation

import "errors"

// Sentinel errors used by Service and mapped to HTTP status codes by Handler
// (mirrors internal/donation/errors.go's convention).
var (
	// ErrForbidden is returned by Export when the caller holds neither Checker nor
	// Admin role — service-layer defense-in-depth beyond the HTTP RequireAnyRole
	// route guard (T-05-02-RBAC).
	// HTTP mapping: 403 Forbidden.
	ErrForbidden = errors.New("edonation: forbidden — checker or admin role required")

	// ErrNoRecords is returned when an export filter matches zero issued donations —
	// the handler maps this to a 4xx so no empty/zero-row file is ever streamed
	// (D-74 spirit: never round-trip an empty artifact).
	// HTTP mapping: 404 Not Found.
	ErrNoRecords = errors.New("edonation: no records match the export filter")
)
