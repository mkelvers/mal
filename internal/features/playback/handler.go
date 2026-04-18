package playback

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

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

	if anime.Episodes > 0 {
		episodeNumber, parseErr := strconv.Atoi(episode)
		if parseErr == nil && episodeNumber > anime.Episodes {
			http.Redirect(w, r, "/watch/"+strconv.Itoa(malID)+"/"+strconv.Itoa(anime.Episodes), http.StatusFound)
			return
		}
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
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
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

	if _, err := h.svc.db.GetAnime(r.Context(), animeID); err != nil {
		anime, fetchErr := h.jikanClient.GetAnimeByID(r.Context(), payload.MalID)
		if fetchErr != nil {
			log.Printf("save progress failed to fetch anime user_id=%s mal_id=%d err=%v", user.ID, payload.MalID, fetchErr)
			http.Error(w, "failed to save progress", http.StatusInternalServerError)
			return
		}

		if _, upsertErr := h.svc.db.UpsertAnime(r.Context(), database.UpsertAnimeParams{
			ID:            animeID,
			TitleOriginal: anime.Title,
			TitleEnglish:  sql.NullString{String: anime.TitleEnglish, Valid: anime.TitleEnglish != ""},
			TitleJapanese: sql.NullString{String: anime.TitleJapanese, Valid: anime.TitleJapanese != ""},
			ImageUrl:      anime.ImageURL(),
			Airing:        sql.NullBool{Bool: anime.Airing, Valid: true},
		}); upsertErr != nil {
			log.Printf("save progress failed to upsert anime user_id=%s mal_id=%d err=%v", user.ID, payload.MalID, upsertErr)
			http.Error(w, "failed to save progress", http.StatusInternalServerError)
			return
		}
	}

	watchListEntry, watchListErr := h.svc.db.GetWatchListEntry(r.Context(), database.GetWatchListEntryParams{
		UserID:  user.ID,
		AnimeID: animeID,
	})
	if watchListErr == nil && watchListEntry.Status == "completed" {
		if err := h.svc.db.DeleteContinueWatchingEntry(r.Context(), database.DeleteContinueWatchingEntryParams{
			UserID:  user.ID,
			AnimeID: animeID,
		}); err != nil {
			log.Printf("save progress failed to clear continue entry user_id=%s mal_id=%d err=%v", user.ID, payload.MalID, err)
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if err := h.svc.db.SaveWatchProgress(r.Context(), database.SaveWatchProgressParams{
		CurrentEpisode:     sql.NullInt64{Int64: int64(payload.Episode), Valid: true},
		CurrentTimeSeconds: timeSeconds,
		UserID:             user.ID,
		AnimeID:            animeID,
	}); err != nil {
		if err.Error() != "sql: no rows in result set" {
			log.Printf("save watchlist progress skipped user_id=%s mal_id=%d err=%v", user.ID, payload.MalID, err)
		}
	}

	if _, err := h.svc.db.UpsertContinueWatchingEntry(r.Context(), database.UpsertContinueWatchingEntryParams{
		ID:                 uuid.New().String(),
		UserID:             user.ID,
		AnimeID:            animeID,
		CurrentEpisode:     sql.NullInt64{Int64: int64(payload.Episode), Valid: true},
		CurrentTimeSeconds: timeSeconds,
	}); err != nil {
		log.Printf("save continue watching failed user_id=%s mal_id=%d err=%v", user.ID, payload.MalID, err)
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

	if _, err := h.svc.db.GetAnime(r.Context(), animeID); err != nil {
		anime, fetchErr := h.jikanClient.GetAnimeByID(r.Context(), payload.MalID)
		if fetchErr != nil {
			log.Printf("complete anime failed to fetch anime user_id=%s mal_id=%d err=%v", user.ID, payload.MalID, fetchErr)
			http.Error(w, "failed to mark anime completed", http.StatusInternalServerError)
			return
		}

		if _, upsertErr := h.svc.db.UpsertAnime(r.Context(), database.UpsertAnimeParams{
			ID:            animeID,
			TitleOriginal: anime.Title,
			TitleEnglish:  sql.NullString{String: anime.TitleEnglish, Valid: anime.TitleEnglish != ""},
			TitleJapanese: sql.NullString{String: anime.TitleJapanese, Valid: anime.TitleJapanese != ""},
			ImageUrl:      anime.ImageURL(),
			Airing:        sql.NullBool{Bool: anime.Airing, Valid: true},
		}); upsertErr != nil {
			log.Printf("complete anime failed to upsert anime user_id=%s mal_id=%d err=%v", user.ID, payload.MalID, upsertErr)
			http.Error(w, "failed to mark anime completed", http.StatusInternalServerError)
			return
		}
	}

	if _, err := h.svc.db.UpsertWatchListEntry(r.Context(), database.UpsertWatchListEntryParams{
		ID:                 uuid.New().String(),
		UserID:             user.ID,
		AnimeID:            animeID,
		Status:             "completed",
		CurrentEpisode:     sql.NullInt64{Int64: int64(payload.Episode), Valid: true},
		CurrentTimeSeconds: 0,
	}); err != nil {
		log.Printf("complete anime failed to upsert watchlist user_id=%s mal_id=%d err=%v", user.ID, payload.MalID, err)
		http.Error(w, "failed to mark anime completed", http.StatusInternalServerError)
		return
	}

	if err := h.svc.db.DeleteContinueWatchingEntry(r.Context(), database.DeleteContinueWatchingEntryParams{
		UserID:  user.ID,
		AnimeID: animeID,
	}); err != nil {
		log.Printf("complete anime failed to delete continue entry user_id=%s mal_id=%d err=%v", user.ID, payload.MalID, err)
		http.Error(w, "failed to mark anime completed", http.StatusInternalServerError)
		return
	}

	if _, err := h.svc.db.GetContinueWatchingEntry(r.Context(), database.GetContinueWatchingEntryParams{
		UserID:  user.ID,
		AnimeID: animeID,
	}); err == nil {
		log.Printf("complete anime failed to clear continue entry user_id=%s mal_id=%d", user.ID, payload.MalID)
		http.Error(w, "failed to mark anime completed", http.StatusInternalServerError)
		return
	} else if !errors.Is(err, sql.ErrNoRows) {
		log.Printf("complete anime failed to verify continue clear user_id=%s mal_id=%d err=%v", user.ID, payload.MalID, err)
		http.Error(w, "failed to mark anime completed", http.StatusInternalServerError)
		return
	}

	if err := h.svc.db.SaveWatchProgress(r.Context(), database.SaveWatchProgressParams{
		CurrentEpisode:     sql.NullInt64{Int64: int64(payload.Episode), Valid: true},
		CurrentTimeSeconds: 0,
		UserID:             user.ID,
		AnimeID:            animeID,
	}); err != nil {
		if err.Error() != "sql: no rows in result set" {
			log.Printf("complete anime failed to reset watchlist progress user_id=%s mal_id=%d err=%v", user.ID, payload.MalID, err)
		}
	}

	w.WriteHeader(http.StatusNoContent)
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
