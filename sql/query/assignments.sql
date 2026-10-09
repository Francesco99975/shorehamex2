-- assignments.sql
--
-- sqlc queries for the assignments table (individual instruments
-- inside an examination).
--
-- Visibility rule — join through examinations + patients:
--     (a.is_developer_data OR e.is_developer_data OR p.is_developer_data)
--         = is_developer_user(viewer_id)
-- In practice a.is_developer_data is stamped from the parent examination,
-- so the examination/patient flags are the real gate.
--
-- Status lifecycle (per instrument):
--   ASSIGNED → IN_PROGRESS (trigger on first response)
--            → READY_FOR_REVIEW (scoring pipeline)
--            → COMPLETED (clinician confirms review)
--
-- mode / due_at / assigned_by live on examinations, not here.
-- page defaults to 1 (tracks the current page within a multi-page instrument).

-- name: GetAssignment :one
SELECT a.* FROM assignments a
JOIN examinations e ON e.id = a.examination_id
JOIN patients p ON p.id = e.patient_id
WHERE a.id = $1
  AND (
      sqlc.narg(viewer_id)::uuid IS NULL
      OR (a.is_developer_data OR e.is_developer_data OR p.is_developer_data)
          = is_developer_user(sqlc.narg(viewer_id)::uuid)
  );

-- name: ListAssignmentsForExamination :many
SELECT
    a.*,
    td.name           AS test_name,
    td.scoring_method AS test_scoring_method,
    td.item_count,
    CASE
        WHEN td.item_count <= 0 THEN 0
        ELSE LEAST(
            100,
            (
                (SELECT COUNT(*)::int
                 FROM jsonb_object_keys(COALESCE(ar.responses, '{}'::jsonb)))
                * 100
            ) / td.item_count
        )
    END AS progress_pct
FROM assignments a
JOIN examinations e ON e.id = a.examination_id
JOIN patients p ON p.id = e.patient_id
JOIN test_definitions td ON td.code = a.test_code
LEFT JOIN assignment_results ar ON ar.assignment_id = a.id
WHERE a.examination_id = $1
  AND (
      sqlc.narg(viewer_id)::uuid IS NULL
      OR (a.is_developer_data OR e.is_developer_data OR p.is_developer_data)
          = is_developer_user(sqlc.narg(viewer_id)::uuid)
  )
ORDER BY a.sequence;

-- name: GetNextAssignmentInExamination :one
-- Resume helper: first incomplete assignment in sequence order.
SELECT a.*
FROM assignments a
JOIN examinations e ON e.id = a.examination_id
JOIN patients p ON p.id = e.patient_id
WHERE a.examination_id = $1
  AND a.status <> 'COMPLETED'
  AND (
      sqlc.narg(viewer_id)::uuid IS NULL
      OR (a.is_developer_data OR e.is_developer_data OR p.is_developer_data)
          = is_developer_user(sqlc.narg(viewer_id)::uuid)
  )
ORDER BY a.sequence
LIMIT 1;

-- name: IncrementAssignmentPage :one
UPDATE assignments a
SET page = a.page + 1
FROM examinations e
JOIN patients p ON p.id = e.patient_id
WHERE a.id = sqlc.arg(assignment_id)
  AND a.examination_id = e.id
  AND (
      sqlc.narg(viewer_id)::uuid IS NULL
      OR (a.is_developer_data OR e.is_developer_data OR p.is_developer_data)
          = is_developer_user(sqlc.narg(viewer_id)::uuid)
  )
RETURNING a.*;

-- name: CountAssignmentsAwaitingReview :one
SELECT COUNT(*) FROM assignments a
JOIN examinations e ON e.id = a.examination_id
JOIN patients p ON p.id = e.patient_id
WHERE a.status = 'READY_FOR_REVIEW'
  AND (
      sqlc.narg(viewer_id)::uuid IS NULL
      OR (a.is_developer_data OR e.is_developer_data OR p.is_developer_data)
          = is_developer_user(sqlc.narg(viewer_id)::uuid)
  );

-- name: CountAssignmentsInProgress :one
SELECT COUNT(*) FROM assignments a
JOIN examinations e ON e.id = a.examination_id
JOIN patients p ON p.id = e.patient_id
WHERE a.status = 'IN_PROGRESS'
  AND (
      sqlc.narg(viewer_id)::uuid IS NULL
      OR (a.is_developer_data OR e.is_developer_data OR p.is_developer_data)
          = is_developer_user(sqlc.narg(viewer_id)::uuid)
  );

-- name: CountAssignmentsCompletedSince :one
SELECT COUNT(*) FROM assignments a
JOIN examinations e ON e.id = a.examination_id
JOIN patients p ON p.id = e.patient_id
WHERE a.status = 'COMPLETED'
  AND a.completed_at >= $1
  AND (
      sqlc.narg(viewer_id)::uuid IS NULL
      OR (a.is_developer_data OR e.is_developer_data OR p.is_developer_data)
          = is_developer_user(sqlc.narg(viewer_id)::uuid)
  );

-- name: CountActiveAssignmentsForPatient :one
SELECT COUNT(*) FROM assignments a
JOIN examinations e ON e.id = a.examination_id
JOIN patients p ON p.id = e.patient_id
WHERE e.patient_id = $1
  AND a.status <> 'COMPLETED'
  AND (
      sqlc.narg(viewer_id)::uuid IS NULL
      OR (a.is_developer_data OR e.is_developer_data OR p.is_developer_data)
          = is_developer_user(sqlc.narg(viewer_id)::uuid)
  );

-- name: CreateAssignment :one
-- is_developer_data is stamped by trigger from the parent examination.
-- mode / due_at / assigned_by live on the examination.
-- page defaults to 1 when not supplied.
INSERT INTO assignments (
    id, examination_id, patient_id, test_code, sequence, status, page
)
SELECT
    sqlc.arg(id),
    e.id,
    e.patient_id,
    sqlc.arg(test_code),
    sqlc.arg(sequence),
    COALESCE(sqlc.narg(status)::assignment_status, 'ASSIGNED') AS status,
    COALESCE(sqlc.narg(page), 1) AS page
FROM examinations e
JOIN patients p ON p.id = e.patient_id
WHERE e.id = sqlc.arg(examination_id)
  AND (
      sqlc.narg(viewer_id)::uuid IS NULL
      OR (e.is_developer_data OR p.is_developer_data)
          = is_developer_user(sqlc.narg(viewer_id)::uuid)
  )
RETURNING *;

-- name: MarkAssignmentReadyForReview :one
-- Scoring pipeline — no viewer/partition check.
UPDATE assignments
SET status = 'READY_FOR_REVIEW',
    completed_at = NOW()
WHERE id = $1
  AND status = 'IN_PROGRESS'
RETURNING *;

-- name: CompleteAssignmentReview :one
UPDATE assignments a
SET status = 'COMPLETED',
    reviewed_at = NOW(),
    reviewed_by = sqlc.arg(reviewer_id)
FROM examinations e
JOIN patients p ON p.id = e.patient_id
WHERE a.id = $1
  AND a.examination_id = e.id
  AND a.status = 'READY_FOR_REVIEW'
  AND (a.is_developer_data OR e.is_developer_data OR p.is_developer_data)
      = is_developer_user(sqlc.arg(reviewer_id))
RETURNING a.*;

-- name: UpdateAssignmentSequence :exec
UPDATE assignments a
SET sequence = sqlc.arg(new_sequence)
FROM examinations e
JOIN patients p ON p.id = e.patient_id
WHERE a.id = sqlc.arg(assignment_id)
  AND a.examination_id = e.id
  AND (
      sqlc.narg(viewer_id)::uuid IS NULL
      OR (a.is_developer_data OR e.is_developer_data OR p.is_developer_data)
          = is_developer_user(sqlc.narg(viewer_id)::uuid)
  );

-- name: UpdateAssignmentPage :exec
UPDATE assignments a
SET page = sqlc.arg(page)
FROM examinations e
JOIN patients p ON p.id = e.patient_id
WHERE a.id = sqlc.arg(assignment_id)
  AND a.examination_id = e.id
  AND (
      sqlc.narg(viewer_id)::uuid IS NULL
      OR (a.is_developer_data OR e.is_developer_data OR p.is_developer_data)
          = is_developer_user(sqlc.narg(viewer_id)::uuid)
  );

-- name: DeleteUnstartedAssignment :exec
DELETE FROM assignments a
USING examinations e
JOIN patients p ON p.id = e.patient_id
WHERE a.id = $1
  AND a.examination_id = e.id
  AND a.status = 'ASSIGNED'
  AND (
      sqlc.narg(viewer_id)::uuid IS NULL
      OR (a.is_developer_data OR e.is_developer_data OR p.is_developer_data)
          = is_developer_user(sqlc.narg(viewer_id)::uuid)
  );

-- name: DeleteAssignment :exec
DELETE FROM assignments a
USING examinations e
JOIN patients p ON p.id = e.patient_id
WHERE a.id = $1
  AND a.examination_id = e.id
  AND (
      sqlc.narg(viewer_id)::uuid IS NULL
      OR (a.is_developer_data OR e.is_developer_data OR p.is_developer_data)
          = is_developer_user(sqlc.narg(viewer_id)::uuid)
  );
