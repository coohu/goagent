package memory

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/coohu/goagent/internal/core"
)

type InMemoryManager struct {
	mu            sync.RWMutex
	toolMemories  map[string][]*core.ToolMemory
	episodes      []*core.Episode
	conversations map[string][]*core.Message
}

func NewInMemoryManager() *InMemoryManager {
	return &InMemoryManager{
		toolMemories:  make(map[string][]*core.ToolMemory),
		conversations: make(map[string][]*core.Message),
	}
}

func (m *InMemoryManager) SaveToolMemory(_ context.Context, mem *core.ToolMemory) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.toolMemories[mem.SessionID] = append(m.toolMemories[mem.SessionID], mem)
	return nil
}

func (m *InMemoryManager) SearchToolMemory(_ context.Context, query, sessionID string, topK int) ([]*core.ToolMemory, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	mems := m.toolMemories[sessionID]
	type scored struct {
		mem   *core.ToolMemory
		score int
	}
	var results []scored
	queryLower := strings.ToLower(query)
	for _, mem := range mems {
		score := 0
		if strings.Contains(strings.ToLower(mem.Summary), queryLower) {
			score += 2
		}
		for _, kp := range mem.KeyPoints {
			if strings.Contains(strings.ToLower(kp), queryLower) {
				score++
			}
		}
		if score > 0 {
			results = append(results, scored{mem, score})
		}
	}
	sort.Slice(results, func(i, j int) bool { return results[i].score > results[j].score })

	out := make([]*core.ToolMemory, 0, topK)
	for i, r := range results {
		if i >= topK {
			break
		}
		out = append(out, r.mem)
	}
	return out, nil
}

func (m *InMemoryManager) SaveEpisode(_ context.Context, ep *core.Episode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.episodes = append(m.episodes, ep)
	return nil
}

func (m *InMemoryManager) SearchEpisodes(_ context.Context, query string, topK int) ([]*core.Episode, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	queryLower := strings.ToLower(query)
	var out []*core.Episode
	for _, ep := range m.episodes {
		if strings.Contains(strings.ToLower(ep.Goal), queryLower) ||
			strings.Contains(strings.ToLower(ep.Summary), queryLower) {
			out = append(out, ep)
		}
		if len(out) >= topK {
			break
		}
	}
	return out, nil
}

func (m *InMemoryManager) AppendMessage(_ context.Context, sessionID string, msg *core.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.conversations[sessionID] = append(m.conversations[sessionID], msg)
	return nil
}

func (m *InMemoryManager) GetConversation(_ context.Context, sessionID string, limit int) ([]*core.Message, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	msgs := m.conversations[sessionID]
	if limit > 0 && len(msgs) > limit {
		msgs = msgs[len(msgs)-limit:]
	}
	return msgs, nil
}

func (m *InMemoryManager) ClearSession(_ context.Context, sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.toolMemories, sessionID)
	delete(m.conversations, sessionID)
	return nil
}
