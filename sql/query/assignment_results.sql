-- assignment_results.sql
--
-- sqlc queries covering all four results-related tables:
--   assignment_results          (generic envelope: responses, raw_scores,
--                                 duration_seconds, scored_at)
--   assignment_sum_band_results (SUM_BANDED display fields — ASQ/BAI/BDI/P3)
--   assignment_mmpi_scales      (MMPI category/scale/subscale tree)
--   assignment_mmpi_indications (MMPI per-category narrative bullets)
--
-- Write/read split, different from patients.sql/assignments.sql:
--
--   WRITE queries here (UpdateAssignmentResponses,
--   FinalizeAssignmentResultScoring, CreateSumBandResult,
--   CreateMMPIScales, CreateMMPIIndications) have NO viewer_id/partition
--   check. These aren't a clinician managing a record through the app's
--   role system — they're the answer-submission and scoring pipeline
--   itself: a patient answering (remote via a redeemed token, or
--   in-clinic on a handed-over device) and the scoring engine writing
--   the outcome once every item is in. There's no meaningful "viewer"
--   in either of those code paths, same reasoning as
--   MarkAssignmentReadyForReview in assignments.sql. They're keyed
--   purely by assignment_id / assignment_result_id, which the calling
--   code already resolved through a trusted path (a consumed
--   remote_access_token, or an assignment a staff member is actively
--   administering in-clinic).
--
--   CreateAssignmentResult is the one exception — it's paired 1:1 with
--   assignments.CreateAssignment, which IS a staff-initiated action, so
--   it carries the same viewer_id guard for consistency with that call.
--
--   READ queries (GetAssignmentResult, GetSumBandResultForAssignment,
--   ListMMPIScalesForResult, ListMMPIIndicationsForResult) are exactly
--   what a clinician browses in the UI, so these DO carry the usual
--   inherited partition check:
--       (a.is_developer_data OR p.is_developer_data) = is_developer_user(viewer_id)
--
-- CreateMMPIScales / CreateMMPIIndications are bulk inserts (a whole
-- category/scale tree, or a whole set of indications, in one round
-- trip) using jsonb_to_recordset() over a single JSONB array parameter.
--
-- An earlier version of this file used unnest() over N parallel array
-- parameters instead. That's invalid for sqlc's own built-in query
-- analyzer (the one used without a live Postgres connection for type-
-- checking): `unnest(a, b, c, ...)` with multiple arguments in a FROM
-- clause is special-cased PARSER behavior in real Postgres — it zips
-- the arrays into rows — not a literal function overload sitting in
-- pg_catalog that takes N arguments. sqlc's analyzer does a plain
-- "does a function with this name/arity exist" lookup and finds
-- nothing, hence `function unnest(unknown, unknown, ...) does not
-- exist`. jsonb_to_recordset() is an ordinary single-argument function,
-- so it doesn't hit that gap — and it's arguably safer anyway: one
-- JSON blob instead of N same-length arrays that could silently drift
-- out of sync with each other.
--
-- For assignment_mmpi_scales, self-referencing parent_scale_id works
-- fine even when a parent and its subscales are inserted in the very
-- same statement: for a NOT DEFERRABLE foreign key (the default, used
-- throughout this schema), Postgres checks the constraint once at the
-- END of the statement, after every row from it is already in the
-- table — not per-row as each is inserted. So a subscale's
-- parent_scale_id can reference a sibling row from the same batch
-- regardless of its position in the JSON array. (Verified against
-- Postgres's documented FK check timing, not assumed.)
--
-- Also included: GetAssignmentResultForTaking (the patient-facing
-- counterpart to UpdateAssignmentResponses — fetching progress, not
-- just writing it) and ListScaleScoreHistoryForPatient (the scale-trend
-- query assignment_mmpi_scales' schema comment promised but was never
-- actually written).

-- name: CreateAssignmentResult :one
-- Paired with assignments.CreateAssignment, same transaction, same
-- actor as viewer_id. responses starts empty; everything else stays
-- NULL until the patient answers and the engine scores it.
INSERT INTO assignment_results (
    id, assignment_id
)
SELECT sqlc.arg(id), a.id
FROM assignments a
JOIN patients p ON p.id = a.patient_id
WHERE a.id = sqlc.arg(assignment_id)
  AND (a.is_developer_data OR p.is_developer_data) = is_developer_user(sqlc.arg(viewer_id))
RETURNING *;

-- name: GetAssignmentResult :one
SELECT ar.* FROM assignment_results ar
JOIN assignments a ON a.id = ar.assignment_id
JOIN patients p ON p.id = a.patient_id
WHERE ar.assignment_id = $1
  AND (a.is_developer_data OR p.is_developer_data) = is_developer_user(sqlc.arg(viewer_id));

-- name: GetAssignmentResultForTaking :one
-- Patient-facing: fetch current responses/progress to resume an
-- in-progress assessment (remote or in-clinic). No viewer_id — see
-- header comment.
SELECT * FROM assignment_results
WHERE assignment_id = $1;

-- name: UpdateAssignmentResponses :one
-- Merges newly-answered items into responses (item_key -> answer).
-- Fires mark_assignment_started_on_response (assignments.sql's header
-- comment) as a side effect — that trigger flips ASSIGNED -> IN_PROGRESS
-- and sets started_at the first time this runs for an assignment.
-- No viewer_id — see header comment.
UPDATE assignment_results
SET responses = responses || sqlc.arg(new_responses)::jsonb
WHERE assignment_id = $1
RETURNING *;

-- name: FinalizeAssignmentResultScoring :one
-- Called by the scoring engine once every item is answered, before it
-- writes the engine-specific rows below. Generic to both SUM_BANDED and
-- MULTISCALE_PROFILE. duration_seconds per the server-timestamp-only
-- mechanism discussed separately (assignment_page_timings). No
-- viewer_id — see header comment.
UPDATE assignment_results
SET raw_scores = sqlc.arg(raw_scores)::jsonb,
    duration_seconds = sqlc.arg(duration_seconds),
    scored_at = NOW()
WHERE assignment_id = $1
RETURNING *;

-- name: CreateSumBandResult :one
-- The SUM_BANDED-specific display fields (ASQ/BAI/BDI/P3). Called right
-- after FinalizeAssignmentResultScoring for those tests only — MMPI
-- results use CreateMMPIScales/CreateMMPIIndications instead. No
-- viewer_id — see header comment.
INSERT INTO assignment_sum_band_results (
    assignment_result_id, headline_score, numeric_score, scale_label,
    severity_label, interpretation_summary
) VALUES (
    $1, $2, $3, $4, $5, $6
)
RETURNING *;

-- name: GetSumBandResultForAssignment :one
-- The single most useful read for rendering a SUM_BANDED result screen:
-- joins straight from assignment_id to the envelope and the display
-- fields, no need to look up assignment_result_id first.
SELECT ar.*, sbr.headline_score, sbr.numeric_score, sbr.scale_label,
       sbr.severity_label, sbr.interpretation_summary
FROM assignment_results ar
JOIN assignment_sum_band_results sbr ON sbr.assignment_result_id = ar.id
JOIN assignments a ON a.id = ar.assignment_id
JOIN patients p ON p.id = a.patient_id
WHERE ar.assignment_id = $1
  AND (a.is_developer_data OR p.is_developer_data) = is_developer_user(sqlc.arg(viewer_id));

-- name: CreateMMPIScales :many
-- Bulk insert for one MMPI result's full category/scale/subscale tree
-- in one round trip. sqlc.arg(scales) is a single JSONB array
-- parameter, one object per scale/subscale row, e.g.:
--   [
--     {"id": "...", "assignment_result_id": "...", "parent_scale_id": null,
--      "category_title": "Validity Scales", "scale_code": "VRIN",
--      "scale_name": "VRIN", "scale_description": "...", "scale_purpose": "...",
--      "raw_score": 3, "score": 45, "display_order": 0},
--     ...
--   ]
-- Position within the array doesn't matter FK-wise — see header comment
-- on why the self-reference is safe within one statement regardless of
-- ordering. No viewer_id — see header comment.
INSERT INTO assignment_mmpi_scales (
    id, assignment_result_id, parent_scale_id, category_title, scale_code,
    scale_name, scale_description, scale_purpose, raw_score, score, display_order
)
SELECT
    id, assignment_result_id, parent_scale_id, category_title, scale_code,
    scale_name, scale_description, scale_purpose, raw_score, score, display_order
FROM jsonb_to_recordset(sqlc.arg(scales)::jsonb) AS x(
    id UUID,
    assignment_result_id UUID,
    parent_scale_id UUID,
    category_title TEXT,
    scale_code TEXT,
    scale_name TEXT,
    scale_description TEXT,
    scale_purpose TEXT,
    raw_score NUMERIC,
    score NUMERIC,
    display_order SMALLINT
)
RETURNING *;

-- name: CreateMMPIIndications :many
-- Bulk insert for one MMPI result's full set of per-category narrative
-- indications. Same jsonb_to_recordset() pattern as CreateMMPIScales —
-- sqlc.arg(indications) is a single JSONB array, one object per
-- indication line. No viewer_id — see header comment.
INSERT INTO assignment_mmpi_indications (
    id, assignment_result_id, category_title, indication_text, display_order
)
SELECT
    id, assignment_result_id, category_title, indication_text, display_order
FROM jsonb_to_recordset(sqlc.arg(indications)::jsonb) AS x(
    id UUID,
    assignment_result_id UUID,
    category_title TEXT,
    indication_text TEXT,
    display_order SMALLINT
)
RETURNING *;

-- name: ListMMPIScalesForResult :many
-- Flat, ordered fetch of every scale/subscale row for one result. The
-- app reconstructs the category/parent-child tree client-side from
-- category_title + parent_scale_id + id — display_order already
-- reflects the intended render sequence.
SELECT ms.* FROM assignment_mmpi_scales ms
JOIN assignment_results ar ON ar.id = ms.assignment_result_id
JOIN assignments a ON a.id = ar.assignment_id
JOIN patients p ON p.id = a.patient_id
WHERE ms.assignment_result_id = $1
  AND (a.is_developer_data OR p.is_developer_data) = is_developer_user(sqlc.arg(viewer_id))
ORDER BY ms.display_order;

-- name: ListMMPIIndicationsForResult :many
SELECT mi.* FROM assignment_mmpi_indications mi
JOIN assignment_results ar ON ar.id = mi.assignment_result_id
JOIN assignments a ON a.id = ar.assignment_id
JOIN patients p ON p.id = a.patient_id
WHERE mi.assignment_result_id = $1
  AND (a.is_developer_data OR p.is_developer_data) = is_developer_user(sqlc.arg(viewer_id))
ORDER BY mi.display_order;

-- name: ListScaleScoreHistoryForPatient :many
-- One scale's score across every MMPI assignment a patient has ever
-- had, oldest first — for a "D scale over time" trend chart. This is
-- the query assignment_mmpi_scales' schema comment promised ("useful
-- for trending a single scale... over time") but that was never
-- actually written until now.
SELECT
    a.id AS assignment_id,
    a.completed_at,
    ms.scale_code,
    ms.scale_name,
    ms.score
FROM assignment_mmpi_scales ms
JOIN assignment_results ar ON ar.id = ms.assignment_result_id
JOIN assignments a ON a.id = ar.assignment_id
JOIN patients p ON p.id = a.patient_id
WHERE a.patient_id = $1
  AND ms.scale_code = $2
  AND (a.is_developer_data OR p.is_developer_data) = is_developer_user(sqlc.arg(viewer_id))
ORDER BY a.completed_at;
