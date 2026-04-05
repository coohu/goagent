package pipeline

import (
	"github.com/coohu/goagent/internal/core"
)

type ToolPipelineCtx struct {
	core.AgentContext
	ToolResult     *core.ToolResult
	ToolName       string
	StepID         int
	FilteredOutput string
	ExtractedInfo  *ExtractedInfo
	Summary        string
	Embedding      []float32
}

type ExtractedInfo struct {
	Summary   string        `json:"summary"`
	KeyPoints []string      `json:"key_points"`
	Entities  []core.Entity `json:"entities"`
	Numbers   []string      `json:"numbers"`
	Relevance string        `json:"relevance"`
}

type ReflectionCtx struct {
	core.AgentContext
	ToolResult  *core.ToolResult
	StepName    string
	ToolName    string
	RetryCount  int
	ReplanCount int
	Result      *ReflectionResult
}

type ContextBuildCtx struct {
	core.AgentContext
	Session       *core.AgentSession
	SearchResults []*core.ToolMemory
	BuiltPrompt   *core.Prompt
}
