package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"os/signal"
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

	dbFile := os.Getenv("DATABASE_FILE")
	if dbFile == "" {
		dbFile = "mal.db"
	}

	db, err := sql.Open("sqlite3", dbFile)
	if err != nil {
		log.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	// Run migrations with tracking
	if err := database.RunMigrations(db); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
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
		DB:          queries,
		JikanClient: jikanClient,
		AuthService: authService,
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
