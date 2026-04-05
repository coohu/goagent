package pipeline

import "context"

type Handler func(ctx context.Context, data any, next func(context.Context, any) error) error

type Pipeline struct {
	handlers []Handler
}

func New(hs ...Handler) *Pipeline {
	return &Pipeline{handlers: hs}
}

func (p *Pipeline) Use(h Handler) *Pipeline {
	p.handlers = append(p.handlers, h)
	return p
}

func (p *Pipeline) Run(ctx context.Context, data any) error {
	return p.run(ctx, data, 0)
}

func (p *Pipeline) run(ctx context.Context, data any, i int) error {
	if i >= len(p.handlers) {
		return nil
	}
	return p.handlers[i](ctx, data, func(c context.Context, d any) error {
		return p.run(c, d, i+1)
	})
}
