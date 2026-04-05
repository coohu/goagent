package git

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/yourorg/goagent/internal/core"
)

type CloneTool struct {
	workspaceRoot string
}

func NewCloneTool(workspaceRoot string) *CloneTool {
	return &CloneTool{workspaceRoot: workspaceRoot}
}

func (t *CloneTool) Name() string        { return "git.clone" }
func (t *CloneTool) Description() string { return "Clone a git repository into the workspace" }

func (t *CloneTool) Schema() core.ToolSchema {
	return core.ToolSchema{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url":  map[string]any{"type": "string", "description": "Repository URL"},
				"dest": map[string]any{"type": "string", "description": "Destination directory name (optional)"},
			},
			"required": []string{"url"},
		},
	}
}

func (t *CloneTool) Validate(input map[string]any) error {
	if _, ok := input["url"].(string); !ok {
		return fmt.Errorf("url is required")
	}
	return nil
}

func (t *CloneTool) Execute(ctx context.Context, input map[string]any) (*core.ToolResult, error) {
	url, _ := input["url"].(string)
	dest, _ := input["dest"].(string)

	args := []string{"clone", url}
	if dest != "" {
		args = append(args, dest)
	}

	return runGit(ctx, t.workspaceRoot, args...)
}

type CommitTool struct {
	workspaceRoot string
}

func NewCommitTool(workspaceRoot string) *CommitTool {
	return &CommitTool{workspaceRoot: workspaceRoot}
}

func (t *CommitTool) Name() string        { return "git.commit" }
func (t *CommitTool) Description() string { return "Stage all changes and create a git commit" }

func (t *CommitTool) Schema() core.ToolSchema {
	return core.ToolSchema{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"message": map[string]any{"type": "string", "description": "Commit message"},
				"cwd":     map[string]any{"type": "string", "description": "Repository directory"},
			},
			"required": []string{"message"},
		},
	}
}

func (t *CommitTool) Validate(input map[string]any) error {
	if _, ok := input["message"].(string); !ok {
		return fmt.Errorf("message is required")
	}
	return nil
}

func (t *CommitTool) Execute(ctx context.Context, input map[string]any) (*core.ToolResult, error) {
	message, _ := input["message"].(string)
	cwd, _ := input["cwd"].(string)
	if cwd == "" {
		cwd = t.workspaceRoot
	}

	if result, _ := runGitIn(ctx, cwd, "add", "-A"); !result.Success {
		return result, nil
	}
	return runGitIn(ctx, cwd, "commit", "-m", message)
}

func runGit(ctx context.Context, cwd string, args ...string) (*core.ToolResult, error) {
	return runGitIn(ctx, cwd, args...)
}

func runGitIn(ctx context.Context, cwd string, args ...string) (*core.ToolResult, error) {
	start := time.Now()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = cwd

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	dur := time.Since(start)

	exitCode := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			exitCode = -1
		}
	}

	out := stdout.String()
	if stderr.Len() > 0 {
		out += "\nSTDERR: " + stderr.String()
	}

	return &core.ToolResult{
		Success:   exitCode == 0,
		Stdout:    stdout.String(),
		Stderr:    stderr.String(),
		ExitCode:  exitCode,
		RawOutput: out,
		Duration:  dur,
	}, nil
}
