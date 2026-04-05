package shell

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/coohu/goagent/internal/core"
)

type ExecTool struct {
	timeout       time.Duration
	workspaceRoot string
}

func NewExecTool(timeout time.Duration, workspaceRoot string) *ExecTool {
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	return &ExecTool{timeout: timeout, workspaceRoot: workspaceRoot}
}

func (t *ExecTool) Name() string        { return "shell.exec" }
func (t *ExecTool) Description() string { return "Execute a shell command and return stdout/stderr" }

func (t *ExecTool) Schema() core.ToolSchema {
	return core.ToolSchema{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"cmd": map[string]any{
					"type":        "string",
					"description": "Shell command to execute",
				},
				"cwd": map[string]any{
					"type":        "string",
					"description": "Working directory (optional)",
				},
			},
			"required": []string{"cmd"},
		},
	}
}

func (t *ExecTool) Validate(input map[string]any) error {
	if _, ok := input["cmd"].(string); !ok {
		return fmt.Errorf("cmd is required")
	}
	return nil
}

func (t *ExecTool) Execute(ctx context.Context, input map[string]any) (*core.ToolResult, error) {
	cmd, _ := input["cmd"].(string)
	cwd, _ := input["cwd"].(string)
	if cwd == "" {
		cwd = t.workspaceRoot
	}

	ctx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()

	start := time.Now()
	c := exec.CommandContext(ctx, "sh", "-c", cmd)
	c.Dir = cwd

	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr

	err := c.Run()
	dur := time.Since(start)

	exitCode := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			exitCode = -1
		}
	}

	raw := stdout.String()
	if stderr.Len() > 0 {
		raw += "\nSTDERR: " + stderr.String()
	}

	return &core.ToolResult{
		Success:   exitCode == 0,
		Stdout:    stdout.String(),
		Stderr:    stderr.String(),
		ExitCode:  exitCode,
		RawOutput: raw,
		Duration:  dur,
	}, nil
}
