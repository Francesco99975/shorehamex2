-- =============================================================================
-- 000004_examinations.down.sql
--
-- Reverses 000004_examinations.up.sql
-- Restores the pre-examination schema (assignments own mode/due_at/assigned_by,
-- remote tokens and reminders point at assignments again).
--
-- WARNING: Destructive. Any data in examinations, and the examination_id
-- linkage on assignments / tokens / reminders / activity_events, will be lost.
-- Safe only while still in development with no production data.
-- =============================================================================

-- ---------------------------------------------------------------------------
-- 1. Drop examination-specific triggers and functions
-- ---------------------------------------------------------------------------
DROP TRIGGER IF EXISTS trigger_log_examination_created ON examinations;
DROP TRIGGER IF EXISTS trigger_sync_examination_status ON assignments;
DROP TRIGGER IF EXISTS trigger_sync_patient_activity_on_examinations ON examinations;
DROP TRIGGER IF EXISTS trigger_sync_patient_activity_on_assignments ON assignments;
DROP TRIGGER IF EXISTS trigger_mark_examination_developer_data ON examinations;

DROP FUNCTION IF EXISTS log_examination_created();
DROP FUNCTION IF EXISTS sync_examination_status();
DROP FUNCTION IF EXISTS mark_examination_developer_data();

-- ---------------------------------------------------------------------------
-- 2. Restore original sync_patient_activity (assignments-only version)
-- ---------------------------------------------------------------------------
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
        SET computed_status  = v_status,
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

-- ---------------------------------------------------------------------------
-- 3. Restore original mark_assignment_developer_data
--    (uses assigned_by, which we are about to put back)
-- ---------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION mark_assignment_developer_data()
RETURNS TRIGGER AS $$
BEGIN
    NEW.is_developer_data := is_developer_user(NEW.assigned_by);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- ---------------------------------------------------------------------------
-- 4. Restore original log_activity_event (no examination_id)
-- ---------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION log_activity_event(
    p_event_type    TEXT,
    p_patient_id    UUID,
    p_assignment_id UUID,
    p_metadata      JSONB DEFAULT '{}'::JSONB
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

-- ---------------------------------------------------------------------------
-- 5. Restore original log_assignment_events / log_link_sent / log_reminder_sent
-- ---------------------------------------------------------------------------
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
                WHEN 'IN_PROGRESS'      THEN 'ASSIGNMENT_STARTED'
                WHEN 'READY_FOR_REVIEW' THEN 'ASSIGNMENT_READY_FOR_REVIEW'
                WHEN 'COMPLETED'        THEN 'ASSIGNMENT_COMPLETED'
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

-- ---------------------------------------------------------------------------
-- 6. Move remote_access_tokens back to assignment_id
-- ---------------------------------------------------------------------------
ALTER TABLE remote_access_tokens
    DROP COLUMN IF EXISTS examination_id,
    ADD COLUMN assignment_id UUID REFERENCES assignments(id) ON DELETE CASCADE;

-- Make it NOT NULL only if you are sure the table is empty; otherwise leave nullable
-- and clean up manually. With no production data we can enforce it:
ALTER TABLE remote_access_tokens
    ALTER COLUMN assignment_id SET NOT NULL;

DROP INDEX IF EXISTS idx_remote_access_tokens_examination_id;
DROP INDEX IF EXISTS idx_remote_access_tokens_active;

CREATE INDEX idx_remote_access_tokens_assignment_id
    ON remote_access_tokens(assignment_id);

CREATE UNIQUE INDEX idx_remote_access_tokens_active
    ON remote_access_tokens(assignment_id)
    WHERE used_at IS NULL AND NOT revoked;

-- ---------------------------------------------------------------------------
-- 7. Move assignment_reminders back to assignment_id
-- ---------------------------------------------------------------------------
ALTER TABLE assignment_reminders
    DROP COLUMN IF EXISTS examination_id,
    ADD COLUMN assignment_id UUID REFERENCES assignments(id) ON DELETE CASCADE;

ALTER TABLE assignment_reminders
    ALTER COLUMN assignment_id SET NOT NULL;

DROP INDEX IF EXISTS idx_assignment_reminders_examination_id;

CREATE INDEX idx_assignment_reminders_assignment_id
    ON assignment_reminders(assignment_id);

-- ---------------------------------------------------------------------------
-- 8. Remove examination_id from activity_events
-- ---------------------------------------------------------------------------
DROP INDEX IF EXISTS idx_activity_events_examination_id;
ALTER TABLE activity_events
    DROP COLUMN IF EXISTS examination_id;

-- ---------------------------------------------------------------------------
-- 9. Restore columns on assignments, drop examination linkage
-- ---------------------------------------------------------------------------
ALTER TABLE assignments
    ADD COLUMN mode        assignment_mode,
    ADD COLUMN assigned_by UUID REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN due_at      TIMESTAMPTZ;

-- With no data we can set a temporary default then drop it, or leave nullable.
-- Prefer leaving nullable so the down migration does not invent values.
-- Application must not rely on these columns until data is re-created.

DROP INDEX IF EXISTS idx_assignments_examination_sequence;
DROP INDEX IF EXISTS idx_assignments_examination_id;

ALTER TABLE assignments
    DROP COLUMN IF EXISTS examination_id,
    DROP COLUMN IF EXISTS sequence;

-- ---------------------------------------------------------------------------
-- 10. Drop examinations table and its enum
-- ---------------------------------------------------------------------------
DROP TABLE IF EXISTS examinations CASCADE;

DROP TYPE IF EXISTS examination_status;

-- ---------------------------------------------------------------------------
-- Done. Schema is back to the pre-examination shape.
-- ---------------------------------------------------------------------------
