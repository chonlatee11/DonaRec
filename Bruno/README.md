# DonaRec — Bruno API Tests

Bruno collection สำหรับทดสอบ RBAC ของ `donnarec-api` (Phase 01 — UAT Test 3:
Admin-only user creation) แบบยิง API จริงพร้อม assertion อัตโนมัติ

## ครอบคลุมอะไร

| Request | Token | คาดหวัง |
|---|---|---|
| Health / Health Check | — | 200 `{"status":"ok"}` |
| Users / Me - admin | admin | 200, roles มี `admin` |
| Users / Create User - admin 201 | admin | **201** (happy path) |
| Users / Create User - maker 403 | maker | **403** (RBAC บล็อก) |
| Users / Create User - checker 403 | checker | **403** (RBAC บล็อก) |
| Users / Create User - no token 401 | — | **401** |
| Users / Create User - invalid body 400 | admin | **400** (JSON ผิดรูป) |
| Users / Create User - missing fields 422 | admin | **422** (validation ล้ม) |

## วิธีใช้

### 1. boot stack + รัน migration
```bash
cd donnarec-api
docker compose up -d --wait

# ⚠️ จำเป็น: docker compose ไม่รัน migration ให้ — app DB จะว่างเปล่า
# ต้องรันเองก่อน ไม่งั้น POST /api/admin/users จะ error (ไม่มีตาราง users/audit_log)
export DATABASE_URL="postgres://donnarec:<DB_PASSWORD>@localhost:5432/donnarec_app?sslmode=disable"
make migrate-up
```

### 2. เตรียม test client + test users ใน Keycloak (ครั้งเดียว, idempotent)
```bash
bash Bruno/setup-test-users.sh
```
สคริปต์จะสร้าง:
- public client `donnarec-test-cli` (เปิด direct access grants + audience mapper → `donnarec-backend`)
- user 3 ราย (password `TestPass123`): `admin-test` / `maker-test` / `checker-test`

> ไม่แตะ `keycloak/realm-donnarec.json` (production import) — เป็นการตั้งค่า local เท่านั้น

### 3. รัน

**ใน Bruno app:** เปิด collection `Bruno/DonaRec/` → เลือก environment `local`
→ รันโฟลเดอร์ **Auth** ก่อน (ดึง token เก็บใน runtime vars) แล้วรัน **Health** / **Users**

**CLI (bru):**
```bash
npm i -g @usebruno/cli
cd Bruno/DonaRec
bru run Auth   --env local   # ดึง token ก่อน
bru run Health --env local
bru run Users  --env local
```

## หมายเหตุทางเทคนิค
- middleware ตรวจ `aud` ของ token ต้องมี `donnarec-backend` → ใช้ audience mapper บน test client
- request `Create User - admin 201` สุ่ม email/keycloak_subject ต่อรอบ กัน unique-constraint conflict
- ปรับ host/port/password ได้ที่ `environments/local.bru`

### ⚠️ Issuer workaround (สำคัญ)
API ตรวจ `iss` ของ token ต้องเท่ากับ `http://keycloak:8080/realms/donnarec` (ค่าจาก
`KEYCLOAK_BASE_URL` ในคอนเทนเนอร์) แต่ Keycloak ตั้ง `KC_HOSTNAME_STRICT=false` → iss จะอิง
Host ของ request ที่เข้ามา ถ้าขอ token ผ่าน `localhost:8080` ตรงๆ จะได้ `iss=localhost:8080`
ซึ่ง **API จะ reject เป็น 401**

collection นี้จึงส่ง header `Host: keycloak:8080` (env var `issuerHost`) ในทุก token request
เพื่อบังคับให้ iss ออกมาเป็น `keycloak:8080` ตรงกับที่ API คาดหวัง

> นี่เป็น workaround — root cause คือ config mismatch ระหว่าง browser-facing URL (localhost)
> กับ backend-facing URL (keycloak) ดูหัวข้อ "Known issues" ด้านล่าง

## Known issues (พบระหว่างสร้าง collection — ควรแก้ที่ phase 01)
1. **Migration ไม่ auto-run ใน docker-compose** — ต้อง `make migrate-up` เองหลัง `up`
   ไม่งั้น live DB ว่างเปล่าและ user-creation/audit ใช้ไม่ได้ (`relation "..." does not exist`)
2. **OIDC issuer mismatch** — token ที่ออกผ่าน localhost (เช่นที่ frontend จริงจะใช้ใน Phase 6)
   มี `iss=localhost:8080` แต่ API คาดหวัง `keycloak:8080` → ถูก reject 401 ทุกตัว
   (collection เลี่ยงด้วย Host header แต่ frontend จริงจะเจอปัญหานี้)
