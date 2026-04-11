package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaultsWhenMissing(t *testing.T) {
	dir := t.TempDir()
	orig := os.Getenv("HOME")
	os.Setenv("HOME", dir)
	defer os.Setenv("HOME", orig)

	s, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.APIURL != "http://127.0.0.1:8080" {
		t.Errorf("wrong default APIURL: %s", s.APIURL)
	}
	if s.Model != "gpt-4o" {
		t.Errorf("wrong default model: %s", s.Model)
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	orig := os.Getenv("HOME")
	os.Setenv("HOME", dir)
	defer os.Setenv("HOME", orig)

	s := &State{
		SessionID:   "sess-abc",
		APIURL:      "http://localhost:9090",
		Model:       "gpt-4o-mini",
		FileMapping: map[string]string{"local.txt": "remote/local.txt"},
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
	if loaded.FileMapping["local.txt"] != "remote/local.txt" {
		t.Error("file mapping not preserved")
	}
}

func TestClearSession(t *testing.T) {
	s := &State{SessionID: "old", FileMapping: map[string]string{"a": "b"}}
	s.ClearSession()
	if s.SessionID != "" {
		t.Error("SessionID should be cleared")
	}
	if len(s.FileMapping) != 0 {
		t.Error("FileMapping should be cleared")
	}
}

func TestStateFilePermissions(t *testing.T) {
	dir := t.TempDir()
	orig := os.Getenv("HOME")
	os.Setenv("HOME", dir)
	defer os.Setenv("HOME", orig)

	s := &State{APIURL: "http://x", Model: "m", FileMapping: map[string]string{}}
	_ = s.Save()

	info, err := os.Stat(filepath.Join(dir, ".goagent", "state.json"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("expected 0600, got %o", info.Mode().Perm())
	}
}
