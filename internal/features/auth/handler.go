package auth

import (
	"net/http"

	"malago/internal/templates"
)

type Handler struct {
	authService *Service
}

func NewHandler(authService *Service) *Handler {
	return &Handler{authService: authService}
}

// Render the login/register pages here (assuming you have these templates)

func (h *Handler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	session, err := h.authService.Login(r.Context(), username, password)
	if err != nil {
		// Just handle generically for now, perhaps via HTMX toast
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	SetSessionCookie(w, session.ID, session.ExpiresAt)

	// HTMX-friendly redirect to root or previous page
	w.Header().Set("HX-Redirect", "/")
	http.Redirect(w, r, "/", http.StatusFound)
}

func (h *Handler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session_id")
	if err == nil {
		_ = h.authService.Logout(r.Context(), cookie.Value)
	}

	ClearSessionCookie(w)
	w.Header().Set("HX-Redirect", "/")
	http.Redirect(w, r, "/", http.StatusFound)
}

func (h *Handler) HandleLoginPage(w http.ResponseWriter, r *http.Request) {
	templates.Login().Render(r.Context(), w)
}
