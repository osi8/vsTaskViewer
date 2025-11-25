package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins in this implementation
	},
}

// WebSocketMessage represents a message sent over WebSocket
type WebSocketMessage struct {
	Type string `json:"type"`
	Data string `json:"data"`
}

// SystemMessage represents a system message (connection status, PID, etc.)
type SystemMessage struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	PID     int    `json:"pid,omitempty"`
}

// safeConn wraps a websocket connection with a mutex for thread-safe writes
type safeConn struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (sc *safeConn) WriteMessage(messageType int, data []byte) error {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	return sc.conn.WriteMessage(messageType, data)
}

// handleWebSocket handles WebSocket connections for live task output
func handleWebSocket(w http.ResponseWriter, r *http.Request, taskManager *TaskManager, config *Config) {
	log.Printf("[WEBSOCKET] Connection attempt from %s", r.RemoteAddr)
	
	// Authenticate request
	claims, err := validateJWT(r, config.Auth.Secret)
	if err != nil {
		log.Printf("[WEBSOCKET] Authentication failed: %v", err)
		http.Error(w, fmt.Sprintf("Unauthorized: %v", err), http.StatusUnauthorized)
		return
	}

	taskID := r.URL.Query().Get("task_id")
	if taskID == "" {
		taskID = claims.TaskID
	}

	if taskID == "" {
		log.Printf("[WEBSOCKET] Missing task_id")
		http.Error(w, "task_id is required", http.StatusBadRequest)
		return
	}

	// Get task information
	task, err := taskManager.GetTask(taskID)
	if err != nil {
		log.Printf("[WEBSOCKET] Task not found: task_id=%s, error=%v", taskID, err)
		http.Error(w, fmt.Sprintf("Task not found: %v", err), http.StatusNotFound)
		return
	}

	// Upgrade connection to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WEBSOCKET] Failed to upgrade connection: %v", err)
		http.Error(w, fmt.Sprintf("Failed to upgrade connection: %v", err), http.StatusInternalServerError)
		return
	}
	defer conn.Close()

	log.Printf("[WEBSOCKET] Socket connected: task_id=%s", taskID)

	// Wrap connection for thread-safe writes
	safeConn := &safeConn{conn: conn}

	// Paths to output files
	stdoutPath := filepath.Join(task.OutputDir, "stdout")
	stderrPath := filepath.Join(task.OutputDir, "stderr")
	pidPath := filepath.Join(task.OutputDir, "pid")

	// Try to read PID and send initial message
	pid := readPID(pidPath)
	if pid > 0 {
		sendSystemMessage(safeConn, "connected", fmt.Sprintf("WebSocket connected. Process PID: %d", pid), pid)
		log.Printf("[WEBSOCKET] Sent initial message with PID=%d for task_id=%s", pid, taskID)
	} else {
		sendSystemMessage(safeConn, "connected", "WebSocket connected. Waiting for process to start...", 0)
		log.Printf("[WEBSOCKET] Sent initial message (no PID yet) for task_id=%s", taskID)
	}

	// Start tailing stdout and stderr
	ctx := r.Context()
	go tailFile(ctx, safeConn, stdoutPath, "stdout", taskID)
	go tailFile(ctx, safeConn, stderrPath, "stderr", taskID)

	// Keep connection alive and handle ping/pong
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	// Send periodic pings
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Handle incoming messages (for pong)
	go func() {
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			safeConn.mu.Lock()
			err := conn.WriteMessage(websocket.PingMessage, nil)
			safeConn.mu.Unlock()
			if err != nil {
				return
			}
		}
	}
}

// readPID reads the PID from the pid file
func readPID(pidPath string) int {
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return 0
	}
	pidStr := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return 0
	}
	return pid
}

// sendSystemMessage sends a system message over WebSocket
func sendSystemMessage(safeConn *safeConn, msgType, message string, pid int) {
	sysMsg := SystemMessage{
		Type:    "system",
		Message: message,
		PID:     pid,
	}
	if data, err := json.Marshal(sysMsg); err == nil {
		safeConn.WriteMessage(websocket.TextMessage, data)
	}
}

// tailFile tails a file and sends updates over WebSocket
func tailFile(ctx context.Context, safeConn *safeConn, filePath, outputType, taskID string) {
	log.Printf("[TAIL] Starting to tail file: %s (type=%s, task_id=%s)", filePath, outputType, taskID)
	// Wait for file to be created (up to 60 seconds)
	fileExists := false
	for i := 0; i < 60; i++ {
		select {
		case <-ctx.Done():
			log.Printf("[TAIL] Context cancelled while waiting for file: %s", filePath)
			return
		default:
		}
		if _, err := os.Stat(filePath); err == nil {
			fileExists = true
			log.Printf("[TAIL] File found: %s (after %d seconds)", filePath, i)
			break
		}
		time.Sleep(1 * time.Second)
	}

	if !fileExists {
		log.Printf("[TAIL] File not found after 60 seconds: %s", filePath)
		// File doesn't exist yet, send waiting message
		msg := WebSocketMessage{
			Type: outputType,
			Data: fmt.Sprintf("Waiting for output file...\n"),
		}
		if data, err := json.Marshal(msg); err == nil {
			safeConn.WriteMessage(websocket.TextMessage, data)
		}
		return
	}

	// Open file for reading
	file, err := os.Open(filePath)
	if err != nil {
		log.Printf("[TAIL] Failed to open file: %s, error: %v", filePath, err)
		return
	}
	defer file.Close()

	log.Printf("[TAIL] Reading existing content from: %s", filePath)
	
	// Read existing content first
	lineCount := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}
		lineCount++
		msg := WebSocketMessage{
			Type: outputType,
			Data: scanner.Text() + "\n",
		}
		if data, err := json.Marshal(msg); err == nil {
			if err := safeConn.WriteMessage(websocket.TextMessage, data); err != nil {
				log.Printf("[TAIL] Error sending message: %v", err)
				return
			}
		}
	}
	log.Printf("[TAIL] Sent %d existing lines from: %s", lineCount, filePath)

	// Get current position
	lastPos, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		return
	}

	// Tail the file by polling for new content
	ticker := time.NewTicker(200 * time.Millisecond) // Poll every 200ms
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Check if file still exists
			if _, err := os.Stat(filePath); os.IsNotExist(err) {
				continue
			}

			// Get current file size
			info, err := os.Stat(filePath)
			if err != nil {
				continue
			}

			currentSize := info.Size()

			// If file has grown, read new content
			if currentSize > lastPos {
				// Reopen file to read new content
				file.Close()
				file, err = os.Open(filePath)
				if err != nil {
					log.Printf("[TAIL] Failed to reopen file: %s, error: %v", filePath, err)
					continue
				}

				// Seek to last known position
				file.Seek(lastPos, io.SeekStart)

				// Read new lines
				newLineCount := 0
				scanner := bufio.NewScanner(file)
				for scanner.Scan() {
					select {
					case <-ctx.Done():
						file.Close()
						return
					default:
					}
					newLineCount++
					msg := WebSocketMessage{
						Type: outputType,
						Data: scanner.Text() + "\n",
					}
					if data, err := json.Marshal(msg); err == nil {
						if err := safeConn.WriteMessage(websocket.TextMessage, data); err != nil {
							log.Printf("[TAIL] Error sending new line: %v", err)
							file.Close()
							return
						}
					}
				}
				if newLineCount > 0 {
					log.Printf("[TAIL] Sent %d new lines from: %s", newLineCount, filePath)
				}

				// Update last position
				lastPos, _ = file.Seek(0, io.SeekEnd)
			}
		}
	}
}

