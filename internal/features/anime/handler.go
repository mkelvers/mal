package anime

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

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
		http.NotFound(w, r)
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
		http.Error(w, "Failed to fetch search page", http.StatusInternalServerError)
		return
	}

	res.Animes = deduplicateAnimes(res.Animes)

	templates.SearchItems(query, res.Animes, page+1, res.HasNextPage).Render(r.Context(), w)
}

func (h *Handler) HandleAPICatalog(w http.ResponseWriter, r *http.Request) {
	page := parsePageParam(r)

	res, err := h.svc.GetTopAnime(r.Context(), page)
	if err != nil {
		log.Printf("top anime error: %v", err)
		http.Error(w, "Failed to fetch top anime", http.StatusInternalServerError)
		return
	}

	res.Animes = deduplicateAnimes(res.Animes)

	templates.CatalogItems(res.Animes, page+1, res.HasNextPage).Render(r.Context(), w)
}

func (h *Handler) HandleAnimeDetails(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Path[len("/anime/"):]
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}

	userID := userIDFromRequest(r)

	anime, currentStatus, err := h.svc.GetAnimeDetails(r.Context(), id, userID)
	if err != nil {
		log.Printf("anime fetch error for %d: %v", id, err)
		http.Error(w, "Failed to fetch anime details", http.StatusInternalServerError)
		return
	}

	templates.AnimeDetails(anime, currentStatus).Render(r.Context(), w)
}

func (h *Handler) HandleAPIAnimeRelations(w http.ResponseWriter, r *http.Request) {
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

	relations, err := h.svc.GetRelations(r.Context(), id)
	if err != nil {
		log.Printf("failed to get relations for anime %d: %v", id, err)
		http.Error(w, "Failed to load relations", http.StatusInternalServerError)
		return
	}
	templates.AnimeRelationsList(relations).Render(r.Context(), w)
}

// HandleAPIAnime routes anime API requests
func (h *Handler) HandleAPIAnime(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path[len("/api/anime/"):]

	// Parse: {id}/relations or {id}/recommendations
	parts := splitPath(path)
	if len(parts) < 2 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	id, err := strconv.Atoi(parts[0])
	if err != nil || id <= 0 {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	switch parts[1] {
	case "relations":
		relations, err := h.svc.GetRelations(r.Context(), id)
		if err != nil {
			log.Printf("relations error for %d: %v", id, err)
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<p style="color: var(--text-muted); font-size: var(--text-sm);">Failed to load relations.</p>`))
			return
		}
		templates.AnimeRelationsList(relations).Render(r.Context(), w)
	case "recommendations":
		recs, err := h.svc.GetRecommendations(r.Context(), id, 12)
		if err != nil {
			log.Printf("recommendations error for %d: %v", id, err)
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<p style="color: var(--text-muted); font-size: var(--text-sm);">Failed to load recommendations.</p>`))
			return
		}
		templates.AnimeRecommendations(recs).Render(r.Context(), w)
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func splitPath(path string) []string {
	var parts []string
	var current string
	for _, c := range path {
		if c == '/' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}

func (h *Handler) HandleQuickSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode([]any{})
		return
	}

	res, err := h.svc.Search(r.Context(), query, 1)
	if err != nil {
		log.Printf("quick search error: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Limit to 5 results
	results := res.Animes
	if len(results) > 5 {
		results = results[:5]
	}

	type SearchResult struct {
		ID    int    `json:"id"`
		Title string `json:"title"`
		Type  string `json:"type"`
		Image string `json:"image"`
	}

	output := make([]SearchResult, len(results))
	for i, anime := range results {
		output[i] = SearchResult{
			ID:    anime.MalID,
			Title: anime.DisplayTitle(),
			Type:  anime.Type,
			Image: anime.ImageURL(),
		}
	}

	w.Header().Set("Content-Type", "application/json")
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

func (h *Handler) HandleSchedule(w http.ResponseWriter, r *http.Request) {
	templates.Schedule().Render(r.Context(), w)
}

func (h *Handler) HandleAPISchedule(w http.ResponseWriter, r *http.Request) {
	day := r.URL.Query().Get("day")
	if day == "" {
		day = "monday"
	}

	res, err := h.svc.GetSchedule(r.Context(), day)
	if err != nil {
		log.Printf("schedule error for %s: %v", day, err)
		http.Error(w, "Failed to fetch schedule", http.StatusInternalServerError)
		return
	}

	res.Animes = deduplicateAnimes(res.Animes)

	templates.ScheduleDay(day, res.Animes).Render(r.Context(), w)
}

func (h *Handler) HandleNotifications(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromRequest(r)

	if userID == "" {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	watching, err := h.svc.GetWatchingAnime(r.Context(), userID)
	if err != nil {
		log.Printf("watching anime error: %v", err)
		http.Error(w, "Failed to fetch watching anime", http.StatusInternalServerError)
		return
	}

	templates.Notifications(watching).Render(r.Context(), w)
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
