package file

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/coohu/goagent/internal/core"
)

type WriteTool struct{}

func NewWriteTool() *WriteTool { return &WriteTool{} }

func (t *WriteTool) Name() string { return "file.write" }
func (t *WriteTool) Description() string {
	return "Write content to a file, creating directories as needed"
}

func (t *WriteTool) Schema() core.ToolSchema {
	return core.ToolSchema{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":    map[string]any{"type": "string", "description": "Target file path"},
				"content": map[string]any{"type": "string", "description": "Content to write"},
			},
			"required": []string{"path", "content"},
		},
	}
}

func (t *WriteTool) Validate(input map[string]any) error {
	if _, ok := input["path"].(string); !ok {
		return fmt.Errorf("path is required")
	}
	if _, ok := input["content"].(string); !ok {
		return fmt.Errorf("content is required")
	}
	return nil
}

func (t *WriteTool) Execute(_ context.Context, input map[string]any) (*core.ToolResult, error) {
	path, _ := input["path"].(string)
	content, _ := input["content"].(string)

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return &core.ToolResult{Success: false, Stderr: err.Error(), RawOutput: err.Error()}, nil
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return &core.ToolResult{Success: false, Stderr: err.Error(), RawOutput: err.Error()}, nil
	}
	msg := fmt.Sprintf("wrote %d bytes to %s", len(content), path)
	return &core.ToolResult{
		Success:      true,
		Stdout:       msg,
		RawOutput:    msg,
		FilesChanged: []string{path},
	}, nil
}
