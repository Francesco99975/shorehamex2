-- =============================================================================
-- Dashboard queries
--
-- Developer scoping: every query takes the nullable sqlc.narg(viewer_id).
--   viewer_id IS NULL -> no scoping (system / internal callers)
--   otherwise         -> <table>.is_developer_data = is_developer_user(viewer_id)
-- so developers see only developer data and everyone else only real data.
--
-- Three round trips per page render, all independent (run them concurrently
-- with errgroup or a pgx.Batch if you like):
--     GetDashboardCounts   -> DashboardCounts (stat cards)
--     ListAttentionItems   -> []AttentionItemProps
--     GetWeeklyCompletion  -> WeeklyCompletionProps.Days (always 7 rows)
-- =============================================================================


-- name: GetDashboardCounts :one
-- One pass each over patients, in-progress examinations and the "hot" slice
-- of assignments (awaiting review or completed in the last 14 days),
-- aggregated with FILTER.
--
-- active_patients      : every active (non-archived) patient.
-- new_patients_week    : active patients created since @week_start
--                        (Monday 00:00 in the clinic timezone, computed in Go).
-- awaiting_review      : assignments READY_FOR_REVIEW.
-- in_progress(_remote) : examinations IN_PROGRESS (remote = mode REMOTE).
-- completed_7d / prev  : assignments whose completed_at falls in the last
--                        7 days / the 7 days before that. Delta % is done in Go.
WITH pt AS (
    SELECT
        COUNT(*) AS active_patients,
        COUNT(*) FILTER (
            WHERE p.created_at >= sqlc.arg(week_start)::timestamptz
        ) AS new_patients_week
    FROM patients p
    WHERE p.is_active
      AND (
          sqlc.narg(viewer_id)::uuid IS NULL
          OR p.is_developer_data = is_developer_user(sqlc.narg(viewer_id)::uuid)
      )
),
exm AS (
    SELECT
        COUNT(*)                                  AS in_progress,
        COUNT(*) FILTER (WHERE e.mode = 'REMOTE') AS in_progress_remote
    FROM examinations e
    WHERE e.status = 'IN_PROGRESS'
      AND (
          sqlc.narg(viewer_id)::uuid IS NULL
          OR e.is_developer_data = is_developer_user(sqlc.narg(viewer_id)::uuid)
      )
),
asg AS (
    SELECT
        COUNT(*) FILTER (WHERE a.status = 'READY_FOR_REVIEW')                   AS awaiting_review,
        COUNT(*) FILTER (WHERE a.completed_at >= now() - INTERVAL '7 days')     AS completed_7d,
        COUNT(*) FILTER (
            WHERE a.completed_at >= now() - INTERVAL '14 days'
              AND a.completed_at <  now() - INTERVAL '7 days'
        )                                                                       AS completed_prev_7d
    FROM assignments a
    WHERE (
            a.status = 'READY_FOR_REVIEW'
         OR a.completed_at >= now() - INTERVAL '14 days'
      )
      AND (
          sqlc.narg(viewer_id)::uuid IS NULL
          OR a.is_developer_data = is_developer_user(sqlc.narg(viewer_id)::uuid)
      )
)
SELECT
    pt.active_patients::int     AS active_patients,
    pt.new_patients_week::int   AS new_patients_week,
    asg.awaiting_review::int    AS awaiting_review,
    exm.in_progress::int        AS in_progress,
    exm.in_progress_remote::int AS in_progress_remote,
    asg.completed_7d::int       AS completed_7d,
    asg.completed_prev_7d::int  AS completed_prev_7d
FROM pt, exm, asg;


-- name: ListAttentionItems :many
-- "Needs your attention" = two kinds of rows, merged and limited in SQL:
--
--   REVIEW  : one row per assignment READY_FOR_REVIEW. Action = Review.
--   OVERDUE : one row per examination (REMOTE or IN_CLINIC) past due_at that
--             still has unfinished work (an assignment ASSIGNED / IN_PROGRESS).
--             Exams whose patient finished everything but are waiting on
--             review are NOT overdue — they already appear as REVIEW rows.
--             Action = Remind (REMOTE) / Resume (IN_CLINIC), decided by `mode`.
--
-- Columns are identical across both branches so the UNION is flat:
--   item_id              : assignment id (REVIEW) / examination id (OVERDUE)
--   target_assignment_id : assignment the row links to — itself (REVIEW) /
--                          first unfinished by sequence (OVERDUE, used by Resume)
--   test_codes           : 'DASS-42' (REVIEW) / 'DASS-42, PCL-C' unfinished (OVERDUE)
--   status               : assignment status (OVERDUE: first unfinished by sequence)
--   event_at             : completed_at (REVIEW) / due_at (OVERDUE) -> note + sort
--   link_usable          : active, unexpired remote token exists (REMOTE OVERDUE only;
--                          FALSE means the Remind handler must issue a new link)
--   last_reminder_at     : newest assignment_reminders.sent_at (REMOTE OVERDUE only)
--
-- Sort: REVIEW first, then oldest event first (longest-waiting on top).
WITH review AS (
    SELECT
        'REVIEW'::text                 AS kind,
        a.id                           AS item_id,
     a.id                           AS target_assignment_id,
        e.id                           AS examination_id,
        p.full_name                    AS patient_name,
        a.test_code::text              AS test_codes,
        a.status                       AS status,
        e.mode                         AS mode,
        FALSE                          AS overdue,
        a.completed_at::timestamptz    AS event_at,
        FALSE                          AS link_usable,
        NULL::timestamptz              AS last_reminder_at
    FROM assignments a
    JOIN examinations e ON e.id = a.examination_id
    JOIN patients     p ON p.id = e.patient_id
    WHERE a.status = 'READY_FOR_REVIEW'
      AND (
          sqlc.narg(viewer_id)::uuid IS NULL
          OR a.is_developer_data = is_developer_user(sqlc.narg(viewer_id)::uuid)
      )
),
overdue_exams AS (
    SELECT
        e.id,
        e.patient_id,
        e.mode,
        e.due_at,
        string_agg(a.test_code, ', ' ORDER BY a.sequence)  AS test_codes,
        (array_agg(a.status ORDER BY a.sequence))[1]        AS status,
        (array_agg(a.id     ORDER BY a.sequence))[1]        AS first_open_assignment_id
    FROM examinations e
    JOIN assignments a
      ON a.examination_id = e.id
     AND a.status IN ('ASSIGNED', 'IN_PROGRESS')
    WHERE e.status <> 'COMPLETED'          -- matches idx_examinations_overdue
      AND e.due_at < now()
      AND (
          sqlc.narg(viewer_id)::uuid IS NULL
          OR e.is_developer_data = is_developer_user(sqlc.narg(viewer_id)::uuid)
      )
    GROUP BY e.id
),
overdue AS (
    SELECT
        'OVERDUE'::text                                     AS kind,
        oe.id                                               AS item_id,
        oe.first_open_assignment_id                         AS target_assignment_id,
        oe.id                                               AS examination_id,
        p.full_name                                         AS patient_name,
        oe.test_codes::text                                 AS test_codes,
        oe.status                                           AS status,
        oe.mode                                             AS mode,
        TRUE                                                AS overdue,
        oe.due_at::timestamptz                              AS event_at,
        COALESCE(tok.expires_at > now(), FALSE)             AS link_usable,
        rem.last_sent_at::timestamptz                       AS last_reminder_at
    FROM overdue_exams oe
    JOIN patients p ON p.id = oe.patient_id
    -- Token/reminder lookups only run for REMOTE exams; IN_CLINIC rows get NULLs.
    -- At most one row thanks to the idx_remote_access_tokens_active unique index.
    LEFT JOIN LATERAL (
        SELECT t.expires_at
        FROM remote_access_tokens t
        WHERE oe.mode = 'REMOTE'
          AND t.examination_id = oe.id
          AND t.used_at IS NULL
          AND NOT t.revoked
        LIMIT 1
    ) tok ON TRUE
    LEFT JOIN LATERAL (
        SELECT MAX(r.sent_at) AS last_sent_at
        FROM assignment_reminders r
        WHERE oe.mode = 'REMOTE'
          AND r.examination_id = oe.id
    ) rem ON TRUE
),
items AS (
    SELECT * FROM review
    UNION ALL
    SELECT * FROM overdue
)
SELECT
    i.kind,
    i.item_id,
    i.target_assignment_id,
    i.examination_id,
    i.patient_name,
    i.test_codes,
    i.status,
    i.mode,
    i.overdue,
    i.event_at,
    i.link_usable,
    i.last_reminder_at,
    (COUNT(*) OVER ())::int AS total_count
FROM items i
ORDER BY
    CASE i.kind WHEN 'REVIEW' THEN 0 ELSE 1 END,
    i.event_at ASC NULLS LAST
LIMIT sqlc.arg(row_limit)::int;


-- name: GetWeeklyCompletion :many
-- Always returns exactly 7 rows (day_offset 0 = Monday ... 6 = Sunday) for
-- the week starting at @week_start, bucketed in the clinic timezone @tz
-- (e.g. 'America/Toronto').
--
-- assigned  : assignments whose parent examination is DUE that day.
-- completed : of those, how many the patient has finished (completed_at set),
--             so the per-day ratio can never exceed 100%.
--
-- The range predicate on e.due_at uses idx_examinations_due_at; the
-- date bucketing happens only on the (small) matching set.
WITH days AS (
    SELECT generate_series(0, 6)::int AS day_offset
),
due AS (
    SELECT
        (
            (e.due_at AT TIME ZONE sqlc.arg(tz)::text)::date
          - (sqlc.arg(week_start)::timestamptz AT TIME ZONE sqlc.arg(tz)::text)::date
        )::int          AS day_offset,
        a.completed_at
    FROM examinations e
    JOIN assignments a ON a.examination_id = e.id
    WHERE e.due_at >= sqlc.arg(week_start)::timestamptz
      AND e.due_at <  sqlc.arg(week_start)::timestamptz + INTERVAL '7 days'
      AND (
          sqlc.narg(viewer_id)::uuid IS NULL
          OR e.is_developer_data = is_developer_user(sqlc.narg(viewer_id)::uuid)
      )
)
SELECT
    d.day_offset,
    COUNT(due.day_offset)::int   AS assigned,
    COUNT(due.completed_at)::int AS completed
FROM days d
LEFT JOIN due ON due.day_offset = d.day_offset
GROUP BY d.day_offset
ORDER BY d.day_offset;


-- -----------------------------------------------------------------------------
-- Optional supporting indexes (add in a new migration if the tables grow):
--
--   -- completed_7d / completed_prev_7d range scan
--   CREATE INDEX idx_assignments_completed_at
--       ON assignments (completed_at) WHERE completed_at IS NOT NULL;
--
--   -- REVIEW branch + awaiting_review count
--   CREATE INDEX idx_assignments_ready_for_review
--       ON assignments (is_developer_data, completed_at)
--       WHERE status = 'READY_FOR_REVIEW';
--
--   -- newest reminder per examination (LATERAL MAX)
--   CREATE INDEX idx_assignment_reminders_exam_sent
--       ON assignment_reminders (examination_id, sent_at DESC);
-- -----------------------------------------------------------------------------
