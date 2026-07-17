// Package main — e2e_flowb_composite_test.go
//
// Closes v1.0 milestone audit WARNING-1 (.planning/v1.0-MILESTONE-AUDIT.md): a
// single automated test spanning the FULL Flow B composite seam — public
// submission through Checker approval to issuance — which was previously only
// covered as two independently-tested halves (TestPublicDonationE2E,
// TestE2E_MakerCheckerIssuancePipeline). Per the project CONVENTIONS.md
// integration-test gate (which caught 3 seam bugs in Phase 3), the untested
// cross-flow handoff between the public-submit seam and the approve/issuance
// seam is exactly the class of defect that can hide from unit/service tests
// and from either half tested alone.
//
// This test drives the REAL router for both halves:
//
//	multipart POST /api/public/donations (unauthenticated)
//	  → DB flow_b pending_review row lookup
//	  → POST /api/donations/{id}/approve (real Keycloak-shaped Checker token)
//	  → status=issued, gap-less receipt_formatted, issue_receipt outbox job,
//	    donation.approve audit row
//
// It adds NO new product behavior and reuses the existing E2E harness
// verbatim (newE2EHarness, doPublicSubmission, validPublicFields,
// settingsPNGBytes, provisionUser, MintTokenForSubject, do, decodeDonation,
// backendClientID, donation.PublicWebUserID) — no new harness, fixtures, or
// production code.
//
// Requires Docker testcontainers. Skip with -short. Run under -race.
package main

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	db "github.com/donnarec/donnarec-api/internal/db/generated"
	"github.com/donnarec/donnarec-api/internal/donation"
)

// TestE2E_FlowBCompositePublicSubmitToIssued proves the full Flow B composite
// handoff over the real HTTP path: an unauthenticated public donation
// submission reaches a Checker-approved, issued state with a gap-less
// receipt number, exactly one issue_receipt outbox job, and an approval
// audit row — closing v1.0 audit WARNING-1.
func TestE2E_FlowBCompositePublicSubmitToIssued(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E integration test in short mode: requires Docker")
	}

	h := newE2EHarness(t)

	// --- Step 1: provision a Checker with a distinct keycloak subject from the
	// flow_b creator (the seeded public-web user) — the SoD precondition this
	// composite must satisfy (approver_id != created_by). ---
	const subChecker = "77777777-7777-7777-7777-777777777777"
	h.provisionUser(t, "checker-flowb-e2e@example.com", "Checker FlowB E2E", subChecker, db.UserRoleEnumChecker)
	checkerToken := h.kc.MintTokenForSubject(subChecker, backendClientID, "checker")

	// --- Step 2: unauthenticated public submission (Flow B) ---
	const donorName = "นาย ทดสอบ Flow B Composite"
	w := h.doPublicSubmission(t, validPublicFields(donorName), "slip.png", settingsPNGBytes(), "")
	require.Equal(t, http.StatusCreated, w.Code, "public submission body: %s", w.Body.String())

	// --- Step 3: look up the created flow_b donation id from the DB — the
	// public response carries a reference_number (REF-...), not the donation
	// id. Assert the SoD precondition: created_by is the public-web UUID,
	// distinct from the approving Checker's subject. ---
	var (
		donationID string
		status     string
		source     string
		createdBy  string
	)
	require.NoError(t, h.pool.QueryRow(h.ctx,
		`SELECT id::text, status, source, created_by::text FROM donations WHERE donor_name = $1`, donorName,
	).Scan(&donationID, &status, &source, &createdBy))
	require.NotEmpty(t, donationID, "flow_b donation row must exist after public submission")
	assert.Equal(t, "pending_review", status)
	assert.Equal(t, "flow_b", source, "public submissions must be source=flow_b")
	assert.Equal(t, donation.PublicWebUserID, createdBy,
		"created_by must be the seeded public-web user — the SoD precondition for the composite approve step")
	assert.NotEqual(t, subChecker, createdBy,
		"flow_b created_by must be distinct from the approving Checker's subject (SoD)")

	// --- Step 4: Checker approves the SAME record over the real HTTP path ---
	w = h.do(t, http.MethodPost, "/api/donations/"+donationID+"/approve", checkerToken, nil)
	require.Equal(t, http.StatusOK, w.Code, "approve body: %s", w.Body.String())
	issued := decodeDonation(t, w)

	// --- Step 5: assert issuance — status=issued, non-empty gap-less receipt ---
	assert.Equal(t, "issued", issued.Status)
	require.NotNil(t, issued.ReceiptFormatted, "receipt_formatted must be set after issuance")
	assert.NotEmpty(t, *issued.ReceiptFormatted,
		"receipt number must be allocated gap-less in the issuance tx")

	// --- Step 6: exactly one issue_receipt outbox job for this donation ---
	var issueCount int
	require.NoError(t, h.pool.QueryRow(h.ctx,
		`SELECT count(*) FROM outbox_jobs WHERE job_type = 'issue_receipt' AND payload->>'donation_id' = $1`, donationID,
	).Scan(&issueCount))
	assert.Equal(t, 1, issueCount, "approving a flow_b record must enqueue exactly one issue_receipt outbox job")

	// --- Step 7: exactly one approval audit row under the Checker's subject ---
	var approveAuditCount int
	require.NoError(t, h.pool.QueryRow(h.ctx,
		`SELECT count(*) FROM audit_log WHERE actor_id = $1 AND action = 'donation.approve'`, subChecker,
	).Scan(&approveAuditCount))
	assert.Equal(t, 1, approveAuditCount, "one in-tx approval audit row must be written under the Checker's Keycloak subject")
}
