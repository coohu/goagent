package cmdparser

import (
	"strings"
)

type Kind int

const (
	KindGoal  Kind = iota
	KindSlash
)

type Command struct {
	Kind  Kind
	Slash string
	Args  []string
	Raw   string
}

func Parse(input string) Command {
	input = strings.TrimSpace(input)
	if input == "" {
		return Command{Kind: KindGoal, Raw: input}
	}
	if !strings.HasPrefix(input, "/") {
		return Command{Kind: KindGoal, Raw: input}
	}
	parts := strings.Fields(input)
	slash := strings.TrimPrefix(parts[0], "/")
	return Command{Kind: KindSlash, Slash: slash, Args: parts[1:], Raw: input}
}

func Known(slash string) bool {
	switch slash {
	case "model", "clear", "session", "help", "exit",
		"upload", "download", "config":
		return true
	}
	return false
}

func HelpText() string {
	return `Available commands:

  /model                           Show scene→model config and available models
  /model <model_id>                Set ALL scenes to the same model
  /model plan|exec|sum|reflect <id>  Set one scene to a specific model

    Scenes:
      plan    → planner (creates the task plan)
      exec    → executor (calls tools, runs ReAct loop)
      sum     → summarizer (compresses tool output into memory)
      reflect → reflector (evaluates step success)

    Examples:
      /model gpt-4o
      /model exec gpt-4o-mini
      /model reflect qwen-plus

  /clear                           Reset context (keeps session, clears scratchpad)
  /session                         Show current session ID
  /session new                     Start a new session
  /session list                    List all sessions on the server
  /session <id>                    Switch to an existing session
  /upload <path> [...]             Upload local file(s) to agent workspace
  /download <remote> [local]       Download a file from agent workspace
  /config <key> <value>            Adjust runtime config (e.g. /config max_steps 50)
  /help                            Show this help
  /exit                            Exit the CLI`
}
