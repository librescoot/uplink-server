package auth

import (
	"fmt"
	"sync"

	"github.com/librescoot/uplink-server/internal/models"
)

// Authenticator handles scooter authentication
type Authenticator struct {
	mu     sync.RWMutex
	tokens map[string]models.ScooterConfig // identifier -> config
}

// NewAuthenticator creates a new authenticator
func NewAuthenticator(config *models.Config) *Authenticator {
	if config.Auth.Tokens == nil {
		config.Auth.Tokens = make(map[string]models.ScooterConfig)
	}
	return &Authenticator{
		tokens: config.Auth.Tokens,
	}
}

// Authenticate validates a scooter's credentials
func (a *Authenticator) Authenticate(identifier, token string) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	scooterConfig, exists := a.tokens[identifier]
	if !exists {
		return fmt.Errorf("unknown identifier: %s", identifier)
	}

	if token != scooterConfig.Token {
		return fmt.Errorf("invalid token for identifier: %s", identifier)
	}

	return nil
}

// GetName returns the human-friendly name for a scooter, or empty string if not set
func (a *Authenticator) GetName(identifier string) string {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if scooterConfig, exists := a.tokens[identifier]; exists {
		return scooterConfig.Name
	}
	return ""
}

// AddToken adds a new token (for dynamic registration)
func (a *Authenticator) AddToken(identifier, token string) {
	a.Add(identifier, token, "")
}

// Add registers or updates a scooter's token and name.
func (a *Authenticator) Add(identifier, token, name string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.tokens[identifier] = models.ScooterConfig{Token: token, Name: name}
}

// RemoveToken removes a token
func (a *Authenticator) RemoveToken(identifier string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.tokens, identifier)
}

// Exists reports whether an identifier is registered.
func (a *Authenticator) Exists(identifier string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	_, ok := a.tokens[identifier]
	return ok
}

// ScooterInfo is a registered scooter without its secret token.
type ScooterInfo struct {
	Identifier string `json:"identifier"`
	Name       string `json:"name,omitempty"`
}

// List returns all registered scooters (without tokens).
func (a *Authenticator) List() []ScooterInfo {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]ScooterInfo, 0, len(a.tokens))
	for id, cfg := range a.tokens {
		out = append(out, ScooterInfo{Identifier: id, Name: cfg.Name})
	}
	return out
}

// Snapshot returns a copy of the token map for persistence.
func (a *Authenticator) Snapshot() map[string]models.ScooterConfig {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make(map[string]models.ScooterConfig, len(a.tokens))
	for id, cfg := range a.tokens {
		out[id] = cfg
	}
	return out
}
