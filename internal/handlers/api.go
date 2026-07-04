package handlers

import (
	"crypto/subtle"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/librescoot/uplink-server/internal/registry"
	"github.com/librescoot/uplink-server/internal/session"
	"github.com/librescoot/uplink-server/internal/storage"
	"github.com/librescoot/uplink-server/internal/store"
)

// defaultQueueTTL is how long an offline-queued command remains deliverable
// before it expires.
const defaultQueueTTL = time.Hour

// APIHandler handles REST API requests
type APIHandler struct {
	wsHandler     *WebSocketHandler
	connMgr       *storage.ConnectionManager
	responseStore *storage.ResponseStore
	stateStore    *storage.StateStore
	eventStore    *storage.EventStore
	db            *store.Store       // durable persistence; may be nil
	registry      *registry.Registry // scooter registration; may be nil
	sessions      *session.Store     // login sessions; may be nil
	users         map[string]string  // username -> password
	apiKey        string
}

// NewAPIHandler creates a new API handler. db and reg may be nil to disable
// durable history endpoints and runtime scooter registration respectively.
func NewAPIHandler(ws *WebSocketHandler, mgr *storage.ConnectionManager, respStore *storage.ResponseStore, stateStore *storage.StateStore, eventStore *storage.EventStore, db *store.Store, reg *registry.Registry, sessions *session.Store, users map[string]string, apiKey string) *APIHandler {
	return &APIHandler{
		wsHandler:     ws,
		connMgr:       mgr,
		responseStore: respStore,
		stateStore:    stateStore,
		eventStore:    eventStore,
		db:            db,
		registry:      reg,
		sessions:      sessions,
		users:         users,
		apiKey:        apiKey,
	}
}

// HandleCommands handles POST /api/commands and GET /api/commands
func (h *APIHandler) HandleCommands(w http.ResponseWriter, r *http.Request) {
	h.cors(h.authenticate(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			h.handleSendCommand(w, r)
		} else if r.Method == http.MethodGet {
			h.writeError(w, http.StatusMethodNotAllowed, "Use POST to send commands or GET /api/commands/{request_id} to retrieve")
		} else {
			h.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		}
	}))(w, r)
}

// HandleCommandResponse handles GET /api/commands/{request_id}
func (h *APIHandler) HandleCommandResponse(w http.ResponseWriter, r *http.Request) {
	h.cors(h.authenticate(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			h.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}

		requestID := extractPathParam(r.URL.Path, "/api/commands/")
		if requestID == "" {
			h.writeError(w, http.StatusBadRequest, "Request ID required")
			return
		}

		h.handleGetCommandResponse(w, r, requestID)
	}))(w, r)
}

// HandleScooters handles GET /api/scooters (list connected) and
// POST /api/scooters (register a new scooter).
func (h *APIHandler) HandleScooters(w http.ResponseWriter, r *http.Request) {
	h.cors(h.authenticate(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/scooters" {
			h.writeError(w, http.StatusNotFound, "Use /api/scooters to list or /api/scooters/{id} for details")
			return
		}
		switch r.Method {
		case http.MethodGet:
			h.handleListScooters(w, r)
		case http.MethodPost:
			h.handleCreateScooter(w, r)
		default:
			h.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		}
	}))(w, r)
}

// HandleRegistry handles GET /api/registry: all registered scooters with a
// connected flag (including those currently offline).
func (h *APIHandler) HandleRegistry(w http.ResponseWriter, r *http.Request) {
	h.cors(h.authenticate(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			h.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}
		if h.registry == nil {
			h.writeError(w, http.StatusServiceUnavailable, "Registry is not enabled")
			return
		}
		list := h.registry.List()
		scooters := make([]map[string]any, 0, len(list))
		for _, s := range list {
			_, connected := h.connMgr.GetConnection(s.Identifier)
			scooters = append(scooters, map[string]any{
				"identifier": s.Identifier,
				"name":       s.Name,
				"connected":  connected,
			})
		}
		h.writeJSON(w, http.StatusOK, map[string]any{
			"scooters": scooters,
			"total":    len(scooters),
		})
	}))(w, r)
}

// handleCreateScooter registers a new scooter and returns its generated token.
func (h *APIHandler) handleCreateScooter(w http.ResponseWriter, r *http.Request) {
	if h.registry == nil {
		h.writeError(w, http.StatusServiceUnavailable, "Registry is not enabled")
		return
	}
	var req struct {
		Identifier string `json:"identifier"`
		Name       string `json:"name"`
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "Failed to read request body")
		return
	}
	if err := json.Unmarshal(body, &req); err != nil {
		h.writeError(w, http.StatusBadRequest, "Invalid JSON format")
		return
	}
	if req.Identifier == "" {
		h.writeError(w, http.StatusBadRequest, "identifier is required")
		return
	}

	token, err := h.registry.Add(req.Identifier, req.Name)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			h.writeError(w, http.StatusConflict, err.Error())
		} else {
			h.writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	h.writeJSON(w, http.StatusCreated, map[string]any{
		"identifier": req.Identifier,
		"name":       req.Name,
		"token":      token,
	})
}

// handleDeleteScooter removes a registered scooter.
func (h *APIHandler) handleDeleteScooter(w http.ResponseWriter, r *http.Request, scooterID string) {
	if h.registry == nil {
		h.writeError(w, http.StatusServiceUnavailable, "Registry is not enabled")
		return
	}
	if err := h.registry.Delete(scooterID); err != nil {
		h.writeError(w, http.StatusNotFound, err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{
		"identifier": scooterID,
		"message":    "Scooter removed",
	})
}

// HandleScooterDetail handles GET/DELETE /api/scooters/{id}/*
func (h *APIHandler) HandleScooterDetail(w http.ResponseWriter, r *http.Request) {
	h.cors(h.authenticate(func(w http.ResponseWriter, r *http.Request) {
		// Check which endpoint is being requested
		if isConfigRequest(r.URL.Path) {
			if r.Method != http.MethodPost {
				h.writeError(w, http.StatusMethodNotAllowed, "Use POST to push config")
				return
			}
			scooterID := extractScooterIDForSuffix(r.URL.Path, "/config")
			if scooterID == "" {
				h.writeError(w, http.StatusBadRequest, "Scooter ID required")
				return
			}
			h.handlePushConfig(w, r, scooterID)
		} else if isHistoryRequest(r.URL.Path) {
			if r.Method != http.MethodGet {
				h.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
				return
			}
			scooterID := extractScooterIDForSuffix(r.URL.Path, "/history")
			if scooterID == "" {
				h.writeError(w, http.StatusBadRequest, "Scooter ID required")
				return
			}
			h.handleGetScooterHistory(w, r, scooterID)
		} else if isCommandHistoryRequest(r.URL.Path) {
			if r.Method != http.MethodGet {
				h.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
				return
			}
			scooterID := extractScooterIDFromCommandPath(r.URL.Path)
			if scooterID == "" {
				h.writeError(w, http.StatusBadRequest, "Scooter ID required")
				return
			}
			h.handleGetScooterCommands(w, r, scooterID)
		} else if isStateRequest(r.URL.Path) {
			if r.Method != http.MethodGet {
				h.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
				return
			}
			scooterID := extractScooterIDFromStatePath(r.URL.Path)
			if scooterID == "" {
				h.writeError(w, http.StatusBadRequest, "Scooter ID required")
				return
			}
			h.handleGetScooterState(w, r, scooterID)
		} else if isEventsRequest(r.URL.Path) {
			scooterID, eventID := extractScooterIDAndEventIDFromEventsPath(r.URL.Path)
			if scooterID == "" {
				h.writeError(w, http.StatusBadRequest, "Scooter ID required")
				return
			}

			if r.Method == http.MethodGet {
				h.handleGetScooterEvents(w, r, scooterID)
			} else if r.Method == http.MethodDelete {
				if eventID == "" {
					// DELETE /api/scooters/{id}/events - clear all events
					h.handleClearScooterEvents(w, r, scooterID)
				} else {
					// DELETE /api/scooters/{id}/events/{eventID} - delete single event
					h.handleDeleteScooterEvent(w, r, scooterID, eventID)
				}
			} else {
				h.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
			}
		} else {
			scooterID := extractPathParam(r.URL.Path, "/api/scooters/")
			if scooterID == "" {
				h.writeError(w, http.StatusBadRequest, "Scooter ID required")
				return
			}
			switch r.Method {
			case http.MethodGet:
				h.handleGetScooter(w, r, scooterID)
			case http.MethodDelete:
				h.handleDeleteScooter(w, r, scooterID)
			default:
				h.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
			}
		}
	}))(w, r)
}

// handleSendCommand sends a command to a scooter
func (h *APIHandler) handleSendCommand(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ScooterID string         `json:"scooter_id"`
		Command   string         `json:"command"`
		Params    map[string]any `json:"params"`
		Queue     bool           `json:"queue"`
		TTL       string         `json:"ttl"`
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "Failed to read request body")
		return
	}

	if err := json.Unmarshal(body, &req); err != nil {
		h.writeError(w, http.StatusBadRequest, "Invalid JSON format")
		return
	}

	if req.ScooterID == "" || req.Command == "" {
		h.writeError(w, http.StatusBadRequest, "scooter_id and command are required")
		return
	}

	if req.Params == nil {
		req.Params = make(map[string]any)
	}

	requestID, err := h.wsHandler.SendCommand(req.ScooterID, req.Command, req.Params)
	if err == nil {
		h.writeJSON(w, http.StatusCreated, map[string]any{
			"request_id": requestID,
			"status":     "sent",
			"message":    "Command sent successfully",
		})
		return
	}

	if err == ErrSendChannelFull {
		h.writeError(w, http.StatusServiceUnavailable, "Send channel full, try again later")
		return
	}
	if err != ErrConnectionNotFound {
		h.writeError(w, http.StatusInternalServerError, "Failed to send command")
		return
	}

	// Offline. Queue when persistence is available and the command is safe to
	// defer; otherwise report not connected.
	if h.db == nil || !req.Queue {
		h.writeError(w, http.StatusNotFound, "Scooter not connected")
		return
	}
	if !IsQueueable(req.Command) {
		h.writeError(w, http.StatusConflict, "Command may not be queued while offline")
		return
	}

	ttl := defaultQueueTTL
	if req.TTL != "" {
		if d, perr := time.ParseDuration(req.TTL); perr == nil {
			ttl = d
		}
	}
	queuedID, qerr := h.wsHandler.EnqueueCommand(req.ScooterID, req.Command, req.Params, ttl)
	if qerr != nil {
		h.writeError(w, http.StatusInternalServerError, "Failed to queue command")
		return
	}
	h.writeJSON(w, http.StatusAccepted, map[string]any{
		"request_id": queuedID,
		"status":     "queued",
		"message":    "Scooter offline; command queued for delivery on reconnect",
	})
}

// handleGetCommandResponse retrieves a command response by request ID
func (h *APIHandler) handleGetCommandResponse(w http.ResponseWriter, r *http.Request, requestID string) {
	record, exists := h.responseStore.Get(requestID)
	if !exists {
		h.writeJSON(w, http.StatusOK, map[string]any{
			"request_id": requestID,
			"status":     "pending",
			"message":    "Response not yet received",
		})
		return
	}

	response := map[string]any{
		"request_id":  record.RequestID,
		"scooter_id":  record.ScooterID,
		"status":      record.Response.Status,
		"received_at": record.ReceivedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	if record.Command != "" {
		response["command"] = record.Command
	}

	if record.Response.Result != nil {
		response["result"] = record.Response.Result
	}

	if record.Response.Error != "" {
		response["error"] = record.Response.Error
	}

	h.writeJSON(w, http.StatusOK, response)
}

// handleListScooters lists all connected scooters
func (h *APIHandler) handleListScooters(w http.ResponseWriter, r *http.Request) {
	connections := h.connMgr.GetAllConnections()

	scooters := make([]map[string]any, 0, len(connections))
	for _, conn := range connections {
		stats := conn.GetStats()
		scooters = append(scooters, map[string]any{
			"identifier":     stats["identifier"],
			"version":        stats["version"],
			"connected_at":   stats["connected_at"],
			"last_seen":      stats["last_seen"],
			"uptime_seconds": stats["uptime_seconds"],
			"authenticated":  stats["authenticated"],
		})
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"scooters": scooters,
		"total":    len(scooters),
	})
}

// handleGetScooter retrieves details for a specific scooter
func (h *APIHandler) handleGetScooter(w http.ResponseWriter, r *http.Request, scooterID string) {
	conn, exists := h.connMgr.GetConnection(scooterID)
	if !exists {
		h.writeError(w, http.StatusNotFound, "Scooter not connected")
		return
	}

	stats := conn.GetStats()
	h.writeJSON(w, http.StatusOK, stats)
}

// handleGetScooterCommands retrieves command history for a scooter
func (h *APIHandler) handleGetScooterCommands(w http.ResponseWriter, r *http.Request, scooterID string) {
	_, exists := h.connMgr.GetConnection(scooterID)
	if !exists {
		h.writeError(w, http.StatusNotFound, "Scooter not connected")
		return
	}

	records := h.responseStore.GetByScooter(scooterID)

	commands := make([]map[string]any, 0, len(records))
	for _, record := range records {
		cmd := map[string]any{
			"request_id":  record.RequestID,
			"status":      record.Response.Status,
			"received_at": record.ReceivedAt.Format("2006-01-02T15:04:05Z07:00"),
		}

		if record.Command != "" {
			cmd["command"] = record.Command
		}

		commands = append(commands, cmd)
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"scooter_id": scooterID,
		"commands":   commands,
		"total":      len(commands),
	})
}

// authenticate middleware accepts either the configured API key or a valid
// login session token, both presented via X-API-Key.
func (h *APIHandler) authenticate(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			next(w, r)
			return
		}

		if !h.validCredential(r.Header.Get("X-API-Key")) {
			h.writeError(w, http.StatusUnauthorized, "Invalid or missing credentials")
			return
		}

		next(w, r)
	}
}

// validCredential reports whether key is the configured API key or a valid
// session token.
func (h *APIHandler) validCredential(key string) bool {
	if key == "" {
		return false
	}
	if subtle.ConstantTimeCompare([]byte(key), []byte(h.apiKey)) == 1 {
		return true
	}
	if h.sessions != nil {
		if _, ok := h.sessions.Validate(key); ok {
			return true
		}
	}
	return false
}

// HandleLogin handles POST /api/login with a username/password body and returns
// a session token on success. This endpoint is intentionally unauthenticated.
func (h *APIHandler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	h.cors(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			h.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}
		if h.sessions == nil || len(h.users) == 0 {
			h.writeError(w, http.StatusServiceUnavailable, "Password login is not enabled")
			return
		}

		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			h.writeError(w, http.StatusBadRequest, "Failed to read request body")
			return
		}
		if err := json.Unmarshal(body, &req); err != nil {
			h.writeError(w, http.StatusBadRequest, "Invalid JSON format")
			return
		}

		if !h.checkPassword(req.Username, req.Password) {
			h.writeError(w, http.StatusUnauthorized, "Invalid username or password")
			return
		}

		token, ttl, err := h.sessions.Create(req.Username)
		if err != nil {
			h.writeError(w, http.StatusInternalServerError, "Failed to create session")
			return
		}
		h.writeJSON(w, http.StatusOK, map[string]any{
			"token":      token,
			"expires_in": int(ttl.Seconds()),
			"username":   req.Username,
		})
	})(w, r)
}

// HandleLogout handles POST /api/logout, invalidating the presented session
// token.
func (h *APIHandler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	h.cors(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			h.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}
		if h.sessions != nil {
			h.sessions.Delete(r.Header.Get("X-API-Key"))
		}
		h.writeJSON(w, http.StatusOK, map[string]any{"message": "Logged out"})
	})(w, r)
}

// checkPassword validates credentials in constant time against the configured
// users. It always performs a comparison to avoid leaking which usernames
// exist via timing.
func (h *APIHandler) checkPassword(username, password string) bool {
	stored, ok := h.users[username]
	if !ok {
		// Compare against a dummy value so timing is similar for unknown users.
		subtle.ConstantTimeCompare([]byte(password), []byte("dummy-placeholder"))
		return false
	}
	return subtle.ConstantTimeCompare([]byte(password), []byte(stored)) == 1
}

// cors middleware adds CORS headers
func (h *APIHandler) cors(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next(w, r)
	}
}

// writeJSON writes a JSON response
func (h *APIHandler) writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("[API] Failed to encode JSON response: %v", err)
	}
}

// writeError writes an error JSON response
func (h *APIHandler) writeError(w http.ResponseWriter, status int, message string) {
	h.writeJSON(w, status, map[string]any{
		"error": message,
	})
}

// extractPathParam extracts a path parameter from a URL
func extractPathParam(path, prefix string) string {
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	return strings.TrimPrefix(path, prefix)
}

// handleGetScooterState retrieves the latest state for a scooter
func (h *APIHandler) handleGetScooterState(w http.ResponseWriter, r *http.Request, scooterID string) {
	_, exists := h.connMgr.GetConnection(scooterID)
	if !exists {
		h.writeError(w, http.StatusNotFound, "Scooter not connected")
		return
	}

	state, exists := h.stateStore.GetState(scooterID)
	if !exists {
		h.writeJSON(w, http.StatusOK, map[string]any{
			"scooter_id": scooterID,
			"state":      map[string]any{},
			"message":    "No state data available yet",
		})
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"scooter_id":   scooterID,
		"state":        state.State,
		"last_updated": state.LastUpdated.Format("2006-01-02T15:04:05Z07:00"),
	})
}

// isCommandHistoryRequest checks if path is for command history
func isCommandHistoryRequest(path string) bool {
	return strings.HasSuffix(path, "/commands")
}

// isHistoryRequest checks if path is for durable telemetry history
func isHistoryRequest(path string) bool {
	return strings.HasSuffix(path, "/history")
}

// isConfigRequest checks if path is for config push
func isConfigRequest(path string) bool {
	return strings.HasSuffix(path, "/config")
}

// handlePushConfig sends dotted-path config deltas to a scooter.
func (h *APIHandler) handlePushConfig(w http.ResponseWriter, r *http.Request, scooterID string) {
	var req struct {
		Deltas  map[string]string `json:"deltas"`
		Restart bool              `json:"restart"`
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "Failed to read request body")
		return
	}
	if err := json.Unmarshal(body, &req); err != nil {
		h.writeError(w, http.StatusBadRequest, "Invalid JSON format")
		return
	}
	if len(req.Deltas) == 0 {
		h.writeError(w, http.StatusBadRequest, "deltas are required")
		return
	}

	err = h.wsHandler.SendConfigUpdate(scooterID, req.Deltas, req.Restart)
	if err == ErrConnectionNotFound {
		h.writeError(w, http.StatusNotFound, "Scooter not connected")
		return
	}
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "Failed to push config")
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{
		"scooter_id": scooterID,
		"status":     "sent",
		"deltas":     len(req.Deltas),
	})
}

// extractScooterIDForSuffix extracts the scooter ID from
// /api/scooters/{id}{suffix}
func extractScooterIDForSuffix(path, suffix string) string {
	path = strings.TrimPrefix(path, "/api/scooters/")
	return strings.TrimSuffix(path, suffix)
}

// handleGetScooterHistory returns persisted telemetry history for a scooter.
// Accepts optional ?from and ?to (RFC3339) and ?limit query parameters.
func (h *APIHandler) handleGetScooterHistory(w http.ResponseWriter, r *http.Request, scooterID string) {
	if h.db == nil {
		h.writeError(w, http.StatusServiceUnavailable, "History persistence is not enabled")
		return
	}

	to := time.Now()
	from := to.Add(-24 * time.Hour)
	if v := r.URL.Query().Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			from = t
		}
	}
	if v := r.URL.Query().Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			to = t
		}
	}
	limit := 1000
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	rows, err := h.db.QueryTelemetry(scooterID, from, to, limit)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "Failed to query history")
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"scooter_id": scooterID,
		"from":       from.Format(time.RFC3339),
		"to":         to.Format(time.RFC3339),
		"count":      len(rows),
		"history":    rows,
	})
}

// isStateRequest checks if path is for state data
func isStateRequest(path string) bool {
	return strings.HasSuffix(path, "/state")
}

// extractScooterIDFromCommandPath extracts scooter ID from /api/scooters/{id}/commands
func extractScooterIDFromCommandPath(path string) string {
	path = strings.TrimPrefix(path, "/api/scooters/")
	path = strings.TrimSuffix(path, "/commands")
	return path
}

// extractScooterIDFromStatePath extracts scooter ID from /api/scooters/{id}/state
func extractScooterIDFromStatePath(path string) string {
	path = strings.TrimPrefix(path, "/api/scooters/")
	path = strings.TrimSuffix(path, "/state")
	return path
}

// isEventsRequest checks if path is for events data
func isEventsRequest(path string) bool {
	return strings.HasSuffix(path, "/events") || strings.Contains(path, "/events/")
}

// extractScooterIDFromEventsPath extracts scooter ID from /api/scooters/{id}/events
func extractScooterIDFromEventsPath(path string) string {
	path = strings.TrimPrefix(path, "/api/scooters/")
	path = strings.TrimSuffix(path, "/events")
	return path
}

// extractScooterIDAndEventIDFromEventsPath extracts scooter ID and event ID from /api/scooters/{id}/events[/{eventID}]
func extractScooterIDAndEventIDFromEventsPath(path string) (string, string) {
	path = strings.TrimPrefix(path, "/api/scooters/")
	parts := strings.Split(path, "/events/")
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	path = strings.TrimSuffix(parts[0], "/events")
	return path, ""
}

// handleGetScooterEvents retrieves events for a scooter
func (h *APIHandler) handleGetScooterEvents(w http.ResponseWriter, r *http.Request, scooterID string) {
	_, exists := h.connMgr.GetConnection(scooterID)
	if !exists {
		h.writeError(w, http.StatusNotFound, "Scooter not connected")
		return
	}

	events := h.eventStore.GetEvents(scooterID, 100) // Get last 100 events

	h.writeJSON(w, http.StatusOK, map[string]any{
		"scooter_id": scooterID,
		"events":     events,
		"total":      len(events),
	})
}

// handleDeleteScooterEvent deletes a single event
func (h *APIHandler) handleDeleteScooterEvent(w http.ResponseWriter, r *http.Request, scooterID, eventID string) {
	_, exists := h.connMgr.GetConnection(scooterID)
	if !exists {
		h.writeError(w, http.StatusNotFound, "Scooter not connected")
		return
	}

	deleted := h.eventStore.DeleteEvent(scooterID, eventID)
	if !deleted {
		h.writeError(w, http.StatusNotFound, "Event not found")
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"message": "Event deleted",
	})
}

// handleClearScooterEvents clears all events for a scooter
func (h *APIHandler) handleClearScooterEvents(w http.ResponseWriter, r *http.Request, scooterID string) {
	_, exists := h.connMgr.GetConnection(scooterID)
	if !exists {
		h.writeError(w, http.StatusNotFound, "Scooter not connected")
		return
	}

	h.eventStore.ClearEvents(scooterID)

	h.writeJSON(w, http.StatusOK, map[string]any{
		"message": "All events cleared",
	})
}
