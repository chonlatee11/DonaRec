-- internal/db/queries/users.sql
-- sqlc-annotated queries for the users and user_roles tables.
-- All queries use explicit column lists (no bare * in INSERT).

-- name: CreateUser :one
INSERT INTO users (
    email,
    display_name,
    keycloak_subject,
    is_active,
    legal_hold,
    created_at,
    updated_at
) VALUES (
    @email,
    @display_name,
    @keycloak_subject,
    true,
    false,
    now(),
    now()
) RETURNING *;

-- name: GetUserByID :one
SELECT
    id, email, display_name, keycloak_subject,
    is_active, legal_hold, created_at, updated_at
FROM users
WHERE id = @id AND is_active = true;

-- name: GetUserByKeycloakSubject :one
SELECT
    id, email, display_name, keycloak_subject,
    is_active, legal_hold, created_at, updated_at
FROM users
WHERE keycloak_subject = @keycloak_subject AND is_active = true;

-- name: ListUsers :many
SELECT
    id, email, display_name, keycloak_subject,
    is_active, legal_hold, created_at, updated_at
FROM users
ORDER BY created_at DESC
LIMIT @limit_n OFFSET @offset_n;

-- name: AssignRole :one
INSERT INTO user_roles (user_id, role)
VALUES (@user_id, @role)
ON CONFLICT (user_id, role) DO NOTHING
RETURNING user_id, role;

-- name: ListRolesForUser :many
SELECT role
FROM user_roles
WHERE user_id = @user_id
ORDER BY role;

-- name: DeactivateUser :one
UPDATE users
SET is_active = false, updated_at = now()
WHERE id = @id
RETURNING id, email, is_active, updated_at;
