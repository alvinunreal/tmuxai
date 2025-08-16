package internal

import (
	"fmt"
	"testing"

	"github.com/alvinunreal/tmuxai/config"
	"github.com/alvinunreal/tmuxai/system"
	"github.com/stretchr/testify/assert"
)

// Test /prepare command behavior with subshell
func TestProcessSubCommand_PrepareSubshell(t *testing.T) {
	manager := &Manager{
		Config:           &config.Config{MaxCaptureLines: 1000},
		SessionOverrides: make(map[string]any),
		Messages:         []ChatMessage{},
		ExecPane: &system.TmuxPaneDetails{
			Id:         "test-pane",
			IsSubShell: true, // This is a subshell
		},
	}

	// Mock system functions to prevent actual tmux calls
	originalTmuxSend := system.TmuxSendCommandToPane
	originalTmuxCapture := system.TmuxCapturePane
	originalTmuxCurrentPaneId := system.TmuxCurrentPaneId
	originalTmuxPanesDetails := system.TmuxPanesDetails
	defer func() {
		system.TmuxSendCommandToPane = originalTmuxSend
		system.TmuxCapturePane = originalTmuxCapture
		system.TmuxCurrentPaneId = originalTmuxCurrentPaneId
		system.TmuxPanesDetails = originalTmuxPanesDetails
	}()

	commandsSent := []string{}
	system.TmuxSendCommandToPane = func(paneId string, command string, enter bool) error {
		commandsSent = append(commandsSent, command)
		return nil
	}

	system.TmuxCapturePane = func(paneId string, maxLines int) (string, error) {
		return "", nil
	}

	// Mock the system functions used by GetTmuxPanes to return the test pane
	system.TmuxCurrentPaneId = func() (string, error) {
		return "main-pane", nil
	}
	system.TmuxPanesDetails = func(windowTarget string) ([]system.TmuxPaneDetails, error) {
		// Return the test pane as the only available pane
		return []system.TmuxPaneDetails{*manager.ExecPane}, nil
	}

	// Test case 1: /prepare with valid shell on subshell (should work and send commands)
	commandsSent = []string{} // Reset
	manager.ProcessSubCommand("/prepare bash")

	assert.Len(t, commandsSent, 2, "Should send PS1 command and clear command for bash")
	assert.Contains(t, commandsSent[0], "PS1=", "Should send bash PS1 command")
	assert.Equal(t, "C-l", commandsSent[1], "Should send clear screen command")

	// Test case 2: /prepare with zsh on subshell
	commandsSent = []string{} // Reset
	manager.ProcessSubCommand("/prepare zsh")

	assert.Len(t, commandsSent, 2, "Should send PROMPT command and clear command for zsh")
	assert.Contains(t, commandsSent[0], "PROMPT=", "Should send zsh PROMPT command")
	assert.Equal(t, "C-l", commandsSent[1], "Should send clear screen command")

	// Test case 3: /prepare with fish on subshell
	commandsSent = []string{} // Reset
	manager.ProcessSubCommand("/prepare fish")

	assert.Len(t, commandsSent, 2, "Should send fish_prompt function and clear command for fish")
	assert.Contains(t, commandsSent[0], "fish_prompt", "Should send fish prompt function")
	assert.Equal(t, "C-l", commandsSent[1], "Should send clear screen command")

	// Test case 4: /prepare without shell specification on subshell (should not send commands, just print warning)
	commandsSent = []string{} // Reset
	manager.ProcessSubCommand("/prepare")

	fmt.Println(commandsSent)
	assert.Len(t, commandsSent, 0, "Should not send commands when no shell specified on subshell (should show warning instead)")
}

// Test /prepare command behavior with normal shell (not subshell)
func TestProcessSubCommand_PrepareNormalShell(t *testing.T) {
	manager := &Manager{
		Config:           &config.Config{MaxCaptureLines: 1000},
		SessionOverrides: make(map[string]any),
		Messages:         []ChatMessage{},
		ExecPane: &system.TmuxPaneDetails{
			Id:             "test-pane",
			IsSubShell:     false,    // This is NOT a subshell
			CurrentCommand: "unknown", // Unsupported shell - should not send commands
		},
	}

	// Mock system functions to prevent actual tmux calls
	originalTmuxSend := system.TmuxSendCommandToPane
	originalTmuxCapture := system.TmuxCapturePane
	originalTmuxCurrentPaneId := system.TmuxCurrentPaneId
	originalTmuxPanesDetails := system.TmuxPanesDetails
	defer func() {
		system.TmuxSendCommandToPane = originalTmuxSend
		system.TmuxCapturePane = originalTmuxCapture
		system.TmuxCurrentPaneId = originalTmuxCurrentPaneId
		system.TmuxPanesDetails = originalTmuxPanesDetails
	}()

	commandsSent := []string{}
	system.TmuxSendCommandToPane = func(paneId string, command string, enter bool) error {
		commandsSent = append(commandsSent, command)
		return nil
	}

	system.TmuxCapturePane = func(paneId string, maxLines int) (string, error) {
		return "", nil
	}

	// Mock the system functions used by GetTmuxPanes to return the test pane
	system.TmuxCurrentPaneId = func() (string, error) {
		return "main-pane", nil
	}
	system.TmuxPanesDetails = func(windowTarget string) ([]system.TmuxPaneDetails, error) {
		// Return the test pane as the only available pane
		return []system.TmuxPaneDetails{*manager.ExecPane}, nil
	}

	// Test case 1: /prepare without shell specification when CurrentCommand is not a shell (should not send commands)
	commandsSent = []string{} // Reset
	manager.ProcessSubCommand("/prepare")

	assert.Len(t, commandsSent, 0, "Should not send commands when CurrentCommand is not a supported shell")

	// Test case 2: /prepare with explicit shell on normal shell (should work)
	commandsSent = []string{} // Reset
	manager.ProcessSubCommand("/prepare zsh")

	assert.Len(t, commandsSent, 2, "Should send commands when explicitly specifying shell on normal pane")
	assert.Contains(t, commandsSent[0], "PROMPT=", "Should send zsh PROMPT command")
	assert.Equal(t, "C-l", commandsSent[1], "Should send clear screen command")
}

// Test IsMessageSubcommand function
func TestIsMessageSubcommand(t *testing.T) {
	manager := &Manager{}

	// Test cases for command detection
	testCases := []struct {
		input    string
		expected bool
		desc     string
	}{
		{"/help", true, "Simple command should be detected"},
		{"/prepare bash", true, "Command with arguments should be detected"},
		{"  /info  ", true, "Command with whitespace should be detected"},
		{"/PREPARE", true, "Uppercase command should be detected"},
		{"hello world", false, "Regular message should not be detected as command"},
		{"", false, "Empty string should not be detected as command"},
		{"/ invalid", true, "Any string starting with / should be detected as command"},
	}

	for _, tc := range testCases {
		result := manager.IsMessageSubcommand(tc.input)
		assert.Equal(t, tc.expected, result, tc.desc)
	}
}
