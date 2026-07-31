
-- Down migration: reverses 000001_initial.up.sql
-- Drop in reverse dependency order

-- Sessions (depends on users)
DROP TABLE IF EXISTS sessions;

-- Pending auth challenges (depends on users)
DROP TABLE IF EXISTS pending_auth_challenges;

-- 2FA backup codes (depends on users)
DROP TABLE IF EXISTS twofa_backup_codes;

-- Email verifications (depends on users)
DROP TABLE IF EXISTS email_verifications;

-- Password resets (depends on users)
DROP TABLE IF EXISTS password_resets;

-- Users (depends on roles)
DROP TABLE IF EXISTS users;

-- Roles
DROP TABLE IF EXISTS roles;

-- Functions used by the update-trigger machinery
DROP FUNCTION IF EXISTS apply_update_trigger(TEXT);
DROP FUNCTION IF EXISTS update_updated_at();

-- Extension enabled by the up migration
DROP EXTENSION IF EXISTS pg_trgm;
