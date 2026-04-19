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

func RunMigrations(db *sql.DB, migrationsDir string) error {
	if migrationsDir == "" {
		return fmt.Errorf("migrations directory is required")
	}

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

	migrations, err := filepath.Glob(filepath.Join(migrationsDir, "*.sql"))
	if err != nil {
		return err
	}
	if len(migrations) == 0 {
		return fmt.Errorf("no migration files found in %s", migrationsDir)
	}

	sort.Strings(migrations)

	appliedNames, err := loadAppliedMigrationNames(db)
	if err != nil {
		return err
	}

	for _, migrationFile := range migrations {
		migrationName := filepath.Base(migrationFile)
		if migrationApplied(appliedNames, migrationName) {
			// already applied, skipping silently
			continue
		}

		// Read and execute migration
		migrationSQL, err := os.ReadFile(migrationFile)
		if err != nil {
			return err
		}

		// Strict execution: if it fails, it halts.
		if _, err := db.Exec(string(migrationSQL)); err != nil {
			return err
		}

		// Mark as applied
		_, err = db.Exec("INSERT INTO migration_version (name) VALUES (?)", migrationName)
		if err != nil {
			return err
		}

		appliedNames[migrationName] = struct{}{}

		log.Printf("migration %s applied successfully", migrationName)
	}

	return nil
}

func loadAppliedMigrationNames(db *sql.DB) (map[string]struct{}, error) {
	rows, err := db.Query("SELECT name FROM migration_version")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[string]struct{})
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}

		applied[name] = struct{}{}
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return applied, nil
}

func migrationApplied(appliedNames map[string]struct{}, migrationName string) bool {
	if _, exists := appliedNames[migrationName]; exists {
		return true
	}

	legacyName := filepath.ToSlash(filepath.Join("migrations", migrationName))
	if _, exists := appliedNames[legacyName]; exists {
		return true
	}

	for appliedName := range appliedNames {
		if strings.EqualFold(filepath.Base(appliedName), migrationName) {
			return true
		}
	}

	return false
}
