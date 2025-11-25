package main

import (
	"net/http"
	"sync"
	"time"
)

// RateLimiter implements a simple token bucket rate limiter per IP
type RateLimiter struct {
	requestsPerMinute int
	buckets           map[string]*bucket
	mu                sync.Mutex
	cleanupInterval   time.Duration
	lastCleanup       time.Time
}

type bucket struct {
	tokens     int
	lastRefill time.Time
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(requestsPerMinute int) *RateLimiter {
	rl := &RateLimiter{
		requestsPerMinute: requestsPerMinute,
		buckets:           make(map[string]*bucket),
		cleanupInterval:   5 * time.Minute,
		lastCleanup:       time.Now(),
	}
	
	// Start cleanup goroutine
	go rl.cleanup()
	
	return rl
}

// cleanup removes old buckets periodically
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(rl.cleanupInterval)
	defer ticker.Stop()
	
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for ip, b := range rl.buckets {
			// Remove buckets older than 10 minutes
			if now.Sub(b.lastRefill) > 10*time.Minute {
				delete(rl.buckets, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// getIP extracts the client IP from the request
func (rl *RateLimiter) getIP(r *http.Request) string {
	// Check X-Forwarded-For header (for proxies)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}
	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	// Fall back to RemoteAddr
	ip := r.RemoteAddr
	// Remove port if present
	if idx := len(ip) - 1; idx >= 0 && ip[idx] == ']' {
		// IPv6 with port
		if colonIdx := len(ip) - 1; colonIdx >= 0 {
			for i := colonIdx; i >= 0; i-- {
				if ip[i] == ':' {
					ip = ip[:i]
					break
				}
			}
		}
	} else {
		// IPv4 with port
		for i := len(ip) - 1; i >= 0; i-- {
			if ip[i] == ':' {
				ip = ip[:i]
				break
			}
		}
	}
	return ip
}

// Allow checks if a request should be allowed
func (rl *RateLimiter) Allow(r *http.Request) bool {
	if rl.requestsPerMinute <= 0 {
		return true // Rate limiting disabled
	}
	
	ip := rl.getIP(r)
	now := time.Now()
	
	rl.mu.Lock()
	defer rl.mu.Unlock()
	
	b, exists := rl.buckets[ip]
	if !exists {
		// Create new bucket with full tokens
		b = &bucket{
			tokens:     rl.requestsPerMinute,
			lastRefill: now,
		}
		rl.buckets[ip] = b
	}
	
	// Refill tokens based on time passed
	elapsed := now.Sub(b.lastRefill)
	if elapsed >= time.Minute {
		// Full refill
		b.tokens = rl.requestsPerMinute
		b.lastRefill = now
	} else {
		// Partial refill: add tokens proportional to time passed
		tokensToAdd := int(float64(rl.requestsPerMinute) * elapsed.Seconds() / 60.0)
		if tokensToAdd > 0 {
			b.tokens += tokensToAdd
			if b.tokens > rl.requestsPerMinute {
				b.tokens = rl.requestsPerMinute
			}
			b.lastRefill = now
		}
	}
	
	// Check if we have tokens
	if b.tokens > 0 {
		b.tokens--
		return true
	}
	
	return false
}

// RateLimitMiddleware wraps a handler with rate limiting
func RateLimitMiddleware(handler http.HandlerFunc, limiter *RateLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !limiter.Allow(r) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":"Rate limit exceeded"}`))
			return
		}
		handler(w, r)
	}
}

