package system

import (
	"regexp"
	"strings"
	"testing"
)

// inlineRe mirrors the inline-code regex used by Cosmetics/processInlineCode.
func inlineRe() *regexp.Regexp {
	return regexp.MustCompile("`([^`]+)`")
}

// withTheme runs fn with the given theme active, restoring the previous theme
// afterwards so the global state does not leak into other tests.
func withTheme(t *testing.T, want string, fn func()) {
	t.Helper()
	prev := theme // capture the current global before shadowing it
	defer SetTheme(prev)
	SetTheme(want)
	fn()
}

func TestSetThemeIsLightTheme(t *testing.T) {
	prev := theme
	defer SetTheme(prev)

	SetTheme("light")
	if !IsLightTheme() {
		t.Errorf("IsLightTheme() = false, want true after SetTheme(\"light\")")
	}
	SetTheme("dark")
	if IsLightTheme() {
		t.Errorf("IsLightTheme() = true, want false after SetTheme(\"dark\")")
	}
	// Unknown values fall back to dark.
	SetTheme("solarized")
	if IsLightTheme() {
		t.Errorf("IsLightTheme() = true, want false for unknown theme")
	}
	// Case-insensitive.
	SetTheme("LIGHT")
	if !IsLightTheme() {
		t.Errorf("IsLightTheme() = false, want true for case-insensitive \"LIGHT\"")
	}
}

func TestHighlightCodeThemeChangesOutput(t *testing.T) {
	const code = "echo hello && ls -la"

	var darkOut, lightOut string
	withTheme(t, "dark", func() {
		out, err := HighlightCode("sh", code)
		if err != nil {
			t.Fatalf("HighlightCode (dark) error: %v", err)
		}
		darkOut = out
	})
	withTheme(t, "light", func() {
		out, err := HighlightCode("sh", code)
		if err != nil {
			t.Fatalf("HighlightCode (light) error: %v", err)
		}
		lightOut = out
	})

	// Both must be valid highlighting (contain ANSI codes and the code text).
	if stripANSI(darkOut) != stripANSI(lightOut) {
		t.Errorf("plain text differs between themes:\n dark=%q\n light=%q", stripANSI(darkOut), stripANSI(lightOut))
	}
	// The two themes must produce different color codes (the whole point of the switch).
	if darkOut == lightOut {
		t.Errorf("expected different ANSI output for dark vs light theme, got identical output: %q", darkOut)
	}
}

func TestProcessInlineCodeTheme(t *testing.T) {
	const input = "run `ls -la` now"

	var darkOut, lightOut string
	withTheme(t, "dark", func() { darkOut = processInlineCode(input, inlineRe()) })
	withTheme(t, "light", func() { lightOut = processInlineCode(input, inlineRe()) })

	// Dark theme uses the dark-blue background (48;5;235).
	if !strings.Contains(darkOut, "\033[48;5;235m") {
		t.Errorf("dark theme inline code missing dark-blue bg, got %q", darkOut)
	}
	// Light theme uses the light-gray background (48;5;254) and must NOT use the dark-blue bg.
	if !strings.Contains(lightOut, "\033[48;5;254m") {
		t.Errorf("light theme inline code missing light-gray bg, got %q", lightOut)
	}
	if strings.Contains(lightOut, "\033[48;5;235m") {
		t.Errorf("light theme inline code should not use dark-blue bg, got %q", lightOut)
	}
	// The code text itself must survive in both.
	if stripANSI(darkOut) != stripANSI(lightOut) {
		t.Errorf("plain text differs between themes:\n dark=%q\n light=%q", stripANSI(darkOut), stripANSI(lightOut))
	}
}