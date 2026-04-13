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

type OpenAIChatAdapter struct {
	baseURL        string
	apiKey         string
	defaultHeaders map[string]string
	http           HTTPDoer
}

func NewOpenAIChat(baseURL, apiKey string, defaultHeaders map[string]string) *OpenAIChatAdapter {
	return &OpenAIChatAdapter{
		baseURL:        strings.TrimRight(baseURL, "/"),
		apiKey:         apiKey,
		defaultHeaders: defaultHeaders,
		http:           &http.Client{},
	}
}

func (a *OpenAIChatAdapter) Endpoint() string { return "/v1/chat/completions" }

type oaiMessage struct {
	Role       string        `json:"role"`
	Content    any           `json:"content"` // string or []ContentPart
	ToolCalls  []oaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
}

type oaiToolCall struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Function oaiToolFunction `json:"function"`
}

type oaiToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type oaiTool struct {
	Type     string     `json:"type"`
	Function oaiToolDef `json:"function"`
}

type oaiToolDef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

type oaiRequest struct {
	Model     string       `json:"model"`
	Messages  []oaiMessage `json:"messages"`
	Tools     []oaiTool    `json:"tools,omitempty"`
	Stream    bool         `json:"stream,omitempty"`
	MaxTokens int          `json:"max_tokens,omitempty"`
}

type oaiChoice struct {
	Message      oaiMessage `json:"message"`
	Delta        oaiMessage `json:"delta"`
	FinishReason string     `json:"finish_reason"`
}

type oaiResponse struct {
	Choices []oaiChoice `json:"choices"`
	Usage   struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error *oaiError `json:"error,omitempty"`
}

type oaiError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

func (a *OpenAIChatAdapter) Complete(ctx context.Context, req *Request) (*Response, error) {
	body, err := a.buildBody(req, nil, false)
	if err != nil {
		return nil, err
	}
	var oai oaiResponse
	if err := a.call(ctx, body, &oai); err != nil {
		return nil, err
	}
	return a.parseResponse(&oai), nil
}

func (a *OpenAIChatAdapter) CompleteWithTools(ctx context.Context, req *Request, tools []core.ToolSchema) (*Response, error) {
	body, err := a.buildBody(req, tools, false)
	if err != nil {
		return nil, err
	}
	var oai oaiResponse
	if err := a.call(ctx, body, &oai); err != nil {
		return nil, err
	}
	return a.parseResponse(&oai), nil
}

func (a *OpenAIChatAdapter) Stream(ctx context.Context, req *Request) (<-chan Chunk, error) {
	body, err := a.buildBody(req, nil, true)
	if err != nil {
		return nil, err
	}

	httpReq, err := a.newRequest(ctx, body)
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
			if data == "[DONE]" {
				ch <- Chunk{Done: true}
				return
			}
			var oai oaiResponse
			if err := json.Unmarshal([]byte(data), &oai); err != nil {
				continue
			}
			if len(oai.Choices) > 0 {
				ch <- Chunk{Delta: oai.Choices[0].Delta.contentString()}
			}
		}
	}()
	return ch, nil
}

func (a *OpenAIChatAdapter) buildBody(req *Request, tools []core.ToolSchema, stream bool) ([]byte, error) {
	msgs := coreMessagesToOAI(req.Messages, req.System)
	oaiTools := schemasToOAITools(tools)
	return json.Marshal(oaiRequest{
		Model:     req.Model,
		Messages:  msgs,
		Tools:     oaiTools,
		Stream:    stream,
		MaxTokens: req.MaxTokens,
	})
}

func (a *OpenAIChatAdapter) call(ctx context.Context, body []byte, out any) error {
	httpReq, err := a.newRequest(ctx, body)
	if err != nil {
		return err
	}
	resp, err := a.http.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		var oaiErr oaiResponse
		_ = json.Unmarshal(raw, &oaiErr)
		if oaiErr.Error != nil {
			return fmt.Errorf("openai api error %d: %s (%s)", resp.StatusCode, oaiErr.Error.Message, oaiErr.Error.Type)
		}
		return fmt.Errorf("openai api error %d: %s", resp.StatusCode, string(raw))
	}
	return json.Unmarshal(raw, out)
}

func (a *OpenAIChatAdapter) newRequest(ctx context.Context, body []byte) (*http.Request, error) {
	url := a.baseURL + a.Endpoint()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	for k, v := range a.defaultHeaders {
		req.Header.Set(k, v)
	}
	return req, nil
}

func (a *OpenAIChatAdapter) parseResponse(oai *oaiResponse) *Response {
	if len(oai.Choices) == 0 {
		return &Response{}
	}
	choice := oai.Choices[0]
	resp := &Response{
		Content:      choice.Message.contentString(),
		FinishReason: choice.FinishReason,
		TokensIn:     oai.Usage.PromptTokens,
		TokensOut:    oai.Usage.CompletionTokens,
	}
	for _, tc := range choice.Message.ToolCalls {
		var input map[string]any
		_ = json.Unmarshal([]byte(tc.Function.Arguments), &input)
		resp.ToolCalls = append(resp.ToolCalls, core.ToolCallResponse{
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: input,
		})
	}
	return resp
}

func (m *oaiMessage) contentString() string {
	if s, ok := m.Content.(string); ok {
		return s
	}
	return ""
}

func coreMessagesToOAI(msgs []core.Message, system string) []oaiMessage {
	out := make([]oaiMessage, 0, len(msgs)+1)
	if system != "" {
		out = append(out, oaiMessage{Role: "system", Content: system})
	}
	for _, m := range msgs {
		out = append(out, oaiMessage{Role: m.Role, Content: m.Content})
	}
	return out
}

func schemasToOAITools(schemas []core.ToolSchema) []oaiTool {
	if len(schemas) == 0 {
		return nil
	}
	tools := make([]oaiTool, len(schemas))
	for i, s := range schemas {
		tools[i] = oaiTool{
			Type: "function",
			Function: oaiToolDef{
				Name:        s.Name,
				Description: s.Description,
				Parameters:  s.Parameters,
			},
		}
	}
	return tools
}
