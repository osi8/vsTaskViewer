package main

import (
	"encoding/json"
	"io"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

const (
	maxJSONSize      = 1024 * 1024 // 1MB max JSON request size
	maxTaskNameLength = 100
)

var (
	taskNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
)

// validateTaskName validates a task name
func validateTaskName(name string) error {
	if name == "" {
		return ErrEmptyTaskName
	}
	if len(name) > maxTaskNameLength {
		return ErrTaskNameTooLong
	}
	if !taskNameRegex.MatchString(name) {
		return ErrInvalidTaskName
	}
	return nil
}

// validateTaskID validates a task ID (must be UUID)
func validateTaskID(taskID string) bool {
	_, err := uuid.Parse(taskID)
	return err == nil
}

// escapeBashCommand escapes a command for safe use in bash script
// This prevents command injection even if config is compromised
func escapeBashCommand(cmd string) string {
	// Replace single quotes with '\''
	escaped := strings.ReplaceAll(cmd, "'", "'\\''")
	// Wrap in single quotes
	return "'" + escaped + "'"
}

// decodeJSONRequest safely decodes JSON with size limit
func decodeJSONRequest(r io.Reader, v interface{}, maxSize int64) error {
	limitedReader := io.LimitReader(r, maxSize)
	decoder := json.NewDecoder(limitedReader)
	return decoder.Decode(v)
}

