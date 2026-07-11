// Package donation — model.go
//
// Request/response Go structs for the donation HTTP layer.
// These types are distinct from the sqlc-generated DB models (internal/db/generated/)
// to enforce the API/DB boundary: never expose raw DB rows to callers (T-03-11).
package donation

import "time"

// CreateDonationRequest is the JSON request body for creating a new donation record (FR-07).
// DonorTaxID is mandatory (D-44): a 13-digit Thai national/tax ID.
// Consent fields are captured per donation snapshot (D-49, NFR-03).
// DonorLanguage drives which bilingual PDF/email template the worker renders (D-55,
// FR-23). Optional at the API boundary — empty string defaults to "th" at the service
// layer (Create/UpdateDraft); the DB column additionally defaults+CHECKs th|en as a
// backstop. Frozen at create-time as part of the immutable snapshot (D-43 precedent) —
// never re-derived after creation.
type CreateDonationRequest struct {
	DonorName          string  `json:"donor_name"           validate:"required,min=1,max=255"`
	DonorTaxID         string  `json:"donor_tax_id"         validate:"required,len=13,numeric"` // D-44: mandatory
	DonorAddress       string  `json:"donor_address"        validate:"max=1000"`
	DonorEmail         string  `json:"donor_email"          validate:"omitempty,email,max=255"`
	Amount             float64 `json:"amount"               validate:"required,gt=0"`
	DonatedAt          string  `json:"donated_at"           validate:"required"` // "YYYY-MM-DD"
	Notes              string  `json:"notes"                validate:"max=2000"`
	ConsentGiven       bool    `json:"consent_given"`
	ConsentTextVersion string  `json:"consent_text_version"`
	ConsentPurpose     string  `json:"consent_purpose"`
	DonorLanguage      string  `json:"donor_language"       validate:"omitempty,oneof=th en"` // D-55, FR-23
}

// PublicDonationRequest is the donor-supplied payload for a Flow B public web
// submission (plan 06-03, FR-01/02/03). It is DELIBERATELY donor-fields-only:
// the CAPTCHA token (turnstile_token) and any rate-limit concern are handled in
// the middleware layer BEFORE the handler parses this struct — never as a
// validated field here (Pitfall 3, 06-RESEARCH.md), mirroring how RequireAuth is
// middleware and never a request-body field.
//
// Mirrors CreateDonationRequest's donor field set (D-79 keeps DonorTaxID
// mandatory). ConsentTextVersion carries the Flow-B-specific consent string
// (D-81). DonorLanguage drives the bilingual ack email / eventual receipt (D-55).
type PublicDonationRequest struct {
	DonorName          string  `json:"donor_name"           validate:"required,min=1,max=255"`
	DonorTaxID         string  `json:"donor_tax_id"         validate:"required,len=13,numeric"` // D-79: mandatory
	DonorAddress       string  `json:"donor_address"        validate:"max=1000"`
	DonorEmail         string  `json:"donor_email"          validate:"omitempty,email,max=255"`
	Amount             float64 `json:"amount"               validate:"required,gt=0"`
	DonatedAt          string  `json:"donated_at"           validate:"required"` // "YYYY-MM-DD"
	Notes              string  `json:"notes"                validate:"max=2000"`
	ConsentGiven       bool    `json:"consent_given"`
	ConsentTextVersion string  `json:"consent_text_version"`
	ConsentPurpose     string  `json:"consent_purpose"`
	DonorLanguage      string  `json:"donor_language"       validate:"omitempty,oneof=th en"` // D-55, FR-23
}

// UpdateDraftRequest is the JSON request body for editing a donation still in draft status (FR-09).
// All donor fields may be changed while the record remains in draft.
// Submitting after edit requires a separate Submit call.
type UpdateDraftRequest struct {
	DonorName          string  `json:"donor_name"           validate:"required,min=1,max=255"`
	DonorTaxID         string  `json:"donor_tax_id"         validate:"required,len=13,numeric"`
	DonorAddress       string  `json:"donor_address"        validate:"max=1000"`
	DonorEmail         string  `json:"donor_email"          validate:"omitempty,email,max=255"`
	Amount             float64 `json:"amount"               validate:"required,gt=0"`
	DonatedAt          string  `json:"donated_at"           validate:"required"`
	Notes              string  `json:"notes"                validate:"max=2000"`
	ConsentGiven       bool    `json:"consent_given"`
	ConsentTextVersion string  `json:"consent_text_version"`
	ConsentPurpose     string  `json:"consent_purpose"`
	DonorLanguage      string  `json:"donor_language"       validate:"omitempty,oneof=th en"` // D-55, FR-23
}

// DonationResponse is the API-level response for a single donation record.
//
// Security rules (T-03-09, D-46):
//   - DonorTaxIDMasked always holds a masked value (last-4 reveal via pii.MaskNationalID).
//   - The plaintext donor tax/national ID is NEVER included in this struct.
//   - For an authorised full-PII reveal (Checker/Admin), use the /pii endpoint (plan 03-05).
type DonationResponse struct {
	ID                 string     `json:"id"`
	Status             string     `json:"status"`
	DonorName          string     `json:"donor_name"`
	DonorTaxIDMasked   string     `json:"donor_tax_id_masked"` // NEVER DonorTaxIDEnc — T-03-09
	DonorAddress       string     `json:"donor_address"`
	DonorEmail         *string    `json:"donor_email,omitempty"`
	Amount             string     `json:"amount"`
	DonatedAt          string     `json:"donated_at,omitempty"`
	Notes              *string    `json:"notes,omitempty"`
	ConsentGiven       bool       `json:"consent_given"`
	ConsentAt          *time.Time `json:"consent_at,omitempty"`
	ConsentTextVersion *string    `json:"consent_text_version,omitempty"`
	ConsentPurpose     *string    `json:"consent_purpose,omitempty"`
	CreatedBy          string     `json:"created_by"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	SubmittedAt        *time.Time `json:"submitted_at,omitempty"`
	// Review/approval fields — populated after Checker action (plan 03-05).
	ReviewedBy       *string    `json:"reviewed_by,omitempty"`       // Checker UUID who returned/rejected
	ReviewedAt       *time.Time `json:"reviewed_at,omitempty"`       // timestamp of review action
	ReviewReason     *string    `json:"review_reason,omitempty"`     // mandatory reason for return/reject
	ApprovedAt       *time.Time `json:"approved_at,omitempty"`       // timestamp of approval
	ReceiptFormatted *string    `json:"receipt_formatted,omitempty"` // frozen receipt number string (D-42)
	// Cancellation fields — populated after Cancel action (plan 03-06, FR-19, D-47).
	CancelledBy  *string    `json:"cancelled_by,omitempty"`  // Checker/Admin UUID who cancelled
	CancelledAt  *time.Time `json:"cancelled_at,omitempty"`  // timestamp of cancellation
	CancelReason *string    `json:"cancel_reason,omitempty"` // mandatory reason for cancel
	// Void & Reissue links — D-50 self-FK on donations table.
	Replaces   *string `json:"replaces,omitempty"`    // UUID of the original record this one replaced
	ReplacedBy *string `json:"replaced_by,omitempty"` // UUID of the replacement record
}

// ReceiptRef is a compact {id, receipt_formatted} reference to another donation record,
// used to expand the replaces/replaced_by self-FK pointers (D-50) into a nested object
// on DonationDetailResponse instead of a bare UUID string (D-R3 detail contract).
type ReceiptRef struct {
	ID               string `json:"id"`
	ReceiptFormatted string `json:"receipt_formatted"`
}

// EmailDeliveryInfo is the most recent email send attempt for an issued/cancelled
// donation's receipt PDF (Screen 3b, FR-27) — sourced from the email_delivery table
// (one row per attempt, worker auto-retry AND staff resend both insert new rows).
// Nil on DonationDetailResponse when no attempt has been recorded yet (the worker
// (04-05) has not finished processing the issue_receipt outbox job).
type EmailDeliveryInfo struct {
	Status            string    `json:"status"` // "sent" | "failed" | "no_email"
	SentTo            *string   `json:"sent_to,omitempty"`
	Attempts          int32     `json:"attempts"`
	ProviderMessageID *string   `json:"provider_message_id,omitempty"`
	LastError         *string   `json:"last_error,omitempty"`
	LastAttemptAt     time.Time `json:"last_attempt_at"`
}

// ReviewHistoryEntry is one return/reject event in a donation's review history,
// sourced from the immutable audit_log (D-R3 detail contract, FR-12).
// Action is normalized to "return" or "reject" (never the raw "donation.return" audit action string).
type ReviewHistoryEntry struct {
	ID        int64     `json:"id"`
	Action    string    `json:"action"` // "return" | "reject"
	Reason    string    `json:"reason"`
	ActorName string    `json:"actor_name"`
	ActedAt   time.Time `json:"acted_at"`
}

// DonationDetailResponse is the API-level response for a single donation record —
// the richer contract consumed by the Phase-3 detail/review screens (D-R3 remediation).
//
// It replaces DonationResponse as the return type of GetByID and every mutation
// (Create/UpdateDraft/Submit/Approve/Return/Reject/Cancel/Reissue), built by the single
// shared buildDetailResponse helper in service.go so all nine call sites stay aligned.
//
// Security rules (T-03-09, D-46, T-11-02):
//   - NationalIDMasked always holds a masked value (last-4 reveal via pii.MaskNationalID).
//   - The plaintext donor tax/national ID is NEVER included in this struct.
//   - For an authorised full-PII reveal (Checker/Admin), use the /pii endpoint.
//
// Server-authoritative auth flags (T-03-31, T-11-01, T-11-03): ViewerIsCreator/CanApprove/
// CanReturn/CanReject/CanRevealPII are computed server-side from the viewer's RESOLVED
// users.id (never the raw Keycloak subject) + role + the record's current status. They are
// UI hints only — mutations independently re-enforce SoD/RBAC server-side regardless of
// what these flags say.
type DonationDetailResponse struct {
	ID                 string     `json:"id"`
	Status             string     `json:"status"`
	DonorName          string     `json:"donor_name"`
	NationalIDMasked   string     `json:"national_id_masked"` // NEVER plaintext — T-03-09/T-11-02
	Address            string     `json:"address"`
	Email              *string    `json:"email,omitempty"`
	Amount             string     `json:"amount"`
	DonatedAt          string     `json:"donated_at,omitempty"`
	Note               *string    `json:"note,omitempty"`
	ConsentGiven       bool       `json:"consent_given"`
	ConsentAt          *time.Time `json:"consent_at,omitempty"`
	ConsentTextVersion *string    `json:"consent_text_version,omitempty"`
	ConsentPurpose     *string    `json:"consent_purpose,omitempty"`
	// CreatedBy is the creator's DISPLAY NAME (not a UUID) — CreatedByID carries the raw
	// users.id UUID string so the UI can route to "my drafts" / compare identity.
	CreatedBy   string     `json:"created_by"`
	CreatedByID string     `json:"created_by_id"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	SubmittedAt *time.Time `json:"submitted_at,omitempty"`
	// Review/approval fields — populated after Checker action.
	ReviewedBy       *string    `json:"reviewed_by,omitempty"`       // Checker UUID who returned/rejected
	ReviewedAt       *time.Time `json:"reviewed_at,omitempty"`       // timestamp of review action
	ReviewReason     *string    `json:"review_reason,omitempty"`     // reason for the LATEST return/reject
	ApprovedAt       *time.Time `json:"approved_at,omitempty"`       // timestamp of approval
	ReceiptFormatted *string    `json:"receipt_formatted,omitempty"` // frozen receipt number string (D-42)
	// Cancellation fields — populated after Cancel action (FR-19, D-47).
	CancelledBy    *string    `json:"cancelled_by,omitempty"`
	CancelledAt    *time.Time `json:"cancelled_at,omitempty"`
	CancelReason   *string    `json:"cancel_reason,omitempty"`
	EdonationKeyed bool       `json:"edonation_keyed"`
	// Void & Reissue links — D-50 self-FK on donations table, expanded to {id,receipt_formatted}.
	Replaces   *ReceiptRef `json:"replaces,omitempty"`
	ReplacedBy *ReceiptRef `json:"replaced_by,omitempty"`
	// Full return/reject history (not just the latest — FR-12, D-R3), oldest→newest.
	ReviewHistory []ReviewHistoryEntry `json:"review_history"`
	// DonorLanguage is the frozen document language ("th"|"en") driving PDF/email
	// rendering (D-55, FR-23) — set at create-time, never re-derived.
	DonorLanguage string `json:"donor_language"`
	// ReceiptPDFObjectKey is non-nil once the outbox worker has frozen the receipt
	// PDF to object storage (D-56). Used by the FE to gate download/resend button
	// enablement without a separate round-trip.
	ReceiptPDFObjectKey *string `json:"receipt_pdf_object_key,omitempty"`
	// EmailDelivery is the latest send attempt (Screen 3b) — see EmailDeliveryInfo
	// doc comment. Only ever populated for status issued|cancelled.
	EmailDelivery *EmailDeliveryInfo `json:"email_delivery,omitempty"`
	// Server-computed authorization flags (T-03-31) — see type doc comment above.
	ViewerIsCreator bool `json:"viewer_is_creator"`
	CanApprove      bool `json:"can_approve"`
	CanReturn       bool `json:"can_return"`
	CanReject       bool `json:"can_reject"`
	CanRevealPII    bool `json:"can_reveal_pii"`
}

// ReviewRequest is the JSON request body for Return and Reject actions (D-45, FR-12).
// Reason is mandatory — empty or whitespace-only returns ErrMissingReason (422 Unprocessable).
type ReviewRequest struct {
	Reason string `json:"reason" validate:"required,min=1,max=2000"`
}

// ListFilter holds optional search/filter criteria for listing donations (FR-10, D-53).
// All fields are optional — nil/zero means no restriction applied to that dimension.
// Tax ID is intentionally excluded as a filter parameter (D-53, T-03-29).
// Source is nil = skip the filter (returns both flow_a and flow_b, D-53 nil-skip
// semantics); otherwise must be "flow_a" or "flow_b" — enforced by the handler
// before it reaches this struct (FR-08, D-77).
type ListFilter struct {
	DonorName *string
	Status    *string
	FromDate  *time.Time
	ToDate    *time.Time
	ReceiptNo *string
	Source    *string
	Offset    int32
	Limit     int32
}

// DonationListItem is a single row in the paginated donation list response (FR-10, D-R2).
//
// Security rules (D-53, T-09-02):
//   - No tax/national ID field of any kind — the list is PII-free by design.
//   - CreatedBy is the creator's display name (for UI labelling); CreatedByID is the
//     raw users.id UUID string (so the UI can route to "my drafts"). If the creator's
//     user row is missing (LEFT JOIN NULL), CreatedBy falls back to "" while
//     CreatedByID still carries the raw UUID from donations.created_by.
//   - Source ("flow_a"/"flow_b", D-77) lets the pending-review queue (plan 07)
//     separate staff-entered records from public web submissions without a
//     second round-trip.
type DonationListItem struct {
	ID               string  `json:"id"`
	Status           string  `json:"status"`
	DonorName        string  `json:"donor_name"`
	Amount           string  `json:"amount"`
	DonatedAt        string  `json:"donated_at"`
	ReceiptFormatted *string `json:"receipt_formatted,omitempty"`
	CreatedBy        string  `json:"created_by"`
	CreatedByID      string  `json:"created_by_id"`
	Source           string  `json:"source"`
	// CreatedAt is the submission timestamp (RFC3339) — the pending-review queue
	// (Screen 11, plan 07) shows it as its "วันที่ส่ง" column, distinct from
	// DonatedAt (the donation date). Already SELECTed by SearchDonations; exposed
	// here so the queue needs no second round-trip.
	CreatedAt string `json:"created_at"`
}

// DonationListResult is the D-R2 pagination envelope payload for GET /api/donations.
// The handler wraps this in the standard {"data": ...} envelope, i.e. the wire shape is
// {"data": {"items": [...], "total": N, "page": P, "per_page": 20}} — never a bare array.
type DonationListResult struct {
	Items   []DonationListItem `json:"items"`
	Total   int64              `json:"total"`
	Page    int                `json:"page"`
	PerPage int                `json:"per_page"`
}

// CancelDonationRequest is the JSON request body for Cancel (void) of an issued receipt (D-47, FR-19).
// Reason is mandatory — empty or whitespace-only returns ErrMissingReason (422).
// RDConfirmationReason is required when edonation_keyed=true (D-51, T-03-25).
type CancelDonationRequest struct {
	Reason               string `json:"reason"                 validate:"required,min=1,max=2000"`
	RDConfirmationReason string `json:"rd_confirmation_reason"`
}

// ReissueDonationRequest is the request body for Void & Reissue (D-50).
// Contains the cancellation authorization fields (reason + optional rd_confirmation_reason)
// plus corrected donor fields for the replacement draft (same shape as CreateDonationRequest).
// The replacement draft earns a fresh number only via the normal Submit → Approve path (D-50).
type ReissueDonationRequest struct {
	// Cancellation authorization (mirrors CancelDonationRequest)
	Reason               string `json:"reason"                 validate:"required,min=1,max=2000"`
	RDConfirmationReason string `json:"rd_confirmation_reason"`
	// Corrected donor data for the new draft (mirrors CreateDonationRequest)
	DonorName          string  `json:"donor_name"           validate:"required,min=1,max=255"`
	DonorTaxID         string  `json:"donor_tax_id"         validate:"required,len=13,numeric"`
	DonorAddress       string  `json:"donor_address"        validate:"max=1000"`
	DonorEmail         string  `json:"donor_email"          validate:"omitempty,email,max=255"`
	Amount             float64 `json:"amount"               validate:"required,gt=0"`
	DonatedAt          string  `json:"donated_at"           validate:"required"`
	Notes              string  `json:"notes"                validate:"max=2000"`
	ConsentGiven       bool    `json:"consent_given"`
	ConsentTextVersion string  `json:"consent_text_version"`
	ConsentPurpose     string  `json:"consent_purpose"`
}

// PIIRevealResponse is returned by GET /api/donations/:id/pii (D-46, T-03-26).
// Full plaintext donor tax/national ID is exposed only to Checker and Admin roles.
// Every reveal is audited (action="pii.reveal") before the plaintext is returned (D-13).
type PIIRevealResponse struct {
	DonationID          string `json:"donation_id"`
	DonorTaxIDPlaintext string `json:"donor_tax_id"` // plaintext — only in this endpoint
}
