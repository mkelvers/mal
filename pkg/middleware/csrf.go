package middleware

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"net/url"
	"sync"
)

const (
	csrfTokenHeader = "X-CSRF-Token"
	csrfTokenCookie = "csrf_token"
	csrfTokenLength = 32
)

var (
	csrfTokens = make(map[string]bool)
	csrfMu     sync.RWMutex
)

func generateCSRFToken() string {
	b := make([]byte, csrfTokenLength)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}

func ValidateCSRFToken(token string) bool {
	csrfMu.RLock()
	defer csrfMu.RUnlock()
	return csrfTokens[token]
}

func addCSRFToken(token string) {
	csrfMu.Lock()
	defer csrfMu.Unlock()
	csrfTokens[token] = true
}

func removeCSRFToken(token string) {
	csrfMu.Lock()
	defer csrfMu.Unlock()
	delete(csrfTokens, token)
}

func CSRFMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			token := generateCSRFToken()
			addCSRFToken(token)
			http.SetCookie(w, &http.Cookie{
				Name:     csrfTokenCookie,
				Value:    token,
				Path:     "/",
				HttpOnly: false,
				SameSite: http.SameSiteStrictMode,
			})
			w.Header().Set("X-CSRF-Token", token)
			next.ServeHTTP(w, r)
			return
		}

		token := r.Header.Get(csrfTokenHeader)
		if token == "" {
			token = r.FormValue("csrf_token")
		}

		if token == "" || !ValidateCSRFToken(token) {
			http.Error(w, "Invalid CSRF token", http.StatusForbidden)
			return
		}

		removeCSRFToken(token)
		next.ServeHTTP(w, r)
	})
}

func VerifyOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}

		origin := r.Header.Get("Origin")
		if origin == "" {
			referer := r.Header.Get("Referer")
			if referer == "" {
				http.Error(w, "Missing Origin or Referer header", http.StatusForbidden)
				return
			}

			refURL, err := url.Parse(referer)
			if err != nil {
				http.Error(w, "Invalid Referer header", http.StatusForbidden)
				return
			}
			origin = refURL.Scheme + "://" + refURL.Host
		}

		originURL, err := url.Parse(origin)
		if err != nil {
			http.Error(w, "Invalid Origin header", http.StatusForbidden)
			return
		}

		host := r.Host
		if forwardedHost := r.Header.Get("X-Forwarded-Host"); forwardedHost != "" {
			host = forwardedHost
		}

		expectedHTTP := "http://" + host
		expectedHTTPS := "https://" + host

		if originURL.Scheme+"://"+originURL.Host != expectedHTTP && originURL.Scheme+"://"+originURL.Host != expectedHTTPS {
			http.Error(w, "Cross-Site Request Forgery (CSRF) origin mismatch", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}
