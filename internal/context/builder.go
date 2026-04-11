package context

import (
	"context"

	"github.com/coohu/goagent/internal/core"
	"github.com/coohu/goagent/internal/pipeline"
	"github.com/coohu/goagent/internal/tools/registry"
)

const agentSystemPrompt = `You are an expert AI software engineering agent. You reason step by step, use tools precisely, and always stay focused on the user's goal. When you are ready to act, use exactly one tool call.`

type Builder struct {
	memory    core.MemoryManager
	registry  *registry.Registry
	maxTokens int
}

func NewBuilder(mem core.MemoryManager, reg *registry.Registry, maxTokens int) *Builder {
	return &Builder{memory: mem, registry: reg, maxTokens: maxTokens}
}

func (b *Builder) Build(ctx context.Context, session *core.AgentSession) (*core.Prompt, error) {
	schemas := b.registry.Schemas(session.Config.AllowedTools)

	msgs, _ := b.memory.GetConversation(ctx, session.ID, 20)
	recentMsgs := make([]core.Message, len(msgs))
	for i, m := range msgs {
		recentMsgs[i] = *m
	}

	cbc := &pipeline.ContextBuildCtx{
		AgentContext: core.AgentContext{
			Goal:              session.Goal,
			Plan:              session.Plan,
			RecentMessages:    recentMsgs,
			RecentToolResults: session.AgentCtx.RecentToolResults,
			Scratchpad:        session.AgentCtx.Scratchpad,
		},
		Session: session,
	}

	p := pipeline.New(
		pipeline.LoadGoalMiddleware(),
		pipeline.LoadPlanMiddleware(),
		pipeline.RetrieveMemoryMiddleware(func(ctx context.Context, query, sessionID string, topK int) ([]*core.ToolMemory, error) {
			return b.memory.SearchToolMemory(ctx, query, sessionID, topK)
		}),
		pipeline.TokenBudgetMiddleware(b.maxTokens),
		pipeline.BuildPromptMiddleware(agentSystemPrompt, schemas),
	)

	if err := p.Run(ctx, cbc); err != nil {
		return nil, err
	}
	return cbc.BuiltPrompt, nil
}
