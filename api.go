package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// StartTaskRequest represents a request to start a task
type StartTaskRequest struct {
	TaskName   string                 `json:"task_name"`
	Parameters map[string]interface{} `json:"parameters,omitempty"` // Optional parameters for the task
}

// StartTaskResponse represents the response when starting a task
type StartTaskResponse struct {
	TaskID    string `json:"task_id"`
	ViewerURL string `json:"viewer_url"`
}

// ErrorResponse represents an error response in JSON format
type ErrorResponse struct {
	Error string `json:"error"`
}

// sendJSONError sends a JSON error response
func sendJSONError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(ErrorResponse{Error: message})
}

// handleStartTask handles requests to start a task
func handleStartTask(w http.ResponseWriter, r *http.Request, taskManager *TaskManager, config *Config) {
	log.Printf("[API] Start task request from %s", r.RemoteAddr)
	
	// Authenticate request - API tokens should have no audience or empty audience
	apiAudience := ""
	_, err := validateJWT(r, config.Auth.Secret, &apiAudience)
	if err != nil {
		log.Printf("[API] Authentication failed: %v", err)
		sendJSONError(w, http.StatusUnauthorized, fmt.Sprintf("Unauthorized: %v", err))
		return
	}

	if r.Method != http.MethodPost {
		sendJSONError(w, http.StatusMethodNotAllowed, "Method not allowed. Use POST.")
		return
	}

	var req StartTaskRequest
	// Use limited reader to prevent memory exhaustion
	if err := decodeJSONRequest(r.Body, &req, maxJSONSize); err != nil {
		log.Printf("[API] Failed to decode request: %v", err)
		sendJSONError(w, http.StatusBadRequest, "Invalid request format")
		return
	}

	if req.TaskName == "" {
		sendJSONError(w, http.StatusBadRequest, "task_name is required")
		return
	}

	// Start the task with parameters
	taskID, err := taskManager.StartTask(req.TaskName, req.Parameters)
	if err != nil {
		log.Printf("[API] Failed to start task '%s': %v", req.TaskName, err)
		sendJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to start task: %v", err))
		return
	}
	
	log.Printf("[API] Task created: task_id=%s, task_name=%s", taskID, req.TaskName)

	// Generate JWT token for viewer access
	viewerToken, err := generateViewerToken(taskID, config.Auth.Secret, 24*time.Hour)
	if err != nil {
		sendJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to generate viewer token: %v", err))
		return
	}

	// Build viewer URL
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	viewerURL := fmt.Sprintf("%s://%s/viewer?task_id=%s&token=%s", scheme, r.Host, taskID, viewerToken)

	// Send response
	response := StartTaskResponse{
		TaskID:    taskID,
		ViewerURL: viewerURL,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// generateViewerToken generates a JWT token for viewer access
// The token includes AUD="viewer" to prevent its use for API requests
func generateViewerToken(taskID, secret string, expiration time.Duration) (string, error) {
	claims := &Claims{
		TaskID: taskID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiration)),
			Audience:  []string{"viewer"}, // Set audience to "viewer" to prevent API token reuse
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

