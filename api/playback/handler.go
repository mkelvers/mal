package playback

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"maps"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"mal/integrations/jikan"
	database "mal/internal/db"
	"mal/internal/middleware"
	"mal/templates"
)

type Handler struct {
	svc         *Service
	jikanClient *jikan.Client
}

func NewHandler(svc *Service, jikanClient *jikan.Client) *Handler {
	return &Handler{svc: svc, jikanClient: jikanClient}
}

func renderNotFoundPage(r *http.Request, w http.ResponseWriter) {
	w.WriteHeader(http.StatusNotFound)
	if err := templates.GetRenderer().ExecuteTemplate(r.Context(), w, "not_found.gohtml", map[string]any{
		"CurrentPath": r.URL.Path,
	}); err != nil {
		log.Printf("render error: %v", err)
	}
}

func (h *Handler) HandleWatchPage(w http.ResponseWriter, r *http.Request) {
	// Path is like /anime/123/watch
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 {
		renderNotFoundPage(r, w)
		return
	}
	idStr := parts[2]
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

	// Fetch episodes sequentially (pages are in correct order: 1-100, 101-200, etc.)
	pageSize := 100
	var allEpisodes []jikan.Episode
	for page := 1; ; page++ {
		resp, err := h.jikanClient.GetEpisodes(r.Context(), id, page)
		if err != nil || len(resp.Data) == 0 {
			break
		}
		allEpisodes = append(allEpisodes, resp.Data...)

		// If we got fewer than pageSize, we've reached the end
		if len(resp.Data) < pageSize {
			break
		}
	}

	user := middleware.GetUser(r.Context())

	var watchlistIDs []int64
	var watchlistStatus string
	if user != nil {
		watchlist, _ := h.svc.db.GetUserWatchList(r.Context(), user.ID)
		watchlistIDs = make([]int64, len(watchlist))
		for i, entry := range watchlist {
			watchlistIDs[i] = entry.AnimeID
			if entry.AnimeID == int64(id) {
				watchlistStatus = entry.Status
			}
		}
	}

	currentEpID := r.URL.Query().Get("ep")
	if currentEpID == "" {
		if user != nil {
			entry, err := h.svc.db.GetWatchListEntry(r.Context(), database.GetWatchListEntryParams{
				UserID:  user.ID,
				AnimeID: int64(id),
			})
			if err == nil && entry.CurrentEpisode.Valid {
				currentEpID = strconv.FormatInt(entry.CurrentEpisode.Int64, 10)
				// Redirect to the correct episode URL to keep state consistent
				http.Redirect(w, r, fmt.Sprintf("/anime/%d/watch?ep=%s", id, currentEpID), http.StatusFound)
				return
			}
		}
		currentEpID = "1"
	}

	mode := r.URL.Query().Get("mode")
	userID := ""
	if user != nil {
		userID = user.ID
	}

	titleCandidates := []string{anime.Title}
	if anime.TitleEnglish != "" && anime.TitleEnglish != anime.Title {
		titleCandidates = append(titleCandidates, anime.TitleEnglish)
	}
	if anime.TitleJapanese != "" {
		titleCandidates = append(titleCandidates, anime.TitleJapanese)
	}

	watchData, err := h.svc.BuildWatchPageData(r.Context(), id, titleCandidates, currentEpID, mode, userID)
	if err != nil {
		log.Printf("watch data error: %v", err)
	}

	// Fill gaps with placeholder episodes if fallback has more
	if watchData.FallbackEpisodes != nil {
		maxCount := 0
		for _, count := range watchData.FallbackEpisodes {
			if count > maxCount {
				maxCount = count
			}
		}

		epMap := make(map[int]jikan.Episode)
		for _, ep := range allEpisodes {
			epMap[ep.MalID] = ep
		}

		if maxCount > 0 {
			var filled []jikan.Episode
			for i := 1; i <= maxCount; i++ {
				if ep, ok := epMap[i]; ok {
					filled = append(filled, ep)
				} else {
					filled = append(filled, jikan.Episode{
						MalID:   i,
						Episode: fmt.Sprintf("Episode %d", i),
						Title:   fmt.Sprintf("Episode %d", i),
					})
				}
			}
			allEpisodes = filled
		}
	}

	sort.Slice(allEpisodes, func(i, j int) bool {
		return allEpisodes[i].MalID < allEpisodes[j].MalID
	})

	if err := templates.GetRenderer().ExecuteTemplate(r.Context(), w, "watch.gohtml", map[string]any{
		"Anime":           anime,
		"Episodes":        allEpisodes,
		"WatchData":       watchData,
		"User":            user,
		"CurrentPath":     r.URL.Path,
		"CurrentEpID":     currentEpID,
		"WatchlistIDs":    watchlistIDs,
		"WatchlistStatus": watchlistStatus,
	}); err != nil {
		log.Printf("render error: %v", err)
	}
}

func (h *Handler) HandleProxy(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "missing token", http.StatusBadRequest)
		return
	}

	scope := proxyScopeStream
	if strings.HasSuffix(r.URL.Path, "/segment") {
		scope = proxyScopeSegment
	} else if strings.HasSuffix(r.URL.Path, "/subtitle") {
		scope = proxyScopeSubtitle
	}

	targetURL, referer, err := h.svc.resolveProxyToken(r.Context(), token, scope)
	if err != nil {
		http.Error(w, "invalid token", http.StatusForbidden)
		return
	}

	rangeHeader := r.Header.Get("Range")

	statusCode, headers, content, bodyReader, err := h.svc.ProxyStream(r.Context(), targetURL, referer, rangeHeader)
	if err != nil {
		log.Printf("proxy error for %s: %v", targetURL, err)
		http.Error(w, "proxy failed", http.StatusBadGateway)
		return
	}

	maps.Copy(w.Header(), headers)
	w.WriteHeader(statusCode)

	if bodyReader != nil {
		defer bodyReader.Close()
		_, _ = io.Copy(w, bodyReader)
	} else {
		_, _ = w.Write(content)
	}
}

func (h *Handler) HandleSaveProgress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := middleware.GetUser(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		MalID       int64   `json:"mal_id"`
		Episode     int     `json:"episode"`
		TimeSeconds float64 `json:"time_seconds"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// We fetch the anime info to seed the DB if it's the first time saving progress for this show
	anime, err := h.jikanClient.GetAnimeByID(r.Context(), int(req.MalID))
	var seed *database.UpsertAnimeParams
	if err == nil {
		seed = &database.UpsertAnimeParams{
			ID:              int64(anime.MalID),
			TitleOriginal:   anime.Title,
			TitleEnglish:    sql.NullString{String: anime.TitleEnglish, Valid: anime.TitleEnglish != ""},
			TitleJapanese:   sql.NullString{String: anime.TitleJapanese, Valid: anime.TitleJapanese != ""},
			ImageUrl:        anime.ImageURL(),
			Airing:          sql.NullBool{Bool: anime.Airing, Valid: true},
			DurationSeconds: sql.NullFloat64{Float64: anime.DurationSeconds(), Valid: anime.DurationSeconds() > 0},
		}
	}

	if err := h.svc.SaveProgress(r.Context(), user.ID, req.MalID, req.Episode, req.TimeSeconds, seed); err != nil {
		log.Printf("failed to save progress: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) HandleCompleteAnime(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := middleware.GetUser(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		MalID   int64 `json:"mal_id"`
		Episode int   `json:"episode"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Seed anime info if needed
	anime, err := h.jikanClient.GetAnimeByID(r.Context(), int(req.MalID))
	var seed *database.UpsertAnimeParams
	if err == nil {
		seed = &database.UpsertAnimeParams{
			ID:              int64(anime.MalID),
			TitleOriginal:   anime.Title,
			TitleEnglish:    sql.NullString{String: anime.TitleEnglish, Valid: anime.TitleEnglish != ""},
			TitleJapanese:   sql.NullString{String: anime.TitleJapanese, Valid: anime.TitleJapanese != ""},
			ImageUrl:        anime.ImageURL(),
			Airing:          sql.NullBool{Bool: anime.Airing, Valid: true},
			DurationSeconds: sql.NullFloat64{Float64: anime.DurationSeconds(), Valid: anime.DurationSeconds() > 0},
		}
	}

	if err := h.svc.CompleteAnime(r.Context(), user.ID, req.MalID, req.Episode, seed); err != nil {
		log.Printf("failed to complete anime: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) HandleEpisodeData(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	// /api/watch/episode/{animeId}/{episodeId}
	if len(parts) < 6 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	animeID, err := strconv.Atoi(parts[4])
	if err != nil {
		http.Error(w, "invalid animeId", http.StatusBadRequest)
		return
	}

	episodeID := parts[5]

	user := middleware.GetUser(r.Context())
	userID := ""
	if user != nil {
		userID = user.ID
	}

	anime, err := h.jikanClient.GetAnimeByID(r.Context(), animeID)
	if err != nil {
		http.Error(w, "anime not found", http.StatusNotFound)
		return
	}

	titleCandidates := []string{anime.Title}
	if anime.TitleEnglish != "" && anime.TitleEnglish != anime.Title {
		titleCandidates = append(titleCandidates, anime.TitleEnglish)
	}
	if anime.TitleJapanese != "" {
		titleCandidates = append(titleCandidates, anime.TitleJapanese)
	}

	watchData, err := h.svc.BuildWatchPageData(r.Context(), animeID, titleCandidates, episodeID, "", userID)
	if err != nil {
		http.Error(w, "failed to build watch data", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"mal_id":          watchData.MalID,
		"title":           watchData.Title,
		"current_episode": watchData.CurrentEpisode,
		"total_episodes":  anime.Episodes,
		"initial_mode":    watchData.InitialMode,
		"token":           "", // The token might be per-source, wait, in Go it was per-mode?
		"available_modes": watchData.AvailableModes,
		"mode_sources":    watchData.ModeSources,
		"segments":        watchData.Segments,
		"episode_title":   "", // Find episode title if possible
	})
}

func (h *Handler) HandleEpisodeThumbnails(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	// /api/watch/thumbnails/{animeId}
	if len(parts) < 5 {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	id, err := strconv.Atoi(parts[4])
	if err != nil {
		http.Error(w, "invalid animeId", http.StatusBadRequest)
		return
	}

	// Fetch episodes sequentially
	pageSize := 100
	var allEpisodes []jikan.Episode
	for page := 1; ; page++ {
		resp, err := h.jikanClient.GetEpisodes(r.Context(), id, page)
		if err != nil || len(resp.Data) == 0 {
			break
		}
		allEpisodes = append(allEpisodes, resp.Data...)
		if len(resp.Data) < pageSize {
			break
		}
	}

	// Fill gaps if anime has known total
	anime, _ := h.jikanClient.GetAnimeByID(r.Context(), id)
	if anime.Episodes > 0 && anime.Episodes > len(allEpisodes) {
		epMap := make(map[int]jikan.Episode)
		for _, ep := range allEpisodes {
			epMap[ep.MalID] = ep
		}
		var filled []jikan.Episode
		for i := 1; i <= anime.Episodes; i++ {
			if ep, ok := epMap[i]; ok {
				filled = append(filled, ep)
			} else {
				filled = append(filled, jikan.Episode{
					MalID:   i,
					Episode: fmt.Sprintf("Episode %d", i),
					Title:   fmt.Sprintf("Episode %d", i),
				})
			}
		}
		allEpisodes = filled
	}

	type Result struct {
		MalID int    `json:"mal_id"`
		Title string `json:"title"`
	}

	results := make([]Result, len(allEpisodes))
	for i, ep := range allEpisodes {
		results[i] = Result{
			MalID: ep.MalID,
			Title: ep.Title,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}
