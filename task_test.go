package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestValidateAndProcessParameters(t *testing.T) {
	tests := []struct {
		name          string
		paramDefs     []ParameterConfig
		providedParams map[string]interface{}
		want          map[string]string
		wantErr       bool
		errContains   string
	}{
		{
			name:          "no parameters defined, none provided",
			paramDefs:     []ParameterConfig{},
			providedParams: map[string]interface{}{},
			want:          map[string]string{},
			wantErr:       false,
		},
		{
			name:          "no parameters defined, but provided",
			paramDefs:     []ParameterConfig{},
			providedParams: map[string]interface{}{"key": "value"},
			want:          nil,
			wantErr:       true,
			errContains:   "does not accept parameters",
		},
		{
			name: "required parameter provided",
			paramDefs: []ParameterConfig{
				{Name: "filename", Type: "string", Optional: false},
			},
			providedParams: map[string]interface{}{"filename": "test.txt"},
			want:          map[string]string{"filename": "test.txt"},
			wantErr:       false,
		},
		{
			name: "required parameter missing",
			paramDefs: []ParameterConfig{
				{Name: "filename", Type: "string", Optional: false},
			},
			providedParams: map[string]interface{}{},
			want:          nil,
			wantErr:       true,
			errContains:   "required parameter",
		},
		{
			name: "optional parameter provided",
			paramDefs: []ParameterConfig{
				{Name: "message", Type: "string", Optional: true},
			},
			providedParams: map[string]interface{}{"message": "hello"},
			want:          map[string]string{"message": "hello"},
			wantErr:       false,
		},
		{
			name: "optional parameter not provided",
			paramDefs: []ParameterConfig{
				{Name: "message", Type: "string", Optional: true},
			},
			providedParams: map[string]interface{}{},
			want:          map[string]string{},
			wantErr:       false,
		},
		{
			name: "multiple parameters all provided",
			paramDefs: []ParameterConfig{
				{Name: "filename", Type: "string", Optional: false},
				{Name: "timeout", Type: "int", Optional: false},
			},
			providedParams: map[string]interface{}{
				"filename": "test.txt",
				"timeout":  30,
			},
			want: map[string]string{
				"filename": "test.txt",
				"timeout":  "30",
			},
			wantErr: false,
		},
		{
			name: "unknown parameter provided",
			paramDefs: []ParameterConfig{
				{Name: "filename", Type: "string", Optional: false},
			},
			providedParams: map[string]interface{}{
				"filename": "test.txt",
				"unknown":  "value",
			},
			want:        nil,
			wantErr:     true,
			errContains: "unknown parameter",
		},
		{
			name: "invalid parameter value",
			paramDefs: []ParameterConfig{
				{Name: "timeout", Type: "int", Optional: false},
			},
			providedParams: map[string]interface{}{
				"timeout": "abc",
			},
			want:        nil,
			wantErr:     true,
			errContains: "invalid characters",
		},
		{
			name: "mixed required and optional",
			paramDefs: []ParameterConfig{
				{Name: "filename", Type: "string", Optional: false},
				{Name: "message", Type: "string", Optional: true},
			},
			providedParams: map[string]interface{}{
				"filename": "test.txt",
			},
			want: map[string]string{
				"filename": "test.txt",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateAndProcessParameters(tt.paramDefs, tt.providedParams)
			if tt.wantErr {
				if err == nil {
					t.Errorf("validateAndProcessParameters() = %v, nil; want error", got)
				} else if tt.errContains != "" && !containsString(err.Error(), tt.errContains) {
					t.Errorf("validateAndProcessParameters() error = %v; want error containing %q", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("validateAndProcessParameters() = %v, %v; want %v, nil", got, err, tt.want)
				} else if !mapsEqual(got, tt.want) {
					t.Errorf("validateAndProcessParameters() = %v; want %v", got, tt.want)
				}
			}
		})
	}
}

func TestSubstituteParameters(t *testing.T) {
	tests := []struct {
		name       string
		command    string
		parameters map[string]string
		want       string
	}{
		{
			name:       "no parameters",
			command:    "echo hello",
			parameters: map[string]string{},
			want:       "echo hello",
		},
		{
			name:       "single parameter",
			command:    "echo {{message}}",
			parameters: map[string]string{"message": "hello"},
			want:       "echo hello",
		},
		{
			name:       "multiple parameters",
			command:    "process {{filename}} with timeout {{timeout}}",
			parameters: map[string]string{
				"filename": "data.txt",
				"timeout":  "30",
			},
			want: "process data.txt with timeout 30",
		},
		{
			name:       "parameter appears multiple times",
			command:    "echo {{name}} and {{name}}",
			parameters: map[string]string{"name": "test"},
			want:       "echo test and test",
		},
		{
			name:       "parameter with special characters",
			command:    "echo {{value}}",
			parameters: map[string]string{"value": "test-value"},
			want:       "echo test-value",
		},
		{
			name:       "unsubstituted placeholder",
			command:    "echo {{missing}}",
			parameters: map[string]string{"other": "value"},
			want:       "echo {{missing}}",
		},
		{
			name:       "empty parameter value",
			command:    "echo {{value}}",
			parameters: map[string]string{"value": ""},
			want:       "echo ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := substituteParameters(tt.command, tt.parameters)
			if got != tt.want {
				t.Errorf("substituteParameters(%q, %v) = %q; want %q", tt.command, tt.parameters, got, tt.want)
			}
		})
	}
}

func TestNewTaskManager(t *testing.T) {
	config := &Config{
		Server: ServerConfig{
			TaskDir: "/tmp/test-tasks",
		},
		Tasks: []TaskConfig{
			{Name: "test-task", Command: "echo test"},
		},
	}

	tm := NewTaskManager(config)
	if tm == nil {
		t.Fatal("NewTaskManager() = nil; want non-nil")
	}
	if tm.config != config {
		t.Error("NewTaskManager() config mismatch")
	}
	if tm.runningTasks == nil {
		t.Error("NewTaskManager() runningTasks = nil; want non-nil")
	}
	if len(tm.runningTasks) != 0 {
		t.Errorf("NewTaskManager() runningTasks length = %d; want 0", len(tm.runningTasks))
	}
}

func TestTaskManagerGetTask(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "task-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	config := &Config{
		Server: ServerConfig{
			TaskDir: tmpDir,
		},
		Tasks: []TaskConfig{
			{Name: "test-task", Command: "echo test"},
		},
	}

	tm := NewTaskManager(config)

	// Test with invalid task ID format
	_, err = tm.GetTask("not-a-uuid")
	if err == nil {
		t.Error("TaskManager.GetTask() with invalid UUID = nil; want error")
	}

	// Test with non-existent task
	validUUID := "550e8400-e29b-41d4-a716-446655440000"
	_, err = tm.GetTask(validUUID)
	if err == nil {
		t.Error("TaskManager.GetTask() with non-existent task = nil; want error")
	}

	// Add a task manually for testing
	taskID := validUUID
	tm.mu.Lock()
	tm.runningTasks[taskID] = &RunningTask{
		ID:        taskID,
		TaskName:  "test-task",
		StartTime: time.Now(),
		OutputDir: filepath.Join(tmpDir, taskID),
	}
	tm.mu.Unlock()

	// Test getting existing task
	task, err := tm.GetTask(taskID)
	if err != nil {
		t.Errorf("TaskManager.GetTask() = %v; want nil", err)
	}
	if task == nil {
		t.Fatal("TaskManager.GetTask() = nil; want non-nil")
	}
	if task.ID != taskID {
		t.Errorf("TaskManager.GetTask() task.ID = %q; want %q", task.ID, taskID)
	}
	if task.TaskName != "test-task" {
		t.Errorf("TaskManager.GetTask() task.TaskName = %q; want %q", task.TaskName, "test-task")
	}
}

func TestTaskManagerGetAllTasks(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "task-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	config := &Config{
		Server: ServerConfig{
			TaskDir: tmpDir,
		},
		Tasks: []TaskConfig{},
	}

	tm := NewTaskManager(config)

	// Initially should be empty
	tasks := tm.GetAllTasks()
	if len(tasks) != 0 {
		t.Errorf("TaskManager.GetAllTasks() length = %d; want 0", len(tasks))
	}

	// Add some tasks
	tm.mu.Lock()
	tm.runningTasks["task1"] = &RunningTask{ID: "task1", TaskName: "task1"}
	tm.runningTasks["task2"] = &RunningTask{ID: "task2", TaskName: "task2"}
	tm.mu.Unlock()

	// Should return all tasks
	tasks = tm.GetAllTasks()
	if len(tasks) != 2 {
		t.Errorf("TaskManager.GetAllTasks() length = %d; want 2", len(tasks))
	}
}

func TestTaskManagerStartTaskValidation(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "task-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	config := &Config{
		Server: ServerConfig{
			TaskDir: tmpDir,
		},
		Tasks: []TaskConfig{
			{Name: "test-task", Command: "echo test"},
		},
	}

	tm := NewTaskManager(config)

	// Test invalid task name
	_, err = tm.StartTask("", map[string]interface{}{})
	if err == nil {
		t.Error("TaskManager.StartTask() with empty name = nil; want error")
	}

	// Test non-existent task
	_, err = tm.StartTask("non-existent", map[string]interface{}{})
	if err == nil {
		t.Error("TaskManager.StartTask() with non-existent task = nil; want error")
	}
	if !containsString(err.Error(), "not found") {
		t.Errorf("TaskManager.StartTask() error = %v; want error containing 'not found'", err)
	}
}

func TestTaskManagerStartTaskParameterValidation(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "task-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	config := &Config{
		Server: ServerConfig{
			TaskDir: tmpDir,
		},
		Tasks: []TaskConfig{
			{
				Name:    "param-task",
				Command: "echo {{filename}}",
				Parameters: []ParameterConfig{
					{Name: "filename", Type: "string", Optional: false},
				},
			},
		},
	}

	tm := NewTaskManager(config)

	// Test missing required parameter
	_, err = tm.StartTask("param-task", map[string]interface{}{})
	if err == nil {
		t.Error("TaskManager.StartTask() with missing parameter = nil; want error")
	}
	if !containsString(err.Error(), "required parameter") {
		t.Errorf("TaskManager.StartTask() error = %v; want error containing 'required parameter'", err)
	}

	// Test invalid parameter value
	_, err = tm.StartTask("param-task", map[string]interface{}{
		"filename": "file/name", // Contains invalid character
	})
	if err == nil {
		t.Error("TaskManager.StartTask() with invalid parameter = nil; want error")
	}
}

// Helper functions

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

