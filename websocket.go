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
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

// createUpgrader creates a WebSocket upgrader with origin checking
func createUpgrader(allowedOrigins []string) websocket.Upgrader {
	return websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			// If no origins specified, allow all (for internal networks)
			if len(allowedOrigins) == 0 {
				return true
			}
			origin := r.Header.Get("Origin")
			for _, allowed := range allowedOrigins {
				if origin == allowed {
					return true
				}
			}
			return false
		},
	}
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
func handleWebSocket(w http.ResponseWriter, r *http.Request, taskManager *TaskManager, config *Config, upgrader websocket.Upgrader) {
	log.Printf("[WEBSOCKET] Connection attempt from %s", r.RemoteAddr)
	
	// Authenticate request
	claims, err := validateJWT(r, config.Auth.Secret)
	if err != nil {
		log.Printf("[WEBSOCKET] Authentication failed: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(ErrorResponse{Error: fmt.Sprintf("Unauthorized: %v", err)})
		return
	}

	taskID := r.URL.Query().Get("task_id")
	if taskID == "" {
		taskID = claims.TaskID
	}

	if taskID == "" {
		log.Printf("[WEBSOCKET] Missing task_id")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "task_id is required"})
		return
	}

	// Get task information
	task, err := taskManager.GetTask(taskID)
	if err != nil {
		log.Printf("[WEBSOCKET] Task not found: task_id=%s, error=%v", taskID, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: fmt.Sprintf("Task not found: %v", err)})
		return
	}

	// Upgrade connection to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WEBSOCKET] Failed to upgrade connection: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: fmt.Sprintf("Failed to upgrade connection: %v", err)})
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
	exitCodePath := filepath.Join(task.OutputDir, "exitcode")

	// Try to read PID and send initial message
	pid := readPID(pidPath)
	if pid > 0 {
		sendSystemMessage(safeConn, "connected", fmt.Sprintf("WebSocket connected. Process started"), pid)
		log.Printf("[WEBSOCKET] Sent initial message with PID=%d for task_id=%s", pid, taskID)
	} else {
		sendSystemMessage(safeConn, "connected", "WebSocket connected. Waiting for process to start...", 0)
		log.Printf("[WEBSOCKET] Sent initial message (no PID yet) for task_id=%s", taskID)
	}

	// Start monitoring process completion and timeout
	ctx := r.Context()
	go monitorProcess(ctx, safeConn, taskManager, taskID, pidPath, exitCodePath, task.OutputDir, task.MaxExecutionTime)

	// Start tailing stdout and stderr
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

// isProcessRunning checks if a process with the given PID is still running
func isProcessRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Signal 0 doesn't actually send a signal, just checks if process exists
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// monitorProcess monitors the process and handles cleanup when it finishes
func monitorProcess(ctx context.Context, safeConn *safeConn, taskManager *TaskManager, taskID, pidPath, exitCodePath, outputDir string, maxExecutionTime time.Duration) {
	// Wait for PID file to be created
	var pid int
	for i := 0; i < 60; i++ {
		select {
		case <-ctx.Done():
			return
		default:
		}
		pid = readPID(pidPath)
		if pid > 0 {
			break
		}
		time.Sleep(1 * time.Second)
	}

	if pid == 0 {
		log.Printf("[MONITOR] PID not found for task_id=%s", taskID)
		return
	}

	log.Printf("[MONITOR] Monitoring process PID=%d for task_id=%s", pid, taskID)

	// Start timeout monitor if max execution time is set
	var timeoutTimer *time.Timer
	var timeoutChan <-chan time.Time
	if maxExecutionTime > 0 {
		timeoutTimer = time.NewTimer(maxExecutionTime)
		timeoutChan = timeoutTimer.C
		log.Printf("[MONITOR] Max execution time set to %v for task_id=%s", maxExecutionTime, taskID)
	}

	// Poll to check if process is still running
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	if timeoutTimer != nil {
		defer timeoutTimer.Stop()
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-timeoutChan:
			// Max execution time exceeded
			handleTimeout(safeConn, taskManager, taskID, pid)
			timeoutChan = nil // Disable timeout channel after handling
		case <-ticker.C:
			if !isProcessRunning(pid) {
				// Process has ended, read exit code
				exitCode := readExitCode(exitCodePath)
				
				// Send completion message
				msg := fmt.Sprintf("Process ended with exit code: %d", exitCode)
				sendSystemMessage(safeConn, "completed", msg, pid)
				log.Printf("[MONITOR] Process ended: task_id=%s, pid=%d, exit_code=%d", taskID, pid, exitCode)

				// Wait a bit for final output to be written and message to be sent
				time.Sleep(2 * time.Second)

				// Remove task from manager
				taskManager.mu.Lock()
				delete(taskManager.runningTasks, taskID)
				taskManager.mu.Unlock()

				// Close WebSocket connection (client should have closed it already, but close it here too)
				safeConn.mu.Lock()
				safeConn.conn.Close()
				safeConn.mu.Unlock()

				// Cleanup: remove task directory (after connection is closed)
				time.Sleep(1 * time.Second)
				if err := os.RemoveAll(outputDir); err != nil {
					log.Printf("[MONITOR] Failed to cleanup directory %s: %v", outputDir, err)
				} else {
					log.Printf("[MONITOR] Cleaned up directory: %s", outputDir)
				}

				return
			}
		}
	}
}

// readExitCode reads the exit code from the exitcode file
func readExitCode(exitCodePath string) int {
	data, err := os.ReadFile(exitCodePath)
	if err != nil {
		return -1 // Unknown exit code
	}
	exitCodeStr := strings.TrimSpace(string(data))
	exitCode, err := strconv.Atoi(exitCodeStr)
	if err != nil {
		return -1
	}
	return exitCode
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
			Data: fmt.Sprintf("Waiting for output file..."),
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
					log.Printf("[TAIL] Failed to reopen file: %s, error: %v", filePath, err)
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

