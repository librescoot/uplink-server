package storage

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/librescoot/uplink-server/internal/models"
)

// ConnectionManager manages active scooter connections
type ConnectionManager struct {
	mu          sync.RWMutex
	connections map[string]*models.Connection

	// Statistics
	totalConnections   int64
	totalAuthenticated int64
	totalTelemetry     int64
	totalCommandsSent  int64
	totalBytesSent     int64
	totalBytesReceived int64
}

// NewConnectionManager creates a new connection manager
func NewConnectionManager() *ConnectionManager {
	return &ConnectionManager{
		connections: make(map[string]*models.Connection),
	}
}

// AddConnection adds a new connection
func (cm *ConnectionManager) AddConnection(conn *models.Connection) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if _, exists := cm.connections[conn.Identifier]; exists {
		return fmt.Errorf("connection already exists for identifier: %s", conn.Identifier)
	}

	cm.connections[conn.Identifier] = conn
	cm.totalConnections++

	log.Printf("[ConnectionManager] Added connection for %s (total: %d)",
		conn.Identifier, len(cm.connections))

	return nil
}

// RemoveConnection removes a connection
func (cm *ConnectionManager) RemoveConnection(identifier string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	conn, exists := cm.connections[identifier]
	if !exists {
		return
	}

	// Update global statistics before removing
	stats := conn.GetStats()
	cm.totalBytesSent += stats["bytes_sent"].(int64)
	cm.totalBytesReceived += stats["bytes_received"].(int64)
	cm.totalTelemetry += stats["telemetry_received"].(int64)
	cm.totalCommandsSent += stats["commands_sent"].(int64)

	delete(cm.connections, identifier)
	log.Printf("[ConnectionManager] Removed connection for %s (remaining: %d)",
		identifier, len(cm.connections))
}

// GetConnection returns a connection by identifier
func (cm *ConnectionManager) GetConnection(identifier string) (*models.Connection, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	conn, exists := cm.connections[identifier]
	return conn, exists
}

// GetAllConnections returns all active connections
func (cm *ConnectionManager) GetAllConnections() []*models.Connection {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	conns := make([]*models.Connection, 0, len(cm.connections))
	for _, conn := range cm.connections {
		conns = append(conns, conn)
	}

	return conns
}

// MarkAuthenticated marks a connection as authenticated
func (cm *ConnectionManager) MarkAuthenticated(identifier string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	conn, exists := cm.connections[identifier]
	if !exists {
		return fmt.Errorf("connection not found: %s", identifier)
	}

	conn.Authenticated = true
	cm.totalAuthenticated++

	log.Printf("[ConnectionManager] Connection authenticated: %s", identifier)

	return nil
}

// GetStats returns connection manager statistics
func (cm *ConnectionManager) GetStats() map[string]any {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	activeConns := len(cm.connections)
	authenticatedConns := 0
	for _, conn := range cm.connections {
		if conn.Authenticated {
			authenticatedConns++
		}
	}

	// Calculate current session stats
	var currentBytesSent, currentBytesReceived int64
	var currentTelemetry, currentCommands int64
	for _, conn := range cm.connections {
		stats := conn.GetStats()
		currentBytesSent += stats["bytes_sent"].(int64)
		currentBytesReceived += stats["bytes_received"].(int64)
		currentTelemetry += stats["telemetry_received"].(int64)
		currentCommands += stats["commands_sent"].(int64)
	}

	return map[string]any{
		"active_connections":     activeConns,
		"authenticated":          authenticatedConns,
		"total_connections":      cm.totalConnections,
		"total_authenticated":    cm.totalAuthenticated,
		"current_bytes_sent":     currentBytesSent,
		"current_bytes_received": currentBytesReceived,
		"total_bytes_sent":       cm.totalBytesSent + currentBytesSent,
		"total_bytes_received":   cm.totalBytesReceived + currentBytesReceived,
		"current_telemetry":      currentTelemetry,
		"total_telemetry":        cm.totalTelemetry + currentTelemetry,
		"current_commands":       currentCommands,
		"total_commands":         cm.totalCommandsSent + currentCommands,
	}
}

// PrintStats prints formatted statistics
func (cm *ConnectionManager) PrintStats() {
	stats := cm.GetStats()

	log.Printf("[Stats] Active: %d/%d auth | Session: ↑%.1fKB ↓%.1fKB tel:%d cmd:%d | Total: ↑%.1fKB ↓%.1fKB tel:%d cmd:%d",
		stats["active_connections"], stats["authenticated"],
		float64(stats["current_bytes_sent"].(int64))/1024,
		float64(stats["current_bytes_received"].(int64))/1024,
		stats["current_telemetry"], stats["current_commands"],
		float64(stats["total_bytes_sent"].(int64))/1024,
		float64(stats["total_bytes_received"].(int64))/1024,
		stats["total_telemetry"], stats["total_commands"])
}

// StartStatsLogger starts periodic stats logging
func (cm *ConnectionManager) StartStatsLogger(interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			cm.PrintStats()
		}
	}()
}
