// Package session provides short-lived bearer tokens issued after a successful
// username/password login. Tokens are accepted anywhere the API key is.
package session

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// Store holds active sessions in memory with a fixed TTL.
type Store struct {
	mu       sync.RWMutex
	sessions map[string]entry
	ttl      time.Duration
}

type entry struct {
	username string
	expires  time.Time
}

// New creates a session store with the given TTL and starts a background
// cleanup goroutine.
func New(ttl time.Duration) *Store {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	s := &Store{sessions: make(map[string]entry), ttl: ttl}
	go s.cleanup()
	return s
}

// Create issues a new token for username and returns it along with its TTL.
func (s *Store) Create(username string) (string, time.Duration, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", 0, err
	}
	token := hex.EncodeToString(b)

	s.mu.Lock()
	s.sessions[token] = entry{username: username, expires: time.Now().Add(s.ttl)}
	s.mu.Unlock()

	return token, s.ttl, nil
}

// Validate returns the username and true if the token is valid and unexpired.
func (s *Store) Validate(token string) (string, bool) {
	if token == "" {
		return "", false
	}
	s.mu.RLock()
	e, ok := s.sessions[token]
	s.mu.RUnlock()
	if !ok || time.Now().After(e.expires) {
		return "", false
	}
	return e.username, true
}

// Delete invalidates a token (logout).
func (s *Store) Delete(token string) {
	s.mu.Lock()
	delete(s.sessions, token)
	s.mu.Unlock()
}

func (s *Store) cleanup() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		s.mu.Lock()
		for token, e := range s.sessions {
			if now.After(e.expires) {
				delete(s.sessions, token)
			}
		}
		s.mu.Unlock()
	}
}
