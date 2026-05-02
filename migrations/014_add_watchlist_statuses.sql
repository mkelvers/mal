-- Add "watching" and "on_hold" to the valid statuses for watch_list_entry

PRAGMA foreign_keys=OFF;

ALTER TABLE watch_list_entry RENAME TO watch_list_entry_old;

CREATE TABLE IF NOT EXISTS watch_list_entry (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES user(id) ON DELETE CASCADE,
    anime_id INTEGER NOT NULL REFERENCES anime(id) ON DELETE CASCADE,
    status TEXT NOT NULL CHECK(status IN ('watching', 'completed', 'dropped', 'plan_to_watch', 'on_hold')),
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    current_episode INTEGER DEFAULT 0,
    last_episode_at DATETIME,
    current_time_seconds REAL NOT NULL DEFAULT 0,
    UNIQUE(user_id, anime_id)
);

INSERT OR IGNORE INTO watch_list_entry (id, user_id, anime_id, status, created_at, updated_at, current_episode, last_episode_at, current_time_seconds)
SELECT id, user_id, anime_id, status, created_at, updated_at, current_episode, last_episode_at, current_time_seconds
FROM watch_list_entry_old;

DROP TABLE watch_list_entry_old;

PRAGMA foreign_keys=ON;
