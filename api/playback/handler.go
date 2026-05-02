package playback

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"

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
	if err := templates.GetRenderer().ExecuteTemplate(w, "not_found.gohtml", map[string]any{
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

	// Try to get video episodes first (for thumbnails)
	episodes, err := h.jikanClient.GetVideoEpisodes(r.Context(), id, 1)
	if err != nil || len(episodes.Data) == 0 {
		// Fallback to standard episodes if no video episodes
		episodes, err = h.jikanClient.GetEpisodes(r.Context(), id, 1)
		if err != nil {
			log.Printf("watch error: %v", err)
		}
	}

	var wg sync.WaitGroup
	for i := range episodes.Data {
		if episodes.Data[i].Images == nil {
			episodes.Data[i].Images = &jikan.EpisodeImages{}
		}
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			episodes.Data[idx].Images.Jpg.ImageURL = episodes.Data[idx].GetFallbackImage(id)
		}(i)
	}
	wg.Wait()

	sort.Slice(episodes.Data, func(i, j int) bool {
		return episodes.Data[i].MalID < episodes.Data[j].MalID
	})

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

		if maxCount > len(episodes.Data) {
			// Fetch metadata for the missing episodes
			start := len(episodes.Data) + 1
			for i := start; i <= maxCount; i++ {
				epStr := strconv.Itoa(i)
				meta, err := h.svc.GetEpisodeMetadata(r.Context(), id, epStr)

				title := fmt.Sprintf("Episode %d", i)
				imgURL := ""

				if err == nil && meta != nil {
					if info, ok := meta["episodeInfo"].(map[string]any); ok {
						if thumbs, ok := info["thumbnails"].([]any); ok && len(thumbs) > 0 {
							if firstThumb, ok := thumbs[0].(string); ok {
								imgURL = firstThumb
							}
						}
					}
					if notes, ok := meta["notes"].(string); ok && notes != "" {
						title = notes
					}
				}

				if imgURL == "" {
					// Last resort fallback
					tmpEp := jikan.Episode{MalID: i}
					imgURL = tmpEp.GetFallbackImage(id)
				}

				episodes.Data = append(episodes.Data, jikan.Episode{
					MalID:   i,
					Episode: fmt.Sprintf("Episode %d", i),
					Title:   title,
					Images: &jikan.EpisodeImages{
						Jpg: struct {
							ImageURL string `json:"image_url"`
						}{ImageURL: imgURL},
					},
				})
			}
		}
	}

	if err := templates.GetRenderer().ExecuteTemplate(w, "watch.gohtml", map[string]any{
		"Anime":       anime,
		"Episodes":    episodes.Data,
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

	for k, v := range headers {
		w.Header()[k] = v
	}
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
			ID:            int64(anime.MalID),
			TitleOriginal: anime.Title,
			TitleEnglish:  sql.NullString{String: anime.TitleEnglish, Valid: anime.TitleEnglish != ""},
			TitleJapanese: sql.NullString{String: anime.TitleJapanese, Valid: anime.TitleJapanese != ""},
			ImageUrl:      anime.ImageURL(),
			Airing:        sql.NullBool{Bool: anime.Airing, Valid: true},
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
			ID:            int64(anime.MalID),
			TitleOriginal: anime.Title,
			TitleEnglish:  sql.NullString{String: anime.TitleEnglish, Valid: anime.TitleEnglish != ""},
			TitleJapanese: sql.NullString{String: anime.TitleJapanese, Valid: anime.TitleJapanese != ""},
			ImageUrl:      anime.ImageURL(),
			Airing:        sql.NullBool{Bool: anime.Airing, Valid: true},
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
