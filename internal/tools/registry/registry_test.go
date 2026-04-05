package registry

import (
	"context"
	"testing"

	"github.com/coohu/goagent/internal/core"
)

type mockTool struct{ name string }

func (m *mockTool) Name() string        { return m.name }
func (m *mockTool) Description() string { return "mock " + m.name }
func (m *mockTool) Schema() core.ToolSchema {
	return core.ToolSchema{Name: m.name, Description: m.Description()}
}
func (m *mockTool) Validate(_ map[string]any) error { return nil }
func (m *mockTool) Execute(_ context.Context, _ map[string]any) (*core.ToolResult, error) {
	return &core.ToolResult{Success: true}, nil
}

func TestRegisterAndGet(t *testing.T) {
	r := New()
	r.Register(&mockTool{"file.read"})

	tool, err := r.Get("file.read")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tool.Name() != "file.read" {
		t.Errorf("wrong tool name: %s", tool.Name())
	}
}

func TestGetNotFound(t *testing.T) {
	r := New()
	_, err := r.Get("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent tool")
	}
}

func TestListAllowed(t *testing.T) {
	r := New()
	r.Register(&mockTool{"file.read"})
	r.Register(&mockTool{"file.write"})
	r.Register(&mockTool{"shell.exec"})

	allowed := r.ListAllowed([]string{"file.read", "file.write"})
	if len(allowed) != 2 {
		t.Errorf("expected 2 allowed tools, got %d", len(allowed))
	}
}

func TestListAllowedEmptyReturnsAll(t *testing.T) {
	r := New()
	r.Register(&mockTool{"file.read"})
	r.Register(&mockTool{"file.write"})

	all := r.ListAllowed(nil)
	if len(all) != 2 {
		t.Errorf("expected 2 tools, got %d", len(all))
	}
}

func TestSchemas(t *testing.T) {
	r := New()
	r.Register(&mockTool{"file.read"})
	r.Register(&mockTool{"shell.exec"})

	schemas := r.Schemas([]string{"file.read"})
	if len(schemas) != 1 {
		t.Errorf("expected 1 schema, got %d", len(schemas))
	}
	if schemas[0].Name != "file.read" {
		t.Errorf("wrong schema name: %s", schemas[0].Name)
	}
}
