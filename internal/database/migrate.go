package database

import (
	"database/sql"
	"log"
	"os"
	"path/filepath"
	"sort"
)

func RunMigrations(db *sql.DB) error {
	// Create migration tracking table
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS migration_version (
			name TEXT PRIMARY KEY,
			applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return err
	}

	migrations, err := filepath.Glob("migrations/*.sql")
	if err != nil {
		return err
	}

	sort.Strings(migrations)

	for _, migrationFile := range migrations {
		// Check if migration already applied
		var exists int
		err := db.QueryRow("SELECT COUNT(*) FROM migration_version WHERE name = ?", migrationFile).Scan(&exists)
		if err != nil {
			return err
		}
		if exists > 0 {
			log.Printf("migration %s already applied, skipping", migrationFile)
			continue
		}

		// Read and execute migration
		migrationSQL, err := os.ReadFile(migrationFile)
		if err != nil {
			return err
		}
		if _, err := db.Exec(string(migrationSQL)); err != nil {
			return err
		}

		// Mark as applied
		_, err = db.Exec("INSERT INTO migration_version (name) VALUES (?)", migrationFile)
		if err != nil {
			return err
		}

		log.Printf("migration %s applied successfully", migrationFile)
	}

	return nil
}
