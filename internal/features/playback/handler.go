package playback

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"mal/internal/jikan"
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
	data, err := h.svc.BuildWatchPageData(ctx, malID, title, episode, mode)
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
		InitialMode:    data.InitialMode,
		AvailableModes: data.AvailableModes,
		ModeSources:    convertModeSources(data.ModeSources),
		Segments:       convertSegments(data.Segments),
	}

	templates.WatchPage(anime, pageData).Render(r.Context(), w)
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
