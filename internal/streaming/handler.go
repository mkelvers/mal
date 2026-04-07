package streaming

import (
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net/http"
	"strconv"

	"mal/internal/jikan"
	"mal/internal/nyaa"
	"mal/internal/templates"
)

type Handler struct {
	svc   *Service
	jikan *jikan.Client
}

func NewHandler(svc *Service, jikanClient *jikan.Client) *Handler {
	return &Handler{svc: svc, jikan: jikanClient}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// Watch page
	mux.HandleFunc("GET /watch/{animeID}/{episode}", h.HandleWatchPage)

	// Search endpoints
	mux.HandleFunc("GET /api/stream/search", h.HandleSearch)
	mux.HandleFunc("GET /api/stream/search/episode", h.HandleSearchEpisode)
	mux.HandleFunc("GET /api/stream/search-htmx", h.HandleSearchHTMX)

	// Streaming endpoints
	mux.HandleFunc("POST /api/stream/start", h.HandleStartStream)
	mux.HandleFunc("GET /api/stream/video/{hash}", h.HandleStreamVideo)
	mux.HandleFunc("GET /api/stream/info/{hash}", h.HandleStreamInfo)
	mux.HandleFunc("DELETE /api/stream/{hash}", h.HandleDropStream)

	// HLS endpoints
	mux.HandleFunc("POST /api/stream/hls/{hash}", h.HandleStartHLS)
	mux.HandleFunc("GET /api/stream/hls/{hash}/playlist.m3u8", h.HandleHLSPlaylist)
	mux.HandleFunc("GET /api/stream/hls/{hash}/{segment}", h.HandleHLSSegment)
}

// HandleWatchPage renders the video player page for an episode
func (h *Handler) HandleWatchPage(w http.ResponseWriter, r *http.Request) {
	animeIDStr := r.PathValue("animeID")
	episodeStr := r.PathValue("episode")

	animeID, err := strconv.Atoi(animeIDStr)
	if err != nil || animeID <= 0 {
		http.NotFound(w, r)
		return
	}

	episode, err := strconv.Atoi(episodeStr)
	if err != nil || episode <= 0 {
		http.NotFound(w, r)
		return
	}

	anime, err := h.jikan.GetAnimeByID(animeID)
	if err != nil {
		log.Printf("failed to get anime %d: %v", animeID, err)
		http.Error(w, "Failed to fetch anime", http.StatusInternalServerError)
		return
	}

	// Build list of title variations to try
	// Fansubs typically use English titles or romaji
	var titles []string
	if anime.TitleEnglish != "" {
		titles = append(titles, anime.TitleEnglish)
	}
	titles = append(titles, anime.Title) // Usually romaji
	titles = append(titles, anime.TitleSynonyms...)

	// Search using title variations until we find results
	var torrents []nyaa.Torrent
	for _, title := range titles {
		torrents, err = h.svc.SearchEpisode(title, episode)
		if err != nil {
			log.Printf("torrent search error for %q: %v", title, err)
			continue
		}
		if len(torrents) > 0 {
			break
		}
	}

	// Filter to 1080p by default, fallback to all
	filtered := nyaa.FilterByQuality(torrents, "1080p")
	if len(filtered) == 0 {
		filtered = torrents
	}

	// Limit to top 10
	if len(filtered) > 10 {
		filtered = filtered[:10]
	}

	templates.WatchPage(anime, episode, filtered).Render(r.Context(), w)
}

// HandleSearch searches nyaa for anime torrents
func (h *Handler) HandleSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "query required", http.StatusBadRequest)
		return
	}

	torrents, err := h.svc.SearchAnime(query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	quality := r.URL.Query().Get("quality")
	if quality != "" {
		torrents = nyaa.FilterByQuality(torrents, quality)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(torrents)
}

// HandleSearchEpisode searches nyaa for a specific episode
func (h *Handler) HandleSearchEpisode(w http.ResponseWriter, r *http.Request) {
	title := r.URL.Query().Get("title")
	episodeStr := r.URL.Query().Get("episode")

	if title == "" || episodeStr == "" {
		http.Error(w, "title and episode required", http.StatusBadRequest)
		return
	}

	episode, err := strconv.Atoi(episodeStr)
	if err != nil {
		http.Error(w, "invalid episode number", http.StatusBadRequest)
		return
	}

	torrents, err := h.svc.SearchEpisode(title, episode)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	quality := r.URL.Query().Get("quality")
	if quality != "" {
		torrents = nyaa.FilterByQuality(torrents, quality)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(torrents)
}

type startStreamRequest struct {
	Magnet string `json:"magnet"`
}

type startStreamResponse struct {
	InfoHash string `json:"info_hash"`
	Name     string `json:"name"`
	Size     int64  `json:"size"`
}

// HandleStartStream starts streaming a torrent from a magnet link
func (h *Handler) HandleStartStream(w http.ResponseWriter, r *http.Request) {
	var req startStreamRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Magnet == "" {
		http.Error(w, "magnet required", http.StatusBadRequest)
		return
	}

	info, err := h.svc.AddMagnet(r.Context(), req.Magnet)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(startStreamResponse{
		InfoHash: info.InfoHash,
		Name:     info.Name,
		Size:     info.Size,
	})
}

// HandleStreamVideo streams the video file from a torrent
func (h *Handler) HandleStreamVideo(w http.ResponseWriter, r *http.Request) {
	hash := r.PathValue("hash")
	if hash == "" {
		http.Error(w, "hash required", http.StatusBadRequest)
		return
	}

	if err := h.svc.StreamVideo(w, r, hash); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// HandleStreamInfo returns info about an active stream
func (h *Handler) HandleStreamInfo(w http.ResponseWriter, r *http.Request) {
	hash := r.PathValue("hash")
	if hash == "" {
		http.Error(w, "hash required", http.StatusBadRequest)
		return
	}

	info, err := h.svc.GetStreamInfo(hash)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

// HandleDropStream stops and removes a stream
func (h *Handler) HandleDropStream(w http.ResponseWriter, r *http.Request) {
	hash := r.PathValue("hash")
	if hash == "" {
		http.Error(w, "hash required", http.StatusBadRequest)
		return
	}

	h.svc.DropTorrent(hash)
	w.WriteHeader(http.StatusNoContent)
}

// TorrentResult for HTMX responses
type TorrentResult struct {
	Torrents []nyaa.Torrent
	Query    string
	Episode  int
}

// HandleSearchHTMX returns HTML for torrent search results
func (h *Handler) HandleSearchHTMX(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		fmt.Fprint(w, `<div class="search-empty">enter a search query</div>`)
		return
	}

	torrents, err := h.svc.SearchAnime(query)
	if err != nil {
		fmt.Fprintf(w, `<div class="search-error">error: %s</div>`, html.EscapeString(err.Error()))
		return
	}

	quality := r.URL.Query().Get("quality")
	if quality != "" {
		torrents = nyaa.FilterByQuality(torrents, quality)
	}

	// Filter to only torrents with magnet links
	var withMagnets []nyaa.Torrent
	for _, t := range torrents {
		if t.Magnet != "" {
			withMagnets = append(withMagnets, t)
		}
	}

	if len(withMagnets) == 0 {
		fmt.Fprint(w, `<div class="search-empty">no torrents found (or no magnet links available)</div>`)
		return
	}

	// Return simple HTML list
	fmt.Fprint(w, `<div class="torrent-list">`)
	for _, t := range withMagnets {
		fmt.Fprintf(w, `
			<div class="torrent-item" data-magnet="%s">
				<div class="torrent-title">%s</div>
				<div class="torrent-meta">
					<span class="torrent-size">%s</span>
					<span class="torrent-seeds">%d seeds</span>
				</div>
			</div>
		`, html.EscapeString(t.Magnet), html.EscapeString(t.Title), html.EscapeString(t.Size), t.Seeders)
	}
	fmt.Fprint(w, `</div>`)
}

// HandleStartHLS starts HLS transcoding for a torrent
func (h *Handler) HandleStartHLS(w http.ResponseWriter, r *http.Request) {
	hash := r.PathValue("hash")
	if hash == "" {
		http.Error(w, "hash required", http.StatusBadRequest)
		return
	}

	// Check torrent exists
	if _, ok := h.svc.GetTorrent(hash); !ok {
		http.Error(w, "torrent not found - start stream first", http.StatusNotFound)
		return
	}

	session, err := h.svc.StartHLS(r.Context(), hash)
	if err != nil {
		log.Printf("HLS start error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"playlist": fmt.Sprintf("/api/stream/hls/%s/playlist.m3u8", hash),
		"status":   "ready",
		"output":   session.OutputDir,
	})
}

// HandleHLSPlaylist serves the HLS playlist file
func (h *Handler) HandleHLSPlaylist(w http.ResponseWriter, r *http.Request) {
	hash := r.PathValue("hash")
	if hash == "" {
		http.Error(w, "hash required", http.StatusBadRequest)
		return
	}

	session, ok := h.svc.GetHLSSession(hash)
	if !ok {
		http.Error(w, "HLS session not found - start transcoding first", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, session.Playlist)
}

// HandleHLSSegment serves an HLS segment file
func (h *Handler) HandleHLSSegment(w http.ResponseWriter, r *http.Request) {
	hash := r.PathValue("hash")
	segment := r.PathValue("segment")

	if hash == "" || segment == "" {
		http.Error(w, "hash and segment required", http.StatusBadRequest)
		return
	}

	session, ok := h.svc.GetHLSSession(hash)
	if !ok {
		http.Error(w, "HLS session not found", http.StatusNotFound)
		return
	}

	// Serve the segment file
	segmentPath := fmt.Sprintf("%s/%s", session.OutputDir, segment)
	w.Header().Set("Content-Type", "video/mp2t")
	http.ServeFile(w, r, segmentPath)
}
