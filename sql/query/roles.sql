-- name: GetRoleByID :one
SELECT id, created_at, updated_at
FROM roles
WHERE id = $1;

-- name: ListRoles :many
SELECT id, created_at, updated_at
FROM roles
ORDER BY id;

-- name: CreateRole :one
INSERT INTO roles (id, created_at, updated_at)
VALUES ($1, $2, $3)
RETURNING id, created_at, updated_at;

-- name: UpdateRole :one
UPDATE roles
SET updated_at = $2
WHERE id = $1
RETURNING id, created_at, updated_at;

-- name: DeleteRole :exec
DELETE FROM roles
WHERE id = $1;

-- name: DeleteAllRoles :exec
DELETE FROM roles;

-- name: CountRoles :one
SELECT COUNT(*) FROM roles;

-- name: GetRoleById :one
SELECT id, created_at, updated_at
FROM roles
WHERE id = $1;
