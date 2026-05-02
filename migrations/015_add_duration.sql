-- Add duration column to anime table to store episode duration in seconds
ALTER TABLE anime ADD COLUMN duration_seconds REAL;

-- Add duration_seconds column to continue_watching_entry to track episode duration
ALTER TABLE continue_watching_entry ADD COLUMN duration_seconds REAL;