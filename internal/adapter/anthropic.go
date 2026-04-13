package adapter

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/coohu/goagent/internal/core"
)

type AnthropicAdapter struct {
	baseURL        string
	apiKey         string
	apiVersion     string
	defaultHeaders map[string]string
	http           HTTPDoer
}

func NewAnthropic(baseURL, apiKey string, defaultHeaders map[string]string) *AnthropicAdapter {
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	headers := map[string]string{
		"anthropic-version": "2023-06-01",
	}
	for k, v := range defaultHeaders {
		headers[k] = v
	}
	return &AnthropicAdapter{
		baseURL:        strings.TrimRight(baseURL, "/"),
		apiKey:         apiKey,
		defaultHeaders: headers,
		http:           &http.Client{},
	}
}

func (a *AnthropicAdapter) Endpoint() string { return "/v1/messages" }

type anthropicContent struct {
	Type  string `json:"type"` // "text", "tool_use", "tool_result"
	Text  string `json:"text,omitempty"`
	ID    string `json:"id,omitempty"`
	Name  string `json:"name,omitempty"`
	Input any    `json:"input,omitempty"`
}

type anthropicMessage struct {
	Role    string             `json:"role"`
	Content []anthropicContent `json:"content"`
}

type anthropicTool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema any    `json:"input_schema"`
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	Messages  []anthropicMessage `json:"messages"`
	System    string             `json:"system,omitempty"`
	Tools     []anthropicTool    `json:"tools,omitempty"`
	MaxTokens int                `json:"max_tokens"`
	Stream    bool               `json:"stream,omitempty"`
}

type anthropicResponse struct {
	ID      string             `json:"id"`
	Type    string             `json:"type"`
	Role    string             `json:"role"`
	Content []anthropicContent `json:"content"`
	Usage   struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	StopReason string `json:"stop_reason"`
	Error      *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type anthropicStreamEvent struct {
	Type  string `json:"type"`
	Delta *struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"delta,omitempty"`
	ContentBlock *anthropicContent `json:"content_block,omitempty"`
}

func (a *AnthropicAdapter) Complete(ctx context.Context, req *Request) (*Response, error) {
	return a.doRequest(ctx, req, nil, false)
}

func (a *AnthropicAdapter) CompleteWithTools(ctx context.Context, req *Request, tools []core.ToolSchema) (*Response, error) {
	return a.doRequest(ctx, req, tools, false)
}

func (a *AnthropicAdapter) Stream(ctx context.Context, req *Request) (<-chan Chunk, error) {
	httpReq, err := a.buildHTTPRequest(ctx, req, nil, true)
	if err != nil {
		return nil, err
	}

	resp, err := a.http.Do(httpReq)
	if err != nil {
		return nil, err
	}

	ch := make(chan Chunk, 64)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			var ev anthropicStreamEvent
			if err := json.Unmarshal([]byte(data), &ev); err != nil {
				continue
			}
			switch ev.Type {
			case "content_block_delta":
				if ev.Delta != nil && ev.Delta.Type == "text_delta" {
					ch <- Chunk{Delta: ev.Delta.Text}
				}
			case "message_stop":
				ch <- Chunk{Done: true}
				return
			}
		}
	}()
	return ch, nil
}

func (a *AnthropicAdapter) doRequest(ctx context.Context, req *Request, tools []core.ToolSchema, stream bool) (*Response, error) {
	httpReq, err := a.buildHTTPRequest(ctx, req, tools, stream)
	if err != nil {
		return nil, err
	}

	resp, err := a.http.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("anthropic api error %d: %s", resp.StatusCode, string(raw))
	}

	var ar anthropicResponse
	if err := json.Unmarshal(raw, &ar); err != nil {
		return nil, err
	}
	if ar.Error != nil {
		return nil, fmt.Errorf("anthropic error [%s]: %s", ar.Error.Type, ar.Error.Message)
	}

	return a.parseResponse(&ar), nil
}

func (a *AnthropicAdapter) buildHTTPRequest(ctx context.Context, req *Request, tools []core.ToolSchema, stream bool) (*http.Request, error) {
	msgs := coreToAnthropic(req.Messages)
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}

	body, err := json.Marshal(anthropicRequest{
		Model:     req.Model,
		Messages:  msgs,
		System:    req.System,
		Tools:     schemasToAnthropicTools(tools),
		MaxTokens: maxTokens,
		Stream:    stream,
	})
	if err != nil {
		return nil, err
	}

	url := a.baseURL + a.Endpoint()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", a.apiKey)
	for k, v := range a.defaultHeaders {
		httpReq.Header.Set(k, v)
	}
	return httpReq, nil
}

func (a *AnthropicAdapter) parseResponse(ar *anthropicResponse) *Response {
	resp := &Response{
		FinishReason: ar.StopReason,
		TokensIn:     ar.Usage.InputTokens,
		TokensOut:    ar.Usage.OutputTokens,
	}
	for _, c := range ar.Content {
		switch c.Type {
		case "text":
			resp.Content += c.Text
		case "tool_use":
			var input map[string]any
			if m, ok := c.Input.(map[string]any); ok {
				input = m
			}
			resp.ToolCalls = append(resp.ToolCalls, core.ToolCallResponse{
				ID:    c.ID,
				Name:  c.Name,
				Input: input,
			})
		}
	}
	return resp
}

func coreToAnthropic(msgs []core.Message) []anthropicMessage {
	out := make([]anthropicMessage, 0, len(msgs))
	for _, m := range msgs {
		// Skip system messages — Anthropic takes system as a top-level field.
		if m.Role == "system" {
			continue
		}
		out = append(out, anthropicMessage{
			Role:    m.Role,
			Content: []anthropicContent{{Type: "text", Text: m.Content}},
		})
	}
	return out
}

func schemasToAnthropicTools(schemas []core.ToolSchema) []anthropicTool {
	if len(schemas) == 0 {
		return nil
	}
	tools := make([]anthropicTool, len(schemas))
	for i, s := range schemas {
		tools[i] = anthropicTool{
			Name:        s.Name,
			Description: s.Description,
			InputSchema: s.Parameters,
		}
	}
	return tools
}
