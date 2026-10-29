-- 000001_create_users.down.sql - rollback for the reference schema.
DROP TRIGGER IF EXISTS trg_users_bump_updated_at ON users;
DROP FUNCTION IF EXISTS bump_updated_at();
DROP INDEX IF EXISTS idx_users_created_at;
DROP INDEX IF EXISTS idx_users_status;
DROP INDEX IF EXISTS idx_users_is_deleted;
DROP TABLE IF EXISTS users;
