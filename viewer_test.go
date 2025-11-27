package main

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestHandleViewer(t *testing.T) {
	// Setup test environment
	tmpDir, err := os.MkdirTemp("", "viewer-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	htmlDir, err := os.MkdirTemp("", "html-test-*")
	if err != nil {
		t.Fatalf("Failed to create HTML temp dir: %v", err)
	}
	defer os.RemoveAll(htmlDir)

	// Create viewer.html
	viewerHTML := `<!DOCTYPE html>
<html>
<head><title>Viewer</title></head>
<body>
	<h1>Task Viewer</h1>
	<p>Task ID: {{.TaskID}}</p>
	<p>WebSocket: {{.WebSocketURL}}</p>
</body>
</html>`
	if err := os.WriteFile(filepath.Join(htmlDir, "viewer.html"), []byte(viewerHTML), 0644); err != nil {
		t.Fatalf("Failed to create viewer.html: %v", err)
	}

	// Create error pages
	for _, code := range []int{400, 401, 404, 405, 500} {
		errorHTML := `<html><body><h1>Error ` + strconv.Itoa(code) + `</h1></body></html>`
		filename := filepath.Join(htmlDir, strconv.Itoa(code)+".html")
		if err := os.WriteFile(filename, []byte(errorHTML), 0644); err != nil {
			t.Fatalf("Failed to create %d.html: %v", code, err)
		}
	}

	htmlCache, err := NewHTMLCache(htmlDir)
	if err != nil {
		t.Fatalf("Failed to create HTML cache: %v", err)
	}

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

	// Create a test task
	taskID, err := taskManager.StartTask("test-task", map[string]interface{}{})
	if err != nil {
		t.Fatalf("Failed to start test task: %v", err)
	}

	tests := []struct {
		name           string
		token          string
		taskID         string
		wantStatusCode int
		wantErr        bool
		errContains    string
	}{
		{
			name:           "valid request with token and task_id",
			token:          createTestToken(t, config.Auth.Secret, "viewer", taskID, time.Hour),
			taskID:         taskID,
			wantStatusCode: http.StatusOK,
			wantErr:        false,
		},
		{
			name:           "valid request with token in claims",
			token:          createTestToken(t, config.Auth.Secret, "viewer", taskID, time.Hour),
			taskID:         "", // Will use taskID from token claims
			wantStatusCode: http.StatusOK,
			wantErr:        false,
		},
		{
			name:           "missing token",
			token:          "",
			taskID:         taskID,
			wantStatusCode: http.StatusUnauthorized,
			wantErr:        true,
		},
		{
			name:           "invalid token",
			token:          "invalid-token",
			taskID:         taskID,
			wantStatusCode: http.StatusUnauthorized,
			wantErr:        true,
		},
		{
			name:           "API token used for viewer",
			token:          createTestToken(t, config.Auth.Secret, "", taskID, time.Hour),
			taskID:         taskID,
			wantStatusCode: http.StatusUnauthorized,
			wantErr:        true,
			errContains:    "Unauthorized",
		},
		{
			name:           "missing task_id",
			token:          createTestToken(t, config.Auth.Secret, "viewer", "", time.Hour),
			taskID:         "",
			wantStatusCode: http.StatusBadRequest,
			wantErr:        true,
		},
		{
			name:           "non-existent task",
			token:          createTestToken(t, config.Auth.Secret, "viewer", "non-existent-task-id", time.Hour),
			taskID:         "non-existent-task-id",
			wantStatusCode: http.StatusNotFound,
			wantErr:        true,
		},
		{
			name:           "expired token",
			token:          createTestToken(t, config.Auth.Secret, "viewer", taskID, -time.Hour),
			taskID:         taskID,
			wantStatusCode: http.StatusUnauthorized,
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/viewer", nil)
			if tt.token != "" {
				req.URL.RawQuery = "token=" + tt.token
			}
			if tt.taskID != "" {
				if req.URL.RawQuery != "" {
					req.URL.RawQuery += "&"
				}
				req.URL.RawQuery += "task_id=" + tt.taskID
			}

			w := httptest.NewRecorder()
			handleViewer(w, req, taskManager, config, htmlCache)

			if w.Code != tt.wantStatusCode {
				t.Errorf("handleViewer() status = %d; want %d", w.Code, tt.wantStatusCode)
			}

			if !tt.wantErr {
				// Verify HTML response
				if w.Header().Get("Content-Type") != "text/html; charset=utf-8" {
					t.Errorf("handleViewer() Content-Type = %q; want %q", w.Header().Get("Content-Type"), "text/html; charset=utf-8")
				}
				body := w.Body.String()
				if body == "" {
					t.Error("handleViewer() body is empty")
				}
				if tt.taskID != "" && !containsStringHelper(body, tt.taskID) {
					t.Errorf("handleViewer() body doesn't contain task_id %q", tt.taskID)
				}
			}
		})
	}
}

func TestHandleViewerWithTLS(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "viewer-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	htmlDir, err := os.MkdirTemp("", "html-test-*")
	if err != nil {
		t.Fatalf("Failed to create HTML temp dir: %v", err)
	}
	defer os.RemoveAll(htmlDir)

	viewerHTML := `<!DOCTYPE html>
<html>
<head><title>Viewer</title></head>
<body>
	<h1>Task Viewer</h1>
	<p>Task ID: {{.TaskID}}</p>
	<p>WebSocket: {{.WebSocketURL}}</p>
</body>
</html>`
	if err := os.WriteFile(filepath.Join(htmlDir, "viewer.html"), []byte(viewerHTML), 0644); err != nil {
		t.Fatalf("Failed to create viewer.html: %v", err)
	}

	htmlCache, err := NewHTMLCache(htmlDir)
	if err != nil {
		t.Fatalf("Failed to create HTML cache: %v", err)
	}

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
	taskID, err := taskManager.StartTask("test-task", map[string]interface{}{})
	if err != nil {
		t.Fatalf("Failed to start test task: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/viewer", nil)
	req.URL.RawQuery = "token=" + createTestToken(t, config.Auth.Secret, "viewer", taskID, time.Hour) + "&task_id=" + taskID
	req.TLS = &tls.ConnectionState{} // Simulate TLS

	w := httptest.NewRecorder()
	handleViewer(w, req, taskManager, config, htmlCache)

	if w.Code != http.StatusOK {
		t.Fatalf("handleViewer() with TLS status = %d; want %d", w.Code, http.StatusOK)
	}

	// Verify WebSocket URL uses wss://
	body := w.Body.String()
	if !containsStringHelper(body, "wss://") {
		t.Errorf("handleViewer() with TLS body = %q; want to contain 'wss://'", body)
	}
}

// Helper function (createTestToken is in auth_test.go)
func containsStringHelper(s, substr string) bool {
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

