-- remote_access_tokens.sql
--
-- Dual storage:
--   token_hash      → patient auth (lookup by hash of presented token)
--   token_encrypted → staff recovery (decrypt to resend the same link)
--
-- Tokens belong to an examination (one active link per delivery unit).
-- Partial unique index: at most one active token per examination.

-- name: SendRemoteAccessToken :one
INSERT INTO remote_access_tokens (
    id,
    examination_id,
    token_hash,
    token_encrypted,
    sent_to_email,
    expires_at
)
SELECT
    sqlc.arg(id),
    e.id,
    sqlc.arg(token_hash),
    sqlc.arg(token_encrypted),
    sqlc.arg(sent_to_email),
    sqlc.arg(expires_at)
FROM examinations e
JOIN patients p ON p.id = e.patient_id
WHERE e.id = sqlc.arg(examination_id)
  AND (
      sqlc.narg(viewer_id)::uuid IS NULL
      OR (e.is_developer_data OR p.is_developer_data)
          = is_developer_user(sqlc.narg(viewer_id)::uuid)
  )
RETURNING *;

-- name: ResendRemoteAccessToken :one
-- Revoke active token (if any) and insert a new one (new hash + new ciphertext).
WITH revoked AS (
    UPDATE remote_access_tokens t
    SET revoked = TRUE
    FROM examinations e
    JOIN patients p ON p.id = e.patient_id
    WHERE t.examination_id = e.id
      AND t.examination_id = sqlc.arg(examination_id)
      AND t.used_at IS NULL
      AND NOT t.revoked
      AND (
          sqlc.narg(viewer_id)::uuid IS NULL
          OR (e.is_developer_data OR p.is_developer_data)
              = is_developer_user(sqlc.narg(viewer_id)::uuid)
      )
    RETURNING t.resend_count
)
INSERT INTO remote_access_tokens (
    id,
    examination_id,
    token_hash,
    token_encrypted,
    sent_to_email,
    expires_at,
    resend_count
)
SELECT
    sqlc.arg(id),
    e.id,
    sqlc.arg(token_hash),
    sqlc.arg(token_encrypted),
    sqlc.arg(sent_to_email),
    sqlc.arg(expires_at),
    COALESCE((SELECT resend_count FROM revoked), -1) + 1
FROM examinations e
JOIN patients p ON p.id = e.patient_id
WHERE e.id = sqlc.arg(examination_id)
  AND (
      sqlc.narg(viewer_id)::uuid IS NULL
      OR (e.is_developer_data OR p.is_developer_data)
          = is_developer_user(sqlc.narg(viewer_id)::uuid)
  )
RETURNING *;

-- name: RevokeActiveRemoteAccessToken :exec
UPDATE remote_access_tokens t
SET revoked = TRUE
FROM examinations e
JOIN patients p ON p.id = e.patient_id
WHERE t.examination_id = e.id
  AND t.examination_id = $1
  AND t.used_at IS NULL
  AND NOT t.revoked
  AND (
      sqlc.narg(viewer_id)::uuid IS NULL
      OR (e.is_developer_data OR p.is_developer_data)
          = is_developer_user(sqlc.narg(viewer_id)::uuid)
  );

-- name: GetActiveRemoteAccessToken :one
-- Staff: show/copy/resend. Decrypt token_encrypted in the app.
SELECT t.* FROM remote_access_tokens t
JOIN examinations e ON e.id = t.examination_id
JOIN patients p ON p.id = e.patient_id
WHERE t.examination_id = $1
  AND t.used_at IS NULL
  AND NOT t.revoked
  AND (
      sqlc.narg(viewer_id)::uuid IS NULL
      OR (e.is_developer_data OR p.is_developer_data)
          = is_developer_user(sqlc.narg(viewer_id)::uuid)
  );

-- name: ListRemoteAccessTokensForExamination :many
SELECT t.* FROM remote_access_tokens t
JOIN examinations e ON e.id = t.examination_id
JOIN patients p ON p.id = e.patient_id
WHERE t.examination_id = $1
  AND (
      sqlc.narg(viewer_id)::uuid IS NULL
      OR (e.is_developer_data OR p.is_developer_data)
          = is_developer_user(sqlc.narg(viewer_id)::uuid)
  )
ORDER BY t.sent_at DESC;

-- name: GetRemoteAccessTokenByHash :one
-- Patient-facing: app hashes the URL token, then looks up by hash.
-- No viewer_id.
SELECT * FROM remote_access_tokens
WHERE token_hash = $1
  AND used_at IS NULL
  AND NOT revoked
  AND expires_at > NOW();

-- name: ConsumeRemoteAccessToken :one
-- Patient-facing: mark used. Lookup by hash again to close races.
UPDATE remote_access_tokens
SET used_at = NOW()
WHERE token_hash = $1
  AND used_at IS NULL
  AND NOT revoked
  AND expires_at > NOW()
RETURNING *;
