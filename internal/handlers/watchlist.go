package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"malago/internal/database"
	"malago/internal/middleware"
	"malago/internal/templates"
)

type WatchlistHandler struct {
	db database.Querier
}

func NewWatchlistHandler(db database.Querier) *WatchlistHandler {
	return &WatchlistHandler{db: db}
}

func (h *WatchlistHandler) HandleUpdateWatchlist(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, ok := r.Context().Value(middleware.UserContextKey).(*database.User)
	if !ok || user == nil {
		w.Header().Set("HX-Redirect", "/login")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}

	animeIDStr := r.FormValue("anime_id")
	animeTitle := r.FormValue("anime_title")
	animeTitleEnglish := r.FormValue("anime_title_english")
	animeTitleJapanese := r.FormValue("anime_title_japanese")
	animeImage := r.FormValue("anime_image")
	status := r.FormValue("status")

	log.Printf("watchlist add: id=%s, title=%s, title_en=%s, title_jp=%s", animeIDStr, animeTitle, animeTitleEnglish, animeTitleJapanese)

	animeID, err := strconv.ParseInt(animeIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid anime ID", http.StatusBadRequest)
		return
	}

	// Ensure the anime exists in our local DB first (foreign key constraint)
	_, err = h.db.UpsertAnime(r.Context(), database.UpsertAnimeParams{
		ID:            animeID,
		TitleOriginal: animeTitle,
		TitleEnglish:  sql.NullString{String: animeTitleEnglish, Valid: animeTitleEnglish != ""},
		TitleJapanese: sql.NullString{String: animeTitleJapanese, Valid: animeTitleJapanese != ""},
		ImageUrl:      animeImage,
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to save anime reference: %v", err), http.StatusInternalServerError)
		return
	}

	// Now insert/update the watchlist entry
	entryID := uuid.New().String()
	_, err = h.db.UpsertWatchListEntry(r.Context(), database.UpsertWatchListEntryParams{
		ID:      entryID,
		UserID:  user.ID,
		AnimeID: animeID,
		Status:  status,
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to update watchlist: %v", err), http.StatusInternalServerError)
		return
	}

	templates.WatchlistDropdown(int(animeID), animeTitle, animeTitleEnglish, animeTitleJapanese, animeImage, status).Render(r.Context(), w)
}

func (h *WatchlistHandler) HandleDeleteWatchlist(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, ok := r.Context().Value(middleware.UserContextKey).(*database.User)
	if !ok || user == nil {
		w.Header().Set("HX-Redirect", "/login")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Parse the path to get anime ID (path is /api/watchlist/{id} possibly with query params)
	path := r.URL.Path[len("/api/watchlist/"):]
	animeID, err := strconv.ParseInt(path, 10, 64)
	if err != nil {
		http.Error(w, "invalid anime ID", http.StatusBadRequest)
		return
	}

	// Get anime info before deleting (for dropdown refresh on anime page)
	anime, err := h.db.GetAnime(r.Context(), animeID)
	if err != nil {
		http.Error(w, "anime not found", http.StatusNotFound)
		return
	}

	err = h.db.DeleteWatchListEntry(r.Context(), database.DeleteWatchListEntryParams{
		UserID:  user.ID,
		AnimeID: animeID,
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to delete from watchlist: %v", err), http.StatusInternalServerError)
		return
	}

	// If called from watchlist page, just return empty (hx-swap="delete" handles removal)
	if r.URL.Query().Get("from") == "watchlist" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Extract nullable strings
	titleEnglish := ""
	if anime.TitleEnglish.Valid {
		titleEnglish = anime.TitleEnglish.String
	}
	titleJapanese := ""
	if anime.TitleJapanese.Valid {
		titleJapanese = anime.TitleJapanese.String
	}

	// Otherwise return updated dropdown for anime page
	templates.WatchlistDropdown(int(animeID), anime.TitleOriginal, titleEnglish, titleJapanese, anime.ImageUrl, "").Render(r.Context(), w)
}

func (h *WatchlistHandler) HandleGetWatchlist(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	layout := r.URL.Query().Get("view")
	if layout != "grid" && layout != "table" {
		layout = "table"
	}

	statusFilter := r.URL.Query().Get("status")

	user, ok := r.Context().Value(middleware.UserContextKey).(*database.User)
	if !ok || user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	entries, err := h.db.GetUserWatchList(r.Context(), user.ID)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to fetch watchlist: %v", err), http.StatusInternalServerError)
		return
	}

	var filteredEntries []database.GetUserWatchListRow
	if statusFilter != "" && statusFilter != "all" {
		for _, entry := range entries {
			if entry.Status == statusFilter {
				filteredEntries = append(filteredEntries, entry)
			}
		}
	} else {
		statusFilter = "all"
		filteredEntries = entries
	}

	templates.Watchlist(filteredEntries, layout, statusFilter).Render(r.Context(), w)
}

// WatchlistExportEntry represents a single entry in the export format
type WatchlistExportEntry struct {
	AnimeID   int64  `json:"anime_id"`
	Title     string `json:"title"`
	ImageURL  string `json:"image_url"`
	Status    string `json:"status"`
	UpdatedAt string `json:"updated_at"`
}

// WatchlistExport is the full export format
type WatchlistExport struct {
	ExportedAt string                 `json:"exported_at"`
	Entries    []WatchlistExportEntry `json:"entries"`
}

func (h *WatchlistHandler) HandleExportWatchlist(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, ok := r.Context().Value(middleware.UserContextKey).(*database.User)
	if !ok || user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	entries, err := h.db.GetUserWatchList(r.Context(), user.ID)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to fetch watchlist: %v", err), http.StatusInternalServerError)
		return
	}

	export := WatchlistExport{
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		Entries:    make([]WatchlistExportEntry, len(entries)),
	}

	for i, entry := range entries {
		export.Entries[i] = WatchlistExportEntry{
			AnimeID:   entry.AnimeID,
			Title:     entry.DisplayTitle(),
			ImageURL:  entry.ImageUrl,
			Status:    entry.Status,
			UpdatedAt: entry.UpdatedAt.Format(time.RFC3339),
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=malago-watchlist.json")
	json.NewEncoder(w).Encode(export)
}

func (h *WatchlistHandler) HandleImportWatchlist(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, ok := r.Context().Value(middleware.UserContextKey).(*database.User)
	if !ok || user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Parse multipart form (max 10MB)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "failed to parse form", http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "no file uploaded", http.StatusBadRequest)
		return
	}
	defer file.Close()

	var export WatchlistExport
	if err := json.NewDecoder(file).Decode(&export); err != nil {
		http.Error(w, "invalid JSON format", http.StatusBadRequest)
		return
	}

	imported := 0
	for _, entry := range export.Entries {
		// Upsert anime - store title as original (we don't know which type it is from export)
		_, err := h.db.UpsertAnime(r.Context(), database.UpsertAnimeParams{
			ID:            entry.AnimeID,
			TitleOriginal: entry.Title,
			TitleEnglish:  sql.NullString{},
			TitleJapanese: sql.NullString{},
			ImageUrl:      entry.ImageURL,
		})
		if err != nil {
			continue
		}

		// Upsert watchlist entry
		_, err = h.db.UpsertWatchListEntry(r.Context(), database.UpsertWatchListEntryParams{
			ID:      uuid.New().String(),
			UserID:  user.ID,
			AnimeID: entry.AnimeID,
			Status:  entry.Status,
		})
		if err != nil {
			continue
		}
		imported++
	}

	w.Header().Set("HX-Redirect", "/watchlist")
	w.WriteHeader(http.StatusOK)
}
