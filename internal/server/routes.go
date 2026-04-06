package server

import (
	"net/http"

	"malago/internal/database"
	"malago/internal/features/auth"
	"malago/internal/features/watchlist"
	"malago/internal/handlers"
	"malago/internal/jikan"
	"malago/internal/middleware"
)

type Config struct {
	DB          *database.Queries
	JikanClient *jikan.Client
	AuthService *auth.Service
}

func NewRouter(cfg Config) http.Handler {
	mux := http.NewServeMux()

	authHandler := auth.NewHandler(cfg.AuthService)
	watchlistSvc := watchlist.NewService(cfg.DB)
	watchlistHandler := watchlist.NewHandler(watchlistSvc)
	animeHandler := handlers.NewAnimeHandler(cfg.JikanClient, cfg.DB)

	// Serve static files
	fs := http.FileServer(http.Dir("./static"))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))

	// Anime / Search / Catalog
	mux.HandleFunc("/", animeHandler.HandleCatalog)
	mux.HandleFunc("/search", animeHandler.HandleSearch)
	mux.HandleFunc("/api/search", animeHandler.HandleAPISearch)
	mux.HandleFunc("/api/catalog", animeHandler.HandleAPICatalog)
	mux.HandleFunc("/anime/", animeHandler.HandleAnimeDetails)
	mux.HandleFunc("/api/anime/", animeHandler.HandleAPIAnimeRelations)

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
	return middleware.Auth(cfg.AuthService)(protectedHandler)
}
