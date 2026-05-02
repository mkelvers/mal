package middleware

import (
	"context"
	"net/http"
	"strings"

	"mal/api/auth"
	ctxpkg "mal/internal/context"
	"mal/internal/db"
)

var authSvc *auth.Service

func InitAuth(service *auth.Service) {
	authSvc = service
}

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

			ctx := context.WithValue(r.Context(), ctxpkg.UserKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session_id")
		if err != nil {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				w.Header().Set("HX-Redirect", "/login")
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
			} else {
				http.Redirect(w, r, "/login", http.StatusFound)
			}
			return
		}

		if authSvc != nil {
			user, err := authSvc.ValidateSession(r.Context(), cookie.Value)
			if err == nil {
				ctx := context.WithValue(r.Context(), ctxpkg.UserKey, user)
				r = r.WithContext(ctx)
			}
		}

		user := GetUser(r.Context())
		if user == nil {
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
	user, ok := ctx.Value(ctxpkg.UserKey).(*database.User)
	if !ok {
		return nil
	}
	return user
}
