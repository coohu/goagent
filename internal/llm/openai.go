package llm

import (
	"context"
	"errors"
	"io"

	"github.com/coohu/goagent/internal/core"
	openai "github.com/sashabaranov/go-openai"
)

type OpenAIClient struct {
	client       *openai.Client
	defaultModel string
}

func NewOpenAIClient(apiKey, baseURL, defaultModel string) *OpenAIClient {
	cfg := openai.DefaultConfig(apiKey)
	if baseURL != "" {
		cfg.BaseURL = baseURL
	}
	return &OpenAIClient{
		client:       openai.NewClientWithConfig(cfg),
		defaultModel: defaultModel,
	}
}

func (c *OpenAIClient) ChatComplete(ctx context.Context, req *core.ChatRequest) (*core.ChatResponse, error) {
	model := req.Model
	if model == "" {
		model = c.defaultModel
	}

	msgs := make([]openai.ChatCompletionMessage, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = openai.ChatCompletionMessage{Role: m.Role, Content: m.Content}
	}

	resp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:     model,
		Messages:  msgs,
		MaxTokens: req.MaxTokens,
	})
	if err != nil {
		return nil, err
	}

	choice := resp.Choices[0]
	return &core.ChatResponse{
		Content:      choice.Message.Content,
		FinishReason: string(choice.FinishReason),
		TokensUsed:   resp.Usage.TotalTokens,
	}, nil
}

func (c *OpenAIClient) ChatStream(ctx context.Context, req *core.ChatRequest) (<-chan core.ChatChunk, error) {
	model := req.Model
	if model == "" {
		model = c.defaultModel
	}

	msgs := make([]openai.ChatCompletionMessage, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = openai.ChatCompletionMessage{Role: m.Role, Content: m.Content}
	}

	stream, err := c.client.CreateChatCompletionStream(ctx, openai.ChatCompletionRequest{
		Model:    model,
		Messages: msgs,
		Stream:   true,
	})
	if err != nil {
		return nil, err
	}

	ch := make(chan core.ChatChunk, 64)
	go func() {
		defer close(ch)
		defer stream.Close()
		for {
			resp, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				ch <- core.ChatChunk{Done: true}
				return
			}
			if err != nil {
				ch <- core.ChatChunk{Err: err, Done: true}
				return
			}
			if len(resp.Choices) > 0 {
				ch <- core.ChatChunk{Delta: resp.Choices[0].Delta.Content}
			}
		}
	}()
	return ch, nil
}

func (c *OpenAIClient) ChatWithTools(ctx context.Context, req *core.ChatRequest, tools []core.ToolSchema) (*core.ChatResponse, error) {
	model := req.Model
	if model == "" {
		model = c.defaultModel
	}

	msgs := make([]openai.ChatCompletionMessage, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = openai.ChatCompletionMessage{Role: m.Role, Content: m.Content}
	}

	oaiTools := make([]openai.Tool, len(tools))
	for i, t := range tools {
		oaiTools[i] = openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		}
	}

	resp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:    model,
		Messages: msgs,
		Tools:    oaiTools,
	})
	if err != nil {
		return nil, err
	}

	choice := resp.Choices[0]
	cr := &core.ChatResponse{
		Content:      choice.Message.Content,
		FinishReason: string(choice.FinishReason),
		TokensUsed:   resp.Usage.TotalTokens,
	}

	for _, tc := range choice.Message.ToolCalls {
		var input map[string]any
		_ = unmarshalJSON([]byte(tc.Function.Arguments), &input)
		cr.ToolCalls = append(cr.ToolCalls, core.ToolCallResponse{
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: input,
		})
	}

	return cr, nil
}

func (c *OpenAIClient) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	resp, err := c.client.CreateEmbeddings(ctx, openai.EmbeddingRequest{
		Input: texts,
		Model: openai.AdaEmbeddingV2,
	})
	if err != nil {
		return nil, err
	}

	result := make([][]float32, len(resp.Data))
	for i, d := range resp.Data {
		result[i] = d.Embedding
	}
	return result, nil
}
