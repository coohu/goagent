package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/coohu/goagent/internal/cli/apiclient"
	"github.com/coohu/goagent/internal/cli/daemon"
	"github.com/coohu/goagent/internal/cli/state"
	"github.com/coohu/goagent/internal/cli/tui"
	"github.com/spf13/cobra"
)

var (
	apiURL      string
	binaryPath  string
	noAutoStart bool
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "goagent",
	Short: "GoAgent CLI — interactive AI agent terminal",
	RunE:  runTUI,
}

var runCmd = &cobra.Command{
	Use:   "run <goal>",
	Short: "Run a single goal non-interactively and print output",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runOnce,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&apiURL, "api-url", "", "GoAgent server URL (default: http://127.0.0.1:8080)")
	rootCmd.PersistentFlags().StringVar(&binaryPath, "binary", "", "Path to goagent-server binary for auto-start")
	rootCmd.PersistentFlags().BoolVar(&noAutoStart, "no-auto-start", false, "Disable automatic server startup")
	rootCmd.AddCommand(runCmd)
}

// ── TUI mode ──────────────────────────────────────────────────────

func runTUI(_ *cobra.Command, _ []string) error {
	appState, err := state.Load()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}
	if apiURL != "" {
		appState.APIURL = apiURL
	}
	if appState.APIURL == "" {
		appState.APIURL = "http://127.0.0.1:8080"
	}

	if err := ensureServer(appState.APIURL); err != nil {
		return fmt.Errorf("server unavailable: %w", err)
	}

	client := apiclient.New(appState.APIURL)
	model := tui.New(client, appState)
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err = p.Run()
	return err
}

// ── One-shot mode ─────────────────────────────────────────────────

func runOnce(_ *cobra.Command, args []string) error {
	goal := strings.Join(args, " ")

	appState, _ := state.Load()
	if apiURL != "" {
		appState.APIURL = apiURL
	}
	if appState.APIURL == "" {
		appState.APIURL = "http://127.0.0.1:8080"
	}

	if err := ensureServer(appState.APIURL); err != nil {
		return fmt.Errorf("server unavailable: %w", err)
	}

	client := apiclient.New(appState.APIURL)
	ctx := context.Background()

	resp, err := client.Run(ctx, goal, nil)
	if err != nil {
		return fmt.Errorf("run: %w", err)
	}
	fmt.Printf("session: %s\n\n", resp.SessionID)

	evCh := make(chan apiclient.SSEEvent, 128)
	go func() {
		_ = client.StreamEvents(ctx, resp.SessionID, 0, evCh)
		close(evCh)
	}()

	for ev := range evCh {
		switch ev.Type {
		case "thought":
			if v, _ := ev.Payload["content"].(string); v != "" {
				fmt.Printf("  ↳ %s\n", v)
			}
		case "tool_call":
			tool, _ := ev.Payload["tool"].(string)
			input, _ := ev.Payload["input"].(string)
			fmt.Printf("  ⚙  [%s] %s\n", tool, input)
		case "tool_result":
			ok, _ := ev.Payload["success"].(bool)
			summary, _ := ev.Payload["summary"].(string)
			icon := "✅"
			if !ok {
				icon = "❌"
			}
			fmt.Printf("  %s %s\n", icon, summary)
		case "done":
			result, _ := ev.Payload["result"].(string)
			fmt.Printf("\n✅ Done: %s\n", result)
			return nil
		case "error":
			reason, _ := ev.Payload["reason"].(string)
			return fmt.Errorf("agent error: %s", reason)
		case "approval_required":
			tool, _ := ev.Payload["tool"].(string)
			fmt.Printf("\n⚠️  Approval required for: %s\nApprove? [y/N]: ", tool)
			var ans string
			fmt.Scanln(&ans)
			approved := ans == "y" || ans == "Y"
			_ = client.Approve(ctx, resp.SessionID, approved, "")
		}
	}
	return nil
}

// ── Auto-start ────────────────────────────────────────────────────

func ensureServer(serverURL string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := apiclient.New(serverURL).Health(ctx); err == nil {
		return nil
	}
	if noAutoStart {
		return fmt.Errorf("server at %s is unreachable (--no-auto-start)", serverURL)
	}
	host := strings.TrimPrefix(strings.TrimPrefix(serverURL, "http://"), "https://")
	return daemon.New(host, binaryPath).EnsureRunning(context.Background())
}
