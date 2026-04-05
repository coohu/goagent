package file

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestReadTool(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello agent"), 0644)

	tool := NewReadTool()

	result, err := tool.Execute(context.Background(), map[string]any{"path": path})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got: %s", result.Stderr)
	}
	if result.Stdout != "hello agent" {
		t.Errorf("expected 'hello agent', got %q", result.Stdout)
	}
}

func TestReadToolMissingFile(t *testing.T) {
	tool := NewReadTool()
	result, err := tool.Execute(context.Background(), map[string]any{"path": "/nonexistent/file.txt"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure for missing file")
	}
}

func TestWriteTool(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "out.txt")

	tool := NewWriteTool()
	result, err := tool.Execute(context.Background(), map[string]any{
		"path":    path,
		"content": "written by agent",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success: %s", result.Stderr)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "written by agent" {
		t.Errorf("file content mismatch: %q", string(data))
	}
}

func TestListTool(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte(""), 0644)
	os.WriteFile(filepath.Join(dir, "b.go"), []byte(""), 0644)
	os.MkdirAll(filepath.Join(dir, "sub"), 0755)

	tool := NewListTool()
	result, err := tool.Execute(context.Background(), map[string]any{"path": dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success: %s", result.Stderr)
	}
	if result.Stdout == "" {
		t.Error("expected non-empty listing")
	}
}

func TestSearchTool(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "util.go"), []byte("package main\n\nfunc helper() {}\n"), 0644)

	tool := NewSearchTool()
	result, err := tool.Execute(context.Background(), map[string]any{
		"path":    dir,
		"pattern": "func main",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success")
	}
	if result.Stdout == "no matches found" {
		t.Error("expected to find 'func main'")
	}
}

func TestValidate(t *testing.T) {
	tool := NewReadTool()

	if err := tool.Validate(map[string]any{"path": "file.go"}); err != nil {
		t.Errorf("expected no error for valid input: %v", err)
	}
	if err := tool.Validate(map[string]any{}); err == nil {
		t.Error("expected error for missing path")
	}
}
