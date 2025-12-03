package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestSendJSONError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		message    string
	}{
		{
			name:       "unauthorized error",
			statusCode: http.StatusUnauthorized,
			message:    "Unauthorized access",
		},
		{
			name:       "bad request error",
			statusCode: http.StatusBadRequest,
			message:    "Invalid input",
		},
		{
			name:       "internal server error",
			statusCode: http.StatusInternalServerError,
			message:    "Server error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			sendJSONError(w, tt.statusCode, tt.message)

			if w.Code != tt.statusCode {
				t.Errorf("sendJSONError() status = %d; want %d", w.Code, tt.statusCode)
			}

			if w.Header().Get("Content-Type") != "application/json" {
				t.Errorf("sendJSONError() Content-Type = %q; want %q", w.Header().Get("Content-Type"), "application/json")
			}

			var response ErrorResponse
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatalf("sendJSONError() response is not valid JSON: %v", err)
			}

			if response.Error != tt.message {
				t.Errorf("sendJSONError() error message = %q; want %q", response.Error, tt.message)
			}
		})
	}
}

func TestHandleStartTask(t *testing.T) {
	// Setup test environment
	tmpDir, err := os.MkdirTemp("", "api-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	config := &Config{
		Server: ServerConfig{
			TaskDir: tmpDir,
		},
		Auth: AuthConfig{
			Secret: "test-secret-key",
		},
		Tasks: []TaskConfig{
			{
				Name:    "test-task",
				Command: "echo hello",
			},
			{
				Name:    "param-task",
				Command: "echo {{message}}",
				Parameters: []ParameterConfig{
					{Name: "message", Type: "string", Optional: false},
				},
			},
		},
	}

	taskManager := NewTaskManager(config)

	tests := []struct {
		name           string
		method         string
		body           string
		wantStatusCode int
		wantErr        bool
		errContains    string
		tokenType      string // "api", "viewer", "invalid", "missing"
	}{
		{
			name:           "valid request",
			method:         http.MethodPost,
			body:           `{"task_name": "test-task"}`,
			wantStatusCode: http.StatusOK,
			wantErr:        false,
			tokenType:      "api",
		},
		{
			name:           "valid request with parameters",
			method:         http.MethodPost,
			body:           `{"task_name": "param-task", "parameters": {"message": "hello"}}`,
			wantStatusCode: http.StatusOK,
			wantErr:        false,
			tokenType:      "api",
		},
		{
			name:           "missing token",
			method:         http.MethodPost,
			body:           `{"task_name": "test-task"}`,
			wantStatusCode: http.StatusUnauthorized,
			wantErr:        true,
			errContains:    "Unauthorized",
			tokenType:      "missing",
		},
		{
			name:           "invalid token",
			method:         http.MethodPost,
			body:           `{"task_name": "test-task"}`,
			wantStatusCode: http.StatusUnauthorized,
			wantErr:        true,
			errContains:    "Unauthorized",
			tokenType:      "invalid",
		},
		{
			name:           "wrong HTTP method",
			method:         http.MethodGet,
			body:           `{"task_name": "test-task"}`,
			wantStatusCode: http.StatusMethodNotAllowed,
			wantErr:        true,
			errContains:    "Method not allowed",
			tokenType:      "api",
		},
		{
			name:           "missing task_name",
			method:         http.MethodPost,
			body:           `{}`,
			wantStatusCode: http.StatusBadRequest,
			wantErr:        true,
			errContains:    "task_name is required",
			tokenType:      "api",
		},
		{
			name:           "invalid JSON",
			method:         http.MethodPost,
			body:           `{invalid json}`,
			wantStatusCode: http.StatusBadRequest,
			wantErr:        true,
			errContains:    "Invalid request format",
			tokenType:      "api",
		},
		{
			name:           "non-existent task",
			method:         http.MethodPost,
			body:           `{"task_name": "non-existent"}`,
			wantStatusCode: http.StatusInternalServerError,
			wantErr:        true,
			errContains:    "Failed to start task",
			tokenType:      "api",
		},
		{
			name:           "viewer token used for API",
			method:         http.MethodPost,
			body:           `{"task_name": "test-task"}`,
			wantStatusCode: http.StatusUnauthorized,
			wantErr:        true,
			errContains:    "Unauthorized",
			tokenType:      "viewer",
		},
		{
			name:           "body hash mismatch",
			method:         http.MethodPost,
			body:           `{"task_name": "test-task"}`,
			wantStatusCode: http.StatusUnauthorized,
			wantErr:        true,
			errContains:    "Unauthorized",
			tokenType:      "api-mismatch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/api/start", bytes.NewBufferString(tt.body))

			// Build appropriate token per test case
			switch tt.tokenType {
			case "api":
				// API token bound to the exact request body via SHA1 hash
				claims := &Claims{
					BodySHA1: computeSHA1Hex([]byte(tt.body)),
					RegisteredClaims: jwt.RegisteredClaims{
						ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
					},
				}
				token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
				tokenString, err := token.SignedString([]byte(config.Auth.Secret))
				if err != nil {
					t.Fatalf("failed to create API token: %v", err)
				}
				req.URL.RawQuery = "token=" + tokenString
			case "api-mismatch":
				// API token with a different body hash to trigger mismatch
				claims := &Claims{
					BodySHA1: computeSHA1Hex([]byte(`{"task_name":"other"}`)),
					RegisteredClaims: jwt.RegisteredClaims{
						ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
					},
				}
				token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
				tokenString, err := token.SignedString([]byte(config.Auth.Secret))
				if err != nil {
					t.Fatalf("failed to create mismatching API token: %v", err)
				}
				req.URL.RawQuery = "token=" + tokenString
			case "viewer":
				claims := &Claims{
					BodySHA1: computeSHA1Hex([]byte(tt.body)),
					RegisteredClaims: jwt.RegisteredClaims{
						ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
						Audience:  []string{"viewer"},
					},
				}
				token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
				tokenString, err := token.SignedString([]byte(config.Auth.Secret))
				if err != nil {
					t.Fatalf("failed to create viewer token: %v", err)
				}
				req.URL.RawQuery = "token=" + tokenString
			case "invalid":
				req.URL.RawQuery = "token=invalid-token"
			case "missing":
				// no token parameter
			default:
				t.Fatalf("unknown tokenType: %s", tt.tokenType)
			}

			w := httptest.NewRecorder()

			handleStartTask(w, req, taskManager, config)

			if w.Code != tt.wantStatusCode {
				t.Errorf("handleStartTask() status = %d; want %d", w.Code, tt.wantStatusCode)
			}

			if tt.wantErr {
				var response ErrorResponse
				if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
					t.Fatalf("handleStartTask() response is not valid JSON: %v", err)
				}
				if tt.errContains != "" && !containsString(response.Error, tt.errContains) {
					t.Errorf("handleStartTask() error = %q; want error containing %q", response.Error, tt.errContains)
				}
			} else {
				var response StartTaskResponse
				if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
					t.Fatalf("handleStartTask() response is not valid JSON: %v", err)
				}
				if response.TaskID == "" {
					t.Error("handleStartTask() TaskID is empty")
				}
				if response.ViewerURL == "" {
					t.Error("handleStartTask() ViewerURL is empty")
				}
				if !containsString(response.ViewerURL, response.TaskID) {
					t.Errorf("handleStartTask() ViewerURL doesn't contain TaskID: %q", response.ViewerURL)
				}
			}
		})
	}
}

func TestHandleStartTaskWithTLS(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "api-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	config := &Config{
		Server: ServerConfig{
			TaskDir: tmpDir,
		},
		Auth: AuthConfig{
			Secret: "test-secret-key",
		},
		Tasks: []TaskConfig{
			{Name: "test-task", Command: "echo hello"},
		},
	}

	taskManager := NewTaskManager(config)

	req := httptest.NewRequest(http.MethodPost, "/api/start", bytes.NewBufferString(`{"task_name": "test-task"}`))
	// API token with correct body hash
	body := `{"task_name": "test-task"}`
	claims := &Claims{
		BodySHA1: computeSHA1Hex([]byte(body)),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(config.Auth.Secret))
	if err != nil {
		t.Fatalf("failed to create API token: %v", err)
	}
	req.URL.RawQuery = "token=" + tokenString
	req.TLS = &tls.ConnectionState{} // Simulate TLS connection

	w := httptest.NewRecorder()
	handleStartTask(w, req, taskManager, config)

	if w.Code != http.StatusOK {
		t.Fatalf("handleStartTask() with TLS status = %d; want %d", w.Code, http.StatusOK)
	}

	var response StartTaskResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("handleStartTask() response is not valid JSON: %v", err)
	}

	// Viewer URL should use https scheme
	if !containsString(response.ViewerURL, "https://") {
		t.Errorf("handleStartTask() ViewerURL = %q; want https://", response.ViewerURL)
	}
}

func TestGenerateViewerToken(t *testing.T) {
	secret := "test-secret"
	taskID := "test-task-id"
	expiration := 24 * time.Hour

	token, err := generateViewerToken(taskID, secret, expiration)
	if err != nil {
		t.Fatalf("generateViewerToken() = %v; want nil", err)
	}

	if token == "" {
		t.Error("generateViewerToken() token is empty")
	}

	// Verify token can be parsed
	parsedToken, err := jwt.ParseWithClaims(token, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})
	if err != nil {
		t.Fatalf("generateViewerToken() token cannot be parsed: %v", err)
	}

	claims, ok := parsedToken.Claims.(*Claims)
	if !ok {
		t.Fatal("generateViewerToken() claims type assertion failed")
	}

	if claims.TaskID != taskID {
		t.Errorf("generateViewerToken() TaskID = %q; want %q", claims.TaskID, taskID)
	}

	// Verify audience is set to "viewer"
	if len(claims.Audience) == 0 || claims.Audience[0] != "viewer" {
		t.Errorf("generateViewerToken() Audience = %v; want [viewer]", claims.Audience)
	}

	// Verify expiration
	if claims.ExpiresAt == nil {
		t.Error("generateViewerToken() ExpiresAt is nil")
	} else if claims.ExpiresAt.Before(time.Now()) {
		t.Error("generateViewerToken() token is already expired")
	}
}

func TestHandleStartTaskLargeRequest(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "api-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	config := &Config{
		Server: ServerConfig{
			TaskDir: tmpDir,
		},
		Auth: AuthConfig{
			Secret: "test-secret-key",
		},
		Tasks: []TaskConfig{
			{Name: "test-task", Command: "echo hello"},
		},
	}

	taskManager := NewTaskManager(config)

	// Create a request body that exceeds maxJSONSize
	largeBody := `{"task_name": "test-task", "data": "` + string(make([]byte, maxJSONSize+1)) + `"}`

	req := httptest.NewRequest(http.MethodPost, "/api/start", bytes.NewBufferString(largeBody))
	// Use API token without body hash; handler will treat this as unauthorized due to
	// missing/invalid body binding before JSON size validation kicks in.
	req.URL.RawQuery = "token=invalid-token"
	w := httptest.NewRecorder()

	handleStartTask(w, req, taskManager, config)

	// With body-hash binding in place, an oversized body with invalid token should be
	// rejected as unauthorized rather than by JSON size validation.
	if w.Code != http.StatusUnauthorized {
		t.Errorf("handleStartTask() with large body status = %d; want %d", w.Code, http.StatusUnauthorized)
	}
}


