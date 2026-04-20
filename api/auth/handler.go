package auth

import (
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
		templates.Login("Something went wrong. Please try again.", "").Render(r.Context(), w)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	if username == "" || password == "" {
		templates.Login("The email or password is wrong.", username).Render(r.Context(), w)
		return
	}

	session, err := h.authService.Login(r.Context(), username, password)
	if err != nil {
		templates.Login("The email or password is wrong.", username).Render(r.Context(), w)
		return
	}

	SetSessionCookie(w, session.ID, session.ExpiresAt)

	// HTMX-friendly redirect to root or previous page
	w.Header().Set("HX-Redirect", "/")
	http.Redirect(w, r, "/", http.StatusFound)
}

func (h *Handler) HandleLoginPage(w http.ResponseWriter, r *http.Request) {
	templates.Login(rateLimitErrorFromQuery(r), "").Render(r.Context(), w)
}
