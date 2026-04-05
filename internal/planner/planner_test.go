package planner

import (
	"testing"

	"github.com/coohu/goagent/internal/core"
)

func TestParseSteps_ValidJSON(t *testing.T) {
	raw := `[
		{"id": 1, "name": "Read file", "description": "Read main.go", "tool": "file.read", "tool_input": {"path": "main.go"}},
		{"id": 2, "name": "Write output", "description": "Write result", "tool": "file.write"}
	]`

	steps, err := parseSteps(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(steps))
	}
	if steps[0].Name != "Read file" {
		t.Errorf("expected 'Read file', got %q", steps[0].Name)
	}
	if steps[0].Tool != "file.read" {
		t.Errorf("expected 'file.read', got %q", steps[0].Tool)
	}
	if steps[0].Status != core.StepPending {
		t.Errorf("new step should be pending, got %s", steps[0].Status)
	}
}

func TestParseSteps_WrappedInMarkdown(t *testing.T) {
	raw := "```json\n[\n  {\"id\": 1, \"name\": \"step1\", \"tool\": \"shell.exec\"}\n]\n```"

	steps, err := parseSteps(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(steps) != 1 {
		t.Errorf("expected 1 step, got %d", len(steps))
	}
}

func TestParseSteps_EmptyResponseError(t *testing.T) {
	_, err := parseSteps("no json here")
	if err == nil {
		t.Error("expected error for non-JSON response")
	}
}

func TestParseSteps_AutoAssignsIDIfMissing(t *testing.T) {
	raw := `[{"name": "step one", "tool": "file.read"}, {"name": "step two", "tool": "file.write"}]`

	steps, err := parseSteps(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if steps[0].ID != 1 {
		t.Errorf("expected ID 1, got %d", steps[0].ID)
	}
	if steps[1].ID != 2 {
		t.Errorf("expected ID 2, got %d", steps[1].ID)
	}
}
