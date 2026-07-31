-- name: CreateUserSession :one
INSERT INTO sessions (
    id,
    user_id,
    session_token_hash,
    ip_address,
    user_agent,
    remember_me,
    expires_at
) VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: GetActiveSession :one
SELECT
    s.*,
    u.username,
    u.email,
    u.role,
    u.full_name,
    u.title,
    u.is_active,
    u.twofa_enabled
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.session_token_hash = $1
  AND s.revoked = FALSE
  AND s.expires_at > NOW();

-- name: GetActiveSessionsByUser :many
SELECT * FROM sessions
WHERE user_id = $1
  AND revoked = FALSE
  AND expires_at > NOW()
ORDER BY last_activity_at DESC;

-- name: TouchSession :exec
UPDATE sessions
SET last_activity_at = NOW()
WHERE id = $1
  AND revoked = FALSE
  AND expires_at > NOW();

-- name: RevokeSession :exec
UPDATE sessions
SET revoked = TRUE, revoked_at = NOW()
WHERE id = $1
  AND revoked = FALSE;

-- name: RevokeSessionByHash :exec
UPDATE sessions
SET revoked = TRUE, revoked_at = NOW()
WHERE session_token_hash = $1
  AND revoked = FALSE;

-- name: RevokeAllUserSessions :exec
UPDATE sessions
SET revoked = TRUE, revoked_at = NOW()
WHERE user_id = $1
  AND revoked = FALSE;

-- name: RevokeAllUserSessionsExcept :exec
UPDATE sessions
SET revoked = TRUE, revoked_at = NOW()
WHERE user_id = $1
  AND id != $2
  AND revoked = FALSE;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions
WHERE expires_at < NOW()
   OR (revoked = TRUE AND revoked_at < NOW() - INTERVAL '7 days');
