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
		token          string
		body           string
		wantStatusCode int
		wantErr        bool
		errContains    string
	}{
		{
			name:           "valid request",
			method:         http.MethodPost,
			token:          createTestToken(t, config.Auth.Secret, "", "", time.Hour),
			body:           `{"task_name": "test-task"}`,
			wantStatusCode: http.StatusOK,
			wantErr:        false,
		},
		{
			name:           "valid request with parameters",
			method:         http.MethodPost,
			token:          createTestToken(t, config.Auth.Secret, "", "", time.Hour),
			body:           `{"task_name": "param-task", "parameters": {"message": "hello"}}`,
			wantStatusCode: http.StatusOK,
			wantErr:        false,
		},
		{
			name:           "missing token",
			method:         http.MethodPost,
			token:          "",
			body:           `{"task_name": "test-task"}`,
			wantStatusCode: http.StatusUnauthorized,
			wantErr:        true,
			errContains:    "Unauthorized",
		},
		{
			name:           "invalid token",
			method:         http.MethodPost,
			token:          "invalid-token",
			body:           `{"task_name": "test-task"}`,
			wantStatusCode: http.StatusUnauthorized,
			wantErr:        true,
			errContains:    "Unauthorized",
		},
		{
			name:           "wrong HTTP method",
			method:         http.MethodGet,
			token:          createTestToken(t, config.Auth.Secret, "", "", time.Hour),
			body:           `{"task_name": "test-task"}`,
			wantStatusCode: http.StatusMethodNotAllowed,
			wantErr:        true,
			errContains:    "Method not allowed",
		},
		{
			name:           "missing task_name",
			method:         http.MethodPost,
			token:          createTestToken(t, config.Auth.Secret, "", "", time.Hour),
			body:           `{}`,
			wantStatusCode: http.StatusBadRequest,
			wantErr:        true,
			errContains:    "task_name is required",
		},
		{
			name:           "invalid JSON",
			method:         http.MethodPost,
			token:          createTestToken(t, config.Auth.Secret, "", "", time.Hour),
			body:           `{invalid json}`,
			wantStatusCode: http.StatusBadRequest,
			wantErr:        true,
			errContains:    "Invalid request format",
		},
		{
			name:           "non-existent task",
			method:         http.MethodPost,
			token:          createTestToken(t, config.Auth.Secret, "", "", time.Hour),
			body:           `{"task_name": "non-existent"}`,
			wantStatusCode: http.StatusInternalServerError,
			wantErr:        true,
			errContains:    "Failed to start task",
		},
		{
			name:           "viewer token used for API",
			method:         http.MethodPost,
			token:          createTestToken(t, config.Auth.Secret, "viewer", "", time.Hour),
			body:           `{"task_name": "test-task"}`,
			wantStatusCode: http.StatusUnauthorized,
			wantErr:        true,
			errContains:    "Unauthorized",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/api/start", bytes.NewBufferString(tt.body))
			if tt.token != "" {
				req.URL.RawQuery = "token=" + tt.token
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
	req.URL.RawQuery = "token=" + createTestToken(t, config.Auth.Secret, "", "", time.Hour)
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
	req.URL.RawQuery = "token=" + createTestToken(t, config.Auth.Secret, "", "", time.Hour)
	w := httptest.NewRecorder()

	handleStartTask(w, req, taskManager, config)

	// Should return bad request due to size limit
	if w.Code != http.StatusBadRequest {
		t.Errorf("handleStartTask() with large body status = %d; want %d", w.Code, http.StatusBadRequest)
	}
}


