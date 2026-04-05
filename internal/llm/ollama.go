package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/yourorg/goagent/internal/core"
)

type OllamaClient struct {
	baseURL string
	model   string
	http    *http.Client
}

func NewOllamaClient(baseURL, model string) *OllamaClient {
	return &OllamaClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		http:    &http.Client{},
	}
}

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

type ollamaChatResponse struct {
	Message ollamaMessage `json:"message"`
	Done    bool          `json:"done"`
}

func (c *OllamaClient) ChatComplete(ctx context.Context, req *core.ChatRequest) (*core.ChatResponse, error) {
	msgs := make([]ollamaMessage, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = ollamaMessage{Role: m.Role, Content: m.Content}
	}

	body, _ := json.Marshal(ollamaChatRequest{
		Model:    c.model,
		Messages: msgs,
		Stream:   false,
	})

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result ollamaChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &core.ChatResponse{
		Content:      result.Message.Content,
		FinishReason: "stop",
	}, nil
}

func (c *OllamaClient) ChatStream(ctx context.Context, req *core.ChatRequest) (<-chan core.ChatChunk, error) {
	msgs := make([]ollamaMessage, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = ollamaMessage{Role: m.Role, Content: m.Content}
	}

	body, _ := json.Marshal(ollamaChatRequest{
		Model:    c.model,
		Messages: msgs,
		Stream:   true,
	})

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, err
	}

	ch := make(chan core.ChatChunk, 64)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			var chunk ollamaChatResponse
			if err := json.Unmarshal(scanner.Bytes(), &chunk); err != nil {
				continue
			}
			ch <- core.ChatChunk{Delta: chunk.Message.Content, Done: chunk.Done}
			if chunk.Done {
				return
			}
		}
	}()
	return ch, nil
}

func (c *OllamaClient) ChatWithTools(ctx context.Context, req *core.ChatRequest, tools []core.ToolSchema) (*core.ChatResponse, error) {
	return c.ChatComplete(ctx, req)
}

func (c *OllamaClient) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	type embedReq struct {
		Model  string `json:"model"`
		Prompt string `json:"prompt"`
	}
	type embedResp struct {
		Embedding []float32 `json:"embedding"`
	}

	result := make([][]float32, len(texts))
	for i, text := range texts {
		body, _ := json.Marshal(embedReq{Model: c.model, Prompt: text})
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/embeddings", bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := c.http.Do(httpReq)
		if err != nil {
			return nil, err
		}
		var er embedResp
		if err := json.NewDecoder(resp.Body).Decode(&er); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("embed decode: %w", err)
		}
		resp.Body.Close()
		result[i] = er.Embedding
	}
	return result, nil
}
