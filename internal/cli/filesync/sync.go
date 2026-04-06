package filesync

import (
	"context"
	"fmt"
	"os"

	"github.com/coohu/goagent/internal/cli/apiclient"
)

type ProgressFn func(name string, done, total int64)

type Engine struct {
	client    *apiclient.Client
	sessionID string
	progress  ProgressFn
}

func New(client *apiclient.Client, sessionID string, progress ProgressFn) *Engine {
	if progress == nil {
		progress = func(_ string, _, _ int64) {}
	}
	return &Engine{client: client, sessionID: sessionID, progress: progress}
}

func (e *Engine) Upload(ctx context.Context, localPaths []string) ([]string, error) {
	for _, p := range localPaths {
		info, err := os.Stat(p)
		if err != nil {
			return nil, fmt.Errorf("file %s: %w", p, err)
		}
		e.progress(p, 0, info.Size())
	}

	result, err := e.client.Upload(ctx, e.sessionID, localPaths)
	if err != nil {
		return nil, err
	}

	for _, p := range localPaths {
		info, _ := os.Stat(p)
		if info != nil {
			e.progress(p, info.Size(), info.Size())
		}
	}

	return result.Uploaded, nil
}

func (e *Engine) Download(ctx context.Context, remotePath, localDest string) error {
	e.progress(remotePath, 0, -1)
	if err := e.client.Download(ctx, e.sessionID, remotePath, localDest); err != nil {
		return err
	}
	info, _ := os.Stat(localDest)
	if info != nil {
		e.progress(remotePath, info.Size(), info.Size())
	}
	return nil
}
