# Shoreham Examination Tool

## Commission for Shoreham Chronic Pain Clinic

### Clinical tool to examine patients for psychological conditions through the following tests: Anxiety Symptoms Questionnaire, Beck Anxiety Inventory, Beck Depression Inventory, P3, and MMPI-2.

# Psychological Test Scoring Server

## Overview

This Go-based web server processes and scores psychological assessments, including the Anxiety Symptoms Questionnaire (ASQ), Beck Anxiety Inventory (BAI), Beck Depression Inventory (BDI), P3, and Minnesota Multiphasic Personality Inventory (MMPI-2). The server calculates scores based on user inputs, determines severity levels (normal, moderate, severe), and provides indications for clinical interpretation. Results are stored in a database and can be updated or retrieved as needed.

This README explains the scoring logic and calculation methods for each test as defined in the `models` package.

## Supported Tests

The server supports the following psychological tests, each with specific scoring rules and maximum scores:

- **Anxiety Symptoms Questionnaire (ASQ)**: Max score = 38
- **Beck Anxiety Inventory (BAI)**: Max score = 63
- **Beck Depression Inventory (BDI)**: Max score = 33
- **P3**: Max score = 88 (with a 44-point adjustment)
- **Minnesota Multiphasic Personality Inventory (MMPI-2)**: Max score = 567

## Test Calculation Logic

### General Scoring Approach

For ASQ, BAI, BDI, and P3, the scoring process involves:

1. **Raw Score Input**: The test score is provided as an integer input.
2. **Percentage Calculation**: The raw score is divided by the test's maximum score and multiplied by 100 to obtain a percentage.
3. **Severity Determination**: The percentage is compared against predefined thresholds to classify the result as "normal," "moderate," or "severe."
4. **Result Compilation**: A formatted string is generated, including the patient's name, percentage score, test name, duration (in minutes and seconds), and severity level.

The MMPI-2 test follows a more complex process, involving multiple scales, gender-specific scoring, and indications derived from a `scales.json` file.

### Test-Specific Calculations

#### Anxiety Symptoms Questionnaire (ASQ)

- **Max Score**: 38
- **Calculation**:
  - Compute percentage: `(score / 38) * 100`
  - Determine severity:
    - **Normal**: Percentage ≤ 30.99%
    - **Moderate**: 31% ≤ Percentage ≤ 45%
    - **Severe**: Percentage &gt; 45%
- **Output**: A string in the format: "The patient, \[patient\], scored \[percentage\]% on the Anxiety Symptom Questionnaire in \[minutes\] minutes at \[seconds\] seconds. This result is in the \[severity\] range."
- **Thresholds**:
  - Low: \~11.4 (30% of max score)
  - High: \~17.1 (45% of max score)

#### Beck Anxiety Inventory (BAI)

- **Max Score**: 63
- **Calculation**:
  - Compute percentage: `(score / 63) * 100`
  - Determine severity:
    - **Normal**: Percentage ≤ 21.99%
    - **Moderate**: 22% ≤ Percentage ≤ 35%
    - **Severe**: Percentage &gt; 35%
- **Output**: A string in the format: "The patient, \[patient\], scored \[percentage\]% on the Beck Anxiety Inventory in \[minutes\] minutes at \[seconds\] seconds. This result is in the \[severity\] range."
- **Thresholds**:
  - Low: \~13.23 (21% of max score)
  - High: \~22.05 (35% of max score)

#### Beck Depression Inventory (BDI)

- **Max Score**: 33
- **Calculation**:
  - Compute percentage: `(score / 33) * 100`
  - Determine severity:
    - **Normal**: Percentage ≤ 9.99%
    - **Moderate**: 10% ≤ Percentage ≤ 18%
    - **Severe**: Percentage &gt; 18%
- **Output**: A string in the format: "The patient, \[patient\], scored \[percentage\]% on the Beck Depression Inventory in \[minutes\] minutes at \[seconds\] seconds. This result is in the \[severity\] range."
- **Thresholds**:
  - Low: \~2.97 (9% of max score)
  - High: \~5.94 (18% of max score)

#### P3

- **Max Score**: 88 (with a 44-point adjustment subtracted from the score in some contexts, though not explicitly applied in the provided calculation function)
- **Calculation**:
  - Compute percentage: `(score / 88) * 100`
  - Determine severity:
    - **Normal**: Percentage ≤ 30.99%
    - **Moderate**: 31% ≤ Percentage ≤ 50%
    - **Severe**: Percentage &gt; 50%
- **Output**: A string in the format: "The patient, \[patient\], scored \[percentage\]% on the P3 in \[minutes\] minutes at \[seconds\] seconds. This result is in the \[severity\] range."
- **Thresholds**:
  - Low: \~26.4 (30% of max score)
  - High: \~44 (50% of max score)
- **Note**: The `P3_ADJUST` constant (44) is defined to account for a non 0 starting value.

#### Minnesota Multiphasic Personality Inventory (MMPI-2)

- **Max Score**: 567
- **Calculation**:
  - **Input**: A string of answers (e.g., "TTFTFFT...") representing true/false responses to 567 questions.
  - **Scales**: The test uses multiple scales (e.g., Hs, D, Hy, Pd, Mf, Pa, Pt, Sc, Ma, Si, VRIN, TRIN, F, Fb, Fp) defined in a `scales.json` file. Each scale has:
    - A base score.
    - Gender-specific T-scores and score offsets.
    - K-correction factors (if applicable).
    - Associated questions and expected answers.
  - **Scoring Process**:
    1. Parse answers into a boolean array (T = true, F = false).
    2. For each scale in `scales.json`:
       - Check if the scale is gender-specific and matches the patient's sex.
       - Calculate raw score by comparing patient answers to expected answers for specific questions.
       - Apply K-correction (if applicable) using the K scale score.
       - Adjust raw score with gender-specific offsets.
       - Map the adjusted score to a T-score using gender-specific T-score tables.
    3. Derive indications based on scale scores and predefined thresholds (e.g., Hs ≥ 75, Mf &lt; 45).
  - **Indications**: Specific clinical interpretations are derived for each scale based on score ranges, such as:
    - **VRIN ≥ 80**: Inconsistent responding.
    - **Hs ≥ 75**: Significant health concerns.
    - **Mf &lt; 45**: Gender role non-conformity (for males/females).
    - Many others, as defined in the `Indications` struct.
  - **Output**: A structured `MMPIResults` object containing:
    - Patient ID, name, sex, and test duration.
    - Categories with scale results (name, description, purpose, score).
    - Derived indications for each category.

## Database Integration

- **Storage**: Test results are stored in a database table `localres` with fields: `id`, `patient`, `sex`, `page`, `answers`, `duration`, and `aid`.
- **Operations**:
  - `Save()`: Inserts a new test result.
  - `Update()`: Updates an existing result with new page, answers, and duration.
  - `Load()`: Retrieves a result by ID.
  - `Calculate()`: Computes MMPI-2 results based on stored answers and `scales.json`.

##
