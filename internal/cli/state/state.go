package state

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type State struct {
	SessionID   string            `json:"session_id"`
	APIURL      string            `json:"api_url"`
	Model       string            `json:"model"`
	FileMapping map[string]string `json:"file_mapping"` // local→remote
}

func defaultPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".goagent", "state.json")
}

func Load() (*State, error) {
	data, err := os.ReadFile(defaultPath())
	if err != nil {
		return &State{APIURL: "http://127.0.0.1:8080", Model: "gpt-4o", FileMapping: map[string]string{}}, nil
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	if s.FileMapping == nil {
		s.FileMapping = map[string]string{}
	}
	return &s, nil
}

func (s *State) Save() error {
	path := defaultPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func (s *State) ClearSession() {
	s.SessionID = ""
	s.FileMapping = map[string]string{}
}
