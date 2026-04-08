package auth

import (
	"net/http"

	"mal/internal/templates"
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

func (h *Handler) HandleRegister(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	if username == "" || password == "" {
		http.Error(w, "username and password are required", http.StatusBadRequest)
		return
	}

	_, err := h.authService.RegisterUser(r.Context(), username, password)
	if err != nil {
		if err == ErrInvalidPassword {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err == ErrUserExists {
			http.Error(w, "username already taken", http.StatusConflict)
			return
		}
		http.Error(w, "registration failed", http.StatusInternalServerError)
		return
	}

	// Auto-login after successful registration
	session, err := h.authService.Login(r.Context(), username, password)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	SetSessionCookie(w, session.ID, session.ExpiresAt)

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

func (h *Handler) HandleRegisterPage(w http.ResponseWriter, r *http.Request) {
	templates.Register().Render(r.Context(), w)
}
