package playback

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"mal/internal/database"
	"mal/internal/jikan"
	"mal/internal/shared/middleware"
	"mal/internal/templates"
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

	title := anime.DisplayTitle()
	userID := watchlistUserIDFromRequest(r)
	data, err := h.svc.BuildWatchPageData(ctx, malID, title, episode, mode, userID)
	if err != nil {
		log.Printf("watch page error for mal_id=%d: %v", malID, err)
		http.Error(w, "Failed to load playback", http.StatusBadGateway)
		return
	}

	// Convert playback.WatchPageData to templates.WatchPageData
	pageData := templates.WatchPageData{
		MalID:          data.MalID,
		Title:          data.Title,
		CurrentEpisode: data.CurrentEpisode,
		CurrentStatus:  data.CurrentStatus,
		InitialMode:    data.InitialMode,
		AvailableModes: data.AvailableModes,
		ModeSources:    convertModeSources(data.ModeSources),
		Segments:       convertSegments(data.Segments),
	}

	templates.WatchPage(anime, pageData).Render(r.Context(), w)
}

func watchlistUserIDFromRequest(r *http.Request) string {
	user, ok := r.Context().Value(middleware.UserContextKey).(*database.User)
	if !ok || user == nil {
		return ""
	}

	return user.ID
}

func convertModeSources(sources map[string]ModeSource) map[string]templates.ModeSource {
	result := make(map[string]templates.ModeSource, len(sources))
	for k, v := range sources {
		subtitles := make([]templates.SubtitleItem, len(v.Subtitles))
		for i, s := range v.Subtitles {
			subtitles[i] = templates.SubtitleItem{
				Lang:    s.Lang,
				URL:     s.URL,
				Referer: s.Referer,
			}
		}
		result[k] = templates.ModeSource{
			URL:       v.URL,
			Referer:   v.Referer,
			Subtitles: subtitles,
		}
	}
	return result
}

func convertSegments(segments []SkipSegment) []templates.SkipSegment {
	result := make([]templates.SkipSegment, len(segments))
	for i, s := range segments {
		result[i] = templates.SkipSegment{
			Type:  s.Type,
			Start: s.Start,
			End:   s.End,
		}
	}
	return result
}

func (h *Handler) HandleProxyStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	mode := normalizeMode(r.URL.Query().Get("mode"))
	if mode == "" {
		mode = "dub"
	}

	state := r.URL.Query().Get("state")
	if strings.TrimSpace(state) == "" {
		http.Error(w, "missing playback state", http.StatusBadRequest)
		return
	}

	modeSources := make(map[string]ModeSource)
	if err := json.Unmarshal([]byte(state), &modeSources); err != nil {
		http.Error(w, "invalid playback state", http.StatusBadRequest)
		return
	}

	source, ok := modeSources[mode]
	if !ok || strings.TrimSpace(source.URL) == "" {
		http.Error(w, "stream mode unavailable", http.StatusBadRequest)
		return
	}

	h.proxyUpstream(w, r, source.URL, source.Referer)
}

func (h *Handler) HandleProxySegment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	targetURL := r.URL.Query().Get("u")
	if strings.TrimSpace(targetURL) == "" {
		http.Error(w, "missing target url", http.StatusBadRequest)
		return
	}

	h.proxyUpstream(w, r, targetURL, r.URL.Query().Get("r"))
}

func (h *Handler) HandleProxySubtitle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	targetURL := r.URL.Query().Get("u")
	if strings.TrimSpace(targetURL) == "" {
		http.Error(w, "missing target url", http.StatusBadRequest)
		return
	}

	h.proxyUpstream(w, r, targetURL, r.URL.Query().Get("r"))
}

func (h *Handler) HandleProxyPreviewMap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	malIDText := strings.TrimSpace(r.URL.Query().Get("mal_id"))
	malID, err := strconv.Atoi(malIDText)
	if err != nil || malID <= 0 {
		http.Error(w, "invalid mal id", http.StatusBadRequest)
		return
	}

	episode := strings.TrimSpace(r.URL.Query().Get("ep"))
	if episode == "" {
		episode = "1"
	}

	mode := normalizeMode(r.URL.Query().Get("mode"))
	if mode == "" {
		mode = "dub"
	}

	source := strings.TrimSpace(r.URL.Query().Get("u"))
	if source == "" {
		http.Error(w, "missing target url", http.StatusBadRequest)
		return
	}

	referer := strings.TrimSpace(r.URL.Query().Get("r"))
	duration := 0.0
	durationText := strings.TrimSpace(r.URL.Query().Get("d"))
	if durationText != "" {
		parsedDuration, parseErr := strconv.ParseFloat(durationText, 64)
		if parseErr != nil || parsedDuration <= 0 {
			http.Error(w, "invalid duration", http.StatusBadRequest)
			return
		}
		duration = parsedDuration
	}

	mapData, previewKey, previewErr := h.svc.EnsurePreviewMap(r.Context(), PreviewRequest{
		MalID:    malID,
		Episode:  episode,
		Mode:     mode,
		Source:   source,
		Referer:  referer,
		Duration: duration,
	})
	if previewErr != nil {
		log.Printf("preview map error mal_id=%d ep=%s mode=%s: %v", malID, episode, mode, previewErr)
		http.Error(w, "failed to generate preview map", http.StatusBadGateway)
		return
	}

	spriteURL := "/watch/proxy/preview-sprite?k=" + url.QueryEscape(previewKey)
	response := struct {
		SpriteURL string     `json:"sprite_url"`
		Map       PreviewMap `json:"map"`
	}{
		SpriteURL: spriteURL,
		Map:       mapData,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("preview map encode error mal_id=%d ep=%s mode=%s: %v", malID, episode, mode, err)
	}
}

func (h *Handler) HandleProxyPreviewSprite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	previewKey := strings.TrimSpace(r.URL.Query().Get("k"))
	spritePath := h.svc.PreviewSpritePath(previewKey)
	if spritePath == "" {
		http.Error(w, "invalid preview key", http.StatusBadRequest)
		return
	}

	if _, err := os.Stat(spritePath); err != nil {
		http.Error(w, "preview sprite not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("Content-Type", "image/jpeg")
	http.ServeFile(w, r, spritePath)
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
