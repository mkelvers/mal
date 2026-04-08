ALTER TABLE anime ADD COLUMN status TEXT DEFAULT '';
ALTER TABLE anime ADD COLUMN relations_synced_at DATETIME;

CREATE TABLE IF NOT EXISTS anime_relation (
    anime_id INTEGER NOT NULL REFERENCES anime(id) ON DELETE CASCADE,
    related_anime_id INTEGER NOT NULL,
    relation_type TEXT NOT NULL,
    PRIMARY KEY (anime_id, related_anime_id)
);
