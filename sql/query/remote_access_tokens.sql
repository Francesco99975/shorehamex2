-- remote_access_tokens.sql
--
-- sqlc queries for the remote_access_tokens table.
--
-- Two different access models in this one file:
--
-- 1. Staff-facing queries (send/resend/revoke/list/get-active) are used
--    by an authenticated clinician or developer through the app, so
--    they carry the same partition rule as patients.sql/assignments.sql
--    — inherited via join, since this table has no is_developer_data
--    column of its own:
--        (a.is_developer_data OR p.is_developer_data) = is_developer_user(viewer_id)
--
-- 2. Patient-facing queries (GetRemoteAccessTokenByHash,
--    ConsumeRemoteAccessToken) are used by a patient who followed a
--    secret emailed link — there's no logged-in app user/role in that
--    flow at all, so there's deliberately NO viewer_id or partition
--    check on these two. Validity is entirely about the token itself
--    (unused, unrevoked, unexpired), same idea as
--    MarkAssignmentReadyForReview having no viewer check in
--    assignments.sql — no human "viewer" exists in that code path to
--    check against.
--
-- Only one usable ("active") token can exist per assignment at a time —
-- enforced by idx_remote_access_tokens_active (a partial unique index
-- on assignment_id WHERE used_at IS NULL AND NOT revoked). That's why
-- ResendRemoteAccessToken revokes the current active token and inserts
-- the replacement in one atomic statement (a writable CTE) — doing it
-- as two round trips would risk a moment where either both or neither
-- token is active.

-- name: SendRemoteAccessToken :one
-- Initial "Send link" action — there should be no existing active token
-- yet. If one exists, the unique partial index rejects this with a
-- constraint violation; the app should have already checked via
-- GetActiveRemoteAccessToken and called ResendRemoteAccessToken instead.
INSERT INTO remote_access_tokens (
    id, assignment_id, token_hash, sent_to_email, expires_at
)
SELECT sqlc.arg(id), a.id, sqlc.arg(token_hash), sqlc.arg(sent_to_email), sqlc.arg(expires_at)
FROM assignments a
JOIN patients p ON p.id = a.patient_id
WHERE a.id = sqlc.arg(assignment_id)
  AND (a.is_developer_data OR p.is_developer_data) = is_developer_user(sqlc.arg(viewer_id))
RETURNING *;

-- name: ResendRemoteAccessToken :one
-- "Resend link" action: atomically revokes the current active token (if
-- any) and issues a new one, carrying resend_count forward + 1. If
-- there's no active token to revoke (e.g. it already expired), this
-- still issues a fresh one at resend_count 0 — a resend on an already-
-- lapsed link is effectively a new send.
WITH revoked AS (
    UPDATE remote_access_tokens t
    SET revoked = TRUE
    FROM assignments a
    JOIN patients p ON p.id = a.patient_id
    WHERE t.assignment_id = a.id
      AND t.assignment_id = sqlc.arg(assignment_id)
      AND t.used_at IS NULL
      AND NOT t.revoked
      AND (a.is_developer_data OR p.is_developer_data) = is_developer_user(sqlc.arg(viewer_id))
    RETURNING t.resend_count
)
INSERT INTO remote_access_tokens (
    id, assignment_id, token_hash, sent_to_email, expires_at, resend_count
)
SELECT sqlc.arg(id), a.id, sqlc.arg(token_hash), sqlc.arg(sent_to_email), sqlc.arg(expires_at),
       COALESCE((SELECT resend_count FROM revoked), -1) + 1
FROM assignments a
JOIN patients p ON p.id = a.patient_id
WHERE a.id = sqlc.arg(assignment_id)
  AND (a.is_developer_data OR p.is_developer_data) = is_developer_user(sqlc.arg(viewer_id))
RETURNING *;

-- name: RevokeActiveRemoteAccessToken :exec
-- Standalone "invalidate this link" action (e.g. patient reports the
-- email was compromised), independent of sending a replacement.
UPDATE remote_access_tokens t
SET revoked = TRUE
FROM assignments a
JOIN patients p ON p.id = a.patient_id
WHERE t.assignment_id = a.id
  AND t.assignment_id = $1
  AND t.used_at IS NULL
  AND NOT t.revoked
  AND (a.is_developer_data OR p.is_developer_data) = is_developer_user(sqlc.arg(viewer_id));

-- name: GetActiveRemoteAccessToken :one
-- The currently usable token for an assignment, if any — backs a "link
-- sent 2 days ago, expires in 5 days" display, and lets the app decide
-- whether a "Send" or "Resend" action is appropriate.
SELECT t.* FROM remote_access_tokens t
JOIN assignments a ON a.id = t.assignment_id
JOIN patients p ON p.id = a.patient_id
WHERE t.assignment_id = $1
  AND t.used_at IS NULL
  AND NOT t.revoked
  AND (a.is_developer_data OR p.is_developer_data) = is_developer_user(sqlc.arg(viewer_id));

-- name: ListRemoteAccessTokensForAssignment :many
-- Full send/resend history for an assignment (audit trail — "link sent,
-- then resent twice, then used").
SELECT t.* FROM remote_access_tokens t
JOIN assignments a ON a.id = t.assignment_id
JOIN patients p ON p.id = a.patient_id
WHERE t.assignment_id = $1
  AND (a.is_developer_data OR p.is_developer_data) = is_developer_user(sqlc.arg(viewer_id))
ORDER BY t.sent_at DESC;

-- name: GetRemoteAccessTokenByHash :one
-- Patient-facing: looks up the token the patient's link decoded to
-- (already hashed by the app before this query runs — never look up by
-- raw token). No viewer_id — see header comment. Only returns a row if
-- the token is actually still usable; an expired/used/revoked token
-- simply doesn't match, which the app should treat as "this link is no
-- longer valid" without distinguishing why.
SELECT * FROM remote_access_tokens
WHERE token_hash = $1
  AND used_at IS NULL
  AND NOT revoked
  AND expires_at > NOW();

-- name: ConsumeRemoteAccessToken :one
-- Patient-facing: marks the token used at the moment they actually
-- start/submit their assessment. Re-checks validity in the same
-- statement (not just relying on a prior GetRemoteAccessTokenByHash
-- call) to close the race where two requests redeem the same link at
-- once — only one can win. Returns zero rows if it was already
-- used/revoked/expired between the two calls.
UPDATE remote_access_tokens
SET used_at = NOW()
WHERE token_hash = $1
  AND used_at IS NULL
  AND NOT revoked
  AND expires_at > NOW()
RETURNING *;
