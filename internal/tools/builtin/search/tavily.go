package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	stdhttp "net/http"
	"strings"
	"time"

	"github.com/yourorg/goagent/internal/core"
)

type TavilyTool struct {
	apiKey     string
	maxResults int
	client     *stdhttp.Client
}

func NewTavilyTool(apiKey string, maxResults int) *TavilyTool {
	if maxResults == 0 {
		maxResults = 5
	}
	return &TavilyTool{
		apiKey:     apiKey,
		maxResults: maxResults,
		client:     &stdhttp.Client{Timeout: 30 * time.Second},
	}
}

func (t *TavilyTool) Name() string        { return "search.web" }
func (t *TavilyTool) Description() string { return "Search the web and return relevant results" }

func (t *TavilyTool) Schema() core.ToolSchema {
	return core.ToolSchema{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "Search query"},
			},
			"required": []string{"query"},
		},
	}
}

func (t *TavilyTool) Validate(input map[string]any) error {
	if _, ok := input["query"].(string); !ok {
		return fmt.Errorf("query is required")
	}
	return nil
}

type tavilyRequest struct {
	APIKey         string `json:"api_key"`
	Query          string `json:"query"`
	MaxResults     int    `json:"max_results"`
	IncludeAnswer  bool   `json:"include_answer"`
}

type tavilyResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Content string `json:"content"`
	Score   float64 `json:"score"`
}

type tavilyResponse struct {
	Answer  string         `json:"answer"`
	Results []tavilyResult `json:"results"`
}

func (t *TavilyTool) Execute(ctx context.Context, input map[string]any) (*core.ToolResult, error) {
	query, _ := input["query"].(string)

	reqBody, _ := json.Marshal(tavilyRequest{
		APIKey:        t.apiKey,
		Query:         query,
		MaxResults:    t.maxResults,
		IncludeAnswer: true,
	})

	start := time.Now()
	httpReq, err := stdhttp.NewRequestWithContext(ctx, "POST", "https://api.tavily.com/search", bytes.NewReader(reqBody))
	if err != nil {
		return &core.ToolResult{Success: false, Stderr: err.Error(), RawOutput: err.Error()}, nil
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(httpReq)
	if err != nil {
		return &core.ToolResult{Success: false, Stderr: err.Error(), RawOutput: err.Error()}, nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var tr tavilyResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		raw := string(body)
		return &core.ToolResult{Success: true, Stdout: raw, RawOutput: raw, Duration: time.Since(start)}, nil
	}

	var sb strings.Builder
	if tr.Answer != "" {
		sb.WriteString("Answer: " + tr.Answer + "\n\n")
	}
	for i, r := range tr.Results {
		sb.WriteString(fmt.Sprintf("[%d] %s\n%s\n%s\n\n", i+1, r.Title, r.URL, r.Content))
	}

	out := sb.String()
	return &core.ToolResult{
		Success:   true,
		Stdout:    out,
		RawOutput: out,
		Duration:  time.Since(start),
	}, nil
}
