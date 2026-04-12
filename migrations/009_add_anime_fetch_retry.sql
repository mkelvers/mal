CREATE TABLE IF NOT EXISTS anime_fetch_retry (
    anime_id INTEGER PRIMARY KEY,
    attempts INTEGER NOT NULL DEFAULT 0,
    next_retry_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_error TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_anime_fetch_retry_next_retry_at
ON anime_fetch_retry(next_retry_at);
