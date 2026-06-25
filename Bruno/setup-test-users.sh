#!/bin/bash
# setup-test-users.sh
# ----------------------------------------------------------------------------
# เตรียม Keycloak ให้ Bruno ใช้ทดสอบ RBAC ของ donnarec-api ได้แบบอัตโนมัติ
#
# สิ่งที่สคริปต์นี้ทำ (idempotent — รันซ้ำได้ปลอดภัย):
#   1. สร้าง public test client `donnarec-test-cli`
#        - directAccessGrantsEnabled=true  → Bruno ขอ token ด้วย grant_type=password ได้
#        - audience mapper → ใส่ `donnarec-backend` ลงใน aud ของ access token
#          (จำเป็น: middleware ตรวจ aud ต้องมี donnarec-backend ไม่งั้น 401)
#   2. สร้าง test user 3 ราย พร้อม realm role ตามชื่อ:
#        admin-test  (role: admin)
#        maker-test  (role: maker)
#        checker-test(role: checker)
#      ตั้ง password แบบถาวร (ผ่าน password policy: length(8)+upperCase+digits)
#
# ⚠️ ใช้เฉพาะ local dev เท่านั้น — ไม่แตะ keycloak/realm-donnarec.json (production import)
#
# Usage:
#   1. boot stack ก่อน: cd donnarec-api && docker compose up -d --wait
#   2. bash Bruno/setup-test-users.sh
#
# Env (อ่านจาก donnarec-api/.env อัตโนมัติ ถ้ามี):
#   KC_BASE_URL        Keycloak base URL (default: http://localhost:8080)
#   KC_ADMIN           Keycloak admin username (default: admin)
#   KC_ADMIN_PASSWORD  Keycloak admin password (REQUIRED)
#   KC_REALM           realm name (default: donnarec)
#   TEST_PASSWORD      password ของ test user ทุกราย (default: TestPass123)
# ----------------------------------------------------------------------------
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# ---- Load .env if present (same convention as scripts/seed-admin.sh) ----
if [ -f "$SCRIPT_DIR/../donnarec-api/.env" ]; then
    set -a
    # shellcheck disable=SC1091
    source "$SCRIPT_DIR/../donnarec-api/.env"
    set +a
fi

KC_BASE_URL="${KC_BASE_URL:-http://localhost:8080}"
KC_ADMIN="${KC_ADMIN:-admin}"
KC_ADMIN_PASSWORD="${KC_ADMIN_PASSWORD:?KC_ADMIN_PASSWORD is required (ตั้งใน donnarec-api/.env)}"
KC_REALM="${KC_REALM:-donnarec}"
TEST_PASSWORD="${TEST_PASSWORD:-TestPass123}"
TEST_CLIENT="donnarec-test-cli"
ADMIN_API="$KC_BASE_URL/admin/realms/$KC_REALM"

for bin in curl jq; do
    command -v "$bin" >/dev/null 2>&1 || { echo "ERROR: ต้องติดตั้ง '$bin' ก่อน"; exit 1; }
done

echo "=== DonaRec Bruno Test Setup ==="
echo "Keycloak : $KC_BASE_URL"
echo "Realm    : $KC_REALM"
echo "Client   : $TEST_CLIENT"
echo ""

# ---- 1. admin token (master realm / admin-cli) ----
echo "[1/3] ขอ Keycloak admin token..."
KC_TOKEN=$(curl -sS -f -X POST \
    "$KC_BASE_URL/realms/master/protocol/openid-connect/token" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    -d "grant_type=password" \
    -d "client_id=admin-cli" \
    -d "username=$KC_ADMIN" \
    -d "password=$KC_ADMIN_PASSWORD" \
    | jq -r '.access_token')

[ -n "$KC_TOKEN" ] && [ "$KC_TOKEN" != "null" ] || { echo "ERROR: ขอ admin token ไม่สำเร็จ"; exit 1; }
echo "  ได้ admin token แล้ว"

auth_hdr=( -H "Authorization: Bearer $KC_TOKEN" )

# ---- 2. test client (with audience mapper → donnarec-backend) ----
echo "[2/3] ตั้งค่า test client '$TEST_CLIENT'..."

CLIENT_BODY=$(jq -n --arg cid "$TEST_CLIENT" '{
    clientId: $cid,
    name: "DonaRec Test CLI (local API testing only)",
    description: "Public client พร้อม direct access grants + audience mapper สำหรับทดสอบผ่าน Bruno เท่านั้น",
    enabled: true,
    publicClient: true,
    bearerOnly: false,
    standardFlowEnabled: false,
    implicitFlowEnabled: false,
    directAccessGrantsEnabled: true,
    protocol: "openid-connect",
    protocolMappers: [{
        name: "donnarec-backend-audience",
        protocol: "openid-connect",
        protocolMapper: "oidc-audience-mapper",
        config: {
            "included.client.audience": "donnarec-backend",
            "id.token.claim": "false",
            "access.token.claim": "true"
        }
    }]
}')

EXISTING_CLIENT_ID=$(curl -sS -f -G "$ADMIN_API/clients" \
    --data-urlencode "clientId=$TEST_CLIENT" \
    "${auth_hdr[@]}" | jq -r '.[0].id // empty')

if [ -z "$EXISTING_CLIENT_ID" ]; then
    curl -sS -f -X POST "$ADMIN_API/clients" \
        "${auth_hdr[@]}" -H "Content-Type: application/json" \
        --data-binary "$CLIENT_BODY" >/dev/null
    echo "  สร้าง client ใหม่แล้ว"
else
    curl -sS -f -X PUT "$ADMIN_API/clients/$EXISTING_CLIENT_ID" \
        "${auth_hdr[@]}" -H "Content-Type: application/json" \
        --data-binary "$CLIENT_BODY" >/dev/null
    echo "  client มีอยู่แล้ว → อัปเดตค่าให้ตรง"
fi

# ---- 3. test users + role assignment ----
echo "[3/3] สร้าง test users..."

create_user_with_role() {
    # realm ตั้ง registrationEmailAsUsername=true → username ต้องเป็น email
    local username="$1" role="$2"

    local user_body
    user_body=$(jq -n \
        --arg u "$username" \
        --arg p "$TEST_PASSWORD" \
        --arg fn "$role" \
        '{
            username: $u,
            email: $u,
            firstName: $fn,
            lastName: "test",
            enabled: true,
            emailVerified: true,
            credentials: [{ type: "password", value: $p, temporary: false }]
        }')

    local code
    code=$(curl -sS -o /dev/null -w "%{http_code}" -X POST "$ADMIN_API/users" \
        "${auth_hdr[@]}" -H "Content-Type: application/json" \
        --data-binary "$user_body")

    if [ "$code" != "201" ] && [ "$code" != "409" ]; then
        echo "  ERROR: สร้าง user $username ไม่สำเร็จ (HTTP $code)"; exit 1
    fi

    # ดึง user id (exact match)
    local uid
    uid=$(curl -sS -f -G "$ADMIN_API/users" \
        --data-urlencode "username=$username" \
        --data-urlencode "exact=true" \
        "${auth_hdr[@]}" | jq -r '.[0].id // empty')
    [ -n "$uid" ] || { echo "  ERROR: หา user id ของ $username ไม่เจอ"; exit 1; }

    # ถ้า user มีอยู่แล้ว (409) ก็ reset password ให้ตรงกับ TEST_PASSWORD
    if [ "$code" = "409" ]; then
        curl -sS -f -X PUT "$ADMIN_API/users/$uid/reset-password" \
            "${auth_hdr[@]}" -H "Content-Type: application/json" \
            --data-binary "$(jq -n --arg p "$TEST_PASSWORD" '{type:"password", value:$p, temporary:false}')" >/dev/null
    fi

    # role id + assign
    local role_json
    role_json=$(curl -sS -f "$ADMIN_API/roles/$role" "${auth_hdr[@]}")
    local mapping
    mapping=$(echo "$role_json" | jq -c '[{id: .id, name: .name}]')
    curl -sS -f -X POST "$ADMIN_API/users/$uid/role-mappings/realm" \
        "${auth_hdr[@]}" -H "Content-Type: application/json" \
        --data-binary "$mapping" >/dev/null

    echo "  ✓ $username → role '$role'"
}

create_user_with_role "admin-test@donnarec.local"   "admin"
create_user_with_role "maker-test@donnarec.local"   "maker"
create_user_with_role "checker-test@donnarec.local" "checker"

echo ""
echo "=== เสร็จสิ้น ==="
echo "test users (password = $TEST_PASSWORD):"
echo "  admin-test@donnarec.local   / role admin"
echo "  maker-test@donnarec.local   / role maker"
echo "  checker-test@donnarec.local / role checker"
echo ""
echo "เปิด Bruno collection ที่โฟลเดอร์ Bruno/DonaRec/ แล้วเลือก environment 'local'"
echo "รันโฟลเดอร์ Auth ก่อน (ดึง token) จากนั้นรัน Users เพื่อ assert RBAC"
echo "อย่าลืม: ต้อง 'make migrate-up' กับ live DB ก่อน ไม่งั้น user-creation จะ error"
