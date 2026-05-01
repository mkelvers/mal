package watchlist

import (
	"log"
	"net/http"

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
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

func (h *Handler) HandleDeleteWatchlist(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

func (h *Handler) HandleDeleteContinueWatching(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

func (h *Handler) HandleGetWatchlist(w http.ResponseWriter, r *http.Request) {
	if err := templates.GetRenderer().ExecuteTemplate(w, "not_found.gohtml", nil); err != nil {
		log.Printf("render error: %v", err)
	}
}
