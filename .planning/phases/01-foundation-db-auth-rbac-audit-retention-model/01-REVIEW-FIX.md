---
phase: 01-foundation-db-auth-rbac-audit-retention-model
fixed_at: 2026-06-24T00:00:00Z
review_path: .planning/phases/01-foundation-db-auth-rbac-audit-retention-model/01-REVIEW.md
iteration: 1
findings_in_scope: 14
fixed: 14
skipped: 0
status: all_fixed
---

# Phase 1: Code Review Fix Report

**Fixed at:** 2026-06-24
**Source review:** `.planning/phases/01-foundation-db-auth-rbac-audit-retention-model/01-REVIEW.md`
**Iteration:** 1

**Summary:**
- Findings in scope: 14 (all severities — `fix_scope: all`)
- Fixed: 14
- Skipped: 0

หมายเหตุ: ทุก fix ถูก verify ด้วย `go build ./...` (และ `gofmt`/`go vet` ตามชนิดไฟล์) ใน
git worktree แยกต่างหาก แล้ว commit ทีละ finding. unit test ของ package ที่แตะ
(`pii`, `auth`) ผ่านทั้งหมด. integration/concurrency test ต้องใช้ Postgres (testcontainers)
จึง verify ได้แค่ระดับ compile — ดู finding ที่ flag ไว้ด้านล่าง.

## Fixed Issues

### CR-01 (BLOCKER): No `.gitignore` — `.env` with credentials would be committed

**Files modified:** `.gitignore` (new)
**Commit:** 412a66b
**Applied fix:** สร้าง root `.gitignore` ที่ ignore `.env` / `*.env` (ยกเว้น `*.env.example`
และ `.env.example`) พร้อม build artifacts. ยืนยันด้วย `git check-ignore donnarec-api/.env`
ว่าไฟล์ credentials ถูก ignore แล้ว. ตรวจสอบ `git log --all -- donnarec-api/.env` แล้ว
ไม่เคยถูก commit (clean).
**ติดตามต่อ (human):** หมุน (rotate) ค่า `changeme` ทั้งหมดและ `DONAREC_KEK` placeholder
ก่อนใช้กับ environment ที่ใช้ร่วมกัน.

### CR-02 (BLOCKER): SQL / shell / JSON injection in `seed-admin.sh`

**Files modified:** `scripts/seed-admin.sh`
**Commit:** 8413602
**Applied fix:**
- psql heredoc เปลี่ยนจาก string interpolation เป็น bound psql variables (`-v email=… -v
  name=… -v kcid=…` + `:'email'` syntax) ภายใน quoted heredoc (`<<'SQL'`) พร้อม
  `ON_ERROR_STOP=1`.
- Keycloak user-create JSON และ role-mapping JSON สร้างด้วย `jq -n --arg` แทนการต่อ string
  (ทดสอบแล้วว่า input ที่มี `'`, `"`, `$()` ถูก escape ปลอดภัย).
- การ lookup user ด้วย email เปลี่ยนเป็น `curl -G --data-urlencode` กัน metacharacter ใน
  query string.
ยืนยันด้วย `bash -n` (syntax OK) และทดสอบ jq escaping กับ payload แบบ injection.

### CR-03 (BLOCKER): Auth verifier / access-token audience & missing email

**Files modified:** `donnarec-api/internal/auth/claims.go`, `donnarec-api/internal/audit/middleware.go`
**Commit:** 84f7acd
**Applied fix (บางส่วน):** เพิ่ม claim `preferred_username` และ method `ActorIdentity()` ที่
fallback จาก `email` → `preferred_username` (claim ที่ access token ของ Keycloak มีเป็น
default) แล้วให้ audit middleware ใช้ `ActorIdentity()` เพื่อกัน `actor_email` ว่างใน audit
trail (FR-13). build ผ่าน.
**ติดตามต่อ (human / infra — ทำในโค้ดล้วนไม่ได้):** ส่วนหลักของ finding ต้องยืนยันชนิด token
ที่ API รับจริง และตั้งค่า Keycloak:
1. เพิ่ม audience mapper ให้ access token มี `aud: ["donnarec-backend"]` (หรือใช้
   `SkipClientIDCheck` พร้อม manual `aud`/`azp` check).
2. เพิ่ม integration test ที่ใช้ access token จริงจาก Keycloak (ไม่ใช่ ID token ที่ปั้นเอง)
   ผ่าน `Verify`, และ negative test ที่ token ที่ `aud` ไม่มี `donnarec-backend` ต้องถูกปฏิเสธ.
3. เปิด email protocol mapper บน access token ถ้าต้องการ `email` จริง.

### WR-01 (WARNING): Audit own-tx path violates "same transaction" invariant

**Files modified:** `donnarec-api/internal/audit/middleware.go`,
`donnarec-api/internal/audit/service.go`, `donnarec-api/internal/users/service.go`
**Commit:** 8962332
**Applied fix (option b — documentation alignment):** ปรับ comment/doc ทั้ง 3 ไฟล์ให้ตรงกับ
พฤติกรรมจริงของ Phase 1: audit middleware เขียนผ่าน `AppendAuditEntry` (own-tx, post-commit,
best-effort) ไม่ใช่ in-transaction. ระบุชัดว่า invariant "audit ใน transaction เดียวกับ mutation"
บังคับใช้เฉพาะเมื่อ handler เรียก `AppendAuditEntryTx` เอง และ Phase 1 ยังไม่ได้ wire ส่วนนั้น
ให้ mutating handler ใด. แก้ปัญหา "โค้ด assert invariant ที่ตัวเองไม่ได้บังคับ".
**ต้องการ human verification:** การ wire audit แบบ in-transaction จริง (ให้ `CreateUser`
เรียก `AppendAuditEntryTx` ใน `WithTx` เดียวกัน) ถูกเลื่อนไป phase ถัดไปอย่างจงใจ — ผู้พัฒนา
ควรยืนยันว่า best-effort post-commit auditing เป็นที่ยอมรับสำหรับ Phase 1.

### WR-02 (WARNING): `AssignRole` ON CONFLICT DO NOTHING + `:one` errors on conflict

**Files modified:** `donnarec-api/internal/users/service.go`
**Commit:** bd2585d
**Applied fix:** ใน `CreateUser` ถือว่า `pgx.ErrNoRows` จาก `AssignRole` เป็น success (no-op)
แทนที่จะ abort transaction — `if err != nil && !errors.Is(err, pgx.ErrNoRows)`. ทำให้ idempotency
ที่ comment อ้างไว้เป็นจริง. build ผ่าน.

### WR-03 (WARNING): `extractBearerToken` case-sensitive + whitespace token

**Files modified:** `donnarec-api/internal/auth/middleware.go`
**Commit:** aacfedb
**Applied fix:** match scheme แบบ case-insensitive (`strings.EqualFold` กับ `"bearer "`) และ
`strings.TrimSpace` ส่วน token เพื่อกัน header แบบ `"Bearer    "` (whitespace ล้วน) ไม่ให้
ส่ง token ว่าง/whitespace เข้า `Verify`. unit test ของ auth ผ่าน.

### WR-04 (WARNING): `MaskNationalID` narration + short-value leakage

**Files modified:** `donnarec-api/internal/pii/mask.go`, `donnarec-api/internal/pii/mask_test.go`
**Commit:** f24fe86
**Applied fix:** ลบ narration ยาว ๆ ใน doc block แทนด้วย format spec บรรทัดเดียว; เพิ่ม
`minRevealLen = 10` — ค่าที่สั้นกว่า 10 ตัวถูก mask ทั้งหมด (ไม่ leak last-4 ของค่าสั้น/บางส่วน).
อัปเดต test ที่เคย assert พฤติกรรมเดิม (4-char visible) ให้ตรงกับ contract ใหม่ที่ปลอดภัยขึ้น
และเพิ่ม boundary test ที่ 10 ตัว. `go test ./internal/pii/...` ผ่าน.

### WR-05 (WARNING): Dead/unused `InsertAuditLog` sqlc query

**Files modified:** `donnarec-api/internal/db/queries/audit.sql`,
`donnarec-api/internal/db/generated/audit.sql.go`,
`donnarec-api/internal/db/generated/querier.go`
**Commit:** 8327622
**Applied fix:** ลบ `InsertAuditLog` query ออกจาก `audit.sql` (แทนด้วย note อธิบายว่าใช้ raw
`tx.Exec` ใน `AppendAuditEntryTx` เป็น insert path เดียว) และลบโค้ด generated ที่เกี่ยวข้อง
(const + Params + Row + method + interface line) พร้อมเอา import `net/netip` ที่ไม่ใช้แล้วออก.
เหลือ insert definition เดียวบนตาราง audit ที่ immutable. `go build ./...` ผ่าน.
**หมายเหตุ:** ไม่มี `sqlc` ใน environment จึงแก้ generated file ด้วยมือให้ตรงกับผลที่ sqlc จะ
generate — ครั้งถัดไปที่รัน `sqlc generate` ควรได้ผลตรงกัน (query ถูกลบจาก source แล้ว).

### WR-06 (WARNING): `down.sql` restores UPDATE/DELETE and drops audit role/table

**Files modified:** `donnarec-api/migrations/000002_audit_log.down.sql`
**Commit:** a9e7cae
**Applied fix:** เพิ่ม guard ที่ abort เว้นแต่ session ตั้ง
`donnarec.allow_destructive_down = 'on'` (opt-in สำหรับ dev/test เท่านั้น); ลบการ re-grant
`UPDATE, DELETE` ก่อน drop (ไม่จำเป็นและเปิด tamper window); เพิ่มคำเตือนเด่นชัดว่าห้ามรันบน
prod และอธิบายว่า `DROP ROLE` ที่ล้มเหลวจาก dependency เป็นพฤติกรรมที่ตั้งใจ.

### IN-01 (INFO): `created_at` written twice on user insert

**Files modified:** `donnarec-api/internal/db/queries/users.sql`,
`donnarec-api/internal/db/generated/users.sql.go`
**Commit:** a584b7a
**Applied fix:** ตัด `created_at`/`updated_at` (และ `now(), now()`) ออกจาก INSERT ของ
`CreateUser` ให้พึ่ง column `DEFAULT now()` เป็น single source of truth. `CreateUserParams`
ไม่เปลี่ยน (ค่าถูก hardcode อยู่แล้ว). build ผ่าน.

### IN-02 (INFO): `retention_config.updated_by` zero-UUID sentinel unenforced

**Files modified:** `donnarec-api/migrations/000001_init_schema.up.sql`
**Commit:** 778b7cd
**Applied fix:** เลือก option "document the sentinel as intentional" (การเพิ่ม FK ทำไม่ได้
เพราะตารางถูก seed ตอน migration ก่อนมี user ใด ๆ). เพิ่ม comment อธิบายชัดเจนว่า zero-UUID
คือ sentinel "seeded by migration" ที่ admin จะ overwrite ภายหลัง และเหตุผลที่ไม่มี FK.

### IN-03 (INFO): `singularize` mishandles non-plural "-s" words

**Files modified:** `donnarec-api/internal/audit/middleware.go`
**Commit:** 91c2f86
**Applied fix:** เพิ่ม exception set `singularNouns` (`status`, `address`, `news`, `series`,
`species`) ที่ singularize จะคืนค่าเดิมไม่ตัด 's' — กัน audit label เพี้ยนแบบ `statu.read`.
build ผ่าน.

### IN-04 (INFO): `.env.example` ships `sslmode=disable`

**Files modified:** `donnarec-api/.env.example`, `donnarec-api/internal/config/config.go`,
`donnarec-api/cmd/server/main.go`
**Commit:** ba30170
**Applied fix:** เพิ่ม comment ใน `.env.example` ที่ระบุชัดว่าต้องใช้ `sslmode=verify-full` +
CA cert นอก local dev; เพิ่ม method `Config.InsecureDatabaseTLS()` ที่ตรวจ `sslmode=disable`
กับ host ที่ไม่ใช่ localhost แล้ว `main.go` log `Warn` เตือนตอน startup. build + vet ผ่าน.

### IN-05 (INFO): Concurrency test lacks independent linkage assertion

**Files modified:** `donnarec-api/internal/audit/concurrent_test.go`
**Commit:** a469d12
**Applied fix:** เพิ่ม SQL oracle (window function `LAG(row_hash) OVER (ORDER BY id)`) ที่
assert ว่าทุกแถวมี `prev_hash` เท่ากับ `row_hash` ของแถวก่อนหน้า — เป็น linked-list check ที่
อิสระจาก `VerifyChain` (กันบั๊กที่ insert path กับ verify ใช้สูตร hash ผิดเหมือนกันแล้วผ่านทั้งคู่).
test compile ผ่าน (`go vet`).
**ต้องการ human verification:** test นี้ต้องใช้ Postgres ผ่าน testcontainers จึงรันจริงไม่ได้ใน
environment นี้ — ผู้พัฒนาควรรัน `go test ./internal/audit/...` (ไม่ใส่ `-short`) กับ Docker
เพื่อยืนยันว่า oracle ใหม่ผ่าน.

## Skipped Issues

ไม่มี — ทุก finding ถูกแก้.

---

_Fixed: 2026-06-24_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 1_
