package main

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestValidateJWT(t *testing.T) {
	secret := "test-secret-key"
	
	tests := []struct {
		name            string
		token           string
		secret          string
		expectedAud     *string
		wantErr         bool
		errContains     string
		wantTaskID      string
	}{
		{
			name:        "valid API token without audience",
			token:       createTestToken(t, secret, "", "test-task-id", time.Hour),
			secret:      secret,
			expectedAud: stringPtr(""),
			wantErr:     false,
			wantTaskID:  "test-task-id",
		},
		{
			name:        "valid viewer token with audience",
			token:       createTestToken(t, secret, "viewer", "test-task-id", time.Hour),
			secret:      secret,
			expectedAud: stringPtr("viewer"),
			wantErr:     false,
			wantTaskID:  "test-task-id",
		},
		{
			name:        "missing token parameter",
			token:       "",
			secret:      secret,
			expectedAud: stringPtr(""),
			wantErr:     true,
			errContains: "missing token",
		},
		{
			name:        "invalid token signature",
			token:       createTestToken(t, "wrong-secret", "", "test-task-id", time.Hour),
			secret:      secret,
			expectedAud: stringPtr(""),
			wantErr:     true,
			errContains: "failed to parse",
		},
		{
			name:        "expired token",
			token:       createTestToken(t, secret, "", "test-task-id", -time.Hour),
			secret:      secret,
			expectedAud: stringPtr(""),
			wantErr:     true,
			errContains: "expired",
		},
		{
			name:        "viewer token used for API (audience mismatch)",
			token:       createTestToken(t, secret, "viewer", "test-task-id", time.Hour),
			secret:      secret,
			expectedAud: stringPtr(""),
			wantErr:     true,
			errContains: "audience mismatch",
		},
		{
			name:        "API token used for viewer (audience mismatch)",
			token:       createTestToken(t, secret, "", "test-task-id", time.Hour),
			secret:      secret,
			expectedAud: stringPtr("viewer"),
			wantErr:     true,
			errContains: "audience mismatch",
		},
		{
			name:        "valid token with nil audience check",
			token:       createTestToken(t, secret, "", "test-task-id", time.Hour),
			secret:      secret,
			expectedAud: nil,
			wantErr:     false,
			wantTaskID:  "test-task-id",
		},
		{
			name:        "malformed token",
			token:       "not.a.valid.token",
			secret:      secret,
			expectedAud: stringPtr(""),
			wantErr:     true,
			errContains: "failed to parse",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := createRequestWithToken(tt.token)
			claims, err := validateJWT(req, tt.secret, tt.expectedAud)
			
			if tt.wantErr {
				if err == nil {
					t.Errorf("validateJWT() = %v, nil; want error", claims)
				} else if tt.errContains != "" && !containsString(err.Error(), tt.errContains) {
					t.Errorf("validateJWT() error = %v; want error containing %q", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("validateJWT() = nil, %v; want claims, nil", err)
				} else if claims.TaskID != tt.wantTaskID {
					t.Errorf("validateJWT() claims.TaskID = %q; want %q", claims.TaskID, tt.wantTaskID)
				}
			}
		})
	}
}

func TestValidateJWTWithDifferentAlgorithms(t *testing.T) {
	secret := "test-secret"
	
	// Test with invalid token (malformed)
	req := createRequestWithToken("invalid")
	_, err := validateJWT(req, secret, nil)
	if err == nil {
		t.Error("validateJWT() with invalid token = nil; want error")
	}
}

func TestValidateJWTExpiration(t *testing.T) {
	secret := "test-secret"
	
	// Token expired 1 hour ago
	expiredToken := createTestToken(t, secret, "", "test", -time.Hour)
	req := createRequestWithToken(expiredToken)
	_, err := validateJWT(req, secret, nil)
	
	if err == nil {
		t.Error("validateJWT() with expired token = nil; want error")
	}
	if !containsString(err.Error(), "expired") {
		t.Errorf("validateJWT() error = %v; want error containing 'expired'", err)
	}
	
	// Token valid for 1 hour
	validToken := createTestToken(t, secret, "", "test", time.Hour)
	req = createRequestWithToken(validToken)
	claims, err := validateJWT(req, secret, nil)
	
	if err != nil {
		t.Errorf("validateJWT() with valid token = nil, %v; want claims, nil", err)
	}
	if claims == nil {
		t.Error("validateJWT() claims = nil; want non-nil")
	}
}

func TestValidateJWTAudience(t *testing.T) {
	secret := "test-secret"
	
	// API token (no audience)
	apiToken := createTestToken(t, secret, "", "test", time.Hour)
	req := createRequestWithToken(apiToken)
	
	// Should work for API
	apiAud := ""
	_, err := validateJWT(req, secret, &apiAud)
	if err != nil {
		t.Errorf("validateJWT() with API token for API = %v; want nil", err)
	}
	
	// Should fail for viewer
	viewerAud := "viewer"
	_, err = validateJWT(req, secret, &viewerAud)
	if err == nil {
		t.Error("validateJWT() with API token for viewer = nil; want error")
	}
	
	// Viewer token (with audience)
	viewerToken := createTestToken(t, secret, "viewer", "test", time.Hour)
	req = createRequestWithToken(viewerToken)
	
	// Should work for viewer
	_, err = validateJWT(req, secret, &viewerAud)
	if err != nil {
		t.Errorf("validateJWT() with viewer token for viewer = %v; want nil", err)
	}
	
	// Should fail for API
	_, err = validateJWT(req, secret, &apiAud)
	if err == nil {
		t.Error("validateJWT() with viewer token for API = nil; want error")
	}
}

func TestAuthMiddleware(t *testing.T) {
	secret := "test-secret"
	apiAud := ""
	
	// Create a handler that sets a header
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Test", "passed")
		w.WriteHeader(http.StatusOK)
	})
	
	// Wrap with auth middleware
	authHandler := authMiddleware(handler, secret, &apiAud)
	
	// Test with valid token
	validToken := createTestToken(t, secret, "", "test", time.Hour)
	req := createRequestWithToken(validToken)
	w := &mockResponseWriter{}
	
	authHandler(w, req)
	
	if w.statusCode != http.StatusOK {
		t.Errorf("authMiddleware() status = %d; want %d", w.statusCode, http.StatusOK)
	}
	if w.headers.Get("X-Test") != "passed" {
		t.Error("authMiddleware() handler not called")
	}
	
	// Test with invalid token
	req = createRequestWithToken("invalid-token")
	w = &mockResponseWriter{}
	
	authHandler(w, req)
	
	if w.statusCode != http.StatusUnauthorized {
		t.Errorf("authMiddleware() with invalid token status = %d; want %d", w.statusCode, http.StatusUnauthorized)
	}
}

// Helper functions

func createTestToken(t *testing.T, secret, audience, taskID string, expiration time.Duration) string {
	t.Helper()
	
	claims := &Claims{
		TaskID: taskID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiration)),
		},
	}
	
	if audience != "" {
		claims.Audience = []string{audience}
	}
	
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("Failed to create test token: %v", err)
	}
	
	return tokenString
}

func createRequestWithToken(token string) *http.Request {
	req := &http.Request{
		URL: &url.URL{
			RawQuery: "token=" + token,
		},
	}
	return req
}

func stringPtr(s string) *string {
	return &s
}

func containsString(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// mockResponseWriter for testing
type mockResponseWriter struct {
	headers    http.Header
	statusCode int
	body       []byte
}

func (m *mockResponseWriter) Header() http.Header {
	if m.headers == nil {
		m.headers = make(http.Header)
	}
	return m.headers
}

func (m *mockResponseWriter) Write(b []byte) (int, error) {
	m.body = append(m.body, b...)
	return len(b), nil
}

func (m *mockResponseWriter) WriteHeader(statusCode int) {
	m.statusCode = statusCode
}

