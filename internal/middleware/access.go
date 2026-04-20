package middleware

import (
	"net/http"
	"strings"

	"mal/internal/db"
)

type AccessPolicy struct {
	PublicPaths map[string]struct{}
	PublicHeads []string
}

func NewAccessPolicy() AccessPolicy {
	return AccessPolicy{
		PublicPaths: map[string]struct{}{
			"/":                 {},
			"/login":            {},
			"/search":           {},
			"/api/search":       {},
			"/api/search-quick": {},
		},
		PublicHeads: []string{
			"/static/",
			"/dist/",
		},
	}
}

func (p AccessPolicy) IsPublicPath(path string) bool {
	if _, ok := p.PublicPaths[path]; ok {
		return true
	}

	for _, head := range p.PublicHeads {
		if strings.HasPrefix(path, head) {
			return true
		}
	}

	return false
}

func RequireGlobalAuthWithPolicy(policy AccessPolicy) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if policy.IsPublicPath(r.URL.Path) {
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
}
