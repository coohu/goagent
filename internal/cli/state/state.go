package state

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// SceneModels mirrors core.SceneModels without importing server packages.
type SceneModels struct {
	Planning  string `json:"planning"`
	Execute   string `json:"execute"`
	Summarize string `json:"summarize"`
	Reflect   string `json:"reflect"`
}

func (s *SceneModels) All() string {
	return s.Planning
}

func DefaultModels() SceneModels {
	return SceneModels{
		Planning:  "gpt-4o",
		Execute:   "gpt-4o",
		Summarize: "gpt-4o-mini",
		Reflect:   "gpt-4o-mini",
	}
}

type State struct {
	SessionID   string            `json:"session_id"`
	APIURL      string            `json:"api_url"`
	Models      SceneModels       `json:"models"`
	FileMapping map[string]string `json:"file_mapping"`
}

func defaultPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".goagent", "state.json")
}

func Load() (*State, error) {
	data, err := os.ReadFile(defaultPath())
	if err != nil {
		return &State{
			APIURL:      "http://127.0.0.1:8080",
			Models:      DefaultModels(),
			FileMapping: map[string]string{},
		}, nil
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	if s.FileMapping == nil {
		s.FileMapping = map[string]string{}
	}
	if s.Models.Planning == "" {
		s.Models = DefaultModels()
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
