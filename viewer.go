package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"
)

// handleViewer serves the HTML viewer page
func handleViewer(w http.ResponseWriter, r *http.Request, taskManager *TaskManager, config *Config, htmlCache *HTMLCache) {
	log.Printf("[VIEWER] Viewer accessed from %s", r.RemoteAddr)
	
	// Authenticate request - Viewer tokens must have audience="viewer"
	viewerAudience := "viewer"
	claims, err := validateJWT(r, config.Auth.Secret, &viewerAudience)
	if err != nil {
		log.Printf("[VIEWER] Authentication failed: %v", err)
		serveErrorHTML(w, http.StatusUnauthorized, htmlCache)
		return
	}

	taskID := r.URL.Query().Get("task_id")
	if taskID == "" {
		taskID = claims.TaskID
	}

	if taskID == "" {
		log.Printf("[VIEWER] Missing task_id")
		serveErrorHTML(w, http.StatusBadRequest, htmlCache)
		return
	}

	// Check if task exists BEFORE rendering viewer
	_, err = taskManager.GetTask(taskID)
	if err != nil {
		log.Printf("[VIEWER] Task not found: task_id=%s, error=%v", taskID, err)
		serveErrorHTML(w, http.StatusNotFound, htmlCache)
		return
	}
	
	log.Printf("[VIEWER] Serving viewer for task_id=%s", taskID)

	// Get token from query
	token := r.URL.Query().Get("token")
	if token == "" {
		serveErrorHTML(w, http.StatusBadRequest, htmlCache)
		return
	}

	// Build WebSocket URL
	scheme := "ws"
	if r.TLS != nil {
		scheme = "wss"
	}
	wsURL := fmt.Sprintf("%s://%s/ws?task_id=%s&token=%s", scheme, r.Host, taskID, token)

	// Load viewer HTML template from cache
	htmlTemplate, err := loadViewerHTML(htmlCache)
	if err != nil {
		log.Printf("[VIEWER] Failed to load viewer.html: %v", err)
		serveErrorHTML(w, http.StatusInternalServerError, htmlCache)
		return
	}

	// Replace template placeholders
	html := htmlTemplate
	html = strings.ReplaceAll(html, "{{.TaskID}}", taskID)
	html = strings.ReplaceAll(html, "{{.WebSocketURL}}", wsURL)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

