-- name: CreatePendingAuthChallenge :one
INSERT INTO pending_auth_challenges (
    id,
    user_id,
    mode,
    secret,
    remember_me,
    expires_at
) VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetPendingAuthChallenge :one
SELECT * FROM pending_auth_challenges
WHERE id = $1
  AND expires_at > NOW();

-- name: GetPendingAuthChallengeByUser :one
SELECT * FROM pending_auth_challenges
WHERE user_id = $1
  AND mode = $2
  AND expires_at > NOW();

-- name: PromoteChallengeToRegistration :one
UPDATE pending_auth_challenges
SET
    mode   = 'totp_registration',
    secret = $2
WHERE id = $1
    AND mode = 'mfa_challenge'
    AND expires_at > NOW()
RETURNING *;

-- name: DeletePendingAuthChallenge :exec
DELETE FROM pending_auth_challenges
WHERE id = $1;

-- name: DeletePendingAuthChallengesByUser :exec
DELETE FROM pending_auth_challenges
WHERE user_id = $1
  AND mode = $2;

-- name: DeleteExpiredPendingAuthChallenges :exec
DELETE FROM pending_auth_challenges
WHERE expires_at < NOW();
