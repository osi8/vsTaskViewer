package main

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestReadPID(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "websocket-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	tests := []struct {
		name     string
		pidValue string
		want     int
	}{
		{
			name:     "valid PID",
			pidValue: "12345",
			want:     12345,
		},
		{
			name:     "PID with whitespace",
			pidValue: "  67890  \n",
			want:     67890,
		},
		{
			name:     "empty file",
			pidValue: "",
			want:     0,
		},
		{
			name:     "invalid PID",
			pidValue: "not-a-number",
			want:     0,
		},
		{
			name:     "zero PID",
			pidValue: "0",
			want:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pidPath := filepath.Join(tmpDir, "pid-"+tt.name)
			if tt.pidValue != "" {
				if err := os.WriteFile(pidPath, []byte(tt.pidValue), 0644); err != nil {
					t.Fatalf("Failed to write PID file: %v", err)
				}
			}

			got := readPID(pidPath)
			if got != tt.want {
				t.Errorf("readPID(%q) = %d; want %d", pidPath, got, tt.want)
			}
		})
	}
}

func TestReadPIDNonExistentFile(t *testing.T) {
	got := readPID("/nonexistent/pid/file")
	if got != 0 {
		t.Errorf("readPID(nonexistent) = %d; want 0", got)
	}
}

func TestReadExitCode(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "websocket-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	tests := []struct {
		name       string
		exitCode   string
		want       int
	}{
		{
			name:     "success exit code",
			exitCode: "0",
			want:     0,
		},
		{
			name:     "error exit code",
			exitCode: "1",
			want:     1,
		},
		{
			name:     "exit code with whitespace",
			exitCode: "  2  \n",
			want:     2,
		},
		{
			name:     "empty file",
			exitCode: "",
			want:     -1,
		},
		{
			name:     "invalid exit code",
			exitCode: "not-a-number",
			want:     -1,
		},
		{
			name:     "negative exit code",
			exitCode: "-1",
			want:     -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exitCodePath := filepath.Join(tmpDir, "exitcode-"+tt.name)
			if tt.exitCode != "" {
				if err := os.WriteFile(exitCodePath, []byte(tt.exitCode), 0644); err != nil {
					t.Fatalf("Failed to write exit code file: %v", err)
				}
			}

			got := readExitCode(exitCodePath)
			if got != tt.want {
				t.Errorf("readExitCode(%q) = %d; want %d", exitCodePath, got, tt.want)
			}
		})
	}
}

func TestReadExitCodeNonExistentFile(t *testing.T) {
	got := readExitCode("/nonexistent/exitcode/file")
	if got != -1 {
		t.Errorf("readExitCode(nonexistent) = %d; want -1", got)
	}
}

func TestIsProcessRunning(t *testing.T) {
	// Test with current process (should be running)
	currentPID := os.Getpid()
	if !isProcessRunning(currentPID) {
		t.Errorf("isProcessRunning(%d) = false; want true (current process)", currentPID)
	}

	// Test with invalid PID (very high number, unlikely to exist)
	invalidPID := 999999999
	if isProcessRunning(invalidPID) {
		t.Errorf("isProcessRunning(%d) = true; want false (invalid PID)", invalidPID)
	}

	// Test with PID 1 (init process, should exist on Linux)
	if !isProcessRunning(1) {
		t.Log("isProcessRunning(1) = false; init process may not exist in test environment")
	}
}

func TestCreateUpgrader(t *testing.T) {
	tests := []struct {
		name           string
		allowedOrigins []string
		wantAllowAll   bool
	}{
		{
			name:           "empty origins - allow all",
			allowedOrigins: []string{},
			wantAllowAll:   true,
		},
		{
			name:           "nil origins - allow all",
			allowedOrigins: nil,
			wantAllowAll:   true,
		},
		{
			name:           "single origin",
			allowedOrigins: []string{"http://localhost:8080"},
			wantAllowAll:   false,
		},
		{
			name:           "multiple origins",
			allowedOrigins: []string{"http://localhost:8080", "https://example.com"},
			wantAllowAll:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upgrader := createUpgrader(tt.allowedOrigins)

		// Test CheckOrigin function
		req := &http.Request{
				Header: make(http.Header),
			}

			// Test with no origin (should be allowed if allowAll)
			req.Header.Set("Origin", "")
			result := upgrader.CheckOrigin(req)
			if tt.wantAllowAll && !result {
				t.Errorf("createUpgrader() CheckOrigin(no origin) = false; want true (allow all)")
			}

			// Test with matching origin
			if len(tt.allowedOrigins) > 0 {
				req.Header.Set("Origin", tt.allowedOrigins[0])
				result = upgrader.CheckOrigin(req)
				if !result {
					t.Errorf("createUpgrader() CheckOrigin(matching origin) = false; want true")
				}

				// Test with non-matching origin
				req.Header.Set("Origin", "http://evil.com")
				result = upgrader.CheckOrigin(req)
				if result {
					t.Errorf("createUpgrader() CheckOrigin(non-matching origin) = true; want false")
				}
			}
		})
	}
}

func TestSendSystemMessage(t *testing.T) {
	// Note: sendSystemMessage requires a real WebSocket connection
	// For unit testing, we skip this test as it would panic with nil connection
	// Full testing would require integration tests with real WebSocket connections
	// This function is tested indirectly through integration tests
	t.Skip("sendSystemMessage requires real WebSocket connection - tested via integration tests")
}

