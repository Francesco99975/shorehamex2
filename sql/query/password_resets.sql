-- name: CreatePasswordReset :one
INSERT INTO password_resets (
    id, user_id, token, expires_at
) VALUES (
    $1, $2, $3, $4
)
RETURNING id, token, expires_at, created_at;

-- name: GetPasswordResetByToken :one
SELECT id, user_id, token, expires_at, used, created_at
FROM password_resets
WHERE token = $1
  AND used = FALSE
  AND expires_at > NOW();

-- name: MarkPasswordResetUsed :exec
UPDATE password_resets
SET used = TRUE
WHERE token = $1;

-- name: DeletePasswordResetByUserID :exec
DELETE FROM password_resets
WHERE user_id = $1;

-- name: CleanupExpiredPasswordResets :exec
DELETE FROM password_resets
WHERE expires_at < NOW() OR used = TRUE;
