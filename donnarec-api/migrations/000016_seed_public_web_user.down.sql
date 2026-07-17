-- migrations/000016_seed_public_web_user.down.sql
-- Reverse 000016: delete the seeded public-web system user by its fixed UUID.
-- user_roles row is removed automatically via ON DELETE CASCADE (000001).

DELETE FROM users
WHERE id = '00000000-0000-4000-8000-000000000006';
