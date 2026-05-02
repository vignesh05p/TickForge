package server

import (
	"crypto/subtle"
	"net"
	"net/http"
	"sync"
	"time"
)

// RequireAPIKey returns a middleware that enforces X-API-Key authentication
// using a constant-time comparison to prevent timing attacks.
// Routes that bypass auth (health, readiness, metrics) should not be wrapped.
func RequireAPIKey(apiKey string) func(http.Handler) http.Handler {
	keyBytes := []byte(apiKey)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got := []byte(r.Header.Get("X-API-Key"))
			// subtle.ConstantTimeCompare returns 1 on match; 0 on mismatch or
			// length difference. This prevents timing side-channels.
			if subtle.ConstantTimeCompare(got, keyBytes) != 1 {
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "valid X-API-Key header required")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ipRateLimiter tracks a token-bucket per client IP.
// It is safe for concurrent use.
type ipRateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*tokenBucket
	limit    int     // tokens replenished per second
	burst    int     // maximum tokens per bucket
	interval float64 // seconds between token replenishments (1/limit)
}

type tokenBucket struct {
	tokens   float64
	lastSeen time.Time
}

func newIPRateLimiter(limitPerSec, burst int) *ipRateLimiter {
	return &ipRateLimiter{
		buckets:  make(map[string]*tokenBucket),
		limit:    limitPerSec,
		burst:    burst,
		interval: 1.0 / float64(limitPerSec),
	}
}

// allow returns true if the request from ip should be allowed.
func (l *ipRateLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	b, ok := l.buckets[ip]
	if !ok {
		l.buckets[ip] = &tokenBucket{tokens: float64(l.burst) - 1, lastSeen: now}
		return true
	}

	// Replenish tokens based on elapsed time.
	elapsed := now.Sub(b.lastSeen).Seconds()
	b.tokens += elapsed * float64(l.limit)
	if b.tokens > float64(l.burst) {
		b.tokens = float64(l.burst)
	}
	b.lastSeen = now

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// cleanup removes stale entries that haven't been seen for more than ttl.
// Call periodically (e.g. every minute) to prevent unbounded map growth.
func (l *ipRateLimiter) cleanup(ttl time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := time.Now().Add(-ttl)
	for ip, b := range l.buckets {
		if b.lastSeen.Before(cutoff) {
			delete(l.buckets, ip)
		}
	}
}

// RateLimit returns a middleware that applies per-client-IP token-bucket rate
// limiting at the HTTP layer — before the tick queue backpressure applies.
// Requests that exceed the limit receive 429 Too Many Requests immediately.
func RateLimit(limitPerSec, burst int) func(http.Handler) http.Handler {
	limiter := newIPRateLimiter(limitPerSec, burst)

	// Background cleanup goroutine to prevent unbounded memory growth.
	// Entries idle for 5 minutes are evicted.
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			limiter.cleanup(5 * time.Minute)
		}
	}()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				ip = r.RemoteAddr // fallback for tests / unix sockets
			}
			if !limiter.allow(ip) {
				w.Header().Set("Retry-After", "1")
				writeError(w, http.StatusTooManyRequests, "RATE_LIMITED",
					"too many requests from this IP — retry after 1 second")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Chain applies a list of middleware in order (first = outermost wrapper).
func Chain(h http.Handler, middleware ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middleware) - 1; i >= 0; i-- {
		h = middleware[i](h)
	}
	return h
}
