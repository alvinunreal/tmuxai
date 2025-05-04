package internal

import (
	"github.com/openai/openai-go"
)

// GetTools returns the OpenAI function tools used by the AI to interact with tmux
func GetTools() []openai.ChatCompletionToolParam {
	return []openai.ChatCompletionToolParam{
		{
			Function: openai.FunctionDefinitionParam{
				Name:        "TmuxSendKeys",
				Description: openai.String("Use this to send keystrokes to the tmux pane. Supported keys include standard characters, function keys (F1-F12), navigation keys (Up,Down,Left,Right,BSpace,BTab,DC,End,Enter,Escape,Home,IC,NPage,PageDown,PgDn,PPage,PageUp,PgUp,Space,Tab), and modifier keys (C-, M-). Example flow of using this tool: TmuxSendKeys([vim somefile, Enter, :set paste, Enter, i, some text, Enter, Escape, :wq, Enter]) Modifier Example: TmuxSendKeys([C-c]) or TmuxSendKeys([M-a])"),
				Parameters: openai.FunctionParameters{
					"type": "object",
					"properties": map[string]interface{}{
						"keys": map[string]interface{}{
							"type":        "array",
							"description": "An array of keys to send to the tmux pane, each item will be processed sequentially",
							"items": map[string]string{
								"type": "string",
							},
						},
					},
					"required": []string{"keys"},
				},
			},
		},
		// {
		// 	Function: openai.FunctionDefinitionParam{
		// 		Name:        "ExecCommand",
		// 		Description: openai.String("If you detect the current prompt in exec pane is any shell prompt(fish, zsh, bash, etc) and is not busy, meaning it's directly possible to execute commands in that tmux exec pane, use this to execute shell commands in that tmux exec pane. Each command must be simple, standalone, and avoid complex chaining (e.g., no >, ;, &&, or similar operators unless necessary).Ensure each command builds logically on the success of the previous one."),
		// 		Parameters: openai.FunctionParameters{
		// 			"type": "object",
		// 			"properties": map[string]interface{}{
		// 				"command": map[string]string{
		// 					"type":        "string",
		// 					"description": "The command to execute in the tmux exec pane",
		// 				},
		// 			},
		// 			"required": []string{"command"},
		// 		},
		// 	},
		// },
		{
			Function: openai.FunctionDefinitionParam{
				Name:        "SendMultilineContent",
				Description: openai.String("You can use this to send multiline content, it's forbidden to use this to execute commands in a shell, when detected fish, bash, zsh etc prompt, for that you should use ExecCommand. Main use for this is when it's vim open and you need to type multiline text, etc."),
				Parameters: openai.FunctionParameters{
					"type": "object",
					"properties": map[string]interface{}{
						"content": map[string]string{
							"type":        "string",
							"description": "The multiline content to paste",
						},
					},
					"required": []string{"content"},
				},
			},
		},
		{
			Function: openai.FunctionDefinitionParam{
				Name:        "StartCountdown",
				Description: openai.String("You can use this to start a pre-configured, wait interval countdown, after which you will receive updated pane content. This is useful for e.g when tmux exec pane is busy running a command and you want to wait for it to finish before proceeding further."),
				Parameters: openai.FunctionParameters{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			},
		},
		{
			Function: openai.FunctionDefinitionParam{
				Name:        "StopAgenticLoop",
				Description: openai.String("Stops the agentic loop. For e.g: The loop should stop when you have a question to ask the user, loop should stop so user can answer it, when task accomplished, loop should stop too."),
				Parameters: openai.FunctionParameters{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			},
		},
		// {
		// 	Function: openai.FunctionDefinitionParam{
		// 		Name:        "ExecAndWait",
		// 		Description: openai.String("Execute a command and wait for it to complete. The command's output will be captured and sent back to the AI."),
		// 		Parameters: openai.FunctionParameters{
		// 			"type": "object",
		// 			"properties": map[string]interface{}{
		// 				"command": map[string]string{
		// 					"type":        "string",
		// 					"description": "The command to execute",
		// 				},
		// 			},
		// 			"required": []string{"command"},
		// 		},
		// 	},
		// },
		// {
		// 	Function: openai.FunctionDefinitionParam{
		// 		Name:        "ChangeState",
		// 		Description: openai.String("Use this to change the state of the tmuxai."),
		// 		Parameters: openai.FunctionParameters{
		// 			"type": "object",
		// 			"properties": map[string]interface{}{
		// 				"state": map[string]interface{}{
		// 					"type":        "string",
		// 					"enum":        []string{"ExecPaneSeemsBusy", "WaitingForUserResponse", "RequestAccomplished"},
		// 					"description": "The state to change to. ExecPaneSeemsBusy: Use this value when you need to wait for the exec pane to finish before proceeding. WaitingForUserResponse: Use this value when you have a question, need input or clarification from the user to accomplish the request. RequestAccomplished: Use this value when you have successfully completed and verified the user's request.",
		// 				},
		// 			},
		// 			"required": []string{"state"},
		// 		},
		// 	},
		// },
	}
}
