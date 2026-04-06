#!/bin/bash
set -e

# Initialize database if it doesn't exist
if [ ! -f "$DATABASE_FILE" ]; then
    sqlite3 "$DATABASE_FILE" < migrations/001_init.sql
    sqlite3 "$DATABASE_FILE" < migrations/002_add_anime_titles.sql
    sqlite3 "$DATABASE_FILE" < migrations/003_add_anime_airing.sql
fi

# Start the app
exec "$@"
