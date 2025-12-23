package handlers

import (
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	"github.com/librescoot/uplink-server/internal/storage"
)

var webUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for now
	},
}

// WebUIHandler handles WebSocket connections from web UI clients
type WebUIHandler struct {
	stateStore *storage.StateStore
	connMgr    *storage.ConnectionManager
	apiKey     string
}

// NewWebUIHandler creates a new web UI WebSocket handler
func NewWebUIHandler(stateStore *storage.StateStore, connMgr *storage.ConnectionManager, apiKey string) *WebUIHandler {
	return &WebUIHandler{
		stateStore: stateStore,
		connMgr:    connMgr,
		apiKey:     apiKey,
	}
}

// WebMessage represents a message sent to/from web UI clients
type WebMessage struct {
	Type      string         `json:"type"`
	Scooters  []ScooterInfo  `json:"scooters,omitempty"`
	Scooter   *ScooterInfo   `json:"scooter,omitempty"`
	ScooterID string         `json:"scooter_id,omitempty"`
	State     map[string]any `json:"state,omitempty"`
	UpdateType string        `json:"update_type,omitempty"` // "full" or "delta"
	Error     string         `json:"error,omitempty"`
	Timestamp string         `json:"timestamp,omitempty"`
}

// ScooterInfo represents scooter connection information
type ScooterInfo struct {
	Identifier       string `json:"identifier"`
	Name             string `json:"name,omitempty"`
	Connected        bool   `json:"connected"`
	Version          string `json:"version,omitempty"`
	Uptime           int64  `json:"uptime_seconds,omitempty"`
	BytesSent        int64  `json:"bytes_sent,omitempty"`
	BytesReceived    int64  `json:"bytes_received,omitempty"`
	TelemetryReceived int64 `json:"telemetry_received,omitempty"`
	CommandsSent     int64  `json:"commands_sent,omitempty"`
}

// HandleWebConnection handles WebSocket connections from web UI
func (h *WebUIHandler) HandleWebConnection(w http.ResponseWriter, r *http.Request) {
	// Authenticate via API key (from header or query param)
	apiKey := r.Header.Get("X-API-Key")
	if apiKey == "" {
		apiKey = r.URL.Query().Get("api_key")
	}

	if apiKey != h.apiKey {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Upgrade to WebSocket
	conn, err := webUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WebUI] WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	log.Printf("[WebUI] Client connected from %s", r.RemoteAddr)

	// Send initial scooter list
	h.sendScooterList(conn)

	// Subscribe to state updates
	updateChan := h.stateStore.Subscribe()
	defer func() {
		// Note: We don't close the channel as other subscribers may be using it
		// The StateStore manages subscriber lifecycle
	}()

	// Send initial state for all connected scooters
	h.sendInitialStates(conn)

	// Start goroutine to listen for state updates and broadcast to client
	done := make(chan struct{})
	go h.broadcastUpdates(conn, updateChan, done)

	// Keep connection alive and handle disconnection
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			log.Printf("[WebUI] Client disconnected: %v", err)
			close(done)
			return
		}
	}
}

// sendScooterList sends the list of connected scooters to the client
func (h *WebUIHandler) sendScooterList(conn *websocket.Conn) {
	connections := h.connMgr.GetAllConnections()
	scooters := make([]ScooterInfo, 0, len(connections))

	for _, c := range connections {
		scooters = append(scooters, ScooterInfo{
			Identifier:       c.Identifier,
			Name:             c.Name,
			Connected:        true,
			Version:          c.Version,
			Uptime:           int64(time.Since(c.ConnectedAt).Seconds()),
			BytesSent:        c.BytesSent,
			BytesReceived:    c.BytesReceived,
			TelemetryReceived: c.TelemetryReceived,
			CommandsSent:     c.CommandsSent,
		})
	}

	msg := WebMessage{
		Type:      "scooter_list",
		Scooters:  scooters,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	if err := conn.WriteJSON(msg); err != nil {
		log.Printf("[WebUI] Failed to send scooter list: %v", err)
	}
}

// sendInitialStates sends the current state for all connected scooters
func (h *WebUIHandler) sendInitialStates(conn *websocket.Conn) {
	connections := h.connMgr.GetAllConnections()

	for _, c := range connections {
		if state, ok := h.stateStore.GetState(c.Identifier); ok {
			msg := WebMessage{
				Type:       "state_update",
				ScooterID:  c.Identifier,
				State:      state.State,
				UpdateType: "full",
				Timestamp:  time.Now().UTC().Format(time.RFC3339),
			}

			if err := conn.WriteJSON(msg); err != nil {
				log.Printf("[WebUI] Failed to send initial state for %s: %v", c.Identifier, err)
			}
		}
	}
}

// broadcastUpdates listens for state updates and sends them to the web client
func (h *WebUIHandler) broadcastUpdates(conn *websocket.Conn, updateChan <-chan storage.StateUpdate, done <-chan struct{}) {
	for {
		select {
		case update := <-updateChan:
			msg := WebMessage{
				Type:       "state_update",
				ScooterID:  update.ScooterID,
				State:      update.State,
				UpdateType: update.Type,
				Timestamp:  update.Timestamp.UTC().Format(time.RFC3339),
			}

			if err := conn.WriteJSON(msg); err != nil {
				log.Printf("[WebUI] Failed to send state update: %v", err)
				return
			}

		case <-done:
			return
		}
	}
}
