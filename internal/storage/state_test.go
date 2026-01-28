package storage

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestStateStore_UpdateAndGet(t *testing.T) {
	ss := NewStateStore("")

	data := map[string]any{
		"battery:0": map[string]any{"charge": "64"},
		"vehicle":   map[string]any{"state": "stand-by"},
	}

	ss.UpdateState("s1", data)

	state, exists := ss.GetState("s1")
	if !exists {
		t.Fatal("expected state to exist")
	}
	if state.ScooterID != "s1" {
		t.Fatalf("expected scooter ID s1, got %s", state.ScooterID)
	}

	battery, ok := state.State["battery:0"].(map[string]any)
	if !ok {
		t.Fatal("expected battery:0 to be a map")
	}
	if battery["charge"] != "64" {
		t.Fatalf("expected charge=64, got %v", battery["charge"])
	}
}

func TestStateStore_GetNonexistent(t *testing.T) {
	ss := NewStateStore("")

	_, exists := ss.GetState("nonexistent")
	if exists {
		t.Fatal("expected state to not exist")
	}
}

func TestStateStore_UpdateChanges(t *testing.T) {
	ss := NewStateStore("")

	// Set initial state
	ss.UpdateState("s1", map[string]any{
		"battery:0": map[string]any{"charge": "64", "voltage": "54000"},
		"vehicle":   map[string]any{"state": "stand-by"},
	})

	// Apply changes
	ss.UpdateChanges("s1", map[string]any{
		"battery:0": map[string]any{"charge": "65"},
	})

	state, _ := ss.GetState("s1")

	battery := state.State["battery:0"].(map[string]any)
	if battery["charge"] != "65" {
		t.Fatalf("expected charge=65 after change, got %v", battery["charge"])
	}
	if battery["voltage"] != "54000" {
		t.Fatalf("expected voltage=54000 preserved, got %v", battery["voltage"])
	}
}

func TestStateStore_UpdateChangesCreatesNewState(t *testing.T) {
	ss := NewStateStore("")

	ss.UpdateChanges("s1", map[string]any{
		"vehicle": map[string]any{"state": "riding"},
	})

	state, exists := ss.GetState("s1")
	if !exists {
		t.Fatal("expected state to be created")
	}
	vehicle := state.State["vehicle"].(map[string]any)
	if vehicle["state"] != "riding" {
		t.Fatalf("expected state=riding, got %v", vehicle["state"])
	}
}

func TestStateStore_SetVersion(t *testing.T) {
	ss := NewStateStore("")

	ss.SetVersion("s1", "1.2.3")

	state, exists := ss.GetState("s1")
	if !exists {
		t.Fatal("expected state to be created by SetVersion")
	}
	if state.Version != "1.2.3" {
		t.Fatalf("expected version=1.2.3, got %s", state.Version)
	}
}

func TestStateStore_RemoveState(t *testing.T) {
	ss := NewStateStore("")

	ss.UpdateState("s1", map[string]any{"key": "value"})
	ss.RemoveState("s1")

	_, exists := ss.GetState("s1")
	if exists {
		t.Fatal("expected state to be removed")
	}
}

func TestStateStore_GetAllStates(t *testing.T) {
	ss := NewStateStore("")

	ss.UpdateState("s1", map[string]any{"a": "1"})
	ss.UpdateState("s2", map[string]any{"b": "2"})

	all := ss.GetAllStates()
	if len(all) != 2 {
		t.Fatalf("expected 2 states, got %d", len(all))
	}
}

func TestStateStore_Subscribe(t *testing.T) {
	ss := NewStateStore("")

	ch, id := ss.Subscribe()

	ss.UpdateState("s1", map[string]any{"test": "data"})

	select {
	case update := <-ch:
		if update.ScooterID != "s1" {
			t.Fatalf("expected scooter ID s1, got %s", update.ScooterID)
		}
		if update.Type != "full" {
			t.Fatalf("expected type=full, got %s", update.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for update")
	}

	ss.Unsubscribe(id)
}

func TestStateStore_SubscribeChanges(t *testing.T) {
	ss := NewStateStore("")

	ch, id := ss.Subscribe()
	defer ss.Unsubscribe(id)

	ss.UpdateChanges("s1", map[string]any{"test": "delta"})

	select {
	case update := <-ch:
		if update.Type != "delta" {
			t.Fatalf("expected type=delta, got %s", update.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for update")
	}
}

func TestStateStore_Unsubscribe(t *testing.T) {
	ss := NewStateStore("")

	_, id := ss.Subscribe()
	ss.Unsubscribe(id)

	// Double unsubscribe should not panic
	ss.Unsubscribe(id)
}

func TestStateStore_FilePersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	ss := NewStateStore(path)
	ss.UpdateState("s1", map[string]any{"key": "value"})

	// Verify file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("state file should exist")
	}

	// Load into new store
	ss2 := NewStateStore(path)
	state, exists := ss2.GetState("s1")
	if !exists {
		t.Fatal("expected state to be loaded from file")
	}
	if state.State["key"] != "value" {
		t.Fatalf("expected key=value, got %v", state.State["key"])
	}
}

func TestStateStore_Concurrent(t *testing.T) {
	ss := NewStateStore("")
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			id := "s1"
			ss.UpdateState(id, map[string]any{"n": n})
			ss.UpdateChanges(id, map[string]any{"n": n + 1})
			ss.GetState(id)
			ss.GetAllStates()
			ss.SetVersion(id, "1.0")
		}(i)
	}

	wg.Wait()
}
