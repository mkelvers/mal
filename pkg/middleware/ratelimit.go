package middleware

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

type visitor struct {
	attempts int
	lastSeen time.Time
}

var (
	visitors = make(map[string]*visitor)
	mu       sync.Mutex
	quit     = make(chan struct{})
)

func init() {
	go cleanupVisitors()
}

func StopCleanup() {
	close(quit)
}

func cleanupVisitors() {
	for {
		select {
		case <-quit:
			return
		case <-time.After(time.Minute):
			mu.Lock()
			for ip, v := range visitors {
				if time.Since(v.lastSeen) > 3*time.Minute {
					delete(visitors, ip)
				}
			}
			mu.Unlock()
		}
	}
}

func getIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		return strings.TrimSpace(ips[0])
	}
	if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
		return realIP
	}
	ip := r.RemoteAddr
	if colonIdx := strings.LastIndex(ip, ":"); colonIdx != -1 {
		ip = ip[:colonIdx]
	}
	return ip
}

func RateLimitAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := getIP(r)

		mu.Lock()
		v, exists := visitors[ip]
		if !exists {
			visitors[ip] = &visitor{1, time.Now()}
		} else {
			// Reset attempts if it's been more than a minute
			if time.Since(v.lastSeen) > time.Minute {
				v.attempts = 0
			}
			v.attempts++
			v.lastSeen = time.Now()
		}

		// If more than 5 attempts within a minute, block
		if exists && v.attempts > 5 {
			mu.Unlock()
			if strings.HasPrefix(r.URL.Path, "/") {
				http.Redirect(w, r, fmt.Sprintf("%s?error=rate_limited", r.URL.Path), http.StatusFound)
				return
			}
			http.Error(w, "Too many requests. Please try again later.", http.StatusTooManyRequests)
			return
		}
		mu.Unlock()

		next.ServeHTTP(w, r)
	})
}
