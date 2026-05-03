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
	mu               sync.RWMutex
	registry         *Registry
	global           core.SceneModels
	fallbackProvider string
}

func NewRouter(reg *Registry, global core.SceneModels, fallbackProvider string) *Router {
	return &Router{registry: reg, global: global, fallbackProvider: fallbackProvider}
}

func (r *Router) For(scene Scene, override *core.SceneModels) (core.LLMClient, error) {
	r.mu.RLock()
	global := r.global
	fb := r.fallbackProvider
	r.mu.RUnlock()

	modelID := modelForScene(scene, &global)
	if override != nil {
		if m := modelForScene(scene, override); m != "" {
			modelID = m
		}
	}
	if modelID == "" {
		return nil, fmt.Errorf("no model configured for scene %q", scene)
	}
	return r.registry.ClientOrFallback(modelID, fb)
}

func (r *Router) GlobalConfig() core.SceneModels {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.global
}

func (r *Router) KnownModels() []ModelDef {
	return r.registry.KnownModels()
}

func (r *Router) Providers() []ProviderConfig {
	return r.registry.Providers()
}

func (r *Router) RegisterProvider(cfg ProviderConfig) {
	r.registry.RegisterProvider(cfg)
}

func (r *Router) RegisterClientIfAbsent(modelID string, factory func() core.LLMClient) {
	r.mu.RLock()
	fb := r.fallbackProvider
	r.mu.RUnlock()
	if _, err := r.registry.Client(modelID); err == nil {
		return
	}

	if fb != "" {
		return
	}

	c := factory()
	r.registry.RegisterProvider(ProviderConfig{
		ID:      "synthetic-" + modelID,
		BaseURL: "",
		Models: []ModelDef{{
			ID:         modelID,
			ProviderID: "synthetic-" + modelID,
			Endpoints:  []Endpoint{EndpointOpenAIChat},
		}},
	})
	_ = c // factory result stored via cache in registry on next Client() call
}

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

func modelForScene(scene Scene, cfg *core.SceneModels) string {
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
