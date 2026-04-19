package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"mal/internal/database"
	"mal/internal/features/auth"
	"mal/internal/jikan"
	"mal/internal/server"
	"mal/internal/shared/middleware"
	"mal/internal/worker"
)

func main() {
	db, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?_foreign_keys=on", dbFile()))
	if err != nil {
		log.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	migrationsDir := migrationsDir()
	if err := database.RunMigrations(db, migrationsDir); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}

	queries := database.New(db)
	jikanClient := jikan.NewClient(queries)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go worker.New(queries, jikanClient).Start(ctx)

	app := server.Config{
		DB:                  queries,
		SQLDB:               db,
		JikanClient:         jikanClient,
		AuthService:         auth.NewService(queries),
		PlaybackProxySecret: playbackSecret(),
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	httpServer := &http.Server{
		Addr:              ":" + port,
		Handler:           server.NewRouter(app),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go gracefulShutdown(httpServer, ctx)

	log.Printf("Server starting on http://localhost:%s", port)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server failed to start: %v", err)
	}
}

func dbFile() string {
	if f := os.Getenv("DATABASE_FILE"); f != "" {
		return f
	}
	return "mal.db"
}

func migrationsDir() string {
	if dir := os.Getenv("MIGRATIONS_DIR"); dir != "" {
		return dir
	}
	wd, _ := os.Getwd()
	return filepath.Join(wd, "migrations")
}

func playbackSecret() string {
	secret := os.Getenv("PLAYBACK_PROXY_SECRET")
	if len(secret) < 32 {
		log.Fatal("PLAYBACK_PROXY_SECRET must be set and at least 32 characters")
	}
	return secret
}

func gracefulShutdown(srv *http.Server, ctx context.Context) {
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("server shutdown failed: %v", err)
	}
	middleware.StopCleanup()
}
