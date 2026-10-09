-- =============================================================================
-- 000004_examinations.up.sql
--
-- Introduce Examinations as the delivery unit (one remote link / one in-clinic
-- session that can contain 1..N ordered Assignments).
--
-- Key decisions:
--   • New table `examinations` owns mode, due_at, assigned_by, the remote token,
--     and the overall session status.
--   • Assignments become children of an examination (examination_id + sequence).
--   • patient_id and is_developer_data are kept on BOTH tables.
--   • Examination status is a new, simpler 3-value enum.
--   • Assignment status keeps the original 4-value enum (per-instrument).
--   • remote_access_tokens and assignment_reminders now point at examinations.
--   • All related triggers, indexes and activity logging are updated.
--
-- Safe to run: no production data exists yet.
-- =============================================================================

-- ---------------------------------------------------------------------------
-- 1. New enum for examination-level status
-- ---------------------------------------------------------------------------
-- ASSIGNED     = examination created, patient has not touched it
-- IN_PROGRESS  = patient clicked the remote link or started in-clinic
-- COMPLETED    = every assignment inside the examination is COMPLETED
CREATE TYPE examination_status AS ENUM (
    'ASSIGNED',
    'IN_PROGRESS',
    'COMPLETED'
);

-- ---------------------------------------------------------------------------
-- 2. Examinations table
-- ---------------------------------------------------------------------------
CREATE TABLE examinations (
    id                UUID PRIMARY KEY,
    patient_id        UUID NOT NULL REFERENCES patients(id) ON DELETE CASCADE,
    mode              assignment_mode NOT NULL,
    status            examination_status NOT NULL DEFAULT 'ASSIGNED',
    assigned_by       UUID REFERENCES users(id) ON DELETE SET NULL,
    due_at            TIMESTAMPTZ,
    started_at        TIMESTAMPTZ,          -- first time any assignment was started
    completed_at      TIMESTAMPTZ,          -- when the last assignment finished
    reviewed_at       TIMESTAMPTZ,          -- optional roll-up when clinician finishes reviewing the whole battery
    reviewed_by       UUID REFERENCES users(id) ON DELETE SET NULL,
    is_developer_data BOOLEAN NOT NULL DEFAULT FALSE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_examinations_patient_id   ON examinations(patient_id);
CREATE INDEX idx_examinations_status       ON examinations(status);
CREATE INDEX idx_examinations_due_at       ON examinations(due_at);
CREATE INDEX idx_examinations_assigned_by  ON examinations(assigned_by);
CREATE INDEX idx_examinations_overdue
    ON examinations(due_at)
    WHERE status <> 'COMPLETED';

SELECT apply_update_trigger('examinations');

-- ---------------------------------------------------------------------------
-- 3. Alter assignments → become children of an examination
-- ---------------------------------------------------------------------------
ALTER TABLE assignments
    ADD COLUMN examination_id UUID REFERENCES examinations(id) ON DELETE CASCADE,
    ADD COLUMN sequence       SMALLINT NOT NULL DEFAULT 1;

-- Unique order inside one examination
CREATE UNIQUE INDEX idx_assignments_examination_sequence
    ON assignments (examination_id, sequence);

CREATE INDEX idx_assignments_examination_id ON assignments(examination_id);

-- Drop columns that now live only on the examination
ALTER TABLE assignments
    DROP COLUMN mode,
    DROP COLUMN assigned_by,
    DROP COLUMN due_at;

-- patient_id and is_developer_data stay on both tables (as requested)
-- status, started_at, completed_at, reviewed_at, reviewed_by stay on assignments
-- (per-instrument semantics)

-- ---------------------------------------------------------------------------
-- 4. Move remote_access_tokens from assignment → examination
-- ---------------------------------------------------------------------------
ALTER TABLE remote_access_tokens
    DROP COLUMN assignment_id,
    ADD COLUMN examination_id UUID NOT NULL REFERENCES examinations(id) ON DELETE CASCADE;

DROP INDEX IF EXISTS idx_remote_access_tokens_assignment_id;
DROP INDEX IF EXISTS idx_remote_access_tokens_active;

CREATE INDEX idx_remote_access_tokens_examination_id
    ON remote_access_tokens(examination_id);

-- Only one active (usable) token per examination
CREATE UNIQUE INDEX idx_remote_access_tokens_active
    ON remote_access_tokens(examination_id)
    WHERE used_at IS NULL AND NOT revoked;

-- ---------------------------------------------------------------------------
-- 5. Move assignment_reminders from assignment → examination
-- ---------------------------------------------------------------------------
ALTER TABLE assignment_reminders
    DROP COLUMN assignment_id,
    ADD COLUMN examination_id UUID NOT NULL REFERENCES examinations(id) ON DELETE CASCADE;

DROP INDEX IF EXISTS idx_assignment_reminders_assignment_id;

CREATE INDEX idx_assignment_reminders_examination_id
    ON assignment_reminders(examination_id);

-- ---------------------------------------------------------------------------
-- 6. Activity events – add examination_id, keep assignment_id for granularity
-- ---------------------------------------------------------------------------
ALTER TABLE activity_events
    ADD COLUMN examination_id UUID REFERENCES examinations(id) ON DELETE SET NULL;

CREATE INDEX idx_activity_events_examination_id ON activity_events(examination_id);

-- ---------------------------------------------------------------------------
-- 7. Developer-data stamping for examinations
-- ---------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION mark_examination_developer_data()
RETURNS TRIGGER AS $$
BEGIN
    NEW.is_developer_data := is_developer_user(NEW.assigned_by);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_mark_examination_developer_data
    BEFORE INSERT ON examinations
    FOR EACH ROW
    EXECUTE FUNCTION mark_examination_developer_data();

-- Keep the existing assignment-level trigger, but it now reads assigned_by
-- from the parent examination when needed. For simplicity we still stamp
-- from the assignment’s own created_by path if present; otherwise the
-- application should set is_developer_data explicitly on insert.
-- (The original mark_assignment_developer_data used assigned_by which we
-- just dropped, so we rewrite it.)

CREATE OR REPLACE FUNCTION mark_assignment_developer_data()
RETURNS TRIGGER AS $$
DECLARE
    v_is_dev BOOLEAN;
BEGIN
    -- Prefer the examination’s flag when available
    IF NEW.examination_id IS NOT NULL THEN
        SELECT is_developer_data INTO v_is_dev
        FROM examinations
        WHERE id = NEW.examination_id;
        NEW.is_developer_data := COALESCE(v_is_dev, FALSE);
    ELSE
        NEW.is_developer_data := FALSE;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- ---------------------------------------------------------------------------
-- 8. Re-work patient activity sync
--    Now driven by examinations (and still by their child assignments).
-- ---------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION sync_patient_activity()
RETURNS TRIGGER AS $$
DECLARE
    v_patient_id UUID;
    v_status     assignment_status;
BEGIN
    -- Determine patient_id from the row that changed
    IF TG_TABLE_NAME = 'examinations' THEN
        IF TG_OP = 'DELETE' THEN
            v_patient_id := OLD.patient_id;
        ELSE
            v_patient_id := NEW.patient_id;
        END IF;
    ELSE  -- assignments
        IF TG_OP = 'DELETE' THEN
            SELECT patient_id INTO v_patient_id
            FROM examinations WHERE id = OLD.examination_id;
        ELSE
            SELECT patient_id INTO v_patient_id
            FROM examinations WHERE id = NEW.examination_id;
        END IF;
    END IF;

    -- Overall patient status = highest-priority assignment status across
    -- all of the patient’s examinations.
    SELECT a.status INTO v_status
    FROM assignments a
    JOIN examinations e ON e.id = a.examination_id
    WHERE e.patient_id = v_patient_id
    ORDER BY
        CASE a.status
            WHEN 'READY_FOR_REVIEW' THEN 1
            WHEN 'IN_PROGRESS'      THEN 2
            WHEN 'ASSIGNED'         THEN 3
            WHEN 'COMPLETED'        THEN 4
        END
    LIMIT 1;

    IF TG_OP = 'DELETE' THEN
        UPDATE patients
        SET computed_status = v_status
        WHERE id = v_patient_id;
        RETURN OLD;
    ELSE
        UPDATE patients
        SET computed_status   = v_status,
            last_activity_at  = NOW()
        WHERE id = v_patient_id;
        RETURN NEW;
    END IF;
END;
$$ LANGUAGE plpgsql;

-- Drop old trigger, create new ones on both tables
DROP TRIGGER IF EXISTS trigger_sync_patient_activity ON assignments;

CREATE TRIGGER trigger_sync_patient_activity_on_assignments
    AFTER INSERT OR UPDATE OR DELETE ON assignments
    FOR EACH ROW
    EXECUTE FUNCTION sync_patient_activity();

CREATE TRIGGER trigger_sync_patient_activity_on_examinations
    AFTER INSERT OR UPDATE OR DELETE ON examinations
    FOR EACH ROW
    EXECUTE FUNCTION sync_patient_activity();

-- ---------------------------------------------------------------------------
-- 9. Keep the “first response starts the assignment” trigger
--    (still valid – it only touches assignment.status / started_at)
-- ---------------------------------------------------------------------------
-- (trigger_mark_assignment_started_on_response stays unchanged)

-- ---------------------------------------------------------------------------
-- 10. Examination-level status maintenance
--     When any assignment changes status we recompute the parent examination.
-- ---------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION sync_examination_status()
RETURNS TRIGGER AS $$
DECLARE
    v_exam_id   UUID;
    v_all_done  BOOLEAN;
    v_any_started BOOLEAN;
BEGIN
    IF TG_OP = 'DELETE' THEN
        v_exam_id := OLD.examination_id;
    ELSE
        v_exam_id := NEW.examination_id;
    END IF;

    -- Any assignment past ASSIGNED?
    SELECT EXISTS (
        SELECT 1 FROM assignments
        WHERE examination_id = v_exam_id
          AND status <> 'ASSIGNED'
    ) INTO v_any_started;

    -- Every assignment COMPLETED?
    SELECT NOT EXISTS (
        SELECT 1 FROM assignments
        WHERE examination_id = v_exam_id
          AND status <> 'COMPLETED'
    ) INTO v_all_done;

    IF v_all_done THEN
        UPDATE examinations
        SET status       = 'COMPLETED',
            completed_at = COALESCE(completed_at, NOW())
        WHERE id = v_exam_id
          AND status <> 'COMPLETED';
    ELSIF v_any_started THEN
        UPDATE examinations
        SET status     = 'IN_PROGRESS',
            started_at = COALESCE(started_at, NOW())
        WHERE id = v_exam_id
          AND status = 'ASSIGNED';
    END IF;

    RETURN COALESCE(NEW, OLD);
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_sync_examination_status
    AFTER INSERT OR UPDATE OF status OR DELETE ON assignments
    FOR EACH ROW
    EXECUTE FUNCTION sync_examination_status();

-- ---------------------------------------------------------------------------
-- 11. Activity logging updates
-- ---------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION log_activity_event(
    p_event_type     TEXT,
    p_patient_id     UUID,
    p_assignment_id  UUID,
    p_examination_id UUID DEFAULT NULL,
    p_metadata       JSONB DEFAULT '{}'::JSONB
) RETURNS VOID AS $$
DECLARE
    v_is_dev BOOLEAN := FALSE;
BEGIN
    IF p_patient_id IS NOT NULL THEN
        SELECT is_developer_data INTO v_is_dev FROM patients WHERE id = p_patient_id;
    END IF;
    IF NOT COALESCE(v_is_dev, FALSE) AND p_examination_id IS NOT NULL THEN
        SELECT is_developer_data INTO v_is_dev FROM examinations WHERE id = p_examination_id;
    END IF;
    IF NOT COALESCE(v_is_dev, FALSE) AND p_assignment_id IS NOT NULL THEN
        SELECT is_developer_data INTO v_is_dev FROM assignments WHERE id = p_assignment_id;
    END IF;

    INSERT INTO activity_events (
        id, event_type, patient_id, assignment_id, examination_id,
        actor_id, is_developer_data, metadata
    ) VALUES (
        gen_random_uuid(), p_event_type, p_patient_id, p_assignment_id,
        p_examination_id, current_actor_id(), COALESCE(v_is_dev, FALSE), p_metadata
    );
END;
$$ LANGUAGE plpgsql;

-- Examination created
CREATE OR REPLACE FUNCTION log_examination_created()
RETURNS TRIGGER AS $$
BEGIN
    PERFORM log_activity_event(
        'EXAMINATION_CREATED',
        NEW.patient_id,
        NULL,
        NEW.id,
        jsonb_build_object('mode', NEW.mode)
    );
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_log_examination_created
    AFTER INSERT ON examinations
    FOR EACH ROW
    EXECUTE FUNCTION log_examination_created();

-- Assignment events (now include examination_id)
CREATE OR REPLACE FUNCTION log_assignment_events()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        PERFORM log_activity_event(
            'ASSIGNMENT_CREATED',
            (SELECT patient_id FROM examinations WHERE id = NEW.examination_id),
            NEW.id,
            NEW.examination_id,
            jsonb_build_object('testCode', NEW.test_code, 'sequence', NEW.sequence)
        );
        RETURN NEW;
    END IF;

    IF NEW.status IS DISTINCT FROM OLD.status THEN
        PERFORM log_activity_event(
            CASE NEW.status
                WHEN 'IN_PROGRESS'      THEN 'ASSIGNMENT_STARTED'
                WHEN 'READY_FOR_REVIEW' THEN 'ASSIGNMENT_READY_FOR_REVIEW'
                WHEN 'COMPLETED'        THEN 'ASSIGNMENT_COMPLETED'
                ELSE 'ASSIGNMENT_STATUS_CHANGED'
            END,
            (SELECT patient_id FROM examinations WHERE id = NEW.examination_id),
            NEW.id,
            NEW.examination_id,
            jsonb_build_object('from', OLD.status, 'to', NEW.status)
        );
    END IF;

    IF NEW.reviewed_at IS NOT NULL AND OLD.reviewed_at IS NULL THEN
        PERFORM log_activity_event(
            'ASSIGNMENT_REVIEWED',
            (SELECT patient_id FROM examinations WHERE id = NEW.examination_id),
            NEW.id,
            NEW.examination_id,
            '{}'::JSONB
        );
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Link sent / reminder sent now reference examination
CREATE OR REPLACE FUNCTION log_link_sent()
RETURNS TRIGGER AS $$
DECLARE
    v_patient_id UUID;
BEGIN
    SELECT patient_id INTO v_patient_id
    FROM examinations WHERE id = NEW.examination_id;

    PERFORM log_activity_event(
        'LINK_SENT',
        v_patient_id,
        NULL,
        NEW.examination_id,
        jsonb_build_object('sentTo', NEW.sent_to_email)
    );
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION log_reminder_sent()
RETURNS TRIGGER AS $$
DECLARE
    v_patient_id UUID;
BEGIN
    SELECT patient_id INTO v_patient_id
    FROM examinations WHERE id = NEW.examination_id;

    PERFORM log_activity_event(
        'REMINDER_SENT',
        v_patient_id,
        NULL,
        NEW.examination_id,
        '{}'::JSONB
    );
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- ---------------------------------------------------------------------------
-- 12. Comments for future readers
-- ---------------------------------------------------------------------------
COMMENT ON TABLE examinations IS
    'Delivery unit: one remote link or one in-clinic session that contains '
    'an ordered list of assignments (instruments).';

COMMENT ON COLUMN examinations.status IS
    'ASSIGNED = untouched; IN_PROGRESS = patient has started; '
    'COMPLETED = every child assignment is COMPLETED.';

COMMENT ON COLUMN assignments.sequence IS
    '1-based order of this instrument inside its examination.';

COMMENT ON COLUMN assignments.status IS
    'Per-instrument status (ASSIGNED / IN_PROGRESS / READY_FOR_REVIEW / COMPLETED). '
    'Independent of the parent examination status.';
