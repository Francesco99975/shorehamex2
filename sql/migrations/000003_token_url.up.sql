-- =============================================================================
-- 000003_remote_token_encrypted.up.sql
--
-- Switch remote_access_tokens from hash-only to encrypted-token storage.
--
-- Goals:
--   • Store an encrypted version of the token so the application can later
--     recover and re-send the *same* link if the patient loses it.
--   • Remove the one-way hash (token_hash) because we no longer need it.
--   • Keep generation / encryption entirely in Go (no database trigger).
--   • Preserve the existing “only one active token per assignment” rule.
--
-- Security model:
--   The database holds only ciphertext.  The encryption key lives solely in
--   the application / secret manager.  A DB leak alone does not yield usable
--   remote links.
-- =============================================================================

-- ---------------------------------------------------------------------------
-- 1. Drop the old unique index that depended on token_hash
-- ---------------------------------------------------------------------------
DROP INDEX IF EXISTS idx_remote_access_tokens_token_hash;   -- if it existed
-- (the original schema only had UNIQUE on token_hash via the column constraint,
--  but we drop any leftover index just in case)

-- ---------------------------------------------------------------------------
-- 2. Drop the hash column
-- ---------------------------------------------------------------------------
ALTER TABLE remote_access_tokens
    DROP COLUMN IF EXISTS token_hash;

-- ---------------------------------------------------------------------------
-- 3. Add the encrypted-token column
--
-- BYTEA is the natural type for AES-GCM (or similar) ciphertext.
-- If you prefer to store base64-encoded ciphertext as TEXT you can change
-- this to TEXT, but BYTEA is cleaner and slightly more efficient.
-- ---------------------------------------------------------------------------
ALTER TABLE remote_access_tokens
    ADD COLUMN token_encrypted BYTEA NOT NULL;

-- ---------------------------------------------------------------------------
-- 4. Re-create the “only one active token per assignment” unique index
--    (the original one survived because it did not reference token_hash,
--     but we recreate it explicitly for clarity and to guarantee the
--     predicate is correct after the column change).
-- ---------------------------------------------------------------------------
DROP INDEX IF EXISTS idx_remote_access_tokens_active;

CREATE UNIQUE INDEX idx_remote_access_tokens_active
    ON remote_access_tokens (assignment_id)
    WHERE used_at IS NULL AND NOT revoked;

-- ---------------------------------------------------------------------------
-- 5. Helpful indexes for the new column / common access patterns
-- ---------------------------------------------------------------------------
-- Looking up a token by its ciphertext is never needed (the app always
-- looks up by assignment_id).  No extra index required on token_encrypted.

-- Keep the existing assignment_id index (already present from init schema):
--   CREATE INDEX idx_remote_access_tokens_assignment_id ...

-- ---------------------------------------------------------------------------
-- 6. Documentation comments (optional but useful for future readers)
-- ---------------------------------------------------------------------------
COMMENT ON COLUMN remote_access_tokens.token_encrypted IS
    'AES-GCM (or equivalent) ciphertext of the remote access token. '
    'Plaintext is never stored.  Decryption happens only in the application '
    'using a key held outside the database.';

COMMENT ON COLUMN remote_access_tokens.sent_to_email IS
    'Email address the link was last sent to.  Updated on every resend.';

COMMENT ON COLUMN remote_access_tokens.expires_at IS
    'Hard expiry.  Application must reject the token after this timestamp '
    'even if used_at is still NULL.';

COMMENT ON COLUMN remote_access_tokens.revoked IS
    'Set to TRUE when a newer token is issued for the same assignment '
    '(“Resend link” flow).  The partial unique index guarantees at most '
    'one non-revoked, unused token per assignment.';
