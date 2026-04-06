#!/bin/bash
set -e

# Run migrations
sqlite3 "$DATABASE_FILE" < migrations/001_init.sql
sqlite3 "$DATABASE_FILE" < migrations/002_add_anime_titles.sql
sqlite3 "$DATABASE_FILE" < migrations/003_add_anime_airing.sql

# Start the app
exec "$@"
