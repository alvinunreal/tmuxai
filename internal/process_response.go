package internal

import (
	"encoding/json"
	"strings"
)

func (m *Manager) parseAIResponse(response string) (AIResponse, error) {
	r := AIResponse{
		Message: response,
	}

	// If OpenAI API response contains function calls, extract them
	if strings.Contains(response, "\"tool_calls\"") || strings.Contains(response, "\"function_call\"") {
		// Extract the AI message content and tool calls
		var openAIResponse struct {
			Choices []struct {
				Message struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						ID       string `json:"id"`
						Type     string `json:"type"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"message"`
			} `json:"choices"`
		}

		err := json.Unmarshal([]byte(response), &openAIResponse)
		if err == nil && len(openAIResponse.Choices) > 0 {
			// Set the clean message without tool calls
			r.Message = openAIResponse.Choices[0].Message.Content

			// Process tool calls
			for _, toolCall := range openAIResponse.Choices[0].Message.ToolCalls {
				if toolCall.Type == "function" {
					switch toolCall.Function.Name {
					case "TmuxSendKeys":
						// Check if we can extract the keys directly
						var argsMap map[string]interface{}
						if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &argsMap); err == nil {
							if keysVal, ok := argsMap["keys"]; ok {
								// Convert to JSON string to store in Content
								keysJSON, _ := json.Marshal(keysVal)
								r.ToolCalls = append(r.ToolCalls, AIToolCall{
									Type:    "TmuxSendKeys",
									Content: string(keysJSON),
								})
							}
						}
					case "ExecCommand":
						var args map[string]string
						if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err == nil {
							if cmd, ok := args["command"]; ok {
								r.ToolCalls = append(r.ToolCalls, AIToolCall{
									Type:    "ExecCommand",
									Content: cmd,
								})
							}
						}
					case "SendMultilineContent":
						var args map[string]string
						if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err == nil {
							if content, ok := args["content"]; ok {
								r.ToolCalls = append(r.ToolCalls, AIToolCall{
									Type:    "PasteMultilineContent",
									Content: content,
								})
							}
						}
					case "StartCountdown":
						r.StartCountdown = true
					case "StopAgenticLoop":
						r.StopAgenticLoop = true
					case "ExecAndWait":
						var args map[string]string
						if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err == nil {
							if cmd, ok := args["command"]; ok {
								r.ToolCalls = append(r.ToolCalls, AIToolCall{
									Type:    "ExecAndWait",
									Content: cmd,
								})
							}
						}
					case "ChangeState":
						var args map[string]string
						if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err == nil {
							if state, ok := args["state"]; ok {
								// r.ToolCalls = append(r.ToolCalls, AIToolCall{
								// 	Type:       "ChangeState",
								// 	StateValue: state,
								// })

								// Set the corresponding boolean flags based on state value
								switch state {
								case "ExecPaneSeemsBusy":
									r.ExecPaneSeemsBusy = true
								case "WaitingForUserResponse":
									r.WaitingForUserResponse = true
								case "RequestAccomplished":
									r.RequestAccomplished = true
								case "WorkingOnUserRequest":
									r.WorkingOnUserRequest = true
								case "NoComment":
									r.NoComment = true
								}
							}
						}
					}
				}
			}
		}
	}

	return r, nil
}
