package agent

import (
	"testing"

	"github.com/coohu/goagent/internal/core"
)

func TestCreateSession(t *testing.T) {
	m := NewSessionManager(5)

	session, err := m.Create("test goal", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if session.ID == "" {
		t.Error("session ID should not be empty")
	}
	if session.Goal != "test goal" {
		t.Errorf("wrong goal: %s", session.Goal)
	}
	if session.GetState() != core.StateIdle {
		t.Errorf("initial state should be IDLE")
	}
}

func TestGetSession(t *testing.T) {
	m := NewSessionManager(5)
	s, _ := m.Create("goal", nil)

	got, err := m.Get(s.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != s.ID {
		t.Errorf("wrong session ID")
	}
}

func TestGetSessionNotFound(t *testing.T) {
	m := NewSessionManager(5)
	_, err := m.Get("nonexistent-id")
	if err == nil {
		t.Error("expected error for nonexistent session")
	}
}

func TestCancelSession(t *testing.T) {
	m := NewSessionManager(5)
	s, _ := m.Create("goal", nil)

	if err := m.Cancel(s.ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.GetState() != core.StateCancelled {
		t.Errorf("expected CANCELLED, got %s", s.GetState())
	}
}

func TestMaxConcurrentSessions(t *testing.T) {
	m := NewSessionManager(2)
	m.Create("g1", nil)
	m.Create("g2", nil)

	_, err := m.Create("g3", nil)
	if err == nil {
		t.Error("expected error when max concurrent sessions reached")
	}
}

func TestDoneSessionDoesNotCountTowardLimit(t *testing.T) {
	m := NewSessionManager(2)
	s1, _ := m.Create("g1", nil)
	s1.SetState(core.StateDone)

	m.Create("g2", nil)
	_, err := m.Create("g3", nil)
	if err != nil {
		t.Errorf("done session should not count toward limit: %v", err)
	}
}
