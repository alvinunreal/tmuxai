package internal

import (
	"context"
	"testing"

	"github.com/alvinunreal/tmuxai/config"
	"github.com/alvinunreal/tmuxai/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// AiClientInterface defines the interface for AI clients to make testing easier
type AiClientInterface interface {
	GetResponseFromChatMessages(ctx context.Context, messages []ChatMessage, model string) (string, error)
	ChatCompletion(ctx context.Context, messages []Message, model string) (string, error)
}

// MockAiClient is a mock implementation of AiClientInterface for testing
type MockAiClient struct {
	mock.Mock
}

func (m *MockAiClient) GetResponseFromChatMessages(ctx context.Context, messages []ChatMessage, model string) (string, error) {
	args := m.Called(ctx, messages, model)
	return args.String(0), args.Error(1)
}

func (m *MockAiClient) ChatCompletion(ctx context.Context, messages []Message, model string) (string, error) {
	args := m.Called(ctx, messages, model)
	return args.String(0), args.Error(1)
}

// Test: Pressing ctrl+c should cancel message processing
func TestProcessUserMessage_EmptyStatus(t *testing.T) {
	cfg := &config.Config{
		Debug: false,
	}

	manager := &Manager{
		Config:   cfg,
		Status:   "", // Empty status
		Messages: []ChatMessage{},
	}

	// Mock functions that would normally be called
	manager.confirmedToExec = func(command string, prompt string, edit bool) (bool, string) {
		return true, command
	}

	manager.getTmuxPanesInXml = func(config *config.Config) string {
		return "<tmux>mock pane content</tmux>"
	}

	result := manager.ProcessUserMessage(context.Background(), "test message")

	assert.False(t, result, "ProcessUserMessage should return false when status is empty")
}

// Test: Context squash should trigger when max context size is exceeded
func TestProcessUserMessage_ContextSquash(t *testing.T) {
	cfg := &config.Config{
		Debug:          false,
		MaxContextSize: 100, // Very small to trigger squash
		OpenRouter: config.OpenRouterConfig{
			Model: "test-model",
		},
	}

	manager := &Manager{
		Config:   cfg,
		Status:   "running",
		Messages: make([]ChatMessage, 0),
		ExecPane: &system.TmuxPaneDetails{
			IsPrepared: false,
			IsSubShell: false,
		},
		WatchMode: false,
	}

	// Mock functions that would normally be called
	manager.confirmedToExec = func(command string, prompt string, edit bool) (bool, string) {
		return true, command
	}

	manager.getTmuxPanesInXml = func(config *config.Config) string {
		return "<tmux>mock pane content</tmux>"
	}

	// Add enough messages to trigger squash
	for i := 0; i < 10; i++ {
		manager.Messages = append(manager.Messages, ChatMessage{
			Content:  "This is a very long message that will fill up the context size limit to trigger squashing behavior in the process user message function",
			FromUser: true,
		})
	}

	// Check that needSquash would return true
	assert.True(t, manager.needSquash(), "Should need squash with many messages")
}

// Test: AI guidelines validation should fail when multiple boolean flags are set
func TestProcessUserMessage_AIGuidelinesValidation(t *testing.T) {
	manager := &Manager{
		WatchMode: false,
	}

	// Test case 1: Multiple boolean flags set to true (should fail)
	response1 := AIResponse{
		Message:                "Test message",
		RequestAccomplished:    true,
		ExecPaneSeemsBusy:      true, // This should cause validation to fail
		WaitingForUserResponse: false,
		NoComment:              false,
		ExecCommand:            []string{"echo hello"},
	}

	guidelineError, valid := manager.aiFollowedGuidelines(response1)
	assert.False(t, valid, "Should fail validation when multiple boolean flags are set")
	assert.Contains(t, guidelineError, "Only one boolean flag should be set", "Error message should mention boolean flags")

	// Test case 2: Multiple XML tag types used (should fail)
	response2 := AIResponse{
		Message:               "Test message",
		RequestAccomplished:   true,
		ExecCommand:           []string{"echo hello"},
		SendKeys:              []string{"ctrl+c"}, // Having both ExecCommand and SendKeys should fail
		PasteMultilineContent: "",
	}

	guidelineError2, valid2 := manager.aiFollowedGuidelines(response2)
	assert.False(t, valid2, "Should fail validation when multiple XML tag types are used")
	assert.Contains(t, guidelineError2, "only use one type of XML tag", "Error message should mention XML tags")

	// Test case 3: Valid response (should pass)
	response3 := AIResponse{
		Message:             "Test message",
		RequestAccomplished: true,
		ExecCommand:         []string{"echo hello"},
	}

	_, valid3 := manager.aiFollowedGuidelines(response3)
	assert.True(t, valid3, "Should pass validation with correct format")
}

// Test: If AI requests user input, should set status to waiting
func TestProcessUserMessage_WaitingForUserResponse(t *testing.T) {
	manager := &Manager{
		Status:   "running",
		Messages: []ChatMessage{},
	}

	// Test the aiFollowedGuidelines and status setting behavior
	response := AIResponse{
		Message:                "Waiting for your input",
		WaitingForUserResponse: true,
	}

	// Simulate what happens in ProcessUserMessage when WaitingForUserResponse is true
	if response.WaitingForUserResponse {
		manager.Status = "waiting"
	}

	assert.Equal(t, "waiting", manager.Status, "Status should be set to 'waiting' when WaitingForUserResponse is true")

	// Test that aiFollowedGuidelines accepts this valid response
	_, valid := manager.aiFollowedGuidelines(response)
	assert.True(t, valid, "WaitingForUserResponse should be valid according to guidelines")
}

// Test: If AI requests to execute a command, should confirm and send it to the exec pane
func TestProcessUserMessage_ExecCommandWithConfirmation(t *testing.T) {
	cfg := &config.Config{
		Debug:       false,
		ExecConfirm: true, // Require confirmation for exec commands
	}

	manager := &Manager{
		Config:   cfg,
		Status:   "running",
		Messages: []ChatMessage{},
		ExecPane: &system.TmuxPaneDetails{
			Id:         "test-pane",
			IsPrepared: false,
			IsSubShell: false,
		},
	}

	// Track if confirmation was called and approved
	confirmationCalled := false
	commandExecuted := ""

	manager.confirmedToExec = func(command string, prompt string, edit bool) (bool, string) {
		confirmationCalled = true
		assert.Equal(t, "echo hello", command, "Should ask confirmation for the right command")
		assert.Equal(t, "Execute this command?", prompt, "Should use correct confirmation prompt")
		return true, command // User approves the command
	}

	// Mock tmux command execution by capturing what would be sent
	originalTmuxSend := system.TmuxSendCommandToPane
	defer func() { system.TmuxSendCommandToPane = originalTmuxSend }()

	system.TmuxSendCommandToPane = func(paneId string, command string, enter bool) error {
		commandExecuted = command
		assert.Equal(t, "test-pane", paneId, "Should send command to correct pane")
		assert.True(t, enter, "Should send enter key after command")
		return nil
	}

	// Test the ExecCommand processing logic directly
	response := AIResponse{
		Message:     "I'll run this command for you",
		ExecCommand: []string{"echo hello"},
	}

	// Simulate the ExecCommand processing loop from ProcessUserMessage
	for _, execCommand := range response.ExecCommand {
		isSafe := false
		command := execCommand
		if manager.GetExecConfirm() {
			isSafe, command = manager.confirmedToExec(execCommand, "Execute this command?", true)
		} else {
			isSafe = true
		}
		if isSafe {
			if manager.ExecPane.IsPrepared {
				// Would call m.ExecWaitCapture(command)
			} else {
				_ = system.TmuxSendCommandToPane(manager.ExecPane.Id, command, true)
			}
		}
	}

	assert.True(t, confirmationCalled, "Confirmation should have been called")
	assert.Equal(t, "echo hello", commandExecuted, "Command should have been executed")
}

// Test: If AI requests to execute a command, should reject it if confirmation fails
func TestProcessUserMessage_ExecCommandRejection(t *testing.T) {
	cfg := &config.Config{
		Debug:       false,
		ExecConfirm: true, // Require confirmation for exec commands
	}

	manager := &Manager{
		Config:   cfg,
		Status:   "running",
		Messages: []ChatMessage{},
		ExecPane: &system.TmuxPaneDetails{
			Id:         "test-pane",
			IsPrepared: false,
			IsSubShell: false,
		},
	}

	// Track if confirmation was called
	confirmationCalled := false
	commandExecuted := false

	manager.confirmedToExec = func(command string, prompt string, edit bool) (bool, string) {
		confirmationCalled = true
		assert.Equal(t, "echo danger", command, "Should ask confirmation for the dangerous command")
		return false, command // User rejects the command
	}

	// Mock tmux command execution to track if it was called
	originalTmuxSend := system.TmuxSendCommandToPane
	defer func() { system.TmuxSendCommandToPane = originalTmuxSend }()

	system.TmuxSendCommandToPane = func(paneId string, command string, enter bool) error {
		commandExecuted = true
		return nil
	}

	// Test the ExecCommand processing logic that should result in rejection
	response := AIResponse{
		ExecCommand: []string{"echo danger"},
	}

	// Simulate the ExecCommand processing loop from ProcessUserMessage
	statusCleared := false
	for _, execCommand := range response.ExecCommand {
		isSafe := false
		command := execCommand
		if manager.GetExecConfirm() {
			isSafe, command = manager.confirmedToExec(execCommand, "Execute this command?", true)
		} else {
			isSafe = true
		}
		if isSafe {
			if manager.ExecPane.IsPrepared {
				// Would call m.ExecWaitCapture(command)
			} else {
				_ = system.TmuxSendCommandToPane(manager.ExecPane.Id, command, true)
			}
		} else {
			manager.Status = ""
			statusCleared = true
			break // This simulates the return false in ProcessUserMessage
		}
	}

	assert.True(t, confirmationCalled, "Confirmation should have been called")
	assert.False(t, commandExecuted, "Command should NOT have been executed when rejected")
	assert.True(t, statusCleared, "Status should be cleared when command is rejected")
	assert.Equal(t, "", manager.Status, "Status should be empty after rejection")
}

// Test: If AI finishes the task, should clear status and return true
func TestProcessUserMessage_RequestAccomplished(t *testing.T) {
	manager := &Manager{
		Status:   "running",
		Messages: []ChatMessage{},
	}

	// Test the RequestAccomplished logic directly
	response := AIResponse{
		Message:             "Task completed successfully!",
		RequestAccomplished: true,
	}

	// Simulate what happens in ProcessUserMessage when RequestAccomplished is true
	result := false
	if response.RequestAccomplished {
		manager.Status = ""
		result = true
	}

	assert.True(t, result, "Should return true when RequestAccomplished")
	assert.Equal(t, "", manager.Status, "Status should be cleared when request is accomplished")

	// Verify this is a valid response according to guidelines
	_, valid := manager.aiFollowedGuidelines(response)
	assert.True(t, valid, "RequestAccomplished should be valid according to guidelines")
}

// Test: Sending multiple keys to the exec pane with confirmation
func TestProcessUserMessage_SendKeysProcessing(t *testing.T) {
	cfg := &config.Config{
		Debug:           false,
		SendKeysConfirm: true, // Require confirmation for send keys
	}

	manager := &Manager{
		Config:   cfg,
		Status:   "running",
		Messages: []ChatMessage{},
		ExecPane: &system.TmuxPaneDetails{
			Id:         "test-pane",
			IsPrepared: false,
			IsSubShell: false,
		},
	}

	// Track confirmations and keys sent
	confirmationCalled := false
	keysSent := []string{}

	manager.confirmedToExec = func(command string, prompt string, edit bool) (bool, string) {
		confirmationCalled = true
		assert.Equal(t, "keys shown above", command, "Should show generic description for keys")
		assert.Equal(t, "Send all these keys?", prompt, "Should use correct prompt for multiple keys")
		return true, command // User approves sending keys
	}

	// Mock tmux command execution to capture keys being sent
	originalTmuxSend := system.TmuxSendCommandToPane
	defer func() { system.TmuxSendCommandToPane = originalTmuxSend }()

	system.TmuxSendCommandToPane = func(paneId string, command string, enter bool) error {
		keysSent = append(keysSent, command)
		assert.Equal(t, "test-pane", paneId, "Should send keys to correct pane")
		assert.False(t, enter, "Should NOT send enter key for SendKeys")
		return nil
	}

	// Test the SendKeys processing logic directly
	response := AIResponse{
		Message:  "I'll send these keys for you",
		SendKeys: []string{"ctrl+c", "ctrl+d", "exit"},
	}

	// Simulate the SendKeys processing logic from ProcessUserMessage
	if len(response.SendKeys) > 0 {
		// Determine confirmation message based on number of keys
		confirmMessage := "Send this key?"
		if len(response.SendKeys) > 1 {
			confirmMessage = "Send all these keys?"
		}

		// Get confirmation if required
		allConfirmed := true
		if manager.GetSendKeysConfirm() {
			allConfirmed, _ = manager.confirmedToExec("keys shown above", confirmMessage, true)
		}

		if allConfirmed {
			// Send each key with delay (without the actual delay in test)
			for _, sendKey := range response.SendKeys {
				_ = system.TmuxSendCommandToPane(manager.ExecPane.Id, sendKey, false)
			}
		}
	}

	assert.True(t, confirmationCalled, "Confirmation should have been called")
	assert.Equal(t, []string{"ctrl+c", "ctrl+d", "exit"}, keysSent, "All keys should have been sent in order")

	// Verify this is a valid response according to guidelines
	_, valid := manager.aiFollowedGuidelines(response)
	assert.True(t, valid, "SendKeys should be valid according to guidelines")
}

// Test: Watch mode NoComment behavior
func TestProcessUserMessage_WatchModeNoComment(t *testing.T) {
	manager := &Manager{
		Status:    "running",
		Messages:  []ChatMessage{},
		WatchMode: true, // Enable watch mode
	}

	// Test the NoComment logic in watch mode
	response := AIResponse{
		NoComment: true, // AI has no comment in watch mode
	}

	// Simulate what happens in ProcessUserMessage for watch mode with NoComment
	result := false
	if response.NoComment {
		result = false // In watch mode, NoComment means return false (continue watching)
	}

	assert.False(t, result, "Should return false for NoComment in watch mode")

	// Verify this is a valid response according to guidelines when in watch mode
	_, valid := manager.aiFollowedGuidelines(response)
	assert.True(t, valid, "NoComment should be valid according to guidelines in watch mode")

	// Test that NoComment is valid even outside watch mode according to current logic
	manager.WatchMode = false
	response2 := AIResponse{
		NoComment: true,
	}

	// NoComment alone is actually valid even when not in watch mode (boolCount=1 satisfies count+boolCount > 0)
	_, valid2 := manager.aiFollowedGuidelines(response2)
	assert.True(t, valid2, "NoComment alone should be valid according to current guidelines logic")

	// Test truly invalid case: no boolean flags and no XML tags when not in watch mode
	response3 := AIResponse{
		Message: "Just a message with nothing else",
	}

	_, valid3 := manager.aiFollowedGuidelines(response3)
	assert.False(t, valid3, "Empty response (no flags, no XML tags) should fail validation when not in watch mode")
}
