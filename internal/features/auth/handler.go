package auth

import (
	"errors"
	"net/http"
	"time"

	"mal/internal/database"
	"mal/internal/templates"
)

type Handler struct {
	authService *Service
}

const rateLimitFormError = "Too many attempts in a short time. Please wait a minute and try again."

const (
	accountPasswordChangedMessage       = "Password updated successfully."
	accountRecoveryKeyRotatedMessage    = "Recovery key rotated. Save this new key now."
	accountPasswordErrorMessage         = "Unable to update password with those details."
	accountRecoveryErrorMessage         = "Unable to rotate recovery key with those details."
	accountUnexpectedErrorMessage       = "Something went wrong. Please try again."
	accountMissingFieldsErrorMessage    = "Please complete all required fields."
	accountPasswordMismatchErrorMessage = "New password and confirm password must match."
)

func (h *Handler) accountUserFromRequest(r *http.Request) (*database.User, bool) {
	cookie, err := r.Cookie("session_id")
	if err != nil {
		return nil, false
	}

	user, err := h.authService.ValidateSession(r.Context(), cookie.Value)
	if err != nil {
		return nil, false
	}

	return user, true
}

func accountCreatedAt(createdAt time.Time) string {
	return createdAt.Local().Format("Jan 2, 2006 at 15:04")
}

func renderAccountPage(w http.ResponseWriter, r *http.Request, user *database.User, passwordError string, passwordSuccess string, recoveryError string, recoverySuccess string, recoveryKey string) {
	templates.Account(user.Username, accountCreatedAt(user.CreatedAt), passwordError, passwordSuccess, recoveryError, recoverySuccess, recoveryKey).Render(r.Context(), w)
}

func NewHandler(authService *Service) *Handler {
	return &Handler{authService: authService}
}

// Render the login/register pages here (assuming you have these templates)

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

func (h *Handler) HandleRegister(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		templates.Register("Something went wrong. Please try again.", "").Render(r.Context(), w)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	if username == "" || password == "" {
		templates.Register("Please enter both email and password.", username).Render(r.Context(), w)
		return
	}

	_, recoveryKey, err := h.authService.RegisterUser(r.Context(), username, password)
	if err != nil {
		if errors.Is(err, ErrInvalidPassword) || errors.Is(err, ErrUserExists) {
			templates.Register("Unable to create account with those details.", username).Render(r.Context(), w)
			return
		}
		templates.Register("Something went wrong. Please try again.", username).Render(r.Context(), w)
		return
	}

	// Auto-login after successful registration
	session, err := h.authService.Login(r.Context(), username, password)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	SetSessionCookie(w, session.ID, session.ExpiresAt)
	templates.RegistrationRecoveryKey(recoveryKey).Render(r.Context(), w)
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
	formError := ""
	if r.URL.Query().Get("error") == "rate_limited" {
		formError = rateLimitFormError
	}
	templates.Login(formError, "").Render(r.Context(), w)
}

func (h *Handler) HandleRegisterPage(w http.ResponseWriter, r *http.Request) {
	formError := ""
	if r.URL.Query().Get("error") == "rate_limited" {
		formError = rateLimitFormError
	}
	templates.Register(formError, "").Render(r.Context(), w)
}

func (h *Handler) HandleRecoverPage(w http.ResponseWriter, r *http.Request) {
	formError := ""
	if r.URL.Query().Get("error") == "rate_limited" {
		formError = rateLimitFormError
	}
	templates.Recover(formError, "", "").Render(r.Context(), w)
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

func (h *Handler) HandleAccountPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, ok := h.accountUserFromRequest(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	renderAccountPage(w, r, user, "", "", "", "", "")
}

func (h *Handler) HandleAccountPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, ok := h.accountUserFromRequest(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	if err := r.ParseForm(); err != nil {
		renderAccountPage(w, r, user, accountUnexpectedErrorMessage, "", "", "", "")
		return
	}

	currentPassword := r.FormValue("current_password")
	newPassword := r.FormValue("new_password")
	confirmNewPassword := r.FormValue("confirm_new_password")

	if currentPassword == "" || newPassword == "" || confirmNewPassword == "" {
		renderAccountPage(w, r, user, accountMissingFieldsErrorMessage, "", "", "", "")
		return
	}

	if newPassword != confirmNewPassword {
		renderAccountPage(w, r, user, accountPasswordMismatchErrorMessage, "", "", "", "")
		return
	}

	err := h.authService.ChangePassword(r.Context(), user.ID, currentPassword, newPassword)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) || errors.Is(err, ErrInvalidPassword) {
			renderAccountPage(w, r, user, accountPasswordErrorMessage, "", "", "", "")
			return
		}
		renderAccountPage(w, r, user, accountUnexpectedErrorMessage, "", "", "", "")
		return
	}

	renderAccountPage(w, r, user, "", accountPasswordChangedMessage, "", "", "")
}

func (h *Handler) HandleAccountRecoveryKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, ok := h.accountUserFromRequest(r)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	if err := r.ParseForm(); err != nil {
		renderAccountPage(w, r, user, "", "", accountUnexpectedErrorMessage, "", "")
		return
	}

	password := r.FormValue("password")
	if password == "" {
		renderAccountPage(w, r, user, "", "", accountMissingFieldsErrorMessage, "", "")
		return
	}

	newRecoveryKey, err := h.authService.RegenerateRecoveryKey(r.Context(), user.ID, password)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			renderAccountPage(w, r, user, "", "", accountRecoveryErrorMessage, "", "")
			return
		}
		renderAccountPage(w, r, user, "", "", accountUnexpectedErrorMessage, "", "")
		return
	}

	renderAccountPage(w, r, user, "", "", "", accountRecoveryKeyRotatedMessage, newRecoveryKey)
}
