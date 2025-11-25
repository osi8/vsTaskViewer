package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
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
	// Authenticate request
	claims, err := validateJWT(r, config.Auth.Secret)
	if err != nil {
		http.Error(w, fmt.Sprintf("Unauthorized: %v", err), http.StatusUnauthorized)
		return
	}

	taskID := r.URL.Query().Get("task_id")
	if taskID == "" {
		taskID = claims.TaskID
	}

	if taskID == "" {
		http.Error(w, "task_id is required", http.StatusBadRequest)
		return
	}

	// Get task information
	task, err := taskManager.GetTask(taskID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Task not found: %v", err), http.StatusNotFound)
		return
	}

	// Upgrade connection to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to upgrade connection: %v", err), http.StatusInternalServerError)
		return
	}
	defer conn.Close()

	// Wrap connection for thread-safe writes
	safeConn := &safeConn{conn: conn}

	// Paths to output files
	stdoutPath := filepath.Join(task.OutputDir, "stdout")
	stderrPath := filepath.Join(task.OutputDir, "stderr")

	// Start tailing stdout and stderr
	ctx := r.Context()
	go tailFile(ctx, safeConn, stdoutPath, "stdout")
	go tailFile(ctx, safeConn, stderrPath, "stderr")

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

// tailFile tails a file and sends updates over WebSocket
func tailFile(ctx context.Context, safeConn *safeConn, filePath, outputType string) {
	// Wait for file to be created (up to 30 seconds)
	fileExists := false
	for i := 0; i < 30; i++ {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if _, err := os.Stat(filePath); err == nil {
			fileExists = true
			break
		}
		time.Sleep(1 * time.Second)
	}

	if !fileExists {
		// File doesn't exist yet, send waiting message
		msg := WebSocketMessage{
			Type: outputType,
			Data: fmt.Sprintf("Waiting for output file...\n"),
		}
		if data, err := json.Marshal(msg); err == nil {
			safeConn.WriteMessage(websocket.TextMessage, data)
		}
		// Continue waiting and checking
		for i := 0; i < 60; i++ {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if _, err := os.Stat(filePath); err == nil {
				fileExists = true
				break
			}
			time.Sleep(1 * time.Second)
		}
		if !fileExists {
			return
		}
	}

	// Open file for reading
	file, err := os.Open(filePath)
	if err != nil {
		return
	}
	defer file.Close()

	// Read existing content first
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}
		msg := WebSocketMessage{
			Type: outputType,
			Data: scanner.Text() + "\n",
		}
		if data, err := json.Marshal(msg); err == nil {
			if err := safeConn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		}
	}

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
					continue
				}

				// Seek to last known position
				file.Seek(lastPos, io.SeekStart)

				// Read new lines
				scanner := bufio.NewScanner(file)
				for scanner.Scan() {
					select {
					case <-ctx.Done():
						file.Close()
						return
					default:
					}
					msg := WebSocketMessage{
						Type: outputType,
						Data: scanner.Text() + "\n",
					}
					if data, err := json.Marshal(msg); err == nil {
						if err := safeConn.WriteMessage(websocket.TextMessage, data); err != nil {
							file.Close()
							return
						}
					}
				}

				// Update last position
				lastPos, _ = file.Seek(0, io.SeekEnd)
			}
		}
	}
}

