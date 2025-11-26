package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
)

// TaskManager manages task execution
type TaskManager struct {
	config       *Config
	runningTasks map[string]*RunningTask
	mu           sync.RWMutex
}

// RunningTask represents a currently running task
type RunningTask struct {
	ID               string
	TaskName         string
	StartTime        time.Time
	OutputDir        string
	MaxExecutionTime time.Duration // Maximum execution time (0 = no limit)
	Terminated       bool          // Whether SIGTERM has been sent
	Killed           bool          // Whether SIGKILL has been sent
}

// NewTaskManager creates a new task manager
func NewTaskManager(config *Config) *TaskManager {
	return &TaskManager{
		config:       config,
		runningTasks: make(map[string]*RunningTask),
	}
}

// StartTask starts a predefined task as a background process
func (tm *TaskManager) StartTask(taskName string, parameters map[string]interface{}) (string, error) {
	// Validate task name
	if err := validateTaskName(taskName); err != nil {
		return "", fmt.Errorf("invalid task name: %w", err)
	}

	// Find task in config
	var taskConfig *TaskConfig
	for i := range tm.config.Tasks {
		if tm.config.Tasks[i].Name == taskName {
			taskConfig = &tm.config.Tasks[i]
			break
		}
	}

	if taskConfig == nil {
		return "", fmt.Errorf("task '%s' not found in configuration", taskName)
	}

	// Validate and process parameters
	validatedParams, err := validateAndProcessParameters(taskConfig.Parameters, parameters)
	if err != nil {
		return "", fmt.Errorf("parameter validation failed: %w", err)
	}

	// Substitute parameters in command
	command := substituteParameters(taskConfig.Command, validatedParams)

	// Generate unique task ID
	taskID := uuid.New().String()

	// Create output directory with restrictive permissions (0700)
	outputDir := filepath.Join(tm.config.Server.TaskDir, taskID)
	if err := os.MkdirAll(outputDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create output directory: %w", err)
	}

	stdoutPath := filepath.Join(outputDir, "stdout")
	stderrPath := filepath.Join(outputDir, "stderr")

	// Create wrapper script that redirects output to files
	// Write PID to file, capture exit code, and use unbuffered output
	// Escape command to prevent injection even if config is compromised
	pidPath := filepath.Join(outputDir, "pid")
	exitCodePath := filepath.Join(outputDir, "exitcode")
	escapedCommand := escapeBashCommand(command)
	escapedOutputDir := escapeBashCommand(outputDir)
	wrapperScript := fmt.Sprintf(`#!/bin/bash
set +e
echo $$ > %s
cd %s
exec > %s 2> %s
bash -c %s
EXIT_CODE=$?
echo $EXIT_CODE > %s
exit $EXIT_CODE
`, pidPath, escapedOutputDir, stdoutPath, stderrPath, escapedCommand, exitCodePath)

	scriptPath := filepath.Join(outputDir, "run.sh")
	// Use 0700 permissions (owner only) instead of 0755
	if err := os.WriteFile(scriptPath, []byte(wrapperScript), 0700); err != nil {
		return "", fmt.Errorf("failed to create wrapper script: %w", err)
	}

	// Start task process directly (replaces `at` command)
	// This works without elevated privileges
	cmd := exec.Command("bash", scriptPath)

	// Set up process attributes for background execution
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true, // Create new session to detach from terminal
	}

	// Redirect stdin to /dev/null to detach from terminal
	stdinFile, err := os.OpenFile("/dev/null", os.O_RDONLY, 0)
	if err != nil {
		return "", fmt.Errorf("failed to open /dev/null: %w", err)
	}
	cmd.Stdin = stdinFile

	// Start the process
	if err := cmd.Start(); err != nil {
		stdinFile.Close()
		log.Printf("[TASK] Failed to start task process: %v", err)
		return "", fmt.Errorf("failed to start task process: %w", err)
	}
	// Close stdin file after process has started (command has its own fd)
	stdinFile.Close()

	// Write PID immediately (the script will also write it, but this ensures it's there)
	pid := cmd.Process.Pid
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(pid)), 0600); err != nil {
		log.Printf("[TASK] Warning: failed to write PID file: %v", err)
	}

	// Don't wait for the process - let it run in background
	// The process will write its own PID and exit code when done
	go func() {
		// Wait for process to complete (in background goroutine)
		// This prevents zombie processes
		cmd.Wait()
	}()

	log.Printf("[TASK] Task started: task_id=%s, task_name=%s, pid=%d, script=%s", taskID, taskName, pid, scriptPath)

	// Calculate max execution time
	var maxExecTime time.Duration
	if taskConfig.MaxExecutionTime > 0 {
		maxExecTime = time.Duration(taskConfig.MaxExecutionTime) * time.Second
	}

	// Register running task
	tm.mu.Lock()
	tm.runningTasks[taskID] = &RunningTask{
		ID:               taskID,
		TaskName:         taskName,
		StartTime:        time.Now(),
		OutputDir:        outputDir,
		MaxExecutionTime: maxExecTime,
		Terminated:       false,
		Killed:           false,
	}
	tm.mu.Unlock()

	return taskID, nil
}

// GetTask returns information about a running task
func (tm *TaskManager) GetTask(taskID string) (*RunningTask, error) {
	// Validate task ID format (must be UUID)
	if !validateTaskID(taskID) {
		return nil, fmt.Errorf("invalid task ID format")
	}

	tm.mu.RLock()
	defer tm.mu.RUnlock()

	task, ok := tm.runningTasks[taskID]
	if !ok {
		return nil, fmt.Errorf("task '%s' not found", taskID)
	}

	return task, nil
}

// GetAllTasks returns all running tasks (for cleanup on shutdown)
func (tm *TaskManager) GetAllTasks() []*RunningTask {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	tasks := make([]*RunningTask, 0, len(tm.runningTasks))
	for _, task := range tm.runningTasks {
		tasks = append(tasks, task)
	}
	return tasks
}

// CleanupAllTasks removes all task directories (for shutdown)
func (tm *TaskManager) CleanupAllTasks() {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	log.Printf("[TASK] Cleaning up %d task directories", len(tm.runningTasks))
	for taskID, task := range tm.runningTasks {
		if err := os.RemoveAll(task.OutputDir); err != nil {
			log.Printf("[TASK] Failed to cleanup directory %s (task_id=%s): %v", task.OutputDir, taskID, err)
		} else {
			log.Printf("[TASK] Cleaned up directory: %s (task_id=%s)", task.OutputDir, taskID)
		}
	}
}

// validateAndProcessParameters validates all parameters according to their definitions
// Returns a map of validated parameter values as strings
func validateAndProcessParameters(paramDefs []ParameterConfig, providedParams map[string]interface{}) (map[string]string, error) {
	validated := make(map[string]string)

	// If no parameters are defined, ensure none are provided
	if len(paramDefs) == 0 {
		if len(providedParams) > 0 {
			return nil, fmt.Errorf("task does not accept parameters, but %d parameter(s) were provided", len(providedParams))
		}
		return validated, nil
	}

	// Create a map of provided parameters for quick lookup
	providedMap := make(map[string]interface{})
	for k, v := range providedParams {
		providedMap[k] = v
	}

	// Validate each defined parameter
	for _, paramDef := range paramDefs {
		value, provided := providedMap[paramDef.Name]

		// Check if required parameter is missing
		if !paramDef.Optional && !provided {
			return nil, fmt.Errorf("required parameter '%s' (type %s) is missing", paramDef.Name, paramDef.Type)
		}

		// If optional and not provided, skip
		if paramDef.Optional && !provided {
			continue
		}

		// Validate the parameter value
		validatedValue, err := validateParameterValue(paramDef.Name, paramDef.Type, value)
		if err != nil {
			return nil, err
		}

		validated[paramDef.Name] = validatedValue
	}

	// Check for unknown parameters (parameters provided but not defined)
	for paramName := range providedMap {
		found := false
		for _, paramDef := range paramDefs {
			if paramDef.Name == paramName {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("unknown parameter '%s' provided (not defined in task configuration)", paramName)
		}
	}

	return validated, nil
}

// substituteParameters substitutes parameter placeholders in the command
// Placeholder format: {{param_name}}
func substituteParameters(command string, parameters map[string]string) string {
	result := command
	for paramName, paramValue := range parameters {
		placeholder := fmt.Sprintf("{{%s}}", paramName)
		result = strings.ReplaceAll(result, placeholder, paramValue)
	}
	return result
}
