-- migrations/000016_seed_public_web_user.up.sql
-- Phase 6: Flow B (public donation web form) — seeded system-actor user (D-76)
--
-- Design decisions realized here:
--   D-76: the public donation form has no authenticated Keycloak identity behind
--         it, but donations.created_by is NOT NULL REFERENCES users(id) and
--         audit_log rows need an actor. Seed exactly one fixed-identity system
--         user that plan 03's CreatePublicSubmission FKs against as created_by
--         AND uses (via its keycloak_subject) as the synthetic audit actor.
--
--   Pitfall 1 (06-RESEARCH.md): internal/audit/service.go's AppendAuditEntryTx
--         calls parseUUID(entry.ActorID) and ROLLS BACK THE WHOLE TRANSACTION
--         (including the just-created pending_review donation) if ActorID is
--         not UUID-shaped. A human-readable sentinel like the literal string
--         'public-web' would make EVERY public submission fail at the audit
--         step. So BOTH id and keycloak_subject below are set to the SAME
--         fixed, literal, valid UUID — never a readable string.
--
--   FIXED UUID (record this value — plan 03 mirrors it as a Go constant,
--   e.g. `const PublicWebUserID = "00000000-0000-4000-8000-000000000006"`):
--
--       00000000-0000-4000-8000-000000000006
--
--   T-06-03 (Elevation of Privilege, threat model): this user is seeded with
--         the least-privileged role ('maker') purely so that IF a JWT ever
--         presents this exact subject as its 'sub' claim (which should never
--         happen — no Keycloak credential is ever issued for this identity),
--         auth.ResolveAppUser resolves it to the lowest-privilege role rather
--         than an unassigned/blank role. It is never used to log in.
--
-- The application connects as role 'donnarec_app'.
-- This migration:
--   1. Inserts one system-user row into users with the fixed UUID above as
--      BOTH id and keycloak_subject (idempotent via ON CONFLICT (id) DO NOTHING).
--   2. Assigns it the least-privileged 'maker' role in user_roles (idempotent).
--   3. No new GRANT — INSERT on users/user_roles is already available to the
--      migration's connecting role; donnarec_app never inserts into users
--      directly (user provisioning is a separate admin/Keycloak-sync concern).

-- ============================================================
-- 1. Seed the public-web system user (fixed UUID, D-76, Pitfall 1)
-- ============================================================

INSERT INTO users (id, email, display_name, keycloak_subject, is_active, legal_hold)
VALUES (
    '00000000-0000-4000-8000-000000000006',
    'public-web-system@donarec.internal',
    'ระบบเว็บสาธารณะ (Public Web Submission)',
    '00000000-0000-4000-8000-000000000006',
    true,
    false
)
ON CONFLICT (id) DO NOTHING;

-- ============================================================
-- 2. Assign the least-privileged role (T-06-03)
-- ============================================================

INSERT INTO user_roles (user_id, role)
VALUES ('00000000-0000-4000-8000-000000000006', 'maker')
ON CONFLICT (user_id, role) DO NOTHING;
