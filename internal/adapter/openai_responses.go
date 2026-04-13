package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/coohu/goagent/internal/core"
)

type OpenAIResponsesAdapter struct {
	baseURL        string
	apiKey         string
	defaultHeaders map[string]string
	http           HTTPDoer
}

func NewOpenAIResponses(baseURL, apiKey string, defaultHeaders map[string]string) *OpenAIResponsesAdapter {
	return &OpenAIResponsesAdapter{
		baseURL:        strings.TrimRight(baseURL, "/"),
		apiKey:         apiKey,
		defaultHeaders: defaultHeaders,
		http:           &http.Client{},
	}
}

func (a *OpenAIResponsesAdapter) Endpoint() string { return "/v1/responses" }

type respInput struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type respToolDef struct {
	Type        string `json:"type"` // "function"
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

type respRequest struct {
	Model           string        `json:"model"`
	Input           []respInput   `json:"input"`
	Tools           []respToolDef `json:"tools,omitempty"`
	MaxOutputTokens int           `json:"max_output_tokens,omitempty"`
}

type respOutputItem struct {
	Type    string `json:"type"` // "message", "function_call"
	Role    string `json:"role,omitempty"`
	Content []struct {
		Type string `json:"type"` // "output_text", "refusal"
		Text string `json:"text,omitempty"`
	} `json:"content,omitempty"`
	// Function call fields
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type respResponse struct {
	Output []respOutputItem `json:"output"`
	Usage  struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
	Status string `json:"status"`
}

func (a *OpenAIResponsesAdapter) Complete(ctx context.Context, req *Request) (*Response, error) {
	return a.call(ctx, req, nil)
}

func (a *OpenAIResponsesAdapter) CompleteWithTools(ctx context.Context, req *Request, tools []core.ToolSchema) (*Response, error) {
	return a.call(ctx, req, tools)
}

func (a *OpenAIResponsesAdapter) Stream(ctx context.Context, req *Request) (<-chan Chunk, error) {
	// /v1/responses streaming uses SSE with event types.
	// Fall back to single-shot for now and emit as a single chunk.
	resp, err := a.Complete(ctx, req)
	if err != nil {
		return nil, err
	}
	ch := make(chan Chunk, 2)
	ch <- Chunk{Delta: resp.Content}
	ch <- Chunk{Done: true}
	close(ch)
	return ch, nil
}

func (a *OpenAIResponsesAdapter) call(ctx context.Context, req *Request, tools []core.ToolSchema) (*Response, error) {
	inputs := buildInputs(req)
	toolDefs := schemasToRespTools(tools)

	body, err := json.Marshal(respRequest{
		Model:           req.Model,
		Input:           inputs,
		Tools:           toolDefs,
		MaxOutputTokens: req.MaxTokens,
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
	httpReq.Header.Set("Authorization", "Bearer "+a.apiKey)
	for k, v := range a.defaultHeaders {
		httpReq.Header.Set(k, v)
	}

	httpResp, err := a.http.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	raw, _ := io.ReadAll(httpResp.Body)
	if httpResp.StatusCode >= 400 {
		return nil, fmt.Errorf("openai responses api error %d: %s", httpResp.StatusCode, string(raw))
	}

	var rr respResponse
	if err := json.Unmarshal(raw, &rr); err != nil {
		return nil, err
	}
	if rr.Error != nil {
		return nil, fmt.Errorf("openai responses error [%s]: %s", rr.Error.Code, rr.Error.Message)
	}

	return parseRespResponse(&rr), nil
}

func parseRespResponse(rr *respResponse) *Response {
	resp := &Response{
		TokensIn:  rr.Usage.InputTokens,
		TokensOut: rr.Usage.OutputTokens,
	}
	for _, item := range rr.Output {
		switch item.Type {
		case "message":
			for _, c := range item.Content {
				if c.Type == "output_text" {
					resp.Content += c.Text
				}
			}
		case "function_call":
			var input map[string]any
			_ = json.Unmarshal([]byte(item.Arguments), &input)
			resp.ToolCalls = append(resp.ToolCalls, core.ToolCallResponse{
				ID:    item.CallID,
				Name:  item.Name,
				Input: input,
			})
		}
	}
	return resp
}

func buildInputs(req *Request) []respInput {
	inputs := make([]respInput, 0, len(req.Messages)+1)
	if req.System != "" {
		inputs = append(inputs, respInput{Role: "system", Content: req.System})
	}
	for _, m := range req.Messages {
		inputs = append(inputs, respInput{Role: m.Role, Content: m.Content})
	}
	return inputs
}

func schemasToRespTools(schemas []core.ToolSchema) []respToolDef {
	if len(schemas) == 0 {
		return nil
	}
	tools := make([]respToolDef, len(schemas))
	for i, s := range schemas {
		tools[i] = respToolDef{
			Type:        "function",
			Name:        s.Name,
			Description: s.Description,
			Parameters:  s.Parameters,
		}
	}
	return tools
}
