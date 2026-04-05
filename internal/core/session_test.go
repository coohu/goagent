package core

import (
	"testing"
	"time"
)

func TestNewSession(t *testing.T) {
	cfg := DefaultConfig()
	s := NewSession("id1", "my goal", cfg)

	if s.ID != "id1" {
		t.Errorf("wrong ID: %s", s.ID)
	}
	if s.Goal != "my goal" {
		t.Errorf("wrong goal: %s", s.Goal)
	}
	if s.GetState() != StateIdle {
		t.Errorf("initial state should be IDLE")
	}
	if s.Config.MaxSteps != 30 {
		t.Errorf("wrong default MaxSteps: %d", s.Config.MaxSteps)
	}
}

func TestSetAndGetState(t *testing.T) {
	s := NewSession("id2", "goal", nil)
	s.SetState(StatePlanning)
	if s.GetState() != StatePlanning {
		t.Errorf("expected PLANNING, got %s", s.GetState())
	}
}

func TestExceedsLimits_MaxSteps(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxSteps = 3
	s := NewSession("id3", "goal", cfg)
	s.Metrics.StepCount = 3

	exceeded, reason := s.ExceedsLimits()
	if !exceeded {
		t.Error("expected limits exceeded")
	}
	if reason != "max_steps" {
		t.Errorf("expected max_steps reason, got %s", reason)
	}
}

func TestExceedsLimits_NotExceeded(t *testing.T) {
	s := NewSession("id4", "goal", nil)
	exceeded, _ := s.ExceedsLimits()
	if exceeded {
		t.Error("new session should not exceed limits")
	}
}

func TestIncrMetrics(t *testing.T) {
	s := NewSession("id5", "goal", nil)
	s.IncrLLM()
	s.IncrLLM()
	s.IncrTool()
	s.IncrReplan()
	s.AddTokens(500)

	if s.Metrics.LLMCallCount != 2 {
		t.Errorf("expected 2 LLM calls, got %d", s.Metrics.LLMCallCount)
	}
	if s.Metrics.ToolCallCount != 1 {
		t.Errorf("expected 1 tool call, got %d", s.Metrics.ToolCallCount)
	}
	if s.Metrics.ReplanCount != 1 {
		t.Errorf("expected 1 replan, got %d", s.Metrics.ReplanCount)
	}
	if s.Metrics.TokensConsumed != 500 {
		t.Errorf("expected 500 tokens, got %d", s.Metrics.TokensConsumed)
	}
}

func TestSessionEventChan(t *testing.T) {
	s := NewSession("id6", "goal", nil)

	ev := Event{Type: EventStart}
	s.EventChan <- ev

	select {
	case got := <-s.EventChan:
		if got.Type != EventStart {
			t.Errorf("wrong event type: %s", got.Type)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("event not received")
	}
}
