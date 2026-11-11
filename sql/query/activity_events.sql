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

-- name: ListRecentActivity :many
-- One row per user-visible action, newest first.
--
-- Events written by the same transaction share created_at (now() is fixed per
-- transaction) and examination_id, so they are collapsed into one action:
--   EXAMINATION_CREATED + N×ASSIGNMENT_CREATED + LINK_SENT -> REMOTE_SENT
--   ASSIGNMENT_STARTED + ASSIGNMENT_READY_FOR_REVIEW       -> TEST_FINISHED
--   ASSIGNMENT_COMPLETED + ASSIGNMENT_REVIEWED             -> TEST_REVIEWED
-- Events without an examination (PATIENT_CREATED) stay on their own.
--
-- Only the newest 100 raw events are scanned (idx_activity_events_created_at),
-- which is far more than 5 actions need.
WITH recent AS (
    SELECT ev.*
    FROM activity_events ev
    WHERE sqlc.narg(viewer_id)::uuid IS NULL
       OR ev.is_developer_data = is_developer_user(sqlc.narg(viewer_id)::uuid)
    ORDER BY ev.created_at DESC
    LIMIT 100
),
grouped AS (
    SELECT
        ((array_agg(r.id ORDER BY r.id))[1])::uuid                                     AS id,
        r.created_at,
        (array_agg(r.examination_id) FILTER (WHERE r.examination_id IS NOT NULL))[1]   AS examination_id,
        (array_agg(r.patient_id)     FILTER (WHERE r.patient_id     IS NOT NULL))[1]   AS patient_id,
        (array_agg(r.assignment_id)  FILTER (
            WHERE r.assignment_id IS NOT NULL AND r.event_type <> 'ASSIGNMENT_CREATED'
        ))[1]                                                                          AS assignment_id,
        (array_agg(r.actor_id)       FILTER (WHERE r.actor_id       IS NOT NULL))[1]   AS actor_id,
        array_agg(r.event_type)                                                        AS event_types,
        array_agg(r.metadata->>'testCode' ORDER BY (r.metadata->>'sequence')::int)
            FILTER (WHERE r.event_type = 'ASSIGNMENT_CREATED')                         AS created_tests,
        max(r.metadata->>'mode')                                                       AS meta_mode,
        max(r.metadata->>'mrn')                                                        AS meta_mrn
    FROM recent r
    GROUP BY r.created_at, COALESCE(r.examination_id, r.id)
    ORDER BY r.created_at DESC
    LIMIT sqlc.arg(row_limit)::int
)
SELECT
    g.id::uuid AS id,
    (CASE
        WHEN 'EXAMINATION_CREATED' = ANY(g.event_types)
         AND COALESCE(e.mode::text, g.meta_mode) = 'REMOTE'          THEN 'REMOTE_SENT'
        WHEN 'EXAMINATION_CREATED' = ANY(g.event_types)              THEN 'CLINIC_STARTED'
        WHEN 'ASSIGNMENT_REVIEWED' = ANY(g.event_types)
          OR 'ASSIGNMENT_COMPLETED' = ANY(g.event_types)             THEN 'TEST_REVIEWED'
        WHEN 'ASSIGNMENT_READY_FOR_REVIEW' = ANY(g.event_types)      THEN 'TEST_FINISHED'
        WHEN 'ASSIGNMENT_STARTED' = ANY(g.event_types)               THEN 'TEST_STARTED'
        WHEN 'ASSIGNMENT_CREATED' = ANY(g.event_types)               THEN 'TESTS_ASSIGNED'
        WHEN 'LINK_SENT' = ANY(g.event_types)                        THEN 'LINK_SENT'
        WHEN 'PATIENT_CREATED' = ANY(g.event_types)                  THEN 'PATIENT_ADDED'
        ELSE 'OTHER'
    END)::text                                                       AS kind,
    (g.event_types[1])::text                                         AS raw_type,
    COALESCE(u.username, '')::text                                   AS actor_username,
    COALESCE(g.actor_id = sqlc.narg(viewer_id)::uuid, FALSE)::bool   AS actor_is_viewer,
    COALESCE(p.full_name, g.meta_mrn, '')::text                      AS patient_name,
    g.patient_id::uuid                                               AS patient_id,
    g.examination_id::uuid                                           AS examination_id,
    COALESCE(
        g.created_tests,
        CASE WHEN a.test_code IS NULL THEN '{}'::text[] ELSE ARRAY[a.test_code::text] END
    )::text[]                                                        AS tests,
    COALESCE(e.mode::text, g.meta_mode, '')::text                    AS mode,
    g.created_at
FROM grouped g
LEFT JOIN users        u ON u.id = g.actor_id
LEFT JOIN patients     p ON p.id = g.patient_id
LEFT JOIN examinations e ON e.id = g.examination_id
LEFT JOIN assignments  a ON a.id = g.assignment_id
ORDER BY g.created_at DESC;
