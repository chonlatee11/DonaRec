---
status: complete
phase: 03-donation-lifecycle-maker-checker-issuance
source:
  - 03-VERIFICATION.md (human_verification checklist — criterion 6b)
  - 03-09..03-13-SUMMARY.md (remediation slice)
started: 2026-07-04T00:00:00Z
updated: 2026-07-04T00:00:00Z
---

## Current Test

[testing complete — 7/7 passed; 3 code issues found and fixed in-session]

## Tests

### 1. Cold Start Smoke Test (bring up the full stack)
expected: Postgres + Keycloak + MinIO + Go API boot healthy via donnarec-api/docker-compose.yml; migrations apply; donnarec-web (npm run dev) loads; unauthenticated visit redirects to Keycloak sign-in; after login you reach /donations with no console errors.
result: pass
reported: "After logout→login, /donations threw Runtime TypeError 'Cannot read properties of undefined (reading length)' at DonationTable.tsx:234 (items.length)."
severity: blocker
root_cause: "NOT a code defect. The long-running dev api container (image built 2026-07-02) predated the 03-09 backend envelope fix (committed 2026-07-03). The stale binary still returned the pre-remediation bare-array list contract `{data:[...]}`; the BFF passed it through, so fetchDonations returned an array (no `.items`), DonationListView's `data ?? {}` guard did not catch a truthy-but-wrong shape, and DonationTable crashed on `items.length`. The remediation code + tests (E2E on fresh testcontainers) were correct — only the deployed container was stale."
resolution: "1) Rebuilt the api container from current code (image built 2026-07-04 00:48, /healthz 200 — now serves the {data:{items,total,page,per_page}} envelope). 2) Defense-in-depth hardening (commit 3b3aeda): fetchDonations normalizes/validates shape (coerce legacy array, throw on unexpected); DonationListView guards items to an array; regression tests added (vitest 12/12, tsc clean)."
result_note: "User confirmed /donations renders correctly after rebuild + hardening."

### 2. Maker→Checker happy path (create → submit → approve → receipt)
expected: Signed in as Maker A, create a donation (donor name, 13-digit national ID, address, amount, date, consent), save draft, then submit. Sign in as a DIFFERENT user Checker B, open the record, Approve it. The record moves to "อนุมัติแล้ว/issued" and shows a gap-less receipt number. The list at /donations reflects the new status.
result: pass
found_issue: "Blocked mid-test: 'ออกจากระบบ' (logout) did not reach the login page — NextAuth signOut cleared only the local session while the Keycloak SSO session stayed alive, silently re-logging the same user, so Maker↔Checker could not be switched."
root_cause: "lib/auth.ts performed no RP-initiated (federated) logout and did not store the id_token."
resolution: "Store id_token in the JWT + events.signOut calls the Keycloak end_session endpoint (id_token_hint) server-side (commit 78b04f1). One-time manual SSO-cookie clear needed for the pre-fix session. After the fix: logout reaches the real Keycloak login page and the full create→submit→approve→receipt (gap-less number) flow completed."

### 3. Token never reaches the browser (D-R1)
expected: With DevTools → Network open while using /donations and a detail page, inspect the responses the browser receives from /api/bff/donations/**. None of them contain the Keycloak access token (no Bearer/JWT in any response body or in client-visible headers). The token only exists server-side in the BFF route handlers.
result: pass
result_note: "User confirmed via DevTools Network: no access token in BFF response bodies/headers; client calls same-origin /api/bff/** (not :8000 directly)."

### 4. SoD blocked state — buttons absent from DOM
expected: Sign in as a Checker who is ALSO the creator of a record (Maker≡Checker), open that record's detail page. The Approve / Return (ตีกลับแก้ไข) / Reject (ปฏิเสธถาวร) buttons are NOT present in the DOM at all (not merely disabled) — the SoD-blocked alert renders instead. Inspect the DOM to confirm the buttons are absent.
result: pass
result_note: "User confirmed: SoD-blocked alert renders and Approve/Return/Reject are absent from the DOM (server-computed viewer_is_creator), not merely disabled."

### 5. PII reveal round-trip (masked → reveal → re-mask + audited)
expected: As Checker/Admin, open a donation. The national ID shows MASKED by default. Click reveal → the full 13-digit plaintext replaces it. Reload the page → it shows MASKED again. (Each reveal is recorded in the audit log server-side.)
result: pass
result_note: "Functionality correct: masked → reveal shows full 13 digits → reload re-masks."
found_issue: "Cosmetic console hydration error on reveal-confirm: 'In HTML, <div> cannot be a descendant of <p>'. RevealPIIDialog's AlertDialogDescription (<p>) rendered the block <Skeleton> (<div>) in its loading branch."
severity: cosmetic
resolution: "Replaced the block Skeleton with an inline <span> skeleton valid inside <p>; removed unused import (commit 88e82ff). tsc clean."

### 6. List filter + pagination
expected: On /donations, filter by donor name, by status, by date range, and by receipt number — the table updates to matching rows each time. Page through results (next/prev or page numbers) and the correct page of rows loads. Thai text and status badges render correctly.
result: pass
result_note: "User confirmed all filters (name/status/date-range/receipt_no) and pagination work; Thai text + status badges render correctly."

### 7. Cancel / Void & Reissue dialogs
expected: On an issued receipt, open Cancel — a reason dialog appears; confirming sets status to "ยกเลิก" while RETAINING the receipt number (no gap, never deleted). Void & Reissue links a replacement to the original. The edonation_keyed=true guard blocks/ warns the disallowed action as designed.
result: pass
result_note: "User confirmed on a fresh issued receipt: Cancel (reason dialog) retains the receipt number (gap-less); Void & Reissue ('ยกเลิกและออกใบแทน', the second button on an issued record for Checker/Admin) cancels the original and links a replacement (D-50). Both actions surface only when status=issued and can_reveal_pii."

## Summary

total: 7
passed: 7
issues: 0
pending: 0
skipped: 0

## Gaps

[none — all 7 checkpoints passed. 3 code issues surfaced during the walkthrough
were fixed in-session, not left as open gaps:]

- 1 (Test 1): stale api container served pre-03-09 bare-array list contract → runtime crash on /donations.
  Resolved: rebuilt api container + defense-in-depth list hardening (commit 3b3aeda).
- 2 (Test 2): NextAuth-only logout left Keycloak SSO alive → could not switch users.
  Resolved: federated logout via id_token_hint (commit 78b04f1).
- 3 (Test 5): RevealPIIDialog rendered a <div> Skeleton inside a <p> → hydration console error (cosmetic).
  Resolved: inline <span> skeleton (commit 88e82ff).
