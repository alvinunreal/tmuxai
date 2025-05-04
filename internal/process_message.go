package internal

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/alvinunreal/tmuxai/logger"
	"github.com/alvinunreal/tmuxai/system"
	"github.com/briandowns/spinner"
)

// Main function to process regular user messages
// Returns true if the request was accomplished and no further processing should happen
func (m *Manager) ProcessUserMessage(message string) bool {
	// Check if context management is needed before sending
	if m.needSquash() {
		m.Println("Exceeded context size, squashing history...")
		m.squashHistory()
	}

	s := spinner.New(spinner.CharSets[26], 100*time.Millisecond)
	s.Start()

	// check for status change before processing
	if m.Status == "" {
		s.Stop()
		return false
	}

	currentTmuxWindow := m.GetTmuxPanesInXml(m.Config)
	var execPaneEnv string
	if !m.ExecPane.IsSubShell && m.ExecPane.Shell != "" && m.ExecPane.OS != "" {
		execPaneEnv = fmt.Sprintf("IMPORTANT: the exec commands syntax should be for the shell: `%s` and OS: `%s`", m.ExecPane.Shell, m.ExecPane.OS)
	}

	currentMessage := ChatMessage{
		Content:   currentTmuxWindow + "\n\n" + execPaneEnv + "\n\n" + message,
		FromUser:  true,
		Timestamp: time.Now(),
	}

	// build current chat history
	var history []ChatMessage
	switch {
	case m.WatchMode:
		history = []ChatMessage{m.watchPrompt()}
	case m.ExecPane.IsPrepared:
		history = []ChatMessage{m.chatAssistantPreparedPrompt()}
	default:
		history = []ChatMessage{m.chatAssistantPrompt()}
	}

	history = append(history, m.Messages...)

	sending := append(history, currentMessage)

	tools := GetTools()
	response, err := m.AiClient.GetResponseFromChatMessages(sending, m.GetOpenRouterModel(), tools)
	if err != nil {
		s.Stop()
		m.Status = ""
		fmt.Println("Failed to get response from AI: " + err.Error())
		return false
	}

	// check for status change again
	if m.Status == "" {
		s.Stop()
		return false
	}

	r, err := m.parseAIResponse(response)
	if err != nil {
		s.Stop()
		m.Status = ""
		fmt.Println("Failed to parse AI response: " + err.Error())
		return false
	}

	if m.Config.Debug {
		debugChatMessages(append(history, currentMessage), response)
	}

	logger.Debug("AIResponse: %s", r.String())

	s.Stop()

	responseMsg := ChatMessage{
		Content:   r.Message,
		FromUser:  false,
		Timestamp: time.Now(),
		ToolCalls: r.ToolCalls, // Include tool calls in the chat message
	}

	// colorize code blocks in the response
	if r.Message != "" {
		fmt.Println(system.Cosmetics(r.Message))
	}

	// Don't append to history if AI is waiting for the pane or is watch mode no comment
	// if r.ExecPaneSeemsBusy || r.NoComment {
	if r.NoComment {
	} else {
		m.Messages = append(m.Messages, currentMessage, responseMsg)
	}

	// Process tool calls in the order they were requested
	for _, toolCall := range r.ToolCalls {
		switch toolCall.Type {
		case "ExecCommand":
			code, _ := system.HighlightCode("sh", toolCall.Content)
			m.Println(code)

			isSafe := false
			command := toolCall.Content
			if m.GetExecConfirm() {
				isSafe, command = m.confirmedToExec(toolCall.Content, "Execute this command?", true)
			} else {
				isSafe = true
			}
			if isSafe {
				m.Println("Executing command: " + command)
				system.TmuxSendCommandToPane(m.ExecPane.Id, command, true)
				time.Sleep(1 * time.Second)
			} else {
				m.Status = ""
				return false
			}

		case "TmuxSendKeys":
			var keys []string
			if err := json.Unmarshal([]byte(toolCall.Content), &keys); err != nil {
				fmt.Println("Failed to unmarshal keys: " + err.Error())
			} else {
				if m.GetSendKeysConfirm() {
					keysDisplay := strings.Join(keys, ", ")
					code, _ := system.HighlightCode("txt", keysDisplay)
					m.Println(code)

					isSafe, _ := m.confirmedToExec(keysDisplay, "Send these keys sequentially?", true)
					if !isSafe {
						m.Status = ""
						return false
					}
				}

				for _, key := range keys {
					m.Println("Sending key: " + key)
					system.TmuxSendCommandToPane(m.ExecPane.Id, key, false)
					time.Sleep(1 * time.Second) // Pause between each key to allow proper processing
				}
			}

		case "SendMultilineContent":
			code, _ := system.HighlightCode("txt", toolCall.Content)
			fmt.Println(code)

			isSafe := false
			if m.GetPasteMultilineConfirm() {
				isSafe, _ = m.confirmedToExec(toolCall.Content, "Paste multiline content?", false)
			} else {
				isSafe = true
			}

			if isSafe {
				m.Println("Pasting...")
				system.TmuxSendCommandToPane(m.ExecPane.Id, toolCall.Content, true)
				time.Sleep(1 * time.Second)
			} else {
				m.Status = ""
				return false
			}

		case "ExecAndWait":
			code, _ := system.HighlightCode("sh", toolCall.Content)
			fmt.Println(code)

			isSafe := false
			command := toolCall.Content
			if m.GetExecConfirm() {
				isSafe, command = m.confirmedToExec(toolCall.Content, "Execute this command?", true)
			} else {
				isSafe = true
			}
			if isSafe {
				m.ExecWaitCapture(command)
			} else {
				m.Status = ""
				return false
			}

		case "ChangeState":
			// ChangeState calls don't need further processing here as they're already
			// handled in parseAIResponse and set the corresponding boolean flags
			// Just logging that a state change was requested
			// m.Println("State change requested: " + toolCall.StateValue)
		}
	}

	if r.StartCountdown {
		m.Countdown(m.GetWaitInterval())
		accomplished := m.ProcessUserMessage("waited for 5 more seconds, here is the current pane(s) content")
		if accomplished {
			return true
		}
	}

	if r.ExecPaneSeemsBusy {
		m.Countdown(m.GetWaitInterval())
		accomplished := m.ProcessUserMessage("waited for 5 more seconds, here is the current pane(s) content")
		if accomplished {
			return true
		}
	}

	if r.RequestAccomplished {
		m.Status = ""
		return true
	}

	if r.WaitingForUserResponse {
		m.Status = "waiting"
		return false
	}

	// watch mode only
	if r.NoComment {
		return false
	}

	if r.StopAgenticLoop {
		m.Status = ""
		return true
	}
	accomplished := m.ProcessUserMessage("sending updated pane(s) content")
	return accomplished

	// this is the agentic loop
	// only when watch mode is false and there are tool calls
	// if !m.WatchMode && len(r.ToolCalls) > 0 {
	// 	accomplished := m.ProcessUserMessage("sending updated pane(s) content")
	// 	if accomplished {
	// 		return true
	// 	}
	// }
}

func (m *Manager) startWatchMode(desc string) {

	// check status
	if m.Status == "" {
		return
	}

	m.Countdown(m.GetWaitInterval())

	accomplished := m.ProcessUserMessage(desc)
	if accomplished {
		m.WatchMode = false
		m.Status = ""
		return
	}

	if m.WatchMode {
		m.startWatchMode(desc)
	}
}

func (m *Manager) aiFollowedGuidelines(r AIResponse) (string, bool) {
	// Count boolean flags
	boolCount := 0
	if r.ExecPaneSeemsBusy {
		boolCount++
	}
	if r.WaitingForUserResponse {
		boolCount++
	}
	if r.RequestAccomplished {
		boolCount++
	}
	if r.NoComment {
		boolCount++
	}

	if boolCount > 1 {
		return "You didn't follow the guidelines. Only one boolean flag should be set to true in your response. Pay attention!", false
	}

	// Count tool call types
	toolCallTypes := make(map[string]int)
	for _, call := range r.ToolCalls {
		toolCallTypes[call.Type]++
	}

	// Check if multiple types of tool calls are used
	typeCount := len(toolCallTypes)

	// In watch mode, no tool calls are expected except possibly ChangeState
	if m.WatchMode {
		// If any tool call other than ChangeState with NoComment is used, reject
		if typeCount > 0 && !(typeCount == 1 && toolCallTypes["ChangeState"] == 1) {
			return "You didn't follow the guidelines. In watch mode, you should only use ChangeState if needed. Pay attention!", false
		}
	} else {
		// Normal mode - there should be at least one tool call or boolean flag
		if typeCount+boolCount == 0 {
			return "You didn't follow the guidelines. You must call at least one function in your response. Pay attention!", false
		}
	}

	// Check if ExecCommand elements have max 120 characters
	for _, call := range r.ToolCalls {
		if call.Type == "ExecCommand" && len(call.Content) > 120 {
			return fmt.Sprintf("You didn't follow the guidelines. ExecCommand content should have max 120 characters, but you provided %d characters: Pay attention!", len(call.Content)), false
		}
	}

	// Check if TmuxSendKeys elements have max 120 characters
	tmuxSendKeysCount := 0
	for _, call := range r.ToolCalls {
		if call.Type == "TmuxSendKeys" {
			tmuxSendKeysCount++
			if len(call.Content) > 120 {
				return fmt.Sprintf("You didn't follow the guidelines. TmuxSendKeys content should have max 120 characters, but you provided %d characters: Pay attention!", len(call.Content)), false
			}
		}
	}

	// Check if there are max 5 TmuxSendKeys elements
	if tmuxSendKeysCount > 5 {
		return fmt.Sprintf("You didn't follow the guidelines. There should be max 5 TmuxSendKeys calls, but you provided %d calls. Pay attention!", tmuxSendKeysCount), false
	}

	return "", true
}
