---
status: testing
phase: 06-public-donation-web-form-flow-b
source: [06-VERIFICATION.md]
started: 2026-07-12T03:30:00Z
updated: 2026-07-12T03:30:00Z
---

## Current Test

number: 1
name: Responsive + bilingual walkthrough (NFR-06, plan 06-08 Task 2)
expected: |
  Public form (/donate) + staff queue (/queue) usable and correctly laid out on desktop
  AND mobile in both Thai and English; AppShell hamburger drawer works (backdrop, Escape,
  focus return); wide tables scroll horizontally below 768px.
awaiting: user response

## Tests

### 1. Responsive + bilingual walkthrough (NFR-06, plan 06-08 Task 2)
expected: Public form + staff queue usable and correctly laid out on desktop AND mobile in both Thai and English; AppShell hamburger drawer works (backdrop, Escape, focus return); wide tables scroll horizontally below 768px.
result: [pending]

### 2. Real Cloudflare Turnstile challenge on a live stack
expected: With real NEXT_PUBLIC_TURNSTILE_SITE_KEY + TURNSTILE_SECRET_KEY, the donor completes an actual Turnstile challenge and submission succeeds; a missing/failed challenge is rejected with the distinct CAPTCHA error (not a field-validation error).
result: [pending]

### 3. Authenticated staff queue end-to-end walkthrough (FR-08)
expected: A logged-in staff user opens /queue, sees both Flow A and Flow B pending_review rows with source badges, the 3-chip source filter narrows correctly, and a Flow B record shows the source-aware creator label ("ผู้บริจาคส่งเอง (ผ่านเว็บไซต์)").
result: [pending]

## Summary

total: 3
passed: 0
issues: 0
pending: 3
skipped: 0
blocked: 0

## Gaps
