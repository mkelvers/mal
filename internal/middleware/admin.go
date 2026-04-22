package middleware

import (
	"net/http"

	"mal/internal/db"
	webcontext "mal/web/context"
	"mal/web/shared/admin"
)

func IsAdmin(user *database.User) bool {
	return admin.IsAdmin(user)
}

func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := r.Context().Value(webcontext.UserKey).(*database.User)
		if !ok || user == nil {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		if !admin.IsAdmin(user) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}
