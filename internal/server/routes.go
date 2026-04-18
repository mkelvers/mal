package server

import (
	"net/http"

	"mal/internal/database"
	"mal/internal/features/anime"
	"mal/internal/features/auth"
	"mal/internal/features/playback"
	"mal/internal/features/watchlist"
	"mal/internal/jikan"
	"mal/internal/shared/middleware"
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

	animeSvc := anime.NewService(cfg.JikanClient, cfg.DB)
	animeHandler := anime.NewHandler(animeSvc)
	playbackSvc := playback.NewService(cfg.JikanClient, cfg.DB)
	playbackHandler := playback.NewHandler(playbackSvc, cfg.JikanClient)

	// Serve static files
	fs := http.FileServer(http.Dir("./static"))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))

	// Serve built frontend assets
	dist := http.FileServer(http.Dir("./dist"))
	mux.Handle("/dist/", http.StripPrefix("/dist/", dist))

	mux.HandleFunc("/", animeHandler.HandleCatalog)
	mux.HandleFunc("/discover", animeHandler.HandleDiscover)
	mux.HandleFunc("/notifications", animeHandler.HandleNotifications)
	mux.HandleFunc("/notifications/upcoming", animeHandler.HandleNotificationsUpcoming)
	mux.HandleFunc("/api/discover/airing", animeHandler.HandleAPIDiscoverAiring)
	mux.HandleFunc("/api/discover/upcoming", animeHandler.HandleAPIDiscoverUpcoming)
	mux.HandleFunc("/search", animeHandler.HandleSearch)
	mux.HandleFunc("/api/search", animeHandler.HandleAPISearch)
	mux.HandleFunc("/api/search-quick", animeHandler.HandleQuickSearch)
	mux.HandleFunc("/api/catalog", animeHandler.HandleAPICatalog)
	mux.HandleFunc("/anime/", animeHandler.HandleAnimeDetails)
	mux.HandleFunc("/api/anime/", animeHandler.HandleAPIAnime)
	mux.HandleFunc("/api/episodes/", animeHandler.HandleAPIEpisodes)
	mux.HandleFunc("/studios/", animeHandler.HandleStudioDetails)
	mux.HandleFunc("/api/studios/", animeHandler.HandleAPIStudioAnime)
	mux.HandleFunc("/watch/", playbackHandler.HandleWatchPage)
	mux.HandleFunc("/watch/proxy/stream", playbackHandler.HandleProxyStream)
	mux.HandleFunc("/watch/proxy/segment", playbackHandler.HandleProxySegment)
	mux.HandleFunc("/watch/proxy/subtitle", playbackHandler.HandleProxySubtitle)
	mux.HandleFunc("/api/watch-progress", playbackHandler.HandleSaveProgress)

	// Auth Endpoints
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			authHandler.HandleLoginPage(w, r)
		} else {
			middleware.RateLimitAuth(middleware.VerifyOrigin(http.HandlerFunc(authHandler.HandleLogin))).ServeHTTP(w, r)
		}
	})
	mux.HandleFunc("/logout", func(w http.ResponseWriter, r *http.Request) {
		middleware.VerifyOrigin(http.HandlerFunc(authHandler.HandleLogout)).ServeHTTP(w, r)
	})

	// Watchlist Endpoints
	mux.HandleFunc("/api/watchlist/export", watchlistHandler.HandleExportWatchlist)
	mux.HandleFunc("/api/watchlist/import", watchlistHandler.HandleImportWatchlist)
	mux.HandleFunc("/api/watchlist", watchlistHandler.HandleUpdateWatchlist)
	mux.HandleFunc("/api/watchlist/", watchlistHandler.HandleDeleteWatchlist)
	mux.HandleFunc("/watchlist", watchlistHandler.HandleGetWatchlist)

	// Wrap mux with global CSRF origin verification and auth checking,
	// THEN auth context parsing.
	protectedHandler := middleware.RequireGlobalAuth(middleware.VerifyOrigin(mux))
	return middleware.Auth(cfg.AuthService)(protectedHandler)
}
