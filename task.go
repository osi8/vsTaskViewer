package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

// TaskManager manages task execution
type TaskManager struct {
	config     *Config
	runningTasks map[string]*RunningTask
	mu         sync.RWMutex
}

// RunningTask represents a currently running task
type RunningTask struct {
	ID        string
	TaskName  string
	StartTime time.Time
	OutputDir string
}

// NewTaskManager creates a new task manager
func NewTaskManager(config *Config) *TaskManager {
	return &TaskManager{
		config:       config,
		runningTasks: make(map[string]*RunningTask),
	}
}

// StartTask starts a predefined task using the `at` command
func (tm *TaskManager) StartTask(taskName string) (string, error) {
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

	// Generate unique task ID
	taskID := uuid.New().String()

	// Create output directory
	outputDir := filepath.Join("/tmp", taskID)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create output directory: %w", err)
	}

	stdoutPath := filepath.Join(outputDir, "stdout")
	stderrPath := filepath.Join(outputDir, "stderr")

	// Create wrapper script that redirects output to files
	// Write PID to file and use unbuffered output to ensure immediate writes
	pidPath := filepath.Join(outputDir, "pid")
	wrapperScript := fmt.Sprintf(`#!/bin/bash
set -e
echo $$ > %s
cd /tmp/%s
exec > %s 2> %s
%s
`, pidPath, taskID, stdoutPath, stderrPath, taskConfig.Command)

	scriptPath := filepath.Join(outputDir, "run.sh")
	if err := os.WriteFile(scriptPath, []byte(wrapperScript), 0755); err != nil {
		return "", fmt.Errorf("failed to create wrapper script: %w", err)
	}

	// Schedule task with `at` command using echo to pipe command
	atCmd := fmt.Sprintf("echo 'bash %s' | at now", scriptPath)
	cmd := exec.Command("sh", "-c", atCmd)
	
	if err := cmd.Run(); err != nil {
		log.Printf("[TASK] Failed to schedule task with at: %v", err)
		return "", fmt.Errorf("failed to schedule task with at: %w", err)
	}
	
	log.Printf("[TASK] Task scheduled: task_id=%s, task_name=%s, script=%s", taskID, taskName, scriptPath)

	// Register running task
	tm.mu.Lock()
	tm.runningTasks[taskID] = &RunningTask{
		ID:        taskID,
		TaskName:  taskName,
		StartTime: time.Now(),
		OutputDir: outputDir,
	}
	tm.mu.Unlock()

	return taskID, nil
}

// GetTask returns information about a running task
func (tm *TaskManager) GetTask(taskID string) (*RunningTask, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	task, ok := tm.runningTasks[taskID]
	if !ok {
		return nil, fmt.Errorf("task '%s' not found", taskID)
	}

	return task, nil
}

