package llm

import (
	"context"
	"fmt"

	"github.com/coohu/goagent/internal/core"
)

type Scene string

const (
	ScenePlanning  Scene = "planning"
	SceneExecute   Scene = "execute"
	SceneSummarize Scene = "summarize"
	SceneReflect   Scene = "reflect"
)

var _ core.LLMClient = (*Router)(nil)

type Router struct {
	clients map[string]core.LLMClient
	scenes  map[Scene]string
}

func NewRouter(clients map[string]core.LLMClient, scenes map[Scene]string) *Router {
	return &Router{clients: clients, scenes: scenes}
}

func (r *Router) For(scene Scene) (core.LLMClient, error) {
	model, ok := r.scenes[scene]
	if !ok {
		model = r.scenes[SceneExecute]
	}
	c, ok := r.clients[model]
	if !ok {
		return nil, fmt.Errorf("no client for model %q", model)
	}
	return c, nil
}

func (r *Router) ChatComplete(ctx context.Context, req *core.ChatRequest) (*core.ChatResponse, error) {
	c, err := r.For(SceneExecute)
	if err != nil {
		return nil, err
	}
	return c.ChatComplete(ctx, req)
}

func (r *Router) ChatStream(ctx context.Context, req *core.ChatRequest) (<-chan core.ChatChunk, error) {
	c, err := r.For(SceneExecute)
	if err != nil {
		return nil, err
	}
	return c.ChatStream(ctx, req)
}

func (r *Router) ChatWithTools(ctx context.Context, req *core.ChatRequest, tools []core.ToolSchema) (*core.ChatResponse, error) {
	c, err := r.For(SceneExecute)
	if err != nil {
		return nil, err
	}
	return c.ChatWithTools(ctx, req, tools)
}

func (r *Router) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	c, err := r.For(SceneSummarize)
	if err != nil {
		return nil, err
	}
	return c.Embed(ctx, texts)
}
