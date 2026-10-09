ALTER TABLE assignment_sum_band_results
    DROP CONSTRAINT assignment_sum_band_results_pkey;

ALTER TABLE assignment_sum_band_results
    ADD CONSTRAINT assignment_sum_band_results_pkey
    PRIMARY KEY (assignment_result_id);

ALTER TABLE assignment_sum_band_results
    DROP COLUMN subscale_code;
