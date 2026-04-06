package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"

	_ "github.com/mattn/go-sqlite3"

	"malago/internal/database"
	"malago/internal/features/auth"
	"malago/internal/jikan"
	"malago/internal/server"
)

func main() {

	dbFile := os.Getenv("DATABASE_FILE")
	if dbFile == "" {
		dbFile = "malago.db"
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
