package core

import (
	"context"
	"sync"
	"time"
)

type AgentConfig struct {
	MaxSteps            int           `json:"max_steps"`
	MaxRuntime          time.Duration `json:"max_runtime"`
	MaxLLMCalls         int           `json:"max_llm_calls"`
	MaxToolCalls        int           `json:"max_tool_calls"`
	MaxReplanCount      int           `json:"max_replan_count"`
	MaxTokenBudget      int           `json:"max_token_budget"`
	PlannerModel        string        `json:"planner_model"`
	ExecutorModel       string        `json:"executor_model"`
	SummaryModel        string        `json:"summary_model"`
	ReflectModel        string        `json:"reflect_model"`
	AllowedTools        []string      `json:"allowed_tools"`
	EnableBrowser       bool          `json:"enable_browser"`
	EnableSandbox       bool          `json:"enable_sandbox"`
	RequireApprovalFor  []string      `json:"require_approval_for"`
	ScratchpadMaxTokens int           `json:"scratchpad_max_tokens"`
	ReActMaxTurns       int           `json:"react_max_turns"`
}

func DefaultConfig() *AgentConfig {
	return &AgentConfig{
		MaxSteps:            30,
		MaxRuntime:          10 * time.Minute,
		MaxLLMCalls:         30,
		MaxToolCalls:        20,
		MaxReplanCount:      5,
		MaxTokenBudget:      200000,
		PlannerModel:        "gpt-4o",
		ExecutorModel:       "gpt-4o",
		SummaryModel:        "gpt-4o-mini",
		ReflectModel:        "gpt-4o-mini",
		EnableSandbox:       true,
		ScratchpadMaxTokens: 20000,
		ReActMaxTurns:       10,
	}
}

type AgentContext struct {
	Goal               string
	Plan               *Plan
	RecentMessages     []Message
	RecentToolResults  []*ToolMemory
	Scratchpad         *Scratchpad
	TokensUsed         int
}

type AgentSession struct {
	ID         string
	Goal       string
	State      AgentState
	Plan       *Plan
	AgentCtx   *AgentContext
	Metrics    *SessionMetrics
	Config     *AgentConfig
	CreatedAt  time.Time
	UpdatedAt  time.Time
	FinishedAt *time.Time

	mu        sync.RWMutex
	cancelFn  context.CancelFunc
	EventChan chan Event
}

func NewSession(id, goal string, cfg *AgentConfig) *AgentSession {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &AgentSession{
		ID:    id,
		Goal:  goal,
		State: StateIdle,
		AgentCtx: &AgentContext{
			Goal:       goal,
			Scratchpad: &Scratchpad{MaxTokens: cfg.ScratchpadMaxTokens},
		},
		Metrics: &SessionMetrics{
			StartTime: time.Now(),
		},
		Config:    cfg,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		EventChan: make(chan Event, 256),
	}
}

func (s *AgentSession) SetState(state AgentState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.State = state
	s.UpdatedAt = time.Now()
}

func (s *AgentSession) GetState() AgentState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.State
}

func (s *AgentSession) IncrLLM() {
	s.mu.Lock()
	s.Metrics.LLMCallCount++
	s.mu.Unlock()
}

func (s *AgentSession) IncrTool() {
	s.mu.Lock()
	s.Metrics.ToolCallCount++
	s.mu.Unlock()
}

func (s *AgentSession) IncrReplan() {
	s.mu.Lock()
	s.Metrics.ReplanCount++
	s.mu.Unlock()
}

func (s *AgentSession) AddTokens(n int) {
	s.mu.Lock()
	s.Metrics.TokensConsumed += n
	s.mu.Unlock()
}

func (s *AgentSession) ExceedsLimits() (bool, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, c := s.Metrics, s.Config
	switch {
	case m.StepCount >= c.MaxSteps:
		return true, "max_steps"
	case m.LLMCallCount >= c.MaxLLMCalls:
		return true, "max_llm_calls"
	case m.ToolCallCount >= c.MaxToolCalls:
		return true, "max_tool_calls"
	case m.ReplanCount >= c.MaxReplanCount:
		return true, "max_replan"
	case m.TokensConsumed >= c.MaxTokenBudget:
		return true, "max_tokens"
	}
	return false, ""
}
