package http

import (
	"context"
	"fmt"
	"io"
	stdhttp "net/http"
	"strings"
	"time"

	"github.com/coohu/goagent/internal/core"
)

type RequestTool struct {
	client *stdhttp.Client
}

func NewRequestTool() *RequestTool {
	return &RequestTool{
		client: &stdhttp.Client{Timeout: 30 * time.Second},
	}
}

func (t *RequestTool) Name() string { return "http.request" }
func (t *RequestTool) Description() string {
	return "Make an HTTP request and return the response body"
}

func (t *RequestTool) Schema() core.ToolSchema {
	return core.ToolSchema{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"method":  map[string]any{"type": "string", "enum": []string{"GET", "POST", "PUT", "DELETE", "PATCH"}, "description": "HTTP method"},
				"url":     map[string]any{"type": "string", "description": "Target URL"},
				"body":    map[string]any{"type": "string", "description": "Request body (optional)"},
				"headers": map[string]any{"type": "object", "description": "Request headers (optional)"},
			},
			"required": []string{"method", "url"},
		},
	}
}

func (t *RequestTool) Validate(input map[string]any) error {
	if _, ok := input["url"].(string); !ok {
		return fmt.Errorf("url is required")
	}
	return nil
}

func (t *RequestTool) Execute(ctx context.Context, input map[string]any) (*core.ToolResult, error) {
	method, _ := input["method"].(string)
	if method == "" {
		method = "GET"
	}
	url, _ := input["url"].(string)
	bodyStr, _ := input["body"].(string)

	start := time.Now()
	req, err := stdhttp.NewRequestWithContext(ctx, method, url, strings.NewReader(bodyStr))
	if err != nil {
		return &core.ToolResult{Success: false, Stderr: err.Error(), RawOutput: err.Error()}, nil
	}

	if headers, ok := input["headers"].(map[string]any); ok {
		for k, v := range headers {
			if vs, ok := v.(string); ok {
				req.Header.Set(k, vs)
			}
		}
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return &core.ToolResult{Success: false, Stderr: err.Error(), RawOutput: err.Error()}, nil
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return &core.ToolResult{Success: false, Stderr: err.Error(), RawOutput: err.Error()}, nil
	}

	out := fmt.Sprintf("Status: %s\n\n%s", resp.Status, string(respBody))
	success := resp.StatusCode >= 200 && resp.StatusCode < 300

	return &core.ToolResult{
		Success:   success,
		Stdout:    out,
		RawOutput: out,
		ExitCode:  resp.StatusCode,
		Duration:  time.Since(start),
	}, nil
}
