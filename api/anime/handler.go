package anime

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"mal/integrations/jikan"
	"mal/internal/db"
	"mal/internal/middleware"
	animecomponents "mal/web/components/anime"
	watchcomponents "mal/web/components/watch"
	"mal/web/templates"
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

func NewHandler(jikanClient *jikan.Client, db database.Querier) *Handler {
	return &Handler{jikanClient: jikanClient, db: db}
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
		res, err := h.jikanClient.Search(r.Context(), query, 1)
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

	res, err := h.jikanClient.Search(r.Context(), query, page)
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

	result, err := h.jikanClient.GetTopAnime(r.Context(), page)
	if err == nil {
		result.Animes = deduplicateAnimes(result.Animes)
		templates.CatalogItems(result.Animes, page+1, result.HasNextPage).Render(r.Context(), w)
		return
	}

	if jikan.IsRetryableError(err) {
		templates.CatalogPlaceholderItems(25).Render(r.Context(), w)
		return
	}

	log.Printf("top anime error: %v", err)
	http.Error(w, "Failed to fetch top anime", http.StatusInternalServerError)
}

func (h *Handler) HandleAnimeDetails(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Path[len("/anime/"):]
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		renderNotFoundPage(r, w)
		return
	}

	userID := userIDFromRequest(r)

	anime, err := h.jikanClient.GetAnimeByID(r.Context(), id)
	if err != nil {
		if jikan.IsNotFoundError(err) {
			renderNotFoundPage(r, w)
			return
		}

		h.jikanClient.EnqueueAnimeFetchRetry(r.Context(), id, err)
		if jikan.IsRetryableError(err) {
			animecomponents.Pending(id).Render(r.Context(), w)
			return
		}

		log.Printf("anime fetch error for %d: %v", id, err)
		http.Error(w, "Failed to fetch anime details", http.StatusInternalServerError)
		return
	}

	currentStatus := ""
	nextEpisode := 1
	if userID != "" {
		entry, err := h.db.GetWatchListEntry(r.Context(), database.GetWatchListEntryParams{
			UserID:  userID,
			AnimeID: int64(id),
		})
		if err == nil {
			currentStatus = entry.Status
			if entry.CurrentEpisode.Valid {
				value := int(entry.CurrentEpisode.Int64)
				if value > 0 {
					nextEpisode = value
				}
			}
		}
	}

	templates.AnimeDetails(anime, currentStatus, nextEpisode).Render(r.Context(), w)
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
		relations, err := h.jikanClient.GetFullRelations(r.Context(), id)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return
			}
			log.Printf("relations error for %d: %v", id, err)
			writeInlineLoadError(w, "Failed to load relations.")
			return
		}
		animecomponents.RelationsList(relations).Render(r.Context(), w)
	case "recommendations":
		recs, err := h.jikanClient.GetRecommendations(r.Context(), id, 12)
		if err != nil {
			log.Printf("recommendations error for %d: %v", id, err)
			writeInlineLoadError(w, "Failed to load recommendations.")
			return
		}
		animecomponents.Recommendations(recs).Render(r.Context(), w)
	case "episodes":
		currentEpisode := r.URL.Query().Get("current")
		episodes, err := h.getEpisodes(r.Context(), id)
		if err != nil {
			log.Printf("episodes error for %d: %v", id, err)
			writeInlineLoadError(w, "Failed to load episodes.")
			return
		}
		watchcomponents.EpisodeList(episodes, currentEpisode, id).Render(r.Context(), w)
	default:
		renderNotFoundPage(r, w)
	}
}

func (h *Handler) HandleAPIEpisodes(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path[len("/api/episodes/"):]
	path = strings.Trim(path, "/")

	id, err := strconv.Atoi(path)
	if err != nil || id <= 0 {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	currentEpisode := r.URL.Query().Get("current")
	episodes, err := h.getEpisodes(r.Context(), id)
	if err != nil {
		log.Printf("episodes error: %v", err)
		writeInlineLoadError(w, "Failed to load episodes.")
		return
	}

	watchcomponents.EpisodeList(episodes, currentEpisode, id).Render(r.Context(), w)
}

func (h *Handler) getEpisodes(ctx context.Context, animeID int) ([]jikan.Episode, error) {
	var allEpisodes []jikan.Episode
	page := 1

	for page <= 20 {
		result, err := h.jikanClient.GetEpisodes(ctx, animeID, page)
		if err != nil {
			if jikan.IsRetryableError(err) && len(allEpisodes) > 0 {
				return allEpisodes, nil
			}
			return nil, err
		}

		allEpisodes = append(allEpisodes, result.Data...)

		if !result.Pagination.HasNextPage {
			break
		}
		page++
	}

	return allEpisodes, nil
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

	res, err := h.jikanClient.GetSeasonsNow(r.Context(), page)
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

	res, err := h.jikanClient.GetSeasonsUpcoming(r.Context(), page)
	if err != nil {
		log.Printf("upcoming anime error: %v", err)
		http.Error(w, "Failed to fetch upcoming anime", http.StatusInternalServerError)
		return
	}

	res.Animes = deduplicateAnimes(res.Animes)

	templates.DiscoverItems(res.Animes, "upcoming", page+1, res.HasNextPage).Render(r.Context(), w)
}

func (h *Handler) HandleStudioDetails(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Path[len("/studios/"):]
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		renderNotFoundPage(r, w)
		return
	}

	producer, err := h.jikanClient.GetProducerByID(r.Context(), id)
	if err != nil {
		if jikan.IsNotFoundError(err) {
			renderNotFoundPage(r, w)
			return
		}

		log.Printf("studio fetch error for %d: %v", id, err)
		http.Error(w, "Failed to fetch studio details", http.StatusInternalServerError)
		return
	}

	result, err := h.jikanClient.GetAnimeByProducer(r.Context(), id, 1)
	if err != nil {
		log.Printf("studio anime fetch error for %d: %v", id, err)
		if jikan.IsRetryableError(err) || errors.Is(err, context.Canceled) {
			// Render page with empty anime list if API is rate limiting
			templates.StudioDetails(producer, []jikan.Anime{}, false, 2).Render(r.Context(), w)
			return
		}
		http.Error(w, "Failed to fetch studio anime", http.StatusInternalServerError)
		return
	}

	templates.StudioDetails(producer, result.Animes, result.HasNextPage, 2).Render(r.Context(), w)
}

func (h *Handler) HandleAPIStudioAnime(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path[len("/api/studios/"):]

	idPart, after, ok := strings.Cut(path, "/")
	if !ok || after != "anime" {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	id, err := strconv.Atoi(idPart)
	if err != nil || id <= 0 {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	page := parsePageParam(r)

	result, err := h.jikanClient.GetAnimeByProducer(r.Context(), id, page)
	if err != nil {
		log.Printf("studio anime pagination error for %d page %d: %v", id, page, err)
		if jikan.IsRetryableError(err) || errors.Is(err, context.Canceled) {
			writeInlineLoadError(w, "Unable to load more results right now. Please retry shortly.")
			return
		}
		http.Error(w, "Failed to fetch studio anime", http.StatusInternalServerError)
		return
	}

	result.Animes = deduplicateAnimes(result.Animes)

	templates.StudioAnimeItems(result.Animes, result.HasNextPage, id, page+1).Render(r.Context(), w)
}
