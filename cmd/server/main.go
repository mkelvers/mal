package main

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"

	"mal/api/auth"
	"mal/integrations/jikan"
	dbpkg "mal/internal/db"
	"mal/internal/server"
	"mal/internal/worker"
	"mal/pkg/middleware"
)

func main() {
	_ = godotenv.Load()

	if len(os.Args) > 1 && os.Args[1] == "create-user" {
		if len(os.Args) != 4 {
			log.Fatalf("Usage: %s create-user <username> <password>", os.Args[0])
		}

		username := os.Args[2]
		password := os.Args[3]

		db, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?_foreign_keys=on", dbFile()))
		if err != nil {
			log.Fatalf("failed to open db: %v", err)
		}
		defer db.Close()

		var existingID string
		err = db.QueryRow("SELECT id FROM user WHERE username = ?", username).Scan(&existingID)
		if err != nil && err != sql.ErrNoRows {
			log.Fatalf("database error: %v", err)
		}

		if err == nil {
			fmt.Printf("User '%s' already exists. Do you want to overwrite their password? [y/N]: ", username)
			reader := bufio.NewReader(os.Stdin)
			response, _ := reader.ReadString('\n')
			response = strings.TrimSpace(strings.ToLower(response))

			if response != "y" && response != "yes" {
				fmt.Println("Operation cancelled.")
				return
			}

			hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
			if err != nil {
				log.Fatalf("failed to hash password: %v", err)
			}

			_, err = db.Exec("UPDATE user SET password_hash = ? WHERE id = ?", string(hash), existingID)
			if err != nil {
				log.Fatalf("failed to update user: %v", err)
			}

			fmt.Printf("✅ Password for '%s' updated successfully!\n", username)
			return
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
		if err != nil {
			log.Fatalf("failed to hash password: %v", err)
		}

		id := uuid.New().String()
		_, err = db.Exec("INSERT INTO user (id, username, password_hash) VALUES (?, ?, ?)", id, username, string(hash))
		if err != nil {
			log.Fatalf("failed to create user: %v", err)
		}

		fmt.Printf("✅ Brugeren '%s' blev oprettet med succes!\n", username)
		return
	}

	db, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?_foreign_keys=on", dbFile()))
	if err != nil {
		log.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	migrationsDir := migrationsDir()
	if err := dbpkg.RunMigrations(db, migrationsDir); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}

	queries := dbpkg.New(db)
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
		WriteTimeout:      120 * time.Second,
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
	wd, err := os.Getwd()
	if err != nil {
		log.Fatalf("failed to get working directory: %v", err)
	}
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
