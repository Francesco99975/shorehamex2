-- Add new values to test_domain
ALTER TYPE test_domain ADD VALUE IF NOT EXISTS 'STRESS';
ALTER TYPE test_domain ADD VALUE IF NOT EXISTS 'TRAUMA';
ALTER TYPE test_domain ADD VALUE IF NOT EXISTS 'MULTI';

-- Add new value to scoring_method
ALTER TYPE scoring_method ADD VALUE IF NOT EXISTS 'SUBSCALE_BANDED';

-- Add notes column to patients
ALTER TABLE patients
    ADD COLUMN notes TEXT NOT NULL DEFAULT '';
