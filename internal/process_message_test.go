package internal

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/alvinunreal/tmuxai/config"
	"github.com/alvinunreal/tmuxai/system"
	"github.com/stretchr/testify/assert"
)

// Mocking utilities
type mockRoundTripper struct {
	doFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.doFunc(req)
}

var tmuxSendCommandToPaneCalls []string
var highlightCodeCalls []string

func setupSystemMocks(t *testing.T) func() {
	tmuxSendCommandToPaneCalls = []string{}
	highlightCodeCalls = []string{}

	originalTmuxCapturePane := system.TmuxCapturePane
	system.TmuxCapturePane = func(paneId string, maxLines int) (string, error) {
		return "", nil
	}

	originalTmuxSendCommand := system.TmuxSendCommandToPane
	system.TmuxSendCommandToPane = func(paneId string, command string, autoenter bool) error {
		tmuxSendCommandToPaneCalls = append(tmuxSendCommandToPaneCalls, command)
		return nil
	}

	originalHighlightCode := system.HighlightCode
	system.HighlightCode = func(language string, code string) (string, error) {
		highlightCodeCalls = append(highlightCodeCalls, code)
		return code, nil
	}

	return func() {
		system.TmuxCapturePane = originalTmuxCapturePane
		system.TmuxSendCommandToPane = originalTmuxSendCommand
		system.HighlightCode = originalHighlightCode
	}
}

func newTestManager(httpClient *http.Client) *Manager {
	cfg := &config.Config{
		Debug: true,
		OpenRouter: config.OpenRouterConfig{
			APIKey: "test-key",
		},
		WaitInterval: 1, // smaller for tests
		ExecConfirm:  true,
	}
	aiClient := NewAiClient(cfg)
	if httpClient != nil {
		aiClient.client = httpClient
	}

	m := &Manager{
		Config:   cfg,
		AiClient: aiClient,
		Status:   "running",
		PaneId:   "test-pane-id",
		ExecPane: &system.TmuxPaneDetails{Id: "exec-pane-id", IsPrepared: false, Shell: "bash"},
		Messages: []ChatMessage{},
		OS:       "linux",
	}
	m.confirmedToExec = m.confirmedToExecFn
	m.getTmuxPanesInXml = m.getTmuxPanesInXmlFn
	return m
}

func TestProcessUserMessage_ExecCommand(t *testing.T) {
	teardown := setupSystemMocks(t)
	defer teardown()

	// Mock AI response
	mockRT := &mockRoundTripper{}
	mockClient := &http.Client{Transport: mockRT}
	m := newTestManager(mockClient)

	// Mock confirmation
	originalConfirmedToExec := m.confirmedToExec
	m.confirmedToExec = func(command, question string, showHelp bool) (bool, string) {
		return true, command
	}
	defer func() { m.confirmedToExec = originalConfirmedToExec }()

	// Mock GetTmuxPanesInXml
	originalGetTmuxPanesInXml := m.getTmuxPanesInXml
	m.getTmuxPanesInXml = func(cfg *config.Config) string {
		return "<panes></panes>"
	}
	defer func() { m.getTmuxPanesInXml = originalGetTmuxPanesInXml }()

	// This will call ProcessUserMessage recursively, we need to handle the second call
	// The second call will have the message "sending updated pane(s) content"
	// We will make the mock http client return a response that accomplishes the request
	var requestCount int
	var mu sync.Mutex
	mockRT.doFunc = func(req *http.Request) (*http.Response, error) {
		mu.Lock()
		requestCount++
		mu.Unlock()

		var respBody string
		if requestCount == 1 {
			respBody = `{
				"choices": [{
					"message": {
						"content": "<response><exec_command>ls -l</exec_command></response>"
					}
				}]
			}`
		} else {
			respBody = `{
				"choices": [{
					"message": {
						"content": "<response><request_accomplished/></response>"
					}
				}]
			}`
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(respBody)),
		},
		nil
	}

	m.ProcessUserMessage(context.Background(), "list files")

	assert.Contains(t, tmuxSendCommandToPaneCalls, "ls -l", "TmuxSendCommandToPane should be called with the command")
	assert.Contains(t, highlightCodeCalls, "ls -l", "HighlightCode should be called with the command")
}

func TestProcessUserMessage_SendKeys(t *testing.T) {
	teardown := setupSystemMocks(t)
	defer teardown()

	// Mock AI response
	mockRT := &mockRoundTripper{}
	mockClient := &http.Client{Transport: mockRT}
	m := newTestManager(mockClient)
	m.Config.SendKeysConfirm = true // Ensure confirmation is requested

	// Mock confirmation
	originalConfirmedToExec := m.confirmedToExec
	m.confirmedToExec = func(command, question string, showHelp bool) (bool, string) {
		assert.Equal(t, "keys shown above", command)
		assert.Equal(t, "Send this key?", question)
		return true, command
	}
	defer func() { m.confirmedToExec = originalConfirmedToExec }()

	// Mock GetTmuxPanesInXml
	originalGetTmuxPanesInXml := m.getTmuxPanesInXml
	m.getTmuxPanesInXml = func(cfg *config.Config) string {
		return "<panes></panes>"
	}
	defer func() { m.getTmuxPanesInXml = originalGetTmuxPanesInXml }()

	var requestCount int
	var mu sync.Mutex
	mockRT.doFunc = func(req *http.Request) (*http.Response, error) {
		mu.Lock()
		requestCount++
		mu.Unlock()

		var respBody string
		if requestCount == 1 {
			respBody = `{
				"choices": [{
					"message": {
						"content": "<response><send_keys>Enter</send_keys></response>"
					}
				}]
			}`
		} else {
			respBody = `{
				"choices": [{
					"message": {
						"content": "<response><request_accomplished/></response>"
					}
				}]
			}`
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(respBody)),
		},
		nil
	}

	m.ProcessUserMessage(context.Background(), "press enter")

	assert.Contains(t, tmuxSendCommandToPaneCalls, "Enter", "TmuxSendCommandToPane should be called with the key")
	assert.Contains(t, highlightCodeCalls, "Enter", "HighlightCode should be called with the key")
}

func TestProcessUserMessage_PasteMultilineContent(t *testing.T) {
	teardown := setupSystemMocks(t)
	defer teardown()

	// Mock AI response
	mockRT := &mockRoundTripper{}
	mockClient := &http.Client{Transport: mockRT}
	m := newTestManager(mockClient)
	m.Config.PasteMultilineConfirm = true

	// Mock confirmation
	originalConfirmedToExec := m.confirmedToExec
	m.confirmedToExec = func(command, question string, showHelp bool) (bool, string) {
		assert.Equal(t, "hello\nworld", command)
		assert.Equal(t, "Paste multiline content?", question)
		return true, command
	}
	defer func() { m.confirmedToExec = originalConfirmedToExec }()

	// Mock GetTmuxPanesInXml
	originalGetTmuxPanesInXml := m.getTmuxPanesInXml
	m.getTmuxPanesInXml = func(cfg *config.Config) string {
		return "<panes></panes>"
	}
	defer func() { m.getTmuxPanesInXml = originalGetTmuxPanesInXml }()

	var requestCount int
	var mu sync.Mutex
	mockRT.doFunc = func(req *http.Request) (*http.Response, error) {
		mu.Lock()
		requestCount++
		mu.Unlock()

		var respBody string
		if requestCount == 1 {
			respBody = `{
				"choices": [{
					"message": {
						"content": "<response><paste_multiline_content>hello\nworld</paste_multiline_content></response>"
					}
				}]
			}`
		} else {
			respBody = `{
				"choices": [{
					"message": {
						"content": "<response><request_accomplished/></response>"
					}
				}]
			}`
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(respBody)),
		},
		nil
	}

	m.ProcessUserMessage(context.Background(), "paste content")

	assert.Contains(t, tmuxSendCommandToPaneCalls, "hello\nworld", "TmuxSendCommandToPane should be called with the content")
	assert.Contains(t, highlightCodeCalls, "hello\nworld", "HighlightCode should be called with the content")
}

func TestProcessUserMessage_ExecPaneSeemsBusy(t *testing.T) {
	teardown := setupSystemMocks(t)
	defer teardown()

	mockRT := &mockRoundTripper{}
	mockClient := &http.Client{Transport: mockRT}
	m := newTestManager(mockClient)

	// Mock confirmation
	originalConfirmedToExec := m.confirmedToExec
	m.confirmedToExec = func(command, question string, showHelp bool) (bool, string) {
		return true, command
	}
	defer func() { m.confirmedToExec = originalConfirmedToExec }()

	// Mock GetTmuxPanesInXml
	originalGetTmuxPanesInXml := m.getTmuxPanesInXml
	m.getTmuxPanesInXml = func(cfg *config.Config) string {
		return "<panes></panes>"
	}
	defer func() { m.getTmuxPanesInXml = originalGetTmuxPanesInXml }()

	var requestCount int
	var mu sync.Mutex
	mockRT.doFunc = func(req *http.Request) (*http.Response, error) {
		mu.Lock()
		requestCount++
		mu.Unlock()

		var respBody string
		switch requestCount {
		case 1:
			// First call, AI says pane is busy
			respBody = `{
				"choices": [{
					"message": {
						"content": "<response><exec_pane_seems_busy/></response>"
					}
				}]
			}`
		case 2:
			// Second call (after waiting), AI gives a command
			respBody = `{
				"choices": [{
					"message": {
						"content": "<response><exec_command>ls -l</exec_command></response>"
					}
				}]
			}`
		default:
			// Third call (after exec), AI says request is accomplished
			respBody = `{
				"choices": [{
					"message": {
						"content": "<response><request_accomplished/></response>"
					}
				}]
			}`
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(respBody)),
		},
		nil
	}

	m.ProcessUserMessage(context.Background(), "list files")

	assert.Equal(t, 3, requestCount, "Should have been 3 requests to the AI")
	assert.Contains(t, tmuxSendCommandToPaneCalls, "ls -l", "TmuxSendCommandToPane should be called with the command")
}

func TestProcessUserMessage_AIGuidelineError(t *testing.T) {
	teardown := setupSystemMocks(t)
	defer teardown()

	mockRT := &mockRoundTripper{}
	mockClient := &http.Client{Transport: mockRT}
	m := newTestManager(mockClient)

	// Mock GetTmuxPanesInXml
	originalGetTmuxPanesInXml := m.getTmuxPanesInXml
	m.getTmuxPanesInXml = func(cfg *config.Config) string {
		return "<panes></panes>"
	}
	defer func() { m.getTmuxPanesInXml = originalGetTmuxPanesInXml }()

	var requestCount int
	var mu sync.Mutex
	mockRT.doFunc = func(req *http.Request) (*http.Response, error) {
		mu.Lock()
		requestCount++
		mu.Unlock()

		var respBody string
		if requestCount == 1 {
			// First call, AI gives an invalid response (multiple booleans)
			respBody = `{
				"choices": [{
					"message": {
						"content": "<response><request_accomplished/><exec_pane_seems_busy/></response>"
					}
				}]
			}`
		} else {
			// Second call (retry), AI gives a valid response
			respBody = `{
				"choices": [{
					"message": {
						"content": "<response><request_accomplished/></response>"
					}
				}]
			}`
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(respBody)),
		},
		nil
	}

	m.ProcessUserMessage(context.Background(), "some message")

	assert.Equal(t, 2, requestCount, "Should have been 2 requests to the AI")
	// The message should be added to history
	assert.Equal(t, 4, len(m.Messages))
	assert.True(t, m.Messages[0].FromUser)
	assert.False(t, m.Messages[1].FromUser)
	assert.Contains(t, m.Messages[1].Content, "<request_accomplished/><exec_pane_seems_busy/>")
}

func TestProcessUserMessage_ExecCommand_PreparedPane(t *testing.T) {
	teardown := setupSystemMocks(t)
	defer teardown()

	// Mock AI response
	mockRT := &mockRoundTripper{}
	mockClient := &http.Client{Transport: mockRT}
	m := newTestManager(mockClient)
	m.ExecPane.IsPrepared = true // Set pane to prepared

	// Mock confirmation
	originalConfirmedToExec := m.confirmedToExec
	m.confirmedToExec = func(command, question string, showHelp bool) (bool, string) {
		return true, command
	}
	defer func() { m.confirmedToExec = originalConfirmedToExec }()

	// Mock GetTmuxPanesInXml
	originalGetTmuxPanesInXml := m.getTmuxPanesInXml
	m.getTmuxPanesInXml = func(cfg *config.Config) string {
		return "<panes></panes>"
	}
	defer func() { m.getTmuxPanesInXml = originalGetTmuxPanesInXml }()

	// Mock for ExecWaitCapture
	originalTmuxCapturePane := system.TmuxCapturePane
	var capturePaneCalls int
	system.TmuxCapturePane = func(paneId string, maxLines int) (string, error) {
		capturePaneCalls++
		if capturePaneCalls > 1 {
			// On second call, return a prompt that indicates command is done
			return "ls -l\n-rw-r--r-- 1 user user 123 Aug 16 12:34 file.txt\nuser@host:~/path[12:34][0]»", nil
		}
		// First call, command is running
		return "ls -l", nil
	}
	defer func() { system.TmuxCapturePane = originalTmuxCapturePane }()

	// Mock TmuxPanesDetails as it's called by Refresh
	originalTmuxPanesDetails := system.TmuxPanesDetails
	system.TmuxPanesDetails = func(target string) ([]system.TmuxPaneDetails, error) {
		return []system.TmuxPaneDetails{
			{Id: "exec-pane-id"},
		},
		nil
	}
	defer func() { system.TmuxPanesDetails = originalTmuxPanesDetails }()

	var requestCount int
	var mu sync.Mutex
	mockRT.doFunc = func(req *http.Request) (*http.Response, error) {
		mu.Lock()
		requestCount++
		mu.Unlock()

		var respBody string
		if requestCount == 1 {
			respBody = `{
				"choices": [{
					"message": {
						"content": "<response><exec_command>ls -l</exec_command></response>"
					}
				}]
			}`
		} else {
			// After ExecWaitCapture, ProcessUserMessage is called again.
			respBody = `{
				"choices": [{
					"message": {
						"content": "<response><request_accomplished/></response>"
					}
				}]
			}`
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(respBody)),
		},
		nil
	}

	m.ProcessUserMessage(context.Background(), "list files")

	assert.Contains(t, tmuxSendCommandToPaneCalls, "ls -l", "TmuxSendCommandToPane should be called with the command")
	assert.True(t, capturePaneCalls > 1, "TmuxCapturePane should be called to check for command completion")
	assert.Equal(t, 2, requestCount, "Should have been 2 requests to the AI")
}

func TestProcessUserMessage_WatchMode_NoComment(t *testing.T) {
	teardown := setupSystemMocks(t)
	defer teardown()

	mockRT := &mockRoundTripper{}
	mockClient := &http.Client{Transport: mockRT}
	m := newTestManager(mockClient)
	m.WatchMode = true

	m.getTmuxPanesInXml = func(cfg *config.Config) string {
		return "<panes></panes>"
	}

	mockRT.doFunc = func(req *http.Request) (*http.Response, error) {
		respBody := `{
			"choices": [{
				"message": {
					"content": "<response><no_comment/></response>"
				}
			}]
		}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(respBody)),
		},
		nil
	}

	accomplished := m.ProcessUserMessage(context.Background(), "watching...")

	assert.False(t, accomplished, "Should not be accomplished")
	assert.Empty(t, m.Messages, "No messages should be added to history on no_comment")
}

func TestProcessUserMessage_WatchMode_ExecCommand(t *testing.T) {
	teardown := setupSystemMocks(t)
	defer teardown()

	mockRT := &mockRoundTripper{}
	mockClient := &http.Client{Transport: mockRT}
	m := newTestManager(mockClient)
	m.WatchMode = true
	m.Config.ExecConfirm = false // disable confirmation for simplicity

	m.getTmuxPanesInXml = func(cfg *config.Config) string {
		return "<panes></panes>"
	}

	var requestCount int
	mockRT.doFunc = func(req *http.Request) (*http.Response, error) {
		requestCount++
		respBody := `{
			"choices": [{
				"message": {
					"content": "<response><exec_command>echo 'suggestion'</exec_command></response>"
				}
			}]
		}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(respBody)),
		},
		nil
	}

	accomplished := m.ProcessUserMessage(context.Background(), "watching for stuff")

	assert.False(t, accomplished, "Should not be accomplished, as watch mode continues")
	assert.Equal(t, 1, requestCount, "Should be only one request to AI, no recursion")
	assert.Contains(t, tmuxSendCommandToPaneCalls, "echo 'suggestion'", "TmuxSendCommandToPane should be called with the command")
	assert.Equal(t, 2, len(m.Messages), "User and AI messages should be added to history")
}

func TestProcessUserMessage_ContextCanceled(t *testing.T) {
	teardown := setupSystemMocks(t)
	defer teardown()

	mockRT := &mockRoundTripper{}
	mockClient := &http.Client{Transport: mockRT}
	m := newTestManager(mockClient)

	m.getTmuxPanesInXml = func(cfg *config.Config) string {
		return "<panes></panes>"
	}

	mockRT.doFunc = func(req *http.Request) (*http.Response, error) {
		return nil, context.Canceled
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	accomplished := m.ProcessUserMessage(ctx, "some message")

	assert.False(t, accomplished, "Should not be accomplished on cancellation")
	assert.Empty(t, m.Messages, "No messages should be added to history on cancellation")
	assert.Equal(t, "", m.Status, "Status should be cleared on error")
}

func TestProcessUserMessage_AIClientError(t *testing.T) {
	teardown := setupSystemMocks(t)
	defer teardown()

	mockRT := &mockRoundTripper{}
	mockClient := &http.Client{Transport: mockRT}
	m := newTestManager(mockClient)

	m.getTmuxPanesInXml = func(cfg *config.Config) string {
		return "<panes></panes>"
	}

	mockRT.doFunc = func(req *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("AI is down")
	}

	accomplished := m.ProcessUserMessage(context.Background(), "some message")

	assert.False(t, accomplished, "Should not be accomplished on AI error")
	assert.Empty(t, m.Messages, "No messages should be added to history on AI error")
	assert.Equal(t, "", m.Status, "Status should be cleared on error")
}

func TestProcessUserMessage_ParseError(t *testing.T) {
	teardown := setupSystemMocks(t)
	defer teardown()

	mockRT := &mockRoundTripper{}
	mockClient := &http.Client{Transport: mockRT}
	m := newTestManager(mockClient)

	m.getTmuxPanesInXml = func(cfg *config.Config) string {
		return "<panes></panes>"
	}

	mockRT.doFunc = func(req *http.Request) (*http.Response, error) {
		respBody := `{
			"choices": [{
				"message": {
					"content": "<response><unclosed_tag></response>"
				}
			}]
		}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(respBody)),
		},
		nil
	}

	accomplished := m.ProcessUserMessage(context.Background(), "some message")

	assert.False(t, accomplished, "Should not be accomplished on parse error")
	assert.Empty(t, m.Messages, "No messages should be added to history on parse error")
	assert.Equal(t, "", m.Status, "Status should be cleared on error")
}

func TestAIFollowedGuidelines(t *testing.T) {
	m := &Manager{} // The manager is needed as it's a method receiver

	tests := []struct {
		name          string
		response      AIResponse
		watchMode     bool
		expectedValid bool
		expectedError string
	}{
		{
			name: "Valid: RequestAccomplished",
			response: AIResponse{
				RequestAccomplished: true,
			},
			watchMode:     false,
			expectedValid: true,
			expectedError: "",
		},
		{
			name: "Valid: ExecPaneSeemsBusy",
			response: AIResponse{
				ExecPaneSeemsBusy: true,
			},
			watchMode:     false,
			expectedValid: true,
			expectedError: "",
		},
		{
			name: "Valid: WaitingForUserResponse",
			response: AIResponse{
				WaitingForUserResponse: true,
			},
			watchMode:     false,
			expectedValid: true,
			expectedError: "",
		},
		{
			name: "Valid: NoComment in WatchMode",
			response: AIResponse{
				NoComment: true,
			},
			watchMode:     true,
			expectedValid: true,
			expectedError: "",
		},
		{
			name: "Invalid: Multiple booleans",
			response: AIResponse{
				RequestAccomplished: true,
				ExecPaneSeemsBusy:   true,
			},
			watchMode:     false,
			expectedValid: false,
			expectedError: "You didn't follow the guidelines. Only one boolean flag should be set to true in your response. Pay attention!",
		},
		{
			name: "Valid: ExecCommand",
			response: AIResponse{
				ExecCommand: []string{"ls"},
			},
			watchMode:     false,
			expectedValid: true,
			expectedError: "",
		},
		{
			name: "Valid: SendKeys",
			response: AIResponse{
				SendKeys: []string{"enter"},
			},
			watchMode:     false,
			expectedValid: true,
			expectedError: "",
		},
		{
			name: "Valid: PasteMultilineContent",
			response: AIResponse{
				PasteMultilineContent: "hello",
			},
			watchMode:     false,
			expectedValid: true,
			expectedError: "",
		},
		{
			name: "Invalid: Multiple tags",
			response: AIResponse{
				ExecCommand: []string{"ls"},
				SendKeys:    []string{"enter"},
			},
			watchMode:     false,
			expectedValid: false,
			expectedError: "You didn't follow the guidelines. You can only use one type of XML tag in your response. Pay attention!",
		},
		{
			name: "Invalid: No tags or booleans in non-watch mode",
			response: AIResponse{
				Message: "hello",
			},
			watchMode:     false,
			expectedValid: false,
			expectedError: "You didn't follow the guidelines. You must use at least one XML tag in your response. Pay attention!",
		},
		{
			name: "Valid: No tags or booleans in watch mode with NoComment",
			response: AIResponse{
				Message:   "hello",
				NoComment: true,
			},
			watchMode:     true,
			expectedValid: true,
			expectedError: "",
		},
		{
			name: "Valid: No tags or booleans in watch mode without NoComment is an invalid case, but the current implementation allows it",
			response: AIResponse{
				Message: "hello",
			},
			watchMode:     true,
			expectedValid: true,
			expectedError: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m.WatchMode = tt.watchMode
			errMsg, isValid := m.aiFollowedGuidelines(tt.response)
			assert.Equal(t, tt.expectedValid, isValid)
			assert.Equal(t, tt.expectedError, errMsg)
		})
	}
}

func TestProcessUserMessage_ExecCommand_UserDenies(t *testing.T) {
	teardown := setupSystemMocks(t)
	defer teardown()

	// Mock AI response
	mockRT := &mockRoundTripper{}
	mockClient := &http.Client{Transport: mockRT}
	m := newTestManager(mockClient)

	// Mock confirmation to return false
	originalConfirmedToExec := m.confirmedToExec
	m.confirmedToExec = func(command, question string, showHelp bool) (bool, string) {
		return false, command
	}
	defer func() { m.confirmedToExec = originalConfirmedToExec }()

	// Mock GetTmuxPanesInXml
	m.getTmuxPanesInXml = func(cfg *config.Config) string {
		return "<panes></panes>"
	}

	// Only one request should be made, as the flow should stop after denial
	mockRT.doFunc = func(req *http.Request) (*http.Response, error) {
		respBody := `{
			"choices": [{
				"message": {
					"content": "<response><exec_command>ls -l</exec_command></response>"
				}
			}]
		}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(respBody)),
		},
		nil
	}

	accomplished := m.ProcessUserMessage(context.Background(), "list files")

	assert.False(t, accomplished, "Should not be accomplished when user denies execution")
	assert.Empty(t, tmuxSendCommandToPaneCalls, "TmuxSendCommandToPane should not be called")
	assert.Equal(t, "", m.Status, "Status should be cleared when user denies")
}

func TestProcessUserMessage_SendKeys_UserDenies(t *testing.T) {
	teardown := setupSystemMocks(t)
	defer teardown()

	mockRT := &mockRoundTripper{}
	mockClient := &http.Client{Transport: mockRT}
	m := newTestManager(mockClient)
	m.Config.SendKeysConfirm = true

	// Mock confirmation to return false
	m.confirmedToExec = func(command, question string, showHelp bool) (bool, string) {
		return false, command
	}

	m.getTmuxPanesInXml = func(cfg *config.Config) string {
		return "<panes></panes>"
	}

	mockRT.doFunc = func(req *http.Request) (*http.Response, error) {
		respBody := `{
			"choices": [{
				"message": {
					"content": "<response><send_keys>Enter</send_keys></response>"
				}
			}]
		}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(respBody)),
		},
		nil
	}

	accomplished := m.ProcessUserMessage(context.Background(), "press enter")

	assert.False(t, accomplished, "Should not be accomplished when user denies sending keys")
	assert.Empty(t, tmuxSendCommandToPaneCalls, "TmuxSendCommandToPane should not be called")
	assert.Equal(t, "", m.Status, "Status should be cleared when user denies")
}

func TestProcessUserMessage_PasteMultilineContent_UserDenies(t *testing.T) {
	teardown := setupSystemMocks(t)
	defer teardown()

	mockRT := &mockRoundTripper{}
	mockClient := &http.Client{Transport: mockRT}
	m := newTestManager(mockClient)
	m.Config.PasteMultilineConfirm = true

	// Mock confirmation to return false
	m.confirmedToExec = func(command, question string, showHelp bool) (bool, string) {
		return false, command
	}

	m.getTmuxPanesInXml = func(cfg *config.Config) string {
		return "<panes></panes>"
	}

	mockRT.doFunc = func(req *http.Request) (*http.Response, error) {
		respBody := `{
			"choices": [{
				"message": {
					"content": "<response><paste_multiline_content>hello\nworld</paste_multiline_content></response>"
				}
			}]
		}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(respBody)),
		},
		nil
	}

	accomplished := m.ProcessUserMessage(context.Background(), "paste content")

	assert.False(t, accomplished, "Should not be accomplished when user denies pasting")
	assert.Empty(t, tmuxSendCommandToPaneCalls, "TmuxSendCommandToPane should not be called")
	assert.Equal(t, "", m.Status, "Status should be cleared when user denies")
}

func TestProcessUserMessage_WaitingForUserResponse(t *testing.T) {
	teardown := setupSystemMocks(t)
	defer teardown()

	mockRT := &mockRoundTripper{}
	mockClient := &http.Client{Transport: mockRT}
	m := newTestManager(mockClient)

	m.getTmuxPanesInXml = func(cfg *config.Config) string {
		return "<panes></panes>"
	}

	var requestCount int
	mockRT.doFunc = func(req *http.Request) (*http.Response, error) {
		requestCount++
		respBody := `{
			"choices": [{
				"message": {
					"content": "<response><waiting_for_user_response/></response>"
				}
			}]
		}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(respBody)),
		},
		nil
	}

	accomplished := m.ProcessUserMessage(context.Background(), "some message")

	assert.False(t, accomplished, "Should not be accomplished when waiting for user response")
	assert.Equal(t, 1, requestCount, "Should only be one request to the AI")
	assert.Equal(t, "waiting", m.Status, "Status should be 'waiting'")
	assert.Equal(t, 2, len(m.Messages), "Messages should be added to history")
}

func TestProcessUserMessage_MultipleSendKeys(t *testing.T) {
	teardown := setupSystemMocks(t)
	defer teardown()

	mockRT := &mockRoundTripper{}
	mockClient := &http.Client{Transport: mockRT}
	m := newTestManager(mockClient)
	m.Config.SendKeysConfirm = true

	// Mock confirmation
	m.confirmedToExec = func(command, question string, showHelp bool) (bool, string) {
		assert.Equal(t, "keys shown above", command)
		assert.Equal(t, "Send all these keys?", question)
		return true, command
	}

	m.getTmuxPanesInXml = func(cfg *config.Config) string {
		return "<panes></panes>"
	}

	var requestCount int
	mockRT.doFunc = func(req *http.Request) (*http.Response, error) {
		requestCount++
		var respBody string
		if requestCount == 1 {
			respBody = `{
				"choices": [{
					"message": {
						"content": "<response><send_keys>key1</send_keys><send_keys>key2</send_keys></response>"
					}
				}]
			}`
		} else {
			respBody = `{
				"choices": [{
					"message": {
						"content": "<response><request_accomplished/></response>"
					}
				}]
			}`
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(respBody)),
		},
		nil
	}

	m.ProcessUserMessage(context.Background(), "send multiple keys")

	assert.Contains(t, tmuxSendCommandToPaneCalls, "key1", "TmuxSendCommandToPane should be called with the first key")
	assert.Contains(t, tmuxSendCommandToPaneCalls, "key2", "TmuxSendCommandToPane should be called with the second key")
	assert.Contains(t, highlightCodeCalls, "key1", "HighlightCode should be called with the first key")
	assert.Contains(t, highlightCodeCalls, "key2", "HighlightCode should be called with the second key")
	assert.Equal(t, 2, requestCount, "Should have been 2 requests to the AI")
}