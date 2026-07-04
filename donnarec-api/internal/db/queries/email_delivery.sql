-- internal/db/queries/email_delivery.sql
-- sqlc queries for Phase 4: email delivery status tracking (D-57, FR-27).
-- One row is inserted per send attempt (auto-retry AND manual resend both
-- insert a new row — never overwrite a prior attempt's record), so the full
-- delivery history for a donation is reconstructable for staff/audit.

-- name: InsertEmailDelivery :one
-- Record one email send attempt (worker auto-retry or staff-triggered resend).
-- provider_message_id is "" for the dev/local EmailSender (D-60); populated
-- once a real provider is wired.
INSERT INTO email_delivery (
    donation_id,
    sent_to,
    status,
    provider_message_id,
    attempts,
    last_error
) VALUES (
    @donation_id,
    @sent_to,
    @status,
    @provider_message_id,
    @attempts,
    @last_error
)
RETURNING id, created_at;

-- name: GetLatestEmailDeliveryForDonation :one
-- Returns the most recent send attempt for a donation — drives the delivery
-- status panel on the donation detail screen (Screen 3b, FR-27).
-- Returns pgx.ErrNoRows if no email has ever been attempted for this donation
-- (e.g. donor has no email address — caller distinguishes this from a failure).
SELECT id, donation_id, sent_to, status, provider_message_id, attempts, last_error, created_at
FROM email_delivery
WHERE donation_id = @donation_id
ORDER BY created_at DESC
LIMIT 1;
