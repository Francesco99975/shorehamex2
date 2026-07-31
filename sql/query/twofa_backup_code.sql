-- name: CreateBackupCodes :exec
INSERT INTO twofa_backup_codes (
    id, user_id, code_hash
) VALUES (
    unnest($1::uuid[]), unnest($2::uuid[]), unnest($3::text[])
);

-- name: CountUnusedBackupCodesForUser :one
SELECT count(*)
FROM twofa_backup_codes
WHERE user_id = $1 AND used = FALSE;

-- name: GetUnusedBackupCodesForUser :many
SELECT id, code_hash
FROM twofa_backup_codes
WHERE user_id = $1 AND used = FALSE;

-- name: GetBackupCodeByHash :one
SELECT id, user_id, used
FROM twofa_backup_codes
WHERE code_hash = $1;

-- name: MarkBackupCodeUsed :exec
UPDATE twofa_backup_codes
SET used = TRUE
WHERE id = $1;

-- name: DeleteUserBackupCodes :exec
DELETE FROM twofa_backup_codes
WHERE user_id = $1;
