package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"

	_ "github.com/mattn/go-sqlite3"

	"context"
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
	jikanClient := jikan.NewClient()

	// Start background workers
	relationsWorker := worker.New(queries, jikanClient)
	go relationsWorker.Start(context.Background())

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

	log.Printf("Server starting on http://localhost:%s", port)
	if err := http.ListenAndServe(":"+port, handler); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
