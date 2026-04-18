CREATE TABLE IF NOT EXISTS continue_watching_entry (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES user(id) ON DELETE CASCADE,
    anime_id INTEGER NOT NULL REFERENCES anime(id) ON DELETE CASCADE,
    current_episode INTEGER,
    current_time_seconds REAL NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, anime_id)
);

CREATE INDEX IF NOT EXISTS idx_continue_watching_user_updated
ON continue_watching_entry(user_id, updated_at DESC);
