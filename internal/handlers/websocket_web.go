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
	EnableCompression: true,
}

// WebUIHandler handles WebSocket connections from web UI clients
type WebUIHandler struct {
	stateStore *storage.StateStore
	eventStore *storage.EventStore
	connMgr    *storage.ConnectionManager
	auth       Authenticator
	apiKey     string
}

// Authenticator interface for getting scooter names
type Authenticator interface {
	GetName(identifier string) string
}

// NewWebUIHandler creates a new web UI WebSocket handler
func NewWebUIHandler(stateStore *storage.StateStore, eventStore *storage.EventStore, connMgr *storage.ConnectionManager, auth Authenticator, apiKey string) *WebUIHandler {
	return &WebUIHandler{
		stateStore: stateStore,
		eventStore: eventStore,
		connMgr:    connMgr,
		auth:       auth,
		apiKey:     apiKey,
	}
}

// WebMessage represents a message sent to/from web UI clients
type WebMessage struct {
	Type       string         `json:"type"`
	Scooters   []ScooterInfo  `json:"scooters,omitempty"`
	Scooter    *ScooterInfo   `json:"scooter,omitempty"`
	ScooterID  string         `json:"scooter_id,omitempty"`
	State      map[string]any `json:"state,omitempty"`
	UpdateType string         `json:"update_type,omitempty"` // "full" or "delta"
	Event      string         `json:"event,omitempty"`
	EventID    string         `json:"event_id,omitempty"`
	EventData  map[string]any `json:"event_data,omitempty"`
	Error      string         `json:"error,omitempty"`
	Timestamp  string         `json:"timestamp,omitempty"`
	// Connection stats (included with state updates for connected scooters)
	BytesSent         *int64 `json:"bytes_sent,omitempty"`
	BytesReceived     *int64 `json:"bytes_received,omitempty"`
	WireBytesSent     *int64 `json:"wire_bytes_sent,omitempty"`
	WireBytesReceived *int64 `json:"wire_bytes_received,omitempty"`
	TelemetryReceived *int64 `json:"telemetry_received,omitempty"`
	CommandsSent      *int64 `json:"commands_sent,omitempty"`
}

// ScooterInfo represents scooter connection information
type ScooterInfo struct {
	Identifier        string `json:"identifier"`
	Name              string `json:"name,omitempty"`
	Connected         bool   `json:"connected"`
	Version           string `json:"version,omitempty"`
	Uptime            int64  `json:"uptime_seconds,omitempty"`
	BytesSent         int64  `json:"bytes_sent,omitempty"`
	BytesReceived     int64  `json:"bytes_received,omitempty"`
	WireBytesS        int64  `json:"wire_bytes_sent,omitempty"`
	WireBytesRecv     int64  `json:"wire_bytes_received,omitempty"`
	TelemetryReceived int64  `json:"telemetry_received,omitempty"`
	CommandsSent      int64  `json:"commands_sent,omitempty"`
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

	// Subscribe to event updates
	eventChan := h.eventStore.Subscribe()

	// Subscribe to connection events
	connChan := h.connMgr.Subscribe()

	// Send initial state for all connected scooters
	h.sendInitialStates(conn)

	// Send initial events for all connected scooters
	h.sendInitialEvents(conn)

	// Start goroutines to listen for updates and broadcast to client
	done := make(chan struct{})
	go h.broadcastUpdates(conn, updateChan, done)
	go h.broadcastEvents(conn, eventChan, done)
	go h.broadcastConnectionEvents(conn, connChan, done)

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

// sendScooterList sends the list of all scooters (connected and disconnected with state)
func (h *WebUIHandler) sendScooterList(conn *websocket.Conn) {
	connections := h.connMgr.GetAllConnections()
	scooterMap := make(map[string]ScooterInfo)

	// Add all currently connected scooters
	for _, c := range connections {
		info := ScooterInfo{
			Identifier:        c.Identifier,
			Name:              c.Name,
			Connected:         true,
			Version:           c.Version,
			Uptime:            int64(time.Since(c.ConnectedAt).Seconds()),
			BytesSent:         c.BytesSent,
			BytesReceived:     c.BytesReceived,
			TelemetryReceived: c.TelemetryReceived,
			CommandsSent:      c.CommandsSent,
		}

		// Add wire-level stats if available
		if c.StatsConn != nil {
			info.WireBytesS = c.StatsConn.BytesWritten()
			info.WireBytesRecv = c.StatsConn.BytesRead()
		}

		scooterMap[c.Identifier] = info
	}

	// Add scooters with persisted state that aren't currently connected
	allStates := h.stateStore.GetAllStates()
	for scooterID, state := range allStates {
		if _, exists := scooterMap[scooterID]; !exists {
			// Scooter has state but is not connected
			scooterMap[scooterID] = ScooterInfo{
				Identifier: scooterID,
				Name:       h.auth.GetName(scooterID),
				Version:    state.Version,
				Connected:  false,
			}
		}
	}

	// Convert map to slice
	scooters := make([]ScooterInfo, 0, len(scooterMap))
	for _, info := range scooterMap {
		scooters = append(scooters, info)
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

// sendInitialStates sends the current state for all scooters with persisted state
func (h *WebUIHandler) sendInitialStates(conn *websocket.Conn) {
	allStates := h.stateStore.GetAllStates()

	for scooterID, state := range allStates {
		msg := WebMessage{
			Type:       "state_update",
			ScooterID:  scooterID,
			State:      state.State,
			UpdateType: "full",
			Timestamp:  time.Now().UTC().Format(time.RFC3339),
		}

		if err := conn.WriteJSON(msg); err != nil {
			log.Printf("[WebUI] Failed to send initial state for %s: %v", scooterID, err)
		}
	}
}

// sendInitialEvents sends stored events for all scooters with events
func (h *WebUIHandler) sendInitialEvents(conn *websocket.Conn) {
	allEvents := h.eventStore.GetAllEvents()

	for scooterID, events := range allEvents {
		// Reverse events so oldest is sent first, then prepending in UI reverses back to newest-first
		for i := len(events) - 1; i >= 0; i-- {
			event := events[i]
			msg := WebMessage{
				Type:      "event",
				ScooterID: event.ScooterID,
				Event:     event.Event,
				EventID:   event.ID,
				EventData: event.Data,
				Timestamp: event.Timestamp.UTC().Format(time.RFC3339),
			}

			if err := conn.WriteJSON(msg); err != nil {
				log.Printf("[WebUI] Failed to send initial event for %s: %v", scooterID, err)
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

			// Include connection stats if scooter is connected
			if c, exists := h.connMgr.GetConnection(update.ScooterID); exists {
				msg.BytesSent = &c.BytesSent
				msg.BytesReceived = &c.BytesReceived
				msg.TelemetryReceived = &c.TelemetryReceived
				msg.CommandsSent = &c.CommandsSent

				// Add wire-level stats if available
				if c.StatsConn != nil {
					wireBytesSent := c.StatsConn.BytesWritten()
					wireBytesRecv := c.StatsConn.BytesRead()
					msg.WireBytesSent = &wireBytesSent
					msg.WireBytesReceived = &wireBytesRecv
				}
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

// broadcastEvents listens for event updates and sends them to the web client
func (h *WebUIHandler) broadcastEvents(conn *websocket.Conn, eventChan <-chan *storage.Event, done <-chan struct{}) {
	for {
		select {
		case event := <-eventChan:
			msg := WebMessage{
				Type:      "event",
				ScooterID: event.ScooterID,
				Event:     event.Event,
				EventID:   event.ID,
				EventData: event.Data,
				Timestamp: event.Timestamp.UTC().Format(time.RFC3339),
			}

			if err := conn.WriteJSON(msg); err != nil {
				log.Printf("[WebUI] Failed to send event update: %v", err)
				return
			}

		case <-done:
			return
		}
	}
}

// broadcastConnectionEvents listens for connection events and sends them to the web client
func (h *WebUIHandler) broadcastConnectionEvents(conn *websocket.Conn, connChan <-chan storage.ConnectionEvent, done <-chan struct{}) {
	for {
		select {
		case event := <-connChan:
			if event.Type == "online" && event.Connection != nil {
				// Scooter came online
				scooterInfo := ScooterInfo{
					Identifier:        event.Connection.Identifier,
					Name:              event.Connection.Name,
					Connected:         true,
					Version:           event.Connection.Version,
					Uptime:            int64(time.Since(event.Connection.ConnectedAt).Seconds()),
					BytesSent:         event.Connection.BytesSent,
					BytesReceived:     event.Connection.BytesReceived,
					TelemetryReceived: event.Connection.TelemetryReceived,
					CommandsSent:      event.Connection.CommandsSent,
				}

				// Add wire-level stats if available
				if event.Connection.StatsConn != nil {
					scooterInfo.WireBytesS = event.Connection.StatsConn.BytesWritten()
					scooterInfo.WireBytesRecv = event.Connection.StatsConn.BytesRead()
				}

				msg := WebMessage{
					Type:      "scooter_online",
					Scooter:   &scooterInfo,
					Timestamp: time.Now().UTC().Format(time.RFC3339),
				}

				if err := conn.WriteJSON(msg); err != nil {
					log.Printf("[WebUI] Failed to send scooter_online: %v", err)
					return
				}
			} else if event.Type == "offline" {
				// Scooter went offline
				msg := WebMessage{
					Type:      "scooter_offline",
					ScooterID: event.Identifier,
					Timestamp: time.Now().UTC().Format(time.RFC3339),
				}

				if err := conn.WriteJSON(msg); err != nil {
					log.Printf("[WebUI] Failed to send scooter_offline: %v", err)
					return
				}
			}

		case <-done:
			return
		}
	}
}
