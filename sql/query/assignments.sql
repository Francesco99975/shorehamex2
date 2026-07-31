-- assignments.sql
--
-- sqlc queries for the assignments table.
--
-- Visibility/CRUD rule — same strict partition as patients.sql:
--     (a.is_developer_data OR p.is_developer_data) = is_developer_user(viewer_id)
-- An assignment is in the developer partition if EITHER the assignment
-- itself OR its patient is developer-flagged (a developer can create a
-- throwaway assignment against an otherwise-real, ADMIN/USER-visible
-- patient — that assignment alone still belongs entirely to the
-- developer partition). ADMIN/USER viewers only ever see/manage
-- assignments fully outside that partition; DEVELOPER viewers only
-- ever see/manage assignments inside it. Every query below joins
-- patients as `p` for this reason, even ones that don't otherwise need
-- patient columns — one uniform predicate shape across the whole file.
--
-- Status lifecycle assumed here: ASSIGNED -> IN_PROGRESS (auto, on
-- first response — see mark_assignment_started_on_response trigger) ->
-- READY_FOR_REVIEW (set by the app once the scoring engine finishes
-- scoring all responses) -> COMPLETED (set once a clinician confirms
-- the review). "Overdue" is not a status value — it's
-- (due_at < now() AND status <> 'COMPLETED'), same as the partial index
-- on assignments(due_at).
--
-- MarkAssignmentReadyForReview is the one write query here WITHOUT a
-- viewer/partition check: it's called by the scoring pipeline itself
-- right after computing scores, not by a user clicking something in the
-- UI, so there's no "viewer" to check against — it should transition
-- any assignment, real or developer-flagged, accurately.
--
-- Both DeleteUnstartedAssignment and DeleteAssignment are unrestricted
-- within a viewer's own partition (no developer-only carve-out) — see
-- patients.sql's header comment for the same flag on DeletePatient.

-- name: GetAssignment :one
SELECT a.* FROM assignments a
JOIN patients p ON p.id = a.patient_id
WHERE a.id = $1
  AND (a.is_developer_data OR p.is_developer_data) = is_developer_user(sqlc.arg(viewer_id));

-- name: ListAssignmentsForPatient :many
-- Powers the patient detail view's assignment list. Joins
-- test_definitions for display name so the UI doesn't need a second
-- round trip per row.
SELECT a.*, td.name AS test_name, td.scoring_method AS test_scoring_method
FROM assignments a
JOIN patients p ON p.id = a.patient_id
JOIN test_definitions td ON td.code = a.test_code
WHERE a.patient_id = $1
  AND (a.is_developer_data OR p.is_developer_data) = is_developer_user(sqlc.arg(viewer_id))
ORDER BY a.created_at DESC;

-- name: ListAssignmentsForPatientByStatus :many
-- Backs the patient detail view's status filter tabs (In progress /
-- Ready to review / Complete). For the "Overdue" tab, use
-- ListOverdueAssignmentsForPatient instead — overdue cuts across
-- multiple statuses, it isn't one itself.
SELECT a.*, td.name AS test_name, td.scoring_method AS test_scoring_method
FROM assignments a
JOIN patients p ON p.id = a.patient_id
JOIN test_definitions td ON td.code = a.test_code
WHERE a.patient_id = $1
  AND a.status = $2
  AND (a.is_developer_data OR p.is_developer_data) = is_developer_user(sqlc.arg(viewer_id))
ORDER BY a.created_at DESC;

-- name: ListOverdueAssignmentsForPatient :many
SELECT a.*, td.name AS test_name
FROM assignments a
JOIN patients p ON p.id = a.patient_id
JOIN test_definitions td ON td.code = a.test_code
WHERE a.patient_id = $1
  AND a.status <> 'COMPLETED'
  AND a.due_at < NOW()
  AND (a.is_developer_data OR p.is_developer_data) = is_developer_user(sqlc.arg(viewer_id))
ORDER BY a.due_at;

-- name: ListAssignmentsAwaitingReview :many
-- Dashboard-wide "needs your attention" list, oldest-waiting-first.
-- Joins patient name too, since this spans multiple patients. For a
-- developer viewer, this is scoped entirely to their team's test
-- assignments, same as everything else in this file.
SELECT a.*, p.full_name AS patient_name, p.mrn AS patient_mrn, td.name AS test_name
FROM assignments a
JOIN patients p ON p.id = a.patient_id
JOIN test_definitions td ON td.code = a.test_code
WHERE a.status = 'READY_FOR_REVIEW'
  AND (a.is_developer_data OR p.is_developer_data) = is_developer_user(sqlc.arg(viewer_id))
ORDER BY a.completed_at
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: ListOverdueAssignments :many
-- Dashboard-wide overdue list, same shape as ListAssignmentsAwaitingReview.
SELECT a.*, p.full_name AS patient_name, p.mrn AS patient_mrn, td.name AS test_name
FROM assignments a
JOIN patients p ON p.id = a.patient_id
JOIN test_definitions td ON td.code = a.test_code
WHERE a.status <> 'COMPLETED'
  AND a.due_at < NOW()
  AND (a.is_developer_data OR p.is_developer_data) = is_developer_user(sqlc.arg(viewer_id))
ORDER BY a.due_at
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CountAssignmentsAwaitingReview :one
-- Backs the dashboard's "Awaiting review" stat card.
SELECT COUNT(*) FROM assignments a
JOIN patients p ON p.id = a.patient_id
WHERE a.status = 'READY_FOR_REVIEW'
  AND (a.is_developer_data OR p.is_developer_data) = is_developer_user(sqlc.arg(viewer_id));

-- name: CountAssignmentsInProgress :one
-- Backs the dashboard's "In progress" stat card.
SELECT COUNT(*) FROM assignments a
JOIN patients p ON p.id = a.patient_id
WHERE a.status = 'IN_PROGRESS'
  AND (a.is_developer_data OR p.is_developer_data) = is_developer_user(sqlc.arg(viewer_id));

-- name: CountAssignmentsCompletedSince :one
-- Backs the dashboard's "Completed (7d)"-style stat card; pass
-- NOW() - INTERVAL '7 days' (or whatever window) as $1.
SELECT COUNT(*) FROM assignments a
JOIN patients p ON p.id = a.patient_id
WHERE a.status = 'COMPLETED'
  AND a.completed_at >= $1
  AND (a.is_developer_data OR p.is_developer_data) = is_developer_user(sqlc.arg(viewer_id));

-- name: CountActiveAssignmentsForPatient :one
-- For a small "N active tests" badge on a patient's row/header.
SELECT COUNT(*) FROM assignments a
JOIN patients p ON p.id = a.patient_id
WHERE a.patient_id = $1
  AND a.status <> 'COMPLETED'
  AND (a.is_developer_data OR p.is_developer_data) = is_developer_user(sqlc.arg(viewer_id));

-- name: CreateAssignment :one
-- is_developer_data is NOT a column here — stamped automatically by
-- trigger_mark_assignment_developer_data based on assigned_by's role.
-- Also implicitly needs a matching assignment_results envelope row —
-- see CreateAssignmentResult in assignment_results.sql, called
-- immediately after this in the same transaction (kept as two queries,
-- not one, since assignment_results.sql owns that table's shape).
INSERT INTO assignments (
    id, patient_id, test_code, mode, assigned_by, due_at
) VALUES (
    $1, $2, $3, $4, $5, $6
)
RETURNING *;

-- name: UpdateAssignmentDueDate :one
UPDATE assignments a
SET due_at = $2
FROM patients p
WHERE a.id = $1
  AND p.id = a.patient_id
  AND (a.is_developer_data OR p.is_developer_data) = is_developer_user(sqlc.arg(viewer_id))
RETURNING a.*;

-- name: MarkAssignmentReadyForReview :one
-- Called by the scoring pipeline once every response is in and scored —
-- see the header comment for why this skips the viewer/partition check.
-- Guarded by the current status so it can't be called out of order
-- (e.g. twice, or on something already COMPLETED).
UPDATE assignments
SET status = 'READY_FOR_REVIEW',
    completed_at = NOW()
WHERE id = $1
  AND status = 'IN_PROGRESS'
RETURNING *;

-- name: CompleteAssignmentReview :one
-- A clinician confirming the review — reviewer_id doubles as both
-- "who reviewed this" and "who's asking" for the partition check, since
-- in every real flow they're the same person.
UPDATE assignments a
SET status = 'COMPLETED',
    reviewed_at = NOW(),
    reviewed_by = sqlc.arg(reviewer_id)
FROM patients p
WHERE a.id = $1
  AND p.id = a.patient_id
  AND a.status = 'READY_FOR_REVIEW'
  AND (a.is_developer_data OR p.is_developer_data) = is_developer_user(sqlc.arg(reviewer_id))
RETURNING a.*;

-- name: DeleteUnstartedAssignment :exec
-- Safe, reversible-in-spirit correction path for a real mistake (wrong
-- test assigned to the wrong patient). Restricted to status =
-- 'ASSIGNED': once a patient has answered even one item, use
-- DeleteAssignment instead, which has no such restriction. Unrestricted
-- by role beyond the usual partition check — any ADMIN/USER can use
-- this on a real assignment, any developer on a developer-flagged one.
DELETE FROM assignments a
USING patients p
WHERE a.id = $1
  AND p.id = a.patient_id
  AND a.status = 'ASSIGNED'
  AND (a.is_developer_data OR p.is_developer_data) = is_developer_user(sqlc.arg(viewer_id));

-- name: DeleteAssignment :exec
-- Hard delete, no status restriction — scoped only by the same
-- partition rule as every other query here, NOT developer-only. Same
-- flag as patients.DeletePatient: this removes any append-only/
-- retention protection on real assignment/result history if that
-- matters for real clinical data.
DELETE FROM assignments a
USING patients p
WHERE a.id = $1
  AND p.id = a.patient_id
  AND (a.is_developer_data OR p.is_developer_data) = is_developer_user(sqlc.arg(viewer_id));
