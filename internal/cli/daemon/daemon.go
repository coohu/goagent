package daemon

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"time"
)

type Manager struct {
	addr       string
	binaryPath string
	proc       *exec.Cmd
}

func New(addr, binaryPath string) *Manager {
	return &Manager{addr: addr, binaryPath: binaryPath}
}

// EnsureRunning checks if the server is reachable; if not, starts it.
func (m *Manager) EnsureRunning(ctx context.Context) error {
	if m.isAlive() {
		return nil
	}
	return m.start(ctx)
}

func (m *Manager) isAlive() bool {
	conn, err := net.DialTimeout("tcp", m.addr, 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func (m *Manager) start(ctx context.Context) error {
	bin := m.binaryPath
	if bin == "" {
		var err error
		bin, err = os.Executable()
		if err != nil {
			return fmt.Errorf("cannot find binary: %w", err)
		}
	}

	if _, err := os.Stat(bin); err != nil {
		return fmt.Errorf("server binary not found at %s", bin)
	}

	cmd := exec.CommandContext(ctx, bin, "serve")
	cmd.Env = os.Environ()
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}
	m.proc = cmd

	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if m.isAlive() {
			return nil
		}
		time.Sleep(300 * time.Millisecond)
	}
	return fmt.Errorf("server did not become ready within 8s")
}

func (m *Manager) Stop() {
	if m.proc != nil && m.proc.Process != nil {
		m.proc.Process.Kill()
	}
}
