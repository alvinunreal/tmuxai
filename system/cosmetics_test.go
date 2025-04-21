package system

import (
	"regexp"
	"strings"
	"testing"
)

func stripANSI(s string) string {
	re := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	return re.ReplaceAllString(s, "")
}

func TestCosmetics(t *testing.T) {
	ansiPattern := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	tests := []struct {
		name     string
		input    string
		expected []string // substrings expected in output (stripped of ANSI)
	}{
		{
			name:  "inline code",
			input: "This is `inline` code.",
			expected: []string{
				"inline",
			},
		},
		{
			name:  "code block",
			input: "Here is a code block:\n```go\nfmt.Println(\"hi\")\n```",
			expected: []string{
				"fmt", "Println", "\"hi\"",
			},
		},
		{
			name:  "mixed",
			input: "Mix `inline` and block:\n```python\nprint('hi')\n``` end.",
			expected: []string{
				"inline",
				"print", "'hi'",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := Cosmetics(tt.input)
			if tt.name != "inline code" {
				// For code blocks, ensure ANSI codes are present
				if !ansiPattern.MatchString(out) {
					t.Errorf("expected ANSI escape codes in output, got %q", out)
				}
			}
			plain := stripANSI(out)
			for _, want := range tt.expected {
				if !strings.Contains(plain, want) {
					t.Errorf("expected output to contain %q, got %q", want, plain)
				}
			}
		})
	}
}
