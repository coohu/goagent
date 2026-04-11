package llm

import (
	"context"
	"testing"

	"github.com/coohu/goagent/internal/core"
)

type stubClient struct{ id string }

func (s *stubClient) ChatComplete(_ context.Context, _ *core.ChatRequest) (*core.ChatResponse, error) {
	return &core.ChatResponse{Content: s.id}, nil
}
func (s *stubClient) ChatStream(_ context.Context, _ *core.ChatRequest) (<-chan core.ChatChunk, error) {
	ch := make(chan core.ChatChunk, 1)
	ch <- core.ChatChunk{Delta: s.id, Done: true}
	close(ch)
	return ch, nil
}
func (s *stubClient) ChatWithTools(_ context.Context, _ *core.ChatRequest, _ []core.ToolSchema) (*core.ChatResponse, error) {
	return &core.ChatResponse{Content: s.id}, nil
}
func (s *stubClient) Embed(_ context.Context, texts []string) ([][]float32, error) {
	return make([][]float32, len(texts)), nil
}

func newTestRouter() *Router {
	clients := map[string]core.LLMClient{
		"gpt-4o":      &stubClient{id: "gpt-4o"},
		"gpt-4o-mini": &stubClient{id: "gpt-4o-mini"},
	}
	global := core.SceneModels{
		Planning:  "gpt-4o",
		Execute:   "gpt-4o",
		Summarize: "gpt-4o-mini",
		Reflect:   "gpt-4o-mini",
	}
	return NewRouter(clients, global)
}

func TestRouterGlobalConfig(t *testing.T) {
	r := newTestRouter()

	c, err := r.For(ScenePlanning, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp, _ := c.ChatComplete(context.Background(), &core.ChatRequest{})
	if resp.Content != "gpt-4o" {
		t.Errorf("planning scene: expected gpt-4o, got %s", resp.Content)
	}

	c, err = r.For(SceneSummarize, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp, _ = c.ChatComplete(context.Background(), &core.ChatRequest{})
	if resp.Content != "gpt-4o-mini" {
		t.Errorf("summarize scene: expected gpt-4o-mini, got %s", resp.Content)
	}
}

func TestRouterSessionOverride(t *testing.T) {
	r := newTestRouter()

	// Override only the execute scene for this session
	override := &core.SceneModels{
		Planning:  "", // empty = use global
		Execute:   "gpt-4o-mini",
		Summarize: "",
		Reflect:   "",
	}

	c, err := r.For(SceneExecute, override)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp, _ := c.ChatComplete(context.Background(), &core.ChatRequest{})
	if resp.Content != "gpt-4o-mini" {
		t.Errorf("overridden execute: expected gpt-4o-mini, got %s", resp.Content)
	}

	// Planning should still use global gpt-4o
	c, err = r.For(ScenePlanning, override)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp, _ = c.ChatComplete(context.Background(), &core.ChatRequest{})
	if resp.Content != "gpt-4o" {
		t.Errorf("non-overridden planning: expected gpt-4o, got %s", resp.Content)
	}
}

func TestRouterAllScenesOverridden(t *testing.T) {
	r := newTestRouter()

	override := &core.SceneModels{
		Planning:  "gpt-4o-mini",
		Execute:   "gpt-4o-mini",
		Summarize: "gpt-4o-mini",
		Reflect:   "gpt-4o-mini",
	}

	for _, scene := range []Scene{ScenePlanning, SceneExecute, SceneSummarize, SceneReflect} {
		c, err := r.For(scene, override)
		if err != nil {
			t.Fatalf("scene %s: unexpected error: %v", scene, err)
		}
		resp, _ := c.ChatComplete(context.Background(), &core.ChatRequest{})
		if resp.Content != "gpt-4o-mini" {
			t.Errorf("scene %s: expected gpt-4o-mini, got %s", scene, resp.Content)
		}
	}
}

func TestRouterRegisterClientAtRuntime(t *testing.T) {
	r := newTestRouter()

	r.RegisterClient("claude-sonnet", &stubClient{id: "claude-sonnet"})

	override := &core.SceneModels{
		Planning: "claude-sonnet",
	}
	c, err := r.For(ScenePlanning, override)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp, _ := c.ChatComplete(context.Background(), &core.ChatRequest{})
	if resp.Content != "claude-sonnet" {
		t.Errorf("runtime registered client: expected claude-sonnet, got %s", resp.Content)
	}
}

func TestRouterNilOverrideUsesGlobal(t *testing.T) {
	r := newTestRouter()

	c, err := r.For(SceneReflect, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp, _ := c.ChatComplete(context.Background(), &core.ChatRequest{})
	if resp.Content != "gpt-4o-mini" {
		t.Errorf("nil override reflect: expected gpt-4o-mini, got %s", resp.Content)
	}
}

func TestRouterKnownModels(t *testing.T) {
	r := newTestRouter()
	known := r.KnownModels()
	if len(known) != 2 {
		t.Errorf("expected 2 known models, got %d", len(known))
	}
}

func TestRouterGlobalConfigReturned(t *testing.T) {
	r := newTestRouter()
	cfg := r.GlobalConfig()
	if cfg.Planning != "gpt-4o" {
		t.Errorf("wrong planning in global config: %s", cfg.Planning)
	}
	if cfg.Summarize != "gpt-4o-mini" {
		t.Errorf("wrong summarize in global config: %s", cfg.Summarize)
	}
}
