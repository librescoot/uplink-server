// Package registry manages scooter registration (token issuance + persistence)
// so scooters can be added/removed at runtime instead of via the CLI.
package registry

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"sync"

	"gopkg.in/yaml.v2"

	"github.com/librescoot/uplink-server/internal/auth"
	"github.com/librescoot/uplink-server/internal/models"
)

// Registry issues scooter credentials and persists them to the config file.
type Registry struct {
	mu   sync.Mutex
	cfg  *models.Config
	path string
	auth *auth.Authenticator
}

// New creates a registry bound to the running config, its file path, and the
// live authenticator.
func New(cfg *models.Config, path string, a *auth.Authenticator) *Registry {
	return &Registry{cfg: cfg, path: path, auth: a}
}

// List returns the registered scooters (without tokens).
func (r *Registry) List() []auth.ScooterInfo {
	return r.auth.List()
}

// Add registers a new scooter, generating a token, updating the live
// authenticator, and persisting the config. It returns the generated token.
func (r *Registry) Add(identifier, name string) (string, error) {
	if identifier == "" {
		return "", fmt.Errorf("identifier is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.auth.Exists(identifier) {
		return "", fmt.Errorf("scooter %q already exists", identifier)
	}
	token, err := generateToken(32)
	if err != nil {
		return "", err
	}
	r.auth.Add(identifier, token, name)
	if err := r.saveLocked(); err != nil {
		// Roll back the in-memory change so state matches disk.
		r.auth.RemoveToken(identifier)
		return "", fmt.Errorf("persist config: %w", err)
	}
	return token, nil
}

// Delete removes a registered scooter and persists the config.
func (r *Registry) Delete(identifier string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.auth.Exists(identifier) {
		return fmt.Errorf("scooter %q not found", identifier)
	}
	r.auth.RemoveToken(identifier)
	return r.saveLocked()
}

// saveLocked marshals the config (with a fresh token snapshot) to disk, keeping
// a .backup of the previous file. Caller holds r.mu.
func (r *Registry) saveLocked() error {
	out := *r.cfg
	out.Auth.Tokens = r.auth.Snapshot()

	data, err := yaml.Marshal(&out)
	if err != nil {
		return err
	}
	if existing, err := os.ReadFile(r.path); err == nil {
		_ = os.WriteFile(r.path+".backup", existing, 0o600)
	}
	return os.WriteFile(r.path, data, 0o600)
}

func generateToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
