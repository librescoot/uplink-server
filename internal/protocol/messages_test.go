package protocol

import (
	"encoding/json"
	"testing"
)

func TestAuthMessageSerialization(t *testing.T) {
	msg := AuthMessage{
		Type:            MsgTypeAuth,
		Identifier:      "scooter-1",
		Token:           "secret",
		Version:         "1.0.0",
		ProtocolVersion: 1,
		Timestamp:       "2025-01-01T00:00:00Z",
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded AuthMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Type != MsgTypeAuth {
		t.Errorf("type = %v, want %v", decoded.Type, MsgTypeAuth)
	}
	if decoded.Identifier != "scooter-1" {
		t.Errorf("identifier = %v, want scooter-1", decoded.Identifier)
	}
	if decoded.Token != "secret" {
		t.Errorf("token = %v, want secret", decoded.Token)
	}
}

func TestStateMessageSerialization(t *testing.T) {
	msg := StateMessage{
		Type: MsgTypeState,
		Data: map[string]any{
			"battery:0": map[string]any{"charge": "64"},
			"vehicle":   map[string]any{"state": "stand-by"},
		},
		Timestamp: "2025-01-01T00:00:00Z",
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded StateMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	battery, ok := decoded.Data["battery:0"].(map[string]any)
	if !ok {
		t.Fatal("expected battery:0 to be a map")
	}
	if battery["charge"] != "64" {
		t.Errorf("charge = %v, want 64", battery["charge"])
	}
}

func TestCommandMessageSerialization(t *testing.T) {
	msg := CommandMessage{
		Type:      MsgTypeCommand,
		RequestID: "req-123",
		Command:   "lock",
		Params:    map[string]any{"force": true},
		Timestamp: "2025-01-01T00:00:00Z",
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded CommandMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Command != "lock" {
		t.Errorf("command = %v, want lock", decoded.Command)
	}
	if decoded.RequestID != "req-123" {
		t.Errorf("request_id = %v, want req-123", decoded.RequestID)
	}
}

func TestCommandResponseSerialization(t *testing.T) {
	msg := CommandResponse{
		Type:      MsgTypeCommandResponse,
		RequestID: "req-123",
		Status:    "success",
		Result:    map[string]any{"message": "done"},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded CommandResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Status != "success" {
		t.Errorf("status = %v, want success", decoded.Status)
	}
}

func TestBaseMessageTypeRouting(t *testing.T) {
	messages := []struct {
		json     string
		expected MessageType
	}{
		{`{"type":"auth"}`, MsgTypeAuth},
		{`{"type":"state"}`, MsgTypeState},
		{`{"type":"change"}`, MsgTypeChange},
		{`{"type":"event"}`, MsgTypeEvent},
		{`{"type":"keepalive"}`, MsgTypeKeepalive},
		{`{"type":"command_response"}`, MsgTypeCommandResponse},
		{`{"type":"auth_response"}`, MsgTypeAuthResponse},
		{`{"type":"command"}`, MsgTypeCommand},
	}

	for _, tt := range messages {
		var base BaseMessage
		if err := json.Unmarshal([]byte(tt.json), &base); err != nil {
			t.Errorf("unmarshal %q: %v", tt.json, err)
			continue
		}
		if base.Type != tt.expected {
			t.Errorf("type from %q = %v, want %v", tt.json, base.Type, tt.expected)
		}
	}
}

func TestTimestamp(t *testing.T) {
	ts := Timestamp()
	if ts == "" {
		t.Fatal("Timestamp() returned empty string")
	}
}

func TestCommandResponseOmitsEmpty(t *testing.T) {
	msg := CommandResponse{
		Type:      MsgTypeCommandResponse,
		RequestID: "req-123",
		Status:    "success",
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]any
	json.Unmarshal(data, &raw)

	if _, exists := raw["result"]; exists {
		t.Error("result should be omitted when nil")
	}
	if _, exists := raw["error"]; exists {
		t.Error("error should be omitted when empty")
	}
}
