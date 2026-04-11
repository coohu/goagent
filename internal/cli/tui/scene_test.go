package tui

import "testing"

func TestNormalizeScene(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"plan", "planning"},
		{"planning", "planning"},
		{"PLAN", "planning"},
		{"exec", "execute"},
		{"execute", "execute"},
		{"EXEC", "execute"},
		{"sum", "summarize"},
		{"summarize", "summarize"},
		{"summary", "summarize"},
		{"reflect", "reflect"},
		{"reflection", "reflect"},
		{"REFLECT", "reflect"},
		{"unknown", ""},
		{"gpt-4o", ""},    // model id, not a scene
		{"", ""},
	}
	for _, c := range cases {
		got := normalizeScene(c.input)
		if got != c.want {
			t.Errorf("normalizeScene(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}
