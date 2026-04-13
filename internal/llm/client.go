package llm

import (
	"context"
	"fmt"

	"github.com/coohu/goagent/internal/core"
	"github.com/coohu/goagent/internal/llm"
	"github.com/coohu/goagent/internal/adapter"
)

type Client struct {
	model   llm.ModelDef
	adapter adapter.Adapter
}

var _ core.LLMClient = (*Client)(nil)
func New(model llm.ModelDef, adapters []adapter.Adapter) (*Client, error) {
	endpoints := make([]llm.Endpoint, len(adapters))
	adapterMap := make(map[llm.Endpoint]adapter.Adapter, len(adapters))
	for i, a := range adapters {
		ep := llm.Endpoint(a.Endpoint())
		endpoints[i] = ep
		adapterMap[ep] = a
	}

	preferred := model.PreferredEndpoint(endpoints)
	if preferred == "" {
		// Fall back to first available adapter.
		if len(adapters) == 0 {
			return nil, fmt.Errorf("no adapters available for model %q", model.ID)
		}
		preferred = llm.Endpoint(adapters[0].Endpoint())
	}

	return &Client{model: model, adapter: adapterMap[preferred]}, nil
}

func (c *Client) ChatComplete(ctx context.Context, req *core.ChatRequest) (*core.ChatResponse, error) {
	aReq := coreToAdapterReq(req, c.model.ID)
	resp, err := c.adapter.Complete(ctx, aReq)
	if err != nil {
		return nil, err
	}
	return adapterToCoreResp(resp), nil
}

func (c *Client) ChatStream(ctx context.Context, req *core.ChatRequest) (<-chan core.ChatChunk, error) {
	aReq := coreToAdapterReq(req, c.model.ID)
	aCh, err := c.adapter.Stream(ctx, aReq)
	if err != nil {
		return nil, err
	}
	ch := make(chan core.ChatChunk, 64)
	go func() {
		defer close(ch)
		for chunk := range aCh {
			ch <- core.ChatChunk{Delta: chunk.Delta, Done: chunk.Done, Err: chunk.Err}
		}
	}()
	return ch, nil
}

func (c *Client) ChatWithTools(ctx context.Context, req *core.ChatRequest, tools []core.ToolSchema) (*core.ChatResponse, error) {
	aReq := coreToAdapterReq(req, c.model.ID)
	resp, err := c.adapter.CompleteWithTools(ctx, aReq, tools)
	if err != nil {
		return nil, err
	}
	return adapterToCoreResp(resp), nil
}

func (c *Client) Embed(_ context.Context, _ []string) ([][]float32, error) {
	return nil, fmt.Errorf("model %q does not support embeddings via this adapter", c.model.ID)
}

func (c *Client) ModelDef() llm.ModelDef { return c.model }

func coreToAdapterReq(req *core.ChatRequest, modelID string) *adapter.Request {
	model := req.Model
	if model == "" {
		model = modelID
	}
	// Extract system prompt from messages if present.
	var system string
	msgs := req.Messages
	if len(msgs) > 0 && msgs[0].Role == "system" {
		system = msgs[0].Content
		msgs = msgs[1:]
	}
	return &adapter.Request{
		Model:     model,
		Messages:  msgs,
		MaxTokens: req.MaxTokens,
		System:    system,
	}
}

func adapterToCoreResp(resp *adapter.Response) *core.ChatResponse {
	return &core.ChatResponse{
		Content:      resp.Content,
		ToolCalls:    resp.ToolCalls,
		TokensUsed:   resp.TotalTokens(),
		FinishReason: resp.FinishReason,
	}
}
