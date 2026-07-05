// Package donation implements the donation lifecycle for the DonaRec back-office.
// State machine: draft → pending_review → issued → cancelled (with return/reject branches).
// Full implementation is spread across plans 03-02 through 03-08.
package donation

import "errors"

// Sentinel errors used by DonationService and mapped to HTTP status codes by DonationHandler.
// Defined here (Wave 0) so test scaffolds and handler error-mapping code can reference them
// before the service implementation is complete (plans 03-03, 03-05, 03-06).

var (
	// ErrMissingTaxID is returned when DonorTaxID is empty in CreateDonationRequest
	// or UpdateDraftRequest. The donor national/tax ID is mandatory at the API boundary
	// (D-44): a donation cannot be created or issued without it.
	// HTTP mapping: 422 Unprocessable Entity.
	ErrMissingTaxID = errors.New("donation: donor tax/national ID is required (D-44)")

	// ErrInvalidTransition is returned when an action is attempted on a donation
	// whose current status does not allow it (e.g. approving a 'draft' record).
	// HTTP mapping: 409 Conflict.
	ErrInvalidTransition = errors.New("donation: invalid state transition")

	// ErrSoDViolation is returned when the approver is the same user who created
	// the donation (Segregation of Duties, FR-14, CLAUDE.md defense-in-depth).
	// HTTP mapping: 403 Forbidden.
	ErrSoDViolation = errors.New("donation: segregation of duties violation — approver cannot be the creator")

	// ErrMissingReason is returned when a review action (return or reject) or
	// cancellation is attempted without providing a mandatory reason (D-45, D-47).
	// HTTP mapping: 422 Unprocessable Entity.
	ErrMissingReason = errors.New("donation: review reason is required for this action")

	// ErrNotFound is returned when the requested donation does not exist.
	// HTTP mapping: 404 Not Found.
	ErrNotFound = errors.New("donation: not found")

	// ErrForbidden is returned when the caller's role does not permit the action
	// (e.g. Maker attempting PII reveal beyond their own draft, D-46).
	// HTTP mapping: 403 Forbidden.
	ErrForbidden = errors.New("donation: forbidden — insufficient role for this action")

	// ErrDraftOnly is returned when an edit operation is attempted on a donation
	// that is no longer in 'draft' status (FR-09).
	// HTTP mapping: 409 Conflict.
	ErrDraftOnly = errors.New("donation: operation is only permitted on draft records")

	// ErrEDonationKeyedCancel is returned when cancellation is attempted on a donation
	// that has been keyed into the RD e-Donation system without providing the required
	// RD reconciliation confirmation reason (D-51).
	// HTTP mapping: 422 Unprocessable Entity.
	ErrEDonationKeyedCancel = errors.New("donation: rd_confirmation_reason is required when edonation_keyed is true")

	// ErrUserNotProvisioned is returned when a request carries a validly-authenticated
	// Keycloak token whose "sub" claim does not resolve to any active row in the
	// application's `users` table. Subject -> users.id resolution now happens in the
	// auth.ResolveAppUser middleware, which 403s (user_not_provisioned) before the
	// handler runs; this sentinel + its handler switch arms remain as defense-in-depth
	// so the FK is never papered over by writing a zero-value UUID (bug:
	// created-by-fk-mismatch).
	// HTTP mapping: 403 Forbidden.
	ErrUserNotProvisioned = errors.New("donation: authenticated identity is not a provisioned application user")

	// ErrReceiptNotReady is returned by Resend and DownloadReceipt when the donation's
	// receipt PDF has not yet been frozen to object storage by the outbox worker
	// (donations.receipt_pdf_object_key is still nil) — the worker has not finished
	// processing the issue_receipt job yet (D-56, FR-27/28).
	// HTTP mapping: 409 Conflict.
	ErrReceiptNotReady = errors.New("donation: receipt PDF is not yet available — the worker has not finished processing")
)
