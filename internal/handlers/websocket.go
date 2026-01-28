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
	keepaliveInterval time.Duration
	messageRateLimit  int
	idleTimeout       time.Duration
}

// NewWebSocketHandler creates a new WebSocket handler
func NewWebSocketHandler(authenticator *auth.Authenticator, connMgr *storage.ConnectionManager, responseStore *storage.ResponseStore, stateStore *storage.StateStore, eventStore *storage.EventStore, keepaliveInterval time.Duration, messageRateLimit int, idleTimeout time.Duration) *WebSocketHandler {
	return &WebSocketHandler{
		auth:              authenticator,
		connMgr:           connMgr,
		responseStore:     responseStore,
		stateStore:        stateStore,
		eventStore:        eventStore,
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

			changeJSON, _ := json.MarshalIndent(changeMsg.Changes, "", "  ")
			log.Printf("[WS] Received state changes from %s:\n%s", conn.Identifier, string(changeJSON))

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

			eventJSON, _ := json.MarshalIndent(eventMsg.Data, "", "  ")
			log.Printf("[WS] Received EVENT '%s' from %s:\n%s", eventMsg.Event, conn.Identifier, string(eventJSON))

		case protocol.MsgTypeCommandResponse:
			var cmdResp protocol.CommandResponse
			if err := json.Unmarshal(message, &cmdResp); err != nil {
				log.Printf("[WS] Failed to parse command response from %s: %v", conn.Identifier, err)
				continue
			}

			h.responseStore.Store(cmdResp.RequestID, conn.Identifier, "", &cmdResp)

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
		log.Printf("[WS] Sent command to %s: %s (request_id=%s)", identifier, command, cmdMsg.RequestID)
		return cmdMsg.RequestID, nil
	default:
		return "", ErrSendChannelFull
	}
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
