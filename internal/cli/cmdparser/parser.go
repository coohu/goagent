package cmdparser

import (
	"strings"
)

type Kind int

const (
	KindGoal    Kind = iota // plain text → send to agent
	KindSlash               // /command [args...]
)

type Command struct {
	Kind   Kind
	Slash  string
	Args   []string
	Raw    string
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
	args := parts[1:]
	return Command{Kind: KindSlash, Slash: slash, Args: args, Raw: input}
}

// Known returns true for built-in slash commands
func Known(slash string) bool {
	switch slash {
	case "model", "clear", "session", "help", "exit",
		"upload", "download", "config":
		return true
	}
	return false
}

// HelpText returns a short description for each command
func HelpText() string {
	return `Available commands:
  /model [name]         View or switch the current LLM model
  /clear                Reset the current reasoning context
  /session [new|list|id] Switch or create a session
  /upload <path>        Upload a local file to the agent workspace
  /download <path>      Download a file from the agent workspace
  /config <key> <val>   Adjust agent config (e.g. /config max_steps 50)
  /help                 Show this help
  /exit                 Exit the CLI`
}
