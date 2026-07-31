-- assignment_reminders.sql
--
-- sqlc queries for the assignment_reminders table — just an audit trail
-- of "Send reminder" events, no scoring/state-machine logic here.
--
-- Same inherited-partition pattern as remote_access_tokens.sql (this
-- table has no is_developer_data column of its own; visibility comes
-- via join to assignments/patients):
--     (a.is_developer_data OR p.is_developer_data) = is_developer_user(viewer_id)
-- applies to the staff-facing queries (CreateAssignmentReminder and the
-- list/count/throttle-check reads).
--
-- ListAssignmentsDueForReminder is different from every other query in
-- this file: it's meant to be called by a scheduled job, not a logged-
-- in staff member, so there's no viewer_id to check against — same
-- reasoning as MarkAssignmentReadyForReview in assignments.sql. But
-- unlike that query, this one still can't just skip the partition
-- question entirely: an automated job blasting reminder emails should
-- never pick up developer test assignments (which likely don't even
-- have real email addresses behind them), so it hard-excludes
-- is_developer_data rows unconditionally, rather than gating on who's
-- asking.
--
-- sent_by is nullable on the table itself specifically so this file can
-- distinguish a human-initiated reminder from a scheduled one — see
-- CreateAssignmentReminder vs CreateSystemAssignmentReminder.

-- name: CreateAssignmentReminder :one
-- A staff member clicking "Send reminder". sent_by is the clicking
-- user, who also doubles as the viewer for the partition check (same
-- reasoning as reviewer_id in assignments.CompleteAssignmentReview).
INSERT INTO assignment_reminders (
    id, assignment_id, sent_by
)
SELECT sqlc.arg(id), a.id, sqlc.arg(sent_by)
FROM assignments a
JOIN patients p ON p.id = a.patient_id
WHERE a.id = sqlc.arg(assignment_id)
  AND (a.is_developer_data OR p.is_developer_data) = is_developer_user(sqlc.arg(sent_by))
RETURNING *;

-- name: CreateSystemAssignmentReminder :one
-- A scheduled job's automated reminder — sent_by is left NULL (no
-- human triggered this), and there's deliberately no partition check
-- here: by the time this is called, the assignment_id already came from
-- ListAssignmentsDueForReminder, which already excluded developer data.
INSERT INTO assignment_reminders (
    id, assignment_id, sent_by
) VALUES (
    $1, $2, NULL
)
RETURNING *;

-- name: ListRemindersForAssignment :many
-- Audit trail for one assignment. sent_by NULL means it was an
-- automated reminder, not a specific staff member's click.
SELECT r.* FROM assignment_reminders r
JOIN assignments a ON a.id = r.assignment_id
JOIN patients p ON p.id = a.patient_id
WHERE r.assignment_id = $1
  AND (a.is_developer_data OR p.is_developer_data) = is_developer_user(sqlc.arg(viewer_id))
ORDER BY r.sent_at DESC;

-- name: GetLastReminderSentAt :one
-- For a UI throttle ("last reminder sent 2h ago, try again after 24h")
-- or to decide whether to grey out the "Send reminder" button. NULL if
-- none have ever been sent.
SELECT MAX(r.sent_at)::TIMESTAMPTZ AS last_sent_at
FROM assignment_reminders r
JOIN assignments a ON a.id = r.assignment_id
JOIN patients p ON p.id = a.patient_id
WHERE r.assignment_id = $1
  AND (a.is_developer_data OR p.is_developer_data) = is_developer_user(sqlc.arg(viewer_id));

-- name: CountRemindersForAssignment :one
-- For a small "3 reminders sent" badge/tooltip.
SELECT COUNT(*) FROM assignment_reminders r
JOIN assignments a ON a.id = r.assignment_id
JOIN patients p ON p.id = a.patient_id
WHERE r.assignment_id = $1
  AND (a.is_developer_data OR p.is_developer_data) = is_developer_user(sqlc.arg(viewer_id));

-- name: ListAssignmentsDueForReminder :many
-- For a scheduled job: not-yet-completed assignments due on or before
-- sqlc.arg(due_before), whose most recent reminder (if any) is older
-- than sqlc.arg(reminder_cutoff) — i.e. either never reminded, or not
-- reminded recently enough to remind again. Always excludes developer
-- test data (see header comment) regardless of who/what calls this.
SELECT
    a.*,
    p.full_name AS patient_name,
    p.email AS patient_email,
    MAX(r.sent_at) AS last_reminder_sent_at
FROM assignments a
JOIN patients p ON p.id = a.patient_id
LEFT JOIN assignment_reminders r ON r.assignment_id = a.id
WHERE a.status <> 'COMPLETED'
  AND a.due_at <= sqlc.arg(due_before)
  AND NOT (a.is_developer_data OR p.is_developer_data)
GROUP BY a.id, p.id
HAVING MAX(r.sent_at) IS NULL OR MAX(r.sent_at) < sqlc.arg(reminder_cutoff)
ORDER BY a.due_at;
