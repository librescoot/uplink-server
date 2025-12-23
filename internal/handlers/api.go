package handlers

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/librescoot/uplink-server/internal/storage"
)

// APIHandler handles REST API requests
type APIHandler struct {
	wsHandler     *WebSocketHandler
	connMgr       *storage.ConnectionManager
	responseStore *storage.ResponseStore
	stateStore    *storage.StateStore
	eventStore    *storage.EventStore
	apiKey        string
}

// NewAPIHandler creates a new API handler
func NewAPIHandler(ws *WebSocketHandler, mgr *storage.ConnectionManager, store *storage.ResponseStore, stateStore *storage.StateStore, eventStore *storage.EventStore, apiKey string) *APIHandler {
	return &APIHandler{
		wsHandler:     ws,
		connMgr:       mgr,
		responseStore: store,
		stateStore:    stateStore,
		eventStore:    eventStore,
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

// HandleScooters handles GET /api/scooters and GET /api/scooters/{id}
func (h *APIHandler) HandleScooters(w http.ResponseWriter, r *http.Request) {
	h.cors(h.authenticate(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			h.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}

		if r.URL.Path == "/api/scooters" {
			h.handleListScooters(w, r)
		} else {
			h.writeError(w, http.StatusNotFound, "Use /api/scooters to list or /api/scooters/{id} for details")
		}
	}))(w, r)
}

// HandleScooterDetail handles GET/DELETE /api/scooters/{id}/*
func (h *APIHandler) HandleScooterDetail(w http.ResponseWriter, r *http.Request) {
	h.cors(h.authenticate(func(w http.ResponseWriter, r *http.Request) {
		// Check which endpoint is being requested
		if isCommandHistoryRequest(r.URL.Path) {
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
			if r.Method != http.MethodGet {
				h.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
				return
			}
			scooterID := extractPathParam(r.URL.Path, "/api/scooters/")
			if scooterID == "" {
				h.writeError(w, http.StatusBadRequest, "Scooter ID required")
				return
			}
			h.handleGetScooter(w, r, scooterID)
		}
	}))(w, r)
}

// handleSendCommand sends a command to a scooter
func (h *APIHandler) handleSendCommand(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ScooterID string         `json:"scooter_id"`
		Command   string         `json:"command"`
		Params    map[string]any `json:"params"`
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
	if err != nil {
		if err == ErrConnectionNotFound {
			h.writeError(w, http.StatusNotFound, "Scooter not connected")
		} else if err == ErrSendChannelFull {
			h.writeError(w, http.StatusServiceUnavailable, "Send channel full, try again later")
		} else {
			h.writeError(w, http.StatusInternalServerError, "Failed to send command")
		}
		return
	}

	h.writeJSON(w, http.StatusCreated, map[string]any{
		"request_id": requestID,
		"status":     "sent",
		"message":    "Command sent successfully",
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

// authenticate middleware checks for valid API key
func (h *APIHandler) authenticate(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			next(w, r)
			return
		}

		apiKey := r.Header.Get("X-API-Key")
		if apiKey != h.apiKey {
			h.writeError(w, http.StatusUnauthorized, "Invalid or missing API key")
			return
		}

		next(w, r)
	}
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
