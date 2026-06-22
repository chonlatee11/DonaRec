# Feature Research

**Domain:** Donation / tax-deductible-receipt issuing system with maker-checker (four-eyes) approval — Thai hospital
**Researched:** 2026-06-22
**Confidence:** HIGH (requirements doc + verified Thai Revenue Department e-Donation sources + maker-checker industry patterns)

## Context Note

This is a **back-office document-issuance system**, not a payment/donation-collection platform. The "product" is a correct, legally-valid PDF receipt produced only after human approval. The two hardest, highest-risk features are (1) **gap-less fiscal-year receipt numbering** and (2) the **maker-checker approval gate**. Everything else exists to serve these two. Scope this phase = **cash donations only**; Phase 1 MVP = Flow A (staff-created) back office per requirements §10.

Feature IDs below map to the requirements doc (FR-xx / NFR-xx) so this feeds directly into the roadmap.

---

## Feature Landscape

### Table Stakes (Users Expect These)

Missing any of these makes the system unusable or legally non-compliant. The whole point is correctness + auditability, so the bar for "table stakes" is high here.

| Feature | FR/NFR | Why Expected | Complexity | Notes |
|---------|--------|--------------|------------|-------|
| Authentication + RBAC (Maker / Checker / Admin roles) | NFR-01, FR-34 | No approval workflow exists without distinct roles; legal/audit control | MEDIUM | Foundational; everything else gates on it. Enforce SoD at code level, not just UI |
| Maker creates donation record (Flow A) | FR-07 | Core entry path for MVP | LOW-MEDIUM | Donor info + amount + date + evidence attachment |
| View/edit record before approval, view attached slip | FR-09 | Checker must compare slip to amount to confirm real donation | MEDIUM | Slip viewer (image/PDF inline) is part of this |
| Explicit status lifecycle (draft → pending → approved/issued → rejected → cancelled) | FR-11 | State machine is the backbone of maker-checker | MEDIUM | Each transition is an audit event; cancelled ≠ deleted |
| Checker approves or returns with mandatory reason | FR-12 | Four-eyes principle; rejection without reason is useless | MEDIUM | No self-approval (maker ≠ checker) enforced server-side |
| **Receipt created only on approval** | FR-14 | Hard product invariant — no auto-issue | LOW (rule), HIGH (correctness) | Number is allocated at approval commit, never before |
| **Gap-less fiscal-year receipt numbering** | FR-15, FR-16, FR-17, FR-18, NFR-04 | Tax/audit requirement — skipped or duplicate numbers fail an audit | **HIGH** | DB sequence/atomic counter per fiscal year, allocated inside the approval transaction. THE critical feature |
| Fiscal-year auto-detection (1 Oct – 30 Sep; Oct–Dec rolls to next FY) | FR-18 | Thai government fiscal year; wrong FY = wrong number = compliance failure | MEDIUM | Anchor on issue date (approval date), not creation date |
| Cancel-by-status, never delete a number | FR-19 | Tax law requires the trail; deleting breaks gap-less integrity | LOW | Cancelled receipts retain their number permanently |
| PDF generation from hospital-branded template | FR-20 | A receipt that doesn't look official isn't usable | MEDIUM | Logo/letterhead, layout |
| Watermark on PDF | FR-21 | Anti-tamper / authenticity expectation for official docs | LOW-MEDIUM | Image overlay |
| Authorized-signatory signature image on PDF | FR-22 | Receipt invalid without authorizing signature | LOW | Image placement (PKI deferred — see anti-features) |
| **Tax-deduction-compliant receipt content** | FR-24, §6 | Whole purpose: donor must be able to claim deduction | MEDIUM-HIGH | Hospital name/address/Tax ID, receipt no + date, donor name/Tax ID or national ID/address, amount in digits AND Thai text, purpose, deduction-rights statement (incl. 1x/2x where applicable), signature + seal. **Exact wording must be confirmed with hospital accounting/legal** |
| Email PDF receipt to donor after approval | FR-25 | Primary delivery channel; replaces manual handoff | MEDIUM | Attach PDF |
| Email send status + resend on failure | FR-27 | Email fails routinely; without resend, receipts get lost | MEDIUM | Track sent/failed; manual resend |
| Staff downloads PDF manually (no-email donors) | FR-28 | Many donors have no email; system unusable for them otherwise | LOW | Fallback delivery |
| Store donor data (name, Tax ID/national ID, address, email) | FR-29 | Required for receipt content and e-Donation key-in | LOW-MEDIUM | Sensitive fields must be encrypted at rest (NFR-02) |
| Export donor/donation data (Excel/CSV) for e-Donation key-in | FR-30 | The hospital's legal route to e-Donation is manual key-in this phase | MEDIUM | **See Thai-specific note: monthly deadline drives this** |
| Audit trail of all actions (who/what/when), tamper-resistant | FR-13, NFR-05 | Tax/internal-control requirement; the system's credibility rests on it | MEDIUM-HIGH | Append-only; covers create/edit/approve/reject/cancel/issue/send |
| Search/filter records (name, date range, status, receipt no) | FR-10 | Unusable at volume without it | LOW-MEDIUM | Standard back-office list |
| Encryption in transit + at rest for sensitive fields | NFR-02 | PDPA legal requirement (national ID) | MEDIUM | HTTPS/TLS + column/field encryption |
| Record PDPA consent (text version + timestamp) | NFR-03, §7 | Legal precondition to storing personal data | LOW-MEDIUM | Store consent version + when/who; Flow A staff records donor's consent basis |
| Regular backup + restore | NFR-08 | Losing the receipt ledger is catastrophic | MEDIUM | Ops requirement; verify restore |

### Differentiators (Add Value, Defer Past Core)

These improve the product but are not required for a usable, compliant MVP. Several are explicitly "Should"/"Could" in the requirements.

| Feature | FR/NFR | Value Proposition | Complexity | Notes |
|---------|--------|-------------------|------------|-------|
| Public donor web form (Flow B) | FR-01, FR-02, FR-05 | Self-service intake; reduces staff data entry | HIGH | Requirements §10 explicitly defers to **Phase 2** |
| Spam/bot protection (CAPTCHA / rate limit) | FR-04 | Only needed once public form exists | LOW-MEDIUM | Pairs with Flow B |
| "Received your request" acknowledgement email (not a receipt) | FR-05 | Sets donor expectations; reduces "where's my receipt" calls | LOW | Pairs with Flow B |
| Bilingual (TH/EN) UI | NFR-06 | Serves foreign donors/staff | MEDIUM | Full bilingual UI deferred to Phase 2; MVP can be TH-first |
| Bilingual PDF receipt (per donor language) | FR-23 | Foreign donors need EN receipt | MEDIUM | Template variants; can follow after TH template proven |
| Bilingual email templates | FR-26 | Matches donor language | LOW-MEDIUM | "Should" priority |
| "Keyed into e-Donation" status flag | FR-31 | Prevents double-keying / missed entries against monthly deadline | LOW | High operational value, low cost — strong early add |
| Donation summary reports (period / totals) | FR-32 | Reconciliation with accounting | MEDIUM | "Should" priority |
| Admin-configurable templates, watermark, signature, number format | FR-33, NFR-09 | Change branding/wording without redeploy | MEDIUM-HIGH | NFR-09 makes config-not-code mandatory eventually; MVP can start with config files + a thin admin later |
| Approval queue / dashboard view of "pending" items | FR-08 | Efficiency for checkers; required once Flow B feeds a queue | LOW-MEDIUM | Becomes table-stakes in Phase 2 |
| Reissue / duplicate-receipt-on-request (lost receipt) | Open Issue §9.6 | Donors lose receipts; reissue with same number + "copy" mark | MEDIUM | Flagged as open question; not in current FR set — decide before building |

### Anti-Features (Deliberately NOT Building This Phase)

Explicitly out of scope per PROJECT.md / requirements §1.2 and §8. Documented to prevent scope creep.

| Feature | Why Requested | Why Problematic (this phase) | Alternative |
|---------|---------------|------------------------------|-------------|
| In-kind (สิ่งของ) donation receipts | Hospitals receive goods too | Valuation, different tax rules, different receipt content — large new domain | Cash only; revisit as separate milestone |
| Payment gateway / online payment | "Let donors pay on the site" | Staff confirm money via manual slip-check 100% today; gateway adds PCI scope, reconciliation, refunds | Donor uploads slip (Flow B); staff verifies manually |
| Direct e-Donation API integration | "Auto-submit to Revenue Dept" | No stable public API for unit-level integration; couples release cadence to gov system; high risk | **Export Excel/CSV (FR-30) + manual key-in.** Add "keyed" flag (FR-31) to manage it |
| General (non-donation) receipts | "We issue other receipts too" | Different legal content, numbering rules, workflows | Requirements §10 defers to **Phase 3** |
| Automatic integration with existing accounting system | "Sync the ledger" | This is an explicitly separate system this phase; integration is brittle | Manual reports/export (FR-32) for reconciliation |
| PKI digital signature on PDF | "Tamper-proof legal signature" | Certificate procurement, key management, cost; not needed to be a valid receipt for a hospital | **Signature image (FR-22)** for MVP; PKI is Open Issue §9.1, revisit if legal pushes |
| Auto-issue receipt without human approval | "Speed up issuance" | Violates the core invariant (FR-14) and the entire control model | Every receipt passes maker-checker — non-negotiable |
| Hard-delete of records / receipts | "Clean up mistakes" | Breaks gap-less integrity (FR-16/FR-19) and audit trail; tax law forbids | Status = cancelled; number retained; audit logged |
| PDPA "right to erasure" auto-delete | Donors may request deletion | Conflicts with ~5-year tax-document retention (§7); auto-deleting tax records is illegal | Documented retention policy; honor access/rectification, restrict erasure where tax law overrides — **confirm with DPO/legal** |

---

## Feature Dependencies

```
Authentication + RBAC (NFR-01, FR-34)
    └──requires──> (everything; nothing happens unauthenticated)

Maker-Checker approval (FR-12, FR-14)
    └──requires──> Status lifecycle (FR-11)
                       └──requires──> Record create/edit + slip view (FR-07, FR-09)

Gap-less FY receipt numbering (FR-15/16/17/18, NFR-04)
    └──allocated AT──> Approval commit (FR-14)         [NOT before]
    └──requires──> FY auto-detection (FR-18)
    └──enables───> PDF generation (FR-20) [needs the number]

PDF generation (FR-20)
    └──requires──> Template + watermark + signature (FR-20/21/22)
    └──requires──> Tax-compliant content + 1x/2x statement (FR-24, §6)
    └──feeds────> Email delivery (FR-25) ──fallback──> Manual download (FR-28)

Donor data store (FR-29) ──encrypted (NFR-02)──> required by PDF content + Export
    └──feeds────> e-Donation export (FR-30) ──tracked by──> "keyed" flag (FR-31)

PDPA consent (NFR-03) ──precondition──> storing donor data (FR-29)

Audit trail (FR-13, NFR-05) ──observes──> every state transition above

Public web form (Flow B, FR-01..05)  [PHASE 2]
    └──feeds────> Pending queue (FR-08) ──into──> existing maker-checker
    └──requires──> Spam protection (FR-04), bilingual UI (NFR-06)

Cancel (FR-19) ──conflicts with──> hard-delete (must never coexist)
Auto-issue ──conflicts with──> Maker-checker (FR-14) (must never coexist)
```

### Dependency Notes

- **Numbering depends on the approval transaction, not the reverse.** The number must be drawn from a per-fiscal-year atomic counter/sequence *inside* the same DB transaction that flips status to approved/issued. Pre-allocating or computing "next number" in app code before commit will create gaps or duplicates under concurrency (NFR-04). This single coupling is the project's highest technical risk.
- **PDF depends on numbering and on confirmed legal content.** Don't build the template until FR-24/§6 wording (especially the 1x/2x deduction statement) is confirmed with hospital accounting/legal — rebuilding the template after legal review is cheap; reissuing wrong receipts is not.
- **Export depends on donor-data completeness.** e-Donation key-in needs donor Tax ID/national ID; if intake doesn't capture it, the export is useless. Validate required fields at intake (FR-29).
- **PDPA consent gates data storage.** In Flow A, staff must record the lawful basis/consent before persisting donor PII. In Flow B, the form blocks submission until consent (FR-03).
- **Flow B reuses the maker-checker engine.** Build the approval + numbering + PDF core generic enough that a web-form-sourced "pending" item flows through the same gate. This is why Flow A first (Phase 1) then Flow B (Phase 2) is the right order.

---

## MVP Definition

### Launch With (Phase 1 — Back office, Flow A)

The minimum that lets the hospital stop the manual process and issue compliant receipts.

- [ ] Auth + RBAC with distinct Maker/Checker/Admin, server-enforced no-self-approval — NFR-01, FR-34
- [ ] Create/edit donation record + attach & view slip — FR-07, FR-09
- [ ] Status lifecycle state machine — FR-11
- [ ] Approve / return-with-reason — FR-12
- [ ] Receipt created only on approval — FR-14
- [ ] Gap-less, concurrency-safe, per-fiscal-year numbering + FY auto-detect + cancel-not-delete — FR-15/16/17/18/19, NFR-04
- [ ] PDF: template + watermark + signature image + tax-compliant content (1x/2x) — FR-20/21/22/24, §6
- [ ] Email receipt + send status/resend + manual download fallback — FR-25/27/28
- [ ] Encrypted donor data store + PDPA consent record — FR-29, NFR-02/03
- [ ] e-Donation Excel/CSV export — FR-30
- [ ] Audit trail (append-only) — FR-13, NFR-05
- [ ] Search/filter records — FR-10
- [ ] Backup/restore — NFR-08

### Add After Validation (Phase 1.x → enabling Phase 2)

- [ ] "Keyed into e-Donation" status flag — FR-31 — *trigger: first real monthly e-Donation submission cycle (cheap, high value, pull forward if easy)*
- [ ] Donation summary reports — FR-32 — *trigger: accounting asks for reconciliation*
- [ ] Admin config UI for templates/number format — FR-33, NFR-09 — *trigger: branding/wording change requested without redeploy*
- [ ] Bilingual PDF + email templates — FR-23, FR-26 — *trigger: first foreign-donor receipt*

### Future Consideration (Phase 2+)

- [ ] Public donor web form + slip upload + consent + bot protection — FR-01/02/03/04/05 — *Phase 2 per §10*
- [ ] Pending-queue dashboard for Flow B — FR-08 — *Phase 2*
- [ ] Full bilingual UI — NFR-06 — *Phase 2*
- [ ] Reissue/duplicate-on-request — Open Issue §9.6 — *decide policy first*
- [ ] General non-donation receipts, deep reports, auto e-Donation/accounting integration — *Phase 3 per §10*

## Feature Prioritization Matrix

| Feature | User Value | Implementation Cost | Priority |
|---------|------------|---------------------|----------|
| Gap-less FY numbering (concurrency-safe) | HIGH | HIGH | P1 |
| Maker-checker approval + status lifecycle | HIGH | MEDIUM | P1 |
| Tax-compliant PDF content (1x/2x) | HIGH | MEDIUM | P1 |
| Auth + RBAC (SoD enforced) | HIGH | MEDIUM | P1 |
| PDF template + watermark + signature | HIGH | MEDIUM | P1 |
| Email delivery + resend + download fallback | HIGH | MEDIUM | P1 |
| Audit trail (append-only) | HIGH | MEDIUM | P1 |
| Encrypted donor store + PDPA consent | HIGH | MEDIUM | P1 |
| e-Donation Excel/CSV export | HIGH | MEDIUM | P1 |
| Search/filter | MEDIUM | LOW | P1 |
| Backup/restore | HIGH | MEDIUM | P1 |
| "Keyed into e-Donation" flag | MEDIUM | LOW | P2 |
| Summary reports | MEDIUM | MEDIUM | P2 |
| Admin config UI | MEDIUM | MEDIUM-HIGH | P2 |
| Bilingual PDF/email | MEDIUM | MEDIUM | P2 |
| Public web form (Flow B) | HIGH | HIGH | P2/P3 |
| Reissue/duplicate | LOW-MEDIUM | MEDIUM | P3 |

**Priority key:** P1 = must have for Phase 1 launch · P2 = add after validation · P3 = future / Phase 2+

---

## Thai-Specific Requirements (Surfaced)

These are the domain specifics that generic "receipt system" knowledge misses. Confidence HIGH unless noted.

1. **Fiscal-year numbering (1 Oct – 30 Sep).** Receipt number = Buddhist-era fiscal year + running no. (e.g., `2569/000123`). A receipt issued 15 Oct 2568 belongs to FY **2569**. Counter resets to 1 each 1 Oct (FR-15/17/18). This is a Thai-government fiscal-year convention, not calendar year.

2. **1x vs 2x deduction for hospitals.** Donations to **government/state hospitals** (and Thai Red Cross, certain state entities) qualify for **double (2x) deduction**, capped so total such donations don't exceed **10% of net income after allowances**. **Private hospitals generally do NOT qualify** unless donated through the hospital's registered foundation (then 1x). The receipt must carry the correct deduction-rights statement. *Action: confirm this hospital's exact eligibility (2x list at rd.go.th/27811.html) and the precise legal wording with hospital accounting/legal — Open Issue §6/§9.3.* (Confidence HIGH on the rule; MEDIUM on this specific hospital's status — must verify.)

3. **e-Donation monthly upload deadline.** Recipient organizations entering donations into e-Donation must submit data **within the month of donation, no later than the 5th of the following month** (and batch upload uses a Revenue-Department-provided Excel template). This makes the FR-30 export + FR-31 "keyed" flag operationally time-critical: design the export around a monthly cadence and make it easy to see what's unkeyed before the 5th. (Confidence HIGH — from RD e-Donation user manual.)

4. **e-Donation as evidence / 2026 mandate trend.** From **1 Jan 2026**, donations to many recipients (notably non-government hospitals and institutions) must go **through e-Donation** to be deductible — the digital record replaces the paper receipt. Government hospitals "designated by the Minister" may be exempt. **This is a compliance flag, not an MVP scope change:** manual key-in remains the route this phase, but confirm whether this hospital falls under the mandate, because it could later force direct e-Donation integration (an explicit anti-feature today). (Confidence HIGH on the trend; the hospital's exact obligation must be confirmed with legal.)

5. **PDPA vs tax retention conflict.** Tax law requires retaining receipts/records (commonly ~5 years); PDPA grants a right to erasure. These conflict. Policy: honor access/rectification, but **restrict erasure where overridden by tax-record retention law**, with a documented, DPO-approved retention policy (§7, Open Issue §9.4). Store consent **version + timestamp** (NFR-03). National ID / Tax ID are sensitive → encrypt at rest + restrict viewing by role (NFR-02).

6. **Receipt content checklist (§6) for deductibility:** hospital name/address + hospital Tax ID; running receipt no + issue date; donor name + Tax ID/national ID + address; amount in **digits and Thai words**; purpose/type of donation; deduction-rights statement (incl. 2x where applicable); authorized signature + hospital seal.

---

## Competitor / Prior-Art Feature Analysis

| Feature | Generic receipt/invoicing SaaS | Banking maker-checker / fintech | Our Approach |
|---------|-------------------------------|----------------------------------|--------------|
| Numbering | Sequential, often calendar-year, gaps tolerated | Strict sequence per ledger | Per **fiscal-year** atomic counter, gap-less, allocated at approval commit |
| Approval | Often single-user / optional | Mandatory four-eyes, code-enforced SoD | Mandatory maker-checker, no self-approval, reason on reject |
| Audit | Basic activity log | Immutable/append-only, WORM | Append-only audit of all transitions (FR-13/NFR-05) |
| Cancellation | Hard delete or void | Void/reverse, never delete | Status = cancelled, number retained (FR-19) |
| Delivery | Email/portal | n/a | Email + manual download fallback (no-email donors) |
| Tax compliance | Generic VAT/invoice | n/a | Thai 1x/2x deduction statement + §6 content + e-Donation export |

---

## Sources

- Thai Revenue Department — รายชื่อสถานพยาบาลของทางราชการ ที่หักลดหย่อนเงินบริจาคได้ 2 เท่า: https://rd.go.th/27811.html (HIGH — official 2x-eligible hospital list)
- Thai Revenue Department — e-Donation user manual for recipient units (epayapp.rd.go.th/rd-edonation): batch upload Excel template + monthly/by-5th deadline (HIGH)
- PwC Worldwide Tax Summaries — Thailand individual deductions (double deduction, 10% cap): https://taxsummaries.pwc.com/thailand/individual/deductions (HIGH)
- Bangkok Global Law — Tax deductions for e-Donations, healthcare/education, 2026 mandate: https://www.bgloballaw.com/2025/05/15/tax-deductions-for-e-donations-made-to-the-healthcare-and-educational-foundation/ (MEDIUM-HIGH)
- etaxgo — e-Donation 1x vs 2x explainer: https://www.etaxgo.com/blog/tips/what-is-e-donation/ (MEDIUM)
- Punpro — hospital donation deduction eligibility: https://www.punpro.com/p/tax-donate-hospital (MEDIUM)
- Maker-checker process / four-eyes / SoD + audit trail patterns: https://blog.xtrm.com/posts/maker-checker-process , https://www.opcito.com/blogs/maker-checker-implementation-guide-for-secure-fintech-systems (MEDIUM — industry standard patterns)
- Project requirements doc `requirements-ระบบออกใบเสร็จบริจาค.md` v1.1 + `.planning/PROJECT.md` (HIGH — primary source for FR/NFR mapping)

---
*Feature research for: Thai hospital donation e-receipt system (DonaRec)*
*Researched: 2026-06-22*
