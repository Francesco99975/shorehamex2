-- Reverse: drop hash, keep encrypted-only (previous state)

DROP INDEX IF EXISTS idx_remote_access_tokens_token_hash;

ALTER TABLE remote_access_tokens
    DROP COLUMN IF EXISTS token_hash;
