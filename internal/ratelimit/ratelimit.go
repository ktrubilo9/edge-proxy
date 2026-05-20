package ratelimit

import (
	"sync"
	"time"
)

type RateLimiter struct {
	limits map[string]*TokenBucket
	mu     sync.RWMutex

	defaultRate     float64
	defaultCapacity float64
	cleanupInterval time.Duration
	stopChan        chan struct{}
	stopped         sync.Once
}

type TokenBucket struct {
	tokens    float64
	capacity  float64
	rate      float64
	lastCheck time.Time
	mu        sync.Mutex
}

func NewRateLimiter(defaultRate, defaultCapacity float64, cleanupInterval time.Duration) *RateLimiter {
	if cleanupInterval <= 0 {
		cleanupInterval = time.Minute
	}

	rl := &RateLimiter{
		limits:          make(map[string]*TokenBucket),
		defaultRate:     defaultRate,
		defaultCapacity: defaultCapacity,
		cleanupInterval: cleanupInterval,
		stopChan:        make(chan struct{}),
	}
	go rl.cleanupLoop()
	return rl
}

func (rl *RateLimiter) Stop() {
	rl.stopped.Do(func() {
		close(rl.stopChan)
	})
}

func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.mu.Lock()
			// Buckets idle long enough are discarded to keep the limiter bounded in memory.
			idleTTL := 10 * time.Minute
			for key, bucket := range rl.limits {
				bucket.mu.Lock()
				if time.Since(bucket.lastCheck) > idleTTL {
					delete(rl.limits, key)
				}
				bucket.mu.Unlock()
			}
			rl.mu.Unlock()
		case <-rl.stopChan:
			return
		}
	}
}

func (rl *RateLimiter) getBucket(ip string) *TokenBucket {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	bucket, exists := rl.limits[ip]
	if exists {
		return bucket
	}

	bucket = &TokenBucket{
		tokens:    rl.defaultCapacity,
		capacity:  rl.defaultCapacity,
		rate:      rl.defaultRate,
		lastCheck: time.Now(),
	}
	rl.limits[ip] = bucket
	return bucket
}

func (rl *RateLimiter) Allow(ip string) bool {
	b := rl.getBucket(ip)
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(b.lastCheck).Seconds()
	b.lastCheck = now

	b.tokens += elapsed * b.rate
	if b.tokens > b.capacity {
		b.tokens = b.capacity
	}

	if b.tokens >= 1 {
		b.tokens -= 1
		return true
	}
	return false
}

func (rl *RateLimiter) SetLimit(ip string, rate, capacity float64) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.limits[ip] = &TokenBucket{
		tokens:    capacity,
		capacity:  capacity,
		rate:      rate,
		lastCheck: time.Now(),
	}
}
