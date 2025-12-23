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
	a.mu.Lock()
	defer a.mu.Unlock()
	a.tokens[identifier] = models.ScooterConfig{Token: token}
}

// RemoveToken removes a token
func (a *Authenticator) RemoveToken(identifier string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.tokens, identifier)
}
