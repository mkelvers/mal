package middleware

import (
	"context"
	"net/http"
	"strings"

	"mal/internal/database"
	"mal/internal/features/auth"
)

type contextKey string

const (
	UserContextKey contextKey = "user"
)

func Auth(authService *auth.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("session_id")
			if err != nil {
				// No session cookie, user is unauthenticated. Proceed, but not logged in.
				next.ServeHTTP(w, r)
				return
			}

			user, err := authService.ValidateSession(r.Context(), cookie.Value)
			if err != nil {
				// Invalid session, proceed as unauthenticated
				// Might also want to clear the invalid cookie here
				auth.ClearSessionCookie(w)
				next.ServeHTTP(w, r)
				return
			}

			// Valid session, bind user to context
			ctx := context.WithValue(r.Context(), UserContextKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := r.Context().Value(UserContextKey).(*database.User)
		if !ok || user == nil {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				w.Header().Set("HX-Redirect", "/login")
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
			} else {
				http.Redirect(w, r, "/login", http.StatusFound)
			}
			return
		}
		next.ServeHTTP(w, r)
	})
}

func RequireGlobalAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Allow unauthenticated access to login, register, search, and static files
		if r.URL.Path == "/login" || r.URL.Path == "/register" || strings.HasPrefix(r.URL.Path, "/static/") ||
			r.URL.Path == "/search" || r.URL.Path == "/api/search" || r.URL.Path == "/api/search-quick" ||
			r.URL.Path == "/" {
			next.ServeHTTP(w, r)
			return
		}

		user, ok := r.Context().Value(UserContextKey).(*database.User)
		if !ok || user == nil {
			if strings.HasPrefix(r.URL.Path, "/api/") || r.Header.Get("HX-Request") == "true" {
				w.Header().Set("HX-Redirect", "/login")
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
			} else {
				http.Redirect(w, r, "/login", http.StatusFound)
			}
			return
		}
		next.ServeHTTP(w, r)
	})
}

func GetUser(ctx context.Context) *database.User {
	user, ok := ctx.Value(UserContextKey).(*database.User)
	if !ok {
		return nil
	}
	return user
}
