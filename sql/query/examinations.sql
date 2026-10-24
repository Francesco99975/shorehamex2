-- examinations.sql
--
-- sqlc queries for the examinations table and its relationship to assignments.
--
-- Partition rule (same as the rest of the app):
--   (e.is_developer_data OR p.is_developer_data) = is_developer_user(viewer_id)
--
-- Examination status is the 3-value enum:
--   ASSIGNED | IN_PROGRESS | COMPLETED
-- Assignment status remains the original 4-value enum.

-- =============================================================================
-- Create
-- =============================================================================


-- name: CountExaminations :one
SELECT COUNT(*)
FROM examinations e
JOIN patients p ON p.id = e.patient_id
WHERE (
    sqlc.narg(viewer_id)::uuid IS NULL
    OR (e.is_developer_data OR p.is_developer_data) = is_developer_user(sqlc.narg(viewer_id)::uuid)
);

-- name: CreateExamination :one
WITH inserted AS (
    INSERT INTO examinations (
        id,
        patient_id,
        mode,
        status,
        assigned_by,
        due_at
    ) VALUES (
        $1, $2, $3, $4, $5, $6
    )
    RETURNING *
)
SELECT
    i.*,
    p.email     AS patient_email,
    p.full_name AS patient_name
FROM inserted i
JOIN patients p ON p.id = i.patient_id;


-- name: CreateAssignmentInExamination :one
-- Insert one instrument into an existing examination at a given sequence.
INSERT INTO assignments (
    id,
    examination_id,
    patient_id,
    test_code,
    sequence,
    status,
    is_developer_data
)
SELECT
    sqlc.arg(id),
    e.id,
    e.patient_id,
    sqlc.arg(test_code),
    sqlc.arg(sequence),
    sqlc.arg(status),
    e.is_developer_data
FROM examinations e
JOIN patients p ON p.id = e.patient_id
WHERE e.id = sqlc.arg(examination_id)
  AND (
      sqlc.narg(viewer_id)::uuid IS NULL
      OR (e.is_developer_data OR p.is_developer_data) = is_developer_user(sqlc.narg(viewer_id)::uuid)
  )
RETURNING *;

-- =============================================================================
-- Read – single
-- =============================================================================

-- name: GetExamination :one
SELECT e.*
FROM examinations e
JOIN patients p ON p.id = e.patient_id
WHERE e.id = $1
  AND (
      sqlc.narg(viewer_id)::uuid IS NULL
      OR (e.is_developer_data OR p.is_developer_data) = is_developer_user(sqlc.narg(viewer_id)::uuid)
  );

-- name: GetExaminationWithPatient :one
SELECT
    e.*,
    p.full_name AS patient_name,
    p.mrn       AS patient_mrn,
    p.email     AS patient_email,
    (
        SELECT COALESCE(
            jsonb_agg(
                jsonb_build_object(
                    'test_code', a.test_code,
                    'completed', (a.status = 'COMPLETED')
                )
                ORDER BY a.sequence
            ),
            '[]'::jsonb
        )
        FROM assignments a
        WHERE a.examination_id = e.id
    ) AS assignment_chips,
    (SELECT COUNT(*) FROM assignments a WHERE a.examination_id = e.id)
        AS tests_total,
    (SELECT COUNT(*) FROM assignments a
     WHERE a.examination_id = e.id AND a.status = 'COMPLETED')
        AS tests_completed,
    (
        SELECT token_encrypted
        FROM remote_access_tokens rat
        WHERE rat.examination_id = e.id
          AND rat.used_at IS NULL
          AND NOT rat.revoked
        LIMIT 1
    ) AS remote_token
FROM examinations e
JOIN patients p ON p.id = e.patient_id
WHERE e.id = $1
    AND (
        sqlc.narg(viewer_id)::uuid IS NULL
        OR (e.is_developer_data OR p.is_developer_data) = is_developer_user(sqlc.narg(viewer_id)::uuid)
    );

-- name: ListExaminations :many
-- Unified list for the Examinations page, dashboard "needs attention",
-- and patient-detail examination history.
--
-- Filters (all optional / nullable):
--   patient_id            – scope to one patient
--   status                – exact examination_status (ASSIGNED|IN_PROGRESS|COMPLETED)
--   only_overdue          – true → status <> 'COMPLETED' AND due_at < NOW()
--   only_awaiting_review  – true → at least one child assignment is READY_FOR_REVIEW
--
-- When only_overdue or only_awaiting_review is true the status filter is ignored.
--
-- Ordering:
--   overdue            → due_at ASC
--   awaiting review    → completed_at ASC
--   everything else    → created_at DESC
--
-- Extra columns for the ExaminationItem component:
--   assignment_chips  JSONB  – ordered array of
--       [{"test_code":"BDI","completed":true}, {"test_code":"BAI","completed":false}, ...]
--       One chip per assignment; "ON" when completed, "OFF" otherwise.
--   tests_total       INT
--   tests_completed   INT
--   remote_token      BYTEA  – the single active token_encrypted (or NULL)
--   patient_name, patient_mrn
--
SELECT
        e.*,
        p.full_name AS patient_name,
        p.mrn       AS patient_mrn,
        -- Chips for the UI: one entry per assignment, ordered by sequence
        (
            SELECT COALESCE(
                jsonb_agg(
                    jsonb_build_object(
                        'test_code', a.test_code,
                        'completed', (a.status = 'READY_FOR_REVIEW' OR a.status = 'COMPLETED')
                    )
                    ORDER BY a.sequence
                ),
                '[]'::jsonb
            )
            FROM assignments a
            WHERE a.examination_id = e.id
        ) AS assignment_chips,
        (SELECT COUNT(*) FROM assignments a WHERE a.examination_id = e.id)
            AS tests_total,
        (SELECT COUNT(*) FROM assignments a
         WHERE a.examination_id = e.id AND a.status = 'COMPLETED')
            AS tests_completed,
        -- The one active remote token (partial unique index guarantees ≤ 1)
        -- active token only
        (
            SELECT token_encrypted
            FROM remote_access_tokens rat
            WHERE rat.examination_id = e.id
              AND rat.used_at IS NULL
              AND NOT rat.revoked
            LIMIT 1
        ) AS remote_token
    FROM examinations e
    JOIN patients p ON p.id = e.patient_id
    WHERE
        (sqlc.narg(patient_id)::uuid IS NULL OR e.patient_id = sqlc.narg(patient_id))
        AND (
            sqlc.narg(viewer_id)::uuid IS NULL
            OR (e.is_developer_data OR p.is_developer_data)
                = is_developer_user(sqlc.narg(viewer_id)::uuid)
        )
        -- status filter (ignored when special flags are set)
        AND (
            sqlc.narg(only_overdue)::bool IS TRUE
            OR sqlc.narg(only_awaiting_review)::bool IS TRUE
            OR sqlc.narg(status)::text IS NULL
            OR e.status = sqlc.narg(status)::examination_status
        )
        -- mode filter
        AND (
            sqlc.narg(mode)::text IS NULL
            OR e.mode = sqlc.narg(mode)::assignment_mode
        )
        -- overdue filter
        AND (
            sqlc.narg(only_overdue)::bool IS NOT TRUE
            OR (e.status <> 'COMPLETED' AND e.due_at < NOW())
        )
        -- awaiting-review = any child assignment is READY_FOR_REVIEW
        AND (
            sqlc.narg(only_awaiting_review)::bool IS NOT TRUE
            OR EXISTS (
                SELECT 1 FROM assignments a
                WHERE a.examination_id = e.id
                  AND a.status = 'READY_FOR_REVIEW'
            )
        )
    ORDER BY
        CASE
            WHEN sqlc.narg(only_overdue)::bool IS TRUE THEN e.due_at
            WHEN sqlc.narg(only_awaiting_review)::bool IS TRUE THEN e.completed_at
            ELSE NULL
        END ASC NULLS LAST,
        e.created_at DESC
    LIMIT sqlc.arg(row_limit)
    OFFSET sqlc.arg(row_offset);


-- name: GetAssignmentInExamination :one
SELECT a.*
FROM assignments a
JOIN examinations e ON e.id = a.examination_id
JOIN patients p ON p.id = e.patient_id
WHERE a.id = $1
  AND a.examination_id = $2
  AND (
      sqlc.narg(viewer_id)::uuid IS NULL
      OR (e.is_developer_data OR p.is_developer_data) = is_developer_user(sqlc.narg(viewer_id)::uuid)
  );


-- name: GetNextAssignment :one
SELECT a.*
FROM assignments a
JOIN examinations e ON e.id = a.examination_id
JOIN patients p ON p.id = e.patient_id
WHERE a.examination_id = $1 AND a.status != 'COMPLETED' AND a.status != 'READY_FOR_REVIEW'
  AND (
      sqlc.narg(viewer_id)::uuid IS NULL
      OR (e.is_developer_data OR p.is_developer_data) = is_developer_user(sqlc.narg(viewer_id)::uuid)
  )
ORDER BY a.sequence ASC
LIMIT 1;

-- =============================================================================
-- Updates
-- =============================================================================

-- name: UpdateExaminationDueAt :one
UPDATE examinations e
SET due_at = sqlc.arg(due_at)
FROM patients p
WHERE e.id = sqlc.arg(id)
  AND p.id = e.patient_id
  AND (
      sqlc.narg(viewer_id)::uuid IS NULL
      OR (e.is_developer_data OR p.is_developer_data) = is_developer_user(sqlc.narg(viewer_id)::uuid)
  )
RETURNING e.*;

-- name: MarkExaminationCompleted :one
UPDATE examinations e
SET status = 'COMPLETED',
    completed_at = NOW()
FROM patients p
WHERE e.id = sqlc.arg(id)
  AND p.id = e.patient_id
  AND (
      sqlc.narg(viewer_id)::uuid IS NULL
      OR (e.is_developer_data OR p.is_developer_data) = is_developer_user(sqlc.narg(viewer_id)::uuid)
  )
RETURNING e.*;

-- name: MarkExaminationReviewed :one
UPDATE examinations e
SET reviewed_at = NOW(),
    reviewed_by = sqlc.arg(reviewed_by)
FROM patients p
WHERE e.id = sqlc.arg(id)
  AND p.id = e.patient_id
  AND (
      sqlc.narg(viewer_id)::uuid IS NULL
      OR (e.is_developer_data OR p.is_developer_data) = is_developer_user(sqlc.narg(viewer_id)::uuid)
  )
RETURNING e.*;


-- =============================================================================
-- Delete
-- =============================================================================

-- name: DeleteExamination :one
-- Cascades to assignments, results, tokens, reminders via FK.
-- Returns the total number of examinations remaining in the table after this delete.
DELETE FROM examinations e
USING patients p
WHERE e.id = $1
  AND p.id = e.patient_id
  AND (
      sqlc.narg(viewer_id)::uuid IS NULL
      OR (e.is_developer_data OR p.is_developer_data) = is_developer_user(sqlc.narg(viewer_id)::uuid)
  )
RETURNING
    (SELECT COUNT(*) FROM examinations) - 1  AS remaining_examinations;

-- name: DeleteAssignmentFromExamination :exec
DELETE FROM assignments a
USING examinations e
JOIN patients p ON p.id = e.patient_id
WHERE a.id = $1
  AND a.examination_id = e.id
  AND (
      sqlc.narg(viewer_id)::uuid IS NULL
      OR (e.is_developer_data OR p.is_developer_data) = is_developer_user(sqlc.narg(viewer_id)::uuid)
  );

-- =============================================================================
-- Counts / badges (dashboard)
-- =============================================================================

-- name: CountExaminationsByStatus :many
SELECT e.status, COUNT(*) AS count
FROM examinations e
JOIN patients p ON p.id = e.patient_id
WHERE (
    sqlc.narg(viewer_id)::uuid IS NULL
    OR (e.is_developer_data OR p.is_developer_data) = is_developer_user(sqlc.narg(viewer_id)::uuid)
)
GROUP BY e.status;

-- name: CountOverdueExaminations :one
SELECT COUNT(*) AS count
FROM examinations e
JOIN patients p ON p.id = e.patient_id
WHERE e.status <> 'COMPLETED'
  AND e.due_at < NOW()
  AND (
      sqlc.narg(viewer_id)::uuid IS NULL
      OR (e.is_developer_data OR p.is_developer_data) = is_developer_user(sqlc.narg(viewer_id)::uuid)
  );

-- name: CountExaminationsAwaitingReview :one
SELECT COUNT(DISTINCT e.id) AS count
FROM examinations e
JOIN patients p ON p.id = e.patient_id
JOIN assignments a ON a.examination_id = e.id
WHERE a.status = 'READY_FOR_REVIEW'
  AND (
      sqlc.narg(viewer_id)::uuid IS NULL
      OR (e.is_developer_data OR p.is_developer_data) = is_developer_user(sqlc.narg(viewer_id)::uuid)
  );
