-- =============================================================================
-- 000005_remote_token_hash_and_encrypted.up.sql
--
-- Restore token_hash for patient-facing lookup/auth, while keeping
-- token_encrypted so staff can recover and resend the same link.
--
-- Patient flow:  hash(presented_token) → WHERE token_hash = $1
-- Staff flow:    decrypt(token_encrypted) → show / re-email plaintext
-- =============================================================================

-- 1. Add hash column (nullable first so existing rows don't break)
ALTER TABLE remote_access_tokens
    ADD COLUMN IF NOT EXISTS token_hash TEXT;

-- 2. Ensure token_encrypted still exists (from 000003). If you never
--    applied encrypted-only and still have only hash, this is a no-op.
--    If the column is missing for any reason:
-- ALTER TABLE remote_access_tokens ADD COLUMN token_encrypted BYTEA;

-- 3. Unique index on hash (only one row per hash value)
CREATE UNIQUE INDEX IF NOT EXISTS idx_remote_access_tokens_token_hash
    ON remote_access_tokens (token_hash)
    WHERE token_hash IS NOT NULL;

-- 4. After backfill (app or one-off script), enforce NOT NULL.
--    With no production data you can set it immediately:
ALTER TABLE remote_access_tokens
    ALTER COLUMN token_hash SET NOT NULL;

-- Optional: keep token_encrypted NOT NULL as well
ALTER TABLE remote_access_tokens
    ALTER COLUMN token_encrypted SET NOT NULL;

COMMENT ON COLUMN remote_access_tokens.token_hash IS
    'SHA-256 (or equivalent) of the plaintext token. Used for patient-facing lookup. Never store plaintext.';

COMMENT ON COLUMN remote_access_tokens.token_encrypted IS
    'AES-GCM ciphertext of the plaintext token. Used so staff can recover/resend the same link. Key lives only in the app.';
