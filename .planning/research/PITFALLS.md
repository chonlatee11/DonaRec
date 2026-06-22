# Pitfalls Research

**Domain:** Thai hospital donation e-receipt system (tax-deductible, compliance-critical document issuance)
**Researched:** 2026-06-22
**Confidence:** HIGH (gap-less numbering, PDPA-vs-tax retention, Thai PDF rendering, email deliverability verified across multiple sources; e-Donation rules verified against Revenue Department + tax-advisory sources; specific receipt-text legal wording remains MEDIUM and must be confirmed with the hospital's accounting/legal team)

> This domain has an unusually high "correctness tax." A duplicate or skipped receipt number, a malformed Thai tone mark on a legal document, or an erasure of a record the Revenue Code requires you to keep are not cosmetic bugs — they are audit/legal failures. The pitfalls below are ordered by how expensive they are to fix after the fact.

---

## Critical Pitfalls

### Pitfall 1: Treating gap-less numbering as a DB sequence (or computing the number before commit)

**What goes wrong:**
The team uses a PostgreSQL `SEQUENCE`, `AUTO_INCREMENT`, or `MAX(running_no)+1` to assign receipt numbers. The result has gaps (sequences never roll back on a failed/rolled-back transaction, and `CACHE > 1` loses values on restart) or duplicates (two concurrent `MAX+1` reads return the same value). Either way FR-16/NFR-04 ("ไม่ซ้ำ และเรียงต่อเนื่อง ห้ามข้ามเลข") is violated — and you usually discover it during a tax audit, not in testing.

**Why it happens:**
"Sequence" sounds like exactly what's needed, and it works perfectly in single-user dev testing. The gap-vs-uniqueness distinction only surfaces under concurrency and under rollback — neither of which a happy-path demo exercises. Most ORMs default to the sequence/auto-increment path.

**How to avoid:**
- Use a dedicated **counter table keyed by fiscal year**, e.g. `receipt_counter(fiscal_year PK, last_no)`, and allocate with `SELECT ... FOR UPDATE` (or `UPDATE ... RETURNING last_no`) **inside the same transaction that inserts the receipt row**. The row lock serializes allocation; on rollback the number is never consumed, so no gap.
- Allocate the number **at the moment of approval/commit**, never precompute it on the draft (the draft may be rejected → that would create a gap).
- Add a `UNIQUE(fiscal_year, running_no)` constraint as a backstop so any logic bug fails loudly instead of silently duplicating.
- Add short retry-with-backoff around the transaction for the rare deadlock/serialization error.
- Keep the approve-and-issue transaction **short** — do NOT generate the PDF or send email inside the lock-holding transaction (see Pitfall 7).

**Warning signs:**
- Code calls `nextval`, relies on `@GeneratedValue`/`AUTO_INCREMENT`, or does `SELECT MAX(...)+1`.
- The receipt number is shown/stored on the draft before approval.
- PDF rendering or SMTP send happens inside the numbering transaction.
- Load/concurrency test (≥2 simultaneous approvals) is missing from the test plan.

**Phase to address:**
Phase 1 (MVP back-office numbering). This is the single highest-risk item — design and concurrency-test it first.

---

### Pitfall 2: Wrong fiscal-year derivation at the Oct–Dec boundary (and Buddhist-year off-by-one)

**What goes wrong:**
A receipt issued on 15 Oct 2568 (Gregorian 2025) is stamped fiscal year 2568 instead of 2569, or the running number does not reset to 1 on 1 Oct. Because the Thai fiscal year runs 1 Oct – 30 Sep, every receipt issued in Oct–Dec belongs to the *next* Buddhist-era fiscal year (FR-18). Getting this wrong corrupts the entire year's numbering and is very hard to unwind once receipts are issued.

**Why it happens:**
Developers reflexively map "year" to `date.getFullYear()` (calendar year, Gregorian). Two transforms stack: calendar→fiscal year AND Gregorian→Buddhist era (+543). Tests written in, say, March never hit the Oct–Dec edge. Timezone bugs (UTC vs Asia/Bangkok) can push a late-night 30 Sep issuance into 1 Oct.

**How to avoid:**
- Centralize a single pure function `fiscalYear(issueDate) -> beYear` and unit-test it with explicit boundary cases: 30 Sep 23:59 Bangkok, 1 Oct 00:00 Bangkok, 31 Dec, 1 Jan. Document the rule inline (Oct→ next BE year).
- Base the fiscal year strictly on the **approval/issue date** (FR-18), never the donation date or the draft-creation date — a Sept donation approved in Oct is a next-FY receipt.
- Pin all date math to `Asia/Bangkok`; never derive fiscal year from a UTC timestamp.
- Make the counter table key the fiscal year so "reset to 1" is automatic (a new FY = a new row starting at 0), rather than a scheduled job that can mis-fire.

**Warning signs:**
- Any `getFullYear()` / `YEAR(date)` used directly for the receipt number.
- A "+543" sprinkled in multiple places instead of one helper.
- A cron/scheduled "reset counter on Oct 1" job exists (fragile — prefer FY-keyed counter).
- No tests dated in October.

**Phase to address:**
Phase 1, together with Pitfall 1 (same numbering subsystem).

---

### Pitfall 3: PDPA right-to-erasure vs. tax-record retention conflict handled as "just delete it"

**What goes wrong:**
A donor invokes their PDPA right to erasure; the team hard-deletes the donor + receipt records. This destroys documents the Revenue Code/accounting law requires the hospital to retain (commonly cited as ~5 years), breaking audit/tax compliance and the immutable audit trail (NFR-05). Conversely, some teams refuse *all* erasure requests, which over-retains and breaches PDPA's storage-limitation principle.

**Why it happens:**
GDPR/PDPA training emphasizes "right to be forgotten" as near-absolute. Teams don't realize the right is **not absolute** — it is overridden where retention is required to comply with a legal obligation (tax) or to establish/exercise/defend legal claims. The conflict is a policy decision, not a code default.

**How to avoid:**
- Treat erasure as a **policy-gated workflow**, not a DELETE. Records under a statutory retention period are exempt from erasure for the duration of that obligation; record the legal basis on each refusal.
- Model retention explicitly: store a `retain_until` derived from the tax-record rule, and a documented legal basis ("legal obligation – Revenue Code"). Only data *not* under a legal hold is erasable/de-identifiable.
- Prefer **de-identification/anonymization** for any erasure that must coexist with audit/financial integrity, rather than physical deletion of the receipt record.
- Capture in the consent/privacy notice (FR-03) that tax-receipt data is retained for the legally mandated period and the erasure right is limited accordingly.
- **Get the exact retention period and wording confirmed by the hospital's legal/DPO + accounting** (Open Issue #4) — do not hardcode "5 years" without confirmation.

**Warning signs:**
- An erasure feature that issues a hard `DELETE` on donor/receipt tables.
- No `retain_until` / legal-hold concept in the data model.
- Privacy notice silent on tax-retention override of erasure.
- Audit trail rows can be deleted by the erasure path.

**Phase to address:**
Phase 1 for the data model (retain_until, soft-delete/anonymize, immutable audit). The donor-facing erasure *request* workflow can land in Phase 2, but the model and policy must exist from Phase 1 so you don't retrofit.

---

### Pitfall 4: Thai tone marks / combining characters broken in the generated PDF

**What goes wrong:**
The PDF renders Thai vowels and tone marks (สระ/วรรณยุกต์ such as ่ ้ ๊ ๋ ◌ี ◌ํ) misplaced, overlapping, or as empty boxes (□). A donation receipt with mangled Thai on a legal document is unacceptable and may be rejected. A known subtle variant: marks render fine when a string starts with Thai but break when it starts with a Latin character (font-run segmentation / GPOS issue).

**Why it happens:**
Thai is a complex script needing proper OpenType GPOS mark positioning. Many PDF toolchains don't shape Thai correctly: `wkhtmltopdf` (Qt/libthai) has long-standing tone-mark bugs; jsPDF/TCPDF need PUA (U+F700–F71F) character substitution to stack marks; aggressive font subsetting can drop substituted glyphs. Missing the Thai font at the OS level yields boxes.

**How to avoid:**
- Choose a renderer with real HarfBuzz/Pango shaping (e.g. WeasyPrint, or a headless-Chromium/Playwright HTML-to-PDF path) over `wkhtmltopdf` for Thai. Verify the choice against actual receipt text early — this is a stack-selection decision.
- **Embed** a proper Thai font that includes Latin glyphs (TH Sarabun New / Sarabun / Noto Sans Thai) via `@font-face`; install it at OS level too. Force one font family across all fields.
- Consider `subset: false` (or careful subsetting) so substituted/stacked-mark glyphs survive.
- Build a **golden-file visual test**: a fixed receipt with worst-case Thai (stacked tone marks, mixed Thai+Latin+digits, long donor names, ฿ and Thai-text amounts) rendered and visually diffed in CI. Include strings that *start with Latin*.

**Warning signs:**
- `wkhtmltopdf` chosen without a Thai rendering spike.
- Fonts referenced by family name only, not embedded.
- No visual/golden-file test for the PDF; correctness judged by "it looks fine in the browser."
- Test data uses only ASCII or simple Thai without stacked marks.

**Phase to address:**
Phase 1. Do a rendering spike with real Thai receipt text *before* committing the PDF library in the stack.

---

### Pitfall 5: Approval workflow without true segregation of duties or an immutable audit trail

**What goes wrong:**
Maker and Checker end up being the same person (or one user holds both roles), or approval/issuance actions are editable/deletable after the fact, or the audit trail (FR-13/NFR-05) is missing the who/what/when on key transitions. This defeats the entire control purpose (US-3) and fails internal-control/audit expectations.

**Why it happens:**
Segregation-of-duties is flagged as an Open Issue (#2) rather than decided, so it slips. Audit logging is often bolted on late and treated as app logs (mutable, purgeable) rather than a tamper-evident record. "Cancel = delete the row" feels simpler than a status transition.

**How to avoid:**
- Enforce in code that the approving user ≠ the creating user for the same record (configurable per hospital policy, but default to enforced per Key Decision in PROJECT.md).
- Make the audit trail **append-only and immutable** (no UPDATE/DELETE on audit rows; restrict via DB grants). Log every state transition (draft→pending→approved/issued→rejected→cancelled) with actor, timestamp, before/after, and reason on reject/cancel (FR-12, FR-19).
- Implement cancellation as a **status change ("ยกเลิก"), never a delete** (FR-19) — the receipt number stays consumed and visible, preserving gap-less integrity.
- Bind the audit trail to the retention/PDPA policy (Pitfall 3) so audit rows are never caught by erasure.

**Warning signs:**
- Role check allows the same user to create and approve.
- Audit data lives in the same mutable table as the entity, or in app log files only.
- A "delete receipt" action exists anywhere.
- Reject/cancel does not require a reason.

**Phase to address:**
Phase 1 (status machine, RBAC, audit trail). These are foundational and expensive to retrofit.

---

### Pitfall 6: Receipt legal content/wording assumed instead of confirmed (incl. 2x deduction & e-Donation mandate)

**What goes wrong:**
The receipt is missing a required field or uses wrong wording for tax deductibility (Section 6 of requirements), or it incorrectly claims/omits the **double (2x) deduction** that applies to public/state hospitals. Worse: the project assumes "manual export to e-Donation" indefinitely, but from **1 January 2026 most donations must flow through the Revenue Department's e-Donation system to be deductible** — and the recording deadline is tight (no later than the 5th of the following month).

**Why it happens:**
The team treats receipt text as UI copy, not regulated content. The 2x ("จ่าย 1 ได้ 2") benefit is hospital-category-specific (public vs private) and capped at 10% of net income — easy to state wrongly. The e-Donation mandate and its monthly cutoff are recent rules the original requirements ("เจ้าหน้าที่คีย์เอง / export แมนวล") predate.

**How to avoid:**
- Treat receipt fields/wording (Section 6, Open Issue #3) as **legal content to be signed off by the hospital's accounting/legal/DPO**, including the precise 2x-deduction statement and the hospital's tax ID — verify this hospital actually qualifies for 2x.
- Make all such text **configurable without deploy** (NFR-09) so a legal correction is a config change, not a release.
- Re-validate the e-Donation assumption now: confirm whether this hospital is in the exempt category or must submit via e-Donation, and design the export to match e-Donation's required fields (13-digit national/tax ID, donation date, cash type) and its **monthly cutoff** so manual keying isn't structurally late.
- Add a "keyed into e-Donation" status (FR-31) and surface aging against the 5th-of-next-month deadline to prevent dropped/late entries.

**Warning signs:**
- Receipt text hardcoded; no sign-off artifact from accounting/legal.
- The system claims a fixed deduction multiple without verifying hospital category.
- Export format invented ad hoc rather than mapped to e-Donation fields.
- No tracking of the monthly e-Donation submission deadline.

**Phase to address:**
Phase 1 (receipt content config + export skeleton). Flag e-Donation mandate verification as a **research/stakeholder task at the start of Phase 1** — it may expand scope.

---

### Pitfall 7: PDF generation and email send coupled to (or blocking) the issuance transaction

**What goes wrong:**
Approval generates the number, renders the PDF, and sends the email all synchronously. The render or SMTP call is slow or fails, so either the numbering transaction holds its lock too long (serializing all other approvals and tanking throughput) or a transient email failure rolls back the approval — leaving the donor without a receipt and possibly burning or skipping a number.

**Why it happens:**
The naive flow is "approve → make PDF → email → done" in one request. Gap-less numbering already forces serialization (Pitfall 1); adding slow I/O inside the lock multiplies the cost. Email delivery is inherently unreliable and shouldn't be in a DB transaction.

**How to avoid:**
- Commit the **number + receipt record first** (short transaction). Then render the PDF and send email **outside** the transaction, ideally via an async job/queue with retry.
- Persist the PDF and a **send status** (FR-27: success/failure) so failures are retryable without re-issuing a number.
- Make PDF render + email idempotent per receipt (re-send must not allocate a new number).
- Meet NFR-07 (~2–3s/receipt) by measuring render time separately from the lock-critical path.

**Warning signs:**
- Approve handler calls render+SMTP before COMMIT.
- An email failure leaves no issued receipt (number not persisted).
- Resend re-runs the numbering logic.

**Phase to address:**
Phase 1 (issuance pipeline architecture).

---

### Pitfall 8: Sensitive ID (national ID / tax ID) stored, logged, or displayed insecurely

**What goes wrong:**
The 13-digit national ID / tax ID (FR-29, NFR-02) is stored in plaintext, written into audit logs or app logs, shown in full to every role, or exported to CSV that anyone can grab. This is a PDPA breach of sensitive data and undermines the access-control requirement (Section 7).

**Why it happens:**
The ID is needed end-to-end (receipt, e-Donation export), so it's tempting to keep it plainly available everywhere. Encryption-at-rest and role-scoped masking are "NFRs" that get deferred.

**How to avoid:**
- Encrypt the national ID at rest (NFR-02); decrypt only where strictly needed (PDF generation, e-Donation export) under access control.
- **Mask by default** in UI/search/reports (e.g. show last 4), reveal full value only to roles that require it (Section 7) and log the reveal.
- Never write full IDs into audit/app logs; redact.
- Protect the e-Donation export file (access-controlled, expiring, logged) — it concentrates the most sensitive data of all donors in one file.

**Warning signs:**
- National ID column is plaintext.
- Logs/audit rows contain full IDs.
- All roles see full IDs; export download is unrestricted.

**Phase to address:**
Phase 1 (data model + RBAC). Export hardening alongside FR-30.

---

## Technical Debt Patterns

| Shortcut | Immediate Benefit | Long-term Cost | When Acceptable |
|----------|-------------------|----------------|-----------------|
| DB sequence/AUTO_INCREMENT for receipt no. | Trivial to build | Gaps/dupes → audit failure; hard to retrofit | **Never** for the legal receipt number |
| Synchronous PDF+email inside issuance txn | One simple handler | Lock contention; email failure breaks issuance | MVP only if single-user and clearly flagged to refactor |
| Image signature instead of PKI | No certificate cost (per Key Decision) | Lower legal tamper-resistance | Acceptable for MVP (Open Issue #1) — revisit if legal weight needed |
| Hardcoded receipt wording / fiscal-year format | Faster first render | Legal change = redeploy; violates NFR-09 | Never — make configurable from the start |
| Hard-delete for "cancel"/erasure | Simplest data op | Breaks gap-less + tax retention + audit | **Never** — use status/anonymize |
| Plaintext national ID | Skip key management | PDPA breach exposure | Never |

## Integration Gotchas

| Integration | Common Mistake | Correct Approach |
|-------------|----------------|------------------|
| Revenue Dept e-Donation | Assuming indefinite "manual export, no rush" | From 1 Jan 2026 most donations must go via e-Donation to be deductible; record by the **5th of the following month**. Verify hospital's category and match export to required fields (13-digit ID, date, cash) |
| SMTP / email provider | Sending receipt PDFs from an unauthenticated domain | Configure aligned SPF + DKIM (2048-bit) + DMARC; authenticated senders ~2.7x more likely to inbox |
| Email with PDF attachment | Attaching large PDFs / generic "invoice.pdf" names → spam/550 blocks | Keep PDF <1MB (clean receipt 50–300KB), neutral filename, no ZIP; consider a hosted/portal link as fallback; use a dedicated transactional sending stream |
| Slip upload (Flow B) | Trusting client-declared MIME type | Validate real file type + size server-side (FR-02); store outside webroot; scan |

## Performance Traps

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|----------------|
| Lock held during PDF/email in numbering txn | Approvals queue up, latency spikes | Commit number first, do I/O async | Even at 2 concurrent approvers (gap-less serializes by design) |
| PDF render time inside the ~2–3s budget under load | NFR-07 missed when several issue at once | Async render queue; measure render separately | Modest concurrency (handful of simultaneous approvals) |
| Counter table bloat (PostgreSQL) | VACUUM pressure on hot counter row | Keep counter table un-overindexed for HOT updates; short txns | High issuance volume |
| Email send blocking request thread | Slow/timeout on approve | Queue + retry (FR-27) | Any SMTP slowdown/outage |

## Security Mistakes

| Mistake | Risk | Prevention |
|---------|------|------------|
| Plaintext national/tax ID | PDPA sensitive-data breach | Encrypt at rest; mask in UI; restrict by role (NFR-02, Section 7) |
| Mutable/deletable audit trail | Loss of tamper-evidence for tax/audit | Append-only audit; revoke UPDATE/DELETE grants (NFR-05) |
| Unrestricted e-Donation export download | Mass donor-PII leak | Access-control + expiry + download logging |
| Receipt PDF guessable URL / no auth | Donor data exposure | Signed/expiring links or authenticated portal |
| Slip upload accepts any file | RCE/stored-XSS/malware | Server-side type+size validation, store outside webroot, no execution (FR-02, FR-04) |
| Missing rate-limit/CAPTCHA on public form | Spam/bot floods of fake donations | FR-04 rate limiting + CAPTCHA (Phase 2 public web) |

## UX Pitfalls

| Pitfall | User Impact | Better Approach |
|---------|-------------|-----------------|
| Showing a receipt "number" on drafts/pending | Staff/donor think a receipt exists before approval | No number until issued; clear status labels (FR-11) |
| Donor with no email blocked from getting receipt | Real-world donors lack email | Staff PDF download fallback (FR-28) |
| English-only UI for receipt content but Thai donors | Wrong-language receipt/email | Drive PDF + email language from donor's chosen language (FR-23, FR-26) |
| Silent email failure | Donor never receives receipt, nobody notices | Surface send status + resend (FR-27) |
| No visibility into e-Donation keying backlog | Late/missed submissions past the 5th | Status + aging view (FR-31) |

## "Looks Done But Isn't" Checklist

- [ ] **Receipt numbering:** Looks fine single-user — verify under ≥2 concurrent approvals AND after a forced rollback (no gap, no dup, UNIQUE constraint present).
- [ ] **Fiscal year:** Looks fine in summer — verify 30 Sep 23:59 vs 1 Oct 00:00 Asia/Bangkok, BE +543, and reset-to-1 on FY rollover.
- [ ] **Thai PDF:** Looks fine in browser — verify in the actual PDF with stacked tone marks, mixed Thai+Latin starting with a Latin char, long names, amounts in Thai words.
- [ ] **Erasure/retention:** Looks compliant — verify erasure does NOT delete records under tax retention; `retain_until` + legal basis recorded; audit rows immune.
- [ ] **Audit trail:** Looks logged — verify it's append-only (try to UPDATE/DELETE; should be denied) and covers every state transition with actor/time/reason.
- [ ] **Cancellation:** Looks handled — verify it's a status change, number still consumed/visible, never deleted (FR-19).
- [ ] **Email:** Looks sent — verify SPF/DKIM/DMARC aligned and a real send (with the real PDF) lands in inbox, not spam; resend doesn't re-number.
- [ ] **e-Donation export:** Looks exported — verify field mapping matches Revenue Dept requirements and respects the monthly deadline; export is access-controlled.
- [ ] **National ID:** Looks stored — verify encrypted at rest, masked in UI, absent from logs.

## Recovery Strategies

| Pitfall | Recovery Cost | Recovery Steps |
|---------|---------------|----------------|
| Gaps/dupes already in issued receipts | HIGH | Reconcile with accounting; you generally cannot renumber issued legal receipts — document gaps/cancellations, fix the allocator, add UNIQUE constraint going forward |
| Wrong fiscal year on issued receipts | HIGH | Cannot silently re-stamp; coordinate corrections with accounting (cancel+reissue under status, not delete) |
| Hard-deleted a record needed for tax retention | HIGH | Restore from backup (NFR-08); if unrecoverable, document the loss — potential compliance incident |
| Broken Thai glyphs only found post-issue | MEDIUM | Reissue corrected PDF under same number (status/version note); fix renderer + add golden test |
| Receipt wording legally wrong | MEDIUM | Correct config (NFR-09), reissue affected receipts, sign-off with legal |
| Emails landing in spam | LOW–MEDIUM | Fix SPF/DKIM/DMARC, shrink PDF, neutral filename, separate transactional stream, resend |

## Pitfall-to-Phase Mapping

| Pitfall | Prevention Phase | Verification |
|---------|------------------|--------------|
| 1. Gap-less numbering under concurrency | Phase 1 (first) | Concurrency + rollback test; UNIQUE(fiscal_year, running_no) |
| 2. Fiscal-year / Oct–Dec / BE boundary | Phase 1 | Unit tests on Sep/Oct/Dec boundaries in Asia/Bangkok |
| 3. PDPA erasure vs tax retention | Phase 1 (model+policy); Phase 2 (donor request UI) | Erasure leaves retained records; retain_until + legal basis present |
| 4. Thai tone-mark PDF rendering | Phase 1 (stack spike first) | Golden-file visual diff incl. Latin-leading strings |
| 5. SoD + immutable audit trail | Phase 1 | Same-user approve blocked; audit UPDATE/DELETE denied |
| 6. Receipt legal content + e-Donation mandate | Phase 1 (config + export); stakeholder verify at phase start | Accounting/legal sign-off; export matches e-Donation fields/deadline |
| 7. Issuance txn coupled to PDF/email | Phase 1 | Number persists on email failure; resend doesn't re-number |
| 8. Sensitive ID handling | Phase 1 | ID encrypted, masked, absent from logs; export access-controlled |

## Sources

- CYBERTEC — Gaps in sequences / PostgreSQL sequences vs invoice numbers: https://www.cybertec-postgresql.com/en/gaps-in-sequences-postgresql/ , https://www.cybertec-postgresql.com/en/postgresql-sequences-vs-invoice-numbers/ (HIGH)
- AppMaster — Concurrency-safe invoice numbering: https://appmaster.io/blog/concurrency-safe-invoice-numbering (MEDIUM)
- kimmobrunfeldt — Postgres gapless counter for invoices: https://github.com/kimmobrunfeldt/howto-everything/blob/master/postgres-gapless-counter-for-invoice-purposes.md (MEDIUM)
- OneTrust — Thai PDPA guide; DLA Piper — Thailand data protection; Norton Rose Fulbright — PDPA overview (right to erasure not absolute; legal-obligation/storage-limitation exemptions): https://www.onetrust.com/blog/the-ultimate-guide-to-thai-pdpa-compliance/ , https://www.dlapiperdataprotection.com/index.html?t=law&c=TH , https://www.nortonrosefulbright.com/en/knowledge/publications/e29d223d/overview-of-thailand-personal-data-protection-act-be2562-2019 (HIGH)
- PwC Worldwide Tax Summaries — Thailand individual/corporate deductions (public hospital 2x, 10% cap, cash-only): https://taxsummaries.pwc.com/thailand/individual/deductions (HIGH)
- Bangkok Global Law / PwC Thailand Tax Alert — e-Donation mandate from 1 Jan 2026: https://www.bgloballaw.com/2025/05/15/tax-deductions-for-e-donations-made-to-the-healthcare-and-educational-foundation/ (MEDIUM)
- Forvis Mazars — e-Donation system overview; Revenue Department e-Donation portal + user manual (recording deadline 5th of following month, 13-digit ID, manual/upload channels): https://www.forvismazars.com/th/en/insights/doing-business-in-thailand/tax/e-donation-system , https://epayapp.rd.go.th/rd-edonation/ (MEDIUM)
- wkhtmltopdf Thai rendering issues #2087/#3944/#5202/#2496; pdfme #1347 (Latin-leading tone-mark break); TCPDF bug #719 (PUA substitution); jsPDF #2650; pdfmake-thai (TH Sarabun New): https://github.com/wkhtmltopdf/wkhtmltopdf/issues/2087 , https://github.com/pdfme/pdfme/issues/1347 , https://sourceforge.net/p/tcpdf/bugs/719/ , https://github.com/pumzth/pdfmake-thai (HIGH for "wkhtmltopdf Thai is unreliable"; MEDIUM for WeasyPrint/HarfBuzz being better — verify with a spike)
- Suped / Mailjet / SMTP2GO / Microsoft Q&A / DMARC Report — transactional email deliverability, PDF attachment risk, SPF/DKIM/DMARC: https://www.suped.com/learn/email-deliverability/do-pdf-attachments-negatively-impact-email-deliverability-and-what-are-the-best-practices , https://dmarcreport.com/blog/improve-email-deliverability-dmarc-spf-dkim-alignment-check-guide/ (MEDIUM)

---
*Pitfalls research for: Thai hospital donation e-receipt system (DonaRec)*
*Researched: 2026-06-22*
