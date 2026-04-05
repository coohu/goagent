package agent

import (
	"context"
	"fmt"

	"github.com/coohu/goagent/internal/core"
	"github.com/coohu/goagent/internal/tools/registry"
)

type DefaultToolRunner struct {
	registry *registry.Registry
}

func NewDefaultToolRunner(reg *registry.Registry) *DefaultToolRunner {
	return &DefaultToolRunner{registry: reg}
}

func (t *DefaultToolRunner) Run(ctx context.Context, call *core.ToolCall, _ core.ToolRegistry) (*core.ToolResult, error) {
	tool, err := t.registry.Get(call.ToolName)
	if err != nil {
		return nil, core.Errorf(core.ErrToolNotFound, fmt.Sprintf("tool %q not found", call.ToolName), err)
	}

	if err := tool.Validate(call.Input); err != nil {
		return nil, core.Errorf(core.ErrToolNotFound, "invalid tool input", err)
	}

	return tool.Execute(ctx, call.Input)
}
