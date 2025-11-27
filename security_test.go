package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestValidateTaskName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr error
	}{
		{
			name:    "valid task name with letters",
			input:   "myTask",
			wantErr: nil,
		},
		{
			name:    "valid task name with numbers",
			input:   "task123",
			wantErr: nil,
		},
		{
			name:    "valid task name with underscore",
			input:   "my_task",
			wantErr: nil,
		},
		{
			name:    "valid task name with hyphen",
			input:   "my-task",
			wantErr: nil,
		},
		{
			name:    "valid task name mixed",
			input:   "task-123_test",
			wantErr: nil,
		},
		{
			name:    "empty task name",
			input:   "",
			wantErr: ErrEmptyTaskName,
		},
		{
			name:    "task name with space",
			input:   "my task",
			wantErr: ErrInvalidTaskName,
		},
		{
			name:    "task name with special characters",
			input:   "task@123",
			wantErr: ErrInvalidTaskName,
		},
		{
			name:    "task name with slash",
			input:   "task/123",
			wantErr: ErrInvalidTaskName,
		},
		{
			name:    "task name too long",
			input:   strings.Repeat("a", maxTaskNameLength+1),
			wantErr: ErrTaskNameTooLong,
		},
		{
			name:    "task name at max length",
			input:   strings.Repeat("a", maxTaskNameLength),
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTaskName(tt.input)
			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("validateTaskName(%q) = %v, want nil", tt.input, err)
				}
			} else {
				if err != tt.wantErr {
					t.Errorf("validateTaskName(%q) = %v, want %v", tt.input, err, tt.wantErr)
				}
			}
		})
	}
}

func TestValidateTaskID(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "valid UUID v4",
			input: "550e8400-e29b-41d4-a716-446655440000",
			want:  true,
		},
		{
			name:  "valid UUID v1",
			input: "6ba7b810-9dad-11d1-80b4-00c04fd430c8",
			want:  true,
		},
		{
			name:  "invalid UUID - too short",
			input: "550e8400",
			want:  false,
		},
		{
			name:  "invalid UUID - wrong format",
			input: "not-a-uuid",
			want:  false,
		},
		{
			name:  "invalid UUID - empty",
			input: "",
			want:  false,
		},
		{
			name:  "valid UUID without hyphens (uuid.Parse accepts this)",
			input: "550e8400e29b41d4a716446655440000",
			want:  true, // uuid.Parse actually accepts UUIDs without hyphens
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validateTaskID(tt.input)
			if got != tt.want {
				t.Errorf("validateTaskID(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestEscapeBashCommand(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple command",
			input: "echo hello",
			want:  "'echo hello'",
		},
		{
			name:  "command with single quote",
			input: "echo 'hello world'",
			want:  "'echo '\\''hello world'\\'''",
		},
		{
			name:  "command with multiple single quotes",
			input: "echo 'hello' and 'world'",
			want:  "'echo '\\''hello'\\'' and '\\''world'\\'''",
		},
		{
			name:  "empty command",
			input: "",
			want:  "''",
		},
		{
			name:  "command with special characters",
			input: "echo $PATH",
			want:  "'echo $PATH'",
		},
		{
			name:  "command with double quotes",
			input: `echo "hello"`,
			want:  `'echo "hello"'`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeBashCommand(tt.input)
			if got != tt.want {
				t.Errorf("escapeBashCommand(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestValidateParameterValue(t *testing.T) {
	tests := []struct {
		name      string
		paramName string
		paramType string
		value     interface{}
		want      string
		wantErr   bool
		errMsg    string
	}{
		// Valid int parameters
		{
			name:      "valid int as string",
			paramName: "timeout",
			paramType: "int",
			value:     "123",
			want:      "123",
			wantErr:   false,
		},
		{
			name:      "valid int as int",
			paramName: "timeout",
			paramType: "int",
			value:     123,
			want:      "123",
			wantErr:   false,
		},
		{
			name:      "valid int as int64",
			paramName: "timeout",
			paramType: "int",
			value:     int64(456),
			want:      "456",
			wantErr:   false,
		},
		{
			name:      "valid int as float64 (whole number)",
			paramName: "timeout",
			paramType: "int",
			value:     float64(789),
			want:      "789",
			wantErr:   false,
		},
		{
			name:      "valid int zero",
			paramName: "count",
			paramType: "int",
			value:     "0",
			want:      "0",
			wantErr:   false,
		},
		// Invalid int parameters
		{
			name:      "invalid int with letters",
			paramName: "timeout",
			paramType: "int",
			value:     "123abc",
			want:      "",
			wantErr:   true,
			errMsg:    "contains invalid characters",
		},
		{
			name:      "invalid int with special chars",
			paramName: "timeout",
			paramType: "int",
			value:     "12-3",
			want:      "",
			wantErr:   true,
			errMsg:    "contains invalid characters",
		},
		{
			name:      "invalid int as float64 (decimal)",
			paramName: "timeout",
			paramType: "int",
			value:     float64(123.45),
			want:      "",
			wantErr:   true,
			errMsg:    "must be an integer",
		},
		{
			name:      "invalid int type mismatch",
			paramName: "timeout",
			paramType: "string",
			value:     123,
			want:      "",
			wantErr:   true,
			errMsg:    "must be of type 'string'",
		},
		// Valid string parameters
		{
			name:      "valid string simple",
			paramName: "filename",
			paramType: "string",
			value:     "data.txt",
			want:      "data.txt",
			wantErr:   false,
		},
		{
			name:      "valid string with hyphen",
			paramName: "filename",
			paramType: "string",
			value:     "my-file.txt",
			want:      "my-file.txt",
			wantErr:   false,
		},
		{
			name:      "valid string with underscore",
			paramName: "filename",
			paramType: "string",
			value:     "my_file.txt",
			want:      "my_file.txt",
			wantErr:   false,
		},
		{
			name:      "valid string with colon",
			paramName: "filename",
			paramType: "string",
			value:     "file:name",
			want:      "file:name",
			wantErr:   false,
		},
		{
			name:      "valid string with comma",
			paramName: "filename",
			paramType: "string",
			value:     "file,name",
			want:      "file,name",
			wantErr:   false,
		},
		{
			name:      "valid string with dot",
			paramName: "filename",
			paramType: "string",
			value:     "file.name",
			want:      "file.name",
			wantErr:   false,
		},
		{
			name:      "valid string as float64",
			paramName: "filename",
			paramType: "string",
			value:     float64(123.45),
			want:      "123.45",
			wantErr:   false,
		},
		// Invalid string parameters
		{
			name:      "invalid string with space",
			paramName: "filename",
			paramType: "string",
			value:     "file name",
			want:      "",
			wantErr:   true,
			errMsg:    "contains invalid characters",
		},
		{
			name:      "invalid string with slash",
			paramName: "filename",
			paramType: "string",
			value:     "file/name",
			want:      "",
			wantErr:   true,
			errMsg:    "contains invalid characters",
		},
		{
			name:      "invalid string with special chars",
			paramName: "filename",
			paramType: "string",
			value:     "file@name",
			want:      "",
			wantErr:   true,
			errMsg:    "contains invalid characters",
		},
		// Invalid type
		{
			name:      "unsupported type bool",
			paramName: "flag",
			paramType: "int",
			value:     true,
			want:      "",
			wantErr:   true,
			errMsg:    "unsupported type",
		},
		{
			name:      "unsupported type slice",
			paramName: "items",
			paramType: "string",
			value:     []string{"a", "b"},
			want:      "",
			wantErr:   true,
			errMsg:    "unsupported type",
		},
		// Invalid param type
		{
			name:      "unknown parameter type",
			paramName: "value",
			paramType: "float",
			value:     "123",
			want:      "",
			wantErr:   true,
			errMsg:    "unknown type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateParameterValue(tt.paramName, tt.paramType, tt.value)
			if tt.wantErr {
				if err == nil {
					t.Errorf("validateParameterValue(%q, %q, %v) = %q, nil; want error", tt.paramName, tt.paramType, tt.value, got)
				} else if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("validateParameterValue(%q, %q, %v) error = %v, want error containing %q", tt.paramName, tt.paramType, tt.value, err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validateParameterValue(%q, %q, %v) = %q, %v; want %q, nil", tt.paramName, tt.paramType, tt.value, got, err, tt.want)
				} else if got != tt.want {
					t.Errorf("validateParameterValue(%q, %q, %v) = %q, nil; want %q", tt.paramName, tt.paramType, tt.value, got, tt.want)
				}
			}
		})
	}
}

func TestDecodeJSONRequest(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		maxSize int64
		wantErr bool
	}{
		{
			name:    "valid JSON within limit",
			json:    `{"task_name": "test", "parameters": {"key": "value"}}`,
			maxSize: 1024,
			wantErr: false,
		},
		{
			name:    "valid JSON at limit",
			json:    `{"task_name": "test"}`,
			maxSize: 30,
			wantErr: false,
		},
		{
			name:    "JSON exceeds limit",
			json:    `{"task_name": "test", "data": "` + strings.Repeat("x", 1000) + `"}`,
			maxSize: 50,
			wantErr: true,
		},
		{
			name:    "invalid JSON",
			json:    `{"task_name": "test"`,
			maxSize: 1024,
			wantErr: true,
		},
		{
			name:    "empty JSON",
			json:    ``,
			maxSize: 1024,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result map[string]interface{}
			reader := bytes.NewReader([]byte(tt.json))
			err := decodeJSONRequest(reader, &result, tt.maxSize)
			if tt.wantErr {
				if err == nil {
					t.Errorf("decodeJSONRequest() = nil; want error")
				}
			} else {
				if err != nil {
					t.Errorf("decodeJSONRequest() = %v; want nil", err)
				}
			}
		})
	}
}

func TestDecodeJSONRequestSizeLimit(t *testing.T) {
	// Test that the size limit is actually enforced
	largeJSON := `{"data": "` + strings.Repeat("x", 1000) + `"}`
	reader := bytes.NewReader([]byte(largeJSON))
	
	var result map[string]interface{}
	err := decodeJSONRequest(reader, &result, 100) // Small limit
	
	if err == nil {
		t.Error("decodeJSONRequest() with oversized input = nil; want error")
	}
	
	// Verify that only partial data was read
	if result["data"] != nil {
		data := result["data"].(string)
		if len(data) > 100 {
			t.Errorf("decodeJSONRequest() read more than limit: got %d bytes", len(data))
		}
	}
}

func TestDecodeJSONRequestWithStruct(t *testing.T) {
	type TestRequest struct {
		TaskName   string                 `json:"task_name"`
		Parameters map[string]interface{} `json:"parameters,omitempty"`
	}

	jsonStr := `{"task_name": "my-task", "parameters": {"key": "value", "num": 42}}`
	reader := bytes.NewReader([]byte(jsonStr))
	
	var req TestRequest
	err := decodeJSONRequest(reader, &req, maxJSONSize)
	
	if err != nil {
		t.Fatalf("decodeJSONRequest() = %v; want nil", err)
	}
	
	if req.TaskName != "my-task" {
		t.Errorf("req.TaskName = %q; want %q", req.TaskName, "my-task")
	}
	
	if req.Parameters == nil {
		t.Error("req.Parameters = nil; want map")
	}
	
	if req.Parameters["key"] != "value" {
		t.Errorf("req.Parameters[\"key\"] = %v; want %q", req.Parameters["key"], "value")
	}
	
	// JSON numbers are decoded as float64
	if req.Parameters["num"] != float64(42) {
		t.Errorf("req.Parameters[\"num\"] = %v; want %v", req.Parameters["num"], float64(42))
	}
}

func TestDecodeJSONRequestWithLargeReader(t *testing.T) {
	// Create a reader that's larger than the limit
	largeData := strings.Repeat("x", 2000)
	jsonStr := `{"data": "` + largeData + `"}`
	reader := bytes.NewReader([]byte(jsonStr))
	
	var result map[string]interface{}
	err := decodeJSONRequest(reader, &result, 100) // Limit to 100 bytes
	
	// Should get an error or truncated data
	if err == nil {
		// If no error, verify data was truncated
		if data, ok := result["data"].(string); ok {
			if len(data) > 100 {
				t.Errorf("decodeJSONRequest() read %d bytes; expected max 100", len(data))
			}
		}
	}
}

func TestDecodeJSONRequestWithNilReader(t *testing.T) {
	// This test verifies that passing nil reader causes an error or panic
	// In practice, this shouldn't happen, but we test it for completeness
	var result map[string]interface{}
	
	// Use defer recover to catch potential panic
	defer func() {
		if r := recover(); r != nil {
			// Panic is expected for nil reader
			t.Logf("decodeJSONRequest with nil reader panicked as expected: %v", r)
		}
	}()
	
	err := decodeJSONRequest(nil, &result, maxJSONSize)
	// If no panic, should return an error
	if err == nil {
		t.Error("decodeJSONRequest(nil, ...) = nil; want error or panic")
	}
}

func TestDecodeJSONRequestWithInvalidTarget(t *testing.T) {
	jsonStr := `{"task_name": "test"}`
	reader := bytes.NewReader([]byte(jsonStr))
	
	// Try to decode into a non-pointer
	var result map[string]interface{}
	err := decodeJSONRequest(reader, result, maxJSONSize)
	
	// This might succeed or fail depending on implementation
	// Just verify it doesn't panic
	_ = err
}

// Test that decodeJSONRequest properly handles io.LimitReader
func TestDecodeJSONRequestLimitReader(t *testing.T) {
	// Create JSON that's exactly at the limit
	exactSizeJSON := `{"data": "` + strings.Repeat("x", 50) + `"}`
	reader := bytes.NewReader([]byte(exactSizeJSON))
	
	var result map[string]interface{}
	err := decodeJSONRequest(reader, &result, 100)
	
	if err != nil {
		t.Errorf("decodeJSONRequest() with exact limit = %v; want nil", err)
	}
}

