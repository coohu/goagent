package core

import "context"

type Tool interface {
	Name() string
	Description() string
	Schema() ToolSchema
	Execute(ctx context.Context, input map[string]any) (*ToolResult, error)
	Validate(input map[string]any) error
}

type ToolRegistry interface {
	Register(tool Tool) error
	Get(name string) (Tool, error)
	List() []Tool
	ListAllowed(names []string) []Tool
	Schemas(names []string) []ToolSchema
}

type LLMClient interface {
	ChatComplete(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
	ChatStream(ctx context.Context, req *ChatRequest) (<-chan ChatChunk, error)
	ChatWithTools(ctx context.Context, req *ChatRequest, tools []ToolSchema) (*ChatResponse, error)
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

type ChatRequest struct {
	Model    string
	Messages []Message
	MaxTokens int
}

type ChatResponse struct {
	Content    string
	ToolCalls  []ToolCallResponse
	TokensUsed int
	FinishReason string
}

type ToolCallResponse struct {
	ID    string
	Name  string
	Input map[string]any
}

type ChatChunk struct {
	Delta string
	Done  bool
	Err   error
}

type Planner interface {
	CreatePlan(ctx context.Context, goal string, agentCtx *AgentContext) (*Plan, error)
	Replan(ctx context.Context, plan *Plan, reason string, agentCtx *AgentContext) (*Plan, error)
}

type Executor interface {
	ExecuteStep(ctx context.Context, session *AgentSession) (*ToolCall, error)
}

type MemoryManager interface {
	SaveToolMemory(ctx context.Context, mem *ToolMemory) error
	SearchToolMemory(ctx context.Context, query, sessionID string, topK int) ([]*ToolMemory, error)
	SaveEpisode(ctx context.Context, ep *Episode) error
	SearchEpisodes(ctx context.Context, query string, topK int) ([]*Episode, error)
	AppendMessage(ctx context.Context, sessionID string, msg *Message) error
	GetConversation(ctx context.Context, sessionID string, limit int) ([]*Message, error)
	ClearSession(ctx context.Context, sessionID string) error
}

type ContextBuilder interface {
	Build(ctx context.Context, session *AgentSession) (*Prompt, error)
}

type EventBus interface {
	Emit(ctx context.Context, event Event) error
	Subscribe(eventType EventType, handler EventHandler) func()
	SubscribeSession(sessionID string, handler EventHandler) func()
	Shutdown(ctx context.Context) error
}

type EventHandler func(ctx context.Context, event Event)

type FSMEngine interface {
	Transition(ctx context.Context, session *AgentSession, event Event) (AgentState, []Event, error)
	RegisterHandler(state AgentState, handler StateHandler)
	CurrentState(sessionID string) AgentState
}

type StateHandler func(ctx context.Context, session *AgentSession, event Event) ([]Event, error)


