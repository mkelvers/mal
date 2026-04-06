#!/bin/bash
set -e

# Create migrations tracking table if it doesn't exist
sqlite3 "$DATABASE_FILE" "CREATE TABLE IF NOT EXISTS schema_migrations (
  version TEXT PRIMARY KEY,
  applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
);"

# Function to run migration
run_migration() {
  local version=$1
  local file=$2
  
  # Check if migration has been run
  if ! sqlite3 "$DATABASE_FILE" "SELECT 1 FROM schema_migrations WHERE version = '$version';" | grep -q 1; then
    echo "Running migration: $version"
    sqlite3 "$DATABASE_FILE" < "$file"
    sqlite3 "$DATABASE_FILE" "INSERT INTO schema_migrations (version) VALUES ('$version');"
  fi
}

# Run migrations
run_migration "001_init" "migrations/001_init.sql"
run_migration "002_add_anime_titles" "migrations/002_add_anime_titles.sql"
run_migration "003_add_anime_airing" "migrations/003_add_anime_airing.sql"

# Start the app
exec "$@"
