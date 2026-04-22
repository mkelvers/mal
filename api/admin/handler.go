package admin

import (
	"database/sql"
	"html"
	"log"
	"net/http"

	"golang.org/x/crypto/bcrypt"

	"mal/api/auth"
	"mal/internal/db"
	"mal/internal/middleware"
	webcontext "mal/web/context"
	"mal/web/templates"
)

type Handler struct {
	db          database.Querier
	authService *auth.Service
}

func NewHandler(db database.Querier, authService *auth.Service) *Handler {
	return &Handler{db: db, authService: authService}
}

func (h *Handler) HandleAdminPage(w http.ResponseWriter, r *http.Request) {
	users, err := h.db.ListUsers(r.Context())
	if err != nil {
		log.Printf("list users error: %v", err)
		http.Error(w, "Failed to load users", http.StatusInternalServerError)
		return
	}

	if err := templates.AdminPage(users).Render(r.Context(), w); err != nil {
		log.Printf("render error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (h *Handler) HandleImpersonateUser(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Path[len("/admin/users/"):]
	if userID == "" {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	targetUser, err := h.db.GetUser(r.Context(), userID)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}
		log.Printf("get user error: %v", err)
		http.Error(w, "Failed to load user", http.StatusInternalServerError)
		return
	}

	if err := templates.AdminImpersonatePage(targetUser).Render(r.Context(), w); err != nil {
		log.Printf("render error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (h *Handler) HandleUserWatchlist(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Path[len("/admin/users/"):]
	userID = userID[:len(userID)-len("/watchlist")]

	entries, err := h.db.GetUserWatchList(r.Context(), userID)
	if err != nil {
		log.Printf("get user watchlist error: %v", err)
		http.Error(w, "Failed to load watchlist", http.StatusInternalServerError)
		return
	}

	if err := templates.AdminUserWatchlist(entries).Render(r.Context(), w); err != nil {
		log.Printf("render error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (h *Handler) HandleUserContinueWatching(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Path[len("/admin/users/"):]
	userID = userID[:len(userID)-len("/continue-watching")]

	entries, err := h.db.GetContinueWatchingEntries(r.Context(), userID)
	if err != nil {
		log.Printf("get continue watching error: %v", err)
		http.Error(w, "Failed to load continue watching", http.StatusInternalServerError)
		return
	}

	if err := templates.AdminUserContinueWatching(entries).Render(r.Context(), w); err != nil {
		log.Printf("render error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (h *Handler) HandleAddUserForm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	if username == "" || password == "" {
		writeInlineError(w, "Username and password are required")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		log.Printf("bcrypt error: %v", err)
		writeInlineError(w, "Failed to create user")
		return
	}

	_, err = h.db.CreateUser(r.Context(), database.CreateUserParams{
		ID:           generateUserID(),
		Username:     username,
		PasswordHash: string(hash),
	})
	if err != nil {
		log.Printf("create user error: %v", err)
		writeInlineError(w, "Failed to create user (may already exist)")
		return
	}

	// Return success - reload the users list
	users, err := h.db.ListUsers(r.Context())
	if err != nil {
		log.Printf("list users error: %v", err)
		writeInlineError(w, "User created but failed to refresh list")
		return
	}

	if err := templates.AdminUsersList(users).Render(r.Context(), w); err != nil {
		log.Printf("render error: %v", err)
		writeInlineError(w, "User created but failed to render list")
	}
}

func (h *Handler) HandleDeleteUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID := r.URL.Path[len("/admin/users/"):]
	if userID == "" {
		writeInlineError(w, "Invalid user ID")
		return
	}

	// Don't allow deleting yourself
	currentUser, ok := r.Context().Value(webcontext.UserKey).(*database.User)
	if !ok || currentUser == nil {
		writeInlineError(w, "Not authenticated")
		return
	}
	if userID == currentUser.ID {
		writeInlineError(w, "Cannot delete your own account")
		return
	}

	err := h.db.DeleteUser(r.Context(), userID)
	if err != nil {
		log.Printf("delete user error: %v", err)
		writeInlineError(w, "Failed to delete user")
		return
	}

	users, err := h.db.ListUsers(r.Context())
	if err != nil {
		log.Printf("list users error: %v", err)
		writeInlineError(w, "User deleted but failed to refresh list")
		return
	}

	if err := templates.AdminUsersList(users).Render(r.Context(), w); err != nil {
		log.Printf("render error: %v", err)
		writeInlineError(w, "User deleted but failed to render list")
	}
}

func writeInlineError(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusBadRequest)
	_, _ = w.Write([]byte(`<p style="color: var(--danger); font-size: var(--text-sm);">` + html.EscapeString(message) + `</p>`))
}

func generateUserID() string {
	// Simple UUID-like generation - in production use proper UUID
	b := make([]byte, 16)
	for i := range b {
		b[i] = byte('a' + (i % 26))
	}
	return string(b)
}

const bcryptCost = 12

func GetImpersonatedUserID(r *http.Request) string {
	// Check for impersonation parameter
	impersonateID := r.URL.Query().Get("as_user")
	if impersonateID == "" {
		return ""
	}

	// Verify the current user is admin
	user, ok := r.Context().Value(webcontext.UserKey).(*database.User)
	if !ok || user == nil || !middleware.IsAdmin(user) {
		return ""
	}

	return impersonateID
}
