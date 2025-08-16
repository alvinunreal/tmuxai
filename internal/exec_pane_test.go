package internal

import (
	"fmt"
	"strings"
	"testing"

	"github.com/alvinunreal/tmuxai/config"
	"github.com/alvinunreal/tmuxai/system"
	"github.com/stretchr/testify/assert"
)

// Test regex matching for bash shell prompts
func TestParseExecPaneCommandHistory_Prompt(t *testing.T) {
	manager := &Manager{
		ExecHistory:      []CommandExecHistory{},
		Config:           &config.Config{MaxCaptureLines: 1000},
		SessionOverrides: make(map[string]interface{}),
	}

	// Mock exec pane content with bash-style prompts
	manager.ExecPane = &system.TmuxPaneDetails{}
	testContent := `user@hostname:/path[14:30][0]» ls -la
total 8
drwxr-xr-x  3 user user 4096 Jan 1 14:30 .
drwxr-xr-x 15 user user 4096 Jan 1 14:29 ..
user@hostname:/path[14:31][0]» echo "hello world"
hello world
user@hostname:/path[14:31][0]» `

	manager.parseExecPaneCommandHistoryWithContent(testContent)

	assert.Len(t, manager.ExecHistory, 2, "Should parse 2 commands from bash prompt")

	// First command: ls -la
	assert.Equal(t, "ls -la", manager.ExecHistory[0].Command)
	assert.Equal(t, 0, manager.ExecHistory[0].Code)
	assert.Contains(t, manager.ExecHistory[0].Output, "total 8")
	assert.Contains(t, manager.ExecHistory[0].Output, "drwxr-xr-x")

	// Second command: echo "hello world"
	assert.Equal(t, "echo \"hello world\"", manager.ExecHistory[1].Command)
	assert.Equal(t, 0, manager.ExecHistory[1].Code)
	assert.Equal(t, "hello world", manager.ExecHistory[1].Output)
}

// Test edge cases and malformed prompts
func TestParseExecPaneCommandHistory_EdgeCases(t *testing.T) {
	manager := &Manager{
		ExecHistory:      []CommandExecHistory{},
		Config:           &config.Config{MaxCaptureLines: 1000},
		SessionOverrides: make(map[string]interface{}),
	}

	// Test with no valid prompts (should result in empty history)
	manager.ExecPane = &system.TmuxPaneDetails{}
	testContent1 := `some random output
without any valid prompts
just plain text`

	manager.parseExecPaneCommandHistoryWithContent(testContent1)
	assert.Len(t, manager.ExecHistory, 0, "Should parse 0 commands from invalid prompt format")

	// Test with only status code, no command
	manager.ExecHistory = []CommandExecHistory{} // Reset
	testContent2 := `user@hostname:~[14:30][0]» 
user@hostname:~[14:30][0]» `

	manager.parseExecPaneCommandHistoryWithContent(testContent2)
	assert.Len(t, manager.ExecHistory, 0, "Should handle prompts with no commands")

	// Test with command but no terminating prompt (incomplete session)
	manager.ExecHistory = []CommandExecHistory{} // Reset
	testContent3 := `user@hostname:~[14:30][0]» long-running-command
output line 1
output line 2
still running...`

	manager.parseExecPaneCommandHistoryWithContent(testContent3)
	assert.Len(t, manager.ExecHistory, 1, "Should handle incomplete commands")
	assert.Equal(t, "long-running-command", manager.ExecHistory[0].Command)
	assert.Equal(t, -1, manager.ExecHistory[0].Code) // No terminating prompt means unknown status
	assert.Contains(t, manager.ExecHistory[0].Output, "output line 1")
	assert.Contains(t, manager.ExecHistory[0].Output, "still running...")
}

// Test mixed shell prompt formats (edge case where prompts might vary)
func TestParseExecPaneCommandHistory_MixedFormats(t *testing.T) {
	manager := &Manager{
		ExecHistory:      []CommandExecHistory{},
		Config:           &config.Config{MaxCaptureLines: 1000},
		SessionOverrides: make(map[string]interface{}),
	}

	// Mix of different time formats and variations
	manager.ExecPane = &system.TmuxPaneDetails{}
	testContent := `user@host:/tmp[09:15][0]» echo "test1"
test1
different@machine:/home[23:59][1]» echo "test2"
test2
user@localhost:~[00:00][0]» `

	manager.parseExecPaneCommandHistoryWithContent(testContent)

	assert.Len(t, manager.ExecHistory, 2, "Should parse commands from mixed prompt formats")
	assert.Equal(t, "echo \"test1\"", manager.ExecHistory[0].Command)
	assert.Equal(t, 1, manager.ExecHistory[0].Code) // Status from next prompt
	assert.Equal(t, "echo \"test2\"", manager.ExecHistory[1].Command)
	assert.Equal(t, 0, manager.ExecHistory[1].Code)
}

// Test PrepareExecPaneWithShell for different shells
func TestPrepareExecPaneWithShell(t *testing.T) {
	manager := &Manager{
		Config:           &config.Config{MaxCaptureLines: 1000},
		SessionOverrides: make(map[string]interface{}),
		ExecPane: &system.TmuxPaneDetails{
			Id:         "test-pane",
			IsPrepared: false,
			Shell:      "",
		},
	}

	// Mock system functions to prevent actual tmux calls
	originalTmuxSend := system.TmuxSendCommandToPane
	originalTmuxCapture := system.TmuxCapturePane
	defer func() {
		system.TmuxSendCommandToPane = originalTmuxSend
		system.TmuxCapturePane = originalTmuxCapture
	}()

	var commandsSent []string
	system.TmuxSendCommandToPane = func(paneId string, command string, enter bool) error {
		commandsSent = append(commandsSent, command)
		return nil
	}

	system.TmuxCapturePane = func(paneId string, maxLines int) (string, error) {
		return "", nil
	}

	// Test bash shell preparation
	manager.PrepareExecPaneWithShell("bash")
	assert.Len(t, commandsSent, 2, "Should send 2 commands for bash")
	assert.Contains(t, commandsSent[0], "PS1=", "Should set PS1 for bash")
	assert.Equal(t, "C-l", commandsSent[1], "Should clear screen")

	// Reset and test zsh shell preparation
	commandsSent = []string{}
	manager.PrepareExecPaneWithShell("zsh")
	assert.Len(t, commandsSent, 2, "Should send 2 commands for zsh")
	assert.Contains(t, commandsSent[0], "PROMPT=", "Should set PROMPT for zsh")
	assert.Equal(t, "C-l", commandsSent[1], "Should clear screen")

	// Reset and test fish shell preparation
	commandsSent = []string{}
	manager.PrepareExecPaneWithShell("fish")
	assert.Len(t, commandsSent, 2, "Should send 2 commands for fish")
	assert.Contains(t, commandsSent[0], "fish_prompt", "Should set fish_prompt for fish")
	assert.Equal(t, "C-l", commandsSent[1], "Should clear screen")

	// Reset and test unsupported shell
	commandsSent = []string{}
	manager.PrepareExecPaneWithShell("tcsh")
	assert.Len(t, commandsSent, 0, "Should not send commands for unsupported shell")
}

// Test prompt regex with error cases that should be handled gracefully
func TestParseExecPaneCommandHistory_ErrorHandling(t *testing.T) {
	manager := &Manager{
		ExecHistory:      []CommandExecHistory{},
		Config:           &config.Config{MaxCaptureLines: 1000},
		SessionOverrides: make(map[string]interface{}),
	}

	// Test with commands containing special characters and complex outputs
	manager.ExecPane = &system.TmuxPaneDetails{}
	testContent := `user@hostname:~[14:30][0]» echo "hello world" && ls -la | grep test
hello world
-rw-r--r-- 1 user user 123 Jan 1 14:30 test.txt
user@hostname:~[14:31][1]» false && echo "this should not appear"
user@hostname:~[14:31][0]» `

	manager.parseExecPaneCommandHistoryWithContent(testContent)
	assert.Len(t, manager.ExecHistory, 2, "Should parse complex commands with pipes and operators")
	assert.Equal(t, `echo "hello world" && ls -la | grep test`, manager.ExecHistory[0].Command)
	assert.Equal(t, 1, manager.ExecHistory[0].Code, "Should capture exit code from next prompt")
	assert.Contains(t, manager.ExecHistory[0].Output, "hello world")
	assert.Contains(t, manager.ExecHistory[0].Output, "test.txt")

	assert.Equal(t, `false && echo "this should not appear"`, manager.ExecHistory[1].Command)
	assert.Equal(t, 0, manager.ExecHistory[1].Code)

	// Test with very long commands and outputs
	manager.ExecHistory = []CommandExecHistory{} // Reset
	longCommand := strings.Repeat("very-long-command-", 10)
	longOutput := strings.Repeat("very long output line ", 50)
	testContent2 := fmt.Sprintf(`user@hostname:~[14:30][0]» %s
%s
user@hostname:~[14:31][0]» `, longCommand, longOutput)

	manager.parseExecPaneCommandHistoryWithContent(testContent2)
	assert.Len(t, manager.ExecHistory, 1, "Should handle long commands and outputs")
	assert.Equal(t, longCommand, manager.ExecHistory[0].Command)
	assert.Contains(t, manager.ExecHistory[0].Output, "very long output line")
	assert.Equal(t, 0, manager.ExecHistory[0].Code)
}

// Test SSH scenario where prompt format might differ and cause parsing issues
func TestExecWaitCapture_SSHScenario(t *testing.T) {
	manager := &Manager{
		ExecHistory:      []CommandExecHistory{},
		Config:           &config.Config{MaxCaptureLines: 1000},
		SessionOverrides: make(map[string]interface{}),
		Status:           "running",
		ExecPane: &system.TmuxPaneDetails{
			Id:       "ssh-pane",
			LastLine: "user@remote-server:~$ ", // SSH prompt without proper formatting
		},
	}

	// Mock system functions to simulate SSH environment
	originalTmuxSend := system.TmuxSendCommandToPane
	originalTmuxCapture := system.TmuxCapturePane
	defer func() { 
		system.TmuxSendCommandToPane = originalTmuxSend
		system.TmuxCapturePane = originalTmuxCapture
	}()

	commandSent := ""
	refreshCount := 0
	system.TmuxSendCommandToPane = func(paneId string, command string, enter bool) error {
		commandSent = command
		return nil
	}

	// Mock SSH server output without proper prompt format
	system.TmuxCapturePane = func(paneId string, maxLines int) (string, error) {
		refreshCount++
		// After a few refresh attempts, clear status to exit the loop
		if refreshCount > 2 {
			manager.Status = ""
		}
		return `user@remote-server:~$ ls -la
total 12
drwx------ 3 user user 4096 Jan 15 10:30 .
drwxr-xr-x 5 root root 4096 Jan 15 10:25 ..
-rw-r--r-- 1 user user   18 Jan 15 10:30 .bashrc
user@remote-server:~$ `, nil
	}

	// Test that ExecWaitCapture handles SSH scenario gracefully
	result, err := manager.ExecWaitCapture("ls -la")

	assert.Error(t, err, "Should return error when SSH prompt format doesn't match expected pattern")
	assert.Contains(t, err.Error(), "failed to parse command history")
	assert.Equal(t, "", result.Command, "Should return empty CommandExecHistory on parsing failure")
	assert.Equal(t, "", result.Output, "Should return empty output on parsing failure")
	assert.Equal(t, 0, result.Code, "Should return default code on parsing failure")
	assert.Equal(t, "ls -la", commandSent, "Should have sent the command to SSH pane")
	assert.True(t, refreshCount > 1, "Should have attempted multiple refreshes before giving up")
}

// Test ExecWaitCapture with successful command execution and proper prompt
func TestExecWaitCapture_SuccessfulExecution(t *testing.T) {
	manager := &Manager{
		ExecHistory:      []CommandExecHistory{},
		Config:           &config.Config{MaxCaptureLines: 1000},
		SessionOverrides: make(map[string]interface{}),
		Status:           "running",
		ExecPane: &system.TmuxPaneDetails{
			Id:       "exec-pane",
			LastLine: "user@hostname:~[14:30][0]»", // Proper prompt ending
		},
	}

	// Mock system functions to simulate successful execution
	originalTmuxSend := system.TmuxSendCommandToPane
	originalTmuxCapture := system.TmuxCapturePane
	defer func() { 
		system.TmuxSendCommandToPane = originalTmuxSend
		system.TmuxCapturePane = originalTmuxCapture
	}()

	commandSent := ""
	system.TmuxSendCommandToPane = func(paneId string, command string, enter bool) error {
		commandSent = command
		// Immediately set the proper ending to simulate quick command completion
		manager.ExecPane.LastLine = "user@hostname:~[14:30][0]»"
		return nil
	}

	// Mock successful command output with proper prompt format
	system.TmuxCapturePane = func(paneId string, maxLines int) (string, error) {
		return `user@hostname:~[14:30][0]» echo "test successful"
test successful
user@hostname:~[14:31][0]» `, nil
	}

	// Test successful command execution
	result, err := manager.ExecWaitCapture("echo \"test successful\"")

	assert.NoError(t, err, "Should not return error for successful execution")
	assert.Equal(t, "echo \"test successful\"", result.Command)
	assert.Equal(t, 0, result.Code, "Should capture successful exit code")
	assert.Equal(t, "test successful", result.Output)
	assert.Equal(t, "echo \"test successful\"", commandSent, "Should have sent the correct command")
}
