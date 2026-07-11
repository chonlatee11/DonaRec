package edonation

import "errors"

// Sentinel errors used by Service and mapped to HTTP status codes by Handler
// (mirrors internal/donation/errors.go's convention).
//
// Note (IN-02): the empty-export 404 is NOT a sentinel — Service.Export returns
// the (possibly empty) row slice and the handler emits 404 directly on
// len(rows)==0. There is deliberately no ErrNoRecords sentinel; adding one back
// without wiring Service.Export to return it would be dead code again.
var (
	// ErrForbidden is returned by Export when the caller holds neither Checker nor
	// Admin role — service-layer defense-in-depth beyond the HTTP RequireAnyRole
	// route guard (T-05-02-RBAC).
	// HTTP mapping: 403 Forbidden.
	ErrForbidden = errors.New("edonation: forbidden — checker or admin role required")
)
