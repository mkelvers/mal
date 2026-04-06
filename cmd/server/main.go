package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"
	"strconv"

	_ "github.com/mattn/go-sqlite3"

	"malago/internal/auth"
	"malago/internal/database"
	"malago/internal/handlers"
	"malago/internal/jikan"
	"malago/internal/middleware"
	"malago/internal/templates"
)

func runMigrations(db *sql.DB) error {
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

	migrations := []string{
		"migrations/001_init.sql",
		"migrations/002_add_anime_titles.sql",
	}

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
	if err := runMigrations(db); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}

	queries := database.New(db)
	authService := auth.NewService(queries)
	authHandler := handlers.NewAuthHandler(authService)
	watchlistHandler := handlers.NewWatchlistHandler(queries)

	jikanClient := jikan.NewClient()

	mux := http.NewServeMux()

	// Serve static files
	fs := http.FileServer(http.Dir("./static"))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))

	// Homepage (Catalog)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		templates.Catalog().Render(r.Context(), w)
	})

	// Search page
	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("q")
		if query == "" {
			templates.Search("").Render(r.Context(), w)
			return
		}

		// Check if HTMX request for results only
		if r.Header.Get("HX-Request") == "true" {
			res, err := jikanClient.Search(query, 1)
			if err != nil {
				log.Printf("search error: %v", err)
				http.Error(w, "Failed to search anime", http.StatusInternalServerError)
				return
			}
			templates.SearchResultsWrapper(query, res.Animes, 2, res.HasNextPage).Render(r.Context(), w)
			return
		}

		// Full page with query
		templates.Search(query).Render(r.Context(), w)
	})

	// Search endpoint (HTMX Infinite Scroll)
	mux.HandleFunc("/api/search", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("q")
		pageStr := r.URL.Query().Get("page")
		page, _ := strconv.Atoi(pageStr)
		if page < 1 {
			page = 1
		}

		res, err := jikanClient.Search(query, page)
		if err != nil {
			log.Printf("search pagination error: %v", err)
			http.Error(w, "Failed to fetch search page", http.StatusInternalServerError)
			return
		}

		templates.SearchItems(query, res.Animes, page+1, res.HasNextPage).Render(r.Context(), w)
	})

	// Catalog endpoint (HTMX Infinite Scroll)
	mux.HandleFunc("/api/catalog", func(w http.ResponseWriter, r *http.Request) {
		pageStr := r.URL.Query().Get("page")
		page, _ := strconv.Atoi(pageStr)
		if page < 1 {
			page = 1
		}

		res, err := jikanClient.GetTopAnime(page)
		if err != nil {
			log.Printf("top anime error: %v", err)
			http.Error(w, "Failed to fetch top anime", http.StatusInternalServerError)
			return
		}

		templates.CatalogItems(res.Animes, page+1, res.HasNextPage).Render(r.Context(), w)
	})

	// Anime Details page
	mux.HandleFunc("/anime/", func(w http.ResponseWriter, r *http.Request) {
		idStr := r.URL.Path[len("/anime/"):]
		id, err := strconv.Atoi(idStr)
		if err != nil || id <= 0 {
			http.NotFound(w, r)
			return
		}

		anime, err := jikanClient.GetAnimeByID(id)
		if err != nil {
			log.Printf("anime fetch error for %d: %v", id, err)
			http.Error(w, "Failed to fetch anime details", http.StatusInternalServerError)
			return
		}

		// Get current watchlist status if user is logged in
		currentStatus := ""
		if user := middleware.GetUser(r.Context()); user != nil {
			entry, err := queries.GetWatchListEntry(r.Context(), database.GetWatchListEntryParams{
				UserID:  user.ID,
				AnimeID: int64(id),
			})
			if err == nil {
				currentStatus = entry.Status
			}
		}

		templates.AnimeDetails(anime, currentStatus).Render(r.Context(), w)
	})

	// Anime Relations API endpoint (HTMX "Suspense")
	mux.HandleFunc("/api/anime/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path[len("/api/anime/"):]
		idStr := ""
		for i, c := range path {
			if c == '/' {
				idStr = path[:i]
				break
			}
		}

		id, _ := strconv.Atoi(idStr)
		if id <= 0 {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}

		relations := jikanClient.GetFullRelations(id)
		templates.AnimeRelationsList(relations).Render(r.Context(), w)
	})

	// Auth Endpoints
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			authHandler.HandleLoginPage(w, r)
		} else {
			authHandler.HandleLogin(w, r)
		}
	})
	mux.HandleFunc("/logout", authHandler.HandleLogout)

	// Watchlist POST endpoint (Protected)
	mux.Handle("/api/watchlist/export", middleware.RequireAuth(http.HandlerFunc(watchlistHandler.HandleExportWatchlist)))
	mux.Handle("/api/watchlist/import", middleware.RequireAuth(http.HandlerFunc(watchlistHandler.HandleImportWatchlist)))
	mux.Handle("/api/watchlist", middleware.RequireAuth(http.HandlerFunc(watchlistHandler.HandleUpdateWatchlist)))
	mux.Handle("/api/watchlist/", middleware.RequireAuth(http.HandlerFunc(watchlistHandler.HandleDeleteWatchlist)))
	mux.Handle("/watchlist", middleware.RequireAuth(http.HandlerFunc(watchlistHandler.HandleGetWatchlist)))

	// Wrap mux with global auth checking, THEN auth context parsing
	protectedHandler := middleware.RequireGlobalAuth(mux)
	handler := middleware.Auth(authService)(protectedHandler)

	log.Println("Server starting on http://localhost:3000")
	if err := http.ListenAndServe(":3000", handler); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
