package internal

import (
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
