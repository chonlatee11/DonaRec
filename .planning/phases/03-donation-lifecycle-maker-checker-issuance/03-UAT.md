---
status: testing
phase: 03-donation-lifecycle-maker-checker-issuance
source:
  - 03-VERIFICATION.md (human_verification checklist — criterion 6b)
  - 03-09..03-13-SUMMARY.md (remediation slice)
started: 2026-07-04T00:00:00Z
updated: 2026-07-04T00:00:00Z
---

## Current Test
<!-- OVERWRITE each test - shows where we are -->

number: 1
name: Cold Start Smoke Test (bring up the full stack)
expected: |
  The full stack boots from scratch: Postgres + Keycloak + MinIO + Go API (via
  donnarec-api/docker-compose.yml) come up healthy, DB migrations apply, and the
  Next.js web app (donnarec-web, npm run dev) loads. Visiting the web app redirects
  an unauthenticated user to the Keycloak sign-in, and after signing in you land on
  /donations without console errors.
awaiting: user response

## Tests

### 1. Cold Start Smoke Test (bring up the full stack)
expected: Postgres + Keycloak + MinIO + Go API boot healthy via donnarec-api/docker-compose.yml; migrations apply; donnarec-web (npm run dev) loads; unauthenticated visit redirects to Keycloak sign-in; after login you reach /donations with no console errors.
result: issue-resolved-in-session
reported: "After logout→login, /donations threw Runtime TypeError 'Cannot read properties of undefined (reading length)' at DonationTable.tsx:234 (items.length)."
severity: blocker
root_cause: "NOT a code defect. The long-running dev api container (image built 2026-07-02) predated the 03-09 backend envelope fix (committed 2026-07-03). The stale binary still returned the pre-remediation bare-array list contract `{data:[...]}`; the BFF passed it through, so fetchDonations returned an array (no `.items`), DonationListView's `data ?? {}` guard did not catch a truthy-but-wrong shape, and DonationTable crashed on `items.length`. The remediation code + tests (E2E on fresh testcontainers) were correct — only the deployed container was stale."
resolution: "Rebuilt the api container from current code: `docker compose up -d --build api` → image built 2026-07-04 00:48, /healthz 200. Container now serves the {data:{items,total,page,per_page}} envelope."
result_note: "awaiting user re-test (hard reload /donations)"

### 2. Maker→Checker happy path (create → submit → approve → receipt)
expected: Signed in as Maker A, create a donation (donor name, 13-digit national ID, address, amount, date, consent), save draft, then submit. Sign in as a DIFFERENT user Checker B, open the record, Approve it. The record moves to "อนุมัติแล้ว/issued" and shows a gap-less receipt number. The list at /donations reflects the new status.
result: [pending]

### 3. Token never reaches the browser (D-R1)
expected: With DevTools → Network open while using /donations and a detail page, inspect the responses the browser receives from /api/bff/donations/**. None of them contain the Keycloak access token (no Bearer/JWT in any response body or in client-visible headers). The token only exists server-side in the BFF route handlers.
result: [pending]

### 4. SoD blocked state — buttons absent from DOM
expected: Sign in as a Checker who is ALSO the creator of a record (Maker≡Checker), open that record's detail page. The Approve / Return (ตีกลับแก้ไข) / Reject (ปฏิเสธถาวร) buttons are NOT present in the DOM at all (not merely disabled) — the SoD-blocked alert renders instead. Inspect the DOM to confirm the buttons are absent.
result: [pending]

### 5. PII reveal round-trip (masked → reveal → re-mask + audited)
expected: As Checker/Admin, open a donation. The national ID shows MASKED by default. Click reveal → the full 13-digit plaintext replaces it. Reload the page → it shows MASKED again. (Each reveal is recorded in the audit log server-side.)
result: [pending]

### 6. List filter + pagination
expected: On /donations, filter by donor name, by status, by date range, and by receipt number — the table updates to matching rows each time. Page through results (next/prev or page numbers) and the correct page of rows loads. Thai text and status badges render correctly.
result: [pending]

### 7. Cancel / Void & Reissue dialogs
expected: On an issued receipt, open Cancel — a reason dialog appears; confirming sets status to "ยกเลิก" while RETAINING the receipt number (no gap, never deleted). Void & Reissue links a replacement to the original. The edonation_keyed=true guard blocks/ warns the disallowed action as designed.
result: [pending]

## Summary

total: 7
passed: 0
issues: 0
pending: 7
skipped: 0

## Gaps

[none yet]
