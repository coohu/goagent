package adapter

import (
	"context"
	"net/http"

	"github.com/coohu/goagent/internal/core"
)

type Adapter interface {
	Endpoint() string
	Complete(ctx context.Context, req *Request) (*Response, error)
	Stream(ctx context.Context, req *Request) (<-chan Chunk, error)
	CompleteWithTools(ctx context.Context, req *Request, tools []core.ToolSchema) (*Response, error)
}

type Request struct {
	Model     string
	Messages  []core.Message
	MaxTokens int
	System string
}

type Response struct {
	Content      string
	ToolCalls    []core.ToolCallResponse
	TokensIn     int
	TokensOut    int
	FinishReason string
}

func (r *Response) TotalTokens() int { return r.TokensIn + r.TokensOut }

type Chunk struct {
	Delta string
	Done  bool
	Err   error
}

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}
