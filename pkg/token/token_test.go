package token

import "testing"

func TestEstimateCount(t *testing.T) {
	cases := []struct {
		input    string
		minCount int
	}{
		{"hello world", 2},
		{"", 0},
		{"the quick brown fox jumps over the lazy dog", 9},
	}
	for _, c := range cases {
		got := EstimateCount(c.input)
		if got < c.minCount {
			t.Errorf("input=%q: expected at least %d tokens, got %d", c.input, c.minCount, got)
		}
	}
}

func TestTruncateToTokens(t *testing.T) {
	long := "one two three four five six seven eight nine ten"
	truncated := TruncateToTokens(long, 3)
	if len(truncated) >= len(long) {
		t.Error("truncated text should be shorter than original")
	}
}

func TestTruncateToTokensNoTruncationNeeded(t *testing.T) {
	short := "hello world"
	result := TruncateToTokens(short, 1000)
	if result != short {
		t.Errorf("should not truncate short text, got %q", result)
	}
}
