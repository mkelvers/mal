package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"mal/internal/database"
	"mal/internal/features/auth"
	"mal/internal/jikan"
	"mal/internal/server"
	"mal/internal/worker"
)

func main() {
	dbFile := strings.TrimSpace(os.Getenv("DATABASE_FILE"))
	if dbFile == "" {
		dbFile = "mal.db"
	}

	dsn := fmt.Sprintf("file:%s?_foreign_keys=on", dbFile)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		log.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("failed to ping db: %v", err)
	}

	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		log.Fatalf("failed to enforce sqlite foreign keys: %v", err)
	}

	var fkState int
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&fkState); err != nil {
		log.Fatalf("failed to verify sqlite foreign keys: %v", err)
	}
	if fkState != 1 {
		log.Fatal("sqlite foreign keys are disabled")
	}

	migrationsDir, err := resolveMigrationsDir()
	if err != nil {
		log.Fatalf("failed to locate migrations directory: %v", err)
	}

	// Run migrations with tracking
	if err := database.RunMigrations(db, migrationsDir); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}

	playbackSecret := strings.TrimSpace(os.Getenv("PLAYBACK_PROXY_SECRET"))
	if len(playbackSecret) < 32 {
		log.Fatal("PLAYBACK_PROXY_SECRET must be set and at least 32 characters")
	}

	queries := database.New(db)
	authService := auth.NewService(queries)
	jikanClient := jikan.NewClient(queries)

	// Start background workers
	relationsWorker := worker.New(queries, jikanClient)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go relationsWorker.Start(ctx)

	app := server.Config{
		DB:                  queries,
		SQLDB:               db,
		JikanClient:         jikanClient,
		AuthService:         authService,
		PlaybackProxySecret: playbackSecret,
	}

	handler := server.NewRouter(app)

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	httpServer := &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("server shutdown failed: %v", err)
		}
	}()

	log.Printf("Server starting on http://localhost:%s", port)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server failed to start: %v", err)
	}
}

func resolveMigrationsDir() (string, error) {
	configured := strings.TrimSpace(os.Getenv("MIGRATIONS_DIR"))
	if configured != "" {
		hasFiles, err := directoryHasSQLFiles(configured)
		if err != nil {
			return "", err
		}

		if !hasFiles {
			return "", fmt.Errorf("MIGRATIONS_DIR has no .sql files: %s", configured)
		}

		return configured, nil
	}

	workingDir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	executablePath, err := os.Executable()
	if err != nil {
		return "", err
	}

	candidates := []string{
		filepath.Join(workingDir, "migrations"),
		filepath.Join(filepath.Dir(executablePath), "migrations"),
	}

	for _, candidate := range candidates {
		hasFiles, checkErr := directoryHasSQLFiles(candidate)
		if checkErr != nil {
			if errors.Is(checkErr, os.ErrNotExist) {
				continue
			}

			return "", checkErr
		}

		if hasFiles {
			return candidate, nil
		}
	}

	return "", errors.New("could not find migrations directory")
}

func directoryHasSQLFiles(dir string) (bool, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return false, err
	}

	if !info.IsDir() {
		return false, fmt.Errorf("not a directory: %s", dir)
	}

	files, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil {
		return false, err
	}

	return len(files) > 0, nil
}
