-- activity_events.sql
--
-- sqlc queries for the activity_events table.
--
-- Unlike every other file in this set, this table is populated almost
-- entirely by triggers (trigger_log_patient_created,
-- trigger_log_patient_active_changed, trigger_log_assignment_events,
-- trigger_log_link_sent, trigger_log_reminder_sent — see the migration),
-- all funneling through the shared log_activity_event() function. There
-- isn't a "CreateActivityEvent" query here for the app to call directly
-- with raw columns; LogActivityEvent below just calls that same shared
-- function, so any manual/ad-hoc event the app wants to record gets
-- exactly the same is_developer_data + actor_id handling as every
-- trigger-generated one, rather than a second, possibly-diverging
-- implementation of that logic.
--
-- Visibility: is_developer_data lives directly ON this table (stamped
-- once at write time — see the column comment in the migration for
-- why), not derived via a join at read time. So the partition check
-- here is just a plain column comparison, no join required for
-- correctness:
--     is_developer_data = is_developer_user(viewer_id)
-- The LEFT JOINs to patients/assignments/users in the list queries
-- below are purely for display (patient name, test name, actor name) —
-- all nullable, since a referenced patient/assignment/user may have
-- since been deleted while the event itself persists as history.

-- name: LogActivityEvent :exec
-- For any event the app wants to record that isn't already covered by
-- a trigger. Delegates entirely to log_activity_event(), so
-- is_developer_data and actor_id are computed identically to every
-- trigger-driven row.
SELECT log_activity_event(
    sqlc.arg(event_type), sqlc.arg(patient_id), sqlc.arg(assignment_id), sqlc.arg(metadata)
);

-- name: ListRecentActivity :many
-- Dashboard-wide "Recent activity" feed, most recent first.
SELECT
    e.*,
    p.full_name AS patient_name,
    u.full_name AS actor_name
FROM activity_events e
LEFT JOIN patients p ON p.id = e.patient_id
LEFT JOIN users u ON u.id = e.actor_id
WHERE e.is_developer_data = is_developer_user(sqlc.arg(viewer_id))
ORDER BY e.created_at DESC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: ListActivityForPatient :many
-- One patient's timeline (patient detail view), most recent first.
SELECT
    e.*,
    u.full_name AS actor_name
FROM activity_events e
LEFT JOIN users u ON u.id = e.actor_id
WHERE e.patient_id = $1
  AND e.is_developer_data = is_developer_user(sqlc.arg(viewer_id))
ORDER BY e.created_at DESC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: ListActivityForAssignment :many
-- One assignment's full history (created, started, ready for review,
-- reviewed, links sent, reminders sent), most recent first.
SELECT
    e.*,
    u.full_name AS actor_name
FROM activity_events e
LEFT JOIN users u ON u.id = e.actor_id
WHERE e.assignment_id = $1
  AND e.is_developer_data = is_developer_user(sqlc.arg(viewer_id))
ORDER BY e.created_at DESC;
