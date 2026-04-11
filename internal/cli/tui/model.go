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
	styleApproval = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("11")).Padding(1, 2)
)

// ── Msg types ────────────────────────────────────────────────────

type sseEventMsg apiclient.SSEEvent
type errMsg struct{ err error }
type approvalRequestMsg struct {
	Tool  string
	Input map[string]any
	Risk  string
}
type sessionStartedMsg struct{ sessionID string }

// ── Conversation entry ────────────────────────────────────────────

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
	ts      time.Time
}

// ── Model ─────────────────────────────────────────────────────────

type Model struct {
	client     *apiclient.Client
	appState   *state.State
	ctx        context.Context
	cancel     context.CancelFunc
	sseChannel <-chan apiclient.SSEEvent

	entries    []entry
	viewport   viewport.Model
	input      textarea.Model
	spinner    spinner.Model
	inputQueue []string

	width, height int
	agentRunning  bool
	approval      *approvalRequestMsg
	statusMsg     string
}

func New(client *apiclient.Client, appState *state.State) *Model {
	ta := textarea.New()
	ta.Placeholder = "Message GoAgent...  (/help for commands, Shift+Enter for newline)"
	ta.Focus()
	ta.SetHeight(3)
	ta.ShowLineNumbers = false
	ta.KeyMap.InsertNewline.SetKeys("shift+enter")

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))

	ctx, cancel := context.WithCancel(context.Background())

	m := &Model{
		client:   client,
		appState: appState,
		ctx:      ctx,
		cancel:   cancel,
		viewport: viewport.New(80, 20),
		input:    ta,
		spinner:  sp,
	}
	m.pushSystem("GoAgent CLI  —  type your goal or /help for commands.")
	return m
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, m.spinner.Tick, tea.EnterAltScreen)
}

// ── Update ────────────────────────────────────────────────────────

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - 6
		m.input.SetWidth(msg.Width - 2)

	case tea.KeyMsg:
		if m.approval != nil {
			return m.handleApprovalKey(msg)
		}
		switch msg.String() {
		case "ctrl+c":
			return m, m.handleCtrlC()
		case "enter":
			if text := strings.TrimSpace(m.input.Value()); text != "" {
				m.input.Reset()
				cmds = append(cmds, m.handleInput(text))
			}
		}

	case sseEventMsg:
		if cmd := m.handleSSEEvent(apiclient.SSEEvent(msg)); cmd != nil {
			cmds = append(cmds, cmd)
		}
		if m.sseChannel != nil {
			cmds = append(cmds, WaitForSSE(m.sseChannel))
		}

	case approvalRequestMsg:
		m.approval = &msg

	case sessionStartedMsg:
		m.appState.SessionID = msg.sessionID
		_ = m.appState.Save()
		if m.sseChannel != nil {
			cmds = append(cmds, WaitForSSE(m.sseChannel))
		}

	case errMsg:
		m.pushSystem("❌ " + msg.err.Error())
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

// ── View ──────────────────────────────────────────────────────────

func (m *Model) View() string {
	if m.approval != nil {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, m.renderApproval())
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		m.viewport.View(),
		m.renderStatusBar(),
		m.input.View(),
	)
}

// ── Input dispatch ────────────────────────────────────────────────

func (m *Model) handleInput(text string) tea.Cmd {
	cmd := cmdparser.Parse(text)
	if cmd.Kind == cmdparser.KindGoal {
		m.pushEntry(entry{kind: entryUser, text: text, ts: time.Now()})
		if m.agentRunning {
			m.inputQueue = append(m.inputQueue, text)
			m.pushSystem("Queued — agent is running.")
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
		m.sseChannel = nil
		m.agentRunning = false
		m.pushSystem("Context cleared.")
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
		m.pushSystem(fmt.Sprintf("Unknown command /%s — type /help", cmd.Slash))
	}
	return nil
}

func (m *Model) handleCtrlC() tea.Cmd {
	if m.agentRunning && m.appState.SessionID != "" {
		sessionID := m.appState.SessionID
		m.agentRunning = false
		m.pushSystem("Cancelling...")
		return func() tea.Msg {
			_ = m.client.Cancel(m.ctx, sessionID)
			return nil
		}
	}
	m.cancel()
	return tea.Quit
}

func (m *Model) handleApprovalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch strings.ToLower(msg.String()) {
	case "y", "enter":
		sessionID := m.appState.SessionID
		m.approval = nil
		m.pushSystem("✅ Approved")
		return m, func() tea.Msg {
			_ = m.client.Approve(m.ctx, sessionID, true, "")
			return nil
		}
	case "n", "q", "escape":
		sessionID := m.appState.SessionID
		m.approval = nil
		m.pushSystem("❌ Rejected")
		return m, func() tea.Msg {
			_ = m.client.Approve(m.ctx, sessionID, false, "")
			return nil
		}
	}
	return m, nil
}

// ── SSE event handler ─────────────────────────────────────────────

func (m *Model) handleSSEEvent(ev apiclient.SSEEvent) tea.Cmd {
	switch ev.Type {
	case "thought":
		if v, ok := ev.Payload["content"].(string); ok && v != "" {
			m.pushEntry(entry{kind: entryThought, text: v, ts: time.Now()})
		}

	case "tool_call":
		tool, _ := ev.Payload["tool"].(string)
		input, _ := ev.Payload["input"].(string)
		m.pushEntry(entry{kind: entryToolCall, text: fmt.Sprintf("[%s]  %s", tool, input), ts: time.Now()})

	case "tool_result":
		ok, _ := ev.Payload["success"].(bool)
		summary, _ := ev.Payload["summary"].(string)
		if summary == "" {
			summary, _ = ev.Payload["content"].(string)
		}
		suc := ok
		m.pushEntry(entry{kind: entryToolResult, text: summary, success: &suc, ts: time.Now()})

	case "state_change":
		if v, ok := ev.Payload["to"].(string); ok {
			m.statusMsg = v
		}

	case "done":
		result, _ := ev.Payload["result"].(string)
		m.pushSystem("✅ " + result)
		m.agentRunning = false
		m.sseChannel = nil
		if len(m.inputQueue) > 0 {
			next := m.inputQueue[0]
			m.inputQueue = m.inputQueue[1:]
			m.pushEntry(entry{kind: entryUser, text: next, ts: time.Now()})
			return m.sendGoal(next)
		}

	case "error":
		reason, _ := ev.Payload["reason"].(string)
		m.pushSystem("❌ " + reason)
		m.agentRunning = false
		m.sseChannel = nil

	case "approval_required":
		tool, _ := ev.Payload["tool"].(string)
		input, _ := ev.Payload["input"].(map[string]any)
		return func() tea.Msg {
			return approvalRequestMsg{Tool: tool, Input: input, Risk: "high"}
		}

	case "__system__":
		if v, ok := ev.Payload["text"].(string); ok {
			m.pushSystem(v)
		}
	}
	return nil
}

// ── Goal submission ───────────────────────────────────────────────

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
			resp, err := m.client.Run(ctx, goal, map[string]any{
				"models": m.appState.Models,
			})
			if err != nil {
				return errMsg{err}
			}
			sessionID = resp.SessionID
		}

		evCh := make(chan apiclient.SSEEvent, 128)
		go func() {
			_ = m.client.StreamEvents(ctx, sessionID, 0, evCh)
			close(evCh)
		}()
		m.sseChannel = evCh

		return sessionStartedMsg{sessionID: sessionID}
	}
}

// ── Slash sub-commands ────────────────────────────────────────────

func (m *Model) handleSessionCmd(args []string) tea.Cmd {
	if len(args) == 0 {
		m.pushSystem("Current session: " + orNone(m.appState.SessionID))
		return nil
	}
	switch args[0] {
	case "new":
		m.appState.ClearSession()
		_ = m.appState.Save()
		m.sseChannel = nil
		m.agentRunning = false
		m.pushSystem("New session will start on next message.")
	case "list":
		return func() tea.Msg {
			sessions, err := m.client.ListSessions(m.ctx)
			if err != nil {
				return errMsg{err}
			}
			var sb strings.Builder
			if len(sessions) == 0 {
				sb.WriteString("  (no sessions)")
			}
			for _, s := range sessions {
				id := s.ID
				if len(id) > 8 {
					id = id[:8]
				}
				sb.WriteString(fmt.Sprintf("  %s  %-12s  %s\n", id, "["+s.State+"]", s.Goal))
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

// handleModelCmd handles:
//
//	/model                           → show current scene config + available models
//	/model <model_id>                → set ALL scenes to model_id
//	/model plan|exec|sum|reflect <model_id> → set one scene
func (m *Model) handleModelCmd(args []string) tea.Cmd {
	if len(args) == 0 {
		return m.listModels()
	}

	// Two-arg form: /model <scene> <model_id>
	if len(args) >= 2 {
		scene := normalizeScene(args[0])
		if scene != "" {
			modelID := args[1]
			m.applySceneModel(scene, modelID)
			_ = m.appState.Save()
			m.pushSystem(fmt.Sprintf("Model [%s] → %s", scene, modelID))
			return m.syncModels()
		}
	}

	// One-arg form: /model <model_id>  → set all scenes
	modelID := args[0]
	m.appState.Models = state.SceneModels{
		Planning:  modelID,
		Execute:   modelID,
		Summarize: modelID,
		Reflect:   modelID,
	}
	_ = m.appState.Save()
	m.pushSystem(fmt.Sprintf("All scenes → %s", modelID))
	return m.syncModels()
}

// normalizeScene maps short aliases to canonical scene names.
// Returns "" if the input is not a recognised scene keyword.
func normalizeScene(s string) string {
	switch strings.ToLower(s) {
	case "plan", "planning":
		return "planning"
	case "exec", "execute":
		return "execute"
	case "sum", "summarize", "summary":
		return "summarize"
	case "reflect", "reflection":
		return "reflect"
	}
	return ""
}

// applySceneModel updates one scene in the local state.
func (m *Model) applySceneModel(scene, modelID string) {
	switch scene {
	case "planning":
		m.appState.Models.Planning = modelID
	case "execute":
		m.appState.Models.Execute = modelID
	case "summarize":
		m.appState.Models.Summarize = modelID
	case "reflect":
		m.appState.Models.Reflect = modelID
	}
}

// syncModels pushes the current SceneModels to the server (if a session is active).
func (m *Model) syncModels() tea.Cmd {
	if m.appState.SessionID == "" {
		return nil
	}
	sessionID := m.appState.SessionID
	models := m.appState.Models
	return func() tea.Msg {
		patch := apiclient.ConfigPatch{
			Models: &apiclient.SceneModelPatch{
				Planning:  models.Planning,
				Execute:   models.Execute,
				Summarize: models.Summarize,
				Reflect:   models.Reflect,
			},
		}
		if err := m.client.UpdateConfig(m.ctx, sessionID, patch); err != nil {
			return errMsg{err}
		}
		return sseEventMsg{Type: "__system__", Payload: map[string]any{
			"text": "Scene models synced to server.",
		}}
	}
}

// listModels fetches available models from the server and shows the current scene config.
func (m *Model) listModels() tea.Cmd {
	return func() tea.Msg {
		models, err := m.client.ListModels(m.ctx)
		if err != nil {
			return errMsg{err}
		}
		cur := m.appState.Models
		var sb strings.Builder
		sb.WriteString("Current scene config:\n")
		sb.WriteString(fmt.Sprintf("  plan     → %s\n", cur.Planning))
		sb.WriteString(fmt.Sprintf("  exec     → %s\n", cur.Execute))
		sb.WriteString(fmt.Sprintf("  sum      → %s\n", cur.Summarize))
		sb.WriteString(fmt.Sprintf("  reflect  → %s\n\n", cur.Reflect))
		sb.WriteString("Available models:\n")
		for _, mod := range models {
			sb.WriteString(fmt.Sprintf("  %s  (%s)\n", mod.ID, mod.Provider))
		}
		sb.WriteString("\nUsage:\n")
		sb.WriteString("  /model <model_id>               set all scenes\n")
		sb.WriteString("  /model plan|exec|sum|reflect <model_id>\n")
		return sseEventMsg{Type: "__system__", Payload: map[string]any{"text": sb.String()}}
	}
}

func (m *Model) handleUpload(args []string) tea.Cmd {
	if len(args) == 0 {
		m.pushSystem("Usage: /upload <local-path> [...]")
		return nil
	}
	if m.appState.SessionID == "" {
		m.pushSystem("No active session. Send a goal first.")
		return nil
	}
	paths := args
	sessionID := m.appState.SessionID
	return func() tea.Msg {
		result, err := m.client.Upload(m.ctx, sessionID, paths)
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
	sessionID := m.appState.SessionID
	return func() tea.Msg {
		if err := m.client.Download(m.ctx, sessionID, remote, local); err != nil {
			return errMsg{err}
		}
		return sseEventMsg{Type: "__system__",
			Payload: map[string]any{"text": "Downloaded → " + local}}
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
	sessionID := m.appState.SessionID
	return func() tea.Msg {
		var patch apiclient.ConfigPatch
		switch key {
		case "max_steps":
			n := 0
			fmt.Sscanf(val, "%d", &n)
			if n > 0 {
				patch.MaxSteps = &n
			}
		case "allowed_tools":
			patch.AllowedTools = strings.Split(val, ",")
		default:
			return sseEventMsg{Type: "__system__",
				Payload: map[string]any{"text": fmt.Sprintf("Unknown config key %q. Supported: max_steps, allowed_tools", key)}}
		}
		if err := m.client.UpdateConfig(m.ctx, sessionID, patch); err != nil {
			return errMsg{err}
		}
		return sseEventMsg{Type: "__system__",
			Payload: map[string]any{"text": fmt.Sprintf("Config: %s = %s", key, val)}}
	}
}

// ── Rendering ─────────────────────────────────────────────────────

func (m *Model) renderEntries() string {
	var sb strings.Builder
	for _, e := range m.entries {
		switch e.kind {
		case entryUser:
			sb.WriteString(styleUser.Render("You") + "  " + e.text + "\n\n")
		case entryThought:
			sb.WriteString(styleThought.Render("  ↳ " + e.text) + "\n")
		case entryToolCall:
			sb.WriteString(styleToolCall.Render("  ⚙  "+e.text) + "\n")
		case entryToolResult:
			icon, style := "✅", styleSuccess
			if e.success != nil && !*e.success {
				icon, style = "❌", styleError
			}
			sb.WriteString(style.Render("  "+icon+"  "+e.text) + "\n\n")
		case entrySystem:
			sb.WriteString(styleSystem.Render(e.text) + "\n\n")
		}
	}
	return sb.String()
}

func (m *Model) renderStatusBar() string {
	sess := orNone(m.appState.SessionID)
	if len(sess) > 8 {
		sess = sess[:8] + "…"
	}
	state := m.statusMsg
	if m.agentRunning {
		state = m.spinner.View() + " " + state
	}
	bar := fmt.Sprintf("  session:%-10s  model:%-14s  %s  %-16s",
		sess, m.appState.Models.Execute, m.appState.APIURL, state)
	return styleStatus.Width(m.width).Render(bar)
}

func (m *Model) renderApproval() string {
	if m.approval == nil {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("⚠️  Approval Required\n\n")
	sb.WriteString(fmt.Sprintf("  Tool:   %s\n", m.approval.Tool))
	for k, v := range m.approval.Input {
		sb.WriteString(fmt.Sprintf("  %-8s %v\n", k+":", v))
	}
	sb.WriteString(fmt.Sprintf("\n  Risk: %s\n\n", m.approval.Risk))
	sb.WriteString("  [y] Approve    [n] Reject\n")
	return styleApproval.Render(sb.String())
}

// ── Helpers ───────────────────────────────────────────────────────

func (m *Model) pushEntry(e entry) { m.entries = append(m.entries, e) }

func (m *Model) pushSystem(text string) {
	m.entries = append(m.entries, entry{kind: entrySystem, text: text, ts: time.Now()})
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}
