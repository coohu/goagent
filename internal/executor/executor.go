package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/coohu/goagent/internal/core"
	"github.com/coohu/goagent/internal/llm"
)

type Executor struct {
	router   *llm.Router
	registry core.ToolRegistry
}

func New(router *llm.Router, reg core.ToolRegistry) *Executor {
	return &Executor{router: router, registry: reg}
}

func (e *Executor) ExecuteStep(ctx context.Context, session *core.AgentSession) (*core.ToolCall, error) {
	if session.Plan == nil || session.Plan.CurrentStep >= len(session.Plan.Steps) {
		return nil, fmt.Errorf("no current step")
	}

	step := session.Plan.Steps[session.Plan.CurrentStep]
	schemas := e.registry.Schemas(session.Config.AllowedTools)
	models := &session.Config.Models

	client, err := e.router.For(llm.SceneExecute, models)
	if err != nil {
		return nil, err
	}

	messages := buildMessages(session, step)

	for turn := 0; turn < session.Config.ReActMaxTurns; turn++ {
		resp, err := client.ChatWithTools(ctx, &core.ChatRequest{
			Messages:  messages,
			MaxTokens: 2000,
		}, schemas)
		if err != nil {
			return nil, core.Errorf(core.ErrLLMTimeout, "executor LLM call failed", err)
		}

		session.IncrLLM()
		session.AddTokens(resp.TokensUsed)

		if len(resp.ToolCalls) > 0 {
			tc := resp.ToolCalls[0]
			return &core.ToolCall{
				ToolName:  tc.Name,
				Input:     tc.Input,
				SessionID: session.ID,
				StepID:    step.ID,
			}, nil
		}

		if resp.Content != "" {
			messages = append(messages, core.Message{Role: "assistant", Content: resp.Content})
			appendScratchpad(session, turn, resp.Content, "", "")
		}
		if resp.FinishReason == "stop" {
			break
		}
	}

	return &core.ToolCall{
		ToolName:  step.Tool,
		Input:     inputFromStep(step),
		SessionID: session.ID,
		StepID:    step.ID,
	}, nil
}

func buildMessages(session *core.AgentSession, step core.Step) []core.Message {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Goal: %s\n\n", session.Goal))
	sb.WriteString(fmt.Sprintf("Current Step (%d/%d): %s\n", step.ID, len(session.Plan.Steps), step.Name))
	if step.Description != "" {
		sb.WriteString(fmt.Sprintf("Description: %s\n", step.Description))
	}
	if step.Tool != "" {
		sb.WriteString(fmt.Sprintf("Suggested Tool: %s\n", step.Tool))
	}
	if session.AgentCtx.Scratchpad != nil && len(session.AgentCtx.Scratchpad.Entries) > 0 {
		sb.WriteString("\nPrevious reasoning:\n")
		for _, e := range session.AgentCtx.Scratchpad.Entries {
			if e.Thought != "" {
				sb.WriteString(fmt.Sprintf("Thought: %s\n", e.Thought))
			}
			if e.Observation != "" {
				sb.WriteString(fmt.Sprintf("Observation: %s\n", e.Observation))
			}
		}
	}
	return []core.Message{
		{Role: "system", Content: executorSystemPrompt},
		{Role: "user", Content: sb.String()},
	}
}

func appendScratchpad(session *core.AgentSession, turn int, thought, action, observation string) {
	if session.AgentCtx.Scratchpad == nil {
		session.AgentCtx.Scratchpad = &core.Scratchpad{}
	}
	session.AgentCtx.Scratchpad.Entries = append(session.AgentCtx.Scratchpad.Entries, core.ScratchpadEntry{
		Turn: turn, Thought: thought, Action: action, Observation: observation,
	})
}

func inputFromStep(step core.Step) map[string]any {
	if step.ToolInput == nil {
		return map[string]any{}
	}
	if m, ok := step.ToolInput.(map[string]any); ok {
		return m
	}
	data, _ := json.Marshal(step.ToolInput)
	var m map[string]any
	_ = json.Unmarshal(data, &m)
	return m
}

const executorSystemPrompt = `You are an expert AI agent. Execute the given step using the available tools. Think carefully, then use exactly one tool call when ready to act.`
