package models

import (
	"sync"
	"testing"
	"time"
)

func TestNewConnection(t *testing.T) {
	conn := NewConnection("test-scooter", nil)

	if conn.Identifier != "test-scooter" {
		t.Fatalf("expected identifier 'test-scooter', got %q", conn.Identifier)
	}
	if conn.Authenticated {
		t.Fatal("new connection should not be authenticated")
	}
	if conn.ConnectedAt.IsZero() {
		t.Fatal("ConnectedAt should be set")
	}
	if conn.LastSeen.IsZero() {
		t.Fatal("LastSeen should be set")
	}
}

func TestUpdateLastSeen(t *testing.T) {
	conn := NewConnection("test", nil)
	initial := conn.GetLastSeen()

	time.Sleep(time.Millisecond)
	conn.UpdateLastSeen()

	if !conn.GetLastSeen().After(initial) {
		t.Fatal("LastSeen should have been updated")
	}
}

func TestStatCounters(t *testing.T) {
	conn := NewConnection("test", nil)

	conn.AddBytesSent(100)
	conn.AddBytesSent(50)
	conn.AddBytesReceived(200)
	conn.IncrementMessagesSent()
	conn.IncrementMessagesSent()
	conn.IncrementMessagesReceived()
	conn.IncrementTelemetryReceived()
	conn.IncrementCommandsSent()

	stats := conn.GetStats()

	if stats["bytes_sent"].(int64) != 150 {
		t.Fatalf("expected bytes_sent=150, got %v", stats["bytes_sent"])
	}
	if stats["bytes_received"].(int64) != 200 {
		t.Fatalf("expected bytes_received=200, got %v", stats["bytes_received"])
	}
	if stats["messages_sent"].(int64) != 2 {
		t.Fatalf("expected messages_sent=2, got %v", stats["messages_sent"])
	}
	if stats["messages_received"].(int64) != 1 {
		t.Fatalf("expected messages_received=1, got %v", stats["messages_received"])
	}
	if stats["telemetry_received"].(int64) != 1 {
		t.Fatalf("expected telemetry_received=1, got %v", stats["telemetry_received"])
	}
	if stats["commands_sent"].(int64) != 1 {
		t.Fatalf("expected commands_sent=1, got %v", stats["commands_sent"])
	}
}

func TestSendChannel(t *testing.T) {
	conn := NewConnection("test", nil)

	msg := []byte("hello")
	conn.SendChannel() <- msg

	received := <-conn.ReceiveChannel()
	if string(received) != "hello" {
		t.Fatalf("expected 'hello', got %q", string(received))
	}
}

func TestDoneChannel(t *testing.T) {
	conn := NewConnection("test", nil)

	select {
	case <-conn.Done():
		t.Fatal("done channel should not be closed yet")
	default:
	}

	conn.Close()

	select {
	case <-conn.Done():
	default:
		t.Fatal("done channel should be closed after Close()")
	}
}

func TestCloseDoesNotCloseSendChan(t *testing.T) {
	conn := NewConnection("test", nil)
	conn.Close()

	// sendChan should still be open (not panic on send)
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("send on sendChan panicked after Close(): %v", r)
		}
	}()

	select {
	case conn.SendChannel() <- []byte("test"):
	default:
		// channel full is fine, panic is not
	}
}

func TestStatCounters_Concurrent(t *testing.T) {
	conn := NewConnection("test", nil)
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn.AddBytesSent(1)
			conn.AddBytesReceived(1)
			conn.IncrementMessagesSent()
			conn.IncrementMessagesReceived()
			conn.IncrementTelemetryReceived()
			conn.IncrementCommandsSent()
			conn.UpdateLastSeen()
			conn.GetStats()
		}()
	}

	wg.Wait()

	stats := conn.GetStats()
	if stats["bytes_sent"].(int64) != 100 {
		t.Fatalf("expected bytes_sent=100 after concurrent ops, got %v", stats["bytes_sent"])
	}
}
