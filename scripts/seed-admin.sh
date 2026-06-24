#!/bin/bash
# seed-admin.sh
# Creates the first admin user in Keycloak and mirrors the user into the app database.
#
# Decision D-05: Bootstrap admin via seed script / env (no setup wizard).
# Decision D-01: Admin-only user management — this script creates the initial admin.
#
# Usage:
#   1. Start the local dev stack: docker compose up -d
#   2. Wait for Keycloak to be healthy (check: curl http://localhost:8080/health/ready)
#   3. Run: bash scripts/seed-admin.sh
#
# Required environment variables (or .env file):
#   KC_BASE_URL      — Keycloak base URL (default: http://localhost:8080)
#   KC_ADMIN         — Keycloak admin username (default: admin)
#   KC_ADMIN_PASSWORD — Keycloak admin password (REQUIRED)
#   ADMIN_EMAIL      — New admin user email (REQUIRED)
#   ADMIN_PASSWORD   — New admin user password (REQUIRED, must meet policy)
#   ADMIN_DISPLAY_NAME — Display name for the admin user (default: "System Admin")
#   DATABASE_URL     — PostgreSQL connection string for the app DB (REQUIRED)

set -euo pipefail

# ---- Load .env if present ----
if [ -f "$(dirname "$0")/../donnarec-api/.env" ]; then
    set -a
    source "$(dirname "$0")/../donnarec-api/.env"
    set +a
fi

KC_BASE_URL="${KC_BASE_URL:-http://localhost:8080}"
KC_ADMIN="${KC_ADMIN:-admin}"
KC_ADMIN_PASSWORD="${KC_ADMIN_PASSWORD:?KC_ADMIN_PASSWORD is required}"
KC_REALM="${KC_REALM:-donnarec}"

ADMIN_EMAIL="${ADMIN_EMAIL:?ADMIN_EMAIL is required (e.g. admin@hospital.th)}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:?ADMIN_PASSWORD is required}"
ADMIN_DISPLAY_NAME="${ADMIN_DISPLAY_NAME:-System Admin}"

DATABASE_URL="${DATABASE_URL:?DATABASE_URL is required}"

echo "=== DonaRec Admin Seed Script ==="
echo "Keycloak: $KC_BASE_URL"
echo "Realm:    $KC_REALM"
echo "Email:    $ADMIN_EMAIL"
echo ""

# ---- 1. Get Keycloak admin token ----
echo "[1/4] Obtaining Keycloak admin token..."
KC_TOKEN=$(curl -sS -f -X POST \
    "$KC_BASE_URL/realms/master/protocol/openid-connect/token" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    -d "grant_type=password" \
    -d "client_id=admin-cli" \
    -d "username=$KC_ADMIN" \
    -d "password=$KC_ADMIN_PASSWORD" \
    | python3 -c "import sys,json; print(json.load(sys.stdin)['access_token'])")

if [ -z "$KC_TOKEN" ]; then
    echo "ERROR: Failed to obtain Keycloak admin token. Check KC_ADMIN/KC_ADMIN_PASSWORD."
    exit 1
fi
echo "  Admin token obtained."

# ---- 2. Create user in Keycloak ----
echo "[2/4] Creating admin user in Keycloak realm '$KC_REALM'..."
USER_CREATE_RESPONSE=$(curl -sS -w "\n%{http_code}" -X POST \
    "$KC_BASE_URL/admin/realms/$KC_REALM/users" \
    -H "Authorization: Bearer $KC_TOKEN" \
    -H "Content-Type: application/json" \
    -d "{
        \"username\": \"$ADMIN_EMAIL\",
        \"email\": \"$ADMIN_EMAIL\",
        \"firstName\": \"$ADMIN_DISPLAY_NAME\",
        \"lastName\": \"\",
        \"enabled\": true,
        \"emailVerified\": true,
        \"credentials\": [{
            \"type\": \"password\",
            \"value\": \"$ADMIN_PASSWORD\",
            \"temporary\": false
        }]
    }")

HTTP_CODE=$(echo "$USER_CREATE_RESPONSE" | tail -1)
if [ "$HTTP_CODE" != "201" ] && [ "$HTTP_CODE" != "409" ]; then
    echo "ERROR: Keycloak user creation failed (HTTP $HTTP_CODE)"
    echo "$USER_CREATE_RESPONSE"
    exit 1
fi

if [ "$HTTP_CODE" = "409" ]; then
    echo "  User already exists in Keycloak. Continuing..."
fi

# ---- 3. Get Keycloak user ID and assign admin role ----
echo "[3/4] Assigning 'admin' realm role..."
KC_USER_ID=$(curl -sS -f \
    "$KC_BASE_URL/admin/realms/$KC_REALM/users?username=$ADMIN_EMAIL&exact=true" \
    -H "Authorization: Bearer $KC_TOKEN" \
    | python3 -c "import sys,json; users=json.load(sys.stdin); print(users[0]['id']) if users else exit(1)")

# Get the admin role ID
ADMIN_ROLE_ID=$(curl -sS -f \
    "$KC_BASE_URL/admin/realms/$KC_REALM/roles/admin" \
    -H "Authorization: Bearer $KC_TOKEN" \
    | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")

# Assign the admin role
curl -sS -f -X POST \
    "$KC_BASE_URL/admin/realms/$KC_REALM/users/$KC_USER_ID/role-mappings/realm" \
    -H "Authorization: Bearer $KC_TOKEN" \
    -H "Content-Type: application/json" \
    -d "[{\"id\": \"$ADMIN_ROLE_ID\", \"name\": \"admin\"}]"

echo "  Admin role assigned. Keycloak user ID: $KC_USER_ID"

# ---- 4. Mirror user into app database ----
echo "[4/4] Mirroring admin user into app database..."

# Use psql to insert the user row and assign admin role
# Note: ON CONFLICT DO NOTHING makes this idempotent (safe to re-run)
psql "$DATABASE_URL" <<-SQL
    -- Insert admin user row (idempotent via ON CONFLICT)
    INSERT INTO users (email, display_name, keycloak_subject, is_active, legal_hold)
    VALUES ('$ADMIN_EMAIL', '$ADMIN_DISPLAY_NAME', '$KC_USER_ID', true, false)
    ON CONFLICT (keycloak_subject) DO UPDATE
        SET email = EXCLUDED.email,
            display_name = EXCLUDED.display_name,
            updated_at = now();

    -- Assign admin role in user_roles junction table (idempotent)
    INSERT INTO user_roles (user_id, role)
    SELECT id, 'admin'::user_role_enum
    FROM users
    WHERE keycloak_subject = '$KC_USER_ID'
    ON CONFLICT (user_id, role) DO NOTHING;

    -- Confirm result
    SELECT u.id, u.email, u.display_name, array_agg(ur.role) AS roles
    FROM users u
    JOIN user_roles ur ON ur.user_id = u.id
    WHERE u.keycloak_subject = '$KC_USER_ID'
    GROUP BY u.id, u.email, u.display_name;
SQL

echo ""
echo "=== Seed complete ==="
echo "Admin user '$ADMIN_EMAIL' created in Keycloak and mirrored to app DB."
echo ""
echo "Login at: $KC_BASE_URL/realms/$KC_REALM/account"
echo "Or use the API: POST $KC_BASE_URL/realms/$KC_REALM/protocol/openid-connect/token"
