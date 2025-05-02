package internal

import "encoding/json"

// Tool represents a tool definition for the AI model.
type Tool struct {
	Type     string             `json:"type"`
	Function FunctionDefinition `json:"function"`
}

// FunctionDefinition defines the structure of a function that the AI can call.
type FunctionDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// tools defines the set of tools available to the AI model.
var tools = []Tool{
	{
		Type: "function",
		Function: FunctionDefinition{
			Name:        "TmuxSendKeys",
			Description: "Send keystrokes to the tmux pane",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"keys": {
						"type": "string",
						"description": "The keystrokes to send"
					}
				},
				"required": ["keys"]
			}`),
		},
	},
	{
		Type: "function",
		Function: FunctionDefinition{
			Name:        "ExecCommand",
			Description: "Execute a shell command in the tmux pane",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"command": {
						"type": "string",
						"description": "The command to execute"
					}
				},
				"required": ["command"]
			}`),
		},
	},
	{
		Type: "function",
		Function: FunctionDefinition{
			Name:        "PasteMultilineContent",
			Description: "Send multiline content into the tmux pane",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"content": {
						"type": "string",
						"description": "The multiline content to paste"
					}
				},
				"required": ["content"]
			}`),
		},
	},
	{
		Type: "function",
		Function: FunctionDefinition{
			Name:        "ExecAndWait",
			Description: "Execute a command and wait for it to finish",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"command": {
						"type": "string",
						"description": "The command to execute"
					}
				},
				"required": ["command"]
			}`),
		},
	},
	{
		Type: "function",
		Function: FunctionDefinition{
			Name:        "ChangeState",
			Description: "Change the internal state of the interaction (e.g., waiting for user, request accomplished).",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"state": {
						"type": "string",
						"description": "The target state",
						"enum": ["WaitingForUserResponse", "RequestAccomplished", "ExecPaneSeemsBusy", "NoComment"]
					}
				},
				"required": ["state"]
			}`),
		},
	},
}
