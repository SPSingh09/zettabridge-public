package tradepush

import (
	"testing"
)

func TestUnregisterRemovesClient(t *testing.T) {
	hub := NewHub(nil)
	c := &client{
		userID: "user-1",
		send:   make(chan []byte, sendBuffer),
		done:   make(chan struct{}),
	}
	hub.mu.Lock()
	hub.clients["user-1"] = map[*client]struct{}{c: {}}
	hub.mu.Unlock()

	hub.unregister(c)
	if hub.ClientCount("user-1") != 0 {
		t.Fatalf("client count want 0 got %d", hub.ClientCount("user-1"))
	}

	select {
	case <-c.done:
	default:
		t.Fatal("expected done channel to be closed")
	}

	hub.unregister(c) // must not panic when called twice
}

func TestBroadcastSkipsDisconnectedClient(t *testing.T) {
	hub := NewHub(nil)
	c := &client{
		userID: "user-1",
		send:   make(chan []byte, sendBuffer),
		done:   make(chan struct{}),
	}
	hub.mu.Lock()
	hub.clients["user-1"] = map[*client]struct{}{c: {}}
	hub.mu.Unlock()

	hub.unregister(c)
	hub.broadcast("user-1", []byte(`{"type":"trade"}`))
}
