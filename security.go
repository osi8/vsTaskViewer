package main

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

const (
	maxJSONSize       = 1024 * 1024 // 1MB max JSON request size
	maxTaskNameLength = 100
)

var (
	taskNameRegex    = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	intParamRegex    = regexp.MustCompile(`^[0-9]+$`)
	stringParamRegex = regexp.MustCompile(`^[-a-zA-Z0-9_:,\.]+$`)
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

// validateParameterValue validates a parameter value based on its type
// Returns the validated value as a string and an error if validation fails
func validateParameterValue(paramName, paramType string, value interface{}) (string, error) {
	// Convert value to string for validation
	var valueStr string
	switch v := value.(type) {
	case string:
		valueStr = v
	case float64:
		// JSON numbers are decoded as float64
		if paramType == "int" {
			// Check if it's a whole number
			if v != float64(int64(v)) {
				return "", fmt.Errorf("parameter '%s' must be an integer, got float: %v", paramName, v)
			}
			valueStr = strconv.FormatInt(int64(v), 10)
		} else {
			valueStr = strconv.FormatFloat(v, 'f', -1, 64)
		}
	case int:
		if paramType == "int" {
			valueStr = strconv.Itoa(v)
		} else {
			return "", fmt.Errorf("parameter '%s' must be of type '%s', got int", paramName, paramType)
		}
	case int64:
		if paramType == "int" {
			valueStr = strconv.FormatInt(v, 10)
		} else {
			return "", fmt.Errorf("parameter '%s' must be of type '%s', got int64", paramName, paramType)
		}
	default:
		return "", fmt.Errorf("parameter '%s' has unsupported type: %T", paramName, v)
	}

	// Validate based on type
	switch paramType {
	case "int":
		if !intParamRegex.MatchString(valueStr) {
			return "", fmt.Errorf("parameter '%s' (type int) contains invalid characters. Only digits 0-9 are allowed, got: %s", paramName, valueStr)
		}
		return valueStr, nil
	case "string":
		if !stringParamRegex.MatchString(valueStr) {
			return "", fmt.Errorf("parameter '%s' (type string) contains invalid characters. Only [-a-zA-Z0-9_:,.] are allowed, got: %s", paramName, valueStr)
		}
		return valueStr, nil
	default:
		return "", fmt.Errorf("parameter '%s' has unknown type: %s (must be 'int' or 'string')", paramName, paramType)
	}
}
