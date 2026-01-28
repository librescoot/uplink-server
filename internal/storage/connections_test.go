package storage

import (
	"sync"
	"testing"

	"github.com/librescoot/uplink-server/internal/models"
)

func TestConnectionManager_AddRemove(t *testing.T) {
	cm := NewConnectionManager(0)

	conn := models.NewConnection("scooter-1", nil)
	if err := cm.AddConnection(conn); err != nil {
		t.Fatalf("AddConnection: %v", err)
	}

	got, exists := cm.GetConnection("scooter-1")
	if !exists {
		t.Fatal("expected connection to exist")
	}
	if got.Identifier != "scooter-1" {
		t.Fatalf("expected scooter-1, got %s", got.Identifier)
	}

	cm.RemoveConnection("scooter-1")

	_, exists = cm.GetConnection("scooter-1")
	if exists {
		t.Fatal("expected connection to be removed")
	}
}

func TestConnectionManager_RemoveNonexistent(t *testing.T) {
	cm := NewConnectionManager(0)
	// Should not panic
	cm.RemoveConnection("nonexistent")
}

func TestConnectionManager_MaxConnections(t *testing.T) {
	cm := NewConnectionManager(2)

	conn1 := models.NewConnection("s1", nil)
	conn2 := models.NewConnection("s2", nil)
	conn3 := models.NewConnection("s3", nil)

	if err := cm.AddConnection(conn1); err != nil {
		t.Fatalf("AddConnection s1: %v", err)
	}
	if err := cm.AddConnection(conn2); err != nil {
		t.Fatalf("AddConnection s2: %v", err)
	}
	if err := cm.AddConnection(conn3); err == nil {
		t.Fatal("expected error when exceeding max connections")
	}

	// Remove one, should allow adding again
	cm.RemoveConnection("s1")
	if err := cm.AddConnection(conn3); err != nil {
		t.Fatalf("AddConnection s3 after removal: %v", err)
	}
}

func TestConnectionManager_UnlimitedConnections(t *testing.T) {
	cm := NewConnectionManager(0)

	for i := 0; i < 100; i++ {
		conn := models.NewConnection("s"+string(rune('A'+i)), nil)
		if err := cm.AddConnection(conn); err != nil {
			t.Fatalf("AddConnection %d: %v", i, err)
		}
	}
}

func TestConnectionManager_GetAllConnections(t *testing.T) {
	cm := NewConnectionManager(0)

	cm.AddConnection(models.NewConnection("s1", nil))
	cm.AddConnection(models.NewConnection("s2", nil))
	cm.AddConnection(models.NewConnection("s3", nil))

	conns := cm.GetAllConnections()
	if len(conns) != 3 {
		t.Fatalf("expected 3 connections, got %d", len(conns))
	}
}

func TestConnectionManager_MarkAuthenticated(t *testing.T) {
	cm := NewConnectionManager(0)

	conn := models.NewConnection("s1", nil)
	cm.AddConnection(conn)

	if err := cm.MarkAuthenticated("s1"); err != nil {
		t.Fatalf("MarkAuthenticated: %v", err)
	}

	got, _ := cm.GetConnection("s1")
	if !got.Authenticated {
		t.Fatal("expected connection to be authenticated")
	}

	if err := cm.MarkAuthenticated("nonexistent"); err == nil {
		t.Fatal("expected error for nonexistent connection")
	}
}

func TestConnectionManager_Stats(t *testing.T) {
	cm := NewConnectionManager(0)

	conn := models.NewConnection("s1", nil)
	conn.Authenticated = true
	conn.AddBytesSent(1024)
	conn.AddBytesReceived(2048)
	cm.AddConnection(conn)

	stats := cm.GetStats()

	if stats["active_connections"].(int) != 1 {
		t.Fatalf("expected 1 active connection, got %v", stats["active_connections"])
	}
	if stats["authenticated"].(int) != 1 {
		t.Fatalf("expected 1 authenticated, got %v", stats["authenticated"])
	}
	if stats["current_bytes_sent"].(int64) != 1024 {
		t.Fatalf("expected 1024 bytes sent, got %v", stats["current_bytes_sent"])
	}
}

func TestConnectionManager_StatsAfterRemoval(t *testing.T) {
	cm := NewConnectionManager(0)

	conn := models.NewConnection("s1", nil)
	conn.AddBytesSent(100)
	conn.AddBytesReceived(200)
	cm.AddConnection(conn)
	cm.RemoveConnection("s1")

	stats := cm.GetStats()
	if stats["total_bytes_sent"].(int64) != 100 {
		t.Fatalf("expected total_bytes_sent=100 after removal, got %v", stats["total_bytes_sent"])
	}
}

func TestConnectionManager_Subscribe(t *testing.T) {
	cm := NewConnectionManager(0)

	ch, id := cm.Subscribe()

	conn := models.NewConnection("s1", nil)
	cm.AddConnection(conn)

	event := <-ch
	if event.Type != "online" {
		t.Fatalf("expected online event, got %s", event.Type)
	}
	if event.Identifier != "s1" {
		t.Fatalf("expected identifier s1, got %s", event.Identifier)
	}

	cm.RemoveConnection("s1")

	event = <-ch
	if event.Type != "offline" {
		t.Fatalf("expected offline event, got %s", event.Type)
	}

	cm.Unsubscribe(id)
}

func TestConnectionManager_Unsubscribe(t *testing.T) {
	cm := NewConnectionManager(0)

	_, id := cm.Subscribe()
	cm.Unsubscribe(id)

	// Double unsubscribe should not panic
	cm.Unsubscribe(id)
}

func TestConnectionManager_Concurrent(t *testing.T) {
	cm := NewConnectionManager(0)
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			id := "s" + string(rune('A'+n))
			conn := models.NewConnection(id, nil)
			cm.AddConnection(conn)
			cm.GetConnection(id)
			cm.GetAllConnections()
			cm.GetStats()
			cm.RemoveConnection(id)
		}(i)
	}

	wg.Wait()
}
