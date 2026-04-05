package file

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/coohu/goagent/internal/core"
)

type ListTool struct{}

func NewListTool() *ListTool { return &ListTool{} }

func (t *ListTool) Name() string        { return "file.list" }
func (t *ListTool) Description() string { return "List files and directories in a path" }

func (t *ListTool) Schema() core.ToolSchema {
	return core.ToolSchema{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":  map[string]any{"type": "string", "description": "Directory path"},
				"depth": map[string]any{"type": "integer", "description": "Max depth, default 2"},
			},
			"required": []string{"path"},
		},
	}
}

func (t *ListTool) Validate(input map[string]any) error {
	if _, ok := input["path"].(string); !ok {
		return fmt.Errorf("path is required")
	}
	return nil
}

func (t *ListTool) Execute(_ context.Context, input map[string]any) (*core.ToolResult, error) {
	root, _ := input["path"].(string)
	maxDepth := 2
	if d, ok := input["depth"].(float64); ok {
		maxDepth = int(d)
	}

	var lines []string
	err := walkDepth(root, root, 0, maxDepth, &lines)
	if err != nil {
		return &core.ToolResult{Success: false, Stderr: err.Error(), RawOutput: err.Error()}, nil
	}
	out := strings.Join(lines, "\n")
	return &core.ToolResult{Success: true, Stdout: out, RawOutput: out}, nil
}

func walkDepth(root, path string, depth, maxDepth int, lines *[]string) error {
	if depth > maxDepth {
		return nil
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	for _, e := range entries {
		rel, _ := filepath.Rel(root, filepath.Join(path, e.Name()))
		indent := strings.Repeat("  ", depth)
		if e.IsDir() {
			*lines = append(*lines, indent+e.Name()+"/")
			_ = walkDepth(root, filepath.Join(path, e.Name()), depth+1, maxDepth, lines)
		} else {
			*lines = append(*lines, indent+rel)
		}
	}
	return nil
}

type SearchTool struct{}

func NewSearchTool() *SearchTool { return &SearchTool{} }

func (t *SearchTool) Name() string        { return "file.search" }
func (t *SearchTool) Description() string { return "Search for a pattern in files within a directory" }

func (t *SearchTool) Schema() core.ToolSchema {
	return core.ToolSchema{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":    map[string]any{"type": "string"},
				"pattern": map[string]any{"type": "string", "description": "Text pattern to search for"},
			},
			"required": []string{"path", "pattern"},
		},
	}
}

func (t *SearchTool) Validate(input map[string]any) error {
	for _, k := range []string{"path", "pattern"} {
		if _, ok := input[k].(string); !ok {
			return fmt.Errorf("%s is required", k)
		}
	}
	return nil
}

func (t *SearchTool) Execute(_ context.Context, input map[string]any) (*core.ToolResult, error) {
	root, _ := input["path"].(string)
	pattern, _ := input["pattern"].(string)

	var matches []string
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if strings.Contains(line, pattern) {
				matches = append(matches, fmt.Sprintf("%s:%d: %s", path, i+1, strings.TrimSpace(line)))
			}
		}
		return nil
	})

	out := strings.Join(matches, "\n")
	if out == "" {
		out = "no matches found"
	}
	return &core.ToolResult{Success: true, Stdout: out, RawOutput: out}, nil
}
