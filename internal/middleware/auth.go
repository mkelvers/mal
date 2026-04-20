package middleware

import (
	"context"
	"net/http"
	"strings"

	"mal/internal/db"
	"mal/api/auth"
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

func GetUser(ctx context.Context) *database.User {
	user, ok := ctx.Value(UserContextKey).(*database.User)
	if !ok {
		return nil
	}
	return user
}
