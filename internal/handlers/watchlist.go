package handlers

import (
	"fmt"
	"net/http"
	"strconv"

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
	animeImage := r.FormValue("anime_image")
	status := r.FormValue("status")

	animeID, err := strconv.ParseInt(animeIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid anime ID", http.StatusBadRequest)
		return
	}

	// Ensure the anime exists in our local DB first (foreign key constraint)
	_, err = h.db.UpsertAnime(r.Context(), database.UpsertAnimeParams{
		ID:       animeID,
		Title:    animeTitle,
		ImageUrl: animeImage,
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

	// For HTMX, we can just return a success toast or update a portion of the UI
	displayStatus := status
	switch status {
	case "on_hold":
		displayStatus = "on hold"
	case "plan_to_watch":
		displayStatus = "plan to watch"
	}

	w.Header().Set("HX-Trigger", fmt.Sprintf(`{"toast": "added to %s"}`, displayStatus))
	w.WriteHeader(http.StatusNoContent)
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

	animeIDStr := r.URL.Path[len("/api/watchlist/"):]
	animeID, err := strconv.ParseInt(animeIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid anime ID", http.StatusBadRequest)
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

	w.Header().Set("HX-Trigger", `{"toast": "removed from watchlist"}`)
	w.WriteHeader(http.StatusOK)
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
