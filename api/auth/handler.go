package auth

import (
	"log"
	"net/http"

	"mal/templates"
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

func (h *Handler) HandleLoginPage(w http.ResponseWriter, r *http.Request) {
	if err := templates.GetRenderer().ExecuteTemplate(w, "login.gohtml", map[string]any{
		"CurrentPath": r.URL.Path,
	}); err != nil {
		log.Printf("render error: %v", err)
	}
}

func (h *Handler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		templates.GetRenderer().ExecuteTemplate(w, "login.gohtml", map[string]any{
			"Error":       "Something went wrong. Please try again.",
			"Username":    "",
			"CurrentPath": r.URL.Path,
		})
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	if username == "" || password == "" {
		templates.GetRenderer().ExecuteTemplate(w, "login.gohtml", map[string]any{
			"Error":       "The email or password is wrong.",
			"Username":    username,
			"CurrentPath": r.URL.Path,
		})
		return
	}

	session, err := h.authService.Login(r.Context(), username, password)
	if err != nil {
		templates.GetRenderer().ExecuteTemplate(w, "login.gohtml", map[string]any{
			"Error":       "The email or password is wrong.",
			"Username":    username,
			"CurrentPath": r.URL.Path,
		})
		return
	}

	SetSessionCookie(w, session.ID, session.ExpiresAt)

	http.Redirect(w, r, "/", http.StatusSeeOther)
}
