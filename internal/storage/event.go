package storage

import (
	"sync"
	"time"
)

// Event represents a single event from a scooter
type Event struct {
	ScooterID string         `json:"scooter_id"`
	Event     string         `json:"event"`
	Data      map[string]any `json:"data"`
	Timestamp time.Time      `json:"timestamp"`
}

// EventStore stores and manages scooter events
type EventStore struct {
	mu            sync.RWMutex
	events        map[string][]*Event // scooter_id -> events list
	maxPerScooter int
	subscribers   []chan<- *Event
}

// NewEventStore creates a new event store
func NewEventStore(maxPerScooter int) *EventStore {
	return &EventStore{
		events:        make(map[string][]*Event),
		maxPerScooter: maxPerScooter,
		subscribers:   make([]chan<- *Event, 0),
	}
}

// Subscribe adds a subscriber channel for event updates
func (s *EventStore) Subscribe() <-chan *Event {
	ch := make(chan *Event, 100)
	s.mu.Lock()
	s.subscribers = append(s.subscribers, ch)
	s.mu.Unlock()
	return ch
}

// broadcast sends an event to all subscribers
func (s *EventStore) broadcast(event *Event) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, ch := range s.subscribers {
		select {
		case ch <- event:
		default:
			// Skip slow subscribers
		}
	}
}

// AddEvent stores a new event for a scooter
func (s *EventStore) AddEvent(scooterID, eventName string, data map[string]any, timestamp time.Time) {
	event := &Event{
		ScooterID: scooterID,
		Event:     eventName,
		Data:      data,
		Timestamp: timestamp,
	}

	s.mu.Lock()
	events, exists := s.events[scooterID]
	if !exists {
		s.events[scooterID] = []*Event{event}
	} else {
		// Prepend new event (most recent first)
		events = append([]*Event{event}, events...)

		// Limit to maxPerScooter events
		if len(events) > s.maxPerScooter {
			events = events[:s.maxPerScooter]
		}

		s.events[scooterID] = events
	}
	s.mu.Unlock()

	// Broadcast to subscribers
	s.broadcast(event)
}

// GetEvents retrieves events for a scooter (most recent first)
func (s *EventStore) GetEvents(scooterID string, limit int) []*Event {
	s.mu.RLock()
	defer s.mu.RUnlock()

	events, exists := s.events[scooterID]
	if !exists {
		return []*Event{}
	}

	if limit > 0 && limit < len(events) {
		return events[:limit]
	}

	return events
}

// GetAllEvents retrieves all events for all scooters
func (s *EventStore) GetAllEvents() map[string][]*Event {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string][]*Event, len(s.events))
	for scooterID, events := range s.events {
		result[scooterID] = events
	}

	return result
}

// ClearEvents clears all events for a scooter
func (s *EventStore) ClearEvents(scooterID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.events, scooterID)
}
