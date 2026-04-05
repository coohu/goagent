package registry

import (
	"fmt"
	"sync"

	"github.com/coohu/goagent/internal/core"
)

type Registry struct {
	mu    sync.RWMutex
	tools map[string]core.Tool
}

func New() *Registry {
	return &Registry{tools: make(map[string]core.Tool)}
}

func (r *Registry) Register(tool core.Tool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[tool.Name()] = tool
	return nil
}

func (r *Registry) Get(name string) (core.Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", core.Errorf(core.ErrToolNotFound, "tool not found", nil), name)
	}
	return t, nil
}

func (r *Registry) List() []core.Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]core.Tool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	return out
}

func (r *Registry) ListAllowed(names []string) []core.Tool {
	if len(names) == 0 {
		return r.List()
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	allowed := make(map[string]struct{}, len(names))
	for _, n := range names {
		allowed[n] = struct{}{}
	}
	var out []core.Tool
	for _, t := range r.tools {
		if _, ok := allowed[t.Name()]; ok {
			out = append(out, t)
		}
	}
	return out
}

func (r *Registry) Schemas(names []string) []core.ToolSchema {
	tools := r.ListAllowed(names)
	schemas := make([]core.ToolSchema, len(tools))
	for i, t := range tools {
		schemas[i] = t.Schema()
	}
	return schemas
}
