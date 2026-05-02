package watchlist

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	ctxpkg "mal/internal/context"
	database "mal/internal/db"
	"mal/templates"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) HandleCardWatchlist(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

func (h *Handler) HandleUpdateWatchlist(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, _ := r.Context().Value(ctxpkg.UserKey).(*database.User)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var body struct {
		AnimeID int64  `json:"animeId"`
		Status  string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if body.Status == "" {
		body.Status = "plan_to_watch"
	}

	if err := h.service.AddToWatchlist(r.Context(), user.ID, body.AnimeID, body.Status); err != nil {
		log.Printf("failed to add to watchlist: %v", err)
		http.Error(w, "failed to add to watchlist", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) HandleDeleteWatchlist(w http.ResponseWriter, r *http.Request) {
	user, _ := r.Context().Value(ctxpkg.UserKey).(*database.User)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	animeIDStr := r.URL.Path[len("/api/watchlist/"):]
	animeID, err := strconv.ParseInt(animeIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid anime id", http.StatusBadRequest)
		return
	}

	if _, err := h.service.RemoveEntry(r.Context(), user.ID, animeID); err != nil {
		log.Printf("failed to remove from watchlist: %v", err)
		http.Error(w, "failed to remove from watchlist", http.StatusInternalServerError)
		return
	}

	w.Header().Set("HX-Redirect", "/watchlist")
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) HandleDeleteContinueWatching(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

func (h *Handler) HandleGetWatchlist(w http.ResponseWriter, r *http.Request) {
	user, _ := r.Context().Value(ctxpkg.UserKey).(*database.User)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	entries, err := h.service.GetUserWatchlist(r.Context(), user.ID)
	if err != nil {
		log.Printf("failed to fetch watchlist: %v", err)
		if err := templates.GetRenderer().ExecuteTemplate(w, "not_found.gohtml", map[string]any{
			"CurrentPath": r.URL.Path,
		}); err != nil {
			log.Printf("render error: %v", err)
		}
		return
	}

	watchlistByStatus := make(map[string][]database.GetUserWatchListRow)
	allEntries := make([]database.GetUserWatchListRow, 0)

	for _, entry := range entries {
		status := entry.Status
		if status == "" {
			status = "plan_to_watch"
		}
		watchlistByStatus[status] = append(watchlistByStatus[status], entry)
		allEntries = append(allEntries, entry)
	}

	data := map[string]any{
		"CurrentPath":       r.URL.Path,
		"WatchlistByStatus": watchlistByStatus,
		"AllEntries":        allEntries,
		"StatusOrder":       []string{"watching", "plan_to_watch", "on_hold", "completed", "dropped"},
		"StatusLabels": map[string]string{
			"watching":      "Currently Watching",
			"plan_to_watch": "Plan to Watch",
			"on_hold":       "On Hold",
			"completed":     "Completed",
			"dropped":       "Dropped",
		},
	}

	templateName := "watchlist.gohtml"
	if r.Header.Get("HX-Request") == "true" {
		templateName = "watchlist_partial.gohtml"
	}

	if err := templates.GetRenderer().ExecuteTemplate(w, templateName, data); err != nil {
		log.Printf("render error: %v", err)
	}
}
