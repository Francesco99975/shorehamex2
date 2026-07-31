-- test_definitions.sql
--
-- sqlc queries for the test_definitions table (the instrument catalog).
-- Covers two consumers:
--   1. The clinician-facing "Test catalog" view from the mockup (grid
--      of active instruments, click-through to a detail view) and the
--      test picker used when assigning a test to a patient.
--   2. A Developer-role CRUD screen for managing the catalog itself —
--      not shown in the mockup, but requested up front, so the create/
--      update/delete/reactivate shapes below are built for that now
--      rather than bolted on later.
--
-- Future-proofing notes:
--   - List queries take LIMIT/OFFSET even though the catalog is small
--     today (4-5 rows) — cheap to include now, and a Developer CRUD
--     screen listing every instrument (active + inactive) is exactly
--     the kind of view that quietly grows over time.
--   - "Metadata" edits (name/domain/scoring_method/requires_sex/
--     item_count/typical_duration_minutes) are a separate query from
--     "source" refreshes (source_path/source_checksum/version). These
--     are two different workflows in the Developer UI — editing catalog
--     facts by hand vs. re-importing a JSON file — and conflating them
--     would make it easy to accidentally bump `version` on a plain
--     metadata edit, or vice versa.
--   - `code` is intentionally never updatable (it's the FK target for
--     assignments.test_code). If a code genuinely needs to change,
--     that's a new row + migrating assignments, not an UPDATE.
--   - DeleteTestDefinition is a real hard delete, left in for
--     completeness, but assignments.test_code has ON DELETE RESTRICT —
--     it will fail with a foreign key violation for any instrument
--     that's ever been assigned. CountTestDefinitionAssignments exists
--     so the Developer UI can check this up front and steer the user
--     toward SetTestDefinitionActive (soft retire) instead of letting
--     them hit a raw DB error.

-- name: GetTestDefinition :one
-- Full row for one instrument. Used for the catalog detail view and by
-- the assignment flow to check is_active/requires_sex/scoring_method
-- before letting a clinician assign it.
SELECT * FROM test_definitions
WHERE code = $1;

-- name: ListTestDefinitions :many
SELECT
  td.*,
  COUNT(a.test_code)::bigint AS assignment_count
FROM test_definitions td
LEFT JOIN assignments a ON a.test_code = td.code
WHERE
  (sqlc.narg(active_only)::bool IS NULL OR td.is_active = sqlc.narg(active_only))
  AND (sqlc.narg(domain)::test_domain IS NULL OR td.domain = sqlc.narg(domain)::test_domain)
GROUP BY td.code          -- adjust to your PK
ORDER BY td.domain, td.name
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);


-- name: SearchActiveTestDefinitions :many
-- Matches the top-nav search bar's placeholder text ("Search patients,
-- tests…"), which isn't actually wired to anything in the static
-- mockup yet. Trigram/ILIKE match on name or code so it works for
-- partial, misspelled, or code-only queries (e.g. typing "bdi").
SELECT * FROM test_definitions
WHERE is_active
  AND (name ILIKE '%' || sqlc.arg(query) || '%' OR code ILIKE '%' || sqlc.arg(query) || '%')
ORDER BY name
LIMIT sqlc.arg(row_limit);

-- name: CreateTestDefinition :one
-- Adds a new instrument row after its JSON file has been authored and
-- placed at source_path. version starts at 1; is_active defaults to
-- TRUE (a newly-added instrument should be immediately assignable
-- unless the Developer explicitly deactivates it).
INSERT INTO test_definitions (
    code, name, domain, scoring_method, requires_sex,
    item_count, typical_duration_minutes,
    source_path, source_checksum
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
)
RETURNING *,
  (SELECT COUNT(*)::bigint FROM test_definitions) AS total_count;

-- name: UpdateTestDefinitionMetadata :one
-- Edits the catalog-facing facts a Developer would change by hand in
-- the CRUD screen. Deliberately excludes `code` (immutable) and the
-- source_path/source_checksum/version trio (see RefreshTestDefinitionSource) —
-- these are two different edit workflows in the UI, not one form.
UPDATE test_definitions
SET
    name = $2,
    domain = $3,
    scoring_method = $4,
    requires_sex = $5,
    item_count = $6,
    typical_duration_minutes = $7
WHERE code = $1
RETURNING *;

-- name: RefreshTestDefinitionSource :one
-- Called after re-importing an instrument's JSON file (content changed,
-- even if the catalog metadata above didn't). Always bumps `version` by
-- 1, so a Developer glancing at the row can tell the file was
-- re-imported without needing to diff checksums by hand.
UPDATE test_definitions
SET
    source_path = $2,
    source_checksum = $3,
    version = version + 1
WHERE code = $1
RETURNING *;

-- name: SetTestDefinitionActive :one
-- Retire/reactivate toggle — a quick action on the CRUD screen's list
-- (e.g. a switch on each row), separate from the full edit form. Retiring
-- an instrument doesn't touch its history: existing assignments and
-- results referencing this code are untouched, it just stops appearing
-- in ListActiveTestDefinitions and can't be picked for new assignments.
UPDATE test_definitions
SET is_active = $2
WHERE code = $1
RETURNING *;

-- name: CountTestDefinitionAssignments :one
-- How many assignments reference this instrument. The Developer CRUD
-- screen should call this before offering a hard delete — a non-zero
-- count means DeleteTestDefinition will fail on the FK, and the UI
-- should steer toward SetTestDefinitionActive(false) instead.
SELECT COUNT(*) FROM assignments
WHERE test_code = $1;

-- name: DeleteTestDefinition :exec
-- Hard delete. Will raise a foreign key violation if any assignment
-- has ever referenced this code (assignments.test_code is
-- ON DELETE RESTRICT, intentionally — deleting an instrument out from
-- under historical results would silently corrupt them). Only safe to
-- call when CountTestDefinitionAssignments returns 0; otherwise use
-- SetTestDefinitionActive to retire it instead.
DELETE FROM test_definitions
WHERE code = $1;
