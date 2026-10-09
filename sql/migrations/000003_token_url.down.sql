-- Reverse of 000003 – restore hash-only design (data will be lost)

ALTER TABLE remote_access_tokens
    DROP COLUMN IF EXISTS token_encrypted;

ALTER TABLE remote_access_tokens
    ADD COLUMN token_hash TEXT UNIQUE NOT NULL;

-- Recreate the original active-token index
DROP INDEX IF EXISTS idx_remote_access_tokens_active;

CREATE UNIQUE INDEX idx_remote_access_tokens_active
    ON remote_access_tokens (assignment_id)
    WHERE used_at IS NULL AND NOT revoked;
