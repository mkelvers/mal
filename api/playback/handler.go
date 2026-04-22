package playback

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"mal/integrations/jikan"
	"mal/internal/db"
	"mal/internal/middleware"
	"mal/web/components/watch"
	"mal/web/shared"
	"mal/web/templates"
)

type Handler struct {
	svc         *Service
	jikanClient *jikan.Client
}

func NewHandler(svc *Service, jikanClient *jikan.Client) *Handler {
	return &Handler{svc: svc, jikanClient: jikanClient}
}

func (h *Handler) HandleWatchPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/watch/")
	path = strings.Trim(path, "/")
	if path == "" || strings.HasPrefix(path, "proxy/") {
		http.NotFound(w, r)
		return
	}

	parts := strings.Split(path, "/")
	if len(parts) < 1 {
		http.NotFound(w, r)
		return
	}

	malID, err := strconv.Atoi(parts[0])
	if err != nil || malID <= 0 {
		http.NotFound(w, r)
		return
	}

	// Get episode from path if provided, otherwise from query
	episode := ""
	if len(parts) >= 2 {
		episode = strings.TrimSpace(parts[1])
	}
	if episode == "" {
		episode = strings.TrimSpace(r.URL.Query().Get("ep"))
	}
	if episode == "" {
		episode = "1"
	}

	mode := strings.TrimSpace(r.URL.Query().Get("mode"))

	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()

	// Fetch anime details
	anime, err := h.jikanClient.GetAnimeByID(ctx, malID)
	if err != nil {
		log.Printf("failed to fetch anime %d: %v", malID, err)
		http.Error(w, "Failed to fetch anime details", http.StatusInternalServerError)
		return
	}

	if anime.Episodes > 0 {
		episodeNumber, parseErr := strconv.Atoi(episode)
		if parseErr == nil && episodeNumber > anime.Episodes {
			http.Redirect(w, r, "/watch/"+strconv.Itoa(malID)+"/"+strconv.Itoa(anime.Episodes), http.StatusFound)
			return
		}
	}

	titleCandidates := playbackTitleCandidates(anime)
	userID := watchlistUserIDFromRequest(r)
	data, err := h.svc.BuildWatchPageData(ctx, malID, titleCandidates, episode, mode, userID)
	if err != nil {
		log.Printf("watch page error for mal_id=%d: %v", malID, err)
		http.Error(w, "Failed to load playback", http.StatusBadGateway)
		return
	}

	// Convert playback.WatchPageData to shared.WatchPageData
	pageData := shared.WatchPageData{
		MalID:            data.MalID,
		Title:            data.Title,
		TitleEnglish:     anime.TitleEnglish,
		TitleJapanese:    anime.TitleJapanese,
		ImageURL:         anime.ImageURL(),
		Airing:           anime.Airing,
		CurrentEpisode:   data.CurrentEpisode,
		TotalEpisodes:    anime.Episodes,
		StartTimeSeconds: data.StartTimeSeconds,
		CurrentStatus:    data.CurrentStatus,
		InitialMode:      data.InitialMode,
		AvailableModes:   data.AvailableModes,
		ModeSources:      convertModeSources(data.ModeSources),
		Segments:         convertSegments(data.Segments),
	}

	if r.Header.Get("HX-Request") == "true" {
		if err := watch.VideoPlayer(pageData).Render(r.Context(), w); err != nil {
			log.Printf("render error: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		return
	}

	if err := templates.WatchPage(anime, pageData).Render(r.Context(), w); err != nil {
		log.Printf("render error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func watchlistUserIDFromRequest(r *http.Request) string {
	user, ok := r.Context().Value(middleware.UserContextKey).(*database.User)
	if !ok || user == nil {
		return ""
	}

	return user.ID
}

func playbackTitleCandidates(anime jikan.Anime) []string {
	out := make([]string, 0, 3+len(anime.TitleSynonyms))
	seen := make(map[string]struct{})

	add := func(value string) {
		normalized := strings.TrimSpace(value)
		if normalized == "" {
			return
		}

		key := strings.ToLower(normalized)
		if _, exists := seen[key]; exists {
			return
		}

		seen[key] = struct{}{}
		out = append(out, normalized)
	}

	add(anime.Title)
	add(anime.TitleEnglish)
	add(anime.TitleJapanese)
	for _, synonym := range anime.TitleSynonyms {
		add(synonym)
	}

	return out
}

func convertModeSources(sources map[string]ModeSource) map[string]shared.ModeSource {
	result := make(map[string]shared.ModeSource, len(sources))
	for k, v := range sources {
		subtitles := make([]shared.SubtitleItem, len(v.Subtitles))
		for i, s := range v.Subtitles {
			subtitles[i] = shared.SubtitleItem{
				Lang:  s.Lang,
				Token: s.Token,
			}
		}
		result[k] = shared.ModeSource{
			Token:     v.Token,
			Subtitles: subtitles,
		}
	}
	return result
}

func convertSegments(segments []SkipSegment) []shared.SkipSegment {
	result := make([]shared.SkipSegment, len(segments))
	for i, s := range segments {
		result[i] = shared.SkipSegment{
			Type:  s.Type,
			Start: s.Start,
			End:   s.End,
		}
	}
	return result
}

func (h *Handler) HandleProxy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		http.Error(w, "missing playback token", http.StatusBadRequest)
		return
	}

	scope := proxyScope(strings.TrimPrefix(r.URL.Path, "/watch/proxy/"))
	scopeLabel := map[proxyScope]string{
		proxyScopeStream:   "stream",
		proxyScopeSegment:  "segment",
		proxyScopeSubtitle: "subtitle",
	}[scope]
	if scopeLabel == "" {
		http.Error(w, "invalid proxy scope", http.StatusBadRequest)
		return
	}

	targetURL, referer, err := h.svc.resolveProxyToken(r.Context(), token, scope)
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid %s token", scopeLabel), http.StatusBadRequest)
		return
	}

	h.proxyUpstream(w, r, targetURL, referer)
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

	type saveProgressRequest struct {
		MalID      int     `json:"mal_id"`
		Episode    int     `json:"episode"`
		TimeSecond float64 `json:"time_seconds"`
	}

	var payload saveProgressRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&payload); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	if payload.MalID <= 0 || payload.Episode <= 0 {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	timeSeconds := payload.TimeSecond
	if timeSeconds < 0 || timeSeconds != timeSeconds {
		timeSeconds = 0
	}

	if h.svc.db == nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}

	animeID := int64(payload.MalID)

	animeSeed, err := h.ensureAnimeSeed(r.Context(), payload.MalID)
	if err != nil {
		log.Printf("save progress failed to resolve anime user_id=%s mal_id=%d err=%v", user.ID, payload.MalID, err)
		http.Error(w, "failed to save progress", http.StatusInternalServerError)
		return
	}

	if err := h.svc.SaveProgress(r.Context(), user.ID, animeID, payload.Episode, timeSeconds, animeSeed); err != nil {
		log.Printf("save progress failed user_id=%s mal_id=%d err=%v", user.ID, payload.MalID, err)
		http.Error(w, "failed to save progress", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
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

	type completeAnimeRequest struct {
		MalID   int `json:"mal_id"`
		Episode int `json:"episode"`
	}

	var payload completeAnimeRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	if payload.MalID <= 0 || payload.Episode <= 0 {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	animeID := int64(payload.MalID)
	animeSeed, err := h.ensureAnimeSeed(r.Context(), payload.MalID)
	if err != nil {
		log.Printf("complete anime failed to resolve anime user_id=%s mal_id=%d err=%v", user.ID, payload.MalID, err)
		http.Error(w, "failed to mark anime completed", http.StatusInternalServerError)
		return
	}

	if err := h.svc.CompleteAnime(r.Context(), user.ID, animeID, payload.Episode, animeSeed); err != nil {
		log.Printf("complete anime failed user_id=%s mal_id=%d err=%v", user.ID, payload.MalID, err)
		http.Error(w, "failed to mark anime completed", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ensureAnimeSeed(ctx context.Context, malID int) (*database.UpsertAnimeParams, error) {
	animeID := int64(malID)
	if _, err := h.svc.db.GetAnime(ctx, animeID); err == nil {
		return nil, nil
	}

	anime, err := h.jikanClient.GetAnimeByID(ctx, malID)
	if err != nil {
		return nil, err
	}

	return &database.UpsertAnimeParams{
		ID:            animeID,
		TitleOriginal: anime.Title,
		TitleEnglish:  sql.NullString{String: anime.TitleEnglish, Valid: anime.TitleEnglish != ""},
		TitleJapanese: sql.NullString{String: anime.TitleJapanese, Valid: anime.TitleJapanese != ""},
		ImageUrl:      anime.ImageURL(),
		Airing:        sql.NullBool{Bool: anime.Airing, Valid: true},
	}, nil
}

func (h *Handler) proxyUpstream(w http.ResponseWriter, r *http.Request, targetURL string, referer string) {
	parsed, err := url.Parse(targetURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		http.Error(w, "invalid upstream url", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	statusCode, headers, rewrittenBody, streamBody, err := h.svc.ProxyStream(ctx, targetURL, referer, r.Header.Get("Range"))
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) || errors.Is(r.Context().Err(), context.Canceled) {
			return
		}

		log.Printf("proxy error for url=%s: %v", targetURL, err)
		http.Error(w, "upstream request failed", http.StatusBadGateway)
		return
	}

	for key, values := range headers {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	w.WriteHeader(statusCode)
	if len(rewrittenBody) > 0 {
		_, _ = w.Write(rewrittenBody)
		return
	}

	if streamBody == nil {
		return
	}
	defer streamBody.Close()
	_, _ = io.Copy(w, streamBody)
}
