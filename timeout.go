package main

import (
	"log"
	"os"
	"syscall"
	"time"
)

// handleTimeout handles when a task exceeds its maximum execution time
func handleTimeout(safeConn *safeConn, taskManager *TaskManager, taskID string, pid int) {
	log.Printf("[TIMEOUT] Max execution time exceeded for task_id=%s, pid=%d", taskID, pid)

	// Get task to check if already terminated/killed
	taskManager.mu.Lock()
	task, exists := taskManager.runningTasks[taskID]
	if !exists {
		taskManager.mu.Unlock()
		return
	}

	// Check if already terminated
	if task.Terminated {
		// Already sent SIGTERM, check if we should send SIGKILL
		if !task.Killed {
			// Check if process is still running
			if isProcessRunning(pid) {
				// Process still running after SIGTERM, send SIGKILL
				task.Killed = true
				taskManager.mu.Unlock()

				sendSystemMessage(safeConn, "timeout", "Process exceeded maximum execution time. Sending SIGKILL...", pid)
				log.Printf("[TIMEOUT] Sending SIGKILL to PID=%d for task_id=%s", pid, taskID)

				process, err := os.FindProcess(pid)
				if err == nil {
					process.Signal(syscall.SIGKILL)
				}
			} else {
				taskManager.mu.Unlock()
			}
		} else {
			taskManager.mu.Unlock()
		}
		return
	}

	// Mark as terminated and send SIGTERM
	task.Terminated = true
	taskManager.mu.Unlock()

	// Send SIGTERM
	sendSystemMessage(safeConn, "timeout", "Process exceeded maximum execution time. Sending SIGTERM (graceful shutdown)...", pid)
	log.Printf("[TIMEOUT] Sending SIGTERM to PID=%d for task_id=%s", pid, taskID)

	process, err := os.FindProcess(pid)
	if err == nil {
		process.Signal(syscall.SIGTERM)
	}

	// Start a goroutine to check after 30 seconds if process is still running
	go func() {
		time.Sleep(30 * time.Second)

		taskManager.mu.Lock()
		task, exists := taskManager.runningTasks[taskID]
		if !exists {
			taskManager.mu.Unlock()
			return
		}

		if !task.Killed && isProcessRunning(pid) {
			// Process still running after 30 seconds, send SIGKILL
			task.Killed = true
			taskManager.mu.Unlock()

			sendSystemMessage(safeConn, "timeout", "Process did not terminate after SIGTERM. Sending SIGKILL...", pid)
			log.Printf("[TIMEOUT] Sending SIGKILL to PID=%d for task_id=%s (after 30s grace period)", pid, taskID)

			process, err := os.FindProcess(pid)
			if err == nil {
				process.Signal(syscall.SIGKILL)
			}
		} else {
			taskManager.mu.Unlock()
		}
	}()
}

