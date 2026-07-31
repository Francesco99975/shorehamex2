-- name: CreateEmailVerification :one
INSERT INTO email_verifications (
    id, user_id, token, email, expires_at
) VALUES (
    $1, $2, $3, $4, $5
)
RETURNING id, token, email, expires_at, created_at;

-- name: GetEmailVerificationByToken :one
SELECT id, user_id, token, email, expires_at, used, created_at
FROM email_verifications
WHERE token = $1
  AND used = FALSE
  AND expires_at > NOW();

-- name: MarkEmailVerificationUsed :exec
UPDATE email_verifications
SET used = TRUE
WHERE token = $1;

-- name: DeleteEmailVerificationByUserID :exec
DELETE FROM email_verifications
WHERE user_id = $1;

-- name: CleanupExpiredEmailVerifications :exec
DELETE FROM email_verifications
WHERE expires_at < NOW() OR used = TRUE;

-- name: GetEmailVerificationByID :one
SELECT id, user_id, token, email, expires_at, used, created_at
FROM email_verifications
WHERE id = $1;

-- name: GetEmailVerificationByUserID :one
SELECT id, user_id, token, email, expires_at, used, created_at
FROM email_verifications
WHERE user_id = $1;
