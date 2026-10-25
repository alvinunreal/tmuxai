package system

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsShell_RecognizesKnownShells(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish", "sh", "dash", "ksh"} {
		assert.True(t, IsShell(shell), "IsShell(%q) should be true", shell)
	}
}

func TestIsShell_RejectsNonShells(t *testing.T) {
	for _, cmd := range []string{"copilot", "vim", "htop", "tmuxai", "node", "python3"} {
		assert.False(t, IsShell(cmd), "IsShell(%q) should be false", cmd)
	}
}

func TestIsShell_EmptyCommand(t *testing.T) {
	assert.False(t, IsShell(""), "IsShell(\"\") should be false")
}