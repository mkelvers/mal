-- name: GetUser :one
SELECT * FROM user WHERE id = ? LIMIT 1;

-- name: GetUserByUsername :one
SELECT * FROM user WHERE username = ? LIMIT 1;

-- name: CreateUser :one
INSERT INTO user (id, username, password_hash)
VALUES (?, ?, ?)
RETURNING *;

-- name: CreateSession :one
INSERT INTO session (id, user_id, expires_at)
VALUES (?, ?, ?)
RETURNING *;

-- name: GetSession :one
SELECT * FROM session WHERE id = ? LIMIT 1;

-- name: DeleteSession :exec
DELETE FROM session WHERE id = ?;

-- name: DeleteUserSessions :exec
DELETE FROM session WHERE user_id = ?;

-- name: UpsertAnime :one
INSERT INTO anime (id, title_original, title_english, title_japanese, image_url, airing)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
    title_original = excluded.title_original,
    title_english = excluded.title_english,
    title_japanese = excluded.title_japanese,
    image_url = excluded.image_url,
    airing = excluded.airing
RETURNING *;

-- name: GetAnime :one
SELECT * FROM anime WHERE id = ? LIMIT 1;

-- name: UpsertWatchListEntry :one
INSERT INTO watch_list_entry (id, user_id, anime_id, status, updated_at)
VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT (user_id, anime_id) DO UPDATE SET
    status = excluded.status,
    updated_at = CURRENT_TIMESTAMP
RETURNING *;

-- name: GetWatchListEntry :one
SELECT * FROM watch_list_entry
WHERE user_id = ? AND anime_id = ? LIMIT 1;

-- name: GetUserWatchList :many
SELECT 
    e.*,
    a.title_original,
    a.title_english,
    a.title_japanese,
    a.image_url,
    a.airing
FROM watch_list_entry e
JOIN anime a ON e.anime_id = a.id
WHERE e.user_id = ?
ORDER BY e.updated_at DESC;

-- name: DeleteWatchListEntry :exec
DELETE FROM watch_list_entry
WHERE user_id = ? AND anime_id = ?;
