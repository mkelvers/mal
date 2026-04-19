package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"mal/internal/database"
)

func TestRequireAuth_UnauthenticatedAPIRequest(t *testing.T) {
	t.Parallel()

	h := RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/watchlist", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}

	if got := rec.Header().Get("HX-Redirect"); got != "/login" {
		t.Fatalf("expected HX-Redirect /login, got %q", got)
	}
}

func TestRequireAuth_AuthenticatedRequestPassesThrough(t *testing.T) {
	t.Parallel()

	h := RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/watchlist", nil)
	ctx := context.WithValue(req.Context(), UserContextKey, &database.User{ID: "user-1"})
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req.WithContext(ctx))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}
}

func TestRequireGlobalAuth_AllowsPublicRoute(t *testing.T) {
	t.Parallel()

	h := RequireGlobalAuthWithPolicy(NewAccessPolicy())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}
