package handlers

import (
	"log"
	"net/http"
	"strconv"

	"malago/internal/database"
	"malago/internal/jikan"
	"malago/internal/middleware"
	"malago/internal/templates"
)

type AnimeHandler struct {
	jikan *jikan.Client
	db    *database.Queries
}

func NewAnimeHandler(jikan *jikan.Client, db *database.Queries) *AnimeHandler {
	return &AnimeHandler{jikan: jikan, db: db}
}

func (h *AnimeHandler) HandleCatalog(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	templates.Catalog().Render(r.Context(), w)
}

func (h *AnimeHandler) HandleSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		templates.Search("").Render(r.Context(), w)
		return
	}

	// Check if HTMX request for results only
	if r.Header.Get("HX-Request") == "true" {
		res, err := h.jikan.Search(query, 1)
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
}

func (h *AnimeHandler) HandleAPISearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	pageStr := r.URL.Query().Get("page")
	page, _ := strconv.Atoi(pageStr)
	if page < 1 {
		page = 1
	}

	res, err := h.jikan.Search(query, page)
	if err != nil {
		log.Printf("search pagination error: %v", err)
		http.Error(w, "Failed to fetch search page", http.StatusInternalServerError)
		return
	}

	templates.SearchItems(query, res.Animes, page+1, res.HasNextPage).Render(r.Context(), w)
}

func (h *AnimeHandler) HandleAPICatalog(w http.ResponseWriter, r *http.Request) {
	pageStr := r.URL.Query().Get("page")
	page, _ := strconv.Atoi(pageStr)
	if page < 1 {
		page = 1
	}

	res, err := h.jikan.GetTopAnime(page)
	if err != nil {
		log.Printf("top anime error: %v", err)
		http.Error(w, "Failed to fetch top anime", http.StatusInternalServerError)
		return
	}

	templates.CatalogItems(res.Animes, page+1, res.HasNextPage).Render(r.Context(), w)
}

func (h *AnimeHandler) HandleAnimeDetails(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Path[len("/anime/"):]
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}

	anime, err := h.jikan.GetAnimeByID(id)
	if err != nil {
		log.Printf("anime fetch error for %d: %v", id, err)
		http.Error(w, "Failed to fetch anime details", http.StatusInternalServerError)
		return
	}

	// Get current watchlist status if user is logged in
	currentStatus := ""
	if user := middleware.GetUser(r.Context()); user != nil {
		entry, err := h.db.GetWatchListEntry(r.Context(), database.GetWatchListEntryParams{
			UserID:  user.ID,
			AnimeID: int64(id),
		})
		if err == nil {
			currentStatus = entry.Status
		}
	}

	templates.AnimeDetails(anime, currentStatus).Render(r.Context(), w)
}

func (h *AnimeHandler) HandleAPIAnimeRelations(w http.ResponseWriter, r *http.Request) {
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

	relations := h.jikan.GetFullRelations(id)
	templates.AnimeRelationsList(relations).Render(r.Context(), w)
}
