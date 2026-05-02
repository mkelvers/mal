package anime

import (
	"encoding/json"
	"html"
	"log"
	"net/http"
	"strconv"
	"strings"

	"mal/integrations/jikan"
	ctxpkg "mal/internal/context"
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
	if err := templates.GetRenderer().ExecuteTemplate(w, "not_found.gohtml", map[string]any{
		"CurrentPath": r.URL.Path,
	}); err != nil {
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

	currentlyAiring, err := h.jikanClient.GetSeasonsNow(r.Context(), 1)
	if err != nil {
		log.Printf("seasons now error: %v", err)
		// non-fatal
	}

	if len(animes.Animes) > 6 {
		animes.Animes = animes.Animes[:6]
	}
	if len(currentlyAiring.Animes) > 6 {
		currentlyAiring.Animes = currentlyAiring.Animes[:6]
	}

	// Fetch continue watching if logged in (user ID in context, handle this safely)
	// We'll skip DB fetch for continue watching for now if it requires complex session parsing
	// Actually we should try to fetch it if we can.
	var cw []database.GetContinueWatchingEntriesRow
	watchlistMap := make(map[int64]bool)
	var watchlistIDs []int64
	user, userOk := r.Context().Value(ctxpkg.UserKey).(*database.User)
	if userOk && user != nil {
		cw, _ = h.db.GetContinueWatchingEntries(r.Context(), user.ID)
		watchlist, _ := h.db.GetUserWatchList(r.Context(), user.ID)
		watchlistIDs = make([]int64, len(watchlist))
		for i, entry := range watchlist {
			watchlistMap[entry.AnimeID] = true
			watchlistIDs[i] = entry.AnimeID
		}
	}

	if err := templates.GetRenderer().ExecuteTemplate(w, "index.gohtml", map[string]any{
		"MostPopular":      animes.Animes,
		"CurrentlyAiring":  currentlyAiring.Animes,
		"ContinueWatching": cw,
		"User":             user,
		"CurrentPath":      r.URL.Path,
		"WatchlistMap":     watchlistMap,
		"WatchlistIDs":     watchlistIDs,
	}); err != nil {
		log.Printf("render error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (h *Handler) HandleBrowse(w http.ResponseWriter, r *http.Request) {
	user, _ := r.Context().Value(ctxpkg.UserKey).(*database.User)

	q := r.URL.Query().Get("q")
	animeType := r.URL.Query().Get("type")
	status := r.URL.Query().Get("status")
	orderBy := r.URL.Query().Get("order_by")
	sort := r.URL.Query().Get("sort")

	var genres []int
	for _, g := range r.URL.Query()["genres"] {
		id, err := strconv.Atoi(g)
		if err == nil {
			genres = append(genres, id)
		}
	}

	res, err := h.jikanClient.SearchAdvanced(r.Context(), q, animeType, status, orderBy, sort, genres, 1, 24)
	if err != nil {
		log.Printf("browse error: %v", err)
	}

	genresList, err := h.jikanClient.GetAnimeGenres(r.Context())
	if err != nil {
		log.Printf("genres error: %v", err)
	}

	watchlistMap := make(map[int64]bool)
	var watchlistIDs []int64
	if user != nil {
		watchlist, _ := h.db.GetUserWatchList(r.Context(), user.ID)
		watchlistIDs = make([]int64, len(watchlist))
		for i, entry := range watchlist {
			watchlistMap[entry.AnimeID] = true
			watchlistIDs[i] = entry.AnimeID
		}
	}

	if err := templates.GetRenderer().ExecuteTemplate(w, "browse.gohtml", map[string]any{
		"User":         user,
		"CurrentPath":  r.URL.Path,
		"Query":        q,
		"Type":         animeType,
		"Status":       status,
		"OrderBy":      orderBy,
		"Sort":         sort,
		"Genres":       genres,
		"GenresList":   genresList,
		"Animes":       res.Animes,
		"WatchlistMap": watchlistMap,
		"WatchlistIDs": watchlistIDs,
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
	idStr := strings.TrimPrefix(r.URL.Path, "/anime/")
	idStr = strings.TrimSuffix(idStr, "/")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		renderNotFoundPage(r, w)
		return
	}

	anime, err := h.jikanClient.GetAnimeByID(r.Context(), id)
	if err != nil {
		renderNotFoundPage(r, w)
		return
	}

	user, _ := r.Context().Value(ctxpkg.UserKey).(*database.User)

	var status string
	var watchlistIDs []int64
	if user != nil {
		entry, err := h.db.GetWatchListEntry(r.Context(), database.GetWatchListEntryParams{
			UserID:  user.ID,
			AnimeID: int64(id),
		})
		if err == nil {
			status = entry.Status
		}
		watchlist, _ := h.db.GetUserWatchList(r.Context(), user.ID)
		watchlistIDs = make([]int64, len(watchlist))
		for i, e := range watchlist {
			watchlistIDs[i] = e.AnimeID
		}
	}

	if err := templates.GetRenderer().ExecuteTemplate(w, "anime.gohtml", map[string]any{
		"Anime":        anime,
		"User":         user,
		"Status":       status,
		"CurrentPath":  r.URL.Path,
		"WatchlistIDs": watchlistIDs,
	}); err != nil {
		log.Printf("render error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (h *Handler) HandleHTMLWatchOrder(w http.ResponseWriter, r *http.Request) {
	animeIdStr := r.URL.Query().Get("animeId")
	id, err := strconv.Atoi(animeIdStr)
	if err != nil {
		http.Error(w, `<div class="mt-8 text-sm text-red-400">Invalid anime ID.</div>`, http.StatusBadRequest)
		return
	}

	relations, err := h.jikanClient.GetFullRelations(r.Context(), id)
	if err != nil {
		log.Printf("watch order error: %v", err)
		http.Error(w, `<div class="mt-8 text-sm text-red-400">Failed to load watch order.</div>`, http.StatusInternalServerError)
		return
	}

	user, _ := r.Context().Value(ctxpkg.UserKey).(*database.User)
	watchlistMap := make(map[int64]bool)
	if user != nil {
		watchlist, _ := h.db.GetUserWatchList(r.Context(), user.ID)
		for _, entry := range watchlist {
			watchlistMap[entry.AnimeID] = true
		}
	}

	if err := templates.GetRenderer().ExecuteFragment(w, "anime.gohtml", "watch_order", map[string]any{
		"Relations":    relations,
		"AnimeID":      id,
		"WatchlistMap": watchlistMap,
	}); err != nil {
		log.Printf("render error: %v", err)
	}
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
	res, err := h.jikanClient.SearchAdvanced(r.Context(), query, "", "", "", "", nil, 1, 5)
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
	trending, err := h.jikanClient.GetSeasonsNow(r.Context(), 1)
	if err != nil {
		log.Printf("seasons now error: %v", err)
	}

	upcoming, err := h.jikanClient.GetSeasonsUpcoming(r.Context(), 1)
	if err != nil {
		log.Printf("seasons upcoming error: %v", err)
	}

	top, err := h.jikanClient.GetTopAnime(r.Context(), 1)
	if err != nil {
		log.Printf("top anime error: %v", err)
	}

	seen := make(map[int]bool)
	uniqueTrending := make([]jikan.Anime, 0)
	for _, a := range trending.Animes {
		if !seen[a.MalID] {
			seen[a.MalID] = true
			uniqueTrending = append(uniqueTrending, a)
		}
		if len(uniqueTrending) >= 10 {
			break
		}
	}

	uniqueUpcoming := make([]jikan.Anime, 0)
	for _, a := range upcoming.Animes {
		if !seen[a.MalID] {
			seen[a.MalID] = true
			uniqueUpcoming = append(uniqueUpcoming, a)
		}
		if len(uniqueUpcoming) >= 10 {
			break
		}
	}

	uniqueTop := make([]jikan.Anime, 0)
	for _, a := range top.Animes {
		if !seen[a.MalID] {
			seen[a.MalID] = true
			uniqueTop = append(uniqueTop, a)
		}
		if len(uniqueTop) >= 10 {
			break
		}
	}

	user, _ := r.Context().Value(ctxpkg.UserKey).(*database.User)
	watchlistMap := make(map[int64]bool)
	var watchlistIDs []int64
	if user != nil {
		watchlist, _ := h.db.GetUserWatchList(r.Context(), user.ID)
		watchlistIDs = make([]int64, len(watchlist))
		for i, entry := range watchlist {
			watchlistMap[entry.AnimeID] = true
			watchlistIDs[i] = entry.AnimeID
		}
	}

	if err := templates.GetRenderer().ExecuteTemplate(w, "discover.gohtml", map[string]any{
		"User":         user,
		"CurrentPath":  r.URL.Path,
		"Trending":     uniqueTrending,
		"Upcoming":     uniqueUpcoming,
		"Top":          uniqueTop,
		"WatchlistMap": watchlistMap,
		"WatchlistIDs": watchlistIDs,
	}); err != nil {
		log.Printf("render error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (h *Handler) HandleRandomAnime(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	anime, err := h.jikanClient.GetRandomAnime(r.Context())
	if err != nil {
		log.Printf("random anime error: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to fetch random anime"})
		return
	}

	if anime.MalID == 0 {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "No anime found"})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{"data": anime})
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
