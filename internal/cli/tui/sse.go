package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/coohu/goagent/internal/cli/apiclient"
)

// WaitForSSE returns a Cmd that reads one event from ch and injects it into the update loop.
// The model re-subscribes after each event, creating a continuous stream.
func WaitForSSE(ch <-chan apiclient.SSEEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return nil
		}
		return sseEventMsg(ev)
	}
}
