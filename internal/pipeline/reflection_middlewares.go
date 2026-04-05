package pipeline

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/coohu/goagent/internal/core"
)

type ReflectionResult struct {
	Success      bool   `json:"success"`
	Action       string `json:"action"`
	Reason       string `json:"reason"`
	SuggestedFix string `json:"suggested_fix"`
}

func EvaluateToolResultMiddleware() Handler {
	return func(ctx context.Context, data any, next func(context.Context, any) error) error {
		rc, ok := data.(*ReflectionCtx)
		if !ok {
			return next(ctx, data)
		}
		if rc.ToolResult != nil && !rc.ToolResult.Success {
			action := "retry"
			if rc.RetryCount >= 2 {
				action = "replan"
			}
			rc.Result = &ReflectionResult{
				Success: false,
				Action:  action,
				Reason:  fmt.Sprintf("tool %s failed: %s", rc.ToolName, rc.ToolResult.Stderr),
			}
		}
		return next(ctx, rc)
	}
}

func LLMJudgeMiddleware(llmClient core.LLMClient) Handler {
	return func(ctx context.Context, data any, next func(context.Context, any) error) error {
		rc, ok := data.(*ReflectionCtx)
		if !ok || rc.Result != nil {
			return next(ctx, data)
		}

		rawOutput := ""
		if rc.ToolResult != nil {
			rawOutput = truncate(rc.ToolResult.RawOutput, 2000)
		}

		prompt := fmt.Sprintf(`Goal: %s
Step: %s
Tool: %s
Tool Result: %s
Retry Count: %d

Evaluate whether this step succeeded relative to the goal.
Return ONLY valid JSON:
{
  "success": true,
  "action": "continue",
  "reason": "brief explanation",
  "suggested_fix": ""
}`, rc.Goal, rc.StepName, rc.ToolName, rawOutput, rc.RetryCount)

		resp, err := llmClient.ChatComplete(ctx, &core.ChatRequest{
			Messages: []core.Message{
				{Role: "system", Content: "You are a precise task evaluator. Return only valid JSON."},
				{Role: "user", Content: prompt},
			},
			MaxTokens: 300,
		})
		if err != nil {
			rc.Result = &ReflectionResult{Success: true, Action: "continue", Reason: "llm judge unavailable"}
			return next(ctx, rc)
		}

		var result ReflectionResult
		if err := json.Unmarshal([]byte(extractJSON(resp.Content)), &result); err != nil {
			rc.Result = &ReflectionResult{Success: true, Action: "continue"}
			return next(ctx, rc)
		}
		rc.Result = &result
		return next(ctx, rc)
	}
}

func GuardLimitsMiddleware() Handler {
	return func(ctx context.Context, data any, next func(context.Context, any) error) error {
		rc, ok := data.(*ReflectionCtx)
		if !ok {
			return next(ctx, data)
		}
		if rc.ReplanCount >= 5 && rc.Result != nil && rc.Result.Action == "replan" {
			rc.Result.Action = "abort"
			rc.Result.Reason = "max replan count reached"
		}
		return next(ctx, rc)
	}
}
