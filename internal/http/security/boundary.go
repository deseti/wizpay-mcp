// Package security provides bounded, credential-free process admission controls.
package security

import (
	"net/http"
	"strings"
	"sync"
	"time"
)

type bucket struct {
	tokens float64
	at     time.Time
}
type Boundary struct {
	mu      sync.Mutex
	buckets map[string]bucket
	now     func() time.Time
}

func New(now func() time.Time) *Boundary { return &Boundary{buckets: map[string]bucket{}, now: now} }
func class(path string) (string, float64) {
	switch {
	case path == "/oauth/token":
		return "token", 30
	case path == "/oauth/authorize":
		return "authorize", 30
	case strings.HasPrefix(path, "/browser/siwe/"):
		return "siwe", 20
	case strings.HasPrefix(path, "/browser/"):
		return "browser", 120
	case path == "/mcp":
		return "mcp", 120
	}
	return "", 0
}
func (b *Boundary) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		k, capacity := class(r.URL.Path)
		if k != "" {
			b.mu.Lock()
			now := b.now()
			v, exists := b.buckets[k]
			if !exists {
				v = bucket{capacity, now}
			}
			elapsed := now.Sub(v.at).Seconds()
			if elapsed > 0 {
				v.tokens += elapsed * capacity / 60
				if v.tokens > capacity {
					v.tokens = capacity
				}
				v.at = now
			}
			allowed := v.tokens >= 1
			if allowed {
				v.tokens--
			}
			b.buckets[k] = v
			b.mu.Unlock()
			if !allowed {
				w.Header().Set("Cache-Control", "no-store")
				w.Header().Set("Retry-After", "3")
				http.Error(w, "Request limit exceeded", 429)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
