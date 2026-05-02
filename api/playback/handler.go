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
	"sync"
	"time"

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

	// Get essential episodes (first and last pages)
	allEpisodes, err := h.jikanClient.GetAllEpisodes(r.Context(), id)
	if err != nil {
		log.Printf("watch error fetching episodes: %v", err)
	}

	// Fetch any metadata overlays (thumbnails)
	videoEpisodes, _ := h.jikanClient.GetVideoEpisodes(r.Context(), id, 1)
	videoMeta := make(map[int]jikan.Episode)
	for _, ve := range videoEpisodes.Data {
		videoMeta[ve.MalID] = ve
	}

	for i, ep := range allEpisodes {
		if ve, ok := videoMeta[ep.MalID]; ok {
			if ve.Images != nil && ve.Images.Jpg.ImageURL != "" {
				allEpisodes[i].Images = ve.Images
			}
		}
	}

	// Deduplicate and prep the list
	seen := make(map[int]bool)
	unique := make([]jikan.Episode, 0)
	for _, ep := range allEpisodes {
		if !seen[ep.MalID] {
			seen[ep.MalID] = true
			unique = append(unique, ep)
		}
	}

	user := middleware.GetUser(r.Context())

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

	// Update episodes list if fallback has more
	if watchData.FallbackEpisodes != nil {
		maxCount := 0
		for _, count := range watchData.FallbackEpisodes {
			if count > maxCount {
				maxCount = count
			}
		}

		epMap := make(map[int]jikan.Episode)
		for _, ep := range unique {
			epMap[ep.MalID] = ep
		}

		if maxCount > 0 {
			var fullList []jikan.Episode
			for i := 1; i <= maxCount; i++ {
				if ep, ok := epMap[i]; ok {
					fullList = append(fullList, ep)
				} else {
					fullList = append(fullList, jikan.Episode{
						MalID:   i,
						Episode: fmt.Sprintf("Episode %d", i),
						Title:   fmt.Sprintf("Episode %d", i),
					})
				}
			}
			unique = fullList
		}
	}

	// Update episodes list if fallback has more
	if watchData.FallbackEpisodes != nil {
		maxCount := 0
		for _, count := range watchData.FallbackEpisodes {
			if count > maxCount {
				maxCount = count
			}
		}

		// Ensure we don't have duplicates or missing episodes in the sequence
		epMap := make(map[int]jikan.Episode)
		for _, ep := range unique {
			epMap[ep.MalID] = ep
		}

		if maxCount > 0 {
			var newEpisodes []jikan.Episode
			// We build the list from 1 to maxCount to ensure order and completeness
			// If we have data from Jikan, we use it. Otherwise we generate a placeholder.
			for i := 1; i <= maxCount; i++ {
				if ep, ok := epMap[i]; ok {
					newEpisodes = append(newEpisodes, ep)
				} else {
					title := fmt.Sprintf("Episode %d", i)
					newEpisodes = append(newEpisodes, jikan.Episode{
						MalID:   i,
						Episode: fmt.Sprintf("Episode %d", i),
						Title:   title,
						Images: &jikan.EpisodeImages{
							Jpg: struct {
								ImageURL string `json:"image_url"`
							}{ImageURL: ""},
						},
					})
				}
			}
			unique = newEpisodes
		}
	}

	sort.Slice(unique, func(i, j int) bool {
		return unique[i].MalID < unique[j].MalID
	})

	if err := templates.GetRenderer().ExecuteTemplate(r.Context(), w, "watch.gohtml", map[string]any{
		"Anime":       anime,
		"Episodes":    unique,
		"WatchData":   watchData,
		"User":        user,
		"CurrentPath": r.URL.Path,
		"CurrentEpID": currentEpID,
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

	// Get essential episodes (first and last pages)
	allEpisodes, err := h.jikanClient.GetAllEpisodes(r.Context(), id)
	if err != nil {
		http.Error(w, "failed to get episodes", http.StatusInternalServerError)
		return
	}

	// Also get video episodes for richer metadata (thumbnails) on recent episodes
	videoEpisodes, _ := h.jikanClient.GetVideoEpisodes(r.Context(), id, 1)

	// Merge metadata
	videoMeta := make(map[int]jikan.Episode)
	for _, ve := range videoEpisodes.Data {
		videoMeta[ve.MalID] = ve
	}

	for i, ep := range allEpisodes {
		if ve, ok := videoMeta[ep.MalID]; ok {
			if ve.Images != nil && ve.Images.Jpg.ImageURL != "" {
				allEpisodes[i].Images = ve.Images
			}
		}
	}

	// Dedup and sort
	seen := make(map[int]bool)
	unique := make([]jikan.Episode, 0, len(allEpisodes))
	for _, ep := range allEpisodes {
		if !seen[ep.MalID] {
			seen[ep.MalID] = true
			unique = append(unique, ep)
		}
	}

	// Calculate total count from anime info for complete list
	anime, _ := h.jikanClient.GetAnimeByID(r.Context(), id)
	maxCount := anime.Episodes

	epMap := make(map[int]jikan.Episode)
	for _, ep := range unique {
		epMap[ep.MalID] = ep
	}

	if maxCount > 0 {
		var fullList []jikan.Episode
		for i := 1; i <= maxCount; i++ {
			if ep, ok := epMap[i]; ok {
				fullList = append(fullList, ep)
			} else {
				fullList = append(fullList, jikan.Episode{
					MalID:   i,
					Episode: fmt.Sprintf("Episode %d", i),
					Title:   fmt.Sprintf("Episode %d", i),
				})
			}
		}
		unique = fullList
	}

	sort.Slice(unique, func(i, j int) bool {
		return unique[i].MalID < unique[j].MalID
	})

	type ThumbResult struct {
		MalID int    `json:"mal_id"`
		URL   string `json:"url"`
		Title string `json:"title,omitempty"`
	}

	results := make([]ThumbResult, len(unique))
	
	// Use a semaphore to limit concurrent scraping requests to avoid MAL bans
	sem := make(chan struct{}, 2)
	var wg sync.WaitGroup
	
	for i := range unique {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			
			sem <- struct{}{} // Acquire
			
			// Add a small jittered delay between requests to avoid 405/429
			time.Sleep(time.Duration(200+idx%300) * time.Millisecond)
			
			defer func() { <-sem }() // Release
			
			ep := unique[idx]
			imgURL := ep.GetFallbackImage(id)
			results[idx] = ThumbResult{
				MalID: ep.MalID,
				URL:   imgURL,
				Title: ep.Title,
			}
		}(i)
	}
	wg.Wait()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}
