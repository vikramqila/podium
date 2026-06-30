package gateway

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"gatewaykit/internal/config"
)

type rateLimiter struct {
	mu      sync.Mutex
	buckets map[string]rateBucket
	now     func() time.Time
}

type rateBucket struct {
	windowStart time.Time
	count       int
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{
		buckets: map[string]rateBucket{},
		now:     time.Now,
	}
}

func (l *rateLimiter) allow(route config.Route, r *http.Request, rule config.RateLimit) bool {
	if rule.Strategy != "fixed_window" {
		return true
	}

	window := parseDuration(rule.Window)
	if rule.Requests <= 0 || window <= 0 {
		return true
	}

	key := rateLimitKey(route, r, rule)
	now := l.now()

	l.mu.Lock()
	defer l.mu.Unlock()

	bucket := l.buckets[key]
	if bucket.windowStart.IsZero() || now.Sub(bucket.windowStart) >= window {
		l.buckets[key] = rateBucket{
			windowStart: now,
			count:       1,
		}
		return true
	}

	if bucket.count >= rule.Requests {
		return false
	}

	bucket.count++
	l.buckets[key] = bucket
	return true
}

func rateLimitKey(route config.Route, r *http.Request, rule config.RateLimit) string {
	scope := "global"
	if rule.Per == "ip" {
		scope = clientIP(r)
	}
	return route.Path + "|" + rule.Per + "|" + scope
}

func clientIP(r *http.Request) string {
	if forwardedFor := r.Header.Get("X-Forwarded-For"); forwardedFor != "" {
		if ip, _, ok := strings.Cut(forwardedFor, ","); ok {
			return strings.TrimSpace(ip)
		}
		return strings.TrimSpace(forwardedFor)
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
