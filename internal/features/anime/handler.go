package anime

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"mal/internal/database"
	"mal/internal/jikan"
	"mal/internal/shared/middleware"
	"mal/internal/templates"
)

func deduplicateAnimes(animes []jikan.Anime) []jikan.Anime {
	seen := make(map[int]bool)
	var result []jikan.Anime
	for _, a := range animes {
		if !seen[a.MalID] {
			seen[a.MalID] = true
			result = append(result, a)
		}
	}
	return result
}

type Handler struct {
	svc *Service
}

type quickSearchResult struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Type  string `json:"type"`
	Image string `json:"image"`
}

func renderNotFoundPage(r *http.Request, w http.ResponseWriter) {
	w.WriteHeader(http.StatusNotFound)
	templates.NotFoundPage().Render(r.Context(), w)
}

func writeInlineLoadError(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "text/html")
	_, _ = w.Write([]byte(`<p style="color: var(--text-muted); font-size: var(--text-sm);">` + message + `</p>`))
}

func parsePageParam(r *http.Request) int {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		return 1
	}

	return page
}

func userIDFromRequest(r *http.Request) string {
	user, ok := r.Context().Value(middleware.UserContextKey).(*database.User)
	if !ok || user == nil {
		return ""
	}

	return user.ID
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) HandleCatalog(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		renderNotFoundPage(r, w)
		return
	}
	templates.Catalog().Render(r.Context(), w)
}

func (h *Handler) HandleSearch(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Vary", "HX-Request")

	query := r.URL.Query().Get("q")
	if query == "" {
		templates.Search("").Render(r.Context(), w)
		return
	}

	if r.Header.Get("HX-Request") == "true" {
		res, err := h.svc.Search(r.Context(), query, 1)
		if err != nil {
			log.Printf("search error: %v", err)
			if jikan.IsRetryableError(err) || errors.Is(err, context.Canceled) {
				writeInlineLoadError(w, "Search is temporarily unavailable. Please retry in a few seconds.")
				return
			}
			http.Error(w, "Failed to search anime", http.StatusInternalServerError)
			return
		}
		templates.SearchResultsWrapper(query, res.Animes, 2, res.HasNextPage).Render(r.Context(), w)
		return
	}

	templates.Search(query).Render(r.Context(), w)
}

func (h *Handler) HandleAPISearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	page := parsePageParam(r)

	res, err := h.svc.Search(r.Context(), query, page)
	if err != nil {
		log.Printf("search pagination error: %v", err)
		if jikan.IsRetryableError(err) || errors.Is(err, context.Canceled) {
			writeInlineLoadError(w, "Unable to load more results right now. Please retry shortly.")
			return
		}
		http.Error(w, "Failed to fetch search page", http.StatusInternalServerError)
		return
	}

	res.Animes = deduplicateAnimes(res.Animes)

	templates.SearchItems(query, res.Animes, page+1, res.HasNextPage).Render(r.Context(), w)
}

func (h *Handler) HandleAPICatalog(w http.ResponseWriter, r *http.Request) {
	page := parsePageParam(r)

	res, fallbackPlaceholder, err := h.svc.GetTopAnimeWithPlaceholder(r.Context(), page)
	if err != nil {
		log.Printf("top anime error: %v", err)
		http.Error(w, "Failed to fetch top anime", http.StatusInternalServerError)
		return
	}

	if fallbackPlaceholder {
		templates.CatalogPlaceholderItems(25).Render(r.Context(), w)
		return
	}

	res.Animes = deduplicateAnimes(res.Animes)

	templates.CatalogItems(res.Animes, page+1, res.HasNextPage).Render(r.Context(), w)
}

func (h *Handler) HandleAnimeDetails(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Path[len("/anime/"):]
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		renderNotFoundPage(r, w)
		return
	}

	userID := userIDFromRequest(r)

	anime, currentStatus, err := h.svc.GetAnimeDetails(r.Context(), id, userID)
	if err != nil {
		if errors.Is(err, ErrAnimePendingFetch) {
			templates.AnimePending(id).Render(r.Context(), w)
			return
		}

		if jikan.IsNotFoundError(err) {
			renderNotFoundPage(r, w)
			return
		}

		log.Printf("anime fetch error for %d: %v", id, err)
		http.Error(w, "Failed to fetch anime details", http.StatusInternalServerError)
		return
	}

	templates.AnimeDetails(anime, currentStatus).Render(r.Context(), w)
}

func (h *Handler) HandleAPIAnime(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path[len("/api/anime/"):]

	idPart, section, ok := strings.Cut(path, "/")
	if !ok || section == "" {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	id, err := strconv.Atoi(idPart)
	if err != nil || id <= 0 {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	switch section {
	case "relations":
		relations, err := h.svc.GetRelations(r.Context(), id)
		if err != nil {
			log.Printf("relations error for %d: %v", id, err)
			writeInlineLoadError(w, "Failed to load relations.")
			return
		}
		templates.AnimeRelationsList(relations).Render(r.Context(), w)
	case "recommendations":
		recs, err := h.svc.GetRecommendations(r.Context(), id, 12)
		if err != nil {
			log.Printf("recommendations error for %d: %v", id, err)
			writeInlineLoadError(w, "Failed to load recommendations.")
			return
		}
		templates.AnimeRecommendations(recs).Render(r.Context(), w)
	default:
		renderNotFoundPage(r, w)
	}
}

func (h *Handler) HandleQuickSearch(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	query := r.URL.Query().Get("q")
	if query == "" {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode([]quickSearchResult{})
		return
	}

	res, err := h.svc.QuickSearch(r.Context(), query, 1, 5)
	if err != nil {
		log.Printf("quick search error: %v", err)
		if jikan.IsRetryableError(err) || errors.Is(err, context.Canceled) {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode([]quickSearchResult{})
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	results := res.Animes

	output := make([]quickSearchResult, len(results))
	for i, anime := range results {
		output[i] = quickSearchResult{
			ID:    anime.MalID,
			Title: anime.DisplayTitle(),
			Type:  anime.Type,
			Image: anime.ImageURL(),
		}
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(output)
}

func (h *Handler) HandleDiscover(w http.ResponseWriter, r *http.Request) {
	templates.Discover().Render(r.Context(), w)
}

func (h *Handler) HandleAPIDiscoverAiring(w http.ResponseWriter, r *http.Request) {
	page := parsePageParam(r)

	res, err := h.svc.GetAiringAnime(r.Context(), page)
	if err != nil {
		log.Printf("airing anime error: %v", err)
		http.Error(w, "Failed to fetch airing anime", http.StatusInternalServerError)
		return
	}

	res.Animes = deduplicateAnimes(res.Animes)

	templates.DiscoverItems(res.Animes, "airing", page+1, res.HasNextPage).Render(r.Context(), w)
}

func (h *Handler) HandleAPIDiscoverUpcoming(w http.ResponseWriter, r *http.Request) {
	page := parsePageParam(r)

	res, err := h.svc.GetUpcomingAnime(r.Context(), page)
	if err != nil {
		log.Printf("upcoming anime error: %v", err)
		http.Error(w, "Failed to fetch upcoming anime", http.StatusInternalServerError)
		return
	}

	res.Animes = deduplicateAnimes(res.Animes)

	templates.DiscoverItems(res.Animes, "upcoming", page+1, res.HasNextPage).Render(r.Context(), w)
}

func (h *Handler) HandleNotifications(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)

	if userID == "" {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	tab := r.URL.Query().Get("tab")
	if tab != "sequels" {
		tab = "tracking"
	}

	var watching []templates.WatchingAnimeWithDetails
	if tab == "tracking" {
		var err error
		watching, err = h.svc.GetWatchingAnime(r.Context(), userID)
		if err != nil {
			log.Printf("watching anime error: %v", err)
			http.Error(w, "Failed to fetch watching anime", http.StatusInternalServerError)
			return
		}
	}

	templates.Notifications(watching, tab).Render(r.Context(), w)
}

func (h *Handler) HandleNotificationsUpcoming(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)

	if userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	upcomingSeasons, err := h.svc.GetUpcomingSeasons(r.Context(), userID)
	if err != nil {
		log.Printf("upcoming seasons error: %v", err)
		http.Error(w, "Failed to fetch upcoming seasons", http.StatusInternalServerError)
		return
	}

	templates.UpcomingSeasonsList(upcomingSeasons).Render(r.Context(), w)
}
