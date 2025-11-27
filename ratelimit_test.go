package main

import (
	"net/http"
	"testing"
	"time"
)

func TestNewRateLimiter(t *testing.T) {
	tests := []struct {
		name              string
		requestsPerMinute int
	}{
		{
			name:              "rate limiter with 60 RPM",
			requestsPerMinute: 60,
		},
		{
			name:              "rate limiter with 0 RPM (disabled)",
			requestsPerMinute: 0,
		},
		{
			name:              "rate limiter with 1 RPM",
			requestsPerMinute: 1,
		},
		{
			name:              "rate limiter with high RPM",
			requestsPerMinute: 1000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rl := NewRateLimiter(tt.requestsPerMinute)
			if rl == nil {
				t.Fatal("NewRateLimiter() = nil; want non-nil")
			}
			if rl.requestsPerMinute != tt.requestsPerMinute {
				t.Errorf("NewRateLimiter() requestsPerMinute = %d; want %d", rl.requestsPerMinute, tt.requestsPerMinute)
			}
			if rl.buckets == nil {
				t.Error("NewRateLimiter() buckets = nil; want non-nil")
			}
		})
	}
}

func TestRateLimiterAllow(t *testing.T) {
	tests := []struct {
		name              string
		requestsPerMinute int
		numRequests       int
		wantAllowed       int
	}{
		{
			name:              "allow all when disabled",
			requestsPerMinute: 0,
			numRequests:       100,
			wantAllowed:       100,
		},
		{
			name:              "allow up to limit",
			requestsPerMinute: 10,
			numRequests:       10,
			wantAllowed:       10,
		},
		{
			name:              "block after limit",
			requestsPerMinute: 5,
			numRequests:       10,
			wantAllowed:       5,
		},
		{
			name:              "single request",
			requestsPerMinute: 1,
			numRequests:       1,
			wantAllowed:       1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rl := NewRateLimiter(tt.requestsPerMinute)
			req := createTestRequest("192.168.1.1:8080")
			
			allowed := 0
			for i := 0; i < tt.numRequests; i++ {
				if rl.Allow(req) {
					allowed++
				}
			}
			
			if allowed != tt.wantAllowed {
				t.Errorf("RateLimiter.Allow() allowed %d requests; want %d", allowed, tt.wantAllowed)
			}
		})
	}
}

func TestRateLimiterTokenRefill(t *testing.T) {
	rl := NewRateLimiter(10)
	req := createTestRequest("192.168.1.1:8080")
	
	// Exhaust tokens
	for i := 0; i < 10; i++ {
		if !rl.Allow(req) {
			t.Errorf("RateLimiter.Allow() request %d = false; want true", i+1)
		}
	}
	
	// Next request should be blocked
	if rl.Allow(req) {
		t.Error("RateLimiter.Allow() after limit = true; want false")
	}
	
	// Wait for token refill (simulate by manipulating time)
	// Note: In real implementation, we'd need to wait or mock time
	// For now, we test that tokens are refilled after time passes
	rl.mu.Lock()
	bucket := rl.buckets["192.168.1.1"]
	if bucket == nil {
		rl.mu.Unlock()
		t.Fatal("bucket not found")
	}
	// Manually set lastRefill to simulate time passing
	bucket.lastRefill = time.Now().Add(-2 * time.Minute)
	rl.mu.Unlock()
	
	// Should be allowed now (tokens refilled)
	if !rl.Allow(req) {
		t.Error("RateLimiter.Allow() after refill = false; want true")
	}
}

func TestRateLimiterMultipleIPs(t *testing.T) {
	rl := NewRateLimiter(5)
	
	req1 := createTestRequest("192.168.1.1:8080")
	req2 := createTestRequest("192.168.1.2:8080")
	
	// Both IPs should get their own buckets
	for i := 0; i < 5; i++ {
		if !rl.Allow(req1) {
			t.Errorf("RateLimiter.Allow() IP1 request %d = false; want true", i+1)
		}
		if !rl.Allow(req2) {
			t.Errorf("RateLimiter.Allow() IP2 request %d = false; want true", i+1)
		}
	}
	
	// Both should be blocked now
	if rl.Allow(req1) {
		t.Error("RateLimiter.Allow() IP1 after limit = true; want false")
	}
	if rl.Allow(req2) {
		t.Error("RateLimiter.Allow() IP2 after limit = true; want false")
	}
}

func TestRateLimiterGetIP(t *testing.T) {
	rl := NewRateLimiter(10)
	
	tests := []struct {
		name           string
		remoteAddr     string
		xForwardedFor  string
		xRealIP        string
		wantIP         string
	}{
		{
			name:       "IPv4 with port",
			remoteAddr: "192.168.1.1:8080",
			wantIP:     "192.168.1.1",
		},
		{
			name:       "IPv6 with port",
			remoteAddr: "[2001:db8::1]:8080",
			wantIP:     "[2001:db8::1]",
		},
		{
			name:          "X-Forwarded-For header",
			remoteAddr:    "192.168.1.1:8080",
			xForwardedFor: "10.0.0.1",
			wantIP:        "10.0.0.1",
		},
		{
			name:       "X-Real-IP header",
			remoteAddr: "192.168.1.1:8080",
			xRealIP:    "10.0.0.2",
			wantIP:     "10.0.0.2",
		},
		{
			name:          "X-Forwarded-For takes precedence",
			remoteAddr:    "192.168.1.1:8080",
			xForwardedFor: "10.0.0.1",
			xRealIP:       "10.0.0.2",
			wantIP:        "10.0.0.1",
		},
		{
			name:       "IPv4 without port",
			remoteAddr: "192.168.1.1",
			wantIP:     "192.168.1.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &http.Request{
				RemoteAddr: tt.remoteAddr,
				Header:     make(http.Header),
			}
			if tt.xForwardedFor != "" {
				req.Header.Set("X-Forwarded-For", tt.xForwardedFor)
			}
			if tt.xRealIP != "" {
				req.Header.Set("X-Real-IP", tt.xRealIP)
			}
			
			gotIP := rl.getIP(req)
			if gotIP != tt.wantIP {
				t.Errorf("RateLimiter.getIP() = %q; want %q", gotIP, tt.wantIP)
			}
		})
	}
}

func TestRateLimitMiddleware(t *testing.T) {
	rl := NewRateLimiter(2) // Allow 2 requests per minute
	
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})
	
	middleware := RateLimitMiddleware(handler, rl)
	
	req := createTestRequest("192.168.1.1:8080")
	
	// First two requests should succeed
	for i := 0; i < 2; i++ {
		w := &mockResponseWriter{headers: make(http.Header)}
		middleware(w, req)
		if w.statusCode != http.StatusOK {
			t.Errorf("RateLimitMiddleware() request %d status = %d; want %d", i+1, w.statusCode, http.StatusOK)
		}
	}
	
	// Third request should be rate limited
	w := &mockResponseWriter{headers: make(http.Header)}
	middleware(w, req)
	if w.statusCode != http.StatusTooManyRequests {
		t.Errorf("RateLimitMiddleware() rate limited request status = %d; want %d", w.statusCode, http.StatusTooManyRequests)
	}
	
	// Check response body
	bodyStr := string(w.body)
	if bodyStr != `{"error":"Rate limit exceeded"}` {
		t.Errorf("RateLimitMiddleware() body = %q; want %q", bodyStr, `{"error":"Rate limit exceeded"}`)
	}
}

func TestRateLimiterDisabled(t *testing.T) {
	rl := NewRateLimiter(0) // Disabled
	req := createTestRequest("192.168.1.1:8080")
	
	// Should allow unlimited requests
	for i := 0; i < 100; i++ {
		if !rl.Allow(req) {
			t.Errorf("RateLimiter.Allow() with disabled limiter request %d = false; want true", i+1)
		}
	}
}

func TestRateLimiterPartialRefill(t *testing.T) {
	rl := NewRateLimiter(60) // 60 requests per minute
	req := createTestRequest("192.168.1.1:8080")
	
	// Exhaust all tokens
	for i := 0; i < 60; i++ {
		rl.Allow(req)
	}
	
	// Should be blocked
	if rl.Allow(req) {
		t.Error("RateLimiter.Allow() after exhaustion = true; want false")
	}
	
	// Simulate 30 seconds passing (should refill 30 tokens)
	rl.mu.Lock()
	bucket := rl.buckets["192.168.1.1"]
	bucket.lastRefill = time.Now().Add(-30 * time.Second)
	rl.mu.Unlock()
	
	// Should allow 30 more requests
	allowed := 0
	for i := 0; i < 60; i++ {
		if rl.Allow(req) {
			allowed++
		}
	}
	
	// Should have refilled approximately 30 tokens
	if allowed < 25 || allowed > 35 {
		t.Errorf("RateLimiter.Allow() after partial refill allowed %d; want ~30", allowed)
	}
}

// Helper function
func createTestRequest(remoteAddr string) *http.Request {
	return &http.Request{
		RemoteAddr: remoteAddr,
		Header:     make(http.Header),
	}
}

