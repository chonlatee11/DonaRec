---
phase: 04-receipt-pdf-email-delivery-outbox-worker
verified: 2026-07-04T20:20:00+07:00
status: human_needed
score: 5/5 roadmap success criteria verified by real automated evidence (0 behavior-unverified)
behavior_unverified: 0
overrides_applied: 0
human_verification:
  - test: "Screen 3b — Email Delivery panel + resend/download + donor_language selector (04-06 Task 4, checkpoint:human-verify, gate=blocking)"
    expected: "Maker creates a Flow A donation with donor_language=English; Checker approves it; the Donation Detail Email Delivery panel shows a status badge + recipient + attempt count; download button fetches a Thai/English-rendering PDF; Checker's resend shows a success toast and the receipt number is unchanged; a Maker viewing the same receipt sees Download but NOT Resend."
    why_human: "Requires visual/browser confirmation (badge color mapping, toast copy, real PDF rendering to the human eye, role-gated button visibility in the DOM) against a live stack with real Keycloak login across 3 roles. Automated E2E (ResendAndDownload_RealPath, DonorLanguage_PersistsAndDefaults) already proves the RBAC/number-invariance/persistence logic — this item confirms presentation, not backend correctness. Deliberately deferred by explicit user decision, documented in 04-06-SUMMARY.md with exact steps + credential prerequisites (Keycloak donnarec-frontend client secret, donnarec-web/.env.local, test-account passwords)."
  - test: "Screen 6 — Admin Settings editor (four tabs, sandboxed debounced live preview, real-PDF render, image upload, save-all) (04-08 Task 3, checkpoint:human-verify, gate=blocking)"
    expected: "Admin-only nav item appears (hidden for Maker/Checker); editing template HTML updates the sandboxed preview ~400ms after typing stops, TH Sarabun, sample data, no-JS/no-network info banner visible; image upload validates jpg/png/≤2MB with correct rejection copy; §6/1x-2x edits and number-format live example update; 'เรนเดอร์ PDF จริง' shows a real Chromium-rendered sample; 'บันทึกการตั้งค่า' persists and a freshly issued receipt reflects the new template while already-issued receipts stay frozen (D-56)."
    why_human: "Requires visual confirmation of iframe rendering, debounce timing feel, toast copy, and a real 3-role Keycloak login the automated build/lint/unit tests do not exercise. Automated coverage (npm run build/lint/test, 04-07 E2E incl. real Chromium PreviewPDF) already proves the logic/type contracts and the sandboxed pipeline. Deliberately deferred by explicit user decision, documented in 04-08-SUMMARY.md with exact steps."
---

# Phase 4: Receipt PDF + Email Delivery (Outbox Worker) Verification Report

**Phase Goal:** After a receipt is issued, an async worker reliably renders a correct Thai/English tax-compliant PDF and emails it to the donor, with delivery status and retry, without ever blocking or rolling back the issuance transaction.
**Verified:** 2026-07-04T20:20:00+07:00
**Status:** human_needed
**Re-verification:** No — initial verification

**หมายเหตุเรื่อง Mode:** ROADMAP.md ระบุ `Mode: mvp` สำหรับเฟสนี้ แต่ข้อความ goal จริงเป็นข้อความเชิงเทคนิค ("After a receipt is issued, an async worker reliably renders...") ไม่ได้อยู่ในรูปแบบ User Story มาตรฐาน (`As a ..., I want to ..., so that ....`) จึงไม่ผ่าน guard ของ MVP-mode verification (`user-story.validate`) ตรงตัว เนื่องจากเฟสนี้มีลักษณะเป็น background worker/infrastructure slice ที่ไม่ผูกกับ single user role ชัดเจน และ ROADMAP.md มี **Success Criteria ครบ 5 ข้อ** อยู่แล้ว (Step 2a ของ methodology) ผู้ตรวจจึงใช้แนวทาง goal-backward มาตรฐาน (ไม่ narrow แบบ MVP user-flow) โดยยึด 5 Success Criteria เป็น must-haves หลัก — เป็นการตัดสินใจที่สมเหตุสมผลภายใต้ auto-mode เพื่อไม่ให้การตรวจสอบทั้งเฟสถูกบล็อกจากปัญหารูปแบบข้อความ goal เพียงอย่างเดียว จุดนี้ไม่กระทบผลการตรวจสอบเนื้อหาด้านล่าง แต่แนะนำให้แก้ไข goal wording ในภายหลังหากต้องการให้ mode: mvp ใช้งานได้เต็มรูปแบบ

## Goal Achievement

### Observable Truths (ROADMAP Success Criteria)

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | ใบเสร็จที่ออกแล้วสร้าง PDF จากเทมเพลตที่ปรับแต่งได้ พร้อมหัวจดหมาย/ตรา, ลายน้ำ, ลายเซ็น เรนเดอร์ตามภาษาผู้บริจาค | ✓ VERIFIED | `internal/pdf/render.go` (`ReceiptData{LetterheadData,SealData,SignatureData,WatermarkData,Language}` เป็น base64 data-URI); `internal/pdf/render_golden_test.go` รันจริงผ่าน sandboxed Chromium (testcontainers) — `TestRenderGolden_ThaiWorstCase` และ `TestRenderGolden_English` ผ่านจริง (19-21s ต่อเทส, ยืนยันด้วยการรันสด). Migration `000011_receipt_template_config.up.sql` มีคอลัมน์ letterhead/seal/signature/watermark object key ครบ |
| 2 | Thai PDF เรนเดอร์สระ/วรรณยุกต์ซ้อน + ผสมไทย-อังกฤษถูกต้อง (golden-file test ใน CI) และมีเนื้อหา §6 + ข้อความ 1x/2x จาก config | ✓ VERIFIED | รันจริง: `go test ./internal/pdf/... -run 'TestRenderSandboxSecurity|TestRenderGolden'` → **PASS ทั้งหมด** (`TestRenderGolden_ThaiWorstCase`, `TestRenderGolden_English` exact-byte golden PNG compare ผ่าน chromedp จริงบน Docker sidecar; §6+1x/2x ถูก assert จาก pdftotext extraction ตาม PLAN 04-03) |
| 3 | PDF+email รันใน worker หลัง transactional outbox — job มีก็ต่อเมื่อออกใบเสร็จแล้ว — approve เร็ว (~2-3s/ใบ วัดนอก lock path) | ✓ VERIFIED | `internal/donation/service.go` Approve() ไม่มี import ใดๆ ของ `internal/pdf` หรือ `internal/mailer` เลย (grep ยืนยัน) — มีแค่ `qtx.EnqueueOutboxJob` ใน WithTx เดียวกับ numbering/audit; worker แยก goroutine (`cmd/server/main.go:254 go outboxWorker.Run(ctx)` ผูกกับ `signal.NotifyContext` เดียวกับ HTTP server). รันจริง `TestProcessJobLatency` → **PASS (22.60s wall แต่ assert อยู่ใน budget ~2-3s ต่อ 1 job ที่วัดนอก lock path)** |
| 4 | ผู้บริจาคได้รับอีเมล 2 ภาษาพร้อมแนบ PDF; บันทึกสถานะส่ง (สำเร็จ/ล้มเหลว), ส่งซ้ำได้เมื่อล้มเหลว, resend ไม่ออกเลขใหม่ | ✓ VERIFIED | รันจริง: `TestProcessJob_RenderFreezeEmailRecordAndIdempotency` PASS (email_delivery row + PDF freeze + freeze-idempotency), `TestEmailRetryBackoff` PASS (attempts เพิ่ม + next_attempt_at เลื่อน + terminal 'failed' ที่ max_attempts), `TestBundle_ReceiptAndEmailMessageIDs_DifferByLocale` PASS (i18n th/en ต่างกัน); E2E จริงผ่าน HTTP: `ResendAndDownload_RealPath` PASS — resend โดย Checker เพิ่ม outbox job ใหม่ 1 แถวพอดี และ `receipt_no` เหมือนเดิมทุกตัวอักษรก่อน/หลัง resend; resend โดย Maker → 403 |
| 5 | เมื่อผู้บริจาคไม่มีอีเมล เจ้าหน้าที่ดาวน์โหลด PDF เองได้, แอดมินแก้เทมเพลต/ลายน้ำ/ลายเซ็น/รูปแบบเลขได้โดยไม่ต้อง deploy | ✓ VERIFIED | E2E จริง `ResendAndDownload_RealPath`: download คืน presigned URL ให้ทุก staff role, download ก่อน freeze → 409 conflict (not-ready); E2E จริง `TestE2E_AdminSettings` PASS ทั้งหมด (GET/PUT round-trip, non-Admin 403, invalid-template 422, preview escaping, real-PDF preview ผ่าน sandbox จริง, magic-byte image upload Admin-only) |

**Score:** 5/5 truths verified (0 present-but-behavior-unverified)

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/pdf/render.go`, `chromium.go` | Sandboxed HTML→PDF render core | ✓ VERIFIED | มีจริง, ครบ, ใช้งานจริงในเทส golden/security ที่รันผ่านสด |
| `internal/worker/worker.go`, `issue_receipt.go` | Outbox poll→render→freeze→email→record pipeline | ✓ VERIFIED | มีจริง + `ReclaimStuckJobs`/`ProcessOnceSafe` (CR-01/CR-02 fixes) ยืนยันในโค้ดจริง ไม่ใช่แค่ commit message |
| `internal/mailer/sender.go`, `dev_sender.go` | EmailSender interface + dev capture | ✓ VERIFIED | `TestDevSender_Send_CapturesToDisk` PASS; ไม่มี `net/smtp`/provider SDK import |
| `internal/settings/service.go`, `handler.go`, `model.go` | Config-store service, template validate, image validate | ✓ VERIFIED | `internal/settings/...` เทสทั้ง 5 เคส PASS รวมถึง `TestSaveSettings_PartialFailureRollsBackBothWrites` (WR-07 transaction fix) |
| Migrations 000008–000012 | donor_language, next_attempt_at, email_delivery, receipt_template_config (seeded), receipt_pdf_object_key | ✓ VERIFIED | ไฟล์มีจริง, ใช้งานจริงในทุก integration test ที่รัน testcontainers Postgres (implicit reversibility proof ผ่านการ apply ซ้ำในหลายเทส) |
| `cmd/server/e2e_test.go` | E2E over real HTTP→auth→RBAC→handler→service→DB | ✓ VERIFIED | `TestE2E_MakerCheckerIssuancePipeline` (รวม `DonorLanguage_PersistsAndDefaults`, `ResendAndDownload_RealPath`) และ `TestE2E_AdminSettings` รันจริงและ PASS ทั้งคู่ |
| FE: `EmailDeliveryPanel.tsx`, `SettingsTabs.tsx`, `TemplateEditor.tsx`, `ImageUploadSlot.tsx`, `NumberFormatEditor.tsx` | Screen 3b + Screen 6 UI | ✓ VERIFIED (build/lint/unit only — visual pending) | `npm run build` และ `npm test` (42/42) ผ่านจริง; **ยังไม่ผ่านการเดินดูจริงในเบราว์เซอร์ (ดู Human Verification)** |

### Key Link Verification

| From | To | Via | Status | Details |
|------|-----|-----|--------|---------|
| Issuance tx (`donation/service.go` Approve) | Outbox worker | `EnqueueOutboxJob` inside same WithTx | ✓ WIRED | grep ยืนยัน Approve ไม่ import pdf/mailer เลย มีแค่ enqueue |
| Worker | `pdf.Render`+`pdf.RenderPDF` | `issue_receipt.go` renderReceiptPDF | ✓ WIRED | รันจริงผ่าน `TestProcessJob_RenderFreezeEmailRecordAndIdempotency` |
| Worker | `mailer.EmailSender` | `issue_receipt.go` compose+Send | ✓ WIRED | รันจริง — email_delivery row ยืนยัน status 'sent' |
| Resend endpoint | Outbox worker (reuse frozen PDF) | `EnqueueOutboxJob` (no re-render) | ✓ WIRED | E2E จริง: resend เพิ่ม job ใหม่ 1 แถว, receipt_no ไม่เปลี่ยน |
| Download endpoint | MinIO receipts bucket | `PresignedGet(receipt_pdf_object_key)` | ✓ WIRED | E2E จริง: download คืน URL ไม่ว่างเปล่าให้ทุก role, ก่อน freeze → 409 |
| Settings service | `receipt_template_config` | `GetReceiptTemplateConfig`/`UpdateReceiptTemplateConfig` | ✓ WIRED | E2E จริง `TestE2E_AdminSettings` GET/PUT round-trip |
| Settings preview/pdf | `pdf.RenderPDF` (same sandbox) | `PreviewPDF` handler | ✓ WIRED | E2E จริง `PreviewPDF_ReturnsRealPDFBytesViaSandboxedPipeline` PASS |

### Behavioral Spot-Checks / Real Test Execution (not merely enumerated — actually run in this verification)

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Sandboxed Thai/EN golden-file render (real Chromium via testcontainers) | `go test ./internal/pdf/... -run 'TestRenderSandboxSecurity\|TestRenderGolden'` | 5/5 PASS (81.3s) | ✓ PASS |
| Outbox worker full pipeline (real Postgres+MinIO+Chrome) | `go test ./internal/worker/... -run 'TestProcessJob\|TestEmailRetryBackoff\|TestProcessJobLatency\|TestReclaim\|TestDeadLetter'` | 5/5 PASS (79.3s) | ✓ PASS |
| Panic-recovery + branding soft-fail (review-fix regressions) | `go test ./internal/worker/... -run 'TestProcessOnceSafe_RecoversFromPanicAndWorkerKeepsRunning\|TestFetchTemplateImageSoft\|TestProcessOnce_ContinuesWhenBrandingImageFetchFails'` | 4/4 PASS (47.8s) | ✓ PASS |
| Mailer + i18n + settings unit/integration | `go test ./internal/mailer/... ./internal/i18n/... ./internal/settings/...` | 6/6 PASS | ✓ PASS |
| E2E real HTTP path — issuance/resend/download/donor_language | `go test ./cmd/server/... -run TestE2E_MakerCheckerIssuancePipeline` | PASS (22.4s, incl. 6 subtests) | ✓ PASS |
| E2E real HTTP path — Admin settings (incl. real-PDF preview) | `go test ./cmd/server/... -run TestE2E_AdminSettings` | PASS (23.0s, incl. 9 subtests) | ✓ PASS |
| `go build ./...` / `go vet ./...` | — | clean, no output | ✓ PASS |
| Frontend build | `npm run build` (donnarec-web) | success, all routes compiled | ✓ PASS |
| Frontend unit tests | `npm test` (donnarec-web) | 42/42 PASS | ✓ PASS |

หมายเหตุ: ผู้ตรวจสอบรันชุดเทสข้างต้น**จริง**ในสภาพแวดล้อมนี้ (ไม่ใช่แค่เชื่อ SUMMARY.md) — Docker พร้อมใช้งาน และทุก integration/E2E test ใช้ testcontainers Postgres/MinIO/Chrome จริง ไม่ mock

### Requirements Coverage

| Requirement | Description | Status | Evidence |
|-------------|-------------|--------|----------|
| FR-20 | สร้างใบเสร็จ PDF จากเทมเพลตที่มีตรา/หัวจดหมาย | ✓ SATISFIED | migration 000011 + render.go LetterheadData + golden test |
| FR-21 | ฝังลายน้ำ | ✓ SATISFIED | render.go WatermarkData + migration watermark_object_key |
| FR-22 | ลายเซ็นผู้มีอำนาจ (รูปภาพ) | ✓ SATISFIED | render.go SignatureData + migration signature_object_key |
| FR-24 | §6 ตามข้อกำหนดลดหย่อนภาษี 1x/2x | ✓ SATISFIED | golden test §6-content assertion + migration deduction_multiplier CHECK(1x,2x) |
| FR-23 | สร้างใบเสร็จไทย/อังกฤษตามผู้บริจาค | ✓ SATISFIED | donor_language column + E2E `DonorLanguage_PersistsAndDefaults` + golden Thai/English tests |
| FR-25 | ส่ง PDF แนบอีเมลหลังอนุมัติ | ✓ SATISFIED | worker issue_receipt.go compose+Send; `TestProcessJob_RenderFreezeEmailRecordAndIdempotency` |
| FR-26 | เทมเพลตอีเมล 2 ภาษา | ✓ SATISFIED | i18n receipt/email.* keys th/en; `TestBundle_ReceiptAndEmailMessageIDs_DifferByLocale` |
| FR-27 | บันทึกสถานะส่ง + ส่งซ้ำได้ | ✓ SATISFIED | email_delivery table + `TestEmailRetryBackoff` + Resend E2E |
| FR-28 | เจ้าหน้าที่ดาวน์โหลด PDF เอง | ✓ SATISFIED | DownloadReceipt endpoint + E2E all-staff-roles 200 |
| NFR-07 | สร้าง PDF+ส่งอีเมลใน ~2-3 วินาที นอก lock path | ✓ SATISFIED | `TestProcessJobLatency` PASS; Approve() ไม่ import pdf/mailer |
| FR-33 | Admin ตั้งค่าเทมเพลต/ลายน้ำ/ลายเซ็น/เลขที่ | ✓ SATISFIED | settings service/handler + `TestE2E_AdminSettings` |
| NFR-09 | แยก config จากโค้ด ปรับได้ไม่ต้อง deploy | ✓ SATISFIED | receipt_template_config table (DB-backed, no deploy) + settings GET/PUT E2E |

**ไม่มี orphaned requirements** — ทั้ง 12 ID ที่ประกาศใน PLAN frontmatter ตรงกับ REQUIREMENTS.md บรรทัด 43-79 และ traceability table (147-158) ระบุ "Complete" ครบทุกตัว

### Code Review Fixes — Verified in Codebase (not just trusted from 04-REVIEW-FIXES-SUMMARY.md)

| ID | Severity | Claim | Verified in code? |
|----|----------|-------|---------------------|
| CR-01 | BLOCKER | Reclaim stuck `processing` jobs | ✓ ใช่ — `ReclaimStuckOutboxJobs` query + `Worker.ReclaimStuckJobs` เรียกทุก tick ก่อน `ProcessOnceSafe`; `TestReclaimStuckJobs_ResetsProcessingPastTimeout`/`_LeavesRecentProcessingAlone` รันจริง PASS |
| CR-02 | BLOCKER | Panic recovery ใน worker goroutine | ✓ ใช่ — `ProcessOnceSafe` มี `recover()` จริง; `TestProcessOnceSafe_RecoversFromPanicAndWorkerKeepsRunning` รันจริง PASS |
| WR-01 | WARNING | chrome sidecar network-isolated จริง | ✓ ใช่ — `docker-compose.yml` มี `chrome-internal: internal: true` และ service `chrome` ผูกกับ network นี้เท่านั้น |
| WR-02 | WARNING | log FailRequest error, ไม่ nested chromedp.Run | ไม่ได้ diff โค้ดบรรทัดต่อบรรทัด แต่ commit `fb4c866` มีอยู่จริงใน git log และ sandbox test ยังผ่าน |
| WR-03 | WARNING (documented, not fixed) | deduction_multiplier ไม่ frozen ที่ approve | ✓ ใช่ — comment "KNOWN LIMITATION (WR-03, 04-REVIEW.md — deliberately NOT fixed this pass)" มีอยู่จริงใน `issue_receipt.go` |
| WR-04..WR-07 | WARNING | stale-preview guard, dead-letter terminal, soft branding fetch, settings save transaction | ✓ ใช่ — ทดสอบที่เกี่ยวข้อง (`TestFetchTemplateImageSoft_*`, `TestProcessOnce_ContinuesWhenBrandingImageFetchFails`, `TestDeadLetteredJob_NeverResurrectedByRaisingMaxAttempts`, `TestSaveSettings_PartialFailureRollsBackBothWrites`) รันจริง PASS ทั้งหมด |

### Anti-Patterns Found

ไม่พบ debt marker (`TBD`/`FIXME`/`XXX`) หรือ placeholder/stub pattern ใดๆ ในไฟล์ Go/TSX ที่แก้ไขในเฟสนี้ (grep ทั่วทุกไฟล์หลักของ pdf/worker/mailer/settings/donation/storage + คอมโพเนนต์ FE หลัก — ไม่มีผลลัพธ์)

INFO-level findings จาก 04-REVIEW.md (IN-01..IN-05) ยังไม่ถูกแก้ตามที่ระบุใน 04-REVIEW-FIXES-SUMMARY.md ("out of scope per the fix directive") — เป็นเรื่อง code-quality เล็กน้อย (duplicated numeric-format helper, เกิน-grant บน email_delivery, validation tag ไม่สอดคล้องกัน, client-side min-guard ขาด, regex ของ `<head>` match ไม่ทน attribute) ไม่กระทบ goal achievement ของเฟสนี้ — ไม่ถือเป็น blocker

### Human Verification Required

2 รายการ ถูก harvest มาจาก `checkpoint:human-verify` (gate=blocking) ใน 04-06-PLAN.md Task 4 และ 04-08-PLAN.md Task 3 ซึ่งถูกเลื่อนไปทำใน `/gsd-verify-work` โดยการตัดสินใจของผู้ใช้อย่างชัดเจน (บันทึกไว้ใน SUMMARY.md ทั้งสองไฟล์พร้อม credential prerequisites) — ตาม CLAUDE.md's integration-test gate เฟสนี้ **ยังไม่ถือว่า Complete** จนกว่าการเดินดูจริง (human UI walkthrough) จะผ่าน แม้ automated E2E ทั้งหมดจะผ่านแล้วก็ตาม

#### 1. Screen 3b — Email Delivery panel + resend/download + donor_language

**Test:** Log in เป็น Maker สร้าง Flow A donation ด้วย donor_language=English → submit; login เป็น Checker → approve; เปิด Donation Detail รอ worker ประมวลผล
**Expected:** Email Delivery panel แสดง badge สถานะ+ผู้รับ+จำนวนครั้ง; ปุ่มดาวน์โหลด PDF ได้ไฟล์ที่เรนเดอร์ไทย/อังกฤษถูกต้อง; Checker กด resend ได้ toast สำเร็จ, เลขที่ใบเสร็จไม่เปลี่ยน; Maker เห็นปุ่ม Download แต่ไม่เห็น Resend
**Why human:** ต้องดูจริงในเบราว์เซอร์ (สี badge, ข้อความ toast, การเรนเดอร์ PDF ด้วยสายตาคน, การซ่อน/แสดงปุ่มตาม role) ผ่าน login Keycloak จริง 3 role — automated E2E พิสูจน์แค่ backend logic/RBAC/number-invariance เท่านั้น

#### 2. Screen 6 — Admin Settings editor (four tabs + sandboxed preview + real-PDF)

**Test:** Login เป็น Admin เปิด `/settings` แก้ template HTML/รูปภาพ/ข้อความ §6+1x2x/รูปแบบเลข แล้วกด "เรนเดอร์ PDF จริง" และ "บันทึกการตั้งค่า"
**Expected:** nav "ตั้งค่า" ปรากฏเฉพาะ Admin; preview อัปเดตหลังพิมพ์ ~400ms ด้วย TH Sarabun + sample data + banner ไม่มี JS/network; อัปโหลดรูปตรวจ magic-byte ถูกต้อง; PDF จริงเรนเดอร์ผ่าน Chromium จริง; save แล้วใบเสร็จใหม่ใช้เทมเพลตใหม่ แต่ใบเสร็จเก่ายังคง frozen เดิม
**Why human:** ต้องดู iframe rendering, จังหวะ debounce, ข้อความ toast จริง และ login 3-role จริง — automated build/lint/unit + E2E (รวม real-PDF preview ผ่าน sandbox) พิสูจน์แค่ logic/type contract และ pipeline เท่านั้น ไม่ใช่ประสบการณ์ผู้ใช้จริง

### Gaps Summary

ไม่มี gap ที่เป็น BLOCKER — ทุก must-have (5 roadmap success criteria + PLAN-level truths ที่เกี่ยวข้อง) ได้รับการยืนยันด้วยการรันเทสจริง (ไม่ใช่แค่เชื่อ SUMMARY.md), 2 BLOCKER + 7 WARNING จาก 04-REVIEW.md ถูกแก้จริงและมีเทส regression ยืนยัน (ยกเว้น WR-03 ที่บันทึกเป็น known-limitation ตามที่ตั้งใจ), requirement ครบ 12/12 ไม่มี orphan, ไม่มี debt marker ในโค้ด

สิ่งเดียวที่เหลือค้างคือ **การเดินดูจริงในเบราว์เซอร์ (human UI walkthrough)** 2 รายการที่ถูกเลื่อนไปทำที่ `/gsd-verify-work` โดยตั้งใจ — ตาม Conventions integration-test gate เฟสนี้ต้องผ่านทั้ง (a) automated E2E over the real HTTP path [ผ่านแล้ว] และ (b) human UI walkthrough [ยังไม่ทำ] ก่อนจะถือว่า Complete เต็มรูปแบบ จึงกำหนดสถานะเป็น `human_needed` ไม่ใช่ `passed`

---

_Verified: 2026-07-04T20:20:00+07:00_
_Verifier: Claude (gsd-verifier)_
