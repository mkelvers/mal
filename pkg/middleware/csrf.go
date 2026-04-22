package middleware

import (
	"net/http"
	"net/url"
)

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
