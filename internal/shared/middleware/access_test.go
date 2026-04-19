package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"mal/internal/database"
)

func TestAccessPolicy_IsPublicPath(t *testing.T) {
	t.Parallel()

	policy := NewAccessPolicy()

	if !policy.IsPublicPath("/") {
		t.Fatal("expected / to be public")
	}

	if !policy.IsPublicPath("/api/search") {
		t.Fatal("expected /api/search to be public")
	}

	if !policy.IsPublicPath("/static/app.css") {
		t.Fatal("expected /static/app.css to be public")
	}

	if policy.IsPublicPath("/watchlist") {
		t.Fatal("expected /watchlist to be private")
	}
}

func TestRequireGlobalAuthWithPolicy_ProtectedPath(t *testing.T) {
	t.Parallel()

	policy := AccessPolicy{
		PublicPaths: map[string]struct{}{"/public": {}},
	}

	h := RequireGlobalAuthWithPolicy(policy)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected redirect status, got %d", rec.Code)
	}

	if location := rec.Header().Get("Location"); location != "/login" {
		t.Fatalf("expected redirect to /login, got %q", location)
	}
}

func TestRequireGlobalAuthWithPolicy_AllowsAuthenticatedUser(t *testing.T) {
	t.Parallel()

	policy := AccessPolicy{
		PublicPaths: map[string]struct{}{},
	}

	h := RequireGlobalAuthWithPolicy(policy)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	ctx := context.WithValue(req.Context(), UserContextKey, &database.User{ID: "user-1"})
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req.WithContext(ctx))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}
}
