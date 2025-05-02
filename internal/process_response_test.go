package internal

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseAIResponse(t *testing.T) {
	m := &Manager{}

	tests := []struct {
		name           string
		input          string
		expectedOutput AIResponse
		expectedError  bool
	}{
		{
			name:  "Plain text only",
			input: "This is a simple response.",
			expectedOutput: AIResponse{
				Sequence: []ActionStep{
					{Type: "message", Content: "This is a simple response."},
				},
				State: "",
			},
			expectedError: false,
		},
		{
			name:  "Single sendKeys action",
			input: `<TmuxSendKeys>{"keys":"ls -la\n"}</TmuxSendKeys>`,
			expectedOutput: AIResponse{
				Sequence: []ActionStep{
					{Type: "sendKeys", Content: "ls -la\n"},
				},
				State: "",
			},
			expectedError: false,
		},
		{
			name:  "Single sendKeys action with tool_code",
			input: `<tool_code><TmuxSendKeys>{"keys":"ls -la\n"}</TmuxSendKeys></tool_code>`,
			expectedOutput: AIResponse{
				Sequence: []ActionStep{
					{Type: "sendKeys", Content: "ls -la\n"},
				},
				State: "",
			},
			expectedError: false,
		},
		{
			name:  "Single sendKeys action with markdown fence",
			input: "`" + `<TmuxSendKeys>{"keys":"echo hello\n"}</TmuxSendKeys>` + "`",
			expectedOutput: AIResponse{
				Sequence: []ActionStep{
					{Type: "sendKeys", Content: "echo hello\n"},
				},
				State: "",
			},
			expectedError: false,
		},
		{
			name:  "Single sendKeys action with markdown fence and tool_code",
			input: "`" + `<tool_code><TmuxSendKeys>{"keys":"echo hello\n"}</TmuxSendKeys></tool_code>` + "`",
			expectedOutput: AIResponse{
				Sequence: []ActionStep{
					{Type: "sendKeys", Content: "echo hello\n"},
				},
				State: "",
			},
			expectedError: false,
		},
		{
			name:  "Single sendKeys action with markdown fences",
			input: "```\n" + `<TmuxSendKeys>{"keys":"echo hello"}</TmuxSendKeys>` + "\n```",
			expectedOutput: AIResponse{
				Sequence: []ActionStep{
					{Type: "sendKeys", Content: "echo hello"},
				},
				State: "",
			},
			expectedError: false,
		},
		{
			name:  "Single sendKeys action with markdown fences and tool_code",
			input: "```\n" + `<tool_code><TmuxSendKeys>{"keys":"echo hello"}</TmuxSendKeys></tool_code>` + "\n```",
			expectedOutput: AIResponse{
				Sequence: []ActionStep{
					{Type: "sendKeys", Content: "echo hello"},
				},
				State: "",
			},
			expectedError: false,
		},
		{
			name:  "Single sendKeys action with xml fences",
			input: "```xml\n" + `<TmuxSendKeys>{"keys":"echo hello"}</TmuxSendKeys>` + "\n```",
			expectedOutput: AIResponse{
				Sequence: []ActionStep{
					{Type: "sendKeys", Content: "echo hello"},
				},
				State: "",
			},
			expectedError: false,
		},
		{
			name:  "Single sendKeys action with xml fences and tool_code",
			input: "```xml\n" + `<tool_code><TmuxSendKeys>{"keys":"echo hello"}</TmuxSendKeys></tool_code>` + "\n```",
			expectedOutput: AIResponse{
				Sequence: []ActionStep{
					{Type: "sendKeys", Content: "echo hello"},
				},
				State: "",
			},
			expectedError: false,
		},
		{
			name:  "No action with markdown fences",
			input: "```\n" + "<Something>value</Something>" + "\n```",
			expectedOutput: AIResponse{
				Sequence: []ActionStep{
					{Type: "message", Content: "```\n<Something>value</Something>\n```"},
				},
				State: "",
			},
			expectedError: false,
		},
		{
			name:  "No action with xml fences",
			input: "```" + "xml\n<Something>value</Something>\n" + "```",
			expectedOutput: AIResponse{
				Sequence: []ActionStep{
					{Type: "message", Content: "```xml\n<Something>value</Something>\n```"},
				},
				State: "",
			},
			expectedError: false,
		},
		{
			name:  "Single execCommand action",
			input: `<ExecCommand>{"command":"git status"}</ExecCommand>`,
			expectedOutput: AIResponse{
				Sequence: []ActionStep{
					{Type: "execCommand", Content: "git status"},
				},
				State: "",
			},
			expectedError: false,
		},
		{
			name:  "Multiple actions",
			input: `<TmuxSendKeys>{"keys":"cd /tmp\n"}</TmuxSendKeys><ExecCommand>{"command":"pwd"}</ExecCommand>`,
			expectedOutput: AIResponse{
				Sequence: []ActionStep{
					{Type: "sendKeys", Content: "cd /tmp\n"},
					{Type: "execCommand", Content: "pwd"},
				},
				State: "",
			},
			expectedError: false,
		},
		{
			name:  "Text and actions mixed",
			input: `First, go to tmp: <TmuxSendKeys>{"keys":"cd /tmp\n"}</TmuxSendKeys> Then, check where you are: <ExecCommand>{"command":"pwd"}</ExecCommand>All done.`,
			expectedOutput: AIResponse{
				Sequence: []ActionStep{
					{Type: "message", Content: "First, go to tmp:"},
					{Type: "sendKeys", Content: "cd /tmp\n"},
					{Type: "message", Content: "Then, check where you are:"},
					{Type: "execCommand", Content: "pwd"},
					{Type: "message", Content: "All done."},
				},
				State: "",
			},
			expectedError: false,
		},
		{
			name:  "Text and actions mixed with markdown fences",
			input: "Okay, I will run the command.\n" + "```xml\n" + `<ExecCommand>{"command":"ls -l"}</ExecCommand>` + "\n```\nLet me know the output.",
			expectedOutput: AIResponse{
				Sequence: []ActionStep{
					{Type: "message", Content: "Okay, I will run the command."},
					{Type: "execCommand", Content: "ls -l"},
					{Type: "message", Content: "Let me know the output."},
				},
				State: "",
			},
			expectedError: false,
		},
		{
			name:  "ChangeState action",
			input: `Changing state now. <ChangeState>{"state":"PROCESSING"}</ChangeState> Done processing.`,
			expectedOutput: AIResponse{
				Sequence: []ActionStep{
					{Type: "message", Content: "Changing state now."},
					{Type: "message", Content: "Done processing."},
				},
				State: "PROCESSING",
			},
			expectedError: false,
		},
		{
			name:  "Multiple ChangeState actions, last one wins",
			input: `<ChangeState>{"state":"START"}</ChangeState>Working...<ChangeState>{"state":"END"}</ChangeState>`,
			expectedOutput: AIResponse{
				Sequence: []ActionStep{
					{Type: "message", Content: "Working..."},
				},
				State: "END",
			},
			expectedError: false,
		},
		{
			name:  "Unknown action",
			input: `<UnknownAction>{"data":"some data"}</UnknownAction>`,
			expectedOutput: AIResponse{
				Sequence: []ActionStep{},
				State:    "",
			},
			expectedError: false,
		},
		{
			name:  "Malformed JSON",
			input: `<TmuxSendKeys>{"keys":"invalid json'}</TmuxSendKeys>`,
			expectedOutput: AIResponse{
				Sequence: []ActionStep{},
				State:    "",
			},
			expectedError: false,
		},
		{
			name:  "Missing arguments field",
			input: `<TmuxSendKeys>{}</TmuxSendKeys>`,
			expectedOutput: AIResponse{
				Sequence: []ActionStep{},
				State:    "",
			},
			expectedError: false,
		},
		{
			name:  "Empty input",
			input: "",
			expectedOutput: AIResponse{
				Sequence: []ActionStep{},
				State:    "",
			},
			expectedError: false,
		},
		{
			name:  "Whitespace input",
			input: "   \n\t   ",
			expectedOutput: AIResponse{
				Sequence: []ActionStep{},
				State:    "",
			},
			expectedError: false,
		},
		{
			name:  "Test case with a comma",
			input: "Test case with a comma",
			expectedOutput: AIResponse{
				Sequence: []ActionStep{
					{Type: "message", Content: "Test case with a comma"},
				},
				State: "",
			},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualOutput, err := m.parseAIResponse(tt.input)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				// Compare Sequence length and content carefully
				if !assert.Equal(t, len(tt.expectedOutput.Sequence), len(actualOutput.Sequence), "Sequence length mismatch") {
					fmt.Println(tt.expectedOutput.Sequence)
					fmt.Println(actualOutput.Sequence)
				}
				for i := range tt.expectedOutput.Sequence {
					if i < len(actualOutput.Sequence) {
						assert.Equal(t, tt.expectedOutput.Sequence[i], actualOutput.Sequence[i], "Sequence item mismatch at index %d", i)
					}
				}
				// Compare state
				assert.Equal(t, tt.expectedOutput.State, actualOutput.State, "State mismatch")
			}
		})
	}
}
