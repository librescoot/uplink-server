package storage

import (
	"bufio"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Event represents a single event from a scooter
type Event struct {
	ID        string         `json:"id"`
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
	filePath      string
}

// NewEventStore creates a new event store
func NewEventStore(maxPerScooter int, filePath string) *EventStore {
	s := &EventStore{
		events:        make(map[string][]*Event),
		maxPerScooter: maxPerScooter,
		subscribers:   make([]chan<- *Event, 0),
		filePath:      filePath,
	}

	// Load events from file if it exists
	if filePath != "" {
		s.loadFromFile()
	}

	return s
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

// loadFromFile loads events from the persistence file
func (s *EventStore) loadFromFile() {
	if _, err := os.Stat(s.filePath); os.IsNotExist(err) {
		return
	}

	file, err := os.Open(s.filePath)
	if err != nil {
		log.Printf("[EventStore] Failed to open events file: %v", err)
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	count := 0

	for scanner.Scan() {
		var event Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			log.Printf("[EventStore] Failed to parse event, skipping: %v", err)
			continue
		}

		// Add to in-memory store (no broadcast or file write on load)
		events := s.events[event.ScooterID]
		events = append(events, &event)
		s.events[event.ScooterID] = events
		count++
	}

	if err := scanner.Err(); err != nil {
		log.Printf("[EventStore] Error reading events file: %v", err)
	}

	// Trim to max per scooter and sort newest-first
	for scooterID, events := range s.events {
		// Events from file are oldest-first (appended), reverse to newest-first
		for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
			events[i], events[j] = events[j], events[i]
		}

		// Trim to max
		if len(events) > s.maxPerScooter {
			events = events[:s.maxPerScooter]
		}
		s.events[scooterID] = events
	}

	log.Printf("[EventStore] Loaded %d events from %s", count, s.filePath)
}

// appendToFile appends an event to the persistence file
func (s *EventStore) appendToFile(event *Event) {
	if s.filePath == "" {
		return
	}

	// Ensure directory exists
	dir := filepath.Dir(s.filePath)
	os.MkdirAll(dir, 0755)

	file, err := os.OpenFile(s.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("[EventStore] Failed to open events file for writing: %v", err)
		return
	}
	defer file.Close()

	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("[EventStore] Failed to marshal event: %v", err)
		return
	}

	if _, err := file.Write(append(data, '\n')); err != nil {
		log.Printf("[EventStore] Failed to write event to file: %v", err)
	}
}

// rewriteFile rewrites the entire events file with current in-memory events
func (s *EventStore) rewriteFile() {
	if s.filePath == "" {
		return
	}

	// Ensure directory exists
	dir := filepath.Dir(s.filePath)
	os.MkdirAll(dir, 0755)

	// Create temporary file
	tmpPath := s.filePath + ".tmp"
	file, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		log.Printf("[EventStore] Failed to create temp events file: %v", err)
		return
	}

	// Write all events (oldest first for file format)
	for _, events := range s.events {
		// Events are stored newest-first in memory, reverse for file
		for i := len(events) - 1; i >= 0; i-- {
			data, err := json.Marshal(events[i])
			if err != nil {
				log.Printf("[EventStore] Failed to marshal event: %v", err)
				continue
			}

			if _, err := file.Write(append(data, '\n')); err != nil {
				log.Printf("[EventStore] Failed to write event to file: %v", err)
				file.Close()
				os.Remove(tmpPath)
				return
			}
		}
	}

	file.Close()

	// Atomically replace the old file
	if err := os.Rename(tmpPath, s.filePath); err != nil {
		log.Printf("[EventStore] Failed to replace events file: %v", err)
		os.Remove(tmpPath)
	}
}

// AddEvent stores a new event for a scooter
func (s *EventStore) AddEvent(scooterID, eventName string, data map[string]any, timestamp time.Time) {
	// Generate unique ID using timestamp and nanoseconds
	eventID := timestamp.Format("20060102150405") + "-" + timestamp.Format("000000000")

	event := &Event{
		ID:        eventID,
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

	// Persist to file
	s.appendToFile(event)

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

// DeleteEvent deletes a single event by ID
func (s *EventStore) DeleteEvent(scooterID, eventID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	events, exists := s.events[scooterID]
	if !exists {
		return false
	}

	// Find and remove the event
	for i, event := range events {
		if event.ID == eventID {
			s.events[scooterID] = append(events[:i], events[i+1:]...)
			s.rewriteFile()
			return true
		}
	}

	return false
}

// ClearEvents clears all events for a scooter
func (s *EventStore) ClearEvents(scooterID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.events, scooterID)
	s.rewriteFile()
}
