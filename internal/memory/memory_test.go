package memory

import (
	"context"
	"testing"
	"time"

	"github.com/coohu/goagent/internal/core"
)

func TestSaveAndSearchToolMemory(t *testing.T) {
	m := NewInMemoryManager()
	ctx := context.Background()

	mem := &core.ToolMemory{
		ID:        "m1",
		SessionID: "s1",
		StepID:    1,
		ToolName:  "file.read",
		Summary:   "read the main go file containing the entry point",
		KeyPoints: []string{"entry point in main.go", "uses gin framework"},
		CreatedAt: time.Now(),
	}
	if err := m.SaveToolMemory(ctx, mem); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	results, err := m.SearchToolMemory(ctx, "main.go entry point", "s1", 5)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected at least one result")
	}
	if results[0].ID != "m1" {
		t.Errorf("expected m1, got %s", results[0].ID)
	}
}

func TestSearchToolMemoryIsolatedBySession(t *testing.T) {
	m := NewInMemoryManager()
	ctx := context.Background()

	m.SaveToolMemory(ctx, &core.ToolMemory{
		ID: "m1", SessionID: "s1", Summary: "golang gin framework", CreatedAt: time.Now(),
	})
	m.SaveToolMemory(ctx, &core.ToolMemory{
		ID: "m2", SessionID: "s2", Summary: "golang gin framework", CreatedAt: time.Now(),
	})

	results, _ := m.SearchToolMemory(ctx, "gin", "s1", 10)
	if len(results) != 1 || results[0].ID != "m1" {
		t.Errorf("expected only s1 memory, got %v", results)
	}
}

func TestConversation(t *testing.T) {
	m := NewInMemoryManager()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		m.AppendMessage(ctx, "s1", &core.Message{Role: "user", Content: "msg"})
	}

	msgs, err := m.GetConversation(ctx, "s1", 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 3 {
		t.Errorf("expected 3 (limit), got %d", len(msgs))
	}
}

func TestClearSession(t *testing.T) {
	m := NewInMemoryManager()
	ctx := context.Background()

	m.SaveToolMemory(ctx, &core.ToolMemory{ID: "m1", SessionID: "s1", Summary: "test", CreatedAt: time.Now()})
	m.AppendMessage(ctx, "s1", &core.Message{Role: "user", Content: "hi"})

	m.ClearSession(ctx, "s1")

	results, _ := m.SearchToolMemory(ctx, "test", "s1", 10)
	if len(results) != 0 {
		t.Error("tool memories should be cleared")
	}

	msgs, _ := m.GetConversation(ctx, "s1", 10)
	if len(msgs) != 0 {
		t.Error("conversation should be cleared")
	}
}

func TestSaveAndSearchEpisodes(t *testing.T) {
	m := NewInMemoryManager()
	ctx := context.Background()

	m.SaveEpisode(ctx, &core.Episode{
		ID:      "e1",
		Goal:    "write a REST API in golang",
		Success: true,
		Summary: "created gin server with CRUD endpoints",
	})

	results, err := m.SearchEpisodes(ctx, "golang REST", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) == 0 || results[0].ID != "e1" {
		t.Error("expected episode e1")
	}
}
