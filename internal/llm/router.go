package llm

import (
	"context"
	"fmt"
	"sync"

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
	mu      sync.RWMutex
	clients map[string]core.LLMClient
	global  core.SceneModels
}

func NewRouter(clients map[string]core.LLMClient, global core.SceneModels) *Router {
	return &Router{clients: clients, global: global}
}

// For resolves the LLM client for a scene.
// If override is non-nil, its non-empty fields take precedence over global config.
func (r *Router) For(scene Scene, override *core.SceneModels) (core.LLMClient, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	model := r.modelForScene(scene, &r.global)
	if override != nil {
		if m := r.modelForScene(scene, override); m != "" {
			model = m
		}
	}

	c, ok := r.clients[model]
	if !ok {
		for _, fallback := range r.clients {
			return fallback, nil
		}
		return nil, fmt.Errorf("no LLM client for model %q", model)
	}
	return c, nil
}

// RegisterClient adds or updates a client at runtime.
func (r *Router) RegisterClient(modelID string, client core.LLMClient) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clients[modelID] = client
}

// GlobalConfig returns the current server-level scene→model mapping.
func (r *Router) GlobalConfig() core.SceneModels {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.global
}

// KnownModels returns IDs of all registered clients.
func (r *Router) KnownModels() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.clients))
	for id := range r.clients {
		ids = append(ids, id)
	}
	return ids
}

func (r *Router) modelForScene(scene Scene, cfg *core.SceneModels) string {
	if cfg == nil {
		return ""
	}
	switch scene {
	case ScenePlanning:
		return cfg.Planning
	case SceneExecute:
		return cfg.Execute
	case SceneSummarize:
		return cfg.Summarize
	case SceneReflect:
		return cfg.Reflect
	}
	return cfg.Execute
}

// ── core.LLMClient passthrough (global config, no session override) ──

func (r *Router) ChatComplete(ctx context.Context, req *core.ChatRequest) (*core.ChatResponse, error) {
	c, err := r.For(SceneExecute, nil)
	if err != nil {
		return nil, err
	}
	return c.ChatComplete(ctx, req)
}

func (r *Router) ChatStream(ctx context.Context, req *core.ChatRequest) (<-chan core.ChatChunk, error) {
	c, err := r.For(SceneExecute, nil)
	if err != nil {
		return nil, err
	}
	return c.ChatStream(ctx, req)
}

func (r *Router) ChatWithTools(ctx context.Context, req *core.ChatRequest, tools []core.ToolSchema) (*core.ChatResponse, error) {
	c, err := r.For(SceneExecute, nil)
	if err != nil {
		return nil, err
	}
	return c.ChatWithTools(ctx, req, tools)
}

func (r *Router) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	c, err := r.For(SceneSummarize, nil)
	if err != nil {
		return nil, err
	}
	return c.Embed(ctx, texts)
}
