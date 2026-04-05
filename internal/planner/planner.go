package planner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/coohu/goagent/internal/core"
	"github.com/coohu/goagent/internal/llm"
	"github.com/google/uuid"
)

type Planner struct {
	router *llm.Router
}

func New(router *llm.Router) *Planner {
	return &Planner{router: router}
}

func (p *Planner) CreatePlan(ctx context.Context, goal string, agentCtx *core.AgentContext) (*core.Plan, error) {
	client, err := p.router.For(llm.ScenePlanning)
	if err != nil {
		return nil, err
	}

	tools := availableToolsDesc(agentCtx)
	prompt := buildCreatePrompt(goal, tools)

	resp, err := client.ChatComplete(ctx, &core.ChatRequest{
		Messages: []core.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: prompt},
		},
		MaxTokens: 2000,
	})
	if err != nil {
		return nil, core.Errorf(core.ErrPlanFailed, "LLM call failed", err)
	}

	steps, err := parseSteps(resp.Content)
	if err != nil {
		return nil, core.Errorf(core.ErrPlanFailed, "failed to parse plan JSON", err)
	}
	if len(steps) > 50 {
		return nil, core.Errorf(core.ErrPlanTooLarge, "plan has too many steps", nil)
	}

	return &core.Plan{
		ID:        uuid.NewString(),
		Goal:      goal,
		Steps:     steps,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, nil
}

func (p *Planner) Replan(ctx context.Context, plan *core.Plan, reason string, agentCtx *core.AgentContext) (*core.Plan, error) {
	client, err := p.router.For(llm.ScenePlanning)
	if err != nil {
		return nil, err
	}

	planJSON, _ := json.Marshal(plan.Steps)
	prompt := buildReplanPrompt(plan.Goal, string(planJSON), plan.CurrentStep, reason)

	resp, err := client.ChatComplete(ctx, &core.ChatRequest{
		Messages: []core.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: prompt},
		},
		MaxTokens: 2000,
	})
	if err != nil {
		return nil, core.Errorf(core.ErrPlanFailed, "LLM replan failed", err)
	}

	newSteps, err := parseSteps(resp.Content)
	if err != nil {
		return nil, core.Errorf(core.ErrPlanFailed, "failed to parse replan JSON", err)
	}

	updated := *plan
	updated.Steps = append(plan.Steps[:plan.CurrentStep], newSteps...)
	updated.Version++
	updated.UpdatedAt = time.Now()
	return &updated, nil
}

func parseSteps(content string) ([]core.Step, error) {
	content = strings.TrimSpace(content)
	start := strings.Index(content, "[")
	end := strings.LastIndex(content, "]")
	if start == -1 || end == -1 || end <= start {
		return nil, fmt.Errorf("no JSON array found in response")
	}
	content = content[start : end+1]

	var raw []struct {
		ID          int    `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Tool        string `json:"tool"`
		ToolInput   any    `json:"tool_input"`
	}
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return nil, err
	}

	steps := make([]core.Step, len(raw))
	for i, r := range raw {
		id := r.ID
		if id == 0 {
			id = i + 1
		}
		steps[i] = core.Step{
			ID:          id,
			Name:        r.Name,
			Description: r.Description,
			Tool:        r.Tool,
			ToolInput:   r.ToolInput,
			Status:      core.StepPending,
		}
	}
	return steps, nil
}

func availableToolsDesc(agentCtx *core.AgentContext) string {
	return "shell.exec, file.read, file.write, file.list, file.search, search.web, rag.search, http.request, git.clone, git.commit"
}

func buildCreatePrompt(goal, tools string) string {
	return fmt.Sprintf(`You are a task planner for an AI agent system.

User Goal: %s

Available Tools: %s

Break this goal into concrete, executable steps.
Each step MUST bind to exactly one tool.
Steps should be atomic — not too large, not too small.

Return ONLY valid JSON array, no markdown, no explanation:
[
  {"id": 1, "name": "...", "description": "...", "tool": "...", "tool_input": {...}},
  ...
]`, goal, tools)
}

func buildReplanPrompt(goal, planJSON string, currentStep int, reason string) string {
	return fmt.Sprintf(`You are replanning an AI agent task.

Goal: %s
Current Plan Steps (JSON): %s
Current Step Index: %d
Failure Reason: %s

Fix the plan from step index %d onwards. Do NOT change completed steps.
Return ONLY the new/updated steps as a JSON array starting from the current step.
[
  {"id": %d, "name": "...", "description": "...", "tool": "...", "tool_input": {...}},
  ...
]`, goal, planJSON, currentStep, reason, currentStep, currentStep+1)
}

const systemPrompt = `You are an expert AI agent planner. You produce concise, executable plans as valid JSON only. Never add explanation or markdown.`
