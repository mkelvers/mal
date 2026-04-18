package auth

import (
	"errors"
	"net/http"

	"mal/internal/templates"
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
	templates.Login(rateLimitErrorFromQuery(r), "").Render(r.Context(), w)
}

func (h *Handler) HandleRecoverPage(w http.ResponseWriter, r *http.Request) {
	templates.Recover(rateLimitErrorFromQuery(r), "", "").Render(r.Context(), w)
}

func (h *Handler) HandleRecover(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		templates.Recover("Something went wrong. Please try again.", "", "").Render(r.Context(), w)
		return
	}

	username := r.FormValue("username")
	recoveryKey := r.FormValue("recovery_key")
	newPassword := r.FormValue("new_password")

	if username == "" || recoveryKey == "" || newPassword == "" {
		templates.Recover("Unable to recover account with those details.", username, recoveryKey).Render(r.Context(), w)
		return
	}

	newRecoveryKey, err := h.authService.RecoverAccount(r.Context(), username, recoveryKey, newPassword)
	if err != nil {
		if errors.Is(err, ErrInvalidRecoveryKey) || errors.Is(err, ErrInvalidPassword) {
			templates.Recover("Unable to recover account with those details.", username, recoveryKey).Render(r.Context(), w)
			return
		}
		templates.Recover("Something went wrong. Please try again.", username, recoveryKey).Render(r.Context(), w)
		return
	}

	templates.RecoveryComplete(newRecoveryKey).Render(r.Context(), w)
}
