package llm

import (
	"fmt"
	"sync"

	"github.com/coohu/goagent/internal/core"
	"github.com/coohu/goagent/internal/llm"
	"github.com/coohu/goagent/internal/adapter"
)

type Registry struct {
	mu        sync.RWMutex
	providers map[string]llm.ProviderConfig
	cache     map[string]core.LLMClient // modelID → client
}

func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]llm.ProviderConfig),
		cache:     make(map[string]core.LLMClient),
	}
}

func (r *Registry) RegisterProvider(cfg llm.ProviderConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[cfg.ID] = cfg
	// Invalidate cached clients for this provider.
	for _, m := range cfg.Models {
		delete(r.cache, m.ID)
	}
}

// Client returns (or builds) a core.LLMClient for the given model ID.
// It searches all registered providers for the model definition.
func (r *Registry) Client(modelID string) (core.LLMClient, error) {
	r.mu.RLock()
	if c, ok := r.cache[modelID]; ok {
		r.mu.RUnlock()
		return c, nil
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()

	// Double-check after acquiring write lock.
	if c, ok := r.cache[modelID]; ok {
		return c, nil
	}

	model, provCfg, err := r.findModel(modelID)
	if err != nil {
		return nil, err
	}

	adapters := buildAdapters(provCfg)
	c, err := New(model, adapters)
	if err != nil {
		return nil, err
	}

	r.cache[modelID] = c
	return c, nil
}

func (r *Registry) ClientOrFallback(modelID, fallbackProviderID string) (core.LLMClient, error) {
	c, err := r.Client(modelID)
	if err == nil {
		return c, nil
	}

	r.mu.RLock()
	provCfg, ok := r.providers[fallbackProviderID]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("model %q not found and fallback provider %q not registered", modelID, fallbackProviderID)
	}

	model := llm.ModelDef{
		ID:           modelID,
		ProviderID:   fallbackProviderID,
		Endpoints:    []llm.Endpoint{llm.EndpointOpenAIChat},
		Capabilities: []llm.Capability{llm.CapabilityTools, llm.CapabilityStreaming},
	}

	adapters := buildAdapters(provCfg)
	client, err := New(model, adapters)
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	r.cache[modelID] = client
	r.mu.Unlock()

	return client, nil
}

func (r *Registry) KnownModels() []llm.ModelDef {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var models []llm.ModelDef
	for _, p := range r.providers {
		models = append(models, p.Models...)
	}
	return models
}

func (r *Registry) Providers() []llm.ProviderConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]llm.ProviderConfig, 0, len(r.providers))
	for _, p := range r.providers {
		out = append(out, p)
	}
	return out
}

func (r *Registry) findModel(modelID string) (llm.ModelDef, llm.ProviderConfig, error) {
	for _, p := range r.providers {
		m, err := p.FindModel(modelID)
		if err == nil {
			return m, p, nil
		}
	}
	return llm.ModelDef{}, llm.ProviderConfig{}, fmt.Errorf("model %q not found in any registered provider", modelID)
}

func buildAdapters(cfg llm.ProviderConfig) []adapter.Adapter {
	var adapters []adapter.Adapter
	switch cfg.ID {
	case "anthropic":
		adapters = append(adapters, adapter.NewAnthropic(cfg.BaseURL, cfg.APIKey, cfg.DefaultHeaders))
	case "ollama":
		adapters = append(adapters, adapter.NewOllamaChat(cfg.BaseURL))
	default:
		// OpenAI-compatible: support both Chat and Responses endpoints.
		adapters = append(adapters,
			adapter.NewOpenAIChat(cfg.BaseURL, cfg.APIKey, cfg.DefaultHeaders),
			adapter.NewOpenAIResponses(cfg.BaseURL, cfg.APIKey, cfg.DefaultHeaders),
		)
	}
	return adapters
}
