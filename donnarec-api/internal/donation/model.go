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
}

// ListFilter holds optional search/filter criteria for listing donations (FR-10, D-53).
// All fields are optional — nil/zero means no restriction applied to that dimension.
type ListFilter struct {
	DonorName *string
	Status    *string
	FromDate  *time.Time
	ToDate    *time.Time
	ReceiptNo *string
	Offset    int32
	Limit     int32
}
