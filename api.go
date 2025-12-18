package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
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

// normalizeJSON normalizes JSON by parsing and re-encoding it in compact form.
// This ensures that semantically identical JSON produces the same hash regardless of:
// - Whitespace and line breaks
// - Key order (Go's json.Marshal automatically sorts map keys alphabetically)
// This normalization prevents hash mismatches when clients send JSON with different formatting.
func normalizeJSON(data []byte) ([]byte, error) {
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	normalized, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return normalized, nil
}

// computeSHA1Hex computes the SHA1 hash of the given data and returns it as a hex string.
// This is used to bind API tokens to a specific request body for integrity protection.
// The data is normalized JSON, so formatting differences (whitespace, line breaks) don't affect the hash.
func computeSHA1Hex(data []byte) string {
	h := sha1.New()
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
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
	claims, err := validateJWT(r, config.Auth.Secret, &apiAudience)
	if err != nil {
		log.Printf("[API] Authentication failed: %v", err)
		sendJSONError(w, http.StatusUnauthorized, fmt.Sprintf("Unauthorized: %v", err))
		return
	}

	if r.Method != http.MethodPost {
		sendJSONError(w, http.StatusMethodNotAllowed, "Method not allowed. Use POST.")
		return
	}

	// Read complete request body (with size limit) for integrity check and JSON decoding
	bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, maxJSONSize))
	if err != nil {
		log.Printf("[API] Failed to read request body: %v", err)
		sendJSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Normalize JSON to ensure formatting differences (whitespace, line breaks) don't affect the hash.
	// This allows clients to send JSON in any valid format while maintaining security.
	normalizedBody, err := normalizeJSON(bodyBytes)
	if err != nil {
		log.Printf("[API] Failed to normalize JSON body: %v", err)
		sendJSONError(w, http.StatusBadRequest, "Invalid JSON format")
		return
	}

	// Compute SHA1 hash of the normalized body and compare with JWT claim.
	// This binds the API token to the request payload and prevents body tampering,
	// while being tolerant of JSON formatting differences.
	bodyHash := computeSHA1Hex(normalizedBody)
	if claims.BodySHA1 == "" || claims.BodySHA1 != bodyHash {
		log.Printf("[API] Body hash mismatch: token_claim=%q, computed=%q", claims.BodySHA1, bodyHash)
		sendJSONError(w, http.StatusUnauthorized, "Unauthorized: request body does not match token")
		return
	}

	var req StartTaskRequest
	// Use limited reader to prevent memory exhaustion
	if err := decodeJSONRequest(bytes.NewReader(bodyBytes), &req, maxJSONSize); err != nil {
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

