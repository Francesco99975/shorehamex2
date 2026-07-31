-- Create a trigger function to update the updated column
CREATE OR REPLACE FUNCTION update_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Create a macro to apply the trigger to a table
CREATE OR REPLACE FUNCTION apply_update_trigger(table_name TEXT)
RETURNS VOID AS $$
BEGIN
 IF NOT EXISTS (
    SELECT 1
    FROM information_schema.triggers
    WHERE trigger_schema = 'public'
      AND trigger_name = format('trigger_update_updated_at_%I', table_name)
  ) THEN
    EXECUTE format('
        CREATE TRIGGER trigger_update_updated_at_%I
        BEFORE UPDATE ON %I
        FOR EACH ROW
        EXECUTE FUNCTION update_updated_at()
    ', table_name, table_name);
  END IF;
END;
$$ LANGUAGE plpgsql;

-- Enable trigram extension
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- ---------------------------------------------------------------------
-- Enums
-- ---------------------------------------------------------------------
CREATE TYPE test_domain AS ENUM ('DEPRESSION', 'ANXIETY', 'SOMATIC', 'PERSONALITY');

-- 2 different scoring engines:
--   SUM_BANDED         -> ASQ, BAI, BDI, P3: one raw score, gravity
--                         ("normal"/"moderate"/"severe") from % of max.
--   MULTISCALE_PROFILE -> MMPI-2: recursive categories/scales/subscales,
--                         gender-specific T-score tables, K-correction,
--                         per-scale narrative indications.
CREATE TYPE scoring_method AS ENUM ('SUM_BANDED', 'MULTISCALE_PROFILE');

-- MMPI-2 T-score tables and gender-restricted scales are keyed off
-- patient sex ("male"/"female" in the legacy engine).
CREATE TYPE sex AS ENUM ('MALE', 'FEMALE');

CREATE TYPE assignment_mode AS ENUM ('IN_CLINIC', 'REMOTE');
CREATE TYPE assignment_status AS ENUM ('ASSIGNED', 'IN_PROGRESS', 'READY_FOR_REVIEW', 'COMPLETED');

-- Roles
CREATE TABLE roles (
    id TEXT PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO roles (id) VALUES
    ('DEVELOPER'),
    ('ADMIN'),
    ('USER')
ON CONFLICT (id) DO NOTHING;

SELECT apply_update_trigger('roles');

-- Users
CREATE TABLE users (
    id UUID PRIMARY KEY,
    role TEXT NOT NULL REFERENCES roles(id) ON DELETE RESTRICT,
    username VARCHAR(21) UNIQUE NOT NULL,
    email TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    full_name TEXT NOT NULL,
    title TEXT NOT NULL DEFAULT 'Clinical Staffer',
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    is_email_verified BOOLEAN NOT NULL DEFAULT FALSE,
    twofa_secret TEXT,
    twofa_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    last_login TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_users_username ON users(username);
-- Username trigram index
CREATE INDEX IF NOT EXISTS idx_users_username_trgm
ON users USING gin (username gin_trgm_ops);

-- Email trigram index
CREATE INDEX IF NOT EXISTS idx_users_email_trgm
ON users USING gin (email gin_trgm_ops);

-- Optional: role index (helps filtering + pagination)
CREATE INDEX IF NOT EXISTS idx_users_role
ON users (role);

SELECT apply_update_trigger('users');

-- Password resets
CREATE TABLE password_resets (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token TEXT UNIQUE NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_password_resets_user_id ON password_resets(user_id);
CREATE INDEX idx_password_resets_token ON password_resets(token);
CREATE INDEX idx_password_resets_expires_at ON password_resets(expires_at);

-- Email verifications
CREATE TABLE email_verifications (
    id UUID PRIMARY KEY,
    email TEXT NOT NULL,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token TEXT UNIQUE NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_email_verifications_user_id ON email_verifications(user_id);
CREATE INDEX idx_email_verifications_token ON email_verifications(token);
CREATE INDEX idx_email_verifications_expires_at ON email_verifications(expires_at);

-- 2FA backup codes
CREATE TABLE twofa_backup_codes (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code_hash TEXT NOT NULL,
    used BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_twofa_backup_codes_user_id ON twofa_backup_codes(user_id);

CREATE TABLE pending_auth_challenges (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    mode TEXT NOT NULL CHECK (mode IN ('totp_registration', 'mfa_challenge')),
    secret TEXT,
    remember_me BOOLEAN NOT NULL DEFAULT FALSE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_pending_auth_challenges_user_id ON pending_auth_challenges(user_id);
CREATE INDEX idx_pending_auth_challenges_expires_at ON pending_auth_challenges(expires_at);

CREATE TABLE sessions (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_token_hash TEXT NOT NULL,
    ip_address TEXT,
    user_agent TEXT,
    remember_me BOOLEAN NOT NULL DEFAULT FALSE,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked BOOLEAN NOT NULL DEFAULT FALSE,
    revoked_at TIMESTAMPTZ,
    last_activity_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_sessions_user_id ON sessions(user_id);
CREATE INDEX idx_sessions_expires_at ON sessions(expires_at);
CREATE UNIQUE INDEX idx_sessions_token_hash_active
    ON sessions(session_token_hash) WHERE revoked = FALSE;

-- ---------------------------------------------------------------------
-- Test catalog. One row per instrument — pure reference data (what a
-- FK needs to point at, what a filter/join needs). Everything else
-- (descriptions, parameters list, scoring rules/items) lives in a JSON
-- file per instrument, loaded into the app at boot — see the shape
-- comment below. That content changes essentially never and reads
-- better reviewed as a file diff/PR than as a DB edit.
-- ---------------------------------------------------------------------
CREATE TABLE test_definitions (
    code TEXT PRIMARY KEY, -- 'ASQ', 'BAI', 'BDI', 'P3', 'MMPI' (matches the Go Exam enum)
    name TEXT NOT NULL, -- "Beck Depression Inventory"
    domain test_domain NOT NULL,
    scoring_method scoring_method NOT NULL,
    requires_sex BOOLEAN NOT NULL DEFAULT FALSE, -- TRUE for MMPI: scoring needs patient.sex
    item_count SMALLINT NOT NULL CHECK (item_count > 0),
    typical_duration_minutes SMALLINT NOT NULL CHECK (typical_duration_minutes > 0),
    is_active BOOLEAN NOT NULL DEFAULT TRUE, -- retire an instrument w/o deleting history
    -- Identifies which JSON file backs this row and whether it's changed.
    -- Bump `version` whenever the file is re-imported so a row visibly
    -- tells you the content may differ from what some old assignment
    -- was scored against (there's no history table — re-importing just
    -- means "this now points at v2 of the file").
    source_path TEXT NOT NULL, -- e.g. 'tests/bdi.json'
    source_checksum TEXT NOT NULL, -- hash of the file, to detect drift/re-import
    version INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
SELECT apply_update_trigger('test_definitions');

-- ---------------------------------------------------------------------
-- Patients. Deliberately not a `users` row: no login, no password, no
-- role — just clinical/contact data plus a couple of cached fields for
-- fast list rendering.
-- ---------------------------------------------------------------------
CREATE TABLE patients (
    id UUID PRIMARY KEY,
    mrn TEXT UNIQUE NOT NULL, -- medical record number; app-supplied, not DB-generated
    full_name TEXT NOT NULL,
    email TEXT,
    phone TEXT,
    date_of_birth DATE, -- store DOB, not age; compute age in queries/app
    -- Nullable at the schema level (only needed for MMPI-2), but the app
    -- should require it before an assignment on a test where
    -- test_definitions.requires_sex is TRUE — calculateTScore() indexes
    -- into a per-sex T-score table and some scales are gender-restricted.
    sex sex,
    -- Pure audit fact: who entered this record. Immutable, and
    -- deliberately not a stand-in for "whose patient is this" — this
    -- clinic doesn't require a patient to be tied to a specific
    -- clinician, so there's no ongoing-ownership field here at all.
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    is_developer_data BOOLEAN NOT NULL DEFAULT FALSE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE, -- soft "archive" instead of hard delete
    -- Cached/derived fields, kept in sync by triggers below, so patient
    -- list/search views don't need to aggregate `assignments` on every read.
    computed_status assignment_status,
    last_activity_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_patients_created_by ON patients(created_by);
CREATE INDEX idx_patients_computed_status ON patients(computed_status);
CREATE INDEX IF NOT EXISTS idx_patients_full_name_trgm
    ON patients USING gin (full_name gin_trgm_ops);
SELECT apply_update_trigger('patients');

-- ---------------------------------------------------------------------
-- Assignments: one instrument assigned to one patient, once.
-- ---------------------------------------------------------------------
CREATE TABLE assignments (
    id UUID PRIMARY KEY,
    patient_id UUID NOT NULL REFERENCES patients(id) ON DELETE CASCADE,
    test_code TEXT NOT NULL REFERENCES test_definitions(code) ON DELETE RESTRICT,
    mode assignment_mode NOT NULL,
    status assignment_status NOT NULL DEFAULT 'ASSIGNED',
    assigned_by UUID REFERENCES users(id) ON DELETE SET NULL,
    due_at TIMESTAMPTZ,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    reviewed_at TIMESTAMPTZ,
    reviewed_by UUID REFERENCES users(id) ON DELETE SET NULL,
    is_developer_data BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_assignments_patient_id ON assignments(patient_id);
CREATE INDEX idx_assignments_status ON assignments(status);
CREATE INDEX idx_assignments_due_at ON assignments(due_at);
CREATE INDEX idx_assignments_test_code ON assignments(test_code);
-- "overdue" in the UI = status not in (COMPLETED) AND due_at < now();
-- this partial index makes that a cheap lookup instead of a full scan.
CREATE INDEX idx_assignments_overdue
    ON assignments(due_at) WHERE status <> 'COMPLETED';
SELECT apply_update_trigger('assignments');

-- ---------------------------------------------------------------------
-- Remote access tokens: single-use secure link, no patient account.
-- Mirrors the password_resets / email_verifications pattern already
-- in the schema (hash the token, never store it plaintext).
-- ---------------------------------------------------------------------
CREATE TABLE remote_access_tokens (
    id UUID PRIMARY KEY,
    assignment_id UUID NOT NULL REFERENCES assignments(id) ON DELETE CASCADE,
    token_hash TEXT UNIQUE NOT NULL,
    sent_to_email TEXT NOT NULL,
    sent_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resend_count INT NOT NULL DEFAULT 0,
    expires_at TIMESTAMPTZ NOT NULL, -- sent_at + 7 days, set by app
    used_at TIMESTAMPTZ,
    revoked BOOLEAN NOT NULL DEFAULT FALSE, -- set true when "Resend link" issues a new one
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_remote_access_tokens_assignment_id ON remote_access_tokens(assignment_id);
-- Only one *active* (usable) token per assignment at a time.
CREATE UNIQUE INDEX idx_remote_access_tokens_active
    ON remote_access_tokens(assignment_id)
    WHERE used_at IS NULL AND NOT revoked;

-- "Send reminder" / "Resend link" clicks, for an audit trail and to
-- drive any "remind again" throttling. Email-only (no channel column) —
-- add one back if a second delivery method is ever needed.
CREATE TABLE assignment_reminders (
    id UUID PRIMARY KEY,
    assignment_id UUID NOT NULL REFERENCES assignments(id) ON DELETE CASCADE,
    sent_by UUID REFERENCES users(id) ON DELETE SET NULL,
    sent_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_assignment_reminders_assignment_id ON assignment_reminders(assignment_id);

-- ---------------------------------------------------------------------
-- Results: one row per assignment, holding BOTH the patient's raw
-- answers (`responses`, filled in progressively as they answer) and the
-- parts of the outcome that are genuinely common to every test, however
-- it's scored. This is deliberately a thin envelope now — the
-- SUM_BANDED-shaped display fields (headline score, severity label,
-- etc.) live in assignment_sum_band_results, and MMPI's category/scale
-- tree lives in assignment_mmpi_scales / assignment_mmpi_indications.
-- Created alongside the assignment row (by the app, with an empty
-- `responses` object) so there's always exactly one row per assignment
-- to update — no upsert-on-first-answer logic needed.
-- ---------------------------------------------------------------------
CREATE TABLE assignment_results (
    id UUID PRIMARY KEY,
    assignment_id UUID UNIQUE NOT NULL REFERENCES assignments(id) ON DELETE CASCADE,
    -- item_key -> answer, e.g. {"1": {"choice": 2}} for a Likert item or
    -- {"12": {"bool": true}} for an MMPI true/false item. For SUM_BANDED
    -- tests, item_key matches definition.items[].key; for MMPI it's the
    -- 1-based question index referenced by Scale.Answers[i][0].
    responses JSONB NOT NULL DEFAULT '{}'::JSONB,
    raw_scores JSONB, -- full scoring engine output snapshot, for debugging/exact re-display; NULL until scored
    -- Active time-on-task, in seconds. NULL until scoring completes.
    -- Derivation is server-timestamp-only (no client-reported values) —
    -- see accompanying write-up for the mechanism before this is wired
    -- up. Lives here, not on assignment_sum_band_results, because it
    -- applies equally to MMPI.
    duration_seconds INT,
    scored_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
SELECT apply_update_trigger('assignment_results');

-- One row per SUM_BANDED assignment result (ASQ, BAI, BDI, P3). Not
-- used for MMPI — see assignment_mmpi_scales / assignment_mmpi_indications.
CREATE TABLE assignment_sum_band_results (
    assignment_result_id UUID PRIMARY KEY REFERENCES assignment_results(id) ON DELETE CASCADE,
    headline_score TEXT NOT NULL, -- "62.86%"
    numeric_score NUMERIC NOT NULL, -- raw (post-adjust) summed score
    scale_label TEXT NOT NULL, -- "% of max score"
    severity_label TEXT NOT NULL, -- gravity: "normal" / "moderate" / "severe"
    interpretation_summary TEXT NOT NULL -- CompileBasicIndication() sentence
);

-- One row per ScaleResult in an MMPI-2 assignment. Mirrors
-- MMPICategoryResult.Scales, including recursive Scale.SubScales via
-- parent_scale_id, so e.g. "D" and its Harris-Lingoes subscales are
-- both queryable and re-displayable in their original hierarchy —
-- useful for trending a single scale (e.g. "D" over time across
-- assignments) too. MMPI-specific (not shared with SUM_BANDED tests) —
-- if a second MULTISCALE_PROFILE-shaped instrument is ever added, this
-- table would need renaming/generalizing at that point, not before.
CREATE TABLE assignment_mmpi_scales (
    id UUID PRIMARY KEY,
    assignment_result_id UUID NOT NULL REFERENCES assignment_results(id) ON DELETE CASCADE,
    parent_scale_id UUID REFERENCES assignment_mmpi_scales(id) ON DELETE CASCADE,
    category_title TEXT NOT NULL, -- MMPICategoryResult.Title
    scale_code TEXT, -- Scale.Code / Name, e.g. 'Hs', 'D', 'VRIN'
    scale_name TEXT NOT NULL, -- ScaleResult.ScaleName
    scale_description TEXT, -- ScaleResult.ScaleDescription (Scale.Title)
    scale_purpose TEXT, -- ScaleResult.ScalePupose (Scale.Text / Comment / "N/A")
    raw_score NUMERIC, -- pre-T-score/pre-K-correction sum, for audit; NULL if not tracked
    score NUMERIC NOT NULL, -- ScaleResult.Score (final T-score or raw score)
    display_order SMALLINT NOT NULL DEFAULT 0
);
CREATE INDEX idx_assignment_mmpi_scales_result_id
    ON assignment_mmpi_scales(assignment_result_id, display_order);
CREATE INDEX idx_assignment_mmpi_scales_parent_id ON assignment_mmpi_scales(parent_scale_id);
CREATE INDEX idx_assignment_mmpi_scales_code ON assignment_mmpi_scales(scale_code);

-- MMPICategoryResult.DerivedIndications: the narrative bullet list
-- ("No particular indications", or the matched indication strings) per
-- category. MMPI-specific, same rationale as assignment_mmpi_scales.
CREATE TABLE assignment_mmpi_indications (
    id UUID PRIMARY KEY,
    assignment_result_id UUID NOT NULL REFERENCES assignment_results(id) ON DELETE CASCADE,
    category_title TEXT NOT NULL,
    indication_text TEXT NOT NULL,
    display_order SMALLINT NOT NULL DEFAULT 0
);
CREATE INDEX idx_assignment_mmpi_indications_result_id
    ON assignment_mmpi_indications(assignment_result_id, display_order);

-- ---------------------------------------------------------------------
-- Activity feed. Store structured events + metadata; render the
-- "Jordan Reyes completed MMPI-2" copy in the app layer, not in SQL.
-- ---------------------------------------------------------------------
CREATE TABLE activity_events (
    id UUID PRIMARY KEY,
    event_type TEXT NOT NULL, -- 'ASSIGNMENT_COMPLETED', 'LINK_SENT', 'PATIENT_ADDED', 'REVIEW_CONFIRMED', ...
    patient_id UUID REFERENCES patients(id) ON DELETE SET NULL,
    assignment_id UUID REFERENCES assignments(id) ON DELETE SET NULL,
    actor_id UUID REFERENCES users(id) ON DELETE SET NULL, -- NULL when the patient triggered it
    -- Stamped once, permanently, by log_activity_event() at insert time —
    -- NOT derived by joining back to patients/assignments at read time.
    -- This matters specifically because patient_id/assignment_id are
    -- ON DELETE SET NULL: once a developer hard-deletes their own test
    -- patient (patients.sql's DeletePatient explicitly allows this),
    -- a join-based check would lose all memory that these leftover
    -- events were ever developer data, silently leaking them into
    -- everyone's view. Caching it here means it survives that deletion.
    is_developer_data BOOLEAN NOT NULL DEFAULT FALSE,
    metadata JSONB NOT NULL DEFAULT '{}'::JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_activity_events_created_at ON activity_events(created_at DESC);
CREATE INDEX idx_activity_events_patient_id ON activity_events(patient_id);

-- ---------------------------------------------------------------------
-- Triggers: keep patients.computed_status / last_activity_at in sync
-- automatically.
-- ---------------------------------------------------------------------

-- Re-derives the patient's overall status from ALL of their
-- assignments (not just whichever row was just written), so it's
-- correct even with multiple concurrent assignments. Priority order:
-- READY_FOR_REVIEW > IN_PROGRESS > ASSIGNED > COMPLETED — i.e. "what
-- needs a clinician's attention" beats "what's furthest along".
-- COMPLETED only wins when every one of the patient's assignments is
-- done (or they have exactly one, and it's done).
CREATE OR REPLACE FUNCTION sync_patient_activity()
RETURNS TRIGGER AS $$
DECLARE
    v_patient_id UUID;
    v_status assignment_status;
BEGIN
    IF TG_OP = 'DELETE' THEN
        v_patient_id := OLD.patient_id;
    ELSE
        v_patient_id := NEW.patient_id;
    END IF;

    SELECT status INTO v_status
    FROM assignments
    WHERE patient_id = v_patient_id
    ORDER BY
        CASE status
            WHEN 'READY_FOR_REVIEW' THEN 1
            WHEN 'IN_PROGRESS' THEN 2
            WHEN 'ASSIGNED' THEN 3
            WHEN 'COMPLETED' THEN 4
        END
    LIMIT 1;

    IF TG_OP = 'DELETE' THEN
        -- Deleting an assignment isn't itself new patient activity, so
        -- last_activity_at is left untouched; only the now-possibly-
        -- stale computed_status needs recomputing (NULL if that was
        -- the patient's last remaining assignment).
        UPDATE patients
        SET computed_status = v_status
        WHERE id = v_patient_id;
        RETURN OLD;
    ELSE
        UPDATE patients
        SET computed_status = v_status,
            last_activity_at = NOW()
        WHERE id = v_patient_id;
        RETURN NEW;
    END IF;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_sync_patient_activity
    AFTER INSERT OR UPDATE OR DELETE ON assignments
    FOR EACH ROW
    EXECUTE FUNCTION sync_patient_activity();

-- Whenever `responses` changes, flip ASSIGNED -> IN_PROGRESS and record
-- when the patient first started answering. This is the one part of the
-- old "progress" trigger that's genuinely stateful (a one-time
-- transition) — actual progress % is cheap to compute at read time from
-- responses + test_definitions.item_count, so it's not cached here.
CREATE OR REPLACE FUNCTION mark_assignment_started_on_response()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.responses IS DISTINCT FROM OLD.responses THEN
        UPDATE assignments
        SET started_at = COALESCE(started_at, NOW()),
            status = CASE WHEN status = 'ASSIGNED' THEN 'IN_PROGRESS' ELSE status END
        WHERE id = NEW.assignment_id;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_mark_assignment_started_on_response
    AFTER UPDATE OF responses ON assignment_results
    FOR EACH ROW
    EXECUTE FUNCTION mark_assignment_started_on_response();

-- ---------------------------------------------------------------------
-- Developer-data visibility helpers + auto-stamping triggers.
-- Defined before activity_events auto-logging below, since
-- log_activity_event() relies on patients.is_developer_data /
-- assignments.is_developer_data already being correctly set on the rows
-- it looks up — which these triggers guarantee, by firing BEFORE INSERT
-- (i.e. before the row is ever visible to anything reading it back).
-- ---------------------------------------------------------------------
CREATE OR REPLACE FUNCTION is_developer_user(p_user_id UUID)
RETURNS BOOLEAN AS $$
DECLARE
    v_is_dev BOOLEAN;
BEGIN
    IF p_user_id IS NULL THEN
        RETURN FALSE;
    END IF;
    SELECT (role = 'DEVELOPER') INTO v_is_dev FROM users WHERE id = p_user_id;
    RETURN COALESCE(v_is_dev, FALSE);
END;
$$ LANGUAGE plpgsql STABLE;

CREATE OR REPLACE FUNCTION mark_patient_developer_data()
RETURNS TRIGGER AS $$
BEGIN
    NEW.is_developer_data := is_developer_user(NEW.created_by);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_mark_patient_developer_data
    BEFORE INSERT ON patients
    FOR EACH ROW
    EXECUTE FUNCTION mark_patient_developer_data();

CREATE OR REPLACE FUNCTION mark_assignment_developer_data()
RETURNS TRIGGER AS $$
BEGIN
    NEW.is_developer_data := is_developer_user(NEW.assigned_by);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_mark_assignment_developer_data
    BEFORE INSERT ON assignments
    FOR EACH ROW
    EXECUTE FUNCTION mark_assignment_developer_data();

-- ---------------------------------------------------------------------
-- activity_events auto-logging.
--
-- current_actor_id() reads a session variable your app sets per request
-- (see the write-up alongside this migration for how). If it's never
-- set — e.g. a patient completing their own remote assessment — this
-- returns NULL, which activity_events already treats as "the patient
-- did this", so no special-casing is needed at the call sites below.
-- ---------------------------------------------------------------------
CREATE OR REPLACE FUNCTION current_actor_id()
RETURNS UUID AS $$
BEGIN
    RETURN NULLIF(current_setting('app.current_user_id', true), '')::UUID;
EXCEPTION WHEN OTHERS THEN
    RETURN NULL; -- malformed/unset session variable -> "no known actor"
END;
$$ LANGUAGE plpgsql STABLE;

-- Shared insert helper so each trigger function below is a one-liner.
-- Also the single place that computes is_developer_data for an event —
-- FIX: earlier versions of this function omitted this entirely, which
-- silently stamped every single activity event as is_developer_data =
-- FALSE regardless of the source patient/assignment, leaking developer
-- test-data events into every real user's activity feed. See the
-- column comment on activity_events for why this is cached here rather
-- than derived at read time via a join.
-- Note: this is the one place in the schema where the DB generates a
-- row's id (gen_random_uuid(), built into Postgres 13+) rather than the
-- app supplying it — a deliberate, isolated exception, since a
-- trigger-driven insert has no app code path to hand it one.
CREATE OR REPLACE FUNCTION log_activity_event(
    p_event_type TEXT,
    p_patient_id UUID,
    p_assignment_id UUID,
    p_metadata JSONB DEFAULT '{}'::JSONB
) RETURNS VOID AS $$
DECLARE
    v_is_dev BOOLEAN := FALSE;
BEGIN
    IF p_patient_id IS NOT NULL THEN
        SELECT is_developer_data INTO v_is_dev FROM patients WHERE id = p_patient_id;
    END IF;
    IF NOT COALESCE(v_is_dev, FALSE) AND p_assignment_id IS NOT NULL THEN
        SELECT is_developer_data INTO v_is_dev FROM assignments WHERE id = p_assignment_id;
    END IF;

    INSERT INTO activity_events (id, event_type, patient_id, assignment_id, actor_id, is_developer_data, metadata)
    VALUES (gen_random_uuid(), p_event_type, p_patient_id, p_assignment_id, current_actor_id(), COALESCE(v_is_dev, FALSE), p_metadata);
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION log_patient_created()
RETURNS TRIGGER AS $$
BEGIN
    PERFORM log_activity_event('PATIENT_CREATED', NEW.id, NULL,
        jsonb_build_object('mrn', NEW.mrn));
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_log_patient_created
    AFTER INSERT ON patients
    FOR EACH ROW
    EXECUTE FUNCTION log_patient_created();

-- FIX: this function + trigger were missing entirely — archiving or
-- reactivating a patient logged nothing at all.
CREATE OR REPLACE FUNCTION log_patient_active_changed()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.is_active IS DISTINCT FROM OLD.is_active THEN
        PERFORM log_activity_event(
            CASE WHEN NEW.is_active THEN 'PATIENT_REACTIVATED' ELSE 'PATIENT_ARCHIVED' END,
            NEW.id, NULL, '{}'::JSONB
        );
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_log_patient_active_changed
    AFTER UPDATE OF is_active ON patients
    FOR EACH ROW
    EXECUTE FUNCTION log_patient_active_changed();

-- Fires on assignment creation, and on the transitions clinicians
-- actually care about seeing in the feed (status changes, and the
-- reviewed_at NULL -> NOT NULL transition). Deliberately does NOT fire
-- on every UPDATE — e.g. progress_pct-style churn isn't feed-worthy —
-- which is why it checks IS DISTINCT FROM rather than firing blindly.
CREATE OR REPLACE FUNCTION log_assignment_events()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        PERFORM log_activity_event('ASSIGNMENT_CREATED', NEW.patient_id, NEW.id,
            jsonb_build_object('testCode', NEW.test_code, 'mode', NEW.mode));
        RETURN NEW;
    END IF;

    IF NEW.status IS DISTINCT FROM OLD.status THEN
        PERFORM log_activity_event(
            CASE NEW.status
                WHEN 'IN_PROGRESS' THEN 'ASSIGNMENT_STARTED'
                WHEN 'READY_FOR_REVIEW' THEN 'ASSIGNMENT_READY_FOR_REVIEW'
                WHEN 'COMPLETED' THEN 'ASSIGNMENT_COMPLETED'
                ELSE 'ASSIGNMENT_STATUS_CHANGED'
            END,
            NEW.patient_id, NEW.id,
            jsonb_build_object('from', OLD.status, 'to', NEW.status)
        );
    END IF;

    IF NEW.reviewed_at IS NOT NULL AND OLD.reviewed_at IS NULL THEN
        PERFORM log_activity_event('ASSIGNMENT_REVIEWED', NEW.patient_id, NEW.id, '{}'::JSONB);
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_log_assignment_events
    AFTER INSERT OR UPDATE ON assignments
    FOR EACH ROW
    EXECUTE FUNCTION log_assignment_events();

CREATE OR REPLACE FUNCTION log_link_sent()
RETURNS TRIGGER AS $$
DECLARE
    v_patient_id UUID;
BEGIN
    SELECT patient_id INTO v_patient_id FROM assignments WHERE id = NEW.assignment_id;
    PERFORM log_activity_event('LINK_SENT', v_patient_id, NEW.assignment_id,
        jsonb_build_object('sentTo', NEW.sent_to_email));
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_log_link_sent
    AFTER INSERT ON remote_access_tokens
    FOR EACH ROW
    EXECUTE FUNCTION log_link_sent();

CREATE OR REPLACE FUNCTION log_reminder_sent()
RETURNS TRIGGER AS $$
DECLARE
    v_patient_id UUID;
BEGIN
    SELECT patient_id INTO v_patient_id FROM assignments WHERE id = NEW.assignment_id;
    PERFORM log_activity_event('REMINDER_SENT', v_patient_id, NEW.assignment_id, '{}'::JSONB);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_log_reminder_sent
    AFTER INSERT ON assignment_reminders
    FOR EACH ROW
    EXECUTE FUNCTION log_reminder_sent();
