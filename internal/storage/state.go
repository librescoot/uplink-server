package storage

import (
	"sync"
	"time"
)

// ScooterState stores the latest state data for a scooter
type ScooterState struct {
	ScooterID    string
	State        map[string]any // Full state snapshot
	LastUpdated  time.Time
	LastChangeAt time.Time
}

// StateStore manages scooter state data
type StateStore struct {
	mu     sync.RWMutex
	states map[string]*ScooterState
}

// NewStateStore creates a new state store
func NewStateStore() *StateStore {
	return &StateStore{
		states: make(map[string]*ScooterState),
	}
}

// UpdateState updates or creates a scooter's full state
func (ss *StateStore) UpdateState(scooterID string, stateData map[string]any) {
	ss.mu.Lock()
	defer ss.mu.Unlock()

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
}

// UpdateChanges applies incremental changes to a scooter's state
func (ss *StateStore) UpdateChanges(scooterID string, changes map[string]any) {
	ss.mu.Lock()
	defer ss.mu.Unlock()

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
		LastUpdated:  state.LastUpdated,
		LastChangeAt: state.LastChangeAt,
	}

	for k, v := range state.State {
		stateCopy.State[k] = v
	}

	return stateCopy, true
}

// RemoveState removes a scooter's state (e.g., when disconnected)
func (ss *StateStore) RemoveState(scooterID string) {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	delete(ss.states, scooterID)
}
