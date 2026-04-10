CREATE INDEX IF NOT EXISTS idx_watch_list_entry_user_status_updated_at
ON watch_list_entry(user_id, status, updated_at);

CREATE INDEX IF NOT EXISTS idx_anime_relation_anime_id_relation_type
ON anime_relation(anime_id, relation_type);

CREATE INDEX IF NOT EXISTS idx_anime_relations_synced_at_status
ON anime(relations_synced_at, status);

CREATE INDEX IF NOT EXISTS idx_jikan_cache_expires_at
ON jikan_cache(expires_at);
