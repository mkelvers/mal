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
		res, err := h.svc.Search(query, 1)
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
	pageStr := r.URL.Query().Get("page")
	page, _ := strconv.Atoi(pageStr)
	if page < 1 {
		page = 1
	}

	res, err := h.svc.Search(query, page)
	if err != nil {
		log.Printf("search pagination error: %v", err)
		http.Error(w, "Failed to fetch search page", http.StatusInternalServerError)
		return
	}

	res.Animes = deduplicateAnimes(res.Animes)

	templates.SearchItems(query, res.Animes, page+1, res.HasNextPage).Render(r.Context(), w)
}

func (h *Handler) HandleAPICatalog(w http.ResponseWriter, r *http.Request) {
	pageStr := r.URL.Query().Get("page")
	page, _ := strconv.Atoi(pageStr)
	if page < 1 {
		page = 1
	}

	res, err := h.svc.GetTopAnime(page)
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

	userID := ""
	if user, ok := r.Context().Value(middleware.UserContextKey).(*database.User); ok && user != nil {
		userID = user.ID
	}

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

	relations := h.svc.GetRelations(id)
	templates.AnimeRelationsList(relations).Render(r.Context(), w)
}

func (h *Handler) HandleQuickSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode([]interface{}{})
		return
	}

	res, err := h.svc.Search(query, 1)
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
	pageStr := r.URL.Query().Get("page")
	page, _ := strconv.Atoi(pageStr)
	if page < 1 {
		page = 1
	}

	res, err := h.svc.GetAiringAnime(page)
	if err != nil {
		log.Printf("airing anime error: %v", err)
		http.Error(w, "Failed to fetch airing anime", http.StatusInternalServerError)
		return
	}

	res.Animes = deduplicateAnimes(res.Animes)

	templates.DiscoverItems(res.Animes, "airing", page+1, res.HasNextPage).Render(r.Context(), w)
}

func (h *Handler) HandleAPIDiscoverUpcoming(w http.ResponseWriter, r *http.Request) {
	pageStr := r.URL.Query().Get("page")
	page, _ := strconv.Atoi(pageStr)
	if page < 1 {
		page = 1
	}

	res, err := h.svc.GetUpcomingAnime(page)
	if err != nil {
		log.Printf("upcoming anime error: %v", err)
		http.Error(w, "Failed to fetch upcoming anime", http.StatusInternalServerError)
		return
	}

	res.Animes = deduplicateAnimes(res.Animes)

	templates.DiscoverItems(res.Animes, "upcoming", page+1, res.HasNextPage).Render(r.Context(), w)
}
