package sse

import (
	"testing"
)

func TestBroadcastAndHistory(t *testing.T) {
	hub := NewHub()

	hub.Broadcast("sess1", map[string]any{"type": "thought", "content": "thinking"})
	hub.Broadcast("sess1", map[string]any{"type": "tool_call", "tool": "file.read"})
	hub.Broadcast("sess1", map[string]any{"type": "done"})

	hist := hub.History("sess1", 0)
	if len(hist) != 3 {
		t.Errorf("expected 3 events in history, got %d", len(hist))
	}
}

func TestHistoryFromCursor(t *testing.T) {
	hub := NewHub()
	for i := 0; i < 5; i++ {
		hub.Broadcast("sess1", map[string]any{"i": i})
	}

	hist := hub.History("sess1", 3)
	if len(hist) != 2 {
		t.Errorf("expected 2 events from cursor 3, got %d", len(hist))
	}
}

func TestHistoryMaxCap(t *testing.T) {
	hub := NewHub()
	hub.maxHist = 3

	for i := 0; i < 10; i++ {
		hub.Broadcast("sess1", map[string]any{"i": i})
	}

	hist := hub.History("sess1", 0)
	if len(hist) > 3 {
		t.Errorf("history should be capped at 3, got %d", len(hist))
	}
}

func TestClearHistory(t *testing.T) {
	hub := NewHub()
	hub.Broadcast("sess1", map[string]any{"type": "test"})
	hub.ClearHistory("sess1")

	hist := hub.History("sess1", 0)
	if len(hist) != 0 {
		t.Errorf("expected empty history after clear, got %d", len(hist))
	}
}

func TestHistoryEmptySession(t *testing.T) {
	hub := NewHub()
	hist := hub.History("nonexistent", 0)
	if hist != nil {
		t.Errorf("expected nil for nonexistent session, got %v", hist)
	}
}

func TestRegisterAndBroadcast(t *testing.T) {
	hub := NewHub()

	c := hub.Register("sess1", 0)
	hub.Broadcast("sess1", map[string]any{"type": "ping"})

	select {
	case data := <-c.ch:
		if len(data) == 0 {
			t.Error("received empty data")
		}
	default:
		t.Error("expected data in channel")
	}

	hub.Unregister("sess1", c)
}

func TestIsolationBetweenSessions(t *testing.T) {
	hub := NewHub()

	c1 := hub.Register("sess1", 0)
	c2 := hub.Register("sess2", 0)
	defer hub.Unregister("sess1", c1)
	defer hub.Unregister("sess2", c2)

	hub.Broadcast("sess1", map[string]any{"msg": "for sess1 only"})

	select {
	case <-c1.ch:
	default:
		t.Error("sess1 client should have received event")
	}
	select {
	case <-c2.ch:
		t.Error("sess2 client should NOT have received sess1 event")
	default:
	}
}
