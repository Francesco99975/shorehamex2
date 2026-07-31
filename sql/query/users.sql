-- name: CreateUser :one
INSERT INTO users (
    id, role, username, email, full_name, title, password_hash
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
)
RETURNING id, role, username, email, full_name, title, is_active, is_email_verified,
         twofa_enabled, last_login, created_at, updated_at;

-- name: UpdateUser :one
UPDATE users
SET
    full_name     = $2,
    username      = $3,
    email         = $4,
    role          = $5,
    title         = $6,
    is_active     = $7,
    password_hash = COALESCE(NULLIF($8, ''), password_hash)
WHERE id = $1
RETURNING id, username, email, role, full_name, title, is_active, is_email_verified, twofa_enabled, created_at, updated_at, last_login;

-- name: GetUserByID :one
SELECT id, role, username, email, full_name, title, is_active,
       is_email_verified, twofa_enabled, last_login,
       created_at, updated_at
FROM users
WHERE id = $1;

-- name: ExistsUserWithEmail :one
SELECT EXISTS(
    SELECT 1
    FROM users
    WHERE email = $1
);

-- name: ExistsUserWithUsername :one
SELECT EXISTS(
    SELECT 1
    FROM users
    WHERE username = $1
);

-- name: ExistDeveloperAccount :one
SELECT EXISTS(
    SELECT 1
    FROM users
    WHERE role = 'DEVELOPER'
);

-- name: GetUserByEmailOrUsername :one
SELECT
    id,
    role,
    username,
    email,
    full_name,
    title,
    password_hash,
    is_active,
    is_email_verified,
    twofa_enabled,
    last_login,
    created_at,
    updated_at
FROM users
WHERE (email = $1 OR username = $1)
  AND is_active = TRUE;

-- name: GetUserByEmail :one
SELECT id, role, username, email, full_name, title, is_active,
       is_email_verified, twofa_enabled, last_login,
       created_at, updated_at
FROM users
WHERE email = $1;

-- name: GetUserByUsername :one
SELECT id, role, username, email, full_name, title, is_active,
       is_email_verified, twofa_enabled, last_login,
       created_at, updated_at
FROM users
WHERE username = $1;

-- name: UpdateUserUsernameOrEmail :one
UPDATE users
SET
    username       = $1,
    email          = $2,
    is_email_verified = CASE
        WHEN email IS DISTINCT FROM $2 THEN FALSE
        ELSE is_email_verified
    END
WHERE id = $3
RETURNING id, role, username, email, full_name, title, is_active, is_email_verified,
         twofa_enabled, last_login, created_at, updated_at;

-- name: UpdateUserUsername :one
UPDATE users
SET
    username       = $1
WHERE id = $2
RETURNING id, role, username, email, full_name, title, is_active, is_email_verified,
         twofa_enabled, last_login, created_at, updated_at;

-- name: UpdateUserEmail :one
UPDATE users
SET
    email          = $1
WHERE id = $2
RETURNING id, role, username, email, full_name, title, is_active, is_email_verified,
         twofa_enabled, last_login, created_at, updated_at;

-- name: UpdateUserLastLogin :exec
UPDATE users
SET last_login = NOW()
WHERE id = $1;

-- name: UpdateUserPassword :exec
UPDATE users
SET password_hash = $1
WHERE id = $2;

-- name: EnableUser2FA :exec
UPDATE users
SET twofa_secret = $1, twofa_enabled = TRUE
WHERE id = $2;

-- name: DisableUser2FA :exec
UPDATE users
SET twofa_secret = NULL, twofa_enabled = FALSE
WHERE id = $1;

-- name: GetPasswordHash :one
SELECT password_hash
FROM users
WHERE id = $1;

-- name: GetUser2FASecret :one
SELECT username, email, full_name, title, role, twofa_secret, password_hash
FROM users
WHERE id = $1 AND twofa_enabled = TRUE;

-- name: VerifyUserEmail :exec
UPDATE users
SET is_email_verified = TRUE
WHERE id = $1;

-- name: DeactivateUser :exec
UPDATE users
SET is_active = FALSE
WHERE id = $1;


-- name: ReactivateUser :exec
UPDATE users
SET is_active = TRUE
WHERE id = $1;

-- name: DeleteUser :exec
DELETE FROM users
WHERE id = $1;

-- name: CleanupInactiveUsers :exec
DELETE FROM users
WHERE is_active = FALSE;

-- name: GetUsers :many
SELECT id, role, username, email, full_name, title, is_active, is_email_verified, twofa_enabled, last_login, created_at, updated_at
FROM users
ORDER BY username
LIMIT $1
OFFSET $2;

-- name: GetUsersCount :one
SELECT COUNT(*)
FROM users;

-- name: GetUserCountBySearch :one
WITH search_input AS (
    SELECT
        $1::text AS raw
)
SELECT COUNT(*)
FROM users, search_input
WHERE
    (
        $1 = ''
        OR username % raw
        OR email % raw
    )
    AND ($2 = '' OR role = $2);

-- name: SearchUsers :many
WITH search_input AS (
    SELECT
        $1::text AS raw
)
SELECT
    id, role, username, email, full_name, title, is_active, is_email_verified,
    twofa_enabled, last_login, created_at, updated_at,
    GREATEST(
        similarity(username, raw),
        similarity(email, raw)
    ) AS rank
FROM users, search_input
WHERE
    (
        $1 = ''
        OR username % raw
        OR email % raw
    )
    AND ($2 = '' OR role = $2)
ORDER BY rank DESC
LIMIT $3
OFFSET ($4 - 1) * $3;


-- name: GetUsersByRole :many
SELECT id, role, username, email, full_name, title, is_active, is_email_verified, twofa_enabled, last_login, created_at, updated_at
FROM users
WHERE role = $1
ORDER BY username
LIMIT $2
OFFSET $3;

-- name: GetUsersByRoleCount :one
SELECT COUNT(*)
FROM users
WHERE role = $1;
