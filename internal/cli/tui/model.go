package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/coohu/goagent/internal/cli/apiclient"
	"github.com/coohu/goagent/internal/cli/cmdparser"
	"github.com/coohu/goagent/internal/cli/state"
)

// ── Styles ────────────────────────────────────────────────────────

var (
	styleUser     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10"))
	styleThought  = lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Italic(true)
	styleToolCall = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)
	styleSuccess  = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	styleError    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	styleSystem   = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	styleStatus   = lipgloss.NewStyle().Background(lipgloss.Color("236")).Foreground(lipgloss.Color("252")).Padding(0, 1)
	styleApproval = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("11")).Padding(0, 1)
)

// ── Message types (bubbletea Msg) ────────────────────────────────

type sseEventMsg apiclient.SSEEvent
type errMsg struct{ err error }
type approvalRequestMsg struct {
	Tool  string
	Input map[string]any
	Risk  string
}
type sessionStartedMsg struct{ sessionID string }
type tickMsg time.Time

// ── Entry in the conversation stream ────────────────────────────

type entryKind int

const (
	entryUser entryKind = iota
	entryThought
	entryToolCall
	entryToolResult
	entrySystem
)

type entry struct {
	kind    entryKind
	text    string
	success *bool
	raw     string
	ts      time.Time
}

// ── Model ────────────────────────────────────────────────────────

type Model struct {
	client   *apiclient.Client
	appState *state.State
	ctx      context.Context
	cancel   context.CancelFunc

	entries    []entry
	viewport   viewport.Model
	input      textarea.Model
	spinner    spinner.Model
	sseChannel <-chan apiclient.SSEEvent

	width, height int
	agentRunning  bool
	approval      *approvalRequestMsg
	statusMsg     string
	inputQueue    []string
}

func New(client *apiclient.Client, appState *state.State) *Model {
	ta := textarea.New()
	ta.Placeholder = "Message GoAgent... (Enter to send, Shift+Enter for newline)"
	ta.Focus()
	ta.SetHeight(3)
	ta.ShowLineNumbers = false
	ta.KeyMap.InsertNewline.SetKeys("shift+enter")

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))

	vp := viewport.New(80, 20)

	ctx, cancel := context.WithCancel(context.Background())

	m := &Model{
		client:   client,
		appState: appState,
		ctx:      ctx,
		cancel:   cancel,
		viewport: vp,
		input:    ta,
		spinner:  sp,
	}
	m.pushSystem("GoAgent CLI ready. Type your goal or /help for commands.")
	return m
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		textarea.Blink,
		m.spinner.Tick,
		tea.EnterAltScreen,
	)
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - 7
		m.input.SetWidth(msg.Width - 2)

	case tea.KeyMsg:
		if m.approval != nil {
			return m.handleApprovalKey(msg, &cmds)
		}
		switch msg.String() {
		case "ctrl+c":
			cmds = append(cmds, m.handleCtrlC())
			return m, tea.Batch(cmds...)
		case "enter":
			text := strings.TrimSpace(m.input.Value())
			m.input.Reset()
			if text != "" {
				cmds = append(cmds, m.handleInput(text))
			}
		}

	case sseEventMsg:
		cmds = append(cmds, m.handleSSEEvent(apiclient.SSEEvent(msg)))

	case approvalRequestMsg:
		m.approval = &msg

	case sessionStartedMsg:
		m.appState.SessionID = msg.sessionID
		_ = m.appState.Save()

	case errMsg:
		m.pushSystem("Error: " + msg.err.Error())
		m.agentRunning = false

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)
	}

	var vpCmd, taCmd tea.Cmd
	m.viewport, vpCmd = m.viewport.Update(msg)
	m.input, taCmd = m.input.Update(msg)
	cmds = append(cmds, vpCmd, taCmd)

	m.viewport.SetContent(m.renderEntries())
	m.viewport.GotoBottom()

	return m, tea.Batch(cmds...)
}

func (m *Model) View() string {
	if m.approval != nil {
		return m.renderApproval()
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		m.viewport.View(),
		m.renderStatusBar(),
		m.input.View(),
	)
}

// ── Handlers ────────────────────────────────────────────────────

func (m *Model) handleInput(text string) tea.Cmd {
	cmd := cmdparser.Parse(text)

	if cmd.Kind == cmdparser.KindGoal {
		m.pushEntry(entry{kind: entryUser, text: text, ts: time.Now()})
		if m.agentRunning {
			m.inputQueue = append(m.inputQueue, text)
			m.pushSystem("Queued (agent is running)")
			return nil
		}
		return m.sendGoal(text)
	}

	return m.handleSlash(cmd)
}

func (m *Model) handleSlash(cmd cmdparser.Command) tea.Cmd {
	switch cmd.Slash {
	case "help":
		m.pushSystem(cmdparser.HelpText())
	case "exit":
		m.cancel()
		return tea.Quit
	case "clear":
		m.entries = nil
		m.appState.ClearSession()
		_ = m.appState.Save()
		m.pushSystem("Context cleared. New session will start on next message.")
	case "session":
		return m.handleSessionCmd(cmd.Args)
	case "model":
		return m.handleModelCmd(cmd.Args)
	case "upload":
		return m.handleUpload(cmd.Args)
	case "download":
		return m.handleDownload(cmd.Args)
	case "config":
		return m.handleConfig(cmd.Args)
	default:
		m.pushSystem(fmt.Sprintf("Unknown command: /%s — type /help", cmd.Slash))
	}
	return nil
}

func (m *Model) handleCtrlC() tea.Cmd {
	if m.agentRunning && m.appState.SessionID != "" {
		go m.client.Cancel(m.ctx, m.appState.SessionID)
		m.pushSystem("Cancelling task...")
		m.agentRunning = false
		return nil
	}
	m.cancel()
	return tea.Quit
}

func (m *Model) handleApprovalKey(msg tea.KeyMsg, cmds *[]tea.Cmd) (tea.Model, tea.Cmd) {
	switch strings.ToLower(msg.String()) {
	case "y", "enter":
		go m.client.Approve(m.ctx, m.appState.SessionID, true, "")
		m.pushSystem("✅ Approved")
		m.approval = nil
	case "n":
		go m.client.Approve(m.ctx, m.appState.SessionID, false, "")
		m.pushSystem("❌ Rejected")
		m.approval = nil
	}
	return m, tea.Batch(*cmds...)
}

func (m *Model) handleSSEEvent(ev apiclient.SSEEvent) tea.Cmd {
	switch ev.Type {
	case "thought":
		if content, ok := ev.Payload["content"].(string); ok {
			m.pushEntry(entry{kind: entryThought, text: content, ts: time.Now()})
		}
	case "tool_call":
		tool, _ := ev.Payload["tool"].(string)
		input, _ := ev.Payload["input"].(string)
		m.pushEntry(entry{kind: entryToolCall, text: fmt.Sprintf("[%s] %s", tool, input), ts: time.Now()})
	case "tool_result":
		ok, _ := ev.Payload["success"].(bool)
		summary, _ := ev.Payload["summary"].(string)
		raw, _ := ev.Payload["content"].(string)
		t := true
		f := false
		suc := &t
		if !ok {
			suc = &f
		}
		m.pushEntry(entry{kind: entryToolResult, text: summary, success: suc, raw: raw, ts: time.Now()})
	case "step_done":
		// handled by tool_result above
	case "done":
		result, _ := ev.Payload["result"].(string)
		m.pushEntry(entry{kind: entrySystem, text: "✅ Done: " + result, ts: time.Now()})
		m.agentRunning = false
		if len(m.inputQueue) > 0 {
			next := m.inputQueue[0]
			m.inputQueue = m.inputQueue[1:]
			return m.sendGoal(next)
		}
	case "error":
		reason, _ := ev.Payload["reason"].(string)
		m.pushEntry(entry{kind: entrySystem, text: "❌ Error: " + reason, ts: time.Now()})
		m.agentRunning = false
	case "approval_required":
		tool, _ := ev.Payload["tool"].(string)
		var input map[string]any
		if v, ok := ev.Payload["input"].(map[string]any); ok {
			input = v
		}
		return func() tea.Msg {
			return approvalRequestMsg{Tool: tool, Input: input, Risk: "high"}
		}
	case "state_change":
		to, _ := ev.Payload["to"].(string)
		m.statusMsg = to
	}
	return nil
}

func (m *Model) sendGoal(goal string) tea.Cmd {
	m.agentRunning = true
	return func() tea.Msg {
		ctx := m.ctx
		var sessionID string

		if m.appState.SessionID != "" {
			if resp, err := m.client.Continue(ctx, m.appState.SessionID, goal); err == nil {
				sessionID = resp.SessionID
			}
		}
		if sessionID == "" {
			resp, err := m.client.Run(ctx, goal, nil)
			if err != nil {
				return errMsg{err}
			}
			sessionID = resp.SessionID
		}

		m.appState.SessionID = sessionID
		_ = m.appState.Save()

		evCh := make(chan apiclient.SSEEvent, 128)
		go func() {
			_ = m.client.StreamEvents(ctx, sessionID, 0, evCh)
			close(evCh)
		}()
		m.sseChannel = evCh

		return sessionStartedMsg{sessionID: sessionID}
	}
}

// ── Slash sub-handlers ──────────────────────────────────────────

func (m *Model) handleSessionCmd(args []string) tea.Cmd {
	if len(args) == 0 {
		m.pushSystem("Current session: " + orNone(m.appState.SessionID))
		return nil
	}
	switch args[0] {
	case "new":
		m.appState.ClearSession()
		_ = m.appState.Save()
		m.pushSystem("New session will start on next message.")
	case "list":
		return func() tea.Msg {
			sessions, err := m.client.ListSessions(m.ctx)
			if err != nil {
				return errMsg{err}
			}
			var sb strings.Builder
			for _, s := range sessions {
				sb.WriteString(fmt.Sprintf("  %s  [%s]  %s\n", s.ID[:8], s.State, s.Goal))
			}
			return sseEventMsg{Type: "__system__", Payload: map[string]any{"text": sb.String()}}
		}
	default:
		m.appState.SessionID = args[0]
		_ = m.appState.Save()
		m.pushSystem("Switched to session: " + args[0])
	}
	return nil
}

func (m *Model) handleModelCmd(args []string) tea.Cmd {
	if len(args) == 0 {
		m.pushSystem("Current model: " + m.appState.Model)
		return nil
	}
	m.appState.Model = args[0]
	_ = m.appState.Save()
	if m.appState.SessionID != "" {
		go m.client.UpdateConfig(m.ctx, m.appState.SessionID, map[string]any{
			"planner_model":  args[0],
			"executor_model": args[0],
		})
	}
	m.pushSystem("Model set to: " + args[0])
	return nil
}

func (m *Model) handleUpload(args []string) tea.Cmd {
	if len(args) == 0 {
		m.pushSystem("Usage: /upload <local-path>")
		return nil
	}
	if m.appState.SessionID == "" {
		m.pushSystem("No active session. Send a message first.")
		return nil
	}
	paths := args
	return func() tea.Msg {
		result, err := m.client.Upload(m.ctx, m.appState.SessionID, paths)
		if err != nil {
			return errMsg{err}
		}
		return sseEventMsg{Type: "__system__",
			Payload: map[string]any{"text": fmt.Sprintf("Uploaded: %v", result.Uploaded)}}
	}
}

func (m *Model) handleDownload(args []string) tea.Cmd {
	if len(args) == 0 {
		m.pushSystem("Usage: /download <remote-path> [local-dest]")
		return nil
	}
	remote := args[0]
	local := remote
	if len(args) > 1 {
		local = args[1]
	}
	if m.appState.SessionID == "" {
		m.pushSystem("No active session.")
		return nil
	}
	return func() tea.Msg {
		if err := m.client.Download(m.ctx, m.appState.SessionID, remote, local); err != nil {
			return errMsg{err}
		}
		return sseEventMsg{Type: "__system__",
			Payload: map[string]any{"text": "Downloaded to: " + local}}
	}
}

func (m *Model) handleConfig(args []string) tea.Cmd {
	if len(args) < 2 {
		m.pushSystem("Usage: /config <key> <value>  (e.g. /config max_steps 50)")
		return nil
	}
	if m.appState.SessionID == "" {
		m.pushSystem("No active session.")
		return nil
	}
	key, val := args[0], args[1]
	return func() tea.Msg {
		if err := m.client.UpdateConfig(m.ctx, m.appState.SessionID, map[string]any{key: val}); err != nil {
			return errMsg{err}
		}
		return sseEventMsg{Type: "__system__",
			Payload: map[string]any{"text": fmt.Sprintf("Config updated: %s = %s", key, val)}}
	}
}

// ── Rendering ───────────────────────────────────────────────────

func (m *Model) renderEntries() string {
	var sb strings.Builder
	for _, e := range m.entries {
		switch e.kind {
		case entryUser:
			sb.WriteString(styleUser.Render("You") + "  " + e.text + "\n\n")
		case entryThought:
			sb.WriteString(styleThought.Render("  ↳ "+e.text) + "\n")
		case entryToolCall:
			sb.WriteString(styleToolCall.Render("  ⚙ "+e.text) + "\n")
		case entryToolResult:
			icon := "✅"
			style := styleSuccess
			if e.success != nil && !*e.success {
				icon = "❌"
				style = styleError
			}
			sb.WriteString(style.Render("  "+icon+" "+e.text) + "\n\n")
		case entrySystem:
			sb.WriteString(styleSystem.Render("  "+e.text) + "\n\n")
		}
	}
	return sb.String()
}

func (m *Model) renderStatusBar() string {
	sess := orNone(m.appState.SessionID)
	if len(sess) > 8 {
		sess = sess[:8]
	}
	status := m.statusMsg
	if m.agentRunning {
		status = m.spinner.View() + " " + status
	}
	bar := fmt.Sprintf(" session:%s  model:%s  %s  %s ",
		sess, m.appState.Model, m.appState.APIURL, status)
	return styleStatus.Width(m.width).Render(bar)
}

func (m *Model) renderApproval() string {
	if m.approval == nil {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("⚠️  Approval Required\n\n")
	sb.WriteString(fmt.Sprintf("Tool:   %s\n", m.approval.Tool))
	if len(m.approval.Input) > 0 {
		for k, v := range m.approval.Input {
			sb.WriteString(fmt.Sprintf("  %s: %v\n", k, v))
		}
	}
	sb.WriteString(fmt.Sprintf("Risk:   %s\n\n", m.approval.Risk))
	sb.WriteString("  [y] Approve    [n] Reject\n")
	return styleApproval.Width(60).Render(sb.String())
}

// ── Helpers ─────────────────────────────────────────────────────

func (m *Model) pushEntry(e entry) {
	m.entries = append(m.entries, e)
}

func (m *Model) pushSystem(text string) {
	m.entries = append(m.entries, entry{kind: entrySystem, text: text, ts: time.Now()})
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}
