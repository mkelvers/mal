package watchlist

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"slices"
	"strconv"

	"mal/internal/database"
	"mal/internal/shared/middleware"
	"mal/internal/templates"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method == method {
		return true
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	return false
}

func (h *Handler) HandleUpdateWatchlist(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	user := middleware.GetUser(r.Context())
	if user == nil {
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

	log.Printf("watchlist add: user_id=%s, anime_id=%s, title=%s", user.ID, animeIDStr, animeTitle)

	animeID, err := strconv.ParseInt(animeIDStr, 10, 64)
	if err != nil || animeID <= 0 {
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
		if errors.Is(err, ErrInvalidAnimeID) || errors.Is(err, ErrInvalidStatus) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		log.Printf("watchlist add failed: user_id=%s anime_id=%d err=%v", user.ID, animeID, err)
		http.Error(w, "failed to update watchlist", http.StatusInternalServerError)
		return
	}

	templates.WatchlistDropdown(int(animeID), animeTitle, animeTitleEnglish, animeTitleJapanese, animeImage, status, airing).Render(r.Context(), w)
}

func (h *Handler) HandleDeleteWatchlist(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodDelete) {
		return
	}

	user := middleware.GetUser(r.Context())
	if user == nil {
		w.Header().Set("HX-Redirect", "/login")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	path := r.URL.Path[len("/api/watchlist/"):]
	animeID, err := strconv.ParseInt(path, 10, 64)
	if err != nil || animeID <= 0 {
		http.Error(w, "invalid anime ID", http.StatusBadRequest)
		return
	}

	anime, err := h.svc.RemoveEntry(r.Context(), user.ID, animeID)
	if err != nil {
		if errors.Is(err, ErrInvalidAnimeID) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		log.Printf("watchlist delete failed: user_id=%s anime_id=%d err=%v", user.ID, animeID, err)
		http.Error(w, "failed to delete from watchlist", http.StatusInternalServerError)
		return
	}

	if r.URL.Query().Get("from") == "watchlist" {
		w.WriteHeader(http.StatusOK)
		return
	}

	title := database.DisplayTitle(anime.TitleEnglish, anime.TitleJapanese, anime.TitleOriginal)
	airing := false
	if anime.Airing.Valid {
		airing = anime.Airing.Bool
	}

	templates.WatchlistDropdown(int(animeID), anime.TitleOriginal, title, "", anime.ImageUrl, "", airing).Render(r.Context(), w)
}

func (h *Handler) HandleGetWatchlist(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	layout := r.URL.Query().Get("view")
	if layout != "grid" && layout != "table" {
		layout = "grid"
	}

	statusFilter := r.URL.Query().Get("status")
	sortBy := r.URL.Query().Get("sort")
	sortOrder := r.URL.Query().Get("order")

	if sortBy != "title" {
		sortBy = "date"
	}
	if sortOrder != "asc" {
		sortOrder = "desc"
	}

	user := middleware.GetUser(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	entries, err := h.svc.GetUserWatchlist(r.Context(), user.ID)
	if err != nil {
		log.Printf("watchlist fetch failed: user_id=%s err=%v", user.ID, err)
		http.Error(w, "failed to fetch watchlist", http.StatusInternalServerError)
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

	// Sort entries
	h.sortEntries(filteredEntries, sortBy, sortOrder)

	templates.Watchlist(filteredEntries, layout, statusFilter, sortBy, sortOrder).Render(r.Context(), w)
}

func (h *Handler) HandleContinueWatching(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	user := middleware.GetUser(r.Context())
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	entries, err := h.svc.GetContinueWatching(r.Context(), user.ID)
	if err != nil {
		log.Printf("continue watching fetch failed: user_id=%s err=%v", user.ID, err)
		http.Error(w, "failed to fetch continue watching", http.StatusInternalServerError)
		return
	}

	templates.ContinueWatching(entries).Render(r.Context(), w)
}

func (h *Handler) HandleDeleteContinueWatching(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodDelete) {
		return
	}

	user := middleware.GetUser(r.Context())
	if user == nil {
		w.Header().Set("HX-Redirect", "/login")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	path := r.URL.Path[len("/api/continue-watching/"):]
	animeID, err := strconv.ParseInt(path, 10, 64)
	if err != nil || animeID <= 0 {
		http.Error(w, "invalid anime ID", http.StatusBadRequest)
		return
	}

	if err := h.svc.DeleteContinueWatching(r.Context(), user.ID, animeID); err != nil {
		log.Printf("continue watching delete failed: user_id=%s anime_id=%d err=%v", user.ID, animeID, err)
		http.Error(w, "failed to delete continue watching entry", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) HandleExportWatchlist(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	user := middleware.GetUser(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	export, err := h.svc.Export(r.Context(), user.ID)
	if err != nil {
		log.Printf("watchlist export failed: user_id=%s err=%v", user.ID, err)
		http.Error(w, "failed to export", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=mal-watchlist.json")
	json.NewEncoder(w).Encode(export)
}

func (h *Handler) HandleImportWatchlist(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	user := middleware.GetUser(r.Context())
	if user == nil {
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
	isAsc := sortOrder == "asc"

	switch sortBy {
	case "title":
		slices.SortFunc(entries, func(a, b database.GetUserWatchListRow) int {
			if a.TitleOriginal < b.TitleOriginal {
				return -1
			}
			if a.TitleOriginal > b.TitleOriginal {
				return 1
			}
			return 0
		})
		if !isAsc {
			slices.Reverse(entries)
		}
	case "date":
		slices.SortFunc(entries, func(a, b database.GetUserWatchListRow) int {
			if a.UpdatedAt.After(b.UpdatedAt) {
				return -1
			}
			if a.UpdatedAt.Before(b.UpdatedAt) {
				return 1
			}
			return 0
		})
		if !isAsc {
			slices.Reverse(entries)
		}
	}
}
