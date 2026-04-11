package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaultsWhenMissing(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("HOME", dir)
	defer os.Unsetenv("HOME")

	s, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.APIURL != "http://127.0.0.1:8080" {
		t.Errorf("wrong default APIURL: %s", s.APIURL)
	}
	if s.Models.Planning != "gpt-4o" {
		t.Errorf("wrong default planning model: %s", s.Models.Planning)
	}
	if s.Models.Summarize != "gpt-4o-mini" {
		t.Errorf("wrong default summarize model: %s", s.Models.Summarize)
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("HOME", dir)
	defer os.Unsetenv("HOME")

	s := &State{
		SessionID: "sess-abc",
		APIURL:    "http://localhost:9090",
		Models: SceneModels{
			Planning:  "gpt-4o",
			Execute:   "gpt-4o-mini",
			Summarize: "gpt-4o-mini",
			Reflect:   "qwen-plus",
		},
		FileMapping: map[string]string{"a.txt": "remote/a.txt"},
	}
	if err := s.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.SessionID != "sess-abc" {
		t.Errorf("wrong session ID: %s", loaded.SessionID)
	}
	if loaded.Models.Execute != "gpt-4o-mini" {
		t.Errorf("wrong execute model: %s", loaded.Models.Execute)
	}
	if loaded.Models.Reflect != "qwen-plus" {
		t.Errorf("wrong reflect model: %s", loaded.Models.Reflect)
	}
	if loaded.FileMapping["a.txt"] != "remote/a.txt" {
		t.Error("file mapping not preserved")
	}
}

func TestClearSession(t *testing.T) {
	s := &State{
		SessionID:   "old",
		Models:      DefaultModels(),
		FileMapping: map[string]string{"x": "y"},
	}
	s.ClearSession()
	if s.SessionID != "" {
		t.Error("SessionID should be cleared")
	}
	if len(s.FileMapping) != 0 {
		t.Error("FileMapping should be cleared")
	}
	// Models should be preserved across session clear
	if s.Models.Planning == "" {
		t.Error("Models should not be cleared by ClearSession")
	}
}

func TestStateFilePermissions(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("HOME", dir)
	defer os.Unsetenv("HOME")

	s := &State{APIURL: "http://x", Models: DefaultModels(), FileMapping: map[string]string{}}
	_ = s.Save()

	info, err := os.Stat(filepath.Join(dir, ".goagent", "state.json"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("expected 0600, got %o", info.Mode().Perm())
	}
}

func TestMigratesLegacyState(t *testing.T) {
	dir := t.TempDir()
	os.Setenv("HOME", dir)
	defer os.Unsetenv("HOME")

	// Write a legacy state with no models field
	legacyJSON := `{"session_id":"s1","api_url":"http://x","file_mapping":{}}`
	statePath := filepath.Join(dir, ".goagent", "state.json")
	os.MkdirAll(filepath.Dir(statePath), 0700)
	os.WriteFile(statePath, []byte(legacyJSON), 0600)

	s, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if s.Models.Planning == "" {
		t.Error("should migrate empty Models to defaults")
	}
	if s.Models.Planning != "gpt-4o" {
		t.Errorf("wrong migrated planning model: %s", s.Models.Planning)
	}
}
