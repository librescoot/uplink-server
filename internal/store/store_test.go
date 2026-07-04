package store

import (
	"path/filepath"
	"testing"
	"time"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestTelemetryRoundTrip(t *testing.T) {
	s := openTemp(t)

	data := map[string]any{
		"gps":        map[string]any{"latitude": "52.5", "longitude": "13.4"},
		"engine-ecu": map[string]any{"speed": "25"},
		"vehicle":    map[string]any{"state": "ready-to-drive"},
	}
	now := time.Now()
	if err := s.InsertTelemetry("VIN1", now, data); err != nil {
		t.Fatalf("insert: %v", err)
	}

	rows, err := s.QueryTelemetry("VIN1", now.Add(-time.Minute), now.Add(time.Minute), 10)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	r := rows[0]
	if r.State != "ready-to-drive" {
		t.Errorf("state = %q", r.State)
	}
	if r.Lat == nil || *r.Lat != 52.5 {
		t.Errorf("lat = %v", r.Lat)
	}
	if r.Speed == nil || *r.Speed != 25 {
		t.Errorf("speed = %v", r.Speed)
	}
	if r.Data["vehicle"].(map[string]any)["state"] != "ready-to-drive" {
		t.Errorf("data blob not preserved")
	}
}

func TestPruneTelemetry(t *testing.T) {
	s := openTemp(t)
	old := time.Now().Add(-48 * time.Hour)
	_ = s.InsertTelemetry("VIN1", old, map[string]any{"vehicle": map[string]any{"state": "parked"}})
	_ = s.InsertTelemetry("VIN1", time.Now(), map[string]any{"vehicle": map[string]any{"state": "parked"}})

	n, err := s.PruneTelemetryBefore(time.Now().Add(-24 * time.Hour))
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if n != 1 {
		t.Errorf("pruned %d, want 1", n)
	}
}

func TestCommandQueueReplay(t *testing.T) {
	s := openTemp(t)

	if err := s.Enqueue("req-1", "VIN1", "lock", map[string]any{"x": "1"}, time.Hour); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if err := s.Enqueue("req-2", "VIN1", "blinker_both", nil, time.Hour); err != nil {
		t.Fatalf("enqueue 2: %v", err)
	}

	queued, err := s.DequeueQueued("VIN1")
	if err != nil {
		t.Fatalf("dequeue: %v", err)
	}
	if len(queued) != 2 {
		t.Fatalf("dequeued %d, want 2", len(queued))
	}
	if queued[0].RequestID != "req-1" || queued[0].Command != "lock" {
		t.Errorf("order/command wrong: %+v", queued[0])
	}
	if queued[0].Params["x"] != "1" {
		t.Errorf("params not preserved: %+v", queued[0].Params)
	}

	// Second dequeue returns nothing (already marked sent).
	again, _ := s.DequeueQueued("VIN1")
	if len(again) != 0 {
		t.Errorf("re-dequeue returned %d, want 0", len(again))
	}

	// Ack the first command.
	if err := s.UpdateResult("req-1", StatusSuccess, map[string]any{"ok": true}, ""); err != nil {
		t.Fatalf("update result: %v", err)
	}
	rec, ok, err := s.GetCommand("req-1")
	if err != nil || !ok {
		t.Fatalf("get command: %v ok=%v", err, ok)
	}
	if rec.Status != StatusSuccess || rec.AckedAt == nil {
		t.Errorf("ack not recorded: %+v", rec)
	}
	if rec.Command != "lock" {
		t.Errorf("command name lost: %q", rec.Command)
	}
}

func TestExpireStale(t *testing.T) {
	s := openTemp(t)
	// A very short TTL expires almost immediately.
	if err := s.Enqueue("req-x", "VIN1", "lock", nil, time.Millisecond); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	n, err := s.ExpireStale()
	if err != nil {
		t.Fatalf("expire: %v", err)
	}
	if n != 1 {
		t.Errorf("expired %d, want 1", n)
	}
	queued, _ := s.DequeueQueued("VIN1")
	if len(queued) != 0 {
		t.Errorf("expired command still deliverable")
	}
}
