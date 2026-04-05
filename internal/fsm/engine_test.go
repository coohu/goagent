package fsm

import (
	"context"
	"testing"
	"time"

	"github.com/coohu/goagent/internal/core"
)

func newTestSession(id string) *core.AgentSession {
	return core.NewSession(id, "test goal", core.DefaultConfig())
}

func makeEv(t core.EventType) core.Event {
	return core.Event{Type: t, Payload: map[string]any{}, CreatedAt: time.Now()}
}

func TestTransition_IdleToPlanning(t *testing.T) {
	engine := NewEngine()
	session := newTestSession("s1")

	next, _, err := engine.Transition(context.Background(), session, makeEv(core.EventStart))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if next != core.StatePlanning {
		t.Errorf("expected PLANNING, got %s", next)
	}
	if session.GetState() != core.StatePlanning {
		t.Errorf("session state not updated")
	}
}

func TestTransition_UnknownEventIsNoOp(t *testing.T) {
	engine := NewEngine()
	session := newTestSession("s2")

	next, events, err := engine.Transition(context.Background(), session, makeEv("unknown_event"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if next != core.StateIdle {
		t.Errorf("expected state unchanged IDLE, got %s", next)
	}
	if len(events) != 0 {
		t.Errorf("expected no events, got %d", len(events))
	}
}

func TestTransition_CancelFromAnyState(t *testing.T) {
	engine := NewEngine()

	states := []core.AgentState{
		core.StatePlanning, core.StateExecuting, core.StateWaitTool,
		core.StateReflecting, core.StateReplanning,
	}
	for _, st := range states {
		session := newTestSession("s-" + string(st))
		session.SetState(st)

		next, _, err := engine.Transition(context.Background(), session, makeEv(core.EventCancel))
		if err != nil {
			t.Fatalf("state %s: unexpected error: %v", st, err)
		}
		if next != core.StateCancelled {
			t.Errorf("state %s: expected CANCELLED, got %s", st, next)
		}
	}
}

func TestTransition_HandlerCalled(t *testing.T) {
	engine := NewEngine()
	session := newTestSession("s3")
	called := false

	engine.RegisterHandler(core.StatePlanning, func(ctx context.Context, s *core.AgentSession, ev core.Event) ([]core.Event, error) {
		called = true
		return []core.Event{makeEv(core.EventPlanCreated)}, nil
	})

	_, events, err := engine.Transition(context.Background(), session, makeEv(core.EventStart))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("handler was not called")
	}
	if len(events) != 1 || events[0].Type != core.EventPlanCreated {
		t.Errorf("expected PlanCreated event, got %v", events)
	}
}

func TestTransition_LoopDetectedGoesToError(t *testing.T) {
	engine := NewEngine()
	session := newTestSession("s4")
	session.SetState(core.StateExecuting)

	next, _, err := engine.Transition(context.Background(), session, makeEv(core.EventLoopDetected))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if next != core.StateError {
		t.Errorf("expected ERROR, got %s", next)
	}
}

func TestCurrentState(t *testing.T) {
	engine := NewEngine()
	session := newTestSession("s5")

	if engine.CurrentState("s5") != core.StateIdle {
		t.Error("initial state should be IDLE for unknown session")
	}

	engine.Transition(context.Background(), session, makeEv(core.EventStart))

	if engine.CurrentState("s5") != core.StatePlanning {
		t.Errorf("expected PLANNING, got %s", engine.CurrentState("s5"))
	}
}
