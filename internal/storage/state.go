package storage

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ScooterState stores the latest state data for a scooter
type ScooterState struct {
	ScooterID    string
	State        map[string]any // Full state snapshot
	Version      string         // Client version
	LastUpdated  time.Time
	LastChangeAt time.Time
}

// StateUpdate represents a state change notification
type StateUpdate struct {
	ScooterID string
	State     map[string]any
	Type      string // "full" or "delta"
	Timestamp time.Time
}

// StateStore manages scooter state data
type StateStore struct {
	mu          sync.RWMutex
	states      map[string]*ScooterState
	subscribers []chan<- StateUpdate
	filePath    string
}

// NewStateStore creates a new state store
func NewStateStore(filePath string) *StateStore {
	ss := &StateStore{
		states:      make(map[string]*ScooterState),
		subscribers: make([]chan<- StateUpdate, 0),
		filePath:    filePath,
	}

	// Load states from file if it exists
	if filePath != "" {
		ss.loadFromFile()
	}

	return ss
}

// Subscribe creates a new subscription channel for state updates
func (ss *StateStore) Subscribe() <-chan StateUpdate {
	ch := make(chan StateUpdate, 100)
	ss.mu.Lock()
	ss.subscribers = append(ss.subscribers, ch)
	ss.mu.Unlock()
	return ch
}

// broadcast sends a state update to all subscribers
func (ss *StateStore) broadcast(update StateUpdate) {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	for _, ch := range ss.subscribers {
		select {
		case ch <- update:
		default:
			// Skip slow subscribers to avoid blocking
		}
	}
}

// UpdateState updates or creates a scooter's full state
func (ss *StateStore) UpdateState(scooterID string, stateData map[string]any) {
	ss.mu.Lock()

	state, exists := ss.states[scooterID]
	if !exists {
		state = &ScooterState{
			ScooterID: scooterID,
			State:     make(map[string]any),
		}
		ss.states[scooterID] = state
	}

	// Replace entire state
	state.State = stateData
	state.LastUpdated = time.Now()
	state.LastChangeAt = time.Now()

	ss.mu.Unlock()

	// Persist to disk
	ss.saveToFile()

	// Broadcast to subscribers (outside lock to avoid deadlock)
	ss.broadcast(StateUpdate{
		ScooterID: scooterID,
		State:     stateData,
		Type:      "full",
		Timestamp: time.Now(),
	})
}

// UpdateChanges applies incremental changes to a scooter's state
func (ss *StateStore) UpdateChanges(scooterID string, changes map[string]any) {
	ss.mu.Lock()

	state, exists := ss.states[scooterID]
	if !exists {
		state = &ScooterState{
			ScooterID: scooterID,
			State:     make(map[string]any),
		}
		ss.states[scooterID] = state
	}

	// Apply changes to existing state
	for key, value := range changes {
		if valueMap, ok := value.(map[string]any); ok {
			// Nested object - merge with existing
			if existing, ok := state.State[key].(map[string]any); ok {
				for subKey, subValue := range valueMap {
					existing[subKey] = subValue
				}
			} else {
				state.State[key] = valueMap
			}
		} else {
			state.State[key] = value
		}
	}

	state.LastUpdated = time.Now()
	state.LastChangeAt = time.Now()

	ss.mu.Unlock()

	// Persist to disk
	ss.saveToFile()

	// Broadcast to subscribers (outside lock to avoid deadlock)
	ss.broadcast(StateUpdate{
		ScooterID: scooterID,
		State:     changes,
		Type:      "delta",
		Timestamp: time.Now(),
	})
}

// GetState retrieves the latest state for a scooter
func (ss *StateStore) GetState(scooterID string) (*ScooterState, bool) {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	state, exists := ss.states[scooterID]
	if !exists {
		return nil, false
	}

	// Return a copy to avoid external modifications
	stateCopy := &ScooterState{
		ScooterID:    state.ScooterID,
		State:        make(map[string]any),
		Version:      state.Version,
		LastUpdated:  state.LastUpdated,
		LastChangeAt: state.LastChangeAt,
	}

	for k, v := range state.State {
		stateCopy.State[k] = v
	}

	return stateCopy, true
}

// GetAllStates retrieves all scooter states
func (ss *StateStore) GetAllStates() map[string]*ScooterState {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	// Return a copy of the states map
	statesCopy := make(map[string]*ScooterState, len(ss.states))
	for id, state := range ss.states {
		statesCopy[id] = state
	}

	return statesCopy
}

// SetVersion updates the version for a scooter
func (ss *StateStore) SetVersion(scooterID, version string) {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	state, exists := ss.states[scooterID]
	if !exists {
		// Create new state entry if it doesn't exist
		state = &ScooterState{
			ScooterID: scooterID,
			State:     make(map[string]any),
		}
		ss.states[scooterID] = state
	}

	state.Version = version
	state.LastUpdated = time.Now()

	// Persist to disk (outside lock to avoid holding it too long)
	go ss.saveToFile()
}

// RemoveState removes a scooter's state (e.g., when disconnected)
func (ss *StateStore) RemoveState(scooterID string) {
	ss.mu.Lock()
	delete(ss.states, scooterID)
	ss.mu.Unlock()

	// Persist after removal
	ss.saveToFile()
}

// loadFromFile loads state snapshot from disk
func (ss *StateStore) loadFromFile() {
	if _, err := os.Stat(ss.filePath); os.IsNotExist(err) {
		return
	}

	data, err := os.ReadFile(ss.filePath)
	if err != nil {
		log.Printf("[StateStore] Failed to read state file: %v", err)
		return
	}

	var states map[string]*ScooterState
	if err := json.Unmarshal(data, &states); err != nil {
		log.Printf("[StateStore] Failed to parse state file: %v", err)
		return
	}

	ss.states = states
	log.Printf("[StateStore] Loaded state for %d scooters from %s", len(states), ss.filePath)
}

// saveToFile writes a snapshot of all states to disk
func (ss *StateStore) saveToFile() {
	if ss.filePath == "" {
		return
	}

	ss.mu.RLock()
	data, err := json.MarshalIndent(ss.states, "", "  ")
	ss.mu.RUnlock()

	if err != nil {
		log.Printf("[StateStore] Failed to marshal states: %v", err)
		return
	}

	// Ensure directory exists
	dir := filepath.Dir(ss.filePath)
	os.MkdirAll(dir, 0755)

	// Write atomically via temp file
	tmpPath := ss.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		log.Printf("[StateStore] Failed to write state file: %v", err)
		return
	}

	if err := os.Rename(tmpPath, ss.filePath); err != nil {
		log.Printf("[StateStore] Failed to rename state file: %v", err)
	}
}
