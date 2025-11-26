package main

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/gorilla/websocket"
)

// WebSocketManager manages all active WebSocket connections
type WebSocketManager struct {
	connections map[*safeConn]bool
	mu          sync.RWMutex
}

// NewWebSocketManager creates a new WebSocket manager
func NewWebSocketManager() *WebSocketManager {
	return &WebSocketManager{
		connections: make(map[*safeConn]bool),
	}
}

// Add adds a connection to the manager
func (wsm *WebSocketManager) Add(conn *safeConn) {
	wsm.mu.Lock()
	defer wsm.mu.Unlock()
	wsm.connections[conn] = true
	log.Printf("[WSM] Connection added, total connections: %d", len(wsm.connections))
}

// Remove removes a connection from the manager
func (wsm *WebSocketManager) Remove(conn *safeConn) {
	wsm.mu.Lock()
	defer wsm.mu.Unlock()
	delete(wsm.connections, conn)
	log.Printf("[WSM] Connection removed, total connections: %d", len(wsm.connections))
}

// BroadcastShutdown sends a shutdown message to all connections and closes them
func (wsm *WebSocketManager) BroadcastShutdown(message string) {
	wsm.mu.Lock()
	defer wsm.mu.Unlock()

	log.Printf("[WSM] Broadcasting shutdown message to %d connections", len(wsm.connections))

	shutdownMsg := SystemMessage{
		Type:    "system",
		Message: message,
		PID:     0,
	}
	data, err := json.Marshal(shutdownMsg)
	if err != nil {
		log.Printf("[WSM] Failed to marshal shutdown message: %v", err)
		return
	}

	for conn := range wsm.connections {
		// Send shutdown message
		conn.mu.Lock()
		conn.conn.WriteMessage(websocket.TextMessage, data)
		conn.conn.Close()
		conn.mu.Unlock()
	}

	log.Printf("[WSM] All connections closed")
}

// Count returns the number of active connections
func (wsm *WebSocketManager) Count() int {
	wsm.mu.RLock()
	defer wsm.mu.RUnlock()
	return len(wsm.connections)
}

