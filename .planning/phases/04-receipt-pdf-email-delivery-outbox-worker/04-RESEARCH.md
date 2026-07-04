# Phase 4: Receipt PDF + Email Delivery (Outbox Worker) - Research

**Researched:** 2026-07-04
**Domain:** Server-side Thai/English tax-compliant PDF rendering via headless Chromium, transactional-outbox worker pattern, email delivery abstraction, admin-editable HTML templates with a Chromium sandboxing security boundary
**Confidence:** HIGH (core Thai-rendering and security-sandbox questions were verified with live spikes in this session, not just literature review)

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

- **D-55 (ภาษาเอกสาร, FR-23):** เพิ่ม column `donor_language` บน donation (`th`/`en`, default `th`), Maker เลือกตอนสร้างรายการ (Flow A). ภาษาถูก freeze เป็นส่วนหนึ่งของ snapshot (หลักเดียวกับ D-43) — PDF + email ทั้งหมดใช้ภาษานี้เสมอ ไม่ใช่ toggle ชั่วคราว. ต้องมี migration ใหม่ + field ใน Phase 3 form + default `th` สำหรับ record เดิม.
- **D-56 (PDF persistence, FR-24, immutability):** Freeze — render ครั้งเดียวตอน worker process job แล้วเก็บไฟล์ PDF ใน MinIO (bucket แยกจาก slip เช่น `donnarec-receipts`, เก็บ reference ใน DB). resend/download ใช้ไฟล์เดิมเสมอ — ไม่ re-render. ห้ามเก็บ PDF เป็น BLOB ใน DB.
- **D-57 (Email retry/resend/download, FR-27/28, NFR-07):** Auto-retry + backoff (planner กำหนดตัวเลข). เกิน max attempts → สถานะ `failed` (dead-letter ในตัว, ไม่ retry อัตโนมัติต่อ). Staff เห็นสถานะ + กด resend เองได้ (re-enqueue, ห้าม allocate เลขใหม่, ใช้ PDF ที่ freeze ไว้). Staff ดาวน์โหลด PDF เองได้เสมอ. บันทึก `email_delivery` record ต่อการส่ง (status/provider message id/attempts/error).
- **D-58 (Config store + Admin UI, FR-33/NFR-09) — ⚠️ SECURITY FLAG:** Full config store (DB) + Admin UI แก้ได้ครบ รวม HTML template editor. **Downstream ต้องแก้ mitigation สำหรับ template-injection/stored-XSS/SSRF surface จาก admin-supplied HTML ผ่าน headless Chromium:** (a) render ด้วย Chromium ที่ปิด JavaScript + network isolation, (b) จำกัด/whitelist placeholder (templating ปลอดภัย ไม่ raw eval), (c) sanitize/scope asset upload (magic-byte เหมือน slip), (d) Admin-only + audit ทุกการแก้.
- **D-59 (1x/2x, FR-24, compliance):** Global config ระดับโรงพยาบาล (ค่าเดียว) — ไม่มี field ต่อ donation ใน MVP.
- **D-60 (Email provider, stakeholder gate):** Build `EmailSender` interface + dev/local implementation เฟสนี้; provider จริง (SES/Postmark) ยังเป็น stakeholder gate เสียบภายหลังโดยไม่แก้ worker. ห้าม self-host SMTP เป็น production path.
- **D-61 (Live preview, admin UX):** Admin template editor ต้องมี live preview + performance สมูท — ห้าม re-render หนักทุก keystroke (debounce/throttle). Fidelity tension ระหว่าง HTML iframe preview (เร็ว/approximate) vs server Chromium render (ตรง/ช้ากว่า) — **แนะนำผสม (c):** live HTML iframe ระหว่างพิมพ์ + ปุ่ม "เรนเดอร์ PDF จริง" สำหรับตรวจ final. Preview ต้องใช้ sample/mock data (ไม่ใช่ PII จริง) + security sandbox เดียวกับ D-58.

### Claude's Discretion

- Worker trigger model — polling loop (partial index `idx_outbox_jobs_pending` มีแล้ว) vs LISTEN/NOTIFY vs asynq/River — DB-backed poll เพียงพอ (ไม่ต้อง Redis). **Research resolves this: DB-backed poll via atomic `UPDATE...WHERE id=(SELECT...FOR UPDATE SKIP LOCKED)` — see Pattern 1.**
- chromedp vs rod — ยืนยันด้วย spike ก่อน lock. **Research resolves this: chromedp v0.14.2, spiked live — see below.**
- จำนวน retry / backoff schedule / max attempts. **Research proposes concrete numbers — see Common Pitfalls / Standard Stack.**
- Schema รายละเอียดของ `email_delivery`, config table(s), receipt-PDF reference, `donor_language`, MinIO bucket/naming.
- โครงสร้าง package Go (`internal/pdf/`, `internal/mailer/`, `internal/worker/`, `internal/settings/`).
- รูปแบบ migration 000008+.

### Deferred Ideas (OUT OF SCOPE)

- การเลือก email provider จริง (SES vs Postmark) + deliverability (SPF/DKIM/DMARC) — stakeholder gate, D-60 interface only this phase.
- 1x/2x ต่อ donation (เลือกต่อรายการ) — global config only (D-59).
- e-Donation export + reports + backup/restore (FR-30/31/32, NFR-08) — Phase 5.
- Flow B public form + acknowledgement email (FR-01..06, FR-05) — Phase 6.
- PKI digital signature — MVP ใช้รูปภาพลายเซ็นเท่านั้น.

</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-------------------|
| FR-20 | สร้างใบเสร็จ PDF จากเทมเพลตที่มีตรา/หัวจดหมายของโรงพยาบาล | Pattern 2/3 (sandboxed Chromium render + html/template); letterhead/seal embedded as base64 data-URI images inside the self-contained HTML the worker assembles |
| FR-21 | ฝังลายน้ำ (watermark) ของโรงพยาบาลบนเอกสาร | Same render pipeline — watermark = CSS `position:absolute`/`opacity` layer or `<img>` data-URI, verified renders correctly alongside Thai text |
| FR-22 | แสดงลายเซ็นผู้มีอำนาจบนใบเสร็จ (รูปภาพ) | Same pipeline — signature = `<img src="data:...">`, config-store asset |
| FR-24 | ข้อมูลครบตามข้อกำหนดลดหย่อนภาษี (§6, 1x/2x) | Config store (Tab 3 in UI-SPEC) supplies §6 text + 1x/2x election as template data; D-59 = single global value |
| FR-23 | ภาษาไทย/อังกฤษตามผู้บริจาค | D-55 `donor_language` frozen at create-time; go-i18n bundle (`internal/i18n`, already exists) selects catalog; **same HTML template**, i18n swaps text nodes |
| FR-25 | ส่งใบเสร็จ PDF แนบอีเมลหลังอนุมัติ | `EmailSender` interface (D-60) + `email_delivery` schema — see Standard Stack / Code Examples |
| FR-26 | เทมเพลตอีเมล 2 ภาษา | Same go-i18n bundle drives email subject/body templates |
| FR-27 | บันทึกสถานะส่ง + ส่งซ้ำได้ | `email_delivery` table + worker retry/backoff (Pattern 1) + resend endpoint (re-enqueues, never re-renders per D-56) |
| FR-28 | Staff ดาวน์โหลด PDF เอง | MinIO presigned GET reusing `internal/storage` pattern already proven in Phase 3 (`PresignedGet`) |
| NFR-07 | PDF+email ~2–3s/receipt, ไม่บล็อก issuance | Already structurally satisfied by Phase 3's outbox enqueue (render happens in worker, off the lock path) — Phase 4 must additionally *measure* worker-side render+send latency; see Validation Architecture |
| FR-33 | Admin ตั้งค่าเทมเพลต/ลายน้ำ/ลายเซ็น/เลขที่ | `internal/settings` config store (DB-backed, extends/parallels Phase 2's `receipt_number_config` pattern) + Admin UI (already specified in 04-UI-SPEC.md) |
| NFR-09 | แยก config จากโค้ด แก้ได้ไม่ต้อง deploy | Same config store — no code/deploy required to change template/images/text/number format |

</phase_requirements>

---

## Summary

This phase's hardest technical question — **can headless Chromium render legally-correct Thai text, and can that same rendering pipeline be locked down enough to safely execute admin-authored HTML** — was answered with **live, reproducible spikes run in this research session**, not literature alone. Both chromedp and the official `chromedp/headless-shell` Docker image were pulled, run, and driven with real Go code in this environment. Findings:

1. **Headless Chromium renders Thai correctly**, including the worst-case stacked-tone-mark cases (ก๊วยเตี๋ยว, ปั๊ม, ตั้งชื่อ) — verified visually via two independent paths (system `google-chrome --headless --print-to-pdf` and chromedp's `Page.PrintToPDF`).
2. **The official `chromedp/headless-shell` Docker image ships with zero Thai font support** — confirmed by reproducing the tofu-box failure CLAUDE.md warns about. Installing `apt-get install fonts-thai-tlwg` (confirmed present in the image's Debian trixie base) fixes it. TH Sarabun New itself is a separate asset the team must source and embed (not in any apt package).
3. **The two D-58 security mitigations both work at the CDP level**: `Emulation.SetScriptExecutionDisabled(true)` stopped an inline `<script>` from executing, and `Fetch.Enable` (catch-all pattern) + `FailRequest` on every paused request blocked 100% of outbound network — verified against a live SSRF probe (`<img src="https://example.invalid/...">` never loaded).
4. **chromedp v0.15.1 (latest) requires Go 1.26**, which is *not* satisfied by this project's current `go 1.25.1` toolchain — installing latest chromedp would force an unplanned Go version bump. **chromedp v0.14.2 is the correct pin** (requires only Go 1.24, compatible with the existing go.mod, verified by building against it).
5. **Golden-file testing does not need a pixel-diff library.** Rendering the same fixture HTML twice through the pinned container produced byte-identical rasterized PNGs (verified via md5sum). Combined with pinning the Chromium image digest and font assets, **exact PNG-byte comparison is sufficient** for CI — no tolerance-based diff dependency needed (and the one candidate Go library, `orisano/pixelmatch`, is a 16-star, 3-year-dormant single-maintainer repo not worth adopting for this).
6. The transactional-outbox worker should use the **atomic `UPDATE ... WHERE id = (SELECT ... FOR UPDATE SKIP LOCKED)` claim pattern** (single round-trip, no separate-select race), and the app should feed HTML into Chromium via `Page.setDocumentContent` (verified) rather than having Chromium navigate to any URL — this means the render container needs **zero network reachability, including to the app itself**, closing the SSRF surface at the network layer too (defense in depth on top of the CDP-level Fetch block).

**Primary recommendation:** Pin `github.com/chromedp/chromedp v0.14.2`; run Chromium as a **separate `chrome` sidecar service** in docker-compose (custom image `FROM chromedp/headless-shell:stable` + `apt-get install fonts-thai-tlwg` + bundle TH Sarabun New via `@font-face` data-URI), reached only via `chromedp.NewRemoteAllocator` over the internal Docker network (no other route out) — this avoids installing a full Chromium into the existing app's minimal Debian-slim static-binary image. Feed the worker-assembled, fully self-contained HTML (all images/fonts inlined as base64 data URIs) into Chromium via `Page.setDocumentContent`, with `Emulation.SetScriptExecutionDisabled(true)` and `Fetch.Enable`+`FailRequest`-all as CDP-level defense-in-depth. Use Go's stdlib `html/template` (not `text/template`) to execute the admin's stored template HTML against donation/settings data — its contextual autoescaping neutralizes donor-supplied field injection even though the surrounding template structure is admin-authored.

---

## Project Constraints (from CLAUDE.md)

These are locked project-wide directives; this research and the eventual plan must comply, not merely consider them:

- **Go 1.23+** — project's `go.mod` currently pins `go 1.25.1`. Any new dependency must not force a toolchain bump without an explicit decision (see chromedp version finding above).
- **HTTP router: gin** (already the code reality — `github.com/gin-gonic/gin v1.12.0` — CLAUDE.md's "chi recommended" language is superseded by what Phases 1–3 actually built; new routes must use gin `RouterGroup` patterns matching `cmd/server/main.go`).
- **Data layer: sqlc + pgx/v5**, raw `SELECT ... FOR UPDATE` where lock control matters. No ORM. golang-migrate for schema (`migrations/NNNNNN_name.up/down.sql`, next available number is `000008`).
- **Headless Chromium is mandatory for Thai PDF rendering** — pure-Go PDF libraries (gofpdf, Maroto, pdf-lib-style) are explicitly forbidden for Thai receipts (documented Thai vowel/tone-mark shaping bugs).
- **TH Sarabun New + `fonts-thai-tlwg`** — both required in the rendering environment (verified in this session that `fonts-thai-tlwg` alone does NOT provide TH Sarabun New; it provides other Thai-shaping-capable fonts like Waree/Purisa/Garuda as a safety net, while the exact TH Sarabun New file must be separately sourced and bundled).
- **App-level AES-256-GCM envelope encryption** for PII — already implemented in `internal/crypto`; no new PII fields introduced this phase (donor_language, email_delivery status/error, template HTML are not PII).
- **No self-hosted SMTP as a production path** — `EmailSender` interface + dev-local capture only this phase (D-60); real provider (SES/Postmark) deferred.
- **Never store PDFs as BLOBs in the database** — MinIO/S3-compatible object storage only, DB stores a reference (D-56 already mandates this).
- **Never render PDF or send email inside the issuance transaction** — already correctly implemented in Phase 3 (`internal/donation/service.go` enqueues the outbox job and returns; Phase 4 only *consumes*).
- **Magic-byte validation for all uploads** — reuse `internal/storage`'s `gabriel-vasile/mimetype`-based `validateSlip` pattern for template image uploads (letterhead/seal/signature/watermark).
- **Integration-test gate (Conventions)** — every new HTTP route this phase introduces (settings save, resend, download, preview) that touches routing/RBAC/DB must ship with a real end-to-end test: `HTTP request → RequireAuth (real Keycloak-shaped token) → RequireRoles → handler → service → DB`, not just unit/service-level tests. Phase 3 shipped three seam bugs that only unit tests missed; this gate exists specifically because of that.

---

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Outbox job polling & dispatch | API/Backend (Go worker goroutine) | Database | Worker is part of the same Go binary/process group as the API (or a dedicated worker binary sharing the module) — DB is system of record for job state |
| PDF rendering (HTML→PDF) | API/Backend (Go, via CDP to a sidecar Chromium) | — | Server-side only; the "PDF render" capability is really its own micro-tier (a controlled browser engine) but it is orchestrated entirely from the backend, never exposed directly to the client |
| Template/config storage | Database | API/Backend (CRUD + validation) | Config store is the source of truth (NFR-09 no-deploy); Go app is the only writer via the Admin API |
| Admin settings UI (editor + tabs) | Frontend Server/Client (Next.js) | API/Backend (settings endpoints) | Editing UX, debounce, and the sandboxed iframe preview all live in the browser/Next.js tier; persistence goes through the Go API |
| Live HTML preview (fast path) | Browser/Client (sandboxed iframe) | — | No JS/network by design (`sandbox="allow-same-origin"` only); renders the *same* self-contained HTML string the server would feed Chromium, for maximum fidelity without a server round-trip on every keystroke |
| Live "real PDF" preview (accurate path) | API/Backend (same render pipeline as production) | Frontend (trigger + display) | Explicit user action (button), not continuous — acceptable to pay the Chromium render cost here |
| Email composition & sending | API/Backend (`EmailSender` interface) | Database (`email_delivery` audit trail) | Business logic (which language, which attachment, retry policy) is backend-owned; the interface boundary is what makes the provider swappable later |
| PDF/file persistence | Database/Storage (MinIO) | API/Backend (client wrapper) | Reuses `internal/storage` pattern verbatim — object storage owns bytes, Postgres owns metadata/reference |
| Resend / download actions | API/Backend (new endpoints on `donationGroup`/`checkerGroup`) | Database/Storage | Role-gated (Checker/Admin for resend per UI-SPEC; any of Maker/Checker/Admin can view per existing `donationGroup` middleware) |
| `donor_language` capture | Frontend (Flow A create/edit form) | API/Backend (persist + freeze) | Captured at donation-create time in the existing Phase 3 form; frozen into the donation row exactly like other snapshot fields (D-43 precedent) |

---

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|---------------|
| `github.com/chromedp/chromedp` | **v0.14.2** (pin — NOT latest) | Drive headless Chromium via CDP: navigate, inject HTML, print-to-PDF, disable JS, block network | [VERIFIED: go proxy + live spike in this session] Largest community (13,152 GitHub stars vs. go-rod's 6,985), official `chromedp/headless-shell` companion Docker image from the same maintainers, and its low-level 1:1 mapping to CDP domains (`emulation.SetScriptExecutionDisabled`, `fetch.Enable`/`FailRequest`) is exactly the control surface the D-58 security mitigations need. **v0.15.1 (latest) requires Go 1.26 and would silently bump this project's `go 1.25.1` toolchain requirement if `go get`'d without pinning — confirmed by attempting the upgrade in this session.** v0.14.2 requires only Go 1.24, compatible with the existing go.mod as-is. |
| `chromedp/headless-shell` (Docker image, tag `stable`) | digest-pin at build time (verified digest this session: `sha256:313ed7255ae1e155fb157631a6d4c0eb8b65bbe06de9e704ed834399bdf678ff` as of 2026-07-04 — re-pull and re-record before locking CI) | The actual Chromium binary, run as a **separate container/service**, not inside the app's own image | [VERIFIED: pulled and ran live] Official, minimal, multi-arch, maintained by the chromedp org; avoids installing a full Chromium into the app's existing `debian:bookworm-slim` static-binary runtime image (`Dockerfile`), which was built deliberately small (`CGO_ENABLED=0`, `wget`-only). Confirmed empty of Thai fonts by default — must extend with `fonts-thai-tlwg` (see Common Pitfalls). |
| `html/template` (Go stdlib) | Go 1.25.1 (bundled) | Execute admin-authored template HTML against donation/settings data with contextual autoescaping | [CITED: pkg.go.dev/html/template] Contextually escapes any `{{.Field}}` pipeline value based on where it appears in the HTML (text node, attribute, URL, JS block) — this is the mechanism that keeps *donor-supplied* data (name/address, which is untrusted end-user input) from breaking out of the admin's HTML structure, even though the admin's raw template markup is trusted-but-risky by a different axis (XSS/SSRF via the render engine, mitigated separately by the Chromium sandbox). Do **not** wrap the whole admin string in `template.HTML(...)` — that disables autoescaping entirely and defeats the point. |

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `golang-migrate/migrate` | v4.19.1 (existing) | Migrations `000008`+ | `donor_language` column, `email_delivery` table, receipt-settings config table(s), receipt-PDF reference column |
| `sqlc` + `jackc/pgx/v5` | existing (pgx v5.10.0) | Type-safe queries, incl. the new worker poll/claim query | Same pipeline as Phases 1–3 |
| `nicksnyder/go-i18n/v2` | v2.6.1 (existing) | PDF + email text per `donor_language` | Reuse `internal/i18n.SetupBundle` bundle as-is; add message IDs for receipt/email strings |
| `minio-go/v7` | v7.2.1 (existing) | Store frozen PDF, template images | Reuse `internal/storage` client pattern; new bucket (e.g. `donnarec-receipts`) separate from slips |
| `gabriel-vasile/mimetype` | v1.4.13 (existing) | Magic-byte validation for template image uploads | Reuse `validateSlip`-equivalent pattern for letterhead/seal/signature/watermark uploads |
| `poppler-utils` (`pdftoppm`) | system package, not a Go module | Rasterize rendered PDFs to PNG for golden-file comparison in CI | Add to the CI image (or a test-only Docker layer) — **verified**: PNG output is byte-identical across repeated renders of the same fixture on the pinned container, so no fuzzy-diff library is required |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `chromedp` | `go-rod/rod` v0.116.2 | [VERIFIED: go proxy] Requires only Go 1.21 (no toolchain-bump risk at all), higher-level ergonomic API (built-in retry/wait helpers), actively pushed as recently as 2026-05. Viable alternative if the team prefers its API; **not recommended primary** because chromedp's lower-level 1:1 CDP domain mapping is a better fit for the security-critical JS-disable/network-block code path, and its official `headless-shell` container pairing is a smoother docker-compose story. |
| Separate `chrome` sidecar container | Install Chromium directly into the app's own `Dockerfile` | Rejected: the app's runtime image is deliberately `debian:bookworm-slim` + a static Go binary (small, fast to build/deploy); bundling Chromium (300+ MB with dependencies) into it bloats every deploy and couples two very different failure/restart domains (a browser crash vs. an API crash). A sidecar keeps them independently restartable and matches the official image's intended usage. |
| DB-backed polling loop | `asynq` or `river` (Redis-backed) | Explicitly resolved by CONTEXT.md discretion note: hospital volume is low; a plain polling loop against the existing partial index is sufficient. Revisit only if job volume or need-for-scheduling grows significantly. |
| Exact-byte PNG comparison for golden-file tests | `orisano/pixelmatch` (tolerance-based perceptual diff) | [Manual audit this session — see Package Legitimacy Audit] 16 GitHub stars, single maintainer, no commits since 2023. Not worth the dependency given the render pipeline was verified deterministic at the PNG level. Keep as a documented fallback only if real-world CI ever shows non-determinism (e.g., anti-aliasing jitter across different host CPUs) — add `checkpoint:human-verify` before adopting it. |
| `aws-sdk-go-v2` (SES) as a go.mod dependency now | Defer to when SES/Postmark is actually chosen (D-60 stakeholder gate) | Don't add the AWS SDK as a dependency this phase — it's a heavy, unused import until the provider decision lands. Build only the `EmailSender` interface + a dependency-free dev/local capture implementation (write `.eml`-shaped output to a local file or MinIO "dev-outbox" prefix + log). |

**Installation:**
```bash
go get github.com/chromedp/chromedp@v0.14.2
```
No other new Go module dependencies are required for the MVP scope of this phase (email sending uses only the interface + a stdlib-only dev capture implementation; PDF rasterization for CI golden-file tests uses the system `pdftoppm` binary, not a Go library).

**Version verification (this session):**
```
$ curl -s https://proxy.golang.org/github.com/chromedp/chromedp/@latest
{"Version":"v0.15.1", ...}          # requires go >= 1.26 (confirmed by `go get` upgrading go.mod)
$ curl -s https://proxy.golang.org/github.com/chromedp/chromedp/@v/v0.14.2.mod
module github.com/chromedp/chromedp
go 1.24                              # compatible with project's go 1.25.1
$ curl -s https://proxy.golang.org/github.com/go-rod/rod/@latest
{"Version":"v0.116.2", ...}          # requires go 1.21 — no conflict either way
$ docker pull chromedp/headless-shell:stable
Digest: sha256:313ed7255ae1e155fb157631a6d4c0eb8b65bbe06de9e704ed834399bdf678ff
```

---

## Package Legitimacy Audit

> The `package-legitimacy check` seam only supports `npm|pypi|crates` ecosystems; this phase's new dependency is a Go module, so this table was compiled manually via `proxy.golang.org` (registry existence) + GitHub API (adoption/activity signals) in this session.

| Package | Registry | Age | Stars / Activity | Source Repo | Verdict | Disposition |
|---------|----------|-----|-------------------|--------------|---------|-------------|
| `github.com/chromedp/chromedp` | Go module proxy | est. 2017, still active | 13,152 stars; pushed 2026-03-23; 177 open issues (healthy, maintained) | github.com/chromedp/chromedp | OK | Approved — pin v0.14.2 |
| `chromedp/headless-shell` (Docker image) | Docker Hub | official chromedp org image | Actively rebuilt/pushed | github.com/chromedp/docker-headless-shell (648 stars) | OK | Approved as base image for the new `chrome` sidecar service |
| `github.com/go-rod/rod` (alternative, not selected) | Go module proxy | active | 6,985 stars; pushed 2026-05-24 | github.com/go-rod/rod | OK | Not adopted this phase — documented as viable alternative |
| `github.com/orisano/pixelmatch` (considered for golden-file diff, not selected) | Go module proxy | v0.0.0-20230914... (last tag 2023) | **16 stars**, 0 open issues, no push since 2023-09-14 | github.com/orisano/pixelmatch | SUS (low adoption, dormant single-maintainer repo) | **REMOVED from recommendation** — use exact PNG-byte comparison instead (verified deterministic this session). If the planner later needs perceptual tolerance, add a `checkpoint:human-verify` task before introducing this dependency. |
| `github.com/aws/aws-sdk-go-v2` (+ `service/ses`, `config`) | Go module proxy | official AWS org | Massive adoption, official | github.com/aws/aws-sdk-go-v2 | OK (verified to exist and current) | **Not added this phase** — D-60 defers the real email provider; do not introduce this dependency until SES is actually chosen (stakeholder gate). |

**Packages removed due to [SLOP] verdict:** none.
**Packages flagged as suspicious [SUS]:** `orisano/pixelmatch` — not recommended for adoption; if the planner overrides this recommendation, gate the install behind `checkpoint:human-verify`.

---

## Architecture Patterns

### System Architecture Diagram

```
                    ┌─────────────────────────────────────────────────────┐
                    │  Postgres: outbox_jobs (pending|processing|done|failed) │
                    └───────────────────────┬───────────────────────────────┘
                                             │ atomic claim
                                             │ UPDATE...WHERE id=(SELECT...FOR UPDATE SKIP LOCKED)
                                             ▼
   ┌──────────────────────────────────────────────────────────────────────────┐
   │  Worker goroutine (internal/worker) — polls every N seconds, own ctx      │
   │  tied to main's signal.NotifyContext for graceful shutdown                │
   └───────────────┬─────────────────────────────────────────────────────────┘
                    │ 1. load donation snapshot + donor_language (frozen, D-55/D-43)
                    │ 2. load settings (internal/settings): template HTML, §6 text,
                    │    1x/2x, images (letterhead/seal/signature/watermark), TH Sarabun
                    ▼
   ┌──────────────────────────────────────────────────────────────────────────┐
   │  internal/pdf — html/template.Execute(templateHTML, data)                 │
   │  → assembles ONE self-contained HTML string:                              │
   │    - donation fields substituted with contextual autoescaping             │
   │    - images embedded as base64 data: URIs (fetched from MinIO by the Go   │
   │      app, which HAS network/MinIO access — Chromium never fetches them)   │
   │    - TH Sarabun New embedded via @font-face + base64 data: URI            │
   └───────────────┬─────────────────────────────────────────────────────────┘
                    │ CDP over ws:// (internal Docker network only)
                    ▼
   ┌──────────────────────────────────────────────────────────────────────────┐
   │  chrome sidecar container (chromedp/headless-shell + fonts-thai-tlwg)     │
   │  — no outbound network route at all (isolated compose network)           │
   │  — Emulation.SetScriptExecutionDisabled(true)                            │
   │  — Fetch.Enable(catch-all) + FailRequest on every paused request          │
   │  — Page.setDocumentContent(html)  [never Page.Navigate to a URL]          │
   │  — Page.PrintToPDF() → PDF bytes returned over the same CDP connection    │
   └───────────────┬─────────────────────────────────────────────────────────┘
                    │ PDF bytes
                    ▼
   ┌──────────────────────────────────────────────────────────────────────────┐
   │  MinIO — PutObject to donnarec-receipts bucket (frozen, D-56)             │
   │  Postgres — INSERT receipt PDF reference (object key)                     │
   └───────────────┬─────────────────────────────────────────────────────────┘
                    │
                    ▼
   ┌──────────────────────────────────────────────────────────────────────────┐
   │  internal/mailer.EmailSender.Send(ctx, to, subject, body, attachment)     │
   │  — dev/local impl this phase (D-60); SES/Postmark adapter plugs in later  │
   └───────────────┬─────────────────────────────────────────────────────────┘
                    │
                    ▼
   ┌──────────────────────────────────────────────────────────────────────────┐
   │  Postgres — INSERT email_delivery row (status/provider_msg_id/attempts/  │
   │  error); UPDATE outbox_jobs status=done, or failed+backoff on error       │
   └──────────────────────────────────────────────────────────────────────────┘

   Admin Settings UI (Next.js) — separate flow, NOT in the above hot path:
   Browser (sandboxed iframe, no JS/network) ⇄ debounced (400ms) POST /settings/preview
        (server executes html/template with SAMPLE data, returns raw HTML — no Chromium)
   Browser "Render Real PDF" button → POST /settings/preview/pdf
        (server runs the SAME pipeline as above, with sample data, through the real
        chrome sidecar) → PDF bytes → displayed inline
```

### Recommended Project Structure

```
donnarec-api/
├── internal/
│   ├── pdf/                    # html/template execution + data-URI asset assembly
│   │   ├── render.go           #   assembles self-contained HTML from template+data+images
│   │   ├── chromium.go         #   chromedp wiring: RemoteAllocator, JS-disable, network-block, PrintToPDF
│   │   └── render_test.go
│   ├── mailer/                 # EmailSender interface + implementations
│   │   ├── sender.go           #   interface: Send(ctx, Message) (ProviderResult, error)
│   │   ├── dev_sender.go       #   dev/local capture (writes to disk/MinIO, no real send)
│   │   └── (ses_sender.go)     #   added later when D-60 stakeholder gate resolves
│   ├── worker/                 # outbox poller
│   │   ├── worker.go           #   Run(ctx) poll loop, graceful shutdown
│   │   ├── issue_receipt.go    #   job_type="issue_receipt" handler: render→store→email
│   │   └── worker_test.go      #   integration test against testcontainers Postgres + chrome
│   ├── settings/                # config store (template HTML, images, tax text, number format)
│   │   ├── service.go
│   │   ├── model.go
│   │   └── handler.go          #   Admin-only routes under existing adminGroup
│   ├── email/                  # (alternative name if preferred over "mailer" — pick one, be consistent)
│   ├── donation/               # EXISTING — add donor_language to model.go + queries
│   ├── storage/                # EXISTING — reused as-is for receipts bucket
│   └── i18n/                   # EXISTING — reused, add new message IDs
├── migrations/
│   ├── 000008_donor_language.up/down.sql
│   ├── 000009_email_delivery.up/down.sql
│   ├── 000010_receipt_settings.up/down.sql       # or ALTER receipt_number_config, see Open Questions
│   └── 000011_receipt_pdf_reference.up/down.sql  # or a column on donations, see Open Questions
├── docker-compose.yml            # ADD: chrome sidecar service
└── docker/chrome.Dockerfile       # NEW: FROM chromedp/headless-shell:stable + fonts-thai-tlwg + TH Sarabun
```

### Pattern 1: Atomic Job Claim (`FOR UPDATE SKIP LOCKED` via single `UPDATE`)

**What:** Claim exactly one pending/failed job per poll tick, race-free, without a separate SELECT-then-UPDATE round trip.
**When to use:** Every worker poll iteration.
**Example:**
```sql
-- name: ClaimNextOutboxJob :one
-- Verified pattern (this session, via web research + cross-checked against
-- existing outbox_jobs schema from migration 000007).
-- Requires a new `next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now()` column
-- (migration 000008 or alongside it) so backoff can be expressed without a
-- separate scheduler.
UPDATE outbox_jobs
SET status = 'processing',
    updated_at = now()
WHERE id = (
    SELECT id FROM outbox_jobs
    WHERE status IN ('pending', 'failed')
      AND next_attempt_at <= now()
      AND attempts < @max_attempts
    ORDER BY created_at
    FOR UPDATE SKIP LOCKED
    LIMIT 1
)
RETURNING id, job_type, payload, attempts;
```
```go
// internal/worker/worker.go
func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.pollInterval) // e.g. 5s
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			w.logger.Info("worker: shutting down")
			return
		case <-ticker.C:
			w.processOnce(ctx)
		}
	}
}
```

### Pattern 2: Sandboxed Chromium Render (verified live this session)

**What:** Render admin-controlled HTML to PDF with JavaScript disabled and all outbound network requests failed — directly implements the D-58 security mitigations.
**When to use:** Every PDF render (production issue_receipt jobs AND the "real PDF" admin preview).
**Example (this exact code was built, compiled, and run successfully against both a local `google-chrome` binary and a remote `chromedp/headless-shell` container in this research session):**
```go
// internal/pdf/chromium.go
// Source: verified live spike, this session (chromedp v0.14.2)
import (
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

func RenderPDF(ctx context.Context, wsURL, selfContainedHTML string) ([]byte, error) {
	allocCtx, cancel := chromedp.NewRemoteAllocator(ctx, wsURL) // e.g. "ws://chrome:9222/devtools/browser/<id>"
	defer cancel()
	cctx, cancel2 := chromedp.NewContext(allocCtx)
	defer cancel2()
	cctx, cancelTO := context.WithTimeout(cctx, 30*time.Second)
	defer cancelTO()

	var pdfBuf []byte
	err := chromedp.Run(cctx,
		// Block ALL outbound network — every request is paused then failed.
		fetch.Enable().WithPatterns([]*fetch.RequestPattern{{URLPattern: "*"}}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			chromedp.ListenTarget(ctx, func(ev interface{}) {
				if ev, ok := ev.(*fetch.EventRequestPaused); ok {
					go func() { _ = chromedp.Run(ctx, fetch.FailRequest(ev.RequestID, "Failed")) }()
				}
			})
			return nil
		}),
		// Disable JavaScript execution entirely.
		emulation.SetScriptExecutionDisabled(true),
		chromedp.Navigate("about:blank"),
		chromedp.ActionFunc(func(ctx context.Context) error {
			ft, err := page.GetFrameTree().Do(ctx)
			if err != nil {
				return err
			}
			// Inject HTML directly — Chromium never performs a navigation/fetch
			// to obtain the template, closing off an entire class of SSRF.
			return page.SetDocumentContent(ft.Frame.ID, selfContainedHTML).Do(ctx)
		}),
		chromedp.Sleep(300*time.Millisecond), // layout settle; no JS/webfonts to await
		chromedp.ActionFunc(func(ctx context.Context) error {
			buf, _, err := page.PrintToPDF().WithPrintBackground(true).Do(ctx)
			pdfBuf = buf
			return err
		}),
	)
	return pdfBuf, err
}
```
**Verified results this session:**
- Inline `<script>document.title = "XSS-EXECUTED-IF-YOU-SEE-THIS"</script>` did NOT execute (title stayed empty).
- `<img src="https://example.invalid/exfil-tracker.png?...">` was intercepted by the `Fetch` handler and failed — the image never loaded, no external DNS/HTTP occurred.
- Thai stacked-tone-mark text rendered correctly through this exact code path (not just via the raw `google-chrome` CLI).

### Pattern 3: Safe Admin-Template Execution via `html/template`

**What:** Execute the admin's raw HTML template string against donation data, letting Go's contextual autoescaping handle donor-supplied field safety.
**When to use:** Building the self-contained HTML before handing it to Chromium.
**Example:**
```go
// internal/pdf/render.go
// Source: pkg.go.dev/html/template — contextual autoescaping is automatic;
// do NOT pre-wrap donor fields in template.HTML(...).
tmpl, err := template.New("receipt").Parse(adminTemplateHTML) // admin's stored HTML
if err != nil {
	return nil, fmt.Errorf("invalid template: %w", err) // surfaces as "Template save failed" per UI-SPEC
}
var buf bytes.Buffer
data := ReceiptData{
	DonorName:      donation.DonorName,      // untrusted end-user input — autoescaped
	ReceiptNo:      donation.ReceiptFormatted,
	Amount:         donation.Amount,
	LetterheadData: letterheadDataURI,        // pre-built by internal/pdf, safe: server-controlled base64
	SignatureData:  signatureDataURI,
	WatermarkData:  watermarkDataURI,
	FontFaceCSS:    thSarabunFontFaceCSS,      // injected once, server-controlled
}
if err := tmpl.Execute(&buf, data); err != nil {
	return nil, err
}
selfContainedHTML := buf.String()
```

### Pattern 4: Chromium as a Docker-Compose Sidecar (verified this session)

```yaml
# docker-compose.yml addition
services:
  chrome:
    build:
      context: .
      dockerfile: docker/chrome.Dockerfile
    # No published ports — reached only via the internal compose network by `api`.
    networks:
      - default   # confirm this network has no route to the internet in prod (defense-in-depth)
    restart: unless-stopped
```
```dockerfile
# docker/chrome.Dockerfile
# Source: verified this session — apt-cache search confirms fonts-thai-tlwg
# is present in the Debian trixie base of chromedp/headless-shell:stable.
FROM chromedp/headless-shell:stable
RUN apt-get update && apt-get install -y --no-install-recommends \
      fonts-thai-tlwg fontconfig \
    && rm -rf /var/lib/apt/lists/* \
    && fc-cache -f
# TH Sarabun New itself is NOT in fonts-thai-tlwg — source it separately and
# either COPY it into the image here, or (recommended) embed it as a base64
# @font-face data URI at render time from internal/pdf so both the container
# AND the browser-side iframe preview reference the identical font bytes.
```

### Anti-Patterns to Avoid

- **Installing Chromium into the app's own runtime image:** bloats the deliberately-small static-binary image, couples two very different crash/restart domains. Use a sidecar (Pattern 4).
- **Wrapping the entire admin template string in `template.HTML(...)`:** disables Go's contextual autoescaping, defeating the one layer that protects against donor-data injection.
- **Having Chromium navigate to any URL (even an internal one) to fetch the template:** requires network reachability from the render container, which is exactly the SSRF surface D-58 flags. Use `Page.setDocumentContent` (verified) instead — assemble the complete HTML server-side (with base64 data-URI images) and inject it directly.
- **Relying on network-namespace isolation alone for the D-58 mitigation:** it's necessary but treat it as one of three layers (network isolation + CDP `Fetch` block + `Emulation.SetScriptExecutionDisabled`), not sufficient by itself — a misconfigured compose network is a single point of failure otherwise.
- **Adding a pixel-diff tolerance library before proving you need one:** this session's determinism test showed exact-match PNG comparison works; don't add `orisano/pixelmatch` (or similar) speculatively.
- **Storing next-retry state only in memory / recomputing backoff purely from `attempts` without a `next_attempt_at` column:** the poll query needs a concrete timestamp to filter on so failed jobs aren't immediately re-claimed on the next 5-second tick — see Pattern 1's schema note.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| HTML→PDF for Thai text | A custom PDF layout engine, or a pure-Go PDF library with manual glyph positioning | Headless Chromium (chromedp) | Thai combining-mark shaping (tone marks over vowels) requires a real text-shaping engine (HarfHuzz-equivalent inside Chromium); this is precisely the class of bug CLAUDE.md's Sources section documents for pdf-lib and similar libraries |
| Job queue / retry semantics | A bespoke in-memory scheduler or cron-based retry | `SELECT ... FOR UPDATE SKIP LOCKED` against the existing `outbox_jobs` table | Already the documented, well-understood Postgres pattern (used by Que, Oban, and multiple production Go blueprints found this session); avoids reinventing locking semantics that are easy to get subtly wrong |
| Golden-file PDF visual regression | A custom rasterizer or screenshot-diff tool | `pdftoppm` (poppler-utils) → PNG → exact-byte comparison | Verified deterministic this session; no need for anything more sophisticated given the pinned-Chromium + pinned-fonts setup |
| Email attachment sending | Raw SMTP client code hand-rolled against a provider's raw protocol | `EmailSender` interface, dev-local capture now, official `aws-sdk-go-v2/service/ses` (or Postmark's Go client) later | CLAUDE.md explicitly forbids self-hosted SMTP as a production path; provider SDKs handle MIME/attachment encoding correctly and are what CLAUDE.md's Sources section cites as the standard |
| Safe HTML template execution | A custom sanitizer/regex-based placeholder substitution | Go stdlib `html/template` | Contextual autoescaping is a solved, extensively-audited stdlib feature; regex-based substitution reliably reintroduces the exact injection classes it's meant to prevent |

**Key insight:** every "don't hand-roll" item in this phase already has a battle-tested, either-stdlib-or-official-vendor solution. The temptation to hand-roll shows up specifically around the *security* boundary (template execution, network isolation) — that is exactly where a subtle DIY bug becomes an SSRF or stored-XSS vulnerability in a system that already handles PII, so it's the last place to improvise.

---

## Common Pitfalls

### Pitfall 1: Chromium's own font fallback masks a missing-font bug until it's a customer-visible tofu box
**What goes wrong:** The `chromedp/headless-shell` base image has literally zero Thai-capable fonts installed. Chromium will not error — it silently substitutes tofu (□□□) boxes for every Thai glyph.
**Why it happens:** Docker images minimize footprint; font packages are large and not included by default.
**How to avoid:** Explicitly `apt-get install fonts-thai-tlwg fontconfig && fc-cache -f` in a custom image layered on `chromedp/headless-shell:stable` (verified working this session), AND separately bundle the actual TH Sarabun New file (not in any apt package) via `@font-face` + base64 data URI so the exact production font is what's used, not a TLWG fallback.
**Warning signs:** Any CI/local test that only checks "PDF generation succeeded" (no error) without a golden-file visual check will pass even with 100% tofu output — this is why the golden-file requirement (SC#2) is load-bearing, not optional polish.

### Pitfall 2: Pinning the wrong chromedp version silently forces a Go toolchain upgrade
**What goes wrong:** `go get github.com/chromedp/chromedp@latest` resolves to v0.15.1, which requires Go 1.26 — `go mod tidy`/`go get` will rewrite the module's `go` directive from `1.25.1` to `1.26`, which then requires every developer/CI runner to have Go 1.26 installed, an unplanned, silent-until-build-fails change.
**Why it happens:** `@latest` doesn't consider your existing `go.mod` toolchain constraint as a selection criterion by default.
**How to avoid:** Pin explicitly: `go get github.com/chromedp/chromedp@v0.14.2` (verified in this session to keep `go.mod` at `go 1.25.1`).
**Warning signs:** `go.mod`'s `go` directive changes in a diff that was supposed to be "add a rendering dependency."

### Pitfall 3: Rendering an admin-uploaded `<img src="https://...">` or a live MinIO presigned URL defeats network isolation
**What goes wrong:** Even with the `chrome` container on an isolated compose network, if the admin's template (or the assembled HTML) contains an external URL or an internal-but-network-reachable URL for an image, Chromium will still *attempt* the fetch (and the `Fetch`-domain block only fails it after the attempt is made and observed — it's still evidence the surface exists, and any escape from network isolation would otherwise succeed).
**Why it happens:** It's tempting to just point `<img src>` at the existing MinIO presigned-URL pattern already used for slips (Phase 3) — that pattern *requires* network reachability from whatever fetches the URL.
**How to avoid:** The Go app (which has legitimate MinIO/network access) fetches image bytes itself and inlines them as base64 `data:` URIs into the HTML string before handing it to Chromium via `page.setDocumentContent`. Verified this session: a TH Sarabun-equivalent font embedded this way rendered correctly with zero network activity.
**Warning signs:** Any `src="http…"` or `src="https…"` (as opposed to `src="data:…"`) surviving into the final HTML string that gets sent to Chromium.

### Pitfall 4: Auto-retry and manual resend both writing to the same `attempts` counter can create confusing UX
**What goes wrong:** If "resend" (staff-triggered) simply flips `status` back to `pending` without resetting/tracking `attempts` separately from auto-retry attempts, the UI's "ครั้งที่ {n}" (attempt #n) counter conflates automated and manual sends, and a job that already exhausted `max_attempts` might become permanently un-resendable if the poll query filters on `attempts < max_attempts`.
**Why it happens:** Reusing one `outbox_jobs` row's `attempts` field for two semantically different actions (system retry vs. staff-initiated resend).
**How to avoid:** Recommend a manual resend explicitly resets `attempts = 0` and `next_attempt_at = now()` on re-arm (this is a deliberate design point flagged in Open Questions below — the CONTEXT.md decisions don't specify this exact reset behavior, so the planner should make and document this call).
**Warning signs:** A "failed" receipt that shows a resend button but resend has no visible effect because the poll query's `attempts < max_attempts` guard still excludes it.

### Pitfall 5: Backoff/retry numbers borrowed uncritically from raw-SMTP-relay conventions
**What goes wrong:** RFC 5321-style SMTP retry guidance ("keep trying for 4–5 days") is designed for relay-to-relay delivery against transient network conditions between mail servers. This system calls a managed provider's HTTP API (SES/Postmark), where most failures (bad API key, quota, malformed request, permanently-invalid recipient) will not resolve merely by waiting days — and staff already have an always-available manual resend + download fallback (D-57).
**Why it happens:** RFC 5321 numbers are the first thing that surfaces in generic "email retry" research and look authoritative, but they answer a different question.
**How to avoid:** [ASSUMED — design recommendation, not an authoritative external fact] A short-but-covering schedule is more appropriate here: **5 max attempts**, backoff roughly `1 min → 5 min → 15 min → 1 hr → 4 hr`, then terminal `failed` (worker stops auto-retrying; UI shows the "max attempts reached" copy already specified in 04-UI-SPEC.md). This is deliberately shorter than generic job-queue advice (5–10 attempts, `min(2^n, cap)` backoff) because staff have a fast, always-available manual path — long automated retry windows mostly just delay staff noticing a real problem.
**Warning signs:** None yet observed (this is a forward-looking design recommendation) — flag for stakeholder/planner confirmation since CONTEXT.md explicitly deferred exact numbers to the planner.

---

## Code Examples

### Email sender interface (D-60)
```go
// internal/mailer/sender.go
package mailer

type Message struct {
	To          string
	Subject     string
	BodyHTML    string
	BodyText    string
	Attachment  Attachment
}

type Attachment struct {
	Filename    string
	ContentType string // "application/pdf"
	Data        []byte
}

type SendResult struct {
	ProviderMessageID string // "" for dev/local sender
	SentAt            time.Time
}

// EmailSender is the seam D-60 requires: swap the concrete implementation
// (dev-local now, SES/Postmark later) without touching worker code.
type EmailSender interface {
	Send(ctx context.Context, msg Message) (SendResult, error)
}
```
```go
// internal/mailer/dev_sender.go — zero external dependencies, D-60 compliant.
// Writes the message to a local directory (or a MinIO "dev-outbox/" prefix)
// so developers/QA can open the .html/.pdf and visually confirm content —
// this is explicitly NOT a production SMTP path (CLAUDE.md forbids that).
type DevSender struct{ OutDir string }

func (d *DevSender) Send(ctx context.Context, msg Message) (SendResult, error) {
	dir := filepath.Join(d.OutDir, uuid.NewString())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return SendResult{}, err
	}
	os.WriteFile(filepath.Join(dir, "body.html"), []byte(msg.BodyHTML), 0o644)
	os.WriteFile(filepath.Join(dir, msg.Attachment.Filename), msg.Attachment.Data, 0o644)
	return SendResult{SentAt: time.Now()}, nil
}
```

### Golden-file test shape (Go, no new dependency)
```go
// internal/pdf/render_golden_test.go
func TestRenderGolden_ThaiWorstCase(t *testing.T) {
	pdfBytes := renderFixture(t, "thai_worst_case") // fixed sample data, fixed template
	pngBytes := rasterizeViaPdftoppm(t, pdfBytes)    // exec.Command("pdftoppm", "-png", "-r", "150", ...)

	goldenPath := "testdata/thai_worst_case.golden.png"
	if *updateGolden {
		os.WriteFile(goldenPath, pngBytes, 0o644)
		return
	}
	golden, err := os.ReadFile(goldenPath)
	require.NoError(t, err)
	// Verified this session: identical fixture + pinned Chromium image + pinned
	// fonts produces byte-identical PNG output across repeated runs — exact
	// comparison is sufficient, no tolerance/pixel-diff library needed.
	if !bytes.Equal(golden, pngBytes) {
		os.WriteFile("testdata/thai_worst_case.actual.png", pngBytes, 0o644) // CI artifact for review
		t.Fatal("rendered PDF no longer matches golden PNG — see thai_worst_case.actual.png")
	}
}
```

---

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|---------------|--------|
| Pure-Go PDF generation libraries (gofpdf, Maroto) for any document with complex script shaping | Headless-browser-engine rendering (Chromium via chromedp/puppeteer/rod) | Long-standing (years) for CJK/Indic/Thai scripts specifically | Confirmed again in this session's live spike — no pure-Go library reproduces HarfBuzz-equivalent combining-mark shaping |
| chromedp requiring the caller to manage a locally-installed Chrome binary path | Official `chromedp/headless-shell` slim Docker image + `NewRemoteAllocator` | Established pattern, actively maintained | Cleanly separates the app's deploy artifact from the browser engine's — verified working end-to-end this session |
| Fuzzy/tolerance-based visual regression diffing as the default approach | Exact-byte comparison when the render pipeline is fully pinned (browser digest + fonts + fixed input) | N/A — pipeline-dependent, verified case-by-case | This session's determinism test justifies skipping a pixel-diff dependency entirely for this specific pipeline |

**Deprecated/outdated:**
- Treating `--no-sandbox` as sufficient security posture for headless Chromium: it disables the OS-level sandbox for container compatibility, which is why the CDP-level mitigations (JS disable + Fetch block) plus network-namespace isolation are all necessary — `--no-sandbox` is an operational necessity in containers, not a security control.

---

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Retry schedule: 5 max attempts, backoff 1min/5min/15min/1hr/4hr | Common Pitfalls (Pitfall 5) | If wrong, staff may see receipts marked `failed` too eagerly (or the auto-retry window annoyingly long) — low risk given manual resend always available (D-57); easy to tune post-launch since it's config, not a schema decision |
| A2 | Manual resend resets `attempts=0` and `next_attempt_at=now()` on the same outbox_jobs row (rather than inserting a new job row, or using a separate `resend_count`) | Common Pitfalls (Pitfall 4), Architecture Patterns | If wrong, staff-triggered resends could get silently excluded by the `attempts < max_attempts` poll filter, appearing to staff as "resend did nothing" — needs explicit confirmation in planning, flagged as Open Question below |
| A3 | TH Sarabun New must be sourced/licensed separately by the team — it is not distributed via `fonts-thai-tlwg` or any apt package (confirmed: that package installs Waree/Purisa/Garuda/Norasi/Kinnari/Kite/etc., not TH Sarabun New) | Standard Stack, Common Pitfalls (Pitfall 1) | If the team assumes `fonts-thai-tlwg` alone satisfies "TH Sarabun New requirement," the actual production receipts will render in the wrong (though still Thai-correct) font — a compliance/branding mismatch, not a shaping bug |
| A4 | `chromedp/headless-shell:stable`'s digest (`sha256:313ed72...`) should be re-verified and explicitly pinned at implementation time, not assumed stable from this research session | Standard Stack | `:stable` is a moving tag; if the plan pins only the tag (not a digest), CI/prod could drift to a Chromium version with different rendering behavior, undermining golden-file determinism |
| A5 | Docker-compose network isolation for the `chrome` sidecar (no outbound route) is achievable in the target deployment environment (on-prem vs. cloud, per PROJECT.md's open hosting question) | Architecture Patterns, Security Domain | If the hosting environment can't easily give this container zero network reachability, the CDP-level mitigations (verified working) become the *sole* line of defense rather than defense-in-depth — still adequate per this session's verification, but worth flagging since D-58 asks for both |

**If this table is empty:** N/A — see entries above; all are flagged for planner/stakeholder confirmation rather than presented as settled fact.

---

## Open Questions

1. **How exactly does "resend" interact with the `attempts`/`next_attempt_at` columns?**
   - What we know: D-57 requires resend to never allocate a new receipt number and to always reuse the frozen PDF (D-56) — that part is settled. The worker's automatic retry needs `attempts < max_attempts` to stop auto-retrying a permanently-failed job.
   - What's unclear: Whether a staff-triggered resend should (a) reset `attempts=0` on the existing outbox job row, (b) insert a fresh outbox job row referencing the same donation with a `manual: true` flag in the payload (cleaner audit separation of auto vs. manual sends), or (c) bypass the outbox table entirely for resend and call the email-send path directly and synchronously from the resend HTTP handler (simpler, but reintroduces "email in the request path" which NFR-07 was designed to avoid — though resend is a rare, staff-initiated action, so this latency tradeoff may be acceptable).
   - Recommendation: **(b)** — insert a new outbox job row (job_type could stay `issue_receipt` or become `resend_receipt`) referencing the same `donation_id`, keeping the async/off-the-request-path property consistent with the rest of the design, and giving a clean per-send audit trail in `email_delivery` (one row per actual send attempt, whether automated or manual).

2. **Does the receipt-PDF reference live as a column on `donations`, or a separate `receipt_pdfs` table?**
   - What we know: D-56 requires exactly one frozen PDF per issued receipt, never re-rendered. CONTEXT.md leaves the schema shape to the planner.
   - What's unclear: A single `receipt_pdf_object_key` column on `donations` is simplest (1:1 relationship is inherent — a receipt has exactly one frozen PDF). A separate table only helps if you anticipate multiple PDF artifacts per receipt (e.g., versioned re-renders), which D-56 explicitly rules out for MVP.
   - Recommendation: a nullable column on `donations` (`receipt_pdf_object_key TEXT`, populated by the worker once rendered) is sufficient and simplest; avoid a separate table unless a concrete future need for multiple artifacts per receipt emerges.

3. **Should the receipt-settings config table be a new table, or an ALTER to Phase 2's `receipt_number_config`?**
   - What we know: CONTEXT.md's canonical_refs explicitly flag "พิจารณาต่อยอด/รวมกับ config table ของ Phase 2" (consider extending/merging with the Phase 2 config table), and the UI-SPEC's Tab 4 surfaces the *existing* Phase 2 config (separator/padding) in the *same* Admin settings screen.
   - What's unclear: `receipt_number_config` (migration 000004) is a single-row table (`id BOOLEAN PRIMARY KEY DEFAULT true`) scoped tightly to number-format fields. Adding template HTML, 4+ image references, and §6/1x2x text to the *same* row would work (it's already single-row-enforced) but makes the row large and mixes very different concerns (a number-format decision vs. a multi-KB HTML blob) in one table.
   - Recommendation: keep `receipt_number_config` as-is (Phase 2 owns number format) and add a **new** single-row `receipt_template_config` table (or similarly named) for template HTML + §6 text + 1x/2x + image references — the Admin UI screen can still present both in one page (Tab 4 reads from `receipt_number_config`, Tabs 1–3 read/write `receipt_template_config`) without forcing a schema merge. This avoids an ALTER to an existing, already-proven Phase 2 table.

4. **Exact TH Sarabun New font file sourcing/licensing** — flagged in Assumptions Log A3; this is a concrete action item for the plan (source the file, confirm redistribution license — TH Sarabun New is commonly distributed under SIL OFL by Thai government/community sources, but this must be confirmed before bundling into the repo), not something research alone can resolve.

---

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|-------------|-----------|---------|----------|
| Docker | `chrome` sidecar service | ✓ (verified this session) | 28.1.1 | — |
| `chromedp/headless-shell:stable` image | PDF rendering | ✓ (pulled and ran successfully this session) | Chrome/148.0.7778.97 (as of pull date) | — |
| `fonts-thai-tlwg` apt package | Thai font fallback in render container | ✓ (confirmed available in image's Debian trixie base) | 1:0.7.3-1 | — |
| TH Sarabun New font file | Correct production Thai typography | ✗ — not present anywhere in this repo or any system package; must be sourced by the team | — | fonts-thai-tlwg's bundled fonts (Waree/Purisa/etc.) render correct Thai shaping but are visually different from TH Sarabun New — acceptable only as a temporary placeholder, not for production receipts |
| `poppler-utils` (`pdftoppm`) | Golden-file CI rasterization | ✓ (present in this dev sandbox; must be added to the CI image) | — | none needed — trivial apt/apk install |
| Go 1.25.1 toolchain | Building with pinned chromedp v0.14.2 | ✓ (matches project's existing go.mod) | go1.25.1 | Upgrading to Go 1.26 would unlock chromedp v0.15.1, but is unnecessary this phase |
| `aws-sdk-go-v2` / SES credentials | Real email delivery | ✗ (deferred, D-60 stakeholder gate) | — | Dev/local `EmailSender` implementation (no external dependency) |

**Missing dependencies with no fallback:**
- TH Sarabun New font file itself — must be sourced before final production receipts can match the mandated typography (a functional fallback exists for interim testing, see above).

**Missing dependencies with fallback:**
- Real email provider (SES/Postmark) — dev/local `EmailSender` implementation covers this phase's scope entirely.

---

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | `go test` + `testify` (existing, `github.com/stretchr/testify v1.11.1`) |
| Config file | none — standard `go test ./...`; testcontainers-go (existing, v0.43.0) spins Postgres for integration tests |
| Quick run command | `go test ./internal/pdf/... ./internal/worker/... ./internal/mailer/... ./internal/settings/... -short` |
| Full suite command | `go test ./... -run Integration` (matches existing Phase 1–3 convention of `_integration_test.go` / `_test.go` suffixes) |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|---------------------|--------------|
| FR-20/21/22 | PDF contains letterhead/watermark/signature, renders without error | golden-file (rasterized PNG exact-match) | `go test ./internal/pdf/... -run TestRenderGolden` | ❌ Wave 0 |
| FR-24 | §6 text + 1x/2x sourced from config appears in rendered output | golden-file + text-extraction assertion (`pdftotext` on the golden PDF) | `go test ./internal/pdf/... -run TestRenderGolden_Section6` | ❌ Wave 0 |
| FR-23 | Thai vs English template selected per `donor_language`, stacked tone marks correct | golden-file, TWO fixtures (th worst-case, en) | `go test ./internal/pdf/... -run TestRenderGolden_ThaiWorstCase` / `_English` | ❌ Wave 0 (fixtures verified feasible this session) |
| NFR-07 | Worker processes one job (render+store+email) in ~2–3s | integration test measuring wall-clock duration against local `chrome` + local dev `EmailSender` | `go test ./internal/worker/... -run TestProcessJobLatency` | ❌ Wave 0 |
| FR-25/26/27 | Email sent with correct attachment/language; `email_delivery` row recorded; retry on failure | integration test (fake `EmailSender` returning induced errors on first N calls) | `go test ./internal/worker/... -run TestEmailRetryBackoff` | ❌ Wave 0 |
| FR-28 | Staff download works with no email on file | E2E integration test (real HTTP path per Conventions gate) | `go test ./internal/donation/... -run TestDownloadReceiptE2E` | ❌ Wave 0 |
| FR-33/NFR-09 | Admin settings save/read round-trip, no deploy required | E2E integration test (real HTTP path, real Keycloak-shaped token, Admin role) | `go test ./internal/settings/... -run TestSettingsE2E` | ❌ Wave 0 |
| D-58 security mitigations | JS disabled + network blocked during render | **Already spiked and verified in this research session** — port the verified spike code into a permanent regression test | `go test ./internal/pdf/... -run TestRenderSandboxSecurity` | ❌ Wave 0 (but logic is proven, not speculative) |

### Sampling Rate
- **Per task commit:** quick run command above (`-short`, skips testcontainers-heavy integration tests)
- **Per wave merge:** full suite command, including the `chrome` sidecar via testcontainers or docker-compose in CI
- **Phase gate:** full suite green + golden-file visual test green in CI before `/gsd-verify-work`, PLUS the Conventions-mandated E2E integration test for every new HTTP route (settings, resend, download, preview) with a real Keycloak-shaped token — this phase's new routes are exactly the kind of surface where Phase 3's seam bugs occurred.

### Wave 0 Gaps
- [ ] `internal/pdf/render_golden_test.go` + `testdata/*.golden.png` fixtures — covers FR-20/21/22/23/24
- [ ] `internal/pdf/render_sandbox_security_test.go` — ports this session's verified JS-disable/network-block spike into a permanent regression test (must fail loudly if a future chromedp upgrade regresses either mitigation)
- [ ] `internal/worker/worker_test.go` + testcontainers wiring for a `chrome` container in CI — covers NFR-07, FR-25/26/27
- [ ] `internal/testutil/chrome.go` — a testcontainers helper analogous to existing `internal/testutil/postgres.go`/`keycloak.go`, starting the `chrome` sidecar image for integration tests
- [ ] CI image additions: `poppler-utils` (for `pdftoppm`), and the ability to build/pull the custom `chrome` sidecar image

---

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|----------------|---------|--------------------|
| V2 Authentication | no (new) | Unchanged — Keycloak OIDC (existing) |
| V3 Session Management | no (new) | Unchanged |
| V4 Access Control | yes | New routes (settings, resend, download, preview) gated via existing `auth.RequireRoles`/`RequireAnyRole` middleware patterns — Admin-only for settings, Checker/Admin for resend, any donationGroup role for download |
| V5 Input Validation | yes | `html/template` contextual autoescaping for donation-data-into-template; magic-byte validation (reuse `internal/storage` pattern) for template image uploads; template HTML itself validated only for parse-ability (`template.Parse` error → reject save), not sanitized/rewritten — the render sandbox is the real control, not input filtering |
| V6 Cryptography | no (new) | Unchanged — no new PII fields this phase |
| V12 File Handling (SSRF-adjacent) | yes | Chromium render sandbox: `Emulation.SetScriptExecutionDisabled`, `Fetch.Enable`+`FailRequest`-all, `Page.setDocumentContent` (never `Page.Navigate` to a caller-influenced URL), network-isolated compose service — all four verified working live this session |

### Known Threat Patterns for This Stack

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|------------------------|
| Admin-authored HTML template containing `<script>` that exfiltrates data or manipulates the render | Tampering / Information Disclosure | `Emulation.SetScriptExecutionDisabled(true)` — verified this session: inline script did not execute |
| Admin-authored HTML template containing `<img>`/`<link>`/`fetch()`-equivalent pointed at an internal service (SSRF) or attacker-controlled endpoint (exfiltration/tracking pixel) | Information Disclosure / SSRF | `Fetch.Enable` + `FailRequest`-all (CDP layer) AND network-isolated compose service (network layer) AND `Page.setDocumentContent` instead of navigation (no legitimate reason for Chromium to need network at all) — all three verified/architected this session |
| Donor-supplied field (name/address, both untrusted end-user input even though entered by a Maker on their behalf) breaking out of the admin's HTML structure to inject markup | Tampering | `html/template` contextual autoescaping (stdlib, not custom) |
| Template image upload (letterhead/seal/signature/watermark) disguised as an image but actually an executable/script | Tampering | Reuse `internal/storage`'s magic-byte (`gabriel-vasile/mimetype`) validation pattern, already proven in Phase 3 for slip uploads |
| Unauthorized template/config modification | Tampering / Repudiation | Admin-only RBAC gate (existing `auth.RequireRoles(auth.RoleAdmin)` pattern, already used for the `adminGroup`) + append-only audit log (existing `internal/audit`, reuse `AppendAuditEntryTx` for every settings save) |
| Worker double-processing the same outbox job across multiple instances | Tampering (data integrity) | Atomic `UPDATE...WHERE id=(SELECT...FOR UPDATE SKIP LOCKED)` claim (Pattern 1) — no window for two workers to claim the same row |

---

## Sources

### Primary (HIGH confidence — verified live in this research session)
- Live spike: `google-chrome --headless --print-to-pdf` rendering worst-case Thai text (stacked tone marks, mixed Thai+Latin, Latin-leading) — visual output inspected directly
- Live spike: chromedp v0.14.2, `Page.PrintToPDF`, identical Thai rendering via the Go API
- Live spike: `docker pull chromedp/headless-shell:stable` (digest `sha256:313ed7255ae1e155fb157631a6d4c0eb8b65bbe06de9e704ed834399bdf678ff`) — confirmed zero Thai font support out of the box (tofu boxes reproduced), then confirmed `apt-get install fonts-thai-tlwg` fixes it
- Live spike: `chromedp.NewRemoteAllocator` connecting to the containerized Chromium over CDP websocket — full render pipeline works end-to-end
- Live spike: `Emulation.SetScriptExecutionDisabled(true)` blocks inline `<script>` execution (verified via `document.title` check)
- Live spike: `Fetch.Enable` (catch-all pattern) + `FailRequest` blocks 100% of outbound requests (verified via an SSRF-probe `<img>` tag)
- Live spike: `@font-face` with a base64 `data:` URI font renders correctly with zero network activity, inside the Thai-font-equipped container
- Live spike: two renders of the identical fixture produce byte-identical (md5-matching) rasterized PNGs
- `proxy.golang.org` queries for `chromedp/chromedp`, `go-rod/rod`, `aws-sdk-go-v2` (+ submodules), `orisano/pixelmatch` — version/`go.mod` requirement verification
- GitHub API queries for stars/last-push/open-issues on the above repos — adoption/health signal
- Direct reads of this repo's existing code: `internal/db/queries/outbox.sql`, `migrations/000007_outbox_jobs.up.sql`, `internal/storage/client.go`, `internal/config/config.go`, `internal/i18n/bundle.go`, `internal/donation/service.go`, `migrations/000004_receipt_number_tables.up.sql`, `migrations/000005_donations.up.sql`, `cmd/server/main.go`, `docker-compose.yml`, `Dockerfile`, `go.mod`

### Secondary (MEDIUM confidence — WebSearch, cross-checked against official docs where cited inline above)
- pkg.go.dev/html/template — contextual autoescaping behavior and the `template.HTML` misuse pitfall
- Chrome DevTools Protocol reference (`Fetch`, `Emulation`, `Page` domains) — chromedevtools.github.io/devtools-protocol
- PostgreSQL `FOR UPDATE SKIP LOCKED` job-queue pattern — multiple independent Go blueprints/articles converged on the same atomic `UPDATE...WHERE id=(SELECT...)` shape
- Puppeteer troubleshooting docs — `fonts-thai-tlwg` requirement for Thai charset support in headless Chromium (cross-checked, matches CLAUDE.md's own cited source)

### Tertiary (LOW confidence — flagged for validation, marked `[ASSUMED]` inline where used)
- Backoff/retry numeric schedule recommendation (Common Pitfalls, Pitfall 5) — a design judgment call, not sourced from an authoritative spec; flagged as A1 in Assumptions Log
- Resend semantics recommendation (Open Question 1) — architectural judgment, flagged for planner confirmation

---

## Metadata

**Confidence breakdown:**
- Standard stack (chromedp version pin, Docker sidecar approach): **HIGH** — verified via live spikes, not just docs
- Thai rendering correctness: **HIGH** — verified visually with worst-case fixtures via two independent render paths
- D-58 security mitigations: **HIGH** — verified live (JS-disable + network-block both confirmed to actually block the attack)
- Golden-file testing approach: **HIGH** — determinism verified empirically this session
- Worker/queue pattern: **HIGH** — well-established pattern, cross-checked across multiple independent sources, directly compatible with existing `outbox_jobs` schema
- Retry/backoff exact numbers: **LOW** — explicitly a design recommendation (see Assumptions Log A1), CONTEXT.md defers this to the planner
- Schema shape for new tables (Open Questions 1–3): **MEDIUM** — reasoned recommendations grounded in existing Phase 1–3 patterns, but genuinely left open by CONTEXT.md for the planner to decide

**Research date:** 2026-07-04
**Valid until:** ~30 days for the architecture/pattern recommendations; re-verify the `chromedp/headless-shell` digest and `fonts-thai-tlwg` package availability immediately before implementation if more than a few days have passed (base images and apt repos do change), and re-check chromedp's latest-vs-Go-version compatibility if the project's Go toolchain is upgraded in the interim.
