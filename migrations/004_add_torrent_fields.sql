-- name: AddAnimeTorrentFields :exec
ALTER TABLE anime ADD COLUMN magnet_link TEXT;
ALTER TABLE anime ADD COLUMN torrent_hash TEXT;