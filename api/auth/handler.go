package auth

import (
	"log"
	"net/http"

	"mal/web/templates"
)

type Handler struct {
	authService *Service
}

const rateLimitFormError = "Too many attempts in a short time. Please wait a minute and try again."

func NewHandler(authService *Service) *Handler {
	return &Handler{authService: authService}
}

func rateLimitErrorFromQuery(r *http.Request) string {
	if r.URL.Query().Get("error") == "rate_limited" {
		return rateLimitFormError
	}

	return ""
}

func (h *Handler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		if renderErr := templates.Login("Something went wrong. Please try again.", "").Render(r.Context(), w); renderErr != nil {
			log.Printf("render error: %v", renderErr)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	if username == "" || password == "" {
		if renderErr := templates.Login("The email or password is wrong.", username).Render(r.Context(), w); renderErr != nil {
			log.Printf("render error: %v", renderErr)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		return
	}

	session, err := h.authService.Login(r.Context(), username, password)
	if err != nil {
		if renderErr := templates.Login("The email or password is wrong.", username).Render(r.Context(), w); renderErr != nil {
			log.Printf("render error: %v", renderErr)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		return
	}

	SetSessionCookie(w, session.ID, session.ExpiresAt)

	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", "/")
		return
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

func (h *Handler) HandleLoginPage(w http.ResponseWriter, r *http.Request) {
	if err := templates.Login(rateLimitErrorFromQuery(r), "").Render(r.Context(), w); err != nil {
		log.Printf("render error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}
