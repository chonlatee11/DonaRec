-- migrations/000001_init_schema.down.sql
-- Drops all Phase 1 foundation tables in reverse dependency order.

DROP TABLE IF EXISTS retention_config;
DROP TABLE IF EXISTS user_roles;
DROP TABLE IF EXISTS users;

DROP TYPE IF EXISTS user_role_enum;
DROP TYPE IF EXISTS legal_basis_enum;
