package cmdparser

import "testing"

func TestParseGoal(t *testing.T) {
	cases := []string{
		"write a hello world program",
		"fix the nil pointer in main.go",
		"  spaces around  ",
	}
	for _, input := range cases {
		cmd := Parse(input)
		if cmd.Kind != KindGoal {
			t.Errorf("expected KindGoal for %q, got slash", input)
		}
	}
}

func TestParseSlash(t *testing.T) {
	cases := []struct {
		input string
		slash string
		nArgs int
	}{
		{"/help", "help", 0},
		{"/model gpt-4o", "model", 1},
		{"/session list", "session", 1},
		{"/upload ./file.txt", "upload", 1},
		{"/config max_steps 50", "config", 2},
	}
	for _, c := range cases {
		cmd := Parse(c.input)
		if cmd.Kind != KindSlash {
			t.Errorf("%q: expected KindSlash", c.input)
			continue
		}
		if cmd.Slash != c.slash {
			t.Errorf("%q: expected slash %q, got %q", c.input, c.slash, cmd.Slash)
		}
		if len(cmd.Args) != c.nArgs {
			t.Errorf("%q: expected %d args, got %d", c.input, c.nArgs, len(cmd.Args))
		}
	}
}

func TestParseEmpty(t *testing.T) {
	cmd := Parse("")
	if cmd.Kind != KindGoal {
		t.Error("empty input should be KindGoal")
	}
}

func TestKnown(t *testing.T) {
	known := []string{"model", "clear", "session", "help", "exit", "upload", "download", "config"}
	for _, k := range known {
		if !Known(k) {
			t.Errorf("%q should be known", k)
		}
	}
	if Known("nonexistent") {
		t.Error("nonexistent should not be known")
	}
}
