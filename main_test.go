package main

import (
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name          string
		configContent string
		wantErr       bool
		errContains   string
	}{
		{
			name: "valid config",
			configContent: `[server]
port = 8080

[auth]
secret = "test-secret"

[[tasks]]
name = "test-task"
command = "echo test"
`,
			wantErr: false,
		},
		{
			name: "missing auth secret",
			configContent: `[server]
port = 8080

[auth]

[[tasks]]
name = "test-task"
command = "echo test"
`,
			wantErr:     true,
			errContains: "auth.secret must be set",
		},
		{
			name: "no tasks",
			configContent: `[server]
port = 8080

[auth]
secret = "test-secret"
`,
			wantErr:     true,
			errContains: "at least one task must be defined",
		},
		{
			name: "task without name",
			configContent: `[server]
port = 8080

[auth]
secret = "test-secret"

[[tasks]]
command = "echo test"
`,
			wantErr:     true,
			errContains: "has no name",
		},
		{
			name: "task without command",
			configContent: `[server]
port = 8080

[auth]
secret = "test-secret"

[[tasks]]
name = "test-task"
`,
			wantErr:     true,
			errContains: "has no command",
		},
		{
			name: "task with invalid parameter type",
			configContent: `[server]
port = 8080

[auth]
secret = "test-secret"

[[tasks]]
name = "test-task"
command = "echo {{param}}"

[[tasks.parameters]]
name = "param"
type = "invalid"
`,
			wantErr:     true,
			errContains: "invalid type",
		},
		{
			name: "task with duplicate parameter names",
			configContent: `[server]
port = 8080

[auth]
secret = "test-secret"

[[tasks]]
name = "test-task"
command = "echo {{param}}"

[[tasks.parameters]]
name = "param"
type = "string"

[[tasks.parameters]]
name = "param"
type = "string"
`,
			wantErr:     true,
			errContains: "duplicate parameter name",
		},
		{
			name: "task with parameter without name",
			configContent: `[server]
port = 8080

[auth]
secret = "test-secret"

[[tasks]]
name = "test-task"
command = "echo {{param}}"

[[tasks.parameters]]
type = "string"
`,
			wantErr:     true,
			errContains: "has parameter at index",
		},
		{
			name: "valid config with parameters",
			configContent: `[server]
port = 8080

[auth]
secret = "test-secret"

[[tasks]]
name = "test-task"
command = "echo {{filename}}"

[[tasks.parameters]]
name = "filename"
type = "string"
optional = false

[[tasks.parameters]]
name = "timeout"
type = "int"
optional = true
`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary config file
			tmpFile, err := os.CreateTemp("", "test-config-*.toml")
			if err != nil {
				t.Fatalf("Failed to create temp file: %v", err)
			}
			defer os.Remove(tmpFile.Name())
			defer tmpFile.Close()

			if _, err := tmpFile.WriteString(tt.configContent); err != nil {
				t.Fatalf("Failed to write config: %v", err)
			}
			tmpFile.Close()

			config, err := loadConfig(tmpFile.Name())
			if tt.wantErr {
				if err == nil {
					t.Errorf("loadConfig() expected error but got none")
					return
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("loadConfig() error = %v, want error containing %q", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("loadConfig() error = %v, want no error", err)
					return
				}
				if config == nil {
					t.Errorf("loadConfig() returned nil config")
					return
				}
				if config.Auth.Secret == "" {
					t.Errorf("loadConfig() config has empty secret")
				}
				if len(config.Tasks) == 0 {
					t.Errorf("loadConfig() config has no tasks")
				}
			}
		})
	}
}

func TestGetBinaryDir(t *testing.T) {
	dir, err := getBinaryDir()
	if err != nil {
		t.Fatalf("getBinaryDir() error = %v", err)
	}
	if dir == "" {
		t.Errorf("getBinaryDir() returned empty string")
	}
	// Verify it's an absolute path
	if !filepath.IsAbs(dir) {
		t.Errorf("getBinaryDir() returned relative path: %s", dir)
	}
}

func TestFindConfigFile(t *testing.T) {
	// Test with flag path
	tmpFile, err := os.CreateTemp("", "test-config-*.toml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	path, err := findConfigFile(tmpFile.Name())
	if err != nil {
		t.Errorf("findConfigFile() with existing file error = %v", err)
	}
	if path != tmpFile.Name() {
		t.Errorf("findConfigFile() = %v, want %v", path, tmpFile.Name())
	}

	// Test with non-existent flag path
	_, err = findConfigFile("/nonexistent/config.toml")
	if err == nil {
		t.Errorf("findConfigFile() with non-existent file expected error")
	}

	// Test with empty flag path (will try to find in default locations)
	// This might fail if default locations don't exist, which is expected
	_, err = findConfigFile("")
	if err == nil {
		// If it succeeds, that's fine - means a config file exists in default location
		t.Logf("findConfigFile() found config in default location: %v", err)
	}
}

func TestFindTemplatesDir(t *testing.T) {
	// Create a temporary directory structure
	tmpDir, err := os.MkdirTemp("", "test-templates-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	htmlDir := filepath.Join(tmpDir, "html")
	if err := os.Mkdir(htmlDir, 0755); err != nil {
		t.Fatalf("Failed to create html dir: %v", err)
	}

	// This test is tricky because findTemplatesDir() looks in the binary directory
	// and /etc/vsTaskViewer/html/. We can't easily mock the binary directory,
	// so we'll just test that it returns an error when templates don't exist
	// in the expected locations (which is the common case in test environments)
	_, err = findTemplatesDir()
	if err == nil {
		// If it succeeds, that's fine - means templates exist in default location
		t.Logf("findTemplatesDir() found templates in default location")
	} else {
		// Expected error when templates don't exist
		if !contains(err.Error(), "templates directory not found") {
			t.Errorf("findTemplatesDir() unexpected error: %v", err)
		}
	}
}

func TestFindTaskDir(t *testing.T) {
	dir, err := findTaskDir()
	if err != nil {
		t.Errorf("findTaskDir() error = %v", err)
	}
	if dir == "" {
		t.Errorf("findTaskDir() returned empty string")
	}
	// Should return /var/vsTaskViewer
	if dir != "/var/vsTaskViewer" {
		t.Errorf("findTaskDir() = %v, want /var/vsTaskViewer", dir)
	}
}

func TestFindExecUser(t *testing.T) {
	user := findExecUser()
	if user == "" {
		t.Errorf("findExecUser() returned empty string")
	}
	if user != "www-data" {
		t.Errorf("findExecUser() = %v, want www-data", user)
	}
}

func TestLookupUser(t *testing.T) {
	// Test with current user (should exist)
	currentUser, err := user.Current()
	if err != nil {
		t.Skipf("Cannot get current user: %v", err)
	}

	uid, gid, err := lookupUser(currentUser.Username)
	if err != nil {
		t.Errorf("lookupUser() with current user error = %v", err)
	}
	if uid <= 0 {
		t.Errorf("lookupUser() returned invalid UID: %d", uid)
	}
	if gid <= 0 {
		t.Errorf("lookupUser() returned invalid GID: %d", gid)
	}

	// Test with non-existent user
	_, _, err = lookupUser("nonexistent-user-12345")
	if err == nil {
		t.Errorf("lookupUser() with non-existent user expected error")
	}
}

func TestValidateTaskDir(t *testing.T) {
	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "test-taskdir-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Set permissions to 0700
	if err := os.Chmod(tmpDir, 0700); err != nil {
		t.Fatalf("Failed to set permissions: %v", err)
	}

	// Should succeed with valid directory
	err = validateTaskDir(tmpDir)
	if err != nil {
		t.Errorf("validateTaskDir() with valid dir error = %v", err)
	}

	// Test with non-existent directory (should create it)
	nonExistentDir := filepath.Join(tmpDir, "new-dir")
	err = validateTaskDir(nonExistentDir)
	if err != nil {
		t.Errorf("validateTaskDir() with non-existent dir error = %v", err)
	}
	// Verify it was created
	if _, err := os.Stat(nonExistentDir); os.IsNotExist(err) {
		t.Errorf("validateTaskDir() did not create directory")
	}

	// Test with file instead of directory
	tmpFile, err := os.CreateTemp("", "test-file-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	err = validateTaskDir(tmpFile.Name())
	if err == nil {
		t.Errorf("validateTaskDir() with file expected error")
	}
	if !contains(err.Error(), "not a directory") {
		t.Errorf("validateTaskDir() error = %v, want error containing 'not a directory'", err)
	}
}

func TestPrepareTaskDir(t *testing.T) {
	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "test-prepare-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Get current user
	currentUser, err := user.Current()
	if err != nil {
		t.Skipf("Cannot get current user: %v", err)
	}

	// Test when not running as root (should skip preparation)
	err = prepareTaskDir(tmpDir, currentUser.Username)
	if err != nil {
		t.Errorf("prepareTaskDir() when not root error = %v", err)
	}

	// Test with non-existent user
	// When not running as root, prepareTaskDir skips preparation and returns nil
	// When running as root, it would fail on user lookup
	if os.Getuid() == 0 {
		err = prepareTaskDir(tmpDir, "nonexistent-user-12345")
		if err == nil {
			t.Errorf("prepareTaskDir() with non-existent user expected error when running as root")
		}
	} else {
		// When not root, it just skips and returns nil
		err = prepareTaskDir(tmpDir, "nonexistent-user-12345")
		if err != nil {
			t.Errorf("prepareTaskDir() when not root should skip and return nil, got error: %v", err)
		}
	}
}

func TestDropPrivileges(t *testing.T) {
	// Get current user
	currentUser, err := user.Current()
	if err != nil {
		t.Skipf("Cannot get current user: %v", err)
	}

	// Test when already running as target user (should succeed)
	err = dropPrivileges(currentUser.Username)
	if err != nil {
		t.Errorf("dropPrivileges() when already target user error = %v", err)
	}

	// Test when not running as root and trying to drop to different user
	// This should fail
	if os.Getuid() != 0 {
		err = dropPrivileges("www-data")
		if err == nil {
			t.Errorf("dropPrivileges() when not root expected error")
		}
		if !contains(err.Error(), "cannot drop privileges") {
			t.Errorf("dropPrivileges() error = %v, want error containing 'cannot drop privileges'", err)
		}
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
