package file

import (
	"context"
	"fmt"
	"os"

	"github.com/coohu/goagent/internal/core"
)

type ReadTool struct{}

func NewReadTool() *ReadTool { return &ReadTool{} }

func (t *ReadTool) Name() string        { return "file.read" }
func (t *ReadTool) Description() string { return "Read the content of a file" }

func (t *ReadTool) Schema() core.ToolSchema {
	return core.ToolSchema{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Path to the file to read",
				},
			},
			"required": []string{"path"},
		},
	}
}

func (t *ReadTool) Validate(input map[string]any) error {
	if _, ok := input["path"].(string); !ok {
		return fmt.Errorf("path is required and must be a string")
	}
	return nil
}

func (t *ReadTool) Execute(_ context.Context, input map[string]any) (*core.ToolResult, error) {
	path, _ := input["path"].(string)
	data, err := os.ReadFile(path)
	if err != nil {
		return &core.ToolResult{
			Success:   false,
			Stderr:    err.Error(),
			RawOutput: err.Error(),
		}, nil
	}
	content := string(data)
	return &core.ToolResult{
		Success:   true,
		Stdout:    content,
		RawOutput: content,
	}, nil
}
