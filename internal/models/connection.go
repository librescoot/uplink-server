package models

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Connection represents an active scooter connection
type Connection struct {
	mu            sync.RWMutex
	Identifier    string
	Name          string // Human-friendly name (optional)
	Conn          *websocket.Conn
	Authenticated bool
	ConnectedAt   time.Time
	LastSeen      time.Time
	Version       string

	// Statistics
	BytesSent         int64
	BytesReceived     int64
	MessagesSent      int64
	MessagesReceived  int64
	TelemetryReceived int64
	CommandsSent      int64

	// Channels for command sending
	sendChan chan []byte
	done     chan struct{}
}

// NewConnection creates a new connection
func NewConnection(identifier string, conn *websocket.Conn) *Connection {
	return &Connection{
		Identifier:  identifier,
		Conn:        conn,
		ConnectedAt: time.Now(),
		LastSeen:    time.Now(),
		sendChan:    make(chan []byte, 256),
		done:        make(chan struct{}),
	}
}

// UpdateLastSeen updates the last seen timestamp
func (c *Connection) UpdateLastSeen() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.LastSeen = time.Now()
}

// AddBytesSent adds to bytes sent counter
func (c *Connection) AddBytesSent(n int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.BytesSent += n
}

// AddBytesReceived adds to bytes received counter
func (c *Connection) AddBytesReceived(n int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.BytesReceived += n
}

// IncrementMessagesSent increments messages sent counter
func (c *Connection) IncrementMessagesSent() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.MessagesSent++
}

// IncrementMessagesReceived increments messages received counter
func (c *Connection) IncrementMessagesReceived() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.MessagesReceived++
}

// IncrementTelemetryReceived increments telemetry received counter
func (c *Connection) IncrementTelemetryReceived() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.TelemetryReceived++
}

// IncrementCommandsSent increments commands sent counter
func (c *Connection) IncrementCommandsSent() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.CommandsSent++
}

// GetStats returns current connection statistics
func (c *Connection) GetStats() map[string]any {
	c.mu.RLock()
	defer c.mu.RUnlock()

	uptime := time.Since(c.ConnectedAt)
	idle := time.Since(c.LastSeen)

	return map[string]any{
		"identifier":         c.Identifier,
		"authenticated":      c.Authenticated,
		"connected_at":       c.ConnectedAt.Format(time.RFC3339),
		"uptime_seconds":     uptime.Seconds(),
		"last_seen":          c.LastSeen.Format(time.RFC3339),
		"idle_seconds":       idle.Seconds(),
		"bytes_sent":         c.BytesSent,
		"bytes_received":     c.BytesReceived,
		"messages_sent":      c.MessagesSent,
		"messages_received":  c.MessagesReceived,
		"telemetry_received": c.TelemetryReceived,
		"commands_sent":      c.CommandsSent,
		"version":            c.Version,
	}
}

// SendChannel returns the send channel for this connection
func (c *Connection) SendChannel() chan<- []byte {
	return c.sendChan
}

// ReceiveChannel returns the receive channel (send channel cast as receive)
func (c *Connection) ReceiveChannel() <-chan []byte {
	return c.sendChan
}

// Done returns the done channel
func (c *Connection) Done() <-chan struct{} {
	return c.done
}

// Close closes the connection
func (c *Connection) Close() {
	close(c.done)
	close(c.sendChan)
}
