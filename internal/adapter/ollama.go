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

type OllamaChatAdapter struct {
	baseURL string
	http    HTTPDoer
}

func NewOllamaChat(baseURL string) *OllamaChatAdapter {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	return &OllamaChatAdapter{baseURL: strings.TrimRight(baseURL, "/"), http: &http.Client{}}
}

func (a *OllamaChatAdapter) Endpoint() string { return "/api/chat" }

type ollamaMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaReq struct {
	Model    string      `json:"model"`
	Messages []ollamaMsg `json:"messages"`
	Stream   bool        `json:"stream"`
}

type ollamaResp struct {
	Message ollamaMsg `json:"message"`
	Done    bool      `json:"done"`
}

func (a *OllamaChatAdapter) Complete(ctx context.Context, req *Request) (*Response, error) {
	body, _ := json.Marshal(ollamaReq{
		Model:    req.Model,
		Messages: coreToOllama(req.Messages, req.System),
		Stream:   false,
	})
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+a.Endpoint(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := a.http.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("ollama error %d: %s", resp.StatusCode, string(raw))
	}
	var or_ ollamaResp
	if err := json.Unmarshal(raw, &or_); err != nil {
		return nil, err
	}
	return &Response{Content: or_.Message.Content}, nil
}

func (a *OllamaChatAdapter) CompleteWithTools(ctx context.Context, req *Request, _ []core.ToolSchema) (*Response, error) {
	// Ollama does not support function calling natively; fall back to plain completion.
	return a.Complete(ctx, req)
}

func (a *OllamaChatAdapter) Stream(ctx context.Context, req *Request) (<-chan Chunk, error) {
	body, _ := json.Marshal(ollamaReq{
		Model:    req.Model,
		Messages: coreToOllama(req.Messages, req.System),
		Stream:   true,
	})
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+a.Endpoint(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
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
			var or_ ollamaResp
			if err := json.Unmarshal(scanner.Bytes(), &or_); err != nil {
				continue
			}
			ch <- Chunk{Delta: or_.Message.Content, Done: or_.Done}
			if or_.Done {
				return
			}
		}
	}()
	return ch, nil
}

func coreToOllama(msgs []core.Message, system string) []ollamaMsg {
	out := make([]ollamaMsg, 0, len(msgs)+1)
	if system != "" {
		out = append(out, ollamaMsg{Role: "system", Content: system})
	}
	for _, m := range msgs {
		out = append(out, ollamaMsg{Role: m.Role, Content: m.Content})
	}
	return out
}
