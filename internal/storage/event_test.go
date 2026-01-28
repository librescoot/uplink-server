package storage

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestEventStore_AddAndGet(t *testing.T) {
	es := NewEventStore(100, "")

	ts := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	es.AddEvent("s1", "battery_low", map[string]any{"level": "5"}, ts)

	events := es.GetEvents("s1", 0)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Event != "battery_low" {
		t.Fatalf("expected battery_low, got %s", events[0].Event)
	}
	if events[0].ScooterID != "s1" {
		t.Fatalf("expected s1, got %s", events[0].ScooterID)
	}
}

func TestEventStore_GetEventsNonexistent(t *testing.T) {
	es := NewEventStore(100, "")

	events := es.GetEvents("nonexistent", 0)
	if len(events) != 0 {
		t.Fatalf("expected 0 events, got %d", len(events))
	}
}

func TestEventStore_Ordering(t *testing.T) {
	es := NewEventStore(100, "")

	ts1 := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	ts2 := time.Date(2025, 1, 1, 12, 1, 0, 0, time.UTC)
	ts3 := time.Date(2025, 1, 1, 12, 2, 0, 0, time.UTC)

	es.AddEvent("s1", "event1", nil, ts1)
	es.AddEvent("s1", "event2", nil, ts2)
	es.AddEvent("s1", "event3", nil, ts3)

	events := es.GetEvents("s1", 0)
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	// Most recent first
	if events[0].Event != "event3" {
		t.Fatalf("expected event3 first, got %s", events[0].Event)
	}
	if events[2].Event != "event1" {
		t.Fatalf("expected event1 last, got %s", events[2].Event)
	}
}

func TestEventStore_Limit(t *testing.T) {
	es := NewEventStore(100, "")

	for i := 0; i < 10; i++ {
		es.AddEvent("s1", "event", nil, time.Now())
	}

	events := es.GetEvents("s1", 3)
	if len(events) != 3 {
		t.Fatalf("expected 3 events with limit, got %d", len(events))
	}
}

func TestEventStore_MaxPerScooter(t *testing.T) {
	es := NewEventStore(5, "")

	for i := 0; i < 10; i++ {
		es.AddEvent("s1", "event", nil, time.Now().Add(time.Duration(i)*time.Second))
	}

	events := es.GetEvents("s1", 0)
	if len(events) != 5 {
		t.Fatalf("expected 5 events (maxPerScooter), got %d", len(events))
	}
}

func TestEventStore_DeleteEvent(t *testing.T) {
	es := NewEventStore(100, "")

	ts := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	es.AddEvent("s1", "event1", nil, ts)

	events := es.GetEvents("s1", 0)
	eventID := events[0].ID

	ok := es.DeleteEvent("s1", eventID)
	if !ok {
		t.Fatal("expected delete to succeed")
	}

	events = es.GetEvents("s1", 0)
	if len(events) != 0 {
		t.Fatalf("expected 0 events after delete, got %d", len(events))
	}
}

func TestEventStore_DeleteNonexistent(t *testing.T) {
	es := NewEventStore(100, "")

	if es.DeleteEvent("s1", "fake-id") {
		t.Fatal("expected delete to return false for nonexistent")
	}
}

func TestEventStore_ClearEvents(t *testing.T) {
	es := NewEventStore(100, "")

	es.AddEvent("s1", "event1", nil, time.Now())
	es.AddEvent("s1", "event2", nil, time.Now())

	es.ClearEvents("s1")

	events := es.GetEvents("s1", 0)
	if len(events) != 0 {
		t.Fatalf("expected 0 events after clear, got %d", len(events))
	}
}

func TestEventStore_GetAllEvents(t *testing.T) {
	es := NewEventStore(100, "")

	es.AddEvent("s1", "event1", nil, time.Now())
	es.AddEvent("s2", "event2", nil, time.Now())

	all := es.GetAllEvents()
	if len(all) != 2 {
		t.Fatalf("expected 2 scooters, got %d", len(all))
	}
}

func TestEventStore_Subscribe(t *testing.T) {
	es := NewEventStore(100, "")

	ch, id := es.Subscribe()

	ts := time.Now()
	es.AddEvent("s1", "test_event", map[string]any{"key": "val"}, ts)

	select {
	case event := <-ch:
		if event.Event != "test_event" {
			t.Fatalf("expected test_event, got %s", event.Event)
		}
		if event.ScooterID != "s1" {
			t.Fatalf("expected s1, got %s", event.ScooterID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}

	es.Unsubscribe(id)
}

func TestEventStore_Unsubscribe(t *testing.T) {
	es := NewEventStore(100, "")

	_, id := es.Subscribe()
	es.Unsubscribe(id)

	// Double unsubscribe should not panic
	es.Unsubscribe(id)
}

func TestEventStore_FilePersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	es := NewEventStore(100, path)
	ts := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	es.AddEvent("s1", "boot", map[string]any{"version": "1.0"}, ts)
	es.AddEvent("s1", "shutdown", nil, ts.Add(time.Hour))

	// Verify file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("events file should exist")
	}

	// Load into new store
	es2 := NewEventStore(100, path)
	events := es2.GetEvents("s1", 0)
	if len(events) != 2 {
		t.Fatalf("expected 2 events loaded from file, got %d", len(events))
	}
	// Newest first
	if events[0].Event != "shutdown" {
		t.Fatalf("expected shutdown first (newest), got %s", events[0].Event)
	}
}

func TestEventStore_Compaction(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	// maxPerScooter=5, so compaction triggers after 5 appends
	es := NewEventStore(5, path)

	for i := 0; i < 10; i++ {
		es.AddEvent("s1", "event", nil, time.Now().Add(time.Duration(i)*time.Second))
	}

	// After compaction, file should contain only the trimmed set
	es2 := NewEventStore(5, path)
	events := es2.GetEvents("s1", 0)
	if len(events) != 5 {
		t.Fatalf("expected 5 events after compaction reload, got %d", len(events))
	}
}

func TestEventStore_Concurrent(t *testing.T) {
	es := NewEventStore(100, "")
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			es.AddEvent("s1", "event", nil, time.Now())
			es.GetEvents("s1", 0)
			es.GetAllEvents()
		}(i)
	}

	wg.Wait()
}
