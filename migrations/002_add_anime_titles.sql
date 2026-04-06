-- Add English and Japanese title columns to anime table
ALTER TABLE anime ADD COLUMN title_english TEXT;
ALTER TABLE anime ADD COLUMN title_japanese TEXT;

-- Rename existing title to title_original for clarity
ALTER TABLE anime RENAME COLUMN title TO title_original;
