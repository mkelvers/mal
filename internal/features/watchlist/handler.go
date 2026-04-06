package watchlist

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"malago/internal/database"
	"malago/internal/shared/middleware"
	"malago/internal/templates"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) HandleUpdateWatchlist(w http.ResponseWriter, r *http.Request) {
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
	airingStr := r.FormValue("airing")
	airing := airingStr == "true"

	log.Printf("watchlist add: id=%s, title=%s", animeIDStr, animeTitle)

	animeID, err := strconv.ParseInt(animeIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid anime ID", http.StatusBadRequest)
		return
	}

	req := AddRequest{
		AnimeID:       animeID,
		TitleOriginal: animeTitle,
		TitleEnglish:  animeTitleEnglish,
		TitleJapanese: animeTitleJapanese,
		ImageURL:      animeImage,
		Status:        status,
		Airing:        airing,
	}

	if err := h.svc.AddEntry(r.Context(), user.ID, req); err != nil {
		http.Error(w, fmt.Sprintf("failed to update watchlist: %v", err), http.StatusInternalServerError)
		return
	}

	templates.WatchlistDropdown(int(animeID), animeTitle, animeTitleEnglish, animeTitleJapanese, animeImage, status, airing).Render(r.Context(), w)
}

func (h *Handler) HandleDeleteWatchlist(w http.ResponseWriter, r *http.Request) {
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

	path := r.URL.Path[len("/api/watchlist/"):]
	animeID, err := strconv.ParseInt(path, 10, 64)
	if err != nil {
		http.Error(w, "invalid anime ID", http.StatusBadRequest)
		return
	}

	anime, err := h.svc.RemoveEntry(r.Context(), user.ID, animeID)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to delete from watchlist: %v", err), http.StatusInternalServerError)
		return
	}

	if r.URL.Query().Get("from") == "watchlist" {
		w.WriteHeader(http.StatusOK)
		return
	}

	titleEnglish := ""
	if anime.TitleEnglish.Valid {
		titleEnglish = anime.TitleEnglish.String
	}
	titleJapanese := ""
	if anime.TitleJapanese.Valid {
		titleJapanese = anime.TitleJapanese.String
	}
	airing := false
	if anime.Airing.Valid {
		airing = anime.Airing.Bool
	}

	templates.WatchlistDropdown(int(animeID), anime.TitleOriginal, titleEnglish, titleJapanese, anime.ImageUrl, "", airing).Render(r.Context(), w)
}

func (h *Handler) HandleGetWatchlist(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	layout := r.URL.Query().Get("view")
	if layout != "grid" && layout != "table" {
		layout = "table"
	}

	statusFilter := r.URL.Query().Get("status")
	sortBy := r.URL.Query().Get("sort")
	sortOrder := r.URL.Query().Get("order")

	if sortBy != "title" && sortBy != "score" {
		sortBy = "date"
	}
	if sortOrder != "asc" {
		sortOrder = "desc"
	}

	user, ok := r.Context().Value(middleware.UserContextKey).(*database.User)
	if !ok || user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	entries, err := h.svc.GetUserWatchlist(r.Context(), user.ID)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to fetch watchlist: %v", err), http.StatusInternalServerError)
		return
	}

	var filteredEntries []database.GetUserWatchListRow
	if statusFilter == "continuing" {
		// Show airing anime with watching or plan_to_watch status
		for _, entry := range entries {
			if entry.Airing.Bool && (entry.Status == "watching" || entry.Status == "plan_to_watch") {
				filteredEntries = append(filteredEntries, entry)
			}
		}
	} else if statusFilter != "" && statusFilter != "all" {
		for _, entry := range entries {
			if entry.Status == statusFilter {
				filteredEntries = append(filteredEntries, entry)
			}
		}
	} else {
		statusFilter = "all"
		filteredEntries = entries
	}

	// Sort entries
	h.sortEntries(filteredEntries, sortBy, sortOrder)

	templates.Watchlist(filteredEntries, layout, statusFilter, sortBy, sortOrder).Render(r.Context(), w)
}

func (h *Handler) HandleExportWatchlist(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, ok := r.Context().Value(middleware.UserContextKey).(*database.User)
	if !ok || user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	export, err := h.svc.Export(r.Context(), user.ID)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to export: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=malago-watchlist.json")
	json.NewEncoder(w).Encode(export)
}

func (h *Handler) HandleImportWatchlist(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, ok := r.Context().Value(middleware.UserContextKey).(*database.User)
	if !ok || user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

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

	var export ExportData
	if err := json.NewDecoder(file).Decode(&export); err != nil {
		http.Error(w, "invalid JSON format", http.StatusBadRequest)
		return
	}

	if _, err := h.svc.Import(r.Context(), user.ID, export); err != nil {
		http.Error(w, "failed to import", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Redirect", "/watchlist")
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) sortEntries(entries []database.GetUserWatchListRow, sortBy, sortOrder string) {
	var less func(int, int) bool

	switch sortBy {
	case "title":
		less = func(i, j int) bool {
			cmp := entries[i].TitleOriginal < entries[j].TitleOriginal
			if sortOrder == "asc" {
				return cmp
			}
			return !cmp
		}
	case "score":
		less = func(i, j int) bool {
			// Score is stored as JSON in the DB, for now just keep default order
			return false
		}
	default: // "date"
		less = func(i, j int) bool {
			cmp := entries[i].UpdatedAt.After(entries[j].UpdatedAt)
			if sortOrder == "asc" {
				return !cmp
			}
			return cmp
		}
	}

	// Simple bubble sort for small lists
	for i := 0; i < len(entries); i++ {
		for j := i + 1; j < len(entries); j++ {
			if less(j, i) {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}
}
