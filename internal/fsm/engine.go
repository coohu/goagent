package fsm

import (
	"context"
	"fmt"
	"sync"

	"github.com/coohu/goagent/internal/core"
)

type Engine struct {
	mu       sync.RWMutex
	handlers map[core.AgentState]core.StateHandler
	states   map[string]core.AgentState
}

func NewEngine() *Engine {
	return &Engine{
		handlers: make(map[core.AgentState]core.StateHandler),
		states:   make(map[string]core.AgentState),
	}
}

func (e *Engine) RegisterHandler(state core.AgentState, handler core.StateHandler) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.handlers[state] = handler
}

func (e *Engine) CurrentState(sessionID string) core.AgentState {
	e.mu.RLock()
	defer e.mu.RUnlock()
	s, ok := e.states[sessionID]
	if !ok {
		return core.StateIdle
	}
	return s
}

func (e *Engine) Transition(ctx context.Context, session *core.AgentSession, event core.Event) (core.AgentState, []core.Event, error) {
	current := session.GetState()

	next, ok := nextState(current, event.Type)
	if !ok {
		return current, nil, nil
	}

	session.SetState(next)

	e.mu.Lock()
	e.states[session.ID] = next
	e.mu.Unlock()

	e.mu.RLock()
	handler, hasHandler := e.handlers[next]
	e.mu.RUnlock()

	if !hasHandler {
		return next, nil, nil
	}

	newEvents, err := handler(ctx, session, event)
	if err != nil {
		return next, nil, fmt.Errorf("handler for state %s: %w", next, err)
	}

	return next, newEvents, nil
}
