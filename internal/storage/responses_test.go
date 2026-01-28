package storage

import (
	"testing"
	"time"

	"github.com/librescoot/uplink-server/internal/protocol"
)

func TestResponseStore_StoreAndGet(t *testing.T) {
	rs := NewResponseStore(time.Hour)

	resp := &protocol.CommandResponse{
		Type:      protocol.MsgTypeCommandResponse,
		RequestID: "req-1",
		Status:    "success",
	}

	rs.Store("req-1", "s1", "lock", resp)

	record, exists := rs.Get("req-1")
	if !exists {
		t.Fatal("expected record to exist")
	}
	if record.RequestID != "req-1" {
		t.Fatalf("expected req-1, got %s", record.RequestID)
	}
	if record.ScooterID != "s1" {
		t.Fatalf("expected s1, got %s", record.ScooterID)
	}
	if record.Command != "lock" {
		t.Fatalf("expected lock, got %s", record.Command)
	}
	if record.Response.Status != "success" {
		t.Fatalf("expected success, got %s", record.Response.Status)
	}
}

func TestResponseStore_GetNonexistent(t *testing.T) {
	rs := NewResponseStore(time.Hour)

	_, exists := rs.Get("nonexistent")
	if exists {
		t.Fatal("expected record to not exist")
	}
}

func TestResponseStore_GetByScooter(t *testing.T) {
	rs := NewResponseStore(time.Hour)

	resp1 := &protocol.CommandResponse{RequestID: "req-1", Status: "success"}
	resp2 := &protocol.CommandResponse{RequestID: "req-2", Status: "success"}
	resp3 := &protocol.CommandResponse{RequestID: "req-3", Status: "error"}

	rs.Store("req-1", "s1", "lock", resp1)
	rs.Store("req-2", "s1", "unlock", resp2)
	rs.Store("req-3", "s2", "lock", resp3)

	results := rs.GetByScooter("s1")
	if len(results) != 2 {
		t.Fatalf("expected 2 results for s1, got %d", len(results))
	}

	results = rs.GetByScooter("s2")
	if len(results) != 1 {
		t.Fatalf("expected 1 result for s2, got %d", len(results))
	}

	results = rs.GetByScooter("nonexistent")
	if len(results) != 0 {
		t.Fatalf("expected 0 results for nonexistent, got %d", len(results))
	}
}

func TestResponseStore_Overwrite(t *testing.T) {
	rs := NewResponseStore(time.Hour)

	resp1 := &protocol.CommandResponse{RequestID: "req-1", Status: "running"}
	resp2 := &protocol.CommandResponse{RequestID: "req-1", Status: "success"}

	rs.Store("req-1", "s1", "lock", resp1)
	rs.Store("req-1", "s1", "lock", resp2)

	record, _ := rs.Get("req-1")
	if record.Response.Status != "success" {
		t.Fatalf("expected overwritten status=success, got %s", record.Response.Status)
	}
}
