package database

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

		// Split by statement and execute one by one
		statements := strings.Split(string(migrationSQL), ";")
		for _, stmt := range statements {
			stmt = strings.TrimSpace(stmt)
			if stmt == "" {
				continue
			}

			if _, err := db.Exec(stmt); err != nil {
				errStr := err.Error()
				// Safely ignore duplicate columns/tables caused by old manual sqlite3 runs
				if strings.Contains(errStr, "duplicate column name") || strings.Contains(errStr, "already exists") {
					log.Printf("warning: ignoring expected error in %s: %v", migrationFile, err)
				} else {
					return fmt.Errorf("failed to execute statement in %s: %v\nStatement: %s", migrationFile, err, stmt)
				}
			}
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
