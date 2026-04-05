package shell

import (
	"bytes"
	"context"
	"os/exec"
	"runtime"
	"time"

	"github.com/coohu/goagent/internal/core"
)

type ExecTool struct {
	timeout       time.Duration
	workspaceRoot string
	shell         string
	shellArgs     []string
}

func NewExecTool(timeout time.Duration, workspaceRoot string) *ExecTool {
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	shell, args := detectShell()

	return &ExecTool{
		timeout:       timeout,
		workspaceRoot: workspaceRoot,
		shell:         shell,
		shellArgs:     args,
	}
}

func detectShell() (string, []string) {
	switch runtime.GOOS {
	case "windows":
		if _, err := exec.LookPath("powershell"); err == nil {
			return "powershell", []string{"-Command"}
		}
		return "cmd", []string{"/C"}
	default:
		return "sh", []string{"-c"}
	}
}

func (t *ExecTool) Name() string        { return "shell.exec" }
func (t *ExecTool) Description() string { return "Execute a shell command and return stdout/stderr" }

func (t *ExecTool) Execute(ctx context.Context, input map[string]any) (*core.ToolResult, error) {
	cmdStr, _ := input["cmd"].(string)
	cwd, _ := input["cwd"].(string)
	if cwd == "" {
		cwd = t.workspaceRoot
	}

	ctx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()

	start := time.Now()

	args := append(t.shellArgs, cmdStr)
	c := exec.CommandContext(ctx, t.shell, args...)
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
