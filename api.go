package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// StartTaskRequest represents a request to start a task
type StartTaskRequest struct {
	TaskName string `json:"task_name"`
}

// StartTaskResponse represents the response when starting a task
type StartTaskResponse struct {
	TaskID    string `json:"task_id"`
	ViewerURL string `json:"viewer_url"`
}

// handleStartTask handles requests to start a task
func handleStartTask(w http.ResponseWriter, r *http.Request, taskManager *TaskManager, config *Config) {
	// Authenticate request
	_, err := validateJWT(r, config.Auth.Secret)
	if err != nil {
		http.Error(w, fmt.Sprintf("Unauthorized: %v", err), http.StatusUnauthorized)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req StartTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	if req.TaskName == "" {
		http.Error(w, "task_name is required", http.StatusBadRequest)
		return
	}

	// Start the task
	taskID, err := taskManager.StartTask(req.TaskName)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to start task: %v", err), http.StatusInternalServerError)
		return
	}

	// Generate JWT token for viewer access
	viewerToken, err := generateViewerToken(taskID, config.Auth.Secret, 24*time.Hour)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to generate viewer token: %v", err), http.StatusInternalServerError)
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
func generateViewerToken(taskID, secret string, expiration time.Duration) (string, error) {
	claims := &Claims{
		TaskID: taskID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiration)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

