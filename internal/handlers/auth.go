package handlers

import (
	"net/http"

	"malago/internal/auth"
	"malago/internal/templates"
)

type AuthHandler struct {
	authService *auth.Service
}

func NewAuthHandler(authService *auth.Service) *AuthHandler {
	return &AuthHandler{authService: authService}
}

// Render the login/register pages here (assuming you have these templates)

func (h *AuthHandler) HandleLogin(w http.ResponseWriter, r *http.Request) {
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

	auth.SetSessionCookie(w, session.ID, session.ExpiresAt)

	// HTMX-friendly redirect to root or previous page
	w.Header().Set("HX-Redirect", "/")
	http.Redirect(w, r, "/", http.StatusFound)
}

func (h *AuthHandler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session_id")
	if err == nil {
		_ = h.authService.Logout(r.Context(), cookie.Value)
	}

	auth.ClearSessionCookie(w)
	w.Header().Set("HX-Redirect", "/")
	http.Redirect(w, r, "/", http.StatusFound)
}

func (h *AuthHandler) HandleLoginPage(w http.ResponseWriter, r *http.Request) {
	templates.Login().Render(r.Context(), w)
}
