package main

import (
	"testing"
)

func TestNewWebSocketManager(t *testing.T) {
	wsm := NewWebSocketManager()
	if wsm == nil {
		t.Fatal("NewWebSocketManager() = nil; want non-nil")
	}
	if wsm.connections == nil {
		t.Error("NewWebSocketManager() connections = nil; want non-nil")
	}
	if len(wsm.connections) != 0 {
		t.Errorf("NewWebSocketManager() connections length = %d; want 0", len(wsm.connections))
	}
}

func TestWebSocketManagerAdd(t *testing.T) {
	wsm := NewWebSocketManager()
	
	// Create a mock safeConn
	conn := &safeConn{
		conn: nil, // We don't need a real connection for testing
	}
	
	wsm.Add(conn)
	
	if len(wsm.connections) != 1 {
		t.Errorf("WebSocketManager.Add() connections length = %d; want 1", len(wsm.connections))
	}
	
	if !wsm.connections[conn] {
		t.Error("WebSocketManager.Add() connection not found in map")
	}
}

func TestWebSocketManagerRemove(t *testing.T) {
	wsm := NewWebSocketManager()
	
	conn := &safeConn{conn: nil}
	
	// Add then remove
	wsm.Add(conn)
	if len(wsm.connections) != 1 {
		t.Fatal("Setup failed: connection not added")
	}
	
	wsm.Remove(conn)
	
	if len(wsm.connections) != 0 {
		t.Errorf("WebSocketManager.Remove() connections length = %d; want 0", len(wsm.connections))
	}
	
	if wsm.connections[conn] {
		t.Error("WebSocketManager.Remove() connection still in map")
	}
}

func TestWebSocketManagerCount(t *testing.T) {
	wsm := NewWebSocketManager()
	
	if wsm.Count() != 0 {
		t.Errorf("WebSocketManager.Count() = %d; want 0", wsm.Count())
	}
	
	conn1 := &safeConn{conn: nil}
	conn2 := &safeConn{conn: nil}
	
	wsm.Add(conn1)
	if wsm.Count() != 1 {
		t.Errorf("WebSocketManager.Count() = %d; want 1", wsm.Count())
	}
	
	wsm.Add(conn2)
	if wsm.Count() != 2 {
		t.Errorf("WebSocketManager.Count() = %d; want 2", wsm.Count())
	}
	
	wsm.Remove(conn1)
	if wsm.Count() != 1 {
		t.Errorf("WebSocketManager.Count() = %d; want 1", wsm.Count())
	}
}

func TestWebSocketManagerBroadcastShutdown(t *testing.T) {
	wsm := NewWebSocketManager()
	
	// Note: BroadcastShutdown requires real websocket connections
	// For unit testing, we just verify it doesn't panic with empty connections
	// Integration tests would be needed for full coverage
	message := "Server shutting down"
	
	// Should not panic even with no connections
	wsm.BroadcastShutdown(message)
	
	// With connections, we'd need real WebSocket connections to test properly
	// This is better suited for integration tests
}

func TestWebSocketManagerConcurrentAccess(t *testing.T) {
	wsm := NewWebSocketManager()
	
	// Test concurrent Add operations
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			conn := &safeConn{conn: nil}
			wsm.Add(conn)
			done <- true
		}(i)
	}
	
	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
	
	if wsm.Count() != 10 {
		t.Errorf("WebSocketManager concurrent Add() count = %d; want 10", wsm.Count())
	}
}

