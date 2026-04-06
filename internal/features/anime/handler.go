package anime

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"malago/internal/database"
	"malago/internal/shared/middleware"
	"malago/internal/templates"
)

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

	// Limit to 10 results
	results := res.Animes
	if len(results) > 10 {
		results = results[:10]
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
