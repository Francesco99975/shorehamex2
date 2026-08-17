-- patients.sql
--
-- sqlc queries for the patients table.
--
-- Visibility/CRUD rule (see is_developer_data on the patients table,
-- and the trigger that sets it): a strict partition, not "developers
-- see everything" —
--     is_developer_data = is_developer_user(viewer_id)
-- ADMIN/USER viewers only ever see, edit, or delete real patients
-- (is_developer_data = FALSE). DEVELOPER viewers only ever see, edit,
-- or delete developer-flagged patients (TRUE) — any developer, not
-- just the one who created a given row, but NEVER real patient data.
-- Neither role can reach into the other's universe, for any operation,
-- including delete — this is deliberate, so a developer account used
-- to debug in prod is structurally incapable of touching real patient
-- data, not just discouraged from it.
--
-- This is enforced at the query layer, not via a DB-level policy (no
-- RLS) — every query below applies it explicitly, including writes, so
-- a viewer can't reach a patient outside their partition by guessing/
-- reusing an id either. That means it only protects you if every code
-- path that touches `patients` goes through these queries.
--
-- Both SetPatientActive (soft archive) and DeletePatient (hard delete)
-- are unrestricted within a viewer's own partition — ADMIN/USER can
-- freely hard-delete real patients, same as a developer can freely
-- hard-delete their own team's test patients. Worth flagging since this
-- removes any append-only/retention protection on real patient
-- records: if that's not actually wanted for real clinical data, say so
-- and DeletePatient can be scoped to developer-only instead, or
-- dropped entirely in favor of SetPatientActive as the only removal path.

-- name: GetPatient :one
SELECT * FROM patients
WHERE id = $1
  AND is_developer_data = is_developer_user(sqlc.arg(viewer_id));

-- name: GetPatientByMRN :one
-- For intake lookups / duplicate-MRN checks before creating a new record.
SELECT * FROM patients
WHERE mrn = $1
  AND is_developer_data = is_developer_user(sqlc.arg(viewer_id));

-- name: ListPatients :many
-- Main patient directory. Alphabetical by name; swap to
-- last_activity_at DESC if the roster should read more like a worklist
-- than a directory — a judgment call, easy to flip either way.
SELECT * FROM patients
WHERE is_active
  AND is_developer_data = is_developer_user(sqlc.arg(viewer_id))
ORDER BY full_name
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: ListPatientsByStatus :many
-- e.g. everything currently READY_FOR_REVIEW, for a "needs your
-- attention" view.
SELECT * FROM patients
WHERE is_active
  AND computed_status = $1
  AND is_developer_data = is_developer_user(sqlc.arg(viewer_id))
ORDER BY last_activity_at DESC NULLS LAST
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: SearchPatients :many
-- Matches the mockup's client-side search (name or id substring match),
-- server-side. Uses the full_name trigram index; also matches on mrn
-- for "type in the chart number" lookups.
-- Supports pagination via page (1-based) and limit (items per page).
SELECT * FROM patients
WHERE is_active
AND is_developer_data = is_developer_user(sqlc.arg(viewer_id))
AND (full_name ILIKE '%' || sqlc.arg(query) || '%' OR mrn ILIKE '%' || sqlc.arg(query) || '%')
ORDER BY full_name
LIMIT sqlc.arg(row_limit)
OFFSET (sqlc.arg(page) - 1) * sqlc.arg(row_limit);

-- name: CountActivePatients :one
-- Backs a dashboard "Active patients" stat. For a developer viewer,
-- this counts only their team's developer-flagged patients — real
-- patients are entirely outside a developer's view, by design.
SELECT COUNT(*) FROM patients
WHERE is_active
  AND is_developer_data = is_developer_user(sqlc.arg(viewer_id));

-- name: CreatePatient :one
-- is_developer_data is NOT a column here — it's stamped automatically
-- by trigger_mark_patient_developer_data based on created_by's role, so
-- there's no path for the app to get this wrong or forget to set it,
-- and no path for a real user to accidentally create a patient that
-- lands in the developer partition or vice versa.
INSERT INTO patients (
    id, mrn, full_name, email, phone, date_of_birth, sex, created_by
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
)
RETURNING *;

-- name: UpdatePatientDetails :one
UPDATE patients
SET
    full_name = $2,
    email = $3,
    phone = $4,
    date_of_birth = $5,
    sex = $6
WHERE mrn = $1
  AND is_developer_data = is_developer_user(sqlc.arg(viewer_id))
RETURNING *;

-- name: SetPatientActive :one
-- Archive/reactivate — the safe, reversible removal path.
UPDATE patients
SET is_active = $2
WHERE id = $1
  AND is_developer_data = is_developer_user(sqlc.arg(viewer_id))
RETURNING *;

-- name: DeletePatient :exec
-- Hard delete. Scoped only by the same partition rule as every other
-- query here — NOT restricted to developer-owned rows. An ADMIN/USER
-- can hard-delete a real patient; a developer can hard-delete a
-- developer-flagged one. Cascades to that patient's assignments
-- (ON DELETE CASCADE), which in turn cascade to their results — see the
-- header comment if real, permanent removal of clinical records isn't
-- actually the intended behavior for the ADMIN/USER side of this.
DELETE FROM patients
WHERE mrn = $1
  AND is_developer_data = is_developer_user(sqlc.arg(viewer_id));
