package anime

import (
	"encoding/json"
	"html"
	"log"
	"net/http"
	"strconv"

	"mal/integrations/jikan"
	"mal/internal/db"
	"mal/templates"
)

type Handler struct {
	jikanClient *jikan.Client
	db          database.Querier
}

type quickSearchResult struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Type  string `json:"type"`
	Image string `json:"image"`
}

func renderNotFoundPage(r *http.Request, w http.ResponseWriter) {
	w.WriteHeader(http.StatusNotFound)
	if err := templates.GetRenderer().ExecuteTemplate(w, "not_found.gohtml", nil); err != nil {
		log.Printf("render error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func writeInlineLoadError(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "text/html")
	_, _ = w.Write([]byte(`<p style="color: var(--text-muted); font-size: var(--text-sm);">` + html.EscapeString(message) + `</p>`))
}

func parsePageParam(r *http.Request) int {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		return 1
	}
	return page
}

func NewHandler(jikanClient *jikan.Client, db database.Querier) *Handler {
	return &Handler{jikanClient: jikanClient, db: db}
}

func (h *Handler) HandleCatalog(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		renderNotFoundPage(r, w)
		return
	}

	animes, err := h.jikanClient.GetTopAnime(r.Context(), 1)
	if err != nil {
		log.Printf("top anime error: %v", err)
		http.Error(w, "Failed to fetch anime", http.StatusInternalServerError)
		return
	}

	if len(animes.Animes) > 4 {
		animes.Animes = animes.Animes[:4]
	}

	if err := templates.GetRenderer().ExecuteTemplate(w, "index.gohtml", map[string]any{
		"Animes": animes.Animes,
	}); err != nil {
		log.Printf("render error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (h *Handler) HandleSearch(w http.ResponseWriter, r *http.Request) {
	renderNotFoundPage(r, w)
}

func (h *Handler) HandleAPISearch(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented yet", http.StatusNotImplemented)
}

func (h *Handler) HandleAPICatalog(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented yet", http.StatusNotImplemented)
}

func (h *Handler) HandleAnimeDetails(w http.ResponseWriter, r *http.Request) {
	renderNotFoundPage(r, w)
}

func (h *Handler) HandleAPIAnime(w http.ResponseWriter, r *http.Request) {
	renderNotFoundPage(r, w)
}

func (h *Handler) HandleAPIEpisodes(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented yet", http.StatusNotImplemented)
}

func (h *Handler) HandleQuickSearch(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	query := r.URL.Query().Get("q")
	if query == "" {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode([]quickSearchResult{})
		return
	}
	res, err := h.jikanClient.SearchWithLimit(r.Context(), query, 1, 5)
	if err != nil {
		log.Printf("quick search error: %v", err)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode([]quickSearchResult{})
		return
	}
	output := make([]quickSearchResult, len(res.Animes))
	for i, anime := range res.Animes {
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
	renderNotFoundPage(r, w)
}

func (h *Handler) HandleAPIDiscoverAiring(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented yet", http.StatusNotImplemented)
}

func (h *Handler) HandleAPIDiscoverUpcoming(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented yet", http.StatusNotImplemented)
}

func (h *Handler) HandleStudioDetails(w http.ResponseWriter, r *http.Request) {
	renderNotFoundPage(r, w)
}

func (h *Handler) HandleAPIStudioAnime(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented yet", http.StatusNotImplemented)
}
