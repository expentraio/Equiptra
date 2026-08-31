package middleware

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// RateLimit is a minimal in-memory, per-client-IP fixed-window limiter.
// Scoped to a single deliberately small use case (the public fault-report
// endpoints, which have no auth at all) rather than a general-purpose
// facility — a single Render instance and a handful of protected routes
// don't warrant a distributed limiter or an external dependency.
func RateLimit(maxRequests int, window time.Duration) func(http.Handler) http.Handler {
	var mu sync.Mutex
	type bucket struct {
		count       int
		windowStart time.Time
	}
	buckets := map[string]*bucket{}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r)

			mu.Lock()
			now := time.Now()
			b, ok := buckets[ip]
			if !ok || now.Sub(b.windowStart) >= window {
				b = &bucket{count: 0, windowStart: now}
				buckets[ip] = b
			}
			b.count++
			blocked := b.count > maxRequests
			mu.Unlock()

			if blocked {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "too many requests — please try again shortly"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// clientIP prefers the first hop of X-Forwarded-For, since Render/Vercel
// sit in front of this app as proxies — RemoteAddr alone would just be the
// proxy's own address.
func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		if first, _, ok := strings.Cut(fwd, ","); ok {
			return strings.TrimSpace(first)
		}
		return strings.TrimSpace(fwd)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
