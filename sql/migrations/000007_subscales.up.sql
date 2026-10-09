ALTER TABLE assignment_sum_band_results
    ADD COLUMN subscale_code text NOT NULL DEFAULT 'TOTAL';

-- Existing records represent the single/overall result.
-- Give them a deterministic code so the migration is backwards compatible.
UPDATE assignment_sum_band_results
SET subscale_code = 'TOTAL'
WHERE subscale_code IS NULL;

ALTER TABLE assignment_sum_band_results
    ALTER COLUMN subscale_code SET NOT NULL;

ALTER TABLE assignment_sum_band_results
    DROP CONSTRAINT assignment_sum_band_results_pkey;

ALTER TABLE assignment_sum_band_results
    ADD CONSTRAINT assignment_sum_band_results_pkey
    PRIMARY KEY (assignment_result_id, subscale_code);
