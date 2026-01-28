package auth

import (
	"sync"
	"testing"

	"github.com/librescoot/uplink-server/internal/models"
)

func newTestAuthenticator() *Authenticator {
	config := &models.Config{
		Auth: models.AuthConfig{
			Tokens: map[string]models.ScooterConfig{
				"scooter-1": {Token: "token-1", Name: "Test Scooter"},
				"scooter-2": {Token: "token-2"},
			},
		},
	}
	return NewAuthenticator(config)
}

func TestAuthenticate_Success(t *testing.T) {
	a := newTestAuthenticator()
	if err := a.Authenticate("scooter-1", "token-1"); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

func TestAuthenticate_UnknownIdentifier(t *testing.T) {
	a := newTestAuthenticator()
	if err := a.Authenticate("unknown", "token-1"); err == nil {
		t.Fatal("expected error for unknown identifier")
	}
}

func TestAuthenticate_WrongToken(t *testing.T) {
	a := newTestAuthenticator()
	if err := a.Authenticate("scooter-1", "wrong-token"); err == nil {
		t.Fatal("expected error for wrong token")
	}
}

func TestGetName(t *testing.T) {
	a := newTestAuthenticator()

	if name := a.GetName("scooter-1"); name != "Test Scooter" {
		t.Fatalf("expected 'Test Scooter', got %q", name)
	}

	if name := a.GetName("scooter-2"); name != "" {
		t.Fatalf("expected empty name, got %q", name)
	}

	if name := a.GetName("nonexistent"); name != "" {
		t.Fatalf("expected empty name for nonexistent, got %q", name)
	}
}

func TestAddRemoveToken(t *testing.T) {
	a := newTestAuthenticator()

	a.AddToken("scooter-3", "token-3")
	if err := a.Authenticate("scooter-3", "token-3"); err != nil {
		t.Fatalf("expected success after AddToken, got %v", err)
	}

	a.RemoveToken("scooter-3")
	if err := a.Authenticate("scooter-3", "token-3"); err == nil {
		t.Fatal("expected error after RemoveToken")
	}
}

func TestAuthenticate_Concurrent(t *testing.T) {
	a := newTestAuthenticator()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a.Authenticate("scooter-1", "token-1")
			a.GetName("scooter-1")
		}()
	}

	wg.Wait()
}
