package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	"github.com/librescoot/uplink-server/internal/auth"
	"github.com/librescoot/uplink-server/internal/models"
	"github.com/librescoot/uplink-server/internal/protocol"
	"github.com/librescoot/uplink-server/internal/storage"
	"github.com/librescoot/uplink-server/internal/store"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
	EnableCompression: true,
}

// WebSocketHandler handles WebSocket connections
type WebSocketHandler struct {
	auth              *auth.Authenticator
	connMgr           *storage.ConnectionManager
	responseStore     *storage.ResponseStore
	stateStore        *storage.StateStore
	eventStore        *storage.EventStore
	db                *store.Store // durable persistence; may be nil
	keepaliveInterval time.Duration
	messageRateLimit  int
	idleTimeout       time.Duration
}

// NewWebSocketHandler creates a new WebSocket handler. db may be nil to disable
// durable persistence and command queuing.
func NewWebSocketHandler(authenticator *auth.Authenticator, connMgr *storage.ConnectionManager, responseStore *storage.ResponseStore, stateStore *storage.StateStore, eventStore *storage.EventStore, db *store.Store, keepaliveInterval time.Duration, messageRateLimit int, idleTimeout time.Duration) *WebSocketHandler {
	return &WebSocketHandler{
		auth:              authenticator,
		connMgr:           connMgr,
		responseStore:     responseStore,
		stateStore:        stateStore,
		eventStore:        eventStore,
		db:                db,
		keepaliveInterval: keepaliveInterval,
		messageRateLimit:  messageRateLimit,
		idleTimeout:       idleTimeout,
	}
}

// HandleConnection handles a WebSocket connection
func (h *WebSocketHandler) HandleConnection(w http.ResponseWriter, r *http.Request) {
	// Wrap response writer to track wire-level bytes
	statsWriter := NewStatsResponseWriter(w)

	conn, err := upgrader.Upgrade(statsWriter, r, nil)
	if err != nil {
		log.Printf("[WS] Upgrade error: %v", err)
		return
	}
	defer conn.Close()

	clientAddr := r.RemoteAddr
	log.Printf("[WS] New connection from %s", clientAddr)

	// Wait for authentication message
	_, message, err := conn.ReadMessage()
	if err != nil {
		log.Printf("[WS] Failed to read auth message from %s: %v", clientAddr, err)
		return
	}

	var baseMsg protocol.BaseMessage
	if err := json.Unmarshal(message, &baseMsg); err != nil {
		log.Printf("[WS] Failed to parse message from %s: %v", clientAddr, err)
		return
	}

	if baseMsg.Type != protocol.MsgTypeAuth {
		log.Printf("[WS] Expected auth message from %s, got %s", clientAddr, baseMsg.Type)
		h.sendAuthResponse(conn, "error", "Expected authentication message")
		return
	}

	var authMsg protocol.AuthMessage
	if err := json.Unmarshal(message, &authMsg); err != nil {
		log.Printf("[WS] Failed to parse auth message from %s: %v", clientAddr, err)
		h.sendAuthResponse(conn, "error", "Invalid authentication message format")
		return
	}

	// Authenticate
	if err := h.auth.Authenticate(authMsg.Identifier, authMsg.Token); err != nil {
		log.Printf("[WS] Authentication failed for %s: %v", authMsg.Identifier, err)
		h.sendAuthResponse(conn, "error", "Authentication failed")
		return
	}

	// Create connection object
	connection := models.NewConnection(authMsg.Identifier, conn)
	connection.Version = authMsg.Version
	connection.Authenticated = true
	connection.Name = h.auth.GetName(authMsg.Identifier)
	connection.StatsConn = statsWriter.GetStatsConn() // Track wire-level bytes

	// Add to connection manager
	if err := h.connMgr.AddConnection(connection); err != nil {
		log.Printf("[WS] Failed to add connection for %s: %v", authMsg.Identifier, err)
		h.sendAuthResponse(conn, "error", "Connection already exists")
		return
	}
	defer h.connMgr.RemoveConnection(authMsg.Identifier)

	// Mark as authenticated
	h.connMgr.MarkAuthenticated(authMsg.Identifier)

	// Update version in state store for persistence
	h.stateStore.SetVersion(authMsg.Identifier, authMsg.Version)

	// Send auth response
	h.sendAuthResponse(conn, "success", "")

	log.Printf("[WS] Client authenticated: %s (version: %s, protocol: %d)", authMsg.Identifier, authMsg.Version, authMsg.ProtocolVersion)

	// Replay any commands queued while this scooter was offline.
	h.replayQueuedCommands(connection)

	// Start keepalive sender
	done := make(chan struct{})
	defer close(done)

	go h.keepaliveSender(connection, done)
	go h.messageSender(connection, done)

	// Main message loop
	h.messageReceiver(connection)
}

// messageReceiver handles incoming messages
func (h *WebSocketHandler) messageReceiver(conn *models.Connection) {
	var rateLimiter <-chan time.Time
	if h.messageRateLimit > 0 {
		ticker := time.NewTicker(time.Second / time.Duration(h.messageRateLimit))
		defer ticker.Stop()
		rateLimiter = ticker.C
	}

	for {
		_, message, err := conn.Conn.ReadMessage()
		if err != nil {
			if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Printf("[WS] Read error from %s: %v", conn.Identifier, err)
			}
			return
		}

		// Rate limit: wait for next token before processing
		if rateLimiter != nil {
			<-rateLimiter
		}

		conn.AddBytesReceived(int64(len(message)))
		conn.IncrementMessagesReceived()
		conn.UpdateLastSeen()

		var baseMsg protocol.BaseMessage
		if err := json.Unmarshal(message, &baseMsg); err != nil {
			log.Printf("[WS] Failed to parse message from %s: %v", conn.Identifier, err)
			continue
		}

		switch baseMsg.Type {
		case protocol.MsgTypeKeepalive:
			log.Printf("[WS] Received keepalive from %s", conn.Identifier)

		case protocol.MsgTypeState:
			var stateMsg protocol.StateMessage
			if err := json.Unmarshal(message, &stateMsg); err != nil {
				log.Printf("[WS] Failed to parse state from %s: %v", conn.Identifier, err)
				continue
			}
			conn.IncrementTelemetryReceived()

			h.stateStore.UpdateState(conn.Identifier, stateMsg.Data)
			h.persistTelemetry(conn.Identifier, stateMsg.Data, time.Now())

			stateJSON, _ := json.MarshalIndent(stateMsg.Data, "", "  ")
			log.Printf("[WS] Received state snapshot from %s:\n%s", conn.Identifier, string(stateJSON))

		case protocol.MsgTypeChange:
			var changeMsg protocol.ChangeMessage
			if err := json.Unmarshal(message, &changeMsg); err != nil {
				log.Printf("[WS] Failed to parse change from %s: %v", conn.Identifier, err)
				continue
			}
			conn.IncrementTelemetryReceived()

			h.stateStore.UpdateChanges(conn.Identifier, changeMsg.Changes)
			h.persistMergedTelemetry(conn.Identifier)

			changeJSON, _ := json.MarshalIndent(changeMsg.Changes, "", "  ")
			log.Printf("[WS] Received state changes from %s:\n%s", conn.Identifier, string(changeJSON))

		case protocol.MsgTypeTelemetryDelta:
			var deltaMsg protocol.TelemetryDeltaMessage
			if err := json.Unmarshal(message, &deltaMsg); err != nil {
				log.Printf("[WS] Failed to parse telemetry delta from %s: %v", conn.Identifier, err)
				continue
			}
			conn.IncrementTelemetryReceived()

			h.stateStore.UpdateTelemetryDelta(conn.Identifier, deltaMsg.Changes, deltaMsg.Removed)
			h.persistMergedTelemetry(conn.Identifier)

			log.Printf("[WS] Received telemetry delta from %s (%d changes, %d removed)",
				conn.Identifier, len(deltaMsg.Changes), len(deltaMsg.Removed))

		case protocol.MsgTypeTelemetryBatch:
			var batchMsg protocol.TelemetryBatchMessage
			if err := json.Unmarshal(message, &batchMsg); err != nil {
				log.Printf("[WS] Failed to parse telemetry batch from %s: %v", conn.Identifier, err)
				continue
			}
			conn.IncrementTelemetryReceived()

			for _, snap := range batchMsg.Snapshots {
				h.stateStore.UpdateState(conn.Identifier, snap.Data)
				ts := parseTimestamp(snap.Timestamp)
				h.persistTelemetry(conn.Identifier, snap.Data, ts)
			}
			log.Printf("[WS] Received telemetry batch from %s (%d snapshots)", conn.Identifier, len(batchMsg.Snapshots))

		case protocol.MsgTypeEvent:
			var eventMsg protocol.EventMessage
			if err := json.Unmarshal(message, &eventMsg); err != nil {
				log.Printf("[WS] Failed to parse event from %s: %v", conn.Identifier, err)
				continue
			}
			conn.IncrementTelemetryReceived()

			// Parse timestamp
			timestamp, err := time.Parse(time.RFC3339, eventMsg.Timestamp)
			if err != nil {
				timestamp = time.Now()
			}

			// Store event
			h.eventStore.AddEvent(conn.Identifier, eventMsg.Event, eventMsg.Data, timestamp)
			if h.db != nil {
				if err := h.db.InsertEvent(conn.Identifier, timestamp, eventMsg.Event, eventMsg.Data); err != nil {
					log.Printf("[WS] Failed to persist event: %v", err)
				}
			}

			eventJSON, _ := json.MarshalIndent(eventMsg.Data, "", "  ")
			log.Printf("[WS] Received EVENT '%s' from %s:\n%s", eventMsg.Event, conn.Identifier, string(eventJSON))

		case protocol.MsgTypeCommandResponse:
			var cmdResp protocol.CommandResponse
			if err := json.Unmarshal(message, &cmdResp); err != nil {
				log.Printf("[WS] Failed to parse command response from %s: %v", conn.Identifier, err)
				continue
			}

			h.responseStore.Store(cmdResp.RequestID, conn.Identifier, "", &cmdResp)

			// Persist terminal outcomes to durable command history. "running"
			// frames are streaming progress and do not close out the record.
			if h.db != nil && cmdResp.Status != "running" {
				status := store.StatusSuccess
				if cmdResp.Status != "success" {
					status = store.StatusFailed
				}
				if err := h.db.UpdateResult(cmdResp.RequestID, status, cmdResp.Result, cmdResp.Error); err != nil {
					log.Printf("[WS] Failed to persist command result: %v", err)
				}
			}

			log.Printf("[WS] Received command response from %s: request_id=%s status=%s",
				conn.Identifier, cmdResp.RequestID, cmdResp.Status)

		default:
			log.Printf("[WS] Unknown message type from %s: %s", conn.Identifier, baseMsg.Type)
		}
	}
}

// messageSender handles outgoing messages from send channel
func (h *WebSocketHandler) messageSender(conn *models.Connection, done <-chan struct{}) {
	for {
		select {
		case <-done:
			return
		case message := <-conn.ReceiveChannel():
			conn.WriteMu.Lock()
			err := conn.Conn.WriteMessage(websocket.TextMessage, message)
			conn.WriteMu.Unlock()

			if err != nil {
				log.Printf("[WS] Write error to %s: %v", conn.Identifier, err)
				return
			}

			conn.AddBytesSent(int64(len(message)))
			conn.IncrementMessagesSent()
		}
	}
}

// keepaliveSender sends periodic keepalive messages and checks idle timeout
func (h *WebSocketHandler) keepaliveSender(conn *models.Connection, done <-chan struct{}) {
	ticker := time.NewTicker(h.keepaliveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			// Check idle timeout
			if h.idleTimeout > 0 {
				idle := time.Since(conn.GetLastSeen())
				if idle > h.idleTimeout {
					log.Printf("[WS] Idle timeout for %s (last seen %s ago)", conn.Identifier, idle.Round(time.Second))
					conn.Conn.Close()
					return
				}
			}

			keepalive := protocol.KeepaliveMessage{
				Type:      protocol.MsgTypeKeepalive,
				Timestamp: protocol.Timestamp(),
			}

			data, err := json.Marshal(keepalive)
			if err != nil {
				log.Printf("[WS] Failed to marshal keepalive for %s: %v", conn.Identifier, err)
				continue
			}

			select {
			case conn.SendChannel() <- data:
				log.Printf("[WS] Sent keepalive to %s", conn.Identifier)
			default:
				log.Printf("[WS] Send channel full for %s, skipping keepalive", conn.Identifier)
			}
		}
	}
}

// sendAuthResponse sends an authentication response
func (h *WebSocketHandler) sendAuthResponse(conn *websocket.Conn, status, errMsg string) {
	response := protocol.AuthResponse{
		Type:       protocol.MsgTypeAuthResponse,
		Status:     status,
		Error:      errMsg,
		ServerTime: protocol.Timestamp(),
	}

	data, err := json.Marshal(response)
	if err != nil {
		log.Printf("[WS] Failed to marshal auth response: %v", err)
		return
	}

	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		log.Printf("[WS] Failed to send auth response: %v", err)
	}
}

// SendCommand sends a command to a scooter
func (h *WebSocketHandler) SendCommand(identifier, command string, params map[string]any) (string, error) {
	conn, exists := h.connMgr.GetConnection(identifier)
	if !exists {
		return "", ErrConnectionNotFound
	}

	if !conn.Authenticated {
		return "", ErrNotAuthenticated
	}

	cmdMsg := protocol.CommandMessage{
		Type:      protocol.MsgTypeCommand,
		RequestID: generateRequestID(),
		Command:   command,
		Params:    params,
		Timestamp: protocol.Timestamp(),
	}

	data, err := json.Marshal(cmdMsg)
	if err != nil {
		return "", err
	}

	select {
	case conn.SendChannel() <- data:
		conn.IncrementCommandsSent()
		if h.db != nil {
			if err := h.db.RecordSent(cmdMsg.RequestID, identifier, command, params); err != nil {
				log.Printf("[WS] Failed to record command: %v", err)
			}
		}
		log.Printf("[WS] Sent command to %s: %s (request_id=%s)", identifier, command, cmdMsg.RequestID)
		return cmdMsg.RequestID, nil
	default:
		return "", ErrSendChannelFull
	}
}

// SendConfigUpdate pushes dotted-path config deltas to an online scooter.
func (h *WebSocketHandler) SendConfigUpdate(identifier string, deltas map[string]string, restart bool) error {
	conn, exists := h.connMgr.GetConnection(identifier)
	if !exists {
		return ErrConnectionNotFound
	}
	if !conn.Authenticated {
		return ErrNotAuthenticated
	}

	msg := protocol.ConfigUpdateMessage{
		Type:      protocol.MsgTypeConfigUpdate,
		Deltas:    deltas,
		Restart:   restart,
		Timestamp: protocol.Timestamp(),
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	select {
	case conn.SendChannel() <- data:
		log.Printf("[WS] Sent config update to %s (%d deltas, restart=%t)", identifier, len(deltas), restart)
		return nil
	default:
		return ErrSendChannelFull
	}
}

// EnqueueCommand persists a command for later delivery to an offline scooter
// and returns the generated request ID.
func (h *WebSocketHandler) EnqueueCommand(identifier, command string, params map[string]any, ttl time.Duration) (string, error) {
	if h.db == nil {
		return "", ErrConnectionNotFound
	}
	requestID := generateRequestID()
	if err := h.db.Enqueue(requestID, identifier, command, params, ttl); err != nil {
		return "", err
	}
	log.Printf("[WS] Queued command for offline scooter %s: %s (request_id=%s)", identifier, command, requestID)
	return requestID, nil
}

// replayQueuedCommands delivers any commands queued while the scooter was
// offline, in enqueue order.
func (h *WebSocketHandler) replayQueuedCommands(conn *models.Connection) {
	if h.db == nil {
		return
	}
	queued, err := h.db.DequeueQueued(conn.Identifier)
	if err != nil {
		log.Printf("[WS] Failed to dequeue commands for %s: %v", conn.Identifier, err)
		return
	}
	for _, qc := range queued {
		cmdMsg := protocol.CommandMessage{
			Type:      protocol.MsgTypeCommand,
			RequestID: qc.RequestID,
			Command:   qc.Command,
			Params:    qc.Params,
			Timestamp: protocol.Timestamp(),
		}
		data, err := json.Marshal(cmdMsg)
		if err != nil {
			continue
		}
		select {
		case conn.SendChannel() <- data:
			conn.IncrementCommandsSent()
			log.Printf("[WS] Replayed queued command to %s: %s (request_id=%s)", conn.Identifier, qc.Command, qc.RequestID)
		default:
			log.Printf("[WS] Send channel full replaying to %s", conn.Identifier)
		}
	}
}

// persistTelemetry stores a full snapshot to durable history.
func (h *WebSocketHandler) persistTelemetry(scooterID string, data map[string]any, ts time.Time) {
	if h.db == nil {
		return
	}
	if err := h.db.InsertTelemetry(scooterID, ts, data); err != nil {
		log.Printf("[WS] Failed to persist telemetry: %v", err)
	}
}

// persistMergedTelemetry stores the post-merge full state after a delta/change.
func (h *WebSocketHandler) persistMergedTelemetry(scooterID string) {
	if h.db == nil {
		return
	}
	if state, ok := h.stateStore.GetState(scooterID); ok {
		h.persistTelemetry(scooterID, state.State, time.Now())
	}
}

// parseTimestamp parses an RFC3339 timestamp, falling back to now for empty or
// non-absolute (monotonic-relative) values.
func parseTimestamp(ts string) time.Time {
	if t, err := time.Parse(time.RFC3339, ts); err == nil {
		return t
	}
	return time.Now()
}

// neverQueue lists commands that must never be deferred to an offline scooter,
// because delivering them late could be unsafe or surprising (physical
// actuation and security-sensitive actions).
var neverQueue = map[string]bool{
	"unlock":       true,
	"open_seatbox": true,
	"force_lock":   true,
}

// IsQueueable reports whether a command may be queued for later delivery.
func IsQueueable(command string) bool {
	return !neverQueue[command]
}

// generateRequestID generates a unique request ID
func generateRequestID() string {
	return time.Now().Format("20060102-150405.000000")
}

// Common errors
var (
	ErrConnectionNotFound = http.ErrBodyNotAllowed // placeholder
	ErrNotAuthenticated   = http.ErrNotSupported   // placeholder
	ErrSendChannelFull    = http.ErrHandlerTimeout // placeholder
)
