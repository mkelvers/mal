#!/bin/sh
set -e

# Clear potentially corrupted Jikan cache entries on each deploy
DB_FILE="${DATABASE_FILE:-/app/data/mal.db}"
if [ -f "$DB_FILE" ]; then
    sqlite3 "$DB_FILE" "DELETE FROM jikan_cache WHERE key LIKE 'top:%';" 2>/dev/null || true
fi

# Start the server
exec ./main_server
