package middleware

import (
	"net/http"
	"net/url"
)

// VerifyOrigin prevents simple CSRF by ensuring the Origin or Referer header matches the Host header
// for state-changing endpoints (POST/PUT/DELETE).
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
				// If neither is present, and it's a POST/PUT/DELETE, reject it (strict policy)
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

		host := r.Host
		// Optional: strip port if you only care about domain
		
		// If origin doesn't match host (accounting for potential schema prefixes)
		expectedHTTP := "http://" + host
		expectedHTTPS := "https://" + host
		
		if origin != expectedHTTP && origin != expectedHTTPS {
			http.Error(w, "Cross-Site Request Forgery (CSRF) origin mismatch", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}
