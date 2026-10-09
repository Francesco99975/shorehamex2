-- assignment_reminders.sql
--
-- sqlc queries for the assignment_reminders table — just an audit trail
-- of "Send reminder" events, no scoring/state-machine logic here.
--
-- After the examinations migration, reminders belong to an examination
-- (the delivery unit that owns the remote link and due_at), not to an
-- individual assignment.
--
-- Same inherited-partition pattern as remote_access_tokens.sql:
--     (e.is_developer_data OR p.is_developer_data) = is_developer_user(viewer_id)
--
-- ListExaminationsDueForReminder is meant for a scheduled job, so it
-- has no viewer_id and hard-excludes developer data.

-- name: CreateAssignmentReminder :one
-- Staff member clicking "Send reminder".
INSERT INTO assignment_reminders (
    id, examination_id, sent_by
)
SELECT sqlc.arg(id), e.id, sqlc.arg(sent_by)
FROM examinations e
JOIN patients p ON p.id = e.patient_id
WHERE e.id = sqlc.arg(examination_id)
  AND (e.is_developer_data OR p.is_developer_data) = is_developer_user(sqlc.arg(sent_by))
RETURNING *;

-- name: CreateSystemAssignmentReminder :one
-- Scheduled job automated reminder — sent_by is NULL.
INSERT INTO assignment_reminders (
    id, examination_id, sent_by
) VALUES (
    $1, $2, NULL
)
RETURNING *;

-- name: ListRemindersForExamination :many
SELECT r.* FROM assignment_reminders r
JOIN examinations e ON e.id = r.examination_id
JOIN patients p ON p.id = e.patient_id
WHERE r.examination_id = $1
  AND (e.is_developer_data OR p.is_developer_data) = is_developer_user(sqlc.arg(viewer_id))
ORDER BY r.sent_at DESC;

-- name: GetLastReminderSentAt :one
SELECT MAX(r.sent_at)::TIMESTAMPTZ AS last_sent_at
FROM assignment_reminders r
JOIN examinations e ON e.id = r.examination_id
JOIN patients p ON p.id = e.patient_id
WHERE r.examination_id = $1
  AND (e.is_developer_data OR p.is_developer_data) = is_developer_user(sqlc.arg(viewer_id));

-- name: CountRemindersForExamination :one
SELECT COUNT(*) FROM assignment_reminders r
JOIN examinations e ON e.id = r.examination_id
JOIN patients p ON p.id = e.patient_id
WHERE r.examination_id = $1
  AND (e.is_developer_data OR p.is_developer_data) = is_developer_user(sqlc.arg(viewer_id));

-- name: ListExaminationsDueForReminder :many
-- Scheduled job: not-yet-completed examinations due on or before
-- due_before, whose most recent reminder (if any) is older than
-- reminder_cutoff. Always excludes developer test data.
SELECT
    e.*,
    p.full_name AS patient_name,
    p.email AS patient_email,
    MAX(r.sent_at) AS last_reminder_sent_at
FROM examinations e
JOIN patients p ON p.id = e.patient_id
LEFT JOIN assignment_reminders r ON r.examination_id = e.id
WHERE e.status <> 'COMPLETED'
  AND e.due_at <= sqlc.arg(due_before)
  AND NOT (e.is_developer_data OR p.is_developer_data)
GROUP BY e.id, p.id
HAVING MAX(r.sent_at) IS NULL OR MAX(r.sent_at) < sqlc.arg(reminder_cutoff)
ORDER BY e.due_at;
