package httpapi

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// rateLimiter is a minimal per-key sliding-window limiter, in-memory and
// unbounded in size. That is an acceptable trade-off for a self-hosted,
// single-instance deployment with a small user base; it is not meant to
// survive a restart or to scale past one process.
type rateLimiter struct {
	mu       sync.Mutex
	max      int
	window   time.Duration
	attempts map[string][]time.Time
}

func newRateLimiter(max int, window time.Duration) *rateLimiter {
	return &rateLimiter{max: max, window: window, attempts: make(map[string][]time.Time)}
}

func (rl *rateLimiter) allow(key string, now time.Time) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	cutoff := now.Add(-rl.window)
	kept := rl.attempts[key][:0]
	for _, t := range rl.attempts[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= rl.max {
		rl.attempts[key] = kept
		return false
	}
	rl.attempts[key] = append(kept, now)
	return true
}

func (rl *rateLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		if !rl.allow(host, time.Now()) {
			writeError(w, http.StatusTooManyRequests, "too many attempts, try again later")
			return
		}
		next.ServeHTTP(w, r)
	})
}
