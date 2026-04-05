package core

import "time"

type AgentState string

const (
	StateIdle          AgentState = "IDLE"
	StatePlanning      AgentState = "PLANNING"
	StateExecuting     AgentState = "EXECUTING"
	StateWaitTool      AgentState = "WAIT_TOOL"
	StateProcessResult AgentState = "PROCESS_RESULT"
	StateUpdateMemory  AgentState = "UPDATE_MEMORY"
	StateBuildContext  AgentState = "BUILD_CONTEXT"
	StateLLMThinking   AgentState = "LLM_THINKING"
	StateReflecting    AgentState = "REFLECTING"
	StateReplanning    AgentState = "REPLANNING"
	StateNextStep      AgentState = "NEXT_STEP"
	StateWaitUser      AgentState = "WAIT_USER_INPUT"
	StateDone          AgentState = "DONE"
	StateError         AgentState = "ERROR"
	StateCancelled     AgentState = "CANCELLED"
	StateTimeout       AgentState = "TIMEOUT"
)

type EventType string

const (
	EventStart          EventType = "start"
	EventStop           EventType = "stop"
	EventCancel         EventType = "cancel"
	EventTimeout        EventType = "timeout"
	EventLoopDetected   EventType = "loop_detected"
	EventCostExceeded   EventType = "cost_exceeded"
	EventPlanRequested  EventType = "plan_requested"
	EventPlanCreated    EventType = "plan_created"
	EventPlanUpdated    EventType = "plan_updated"
	EventStepStart      EventType = "step_start"
	EventStepDone       EventType = "step_done"
	EventStepFailed     EventType = "step_failed"
	EventToolCall       EventType = "tool_call"
	EventToolResult     EventType = "tool_result"
	EventToolError      EventType = "tool_error"
	EventMemoryUpdate   EventType = "memory_update"
	EventMemoryUpdated  EventType = "memory_updated"
	EventLLMRequest     EventType = "llm_request"
	EventLLMResponse    EventType = "llm_response"
	EventLLMError       EventType = "llm_error"
	EventReflectStart   EventType = "reflect_start"
	EventReflectDone    EventType = "reflect_done"
	EventReplanNeeded   EventType = "replan_needed"
	EventContextBuilt   EventType = "context_built"
	EventHasNextStep    EventType = "has_next_step"
	EventAllDone        EventType = "all_done"
	EventProcessed      EventType = "processed"
	EventApprovalNeeded EventType = "approval_required"
	EventApproved       EventType = "approved"
)

type Event struct {
	ID        string
	Type      EventType
	Payload   map[string]any
	SessionID string
	Priority  int
	TTL       int
	Hash      string
	CreatedAt time.Time
	ExpiresAt time.Time
}

type StepStatus string

const (
	StepPending StepStatus = "pending"
	StepRunning StepStatus = "running"
	StepDone    StepStatus = "done"
	StepFailed  StepStatus = "failed"
	StepSkipped StepStatus = "skipped"
)

type Step struct {
	ID          int        `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Tool        string     `json:"tool"`
	ToolInput   any        `json:"tool_input,omitempty"`
	Status      StepStatus `json:"status"`
	Result      string     `json:"result,omitempty"`
	RetryCount  int        `json:"retry_count"`
}

type Plan struct {
	ID          string    `json:"id"`
	Goal        string    `json:"goal"`
	Steps       []Step    `json:"steps"`
	CurrentStep int       `json:"current_step"`
	Version     int       `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type ToolResult struct {
	Success      bool          `json:"success"`
	Stdout       string        `json:"stdout,omitempty"`
	Stderr       string        `json:"stderr,omitempty"`
	ExitCode     int           `json:"exit_code,omitempty"`
	Data         map[string]any `json:"data,omitempty"`
	RawOutput    string        `json:"raw_output"`
	FilesChanged []string      `json:"files_changed,omitempty"`
	Duration     time.Duration `json:"duration"`
}

type ToolCall struct {
	ToolName  string         `json:"tool_name"`
	Input     map[string]any `json:"input"`
	SessionID string         `json:"session_id"`
	StepID    int            `json:"step_id"`
}

type ToolSchema struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Prompt struct {
	System     string
	Messages   []Message
	Tools      []ToolSchema
	TokensUsed int
}

type Entity struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type ToolMemory struct {
	ID        string    `json:"id" db:"id"`
	SessionID string    `json:"session_id" db:"session_id"`
	StepID    int       `json:"step_id" db:"step_id"`
	ToolName  string    `json:"tool_name" db:"tool_name"`
	RawOutput string    `json:"raw_output" db:"raw_output"`
	Summary   string    `json:"summary" db:"summary"`
	KeyPoints []string  `json:"key_points" db:"key_points"`
	Entities  []Entity  `json:"entities" db:"entities"`
	Numbers   []string  `json:"numbers" db:"numbers"`
	Embedding []float32 `json:"embedding,omitempty"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

type Episode struct {
	ID        string    `json:"id" db:"id"`
	Goal      string    `json:"goal" db:"goal"`
	Plan      string    `json:"plan" db:"plan"`
	Result    string    `json:"result" db:"result"`
	Success   bool      `json:"success" db:"success"`
	Summary   string    `json:"summary" db:"summary"`
	Embedding []float32 `json:"embedding,omitempty"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

type ScratchpadEntry struct {
	Turn        int    `json:"turn"`
	Thought     string `json:"thought"`
	Action      string `json:"action"`
	Observation string `json:"observation"`
}

type Scratchpad struct {
	Entries   []ScratchpadEntry
	MaxTokens int
}

type SessionMetrics struct {
	StepCount      int
	LLMCallCount   int
	ToolCallCount  int
	ReplanCount    int
	TokensConsumed int
	TotalCost      float64
	StartTime      time.Time
}
