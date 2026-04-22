package middleware

import (
	"context"
	"net/http"
	"strings"

	"mal/api/auth"
	"mal/internal/db"
	webcontext "mal/web/context"
)

func Auth(authService *auth.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("session_id")
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			user, err := authService.ValidateSession(r.Context(), cookie.Value)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			ctx := context.WithValue(r.Context(), webcontext.UserKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := r.Context().Value(webcontext.UserKey).(*database.User)
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
	user, ok := ctx.Value(webcontext.UserKey).(*database.User)
	if !ok {
		return nil
	}
	return user
}