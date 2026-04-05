package agent

import (
	"fmt"
	"sync"

	"github.com/coohu/goagent/internal/core"
	"github.com/google/uuid"
)

type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*core.AgentSession
	maxConc  int
}

func NewSessionManager(maxConcurrent int) *SessionManager {
	return &SessionManager{
		sessions: make(map[string]*core.AgentSession),
		maxConc:  maxConcurrent,
	}
}

func (m *SessionManager) Create(goal string, cfg *core.AgentConfig) (*core.AgentSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	active := 0
	for _, s := range m.sessions {
		st := s.GetState()
		if st != core.StateDone && st != core.StateError && st != core.StateCancelled && st != core.StateTimeout {
			active++
		}
	}
	if active >= m.maxConc {
		return nil, core.Errorf(core.ErrSessionFull, fmt.Sprintf("max concurrent sessions (%d) reached", m.maxConc), nil)
	}

	id := uuid.NewString()
	session := core.NewSession(id, goal, cfg)
	m.sessions[id] = session
	return session, nil
}

func (m *SessionManager) Get(id string) (*core.AgentSession, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[id]
	if !ok {
		return nil, core.Errorf(core.ErrSessionNotFound, "session not found", nil)
	}
	return s, nil
}

func (m *SessionManager) Cancel(id string) error {
	s, err := m.Get(id)
	if err != nil {
		return err
	}
	s.SetState(core.StateCancelled)
	return nil
}

func (m *SessionManager) Delete(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, id)
}

func (m *SessionManager) List() []*core.AgentSession {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*core.AgentSession, 0, len(m.sessions))
	for _, s := range m.sessions {
		out = append(out, s)
	}
	return out
}
