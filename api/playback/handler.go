package playback

import (
	"log"
	"net/http"

	"mal/integrations/jikan"
	"mal/templates"
)

type Handler struct {
	svc         *Service
	jikanClient *jikan.Client
}

func NewHandler(svc *Service, jikanClient *jikan.Client) *Handler {
	return &Handler{svc: svc, jikanClient: jikanClient}
}

func (h *Handler) HandleWatchPage(w http.ResponseWriter, r *http.Request) {
	if err := templates.GetRenderer().ExecuteTemplate(w, "not_found.gohtml", nil); err != nil {
		log.Printf("render error: %v", err)
	}
}

func (h *Handler) HandleProxy(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

func (h *Handler) HandleSaveProgress(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

func (h *Handler) HandleCompleteAnime(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

func (h *Handler) HandleEpisodeData(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}
