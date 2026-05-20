package middleware

import (
	"edge-proxy/internal/logger"
	"edge-proxy/internal/proxy/runtime"
	"edge-proxy/internal/ratelimit"
	"net"
	"net/http"

	"strconv"
	"strings"
	"sync"
	"time"
)

type RateLimitMiddleware struct {
	state        *runtime.Runtime
	rateLimiters map[string]*ratelimit.RateLimiter
	mu           sync.RWMutex
}

func NewRateLimitMiddleware(state *runtime.Runtime) *RateLimitMiddleware {
	return &RateLimitMiddleware{
		state:        state,
		rateLimiters: make(map[string]*ratelimit.RateLimiter),
	}
}

func (rl *RateLimitMiddleware) getRateLimiter(host string) *ratelimit.RateLimiter {
	rl.mu.RLock()
	limiter, exists := rl.rateLimiters[host]
	rl.mu.RUnlock()

	if exists {
		return limiter
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Double-check after taking the write lock to avoid creating duplicate limiters.
	limiter, exists = rl.rateLimiters[host]
	if exists {
		return limiter
	}

	cfg := rl.state.GetSecurityConfigHost(host)
	rate := float64(cfg.RateLimiting.RatePerIP)
	burst := float64(cfg.RateLimiting.Burst)
	window := time.Duration(cfg.RateLimiting.WindowSec) * time.Second
	limiter = ratelimit.NewRateLimiter(rate, burst, window)
	rl.rateLimiters[host] = limiter

	return limiter
}

func (rl *RateLimitMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg := rl.state.GetSecurityConfigHost(r.Host)

		if !cfg.RateLimiting.Enabled {
			next.ServeHTTP(w, r)
			return
		}

		clientIP := getRealIP(r)

		host := r.Host
		if strings.Contains(host, ":") {
			host = strings.Split(host, ":")[0]
		}

		limiter := rl.getRateLimiter(host)
		if !limiter.Allow(clientIP) {
			rl.state.Metrics.RecordRateLimitBlock()
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(int(cfg.RateLimiting.RatePerIP)))
			w.Header().Set("X-RateLimit-Window", strconv.Itoa(int(cfg.RateLimiting.WindowSec)))
			w.Header().Set("X-RateLimit-Blocked", "true")
			http.Error(w, "Too many requests", http.StatusTooManyRequests)
			return
		}
		rl.state.Metrics.RecordRateLimitAllow()

		next.ServeHTTP(w, r)
	})
}

func getRealIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		return strings.TrimSpace(parts[0])
	}

	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

func (rl *RateLimitMiddleware) CleanupExpiredLimiters(cleanupInterval time.Duration) {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		for host, limiter := range rl.rateLimiters {
			if !rl.hasVirtualHost(host) {
				delete(rl.rateLimiters, host)
				limiter.Stop()
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *RateLimitMiddleware) hasVirtualHost(host string) bool {
	snapshot := rl.state.SnapshotView()
	for _, vhost := range snapshot.Raw.VirtualHosts {
		if vhost.Domain == host {
			return true
		}
	}
	return false
}

func (rl *RateLimitMiddleware) GetRateLimitStats() map[string]interface{} {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	stats := map[string]interface{}{
		"total_rate_limiters": len(rl.rateLimiters),
		"hosts":               make([]string, 0, len(rl.rateLimiters)),
	}
	for host := range rl.rateLimiters {
		stats["hosts"] = append(stats["hosts"].([]string), host)
	}

	return stats
}

func (rl *RateLimitMiddleware) UpdateRateLimiter(host string, ratePerIP, burst int, windowSec int) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if old, ok := rl.rateLimiters[host]; ok {
		old.Stop()
	}

	limiter := ratelimit.NewRateLimiter(
		float64(ratePerIP),
		float64(burst),
		time.Duration(windowSec)*time.Second,
	)

	rl.rateLimiters[host] = limiter

	logger.Info("Rate limiter updated", map[string]interface{}{
		"host":        host,
		"rate_per_ip": ratePerIP,
		"burst":       burst,
		"window_sec":  windowSec,
	})
}
