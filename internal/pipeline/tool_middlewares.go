package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/coohu/goagent/internal/core"
)

func FilterMiddleware(maxRawBytes int) Handler {
	return func(ctx context.Context, data any, next func(context.Context, any) error) error {
		tpc, ok := data.(*ToolPipelineCtx)
		if !ok {
			return next(ctx, data)
		}
		raw := tpc.ToolResult.RawOutput
		if len(raw) > maxRawBytes {
			raw = raw[:maxRawBytes] + "\n... [truncated]"
		}
		tpc.FilteredOutput = raw
		return next(ctx, tpc)
	}
}

func SummarizeMiddleware(llmCall func(ctx context.Context, goal, toolName, output string) (*ExtractedInfo, error)) Handler {
	return func(ctx context.Context, data any, next func(context.Context, any) error) error {
		tpc, ok := data.(*ToolPipelineCtx)
		if !ok {
			return next(ctx, data)
		}
		info, err := llmCall(ctx, tpc.Goal, tpc.ToolName, tpc.FilteredOutput)
		if err != nil {
			info = &ExtractedInfo{Summary: tpc.FilteredOutput}
		}
		tpc.ExtractedInfo = info
		tpc.Summary = info.Summary
		return next(ctx, tpc)
	}
}

func EmbedMiddleware(embedFn func(ctx context.Context, text string) ([]float32, error)) Handler {
	return func(ctx context.Context, data any, next func(context.Context, any) error) error {
		tpc, ok := data.(*ToolPipelineCtx)
		if !ok {
			return next(ctx, data)
		}
		if tpc.Summary != "" {
			if vec, err := embedFn(ctx, tpc.Summary); err == nil {
				tpc.Embedding = vec
			}
		}
		return next(ctx, tpc)
	}
}

func StoreMiddleware(storeFn func(ctx context.Context, mem *core.ToolMemory) error, sessionID string) Handler {
	return func(ctx context.Context, data any, next func(context.Context, any) error) error {
		tpc, ok := data.(*ToolPipelineCtx)
		if !ok {
			return next(ctx, data)
		}
		mem := &core.ToolMemory{
			ID:        fmt.Sprintf("%s-step%d", sessionID, tpc.StepID),
			SessionID: sessionID,
			StepID:    tpc.StepID,
			ToolName:  tpc.ToolName,
			RawOutput: tpc.FilteredOutput,
			Summary:   tpc.Summary,
			Embedding: tpc.Embedding,
		}
		if tpc.ExtractedInfo != nil {
			mem.KeyPoints = tpc.ExtractedInfo.KeyPoints
			mem.Entities = tpc.ExtractedInfo.Entities
			mem.Numbers = tpc.ExtractedInfo.Numbers
		}
		_ = storeFn(ctx, mem)
		return next(ctx, tpc)
	}
}

func BuildSummarizeCall(llmClient core.LLMClient) func(ctx context.Context, goal, toolName, output string) (*ExtractedInfo, error) {
	return func(ctx context.Context, goal, toolName, output string) (*ExtractedInfo, error) {
		prompt := fmt.Sprintf(`Tool: %s
User Goal: %s

Raw Tool Output:
%s

Extract the following and return ONLY valid JSON (no markdown):
{
  "summary": "short summary under 100 words",
  "key_points": ["point1", "point2"],
  "entities": [{"name": "...", "type": "file|function|error|command"}],
  "numbers": ["any metrics or counts"],
  "relevance": "high|medium|low"
}`, toolName, goal, truncate(output, 4000))

		resp, err := llmClient.ChatComplete(ctx, &core.ChatRequest{
			Messages: []core.Message{
				{Role: "system", Content: "You are a precise tool result summarizer. Return only valid JSON."},
				{Role: "user", Content: prompt},
			},
			MaxTokens: 500,
		})
		if err != nil {
			return nil, err
		}
		var info ExtractedInfo
		if err := json.Unmarshal([]byte(extractJSON(resp.Content)), &info); err != nil {
			return &ExtractedInfo{Summary: resp.Content}, nil
		}
		return &info, nil
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return s
}
