-- Add episode tracking to watch list entries
ALTER TABLE watch_list_entry ADD COLUMN current_episode INTEGER DEFAULT 0;
ALTER TABLE watch_list_entry ADD COLUMN last_episode_at DATETIME;

-- Add notification preferences
CREATE TABLE IF NOT EXISTS notification_preference (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES user(id) ON DELETE CASCADE,
    notify_new_episodes BOOLEAN NOT NULL DEFAULT TRUE,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id)
);
