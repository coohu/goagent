package pipeline

import (
	"context"
	"testing"

	"github.com/coohu/goagent/internal/core"
)

func TestPipelineRunsInOrder(t *testing.T) {
	order := []int{}

	p := New(
		func(_ context.Context, data any, next func(context.Context, any) error) error {
			order = append(order, 1)
			return next(context.Background(), data)
		},
		func(_ context.Context, data any, next func(context.Context, any) error) error {
			order = append(order, 2)
			return next(context.Background(), data)
		},
		func(_ context.Context, data any, next func(context.Context, any) error) error {
			order = append(order, 3)
			return next(context.Background(), data)
		},
	)

	if err := p.Run(context.Background(), nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(order) != 3 || order[0] != 1 || order[1] != 2 || order[2] != 3 {
		t.Errorf("expected [1,2,3], got %v", order)
	}
}

func TestFilterMiddleware(t *testing.T) {
	tpc := &ToolPipelineCtx{
		ToolResult: &core.ToolResult{RawOutput: "hello world this is a long string"},
		ToolName:   "file.read",
	}

	p := New(FilterMiddleware(10))
	if err := p.Run(context.Background(), tpc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(tpc.FilteredOutput) != 10+len("\n... [truncated]") {
		t.Logf("FilteredOutput: %q", tpc.FilteredOutput)
	}
	if tpc.FilteredOutput == "" {
		t.Error("FilteredOutput should not be empty")
	}
}

func TestFilterMiddlewareNoTruncation(t *testing.T) {
	tpc := &ToolPipelineCtx{
		ToolResult: &core.ToolResult{RawOutput: "short"},
		ToolName:   "file.read",
	}

	p := New(FilterMiddleware(1000))
	p.Run(context.Background(), tpc)

	if tpc.FilteredOutput != "short" {
		t.Errorf("expected 'short', got %q", tpc.FilteredOutput)
	}
}

func TestReflectionEvaluateFailedTool(t *testing.T) {
	rc := &ReflectionCtx{
		ToolResult:  &core.ToolResult{Success: false, Stderr: "command not found"},
		ToolName:    "shell.exec",
		RetryCount:  0,
		ReplanCount: 0,
	}

	p := New(EvaluateToolResultMiddleware(), GuardLimitsMiddleware())
	p.Run(context.Background(), rc)

	if rc.Result == nil {
		t.Fatal("result should be set")
	}
	if rc.Result.Action != "retry" {
		t.Errorf("expected retry, got %s", rc.Result.Action)
	}
}

func TestReflectionEscalatesAfterMaxRetry(t *testing.T) {
	rc := &ReflectionCtx{
		ToolResult:  &core.ToolResult{Success: false, Stderr: "error"},
		ToolName:    "shell.exec",
		RetryCount:  3,
		ReplanCount: 0,
	}

	p := New(EvaluateToolResultMiddleware(), GuardLimitsMiddleware())
	p.Run(context.Background(), rc)

	if rc.Result.Action != "replan" {
		t.Errorf("expected replan after max retries, got %s", rc.Result.Action)
	}
}

func TestGuardLimitsAbortsWhenReplanExhausted(t *testing.T) {
	rc := &ReflectionCtx{
		ToolResult:  &core.ToolResult{Success: false, Stderr: "err"},
		ToolName:    "shell.exec",
		RetryCount:  5,
		ReplanCount: 5,
		Result:      &ReflectionResult{Action: "replan"},
	}

	p := New(GuardLimitsMiddleware())
	p.Run(context.Background(), rc)

	if rc.Result.Action != "abort" {
		t.Errorf("expected abort when replan exhausted, got %s", rc.Result.Action)
	}
}
