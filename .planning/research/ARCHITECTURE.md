# Architecture Research

**Domain:** Thai hospital donation e-receipt system (back-office maker/checker workflow + public donation form, gap-less fiscal-year receipt numbering, PDF generation, email delivery, PDPA/audit)
**Researched:** 2026-06-22
**Confidence:** HIGH (numbering concurrency design verified against PostgreSQL docs + multiple expert sources; component structure is standard maker-checker/document-workflow architecture)

## Standard Architecture

This is a **modular monolith** with an asynchronous worker for slow side-effects (PDF + email). A monolith is the correct choice: the domain is a single bounded context (donation → receipt), volume is low (a hospital issues hundreds–low-thousands of receipts/year, not per second), and the hard requirement (gap-less numbering inside one DB transaction) is *easiest* to guarantee when everything shares one database. Microservices would add distributed-transaction risk for zero benefit here.

The one component that should be **out-of-band** is the PDF+email side-effect, because NFR-07 wants approval to feel fast and FR-27 requires retry on email failure — both point to a durable job queue rather than doing this work inline in the approval HTTP request.

### System Overview

```
┌──────────────────────────────────────────────────────────────────────┐
│                        CLIENTS                                         │
│  ┌──────────────┐   ┌──────────────────┐   ┌──────────────────────┐  │
│  │ Public donor │   │ Back-office SPA  │   │  Admin SPA           │  │
│  │ form (Flow B)│   │ Maker / Checker  │   │  settings / users    │  │
│  └──────┬───────┘   └────────┬─────────┘   └──────────┬───────────┘  │
└─────────┼────────────────────┼─────────────────────────┼──────────────┘
          │ (public, rate-ltd) │ (authn + RBAC)           │ (authn + RBAC)
┌─────────┴────────────────────┴─────────────────────────┴──────────────┐
│                     APPLICATION (modular monolith)                     │
├────────────────────────────────────────────────────────────────────── ┤
│  ┌───────────┐ ┌───────────────┐ ┌──────────────┐ ┌────────────────┐  │
│  │ Auth/RBAC │ │ Donation core │ │ Approval /   │ │ Receipt number │  │
│  │  module   │ │ (records, FSM)│ │ workflow     │ │  module (★)    │  │
│  └───────────┘ └───────┬───────┘ └──────┬───────┘ └───────┬────────┘  │
│  ┌───────────┐ ┌───────┴───────┐ ┌──────┴───────┐ ┌───────┴────────┐  │
│  │ Slip/file │ │ Donor / PII   │ │ Audit log    │ │ Config / tmpl  │  │
│  │ storage   │ │ (encrypted)   │ │ (append-only)│ │ store (NFR-09) │  │
│  └───────────┘ └───────────────┘ └──────────────┘ └────────────────┘  │
│  ┌──────────────────┐ ┌─────────────────────────────────────────────┐ │
│  │ e-Donation export│ │  Job enqueue (transactional outbox)         │ │
│  └──────────────────┘ └───────────────────┬─────────────────────────┘ │
└─────────────────────────────────────────────┼─────────────────────────┘
          ┌───────────────────────────────────┼──────────────────┐
          │                                    │                  │
┌─────────┴─────────┐              ┌───────────┴────────┐  ┌──────┴──────┐
│  PostgreSQL       │              │  Worker process    │  │  Object/file│
│  - donation_record│              │  - PDF render      │  │  store      │
│  - donor (enc PII)│◄─────────────┤  - email send+retry│  │  - slips    │
│  - receipt_counter│   reads/     │  (consumes queue)  │  │  - receipts │
│  - audit_log      │   writes     └─────────┬──────────┘  │  - assets   │
│  - job_queue      │                        │             └─────────────┘
│  - config/templates│                       ▼
└───────────────────┘              ┌────────────────────┐
                                   │  SMTP / email API  │
                                   └────────────────────┘

★ = highest-correctness component (gap-less numbering)
```

### Component Responsibilities

| Component | Responsibility (owns) | Boundary / talks to |
|-----------|------------------------|----------------------|
| **Auth/RBAC** | Login, password hashing, sessions, role checks (Donor/Maker/Checker/Admin), field-level visibility of national ID (PDPA) | Gatekeeper in front of all back-office/admin endpoints; writes to audit log on auth events |
| **Donation core** | The `donation_record` aggregate + its **state machine** (draft→pending→approved/issued→rejected→cancelled); validation; search/filter (FR-10) | Calls Donor/PII for personal data; calls Slip storage for attachments; emits state transitions to Audit |
| **Approval/workflow** | Maker submit, Checker approve/reject with reason, segregation-of-duties enforcement (maker ≠ checker) | Orchestrates the **issue transaction**: state→issued + number allocation + outbox enqueue, all in one DB tx |
| **Receipt number (★)** | Allocate gap-less per-fiscal-year running number at the moment of approval commit; derive fiscal year from issue date | Pure DB-transaction service called only by Approval; no other caller may touch the counter |
| **Donor / PII** | Donor identity records; **encryption-at-rest of national ID / tax ID**; controls who can decrypt/see full ID | Only module that reads/writes the plaintext ID; everyone else sees masked value |
| **Slip / file storage** | Store uploaded slips (Flow B) and generated receipt PDFs; type/size validation (FR-02) | Object store (filesystem or S3-compatible); stores keys, not blobs, in DB |
| **Audit log** | Append-only record of every significant action (who/what/when/before→after) | Written by every module; never updated/deleted; read-only UI for Admin |
| **Config / template store** | Templates, watermark, signature image, number format — **editable without deploy** (NFR-09) | Read by Donation core, Receipt number, PDF worker; written only by Admin |
| **e-Donation export** | Generate Excel/CSV for manual keying; flag "exported / keyed" status (FR-30/31) | Reads issued records + donor PII (decrypted for export, audited) |
| **Job enqueue (outbox)** | Insert a job row in the **same transaction** as receipt issuance | Decouples approval from PDF/email; guarantees a job exists iff a receipt was issued |
| **Worker** | Render PDF (template+watermark+signature+language), send email, record delivery status, retry with backoff (FR-25/27) | Consumes job_queue; writes PDF to file store; updates record + audit |

## Recommended Project Structure

Organize by **domain module**, not by technical layer. Each module owns its routes, services, data access, and tests. This keeps the gap-less-numbering and PII boundaries explicit and hard to accidentally bypass.

```
src/
├── modules/
│   ├── auth/                 # login, sessions, RBAC guards, password hashing
│   ├── donation/             # donation_record aggregate + state machine (FSM)
│   │   ├── state-machine.*   # the canonical transition table — single source of truth
│   │   ├── donation.service.*
│   │   └── donation.routes.*
│   ├── approval/             # maker submit, checker approve/reject; the ISSUE transaction
│   ├── receipt-number/       # ★ gap-less per-fiscal-year allocator (DB tx only)
│   │   ├── fiscal-year.*     # derive fiscal year from issue date (Oct 1 boundary)
│   │   └── allocator.*       # UPDATE ... RETURNING on counter row
│   ├── donor/                # PII + encryption-at-rest + ID masking
│   ├── files/                # slip upload + receipt storage (object store adapter)
│   ├── audit/                # append-only writer + admin read views
│   ├── config/               # template/signature/number-format store (no-deploy edits)
│   ├── export/               # e-Donation Excel/CSV + keyed-status flag
│   └── public-form/          # Flow B donor form, consent capture, rate limit/CAPTCHA
├── jobs/
│   ├── queue.*               # transactional outbox + claim/lease semantics
│   └── worker.*              # PDF render + email send + retry
├── pdf/                      # template engine, Thai/EN fonts, watermark, signature compositing
├── email/                    # SMTP/API adapter, 2-language templates
├── db/
│   ├── migrations/
│   └── schema.*
└── shared/                   # i18n, validation, crypto helpers, errors
```

### Structure Rationale

- **modules/receipt-number/ is isolated** so the only path to a number is through the allocator. No `MAX(id)+1` can sneak in elsewhere.
- **modules/donor/ owns all PII** so encryption and ID-masking can't be bypassed by another module reading the column directly.
- **jobs/ + pdf/ + email/ are separate from request handlers** so approval returns fast and email retry lives in a durable worker, satisfying NFR-07 and FR-27.
- **config/ is data, not code**, directly serving NFR-09 (edit templates/number format without deploy).

## Architectural Patterns

### Pattern 1: Explicit state machine for the donation record

**What:** A single canonical transition table governs `draft → pending_review → approved/issued`, plus `rejected` (back to draft/editable) and `cancelled` (terminal, number retained). Every transition is a guarded operation that writes to the audit log.

**When to use:** Always here — FR-11 mandates explicit statuses and FR-14 mandates "receipt only exists after approval."

**Trade-offs:** Slightly more ceremony than ad-hoc status fields, but it makes illegal transitions (e.g., editing an issued receipt, or issuing without approval) impossible and gives audit/PDPA a natural hook.

**State model:**
```
                 submit            approve (★ ISSUE TX)
   draft ───────────────► pending_review ──────────────► issued
     ▲                        │                            │
     │ reject (reason)        │ reject (reason)            │ cancel (reason)
     └────────────────────────┘                            ▼
                                                        cancelled  (number kept, FR-19)
```
Key rules: number is allocated **only** on the approve→issued transition; `issued` is otherwise immutable; `cancelled` keeps its number (no gap); `rejected` returns the record to an editable state (no number ever allocated).

### Pattern 2: Gap-less per-fiscal-year numbering via locked counter row (★ the critical design)

**What:** A `receipt_counter` table keyed by fiscal year. Allocation happens **inside the same database transaction** that flips the record to `issued`, using a single atomic `UPDATE ... RETURNING`, which takes a row-level lock held until commit.

**Why not a DB sequence:** PostgreSQL sequences are explicitly *not* gap-less by design — `nextval` is never rolled back, values are lost on rollback/crash/cache, so they cannot satisfy FR-16/NFR-04. This is confirmed by the PostgreSQL documentation itself. Sequences guarantee *uniqueness*, not *gaplessness*.

**Why this is safe AND cheap here:** The counter-row approach serializes concurrent issuances (only one approval at a time gets the next number for a given year). That serialization is the textbook performance cost of gap-less numbering — but a hospital issues a handful of receipts at a time, so contention is effectively zero. The classic objection (throughput) does not apply to this domain. We get correctness with no practical penalty.

**Schema + allocation:**
```sql
-- one row per fiscal year; NO secondary index (HOT updates, avoid bloat)
CREATE TABLE receipt_counter (
  fiscal_year  INT PRIMARY KEY,   -- e.g. 2569
  last_number  BIGINT NOT NULL DEFAULT 0
);

-- Inside the SAME transaction that sets record -> issued:
BEGIN;
  -- 1) derive fiscal year from the issue date (approval date), Oct 1 boundary
  --    (Oct–Dec  -> next Buddhist fiscal year)
  -- 2) ensure the year row exists (auto-reset to 1 on new year = INSERT ... DEFAULT 0)
  INSERT INTO receipt_counter (fiscal_year)
    VALUES (:fy) ON CONFLICT (fiscal_year) DO NOTHING;
  -- 3) atomic increment + return; row lock held to commit, serializes peers
  UPDATE receipt_counter
     SET last_number = last_number + 1
   WHERE fiscal_year = :fy
  RETURNING last_number;          -- -> the running number
  -- 4) format with config (separator, padding): e.g. 2569/000123
  -- 5) UPDATE donation_record SET status='issued', receipt_no=..., issued_at=...
  -- 6) INSERT into job_queue (transactional outbox)  -- same tx!
  -- 7) INSERT audit_log row                          -- same tx!
COMMIT;
```

**Trade-offs / rules that protect the invariant:**
- Use a **single atomic `UPDATE ... RETURNING`**, not `SELECT ... FOR UPDATE` then a separate `UPDATE` — the latter can hand out duplicates under READ COMMITTED.
- Keep the issue transaction **short**: do **no** PDF/email/network work inside it. Those go to the outbox and run in the worker afterward. (This is *why* the outbox pattern is mandatory, not optional.)
- A `UNIQUE (fiscal_year, receipt_no)` constraint on `donation_record` is a cheap belt-and-suspenders backstop.
- Fiscal year is derived from **issue date (approval date)**, never pre-computed at draft time (FR-18) — a record drafted in September but approved in October belongs to the *next* fiscal year.
- New fiscal year auto-resets to 1 simply because the year's counter row starts at 0 (FR-17).

### Pattern 3: Transactional outbox for PDF + email (decouple the slow path)

**What:** The act of issuing a receipt and the act of *enqueuing the PDF/email job* happen in one transaction (the job row is inserted alongside the status change). A separate worker polls/claims jobs, renders the PDF, sends the email, records delivery status, and retries failures with backoff.

**When to use:** Whenever a committed business fact must reliably trigger a side-effect. Here it guarantees: **a receipt is issued ⇔ a PDF/email job exists** — no lost emails, no emails for un-issued receipts.

**Trade-offs:** Adds a worker process and a job table. Worth it: it satisfies NFR-07 (fast approval), FR-27 (retry + delivery status), and keeps the critical numbering transaction short. For MVP the queue can be a Postgres table with `FOR UPDATE SKIP LOCKED` claiming — no external broker (Redis/RabbitMQ) needed until volume demands it.

```
approve → [TX: status=issued + number + outbox job + audit] → COMMIT (fast return to user)
                                   │
                          worker claims job
                                   ▼
                 render PDF (tmpl+watermark+signature+lang)
                                   ▼
                 store PDF → send email → record status → audit
                                   ▼  (on failure)
                 increment attempts, reschedule with backoff (FR-27)
```

### Pattern 4: PII encryption boundary owned by one module

**What:** National ID / tax ID is encrypted at rest (application-level envelope encryption recommended over relying solely on disk encryption, so role-based decryption and audit are possible per NFR-02/PDPA). Only the `donor` module decrypts; all other modules and most UI surfaces see a masked value (e.g. `x-xxxx-xxxxx-12-3`). Decryption for export is an audited action.

**Trade-offs:** Encrypted columns aren't searchable by exact value without a deterministic scheme or a separate searchable hash — plan a blind-index/HMAC column if "find donor by ID" is required.

## Data Flow

### The core flow: donation → issued, emailed receipt

```
FLOW A (staff)                         FLOW B (public donor)
Maker creates record (draft)           Donor submits form + slip + consent
        │                                       │ (rate-limited, CAPTCHA)
        │                              record created as pending_review
        ▼                                       │ slip stored, consent logged
   Maker submits ──► pending_review ◄───────────┘
                          │
                 Checker reviews (sees slip, donor data, masked ID)
                    ┌─────┴─────┐
                reject(reason)  approve
                    │             │
              back to draft   ┌───▼──────────────────────────────────┐
              (audited)       │ ISSUE TRANSACTION (single DB tx):     │
                              │  derive fiscal year from issue date   │
                              │  allocate gap-less number (UPDATE RET) │
                              │  status=issued, receipt_no set         │
                              │  enqueue PDF/email job (outbox)        │
                              │  write audit log                       │
                              └───┬────────────────────────────────────┘
                                  │ COMMIT (user sees "issued" instantly)
                                  ▼
                         WORKER picks up job
                         render PDF → store → email donor
                         record delivery status (FR-27)
                                  │
                         (later) Admin/staff: e-Donation export → mark keyed (FR-31)
```

### Read/query flow (back-office list & search)

```
Maker/Checker UI → authn+RBAC guard → donation.service.search(filters)
   → PostgreSQL (status, date range, name, receipt_no indexes)
   → results with MASKED national ID (full ID only on explicit, audited reveal)
```

### Key data flows (summary)

1. **Issuance flow:** approval → atomic [number + status + outbox + audit] → worker → PDF + email + delivery status. The transactional boundary is the whole correctness story.
2. **PII flow:** plaintext ID enters only via donor module → encrypted at rest → masked everywhere → decrypted only for receipt PDF and audited export.
3. **Config flow:** Admin edits template/signature/number-format rows → read live by donation/receipt-number/PDF modules → effective without deploy (NFR-09).
4. **Audit flow:** every state transition and sensitive read/export appends one immutable row; never mutated.

## Suggested Build Order (dependency ordering)

This ordering follows the document's Phase-1 MVP (Flow A back-office first) and builds each component only after its dependencies exist. The riskiest piece (numbering) is built and concurrency-tested early, before any UI depends on it.

```
0. Foundation
   └─ DB schema + migrations, Auth/RBAC, audit log writer
        (everything writes audit; RBAC gates everything)

1. Donation core + state machine        ← needs: foundation
   └─ create/edit draft, status model, search/filter
   └─ Donor/PII module with encryption + masking (needed to store donors)

2. Receipt-number module (★)            ← needs: DB + fiscal-year logic
   └─ counter table, fiscal-year derivation, UPDATE...RETURNING allocator
   └─ CONCURRENCY TEST HERE (parallel issue, assert no dup/no gap) before UI build-out

3. Approval/workflow + ISSUE transaction ← needs: state machine + numbering + audit
   └─ maker submit, checker approve/reject, segregation of duties
   └─ wires the single-transaction issue (status+number+outbox+audit)

4. Job queue (outbox) + Worker          ← needs: issue transaction emitting jobs
   ├─ PDF pipeline (template+watermark+signature+Thai/EN)   ← needs: config store
   └─ Email pipeline (2-lang, delivery status, retry)
   └─ Config/template store feeds both PDF and numbering format

5. e-Donation export + keyed-status     ← needs: issued records + donor PII
6. Reports + Admin settings UI          ← needs: config store, RBAC

— Phase 2 —
7. Public donation form (Flow B)        ← needs: donation core (pending_review), files,
   └─ consent capture, slip upload, rate limit/CAPTCHA, donor status email
```

**Critical ordering constraints:**
- **Numbering (2) before approval (3):** the issue transaction cannot be wired until the allocator exists and is proven concurrency-safe.
- **Config store (4) before/with PDF:** number *format* and templates both read config; build config storage alongside the worker so PDF and number formatting are configurable from day one (NFR-09).
- **Outbox (4) requires the issue transaction (3)** to emit the job in-transaction — don't build the worker to be triggered by an HTTP call, or you reintroduce the lost-email/half-commit risk.
- **Audit log (0) first:** it's a dependency of literally every later component; retrofitting audit is painful and risks gaps in the trail (NFR-05).
- **Flow B (7) last:** it reuses the entire Flow-A pipeline (it just creates records in `pending_review` instead of `draft`), so it should not be built until that pipeline is solid.

## Scaling Considerations

| Scale | Architecture adjustments |
|-------|--------------------------|
| Hospital MVP (hundreds–low-thousands of receipts/year) | Single monolith + one worker + Postgres job table is more than enough. Gap-less serialization is invisible at this volume. |
| Higher volume / multiple branches | Counter is already partitioned by fiscal year; could add branch/series to the counter key to reduce (already negligible) contention. Move job queue to Redis/broker only if worker throughput becomes the limit. |
| Large file/PDF volume | Move slip/receipt storage to S3-compatible object store (already abstracted behind files module); serve receipts via signed URLs. |

### Scaling priorities

1. **First bottleneck is almost never numbering** at this domain's volume — it's PDF rendering CPU. Scale by running more worker processes (jobs are independent once issued).
2. **Second:** email provider rate limits — backoff/retry already handles this; batch if needed.

## Anti-Patterns

### Anti-Pattern 1: Pre-computing or reserving the receipt number before approval
**What people do:** Assign a number when the draft is created or "reserve" the next number in the UI.
**Why it's wrong:** A rejected/cancelled draft then burns a number → a gap (violates FR-16/19), and fiscal year would be wrong for records that cross the Oct 1 boundary (violates FR-18).
**Do this instead:** Allocate only inside the approve→issued transaction, fiscal year derived from issue date.

### Anti-Pattern 2: Using a PostgreSQL sequence / `SERIAL` / `MAX(id)+1` for the receipt number
**What people do:** Reach for the database's built-in auto-increment.
**Why it's wrong:** Sequences are gap-prone by design (never rolled back, lost on crash); `MAX+1` produces duplicates under concurrency.
**Do this instead:** Scoped counter row + atomic `UPDATE ... RETURNING` inside the transaction.

### Anti-Pattern 3: Generating PDF and sending email inside the approval HTTP request
**What people do:** Render and email synchronously so the code is "simpler."
**Why it's wrong:** It lengthens the numbering transaction (more lock contention, NFR-07 latency), and a failed email after commit means a lost receipt with no retry (violates FR-27).
**Do this instead:** Transactional outbox + worker with retry.

### Anti-Pattern 4: Deleting cancelled receipts or reusing their numbers
**What people do:** Hard-delete a mistaken receipt to "clean up."
**Why it's wrong:** Breaks the audit/tax trail and creates a gap.
**Do this instead:** `cancelled` status, number retained (FR-19), audit row written.

### Anti-Pattern 5: Storing national ID in plaintext or letting every module read it
**What people do:** Plain column, read everywhere.
**Why it's wrong:** Violates NFR-02/PDPA; uncontrolled exposure of sensitive PII.
**Do this instead:** Encrypt at rest in the donor module, mask everywhere, audit every reveal/export.

## Integration Points

### External Services

| Service | Integration pattern | Notes |
|---------|---------------------|-------|
| SMTP / email API | Adapter behind email module, called only by worker | Record provider message-id + status; retry with backoff; never inline in request |
| Object/file store | Adapter behind files module (filesystem for MVP, S3-compatible later) | Validate type/size on upload (FR-02); store keys in DB, not blobs |
| e-Donation (กรมสรรพากร) | **No API** — manual export only (Excel/CSV) | Confirm exact field layout with accounting (Open Issue #5) before finalizing export schema |

### Internal Boundaries

| Boundary | Communication | Notes |
|----------|---------------|-------|
| Approval ↔ Receipt-number | Direct in-process call **inside one DB tx** | The only caller allowed to allocate a number |
| Issue tx ↔ Worker | Async via outbox table | Job exists iff receipt issued |
| All modules ↔ Audit | Direct append-only write | Never update/delete |
| All modules ↔ Donor PII | Direct call; only donor module decrypts | Masked value crosses the boundary by default |
| Config ↔ (Donation, Receipt-number, PDF) | Direct read of config rows | Enables no-deploy edits (NFR-09) |

## Sources

- PostgreSQL: Sequences vs. Invoice numbers — CYBERTEC — https://www.cybertec-postgresql.com/en/postgresql-sequences-vs-invoice-numbers/ (HIGH)
- Gaps in sequences in PostgreSQL, causes and remedies — CYBERTEC — https://www.cybertec-postgresql.com/en/gaps-in-sequences-postgresql/ (HIGH)
- Gapless number generation (UPDATE ... RETURNING counter table) — Thoughts about SQL — https://blog.sql-workbench.eu/post/gapless-sequence/ (HIGH)
- No-gap sequence in PostgreSQL and YugabyteDB — DEV Community — https://dev.to/yugabyte/no-gap-sequence-in-postgresql-and-yugabytedb-3feo (MEDIUM)
- Postgres gapless counter for invoice purposes — kimmobrunfeldt/howto-everything — https://github.com/kimmobrunfeldt/howto-everything/blob/master/postgres-gapless-counter-for-invoice-purposes.md (MEDIUM)
- Concurrency-safe invoice numbering — AppMaster — https://appmaster.io/blog/concurrency-safe-invoice-numbering (MEDIUM)
- Project requirements: requirements-ระบบออกใบเสร็จบริจาค.md v1.1 + .planning/PROJECT.md (HIGH — authoritative for this domain)

---
*Architecture research for: Thai hospital donation e-receipt system (DonaRec)*
*Researched: 2026-06-22*
