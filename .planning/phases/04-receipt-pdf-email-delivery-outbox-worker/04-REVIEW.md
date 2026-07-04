---
phase: 04-receipt-pdf-email-delivery-outbox-worker
reviewed: 2026-07-04T00:00:00Z
depth: standard
files_reviewed: 41
files_reviewed_list:
  - donnarec-api/internal/pdf/render.go
  - donnarec-api/internal/pdf/chromium.go
  - donnarec-api/internal/worker/worker.go
  - donnarec-api/internal/worker/issue_receipt.go
  - donnarec-api/internal/storage/client.go
  - donnarec-api/internal/settings/service.go
  - donnarec-api/internal/settings/handler.go
  - donnarec-api/internal/settings/model.go
  - donnarec-api/internal/donation/service.go
  - donnarec-api/internal/donation/handler.go
  - donnarec-api/internal/donation/model.go
  - donnarec-api/internal/donation/errors.go
  - donnarec-api/internal/mailer/sender.go
  - donnarec-api/internal/mailer/dev_sender.go
  - donnarec-api/internal/config/config.go
  - donnarec-api/cmd/server/main.go
  - donnarec-api/internal/db/queries/outbox.sql
  - donnarec-api/internal/db/queries/email_delivery.sql
  - donnarec-api/internal/db/queries/settings.sql
  - donnarec-api/internal/db/queries/donations.sql
  - donnarec-api/internal/db/queries/receiptno.sql
  - donnarec-api/migrations/000008_donor_language.up.sql
  - donnarec-api/migrations/000009_outbox_next_attempt_at.up.sql
  - donnarec-api/migrations/000010_email_delivery.up.sql
  - donnarec-api/migrations/000011_receipt_template_config.up.sql
  - donnarec-api/migrations/000012_receipt_pdf_reference.up.sql
  - donnarec-api/docker/chrome.Dockerfile
  - donnarec-api/docker-compose.yml
  - donnarec-web/app/api/bff/donations/[id]/resend/route.ts
  - donnarec-web/app/api/bff/donations/[id]/receipt-pdf/route.ts
  - donnarec-web/app/api/bff/settings/route.ts
  - donnarec-web/app/api/bff/settings/preview/route.ts
  - donnarec-web/app/api/bff/settings/preview/pdf/route.ts
  - donnarec-web/app/api/bff/settings/images/[slot]/route.ts
  - donnarec-web/app/admin/settings/page.tsx
  - donnarec-web/components/SettingsTabs.tsx
  - donnarec-web/components/TemplateEditor.tsx
  - donnarec-web/components/ImageUploadSlot.tsx
  - donnarec-web/components/NumberFormatEditor.tsx
  - donnarec-web/components/EmailDeliveryPanel.tsx
  - donnarec-web/components/DeliveryStatusBadge.tsx
  - donnarec-web/components/DonationForm.tsx
  - donnarec-web/components/DonationDetailView.tsx
  - donnarec-web/lib/session-role.ts
  - donnarec-web/lib/settings.ts
  - donnarec-web/lib/donations.ts
  - donnarec-web/lib/receipt-number-format.ts
  - donnarec-web/lib/debounce.ts
  - donnarec-web/lib/bff.ts
findings:
  critical: 2
  warning: 7
  info: 5
  total: 14
status: issues_found
---

# Phase 4: Code Review Report

**Reviewed:** 2026-07-04
**Depth:** standard
**Files Reviewed:** 41 (ไฟล์ Go ของ donnarec-api และ TypeScript/React ของ donnarec-web ที่เกี่ยวข้องกับ Receipt PDF + Email Delivery + Outbox Worker)
**Status:** issues_found

## Summary

ตรวจสอบโค้ด Phase 4 (Receipt PDF rendering, sandboxed Chromium, outbox worker, email delivery, admin settings config store + preview UI) ทั้งฝั่ง Go backend และ Next.js frontend อย่างละเอียด โดยเน้นตรวจ 3 คุณสมบัติหลักตาม CLAUDE.md: (1) sandbox การ render PDF (JS-disable/network-block/autoescape), (2) freeze-idempotency ของ outbox worker ที่ต้องไม่ re-render/re-number, (3) RBAC/SoD ของ resend, download, settings

**จุดที่ทำได้ดี:** contextual autoescaping ของ `html/template` ถูกใช้ถูกต้อง (ไม่มีการ wrap เป็น `template.HTML`), sandbox 3 ชั้น (JS-disable + CDP fetch-block + document-content-injection แทน navigate) ถูก implement และมี regression test คลุม, freeze-idempotency ของ resend ถูกป้องกันด้วย guard `ErrReceiptNotReady` อย่างถูกต้อง (resend เรียกได้เฉพาะเมื่อ PDF ถูก freeze แล้ว ทำให้ไม่มี race ที่จะ render ซ้ำ), RBAC ของ resend/download/settings ใช้ `RequireAnyRole`/`RequireRoles` (OR semantics) ไม่มีการกลับไปทำ bug แบบ AND ของ Phase 3, brand-image upload ใช้ magic-byte validation (`mimetype.Detect`) ไม่ใช่ extension/MIME header, และ preview endpoint ใช้ sample data คงที่ (ไม่มี donation id/donor field เข้าไปใน request) ตาม D-61

พบปัญหาสำคัญ 2 จุดที่เป็น **BLOCKER** เกี่ยวกับความน่าเชื่อถือของ pipeline การส่งใบเสร็จ (ไม่มีกลไก reclaim job ที่ค้าง + ไม่มี panic recovery ใน worker goroutine ซึ่งกระทบทั้งโปรเซส) และพบปัญหาระดับ WARNING อีก 7 จุด ส่วนใหญ่เกี่ยวกับช่องว่างระหว่าง "สิ่งที่ comment อ้างว่าทำ" กับ "สิ่งที่ config จริงทำ" (เช่น network isolation ของ chrome sidecar) และ robustness ของ render/preview path

## Critical Issues

### CR-01: Outbox job ที่ค้างสถานะ `processing` ไม่มีทางถูก reclaim — ใบเสร็จบางใบอาจไม่มีวันถูกส่งอีเมลได้เลย

**File:** `donnarec-api/internal/db/queries/outbox.sql:27-39` (ClaimNextOutboxJob), `donnarec-api/internal/worker/worker.go:157-198` (ProcessOnce)
**Issue:**
`ClaimNextOutboxJob` เปลี่ยน status ของ job เป็น `'processing'` ทันทีที่ claim สำเร็จ แต่ query เดียวกันนี้เองที่ใช้ claim งานถัดไปกลับกรองด้วย `WHERE j.status IN ('pending', 'failed')` — ไม่มี `'processing'` อยู่ในเงื่อนไขนี้เลย และในทั้ง `outbox.sql`/`worker.go` ไม่มี query หรือ mechanism ใดที่จะ "reclaim" job ที่ค้างอยู่ที่สถานะ `processing` (เช่น timeout-based reset)

ถ้า process ของ worker ถูก restart แบบไม่ graceful (panic — ดู CR-02, `docker compose down`/rolling restart ของ container, OOM-kill, หรือ pod eviction บน k8s) ระหว่างที่ job ถูก claim ไปแล้วแต่ยังไม่ทันเรียก `MarkOutboxJobDone`/`MarkOutboxJobFailed` job แถวนั้นจะค้างสถานะ `processing` ตลอดไป — ไม่มี auto-retry, ไม่มี dead-letter, และเจ้าหน้าที่ (staff) ก็ไม่มีทางกดปุ่ม resend ผ่าน UI ได้ด้วยซ้ำ เพราะ `DonationService.Resend` (`internal/donation/service.go:1237`) จะ block ด้วย `ErrReceiptNotReady` ถ้า `receipt_pdf_object_key` ยังเป็น null — ซึ่งจะเป็น null เสมอถ้า process ถูกฆ่าตายก่อน render เสร็จรอบแรก

ผลคือ: บริจาครายนั้นจะไม่มีวันได้รับใบเสร็จทางอีเมล และไม่มี path ใดๆ (อัตโนมัติหรือ manual) ที่จะกู้คืนสถานะนี้ได้เลยนอกจากแอดมินไป UPDATE ตาราง `outbox_jobs` เองตรงๆ ผ่าน DB — ขัดกับ FR-27/28 ที่ต้องมีทั้ง auto-retry และ manual resend fallback เสมอ

**Fix:** เพิ่มกลไก reclaim job ที่ค้างสถานะ `processing` เกิน timeout ที่กำหนด (เช่น เทียบ `updated_at` กับ now() ลบ threshold) กลับไปเป็น `pending` เพื่อให้ claim ใหม่ได้ เช่น
```sql
-- name: ReclaimStuckOutboxJobs :exec
UPDATE outbox_jobs
SET status = 'pending', updated_at = now()
WHERE status = 'processing'
  AND updated_at < now() - INTERVAL '10 minutes';
```
เรียกใน `Worker.Run`'s ticker loop (หรือ cron แยก) ก่อน `ProcessOnce` ทุกครั้ง หรืออย่างน้อยที่สุดเปิด endpoint/manual query ให้แอดมิน trigger ได้โดยไม่ต้องแตะ DB ตรงๆ

---

### CR-02: Outbox worker goroutine ไม่มี panic recovery — panic ระหว่างประมวลผล 1 job จะทำให้ทั้งโปรเซส donnarec-api ตายทั้งตัว (รวม HTTP API)

**File:** `donnarec-api/internal/worker/worker.go:130-144` (Run), `donnarec-api/cmd/server/main.go:251` (`go outboxWorker.Run(ctx)`)
**Issue:**
HTTP path มี `gin.Recovery()` middleware (`cmd/server/main.go:281`) ที่ดักจับ panic ต่อ-request ไม่ให้ล้มทั้งโปรเซส แต่ outbox worker ถูกเรียกด้วย `go outboxWorker.Run(ctx)` เฉยๆ โดยไม่มี `recover()` ใดๆ ห่ออยู่เลย ทั้งใน `Run`, `ProcessOnce`, หรือ `handleIssueReceipt`

ตรวจสอบยืนยันด้วย `grep -n "recover()"` ทั่วทั้ง `internal/worker/` และ `cmd/server/` แล้วไม่พบการเรียก `recover()` เลยสักที่ pipeline นี้เรียก `chromedp`/`cdproto` (external process ผ่าน CDP protocol), `mimetype.Detect`, และ `html/template.Execute` ต่อ 1 job ซึ่งล้วนเป็นจุดที่ third-party library อาจ panic ได้จาก edge case (nil deref, index out of range) — เมื่อเกิด panic ที่ไม่ถูก recover ใน goroutine ใดๆ ของ Go runtime ทั้งโปรเซสจะ terminate ทันที (ไม่ใช่แค่ goroutine เดียว) ซึ่งหมายความว่า **JSON API ทั้งระบบจะล่มไปด้วย** จาก error ของการ render ใบเสร็จเพียง 1 ใบ — เป็นความเสี่ยง availability ที่รุนแรงสำหรับระบบที่ CLAUDE.md ระบุว่าเป็น compliance-critical

**Fix:** ห่อ loop body ของ `Run` (หรือทุกครั้งที่เรียก `ProcessOnce`) ด้วย deferred recover ที่ log แล้วให้ worker ทำงานต่อ:
```go
func (w *Worker) Run(ctx context.Context) {
    ticker := time.NewTicker(w.cfg.PollInterval)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            w.safeProcessOnce(ctx)
        }
    }
}

func (w *Worker) safeProcessOnce(ctx context.Context) {
    defer func() {
        if r := recover(); r != nil {
            w.logger.Error("worker: recovered from panic", zap.Any("panic", r))
        }
    }()
    if err := w.ProcessOnce(ctx); err != nil && !errors.Is(err, ErrNoJob) {
        w.logger.Error("worker: process tick failed", zap.Error(err))
    }
}
```

---

## Warnings

### WR-01: การอ้างว่า chrome sidecar "network-isolated" (D-58 Layer 1) ไม่ตรงกับ docker-compose config จริง

**File:** `donnarec-api/docker-compose.yml:156-162`, `donnarec-api/docker/chrome.Dockerfile:1-9`, `donnarec-api/internal/pdf/chromium.go:5-11`
**Issue:** comment ในทั้ง 3 ไฟล์ยืนยันตรงกันว่า "Layer 1 ของ defense-in-depth คือ network-isolated chrome sidecar — ไม่มี outbound network route เลย" แต่ service `chrome` ใน `docker-compose.yml` ไม่มีการประกาศ network แบบ `internal: true` เลยสักที่ — สิ่งที่ทำจริงคือแค่ "ไม่มี `ports:` mapping" ซึ่งบล็อกแค่การเข้าถึงจาก host เข้า container (inbound) เท่านั้น ไม่ได้บล็อก container ออกไปหา internet (outbound/egress) เลย บน Docker default bridge network container ยังสามารถ resolve DNS และเชื่อมต่อออกอินเทอร์เน็ตได้ตามปกติ ถ้า Layer 2 (CDP `fetch.FailRequest` แบบ catch-all ใน `chromium.go`) มี bug/race แม้แต่จุดเดียว การป้องกัน SSRF ที่ comment อ้างว่ามีอยู่ 3 ชั้นจริงๆ จะเหลือแค่ 2 ชั้น ไม่ใช่ 3 ชั้นตามที่เอกสารอ้าง
**Fix:** เพิ่ม top-level `networks:` block พร้อม `internal: true` แล้วผูก service `chrome` เข้ากับ network นั้นแทน `default` เพื่อให้ "network-isolated" เป็นจริงตามที่ comment อ้าง เช่น
```yaml
networks:
  chrome-internal:
    internal: true
services:
  chrome:
    networks:
      - chrome-internal
```

### WR-02: `renderInSandbox`'s fetch-block handler กลืน error ทิ้งและสร้าง goroutine ที่ไม่ sync กับ action หลัก

**File:** `donnarec-api/internal/pdf/chromium.go:120-129`
**Issue:** ทุกครั้งที่มี request ถูก pause โดย CDP จะ spawn goroutine ใหม่เรียก `chromedp.Run(ctx, fetch.FailRequest(...))` แล้วทิ้ง error ด้วย `_ =` — ถ้า `FailRequest` fail (เช่น request ID ถูกจัดการไปแล้ว หรือ race กับ context ที่กำลังถูก cancel) จะไม่มีการ log ใดๆ เลย request ที่ pause ค้างจะไม่ถูกยกเลิกอย่างชัดเจน ทำให้การ render ทั้งหมดค้างจนกว่าจะครบ `renderTimeout` (30 วินาที) โดยไม่มีสัญญาณ diagnostic ใดๆ ว่าเกิดอะไรขึ้น การยิง `chromedp.Run` แบบ concurrent จากหลาย goroutine พร้อมกับ action sequence หลัก (ที่กำลังรอ `PrintToPDF`) บน browser context เดียวกันก็เป็นรูปแบบที่ chromedp เตือนว่าต้องระวังเรื่อง protocol interleaving
**Fix:** log error จาก `FailRequest` แทนการทิ้งเงียบๆ และพิจารณาใช้ executor ของ context ที่มีอยู่แล้วโดยตรง (`fetch.FailRequest(...).Do(cdp.WithExecutor(ctx, chromedp.FromContext(ctx).Target))`) แทนการเปิด `chromedp.Run` ใหม่ซ้อนกันทุกครั้งที่มี event เข้ามา — ตรงกับ pattern ที่ chromedp official examples ใช้

### WR-03: Config สำคัญของใบเสร็จ (deduction_multiplier/section6_text/template_html) ถูกอ่านสดจาก `receipt_template_config` ตอน "render ครั้งแรก" ไม่ใช่ตอน Approve — มีช่วงเวลาที่แอดมินแก้ config แล้วกระทบใบเสร็จที่อนุมัติไปแล้วแต่ยังไม่ถูก render

**File:** `donnarec-api/internal/worker/issue_receipt.go:171-223`
**Issue:** ตามที่ระบุใน design comment เอง ("deduction_multiplier is a single hospital-wide config value, read fresh ... only relevant the FIRST time a receipt renders") — ถ้าแอดมินไปแก้ template/ข้อความ §6/ตัวคูณลดหย่อนใน settings ระหว่างช่วงเวลาสั้นๆ ที่ receipt ถูก Approve ไปแล้วแต่ outbox worker ยังไม่ทัน process (poll interval ปกติ 5 วินาที แต่ backlog อาจนานกว่านั้นถ้ามี job คั่งค้าง) ใบเสร็จที่ถูก render ออกมาจะใช้ค่า config ใหม่ ไม่ใช่ค่าที่มีผล ณ ตอน checker กด approve — ซึ่งขัดกับหลักการ "frozen at approval" ที่ระบบใช้กับฟิลด์อื่นๆ ทั้งหมด (receipt number, donor snapshot) และมีนัยด้านกฎหมายภาษี (มาตรา 1x/2x) ตามที่ CLAUDE.md เน้นย้ำว่าต้องถูกต้องตามประมวลรัษฎากร
**Fix:** พิจารณา snapshot ค่าที่จำเป็น (deduction_multiplier อย่างน้อย) ไว้ที่ตอน Approve เป็นส่วนหนึ่งของ donation row หรือ outbox payload แทนที่จะให้ worker ไปอ่านค่าปัจจุบันตอน render — หรือถ้ายอมรับความเสี่ยงนี้ได้ ให้บันทึกเป็น known-limitation ในเอกสารอย่างชัดเจนแทนที่จะเขียนแค่ใน comment เดียว

### WR-04: `TemplateLivePreview`'s debounced fetch ไม่มีการป้องกัน response ที่มาไม่เรียงลำดับ (stale response overwrite)

**File:** `donnarec-web/components/TemplateEditor.tsx:164-190`
**Issue:** `debouncedFetchRef.current?.(...)` เรียก `fetchPreviewHTML` แล้ว `.then(result => setHtml(result))` โดยไม่มี request-id/AbortController guard ใดๆ ถ้าผู้ใช้แก้ template แล้วสลับภาษา (หรือแก้อีกครั้ง) ห่างกันเกิน 400ms (เกินช่วง debounce เดิม) จะมี fetch 2 คำขอ in-flight พร้อมกัน และถ้า response ของคำขอที่เก่ากว่ากลับมาช้ากว่า (network ไม่รับประกันลำดับ) จะเขียนทับ preview ที่ใหม่กว่าด้วยข้อมูลเก่า — แสดงพรีวิวผิดโดยที่ผู้ใช้ไม่รู้ตัว
**Fix:** ใช้ `AbortController`ต่อ fetch หรือเก็บ request sequence number แล้ว ignore response ที่ sequence ไม่ตรงกับ request ล่าสุด เช่น
```ts
const reqIdRef = useRef(0);
// ในตัว debounced callback:
const myId = ++reqIdRef.current;
fetchPreviewHTML(req).then((result) => {
  if (myId === reqIdRef.current) setHtml(result);
});
```

### WR-05: การ resurrect job ที่ dead-letter แล้วถ้าปรับ `WORKER_MAX_ATTEMPTS` เพิ่มขึ้นภายหลัง

**File:** `donnarec-api/internal/db/queries/outbox.sql:27-39` (ClaimNextOutboxJob), `56` (MarkOutboxJobFailed)
**Issue:** `ClaimNextOutboxJob`'s WHERE clause รวม `status IN ('pending', 'failed')` เข้าด้วยกัน — ตัวกันไม่ให้ job ที่ dead-letter (`status='failed'`) ถูก claim ซ้ำมีแค่เงื่อนไข `attempts < @max_attempts` เท่านั้น ถ้า operator ปรับ env `WORKER_MAX_ATTEMPTS` ให้มากขึ้นหลังจาก job บางตัว dead-letter ไปแล้วที่ limit เดิม job เหล่านั้นจะ "ฟื้นคืนชีพ" กลับมา claim ได้อีกครั้งโดยไม่มีการแจ้งเตือนใดๆ ว่านี่คือผลข้างเคียงของการเปลี่ยน config — ไม่ตรงกับ comment ที่ระบุว่า `'failed'` คือ terminal state ("no further auto-retry")
**Fix:** ถ้าต้องการให้ `'failed'` เป็น terminal จริง ให้ตัด `'failed'` ออกจาก claim query's `IN (...)` แล้วให้ resend (ที่ enqueue job ใหม่) เป็นทางเดียวที่จะ "รีไทร" job ที่ dead-letter ไปแล้ว — ไม่ผูกกับ config ที่เปลี่ยนได้

### WR-06: การ fetch รูป branding (letterhead/seal/signature/watermark) ที่ล้มเหลวชั่วคราวทำให้การ render ใบเสร็จทั้งใบ (ซึ่งเป็น critical path) ล้มเหลวไปด้วย

**File:** `donnarec-api/internal/worker/issue_receipt.go:184-199` (renderReceiptPDF), `243-253` (fetchTemplateImage)
**Issue:** `renderReceiptPDF` เรียก `fetchTemplateImage` 4 ครั้งแบบ synchronous และ return error ทันทีถ้าตัวใดตัวหนึ่ง fail (`internal/worker/issue_receipt.go:185-198`) — ถ้า MinIO มีปัญหาเชื่อมต่อชั่วคราวตอนดึงแค่รูป watermark (ซึ่งเป็น decorative element ไม่ใช่เนื้อหาที่จำเป็นทางกฎหมาย) การ render ใบเสร็จทั้งใบจะ fail และถูกจัดคิว retry/backoff ทั้งที่เนื้อหาหลัก (เลขที่ใบเสร็จ, จำนวนเงิน, ชื่อผู้บริจาค) พร้อมแล้ว — ทำให้การส่งใบเสร็จล่าช้าโดยไม่จำเป็นจากปัญหาที่ไม่เกี่ยวกับความถูกต้องของเอกสาร
**Fix:** พิจารณาให้ `fetchTemplateImage` fail แบบ soft (log warning + คืนค่าว่าง) สำหรับรูป branding ที่ไม่ critical แทนที่จะทำให้ทั้ง pipeline fail — หรืออย่างน้อยแยก error ประเภทนี้ออกจาก error ประเภทอื่นเพื่อไม่ให้กิน backoff attempts ของ critical path

### WR-07: `SaveSettings`/`SaveTemplateImage` ไม่ได้ทำเป็น transaction เดียว — เขียน 2 ตาราง (template config + number format config) แยกกันโดยไม่มี rollback ร่วม

**File:** `donnarec-api/internal/settings/service.go:145-171` (SaveSettings)
**Issue:** `SaveSettings` เรียก `UpdateReceiptTemplateConfig` แล้วตามด้วย `UpdateReceiptNumberConfig` เป็น 2 statement แยกกันนอก transaction — ถ้า statement แรกสำเร็จแต่ statement ที่สอง fail (เช่น DB connection ขาดกลางคัน) จะเกิด partial-save ที่ comment ของ `SaveSettings` เองบอกว่าต้องไม่เกิดขึ้น ("no partial save") แต่โค้ดจริงไม่มี `dbhelpers.WithTx` ห่อการเขียนทั้งสองไว้เลย
**Fix:** ห่อทั้งสอง query ด้วย `dbhelpers.WithTx` เดียวกันเหมือนที่ `internal/donation/service.go` ทำกับทุก mutation ที่ต้องเป็น atomic

## Info

### IN-01: Duplicated numeric-formatting helper ระหว่าง 2 package

**File:** `donnarec-api/internal/worker/issue_receipt.go:311-342` (formatAmount) ซ้ำกับ `donnarec-api/internal/donation/service.go:1613-1645` (numericStr)
**Issue:** ทั้งสองฟังก์ชันแปลง `pgtype.Numeric` เป็น string ด้วย logic แทบเหมือนกันทุกตัวอักษร — comment ใน `issue_receipt.go` ยอมรับเองว่า "duplicated here rather than exported cross-package"
**Fix:** พิจารณา export ฟังก์ชันร่วมจาก package กลาง (เช่น `internal/db` helpers) เพื่อลดความเสี่ยงที่ 2 จุดจะ drift ออกจากกันในอนาคต

### IN-02: `email_delivery` table ถูก grant สิทธิ์ UPDATE ทั้งที่ไม่มี query ใดใช้

**File:** `donnarec-api/migrations/000010_email_delivery.up.sql:47`
**Issue:** `GRANT SELECT, INSERT, UPDATE ON email_delivery TO donnarec_app;` แต่ตรวจสอบ `internal/db/queries/email_delivery.sql` แล้วมีแค่ `InsertEmailDelivery` (INSERT) และ `GetLatestEmailDeliveryForDonation` (SELECT) เท่านั้น — ไม่มี query ใดที่ UPDATE ตารางนี้เลย (ตั้งใจให้เป็น append-only ตาม design comment)
**Fix:** ตัด `UPDATE` ออกจาก GRANT เพื่อให้ตรงกับหลัก least-privilege และสอดคล้องกับเจตนา "one row per attempt, never overwrite"

### IN-03: `PreviewRequest.DeductionMultiplier` ไม่มี validation `oneof=1x 2x` ต่างจาก `ReceiptSettings.DeductionMultiplier`

**File:** `donnarec-api/internal/settings/model.go:39` (ReceiptSettings, มี `validate:"required,oneof=1x 2x"`) เทียบกับ `model.go:70` (PreviewRequest, ไม่มี validate tag)
**Issue:** ไม่มีผลด้าน security เพราะ `html/template` ยัง autoescape ค่านี้อยู่ดี แต่เป็นความไม่สอดคล้องกันของ validation rule ระหว่าง struct ที่ควรจะมี schema เดียวกัน อาจทำให้ preview แสดงค่าที่ save จริงไม่ผ่าน validation
**Fix:** เพิ่ม `validate:"omitempty,oneof=1x 2x"` ให้ตรงกัน

### IN-04: `NumberFormatEditor` ยอมให้พิมพ์ `running_no_padding` เป็น 0 หรือค่าติดลบได้ฝั่ง client โดยไม่เตือนจนกว่าจะ save แล้วเจอ error message ที่สื่อผิดสาเหตุ

**File:** `donnarec-web/components/NumberFormatEditor.tsx:85-88`, `donnarec-web/components/SettingsTabs.tsx:132-140` (onError)
**Issue:** `onRunningNoPaddingChange(Number.isNaN(parsed) ? 1 : parsed)` ไม่กันค่า 0/ติดลบตอน onChange (ฝั่ง Go จะ reject ด้วย `validate:"min=1"` ตอน 422 อยู่แล้ว) แต่ `SettingsTabs`'s `saveMutation.onError` แสดง toast ว่า "เทมเพลตมีข้อผิดพลาด" (`saveValidationError`) แม้ว่าสาเหตุจริงจะมาจาก tab เลขที่ใบเสร็จ ไม่ใช่ template — ผู้ใช้จะงงว่าต้องแก้ tab ไหน
**Fix:** เพิ่ม client-side `min(1)` guard ที่ input และ/หรือแยกข้อความ error ตามฟิลด์ที่ backend ระบุจริง

### IN-05: `withPreviewFontFace` match เฉพาะ `<head>` แบบไม่มี attribute — เทมเพลตที่มี `<head lang="...">` จะได้ style block ถูกแปะไว้ก่อน `<!DOCTYPE html>`

**File:** `donnarec-web/components/TemplateEditor.tsx:116-126`
**Issue:** `html.toLowerCase().indexOf("<head>")` หา literal string `<head>` เท่านั้น ถ้า admin เขียน head tag ที่มี attribute (เช่น `<head lang="th">`) จะไม่ match แล้ว fallback ไป prepend `PREVIEW_FONT_FACE` ไว้หน้าสุดของเอกสาร (ก่อน `<!DOCTYPE html>`) ซึ่งเป็นตำแหน่งที่ผิดหลัก HTML (แม้ browser จะ tolerant ก็ตาม) — กระทบแค่ preview iframe เท่านั้น ไม่กระทบ production PDF (ซึ่งไม่ผ่านฟังก์ชันนี้)
**Fix:** ใช้ regex ที่ทนต่อ attribute เช่น `/<head[^>]*>/i` แทน literal string match

---

_Reviewed: 2026-07-04_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
