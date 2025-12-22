package storage

import (
	"sync"
	"time"

	"github.com/librescoot/uplink-server/internal/protocol"
)

// CommandResponseRecord stores a command response with metadata
type CommandResponseRecord struct {
	RequestID  string
	ScooterID  string
	Command    string
	Response   *protocol.CommandResponse
	ReceivedAt time.Time
}

// ResponseStore manages command responses with TTL-based cleanup
type ResponseStore struct {
	mu        sync.RWMutex
	responses map[string]*CommandResponseRecord
	ttl       time.Duration
}

// NewResponseStore creates a new response store with the specified TTL
func NewResponseStore(ttl time.Duration) *ResponseStore {
	store := &ResponseStore{
		responses: make(map[string]*CommandResponseRecord),
		ttl:       ttl,
	}

	go store.cleanup()

	return store
}

// Store saves a command response
func (rs *ResponseStore) Store(requestID, scooterID, command string, resp *protocol.CommandResponse) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	rs.responses[requestID] = &CommandResponseRecord{
		RequestID:  requestID,
		ScooterID:  scooterID,
		Command:    command,
		Response:   resp,
		ReceivedAt: time.Now(),
	}
}

// Get retrieves a command response by request ID
func (rs *ResponseStore) Get(requestID string) (*CommandResponseRecord, bool) {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	record, exists := rs.responses[requestID]
	return record, exists
}

// GetByScooter retrieves all command responses for a specific scooter
func (rs *ResponseStore) GetByScooter(scooterID string) []*CommandResponseRecord {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	var results []*CommandResponseRecord
	for _, record := range rs.responses {
		if record.ScooterID == scooterID {
			results = append(results, record)
		}
	}

	return results
}

// cleanup runs a background goroutine to remove expired responses
func (rs *ResponseStore) cleanup() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		rs.mu.Lock()
		now := time.Now()
		for requestID, record := range rs.responses {
			if now.Sub(record.ReceivedAt) > rs.ttl {
				delete(rs.responses, requestID)
			}
		}
		rs.mu.Unlock()
	}
}
