package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"

	_ "github.com/mattn/go-sqlite3"

	"mal/internal/database"
	"mal/internal/features/auth"
)

func main() {
	email := flag.String("email", "", "Email/Username for the new user")
	password := flag.String("password", "", "Password for the new user")
	flag.Parse()

	if *email == "" || *password == "" {
		fmt.Println("Usage: make create-user EMAIL=user@example.com PASSWORD=secret")
		fmt.Println("Or   : go run ./cmd/create-user -email=user@example.com -password=secret")
		os.Exit(1)
	}

	dbFile := os.Getenv("DATABASE_FILE")
	if dbFile == "" {
		dbFile = "mal.db"
	}

	db, err := sql.Open("sqlite3", dbFile)
	if err != nil {
		log.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	queries := database.New(db)
	authService := auth.NewService(queries)

	ctx := context.Background()

	// Try to create the user
	user, err := authService.RegisterUser(ctx, *email, *password)
	if err != nil {
		log.Fatalf("Failed to create user: %v", err)
	}

	fmt.Printf("Successfully created user: %s (ID: %s)\n", user.Username, user.ID)
}
