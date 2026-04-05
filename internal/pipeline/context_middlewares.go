package pipeline

import (
	"context"
	"fmt"
	"strings"

	"github.com/coohu/goagent/internal/core"
)

func LoadGoalMiddleware() Handler {
	return func(ctx context.Context, data any, next func(context.Context, any) error) error {
		return next(ctx, data)
	}
}

func LoadPlanMiddleware() Handler {
	return func(ctx context.Context, data any, next func(context.Context, any) error) error {
		return next(ctx, data)
	}
}

func RetrieveMemoryMiddleware(searchFn func(ctx context.Context, query, sessionID string, topK int) ([]*core.ToolMemory, error)) Handler {
	return func(ctx context.Context, data any, next func(context.Context, any) error) error {
		cbc, ok := data.(*ContextBuildCtx)
		if !ok {
			return next(ctx, data)
		}
		query := buildSearchQuery(cbc)
		results, err := searchFn(ctx, query, cbc.Session.ID, 10)
		if err == nil {
			cbc.SearchResults = results
		}
		return next(ctx, cbc)
	}
}

func TokenBudgetMiddleware(maxTokens int) Handler {
	return func(ctx context.Context, data any, next func(context.Context, any) error) error {
		cbc, ok := data.(*ContextBuildCtx)
		if !ok {
			return next(ctx, data)
		}
		if len(cbc.SearchResults) > 5 {
			cbc.SearchResults = cbc.SearchResults[:5]
		}
		if len(cbc.RecentMessages) > 20 {
			cbc.RecentMessages = cbc.RecentMessages[len(cbc.RecentMessages)-20:]
		}
		return next(ctx, cbc)
	}
}

func BuildPromptMiddleware(systemPrompt string, toolSchemas []core.ToolSchema) Handler {
	return func(ctx context.Context, data any, next func(context.Context, any) error) error {
		cbc, ok := data.(*ContextBuildCtx)
		if !ok {
			return next(ctx, data)
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("[Goal]\n%s\n\n", cbc.Goal))

		if cbc.Plan != nil {
			sb.WriteString("[Current Plan]\n")
			for i, step := range cbc.Plan.Steps {
				marker := " "
				if i < cbc.Plan.CurrentStep {
					marker = "✓"
				} else if i == cbc.Plan.CurrentStep {
					marker = "→"
				}
				sb.WriteString(fmt.Sprintf("%s Step %d: %s\n", marker, step.ID, step.Name))
			}
			sb.WriteString("\n")
		}

		if len(cbc.SearchResults) > 0 {
			sb.WriteString("[Relevant Tool Results]\n")
			for _, mem := range cbc.SearchResults {
				sb.WriteString(fmt.Sprintf("- [%s] %s\n", mem.ToolName, mem.Summary))
			}
			sb.WriteString("\n")
		}

		if cbc.Scratchpad != nil && len(cbc.Scratchpad.Entries) > 0 {
			sb.WriteString("[Scratchpad]\n")
			for _, e := range cbc.Scratchpad.Entries {
				if e.Thought != "" {
					sb.WriteString(fmt.Sprintf("Thought: %s\n", e.Thought))
				}
				if e.Observation != "" {
					sb.WriteString(fmt.Sprintf("Observation: %s\n", e.Observation))
				}
			}
		}

		messages := []core.Message{{Role: "system", Content: systemPrompt}}
		for _, m := range cbc.RecentMessages {
			messages = append(messages, m)
		}
		messages = append(messages, core.Message{Role: "user", Content: sb.String()})

		cbc.BuiltPrompt = &core.Prompt{
			System:   systemPrompt,
			Messages: messages,
			Tools:    toolSchemas,
		}
		return next(ctx, cbc)
	}
}

func buildSearchQuery(cbc *ContextBuildCtx) string {
	parts := []string{cbc.Goal}
	if cbc.Plan != nil && cbc.Plan.CurrentStep < len(cbc.Plan.Steps) {
		parts = append(parts, cbc.Plan.Steps[cbc.Plan.CurrentStep].Name)
	}
	if len(cbc.RecentToolResults) > 0 {
		last := cbc.RecentToolResults[len(cbc.RecentToolResults)-1]
		if last.Summary != "" {
			parts = append(parts, last.Summary)
		}
	}
	return strings.Join(parts, " ")
}
