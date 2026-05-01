package playback

import (
	"encoding/json"
	"io"
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

	user, _ := r.Context().Value(ctxpkg.UserKey).(*database.User)

	currentEpID := r.URL.Query().Get("ep")
	if currentEpID == "" {
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
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

func (h *Handler) HandleCompleteAnime(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
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

	user, _ := r.Context().Value(ctxpkg.UserKey).(*database.User)
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
		"mal_id":           watchData.MalID,
		"title":            watchData.Title,
		"current_episode":  watchData.CurrentEpisode,
		"total_episodes":   anime.Episodes,
		"initial_mode":     watchData.InitialMode,
		"token":            "", // The token might be per-source, wait, in Go it was per-mode?
		"available_modes":  watchData.AvailableModes,
		"mode_sources":     watchData.ModeSources,
		"segments":         watchData.Segments,
		"episode_title":    "", // Find episode title if possible
	})
}
