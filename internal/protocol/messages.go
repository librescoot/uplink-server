package protocol

import "time"

// MessageType represents the type of message
type MessageType string

const (
	// Client → Server
	MsgTypeAuth            MessageType = "auth"
	MsgTypeState           MessageType = "state"
	MsgTypeChange          MessageType = "change"
	MsgTypeTelemetryDelta  MessageType = "telemetry_delta"
	MsgTypeTelemetryBatch  MessageType = "telemetry_batch"
	MsgTypeEvent           MessageType = "event"
	MsgTypeKeepalive       MessageType = "keepalive"
	MsgTypeCommandResponse MessageType = "command_response"

	// Server → Client
	MsgTypeAuthResponse MessageType = "auth_response"
	MsgTypeCommand      MessageType = "command"
	MsgTypeConfigUpdate MessageType = "config_update"
)

// BaseMessage is the base structure for all messages
type BaseMessage struct {
	Type      MessageType `json:"type"`
	Timestamp string      `json:"timestamp"`
}

// AuthMessage - Client authenticates with server
type AuthMessage struct {
	Type            MessageType `json:"type"`
	Identifier      string      `json:"identifier"`
	Token           string      `json:"token"`
	Version         string      `json:"version"`
	ProtocolVersion int         `json:"protocol_version"`
	Timestamp       string      `json:"timestamp"`
}

// AuthResponse - Server responds to authentication
type AuthResponse struct {
	Type       MessageType `json:"type"`
	Status     string      `json:"status"` // "success" or "error"
	Error      string      `json:"error,omitempty"`
	ServerTime string      `json:"server_time"`
}

// StateMessage - Client sends full state snapshot
// Data uses nested object structure where top-level keys are component identifiers
// (e.g., "battery:0", "vehicle", "engine-ecu") and values are objects containing
// the component's fields. This preserves semantic grouping and enables easier querying.
//
// Example:
//
//	{
//	  "battery:0": {"charge": "64", "voltage": "54214"},
//	  "vehicle": {"state": "stand-by"},
//	  "engine-ecu": {"speed": "0", "odometer": "1234567"}
//	}
type StateMessage struct {
	Type      MessageType    `json:"type"`
	Data      map[string]any `json:"data"`
	Timestamp string         `json:"timestamp"`
}

// ChangeMessage - Client sends field-level deltas
// Changes uses nested object structure matching StateMessage format.
// Only changed fields need to be included for each component.
//
// Example:
//
//	{
//	  "battery:0": {"charge": "65", "current": "-180"},
//	  "engine-ecu": {"speed": "25"}
//	}
type ChangeMessage struct {
	Type      MessageType    `json:"type"`
	Changes   map[string]any `json:"changes"`
	Timestamp string         `json:"timestamp"`
}

// TelemetryDeltaMessage - Client sends changed leaves plus a list of removed
// paths (dotted "hash.field" keys). Changes are deep-merged into stored state;
// each removed path is then deleted, because a merge alone can never remove a
// key.
type TelemetryDeltaMessage struct {
	Type      MessageType    `json:"type"`
	Changes   map[string]any `json:"changes"`
	Removed   []string       `json:"removed,omitempty"`
	Timestamp string         `json:"timestamp"`
}

// TelemetrySnapshot is one timestamped full-state snapshot within a batch.
type TelemetrySnapshot struct {
	Data      map[string]any `json:"data"`
	Timestamp string         `json:"timestamp"`
}

// TelemetryBatchMessage - Client replays buffered offline snapshots.
type TelemetryBatchMessage struct {
	Type      MessageType         `json:"type"`
	Snapshots []TelemetrySnapshot `json:"snapshots"`
	Timestamp string              `json:"timestamp"`
}

// EventMessage - Client sends critical event
type EventMessage struct {
	Type      MessageType    `json:"type"`
	Event     string         `json:"event"` // event name/type
	Data      map[string]any `json:"data"`
	Timestamp string         `json:"timestamp"`
}

// KeepaliveMessage - Bidirectional keepalive
type KeepaliveMessage struct {
	Type      MessageType `json:"type"`
	Timestamp string      `json:"timestamp"`
}

// CommandMessage - Server sends command to client
type CommandMessage struct {
	Type      MessageType    `json:"type"`
	RequestID string         `json:"request_id"`
	Command   string         `json:"command"`
	Params    map[string]any `json:"params,omitempty"`
	Timestamp string         `json:"timestamp"`
}

// CommandResponse - Client responds to command
type CommandResponse struct {
	Type      MessageType    `json:"type"`
	RequestID string         `json:"request_id"`
	Status    string         `json:"status"` // "success", "error", "running"
	Result    map[string]any `json:"result,omitempty"`
	Error     string         `json:"error,omitempty"`
	Timestamp string         `json:"timestamp"`
}

// ConfigUpdateMessage - Server pushes dotted-path config deltas to the client.
type ConfigUpdateMessage struct {
	Type      MessageType       `json:"type"`
	Deltas    map[string]string `json:"deltas"`
	Restart   bool              `json:"restart,omitempty"`
	Timestamp string            `json:"timestamp"`
}

// Helper function to create timestamp string
func Timestamp() string {
	return time.Now().UTC().Format(time.RFC3339)
}
