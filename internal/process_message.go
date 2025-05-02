package internal

import (
	"fmt"
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
	if !m.ExecPane.IsSubShell {
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

	response, err := m.AiClient.GetResponseFromChatMessages(sending, m.GetOpenRouterModel())
	if err != nil {
		s.Stop()
		m.Status = ""
		fmt.Println("Failed to get response from AI: " + err.Error())
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
		Content:   response,
		FromUser:  false,
		Timestamp: time.Now(),
	}

	// Don't append to history if AI is waiting for the pane or is watch mode no comment
	if r.State == "ExecPaneSeemsBusy" || r.State == "NoComment" {
	} else {
		m.Messages = append(m.Messages, currentMessage, responseMsg)
	}

	// --- Execute Action Sequence ---
	for _, step := range r.Sequence {
		// Check status before each step
		if m.Status == "" {
			logger.Info("Status changed, aborting sequence execution.")
			return false
		}

		switch step.Type {
		case "message":
			// colorize code blocks in the message
			if step.Content != "" {
				m.Println(system.Cosmetics(step.Content))
			}
		case "execCommand":
			code, _ := system.HighlightCode("sh", step.Content)
			m.Println(code)

			isSafe := false
			command := step.Content
			if m.GetExecConfirm() {
				isSafe, command = m.confirmedToExec(step.Content, "Execute this command?", true)
			} else {
				isSafe = true
			}
			if isSafe {
				fmt.Println("Executing command: " + command)
				system.TmuxSendCommandToPane(m.ExecPane.Id, command, true)
				time.Sleep(1 * time.Second) // Consider making delay configurable or smarter
				m.Println("")
			} else {
				m.Status = ""
				return false // User cancelled
			}
		case "sendKeys":
			code, _ := system.HighlightCode("txt", step.Content)
			m.Println(code)

			isSafe := false
			keys := step.Content
			if m.GetSendKeysConfirm() {
				isSafe, keys = m.confirmedToExec(step.Content, "Send this key(s)?", true)
			} else {
				isSafe = true
			}
			if isSafe {
				fmt.Println("Sending keys: " + keys)
				system.TmuxSendCommandToPane(m.ExecPane.Id, keys, false) // Note: send keys usually don't need Enter appended
				time.Sleep(1 * time.Second)                              // Consider making delay configurable or smarter
			} else {
				m.Status = ""
				return false // User cancelled
			}
		// Prepare mode only
		case "execAndWait":
			code, _ := system.HighlightCode("sh", step.Content)
			fmt.Println(code)

			isSafe := false
			command := step.Content
			if m.GetExecConfirm() {
				isSafe, command = m.confirmedToExec(step.Content, "Execute this command and wait?", true)
			} else {
				isSafe = true
			}
			if isSafe {
				m.ExecWaitCapture(command) // This function handles its own waiting/processing
			} else {
				m.Status = ""
				return false // User cancelled
			}
		case "pasteMultiline":
			code, _ := system.HighlightCode("txt", step.Content)
			fmt.Println(code)

			isSafe := false
			content := step.Content
			if m.GetPasteMultilineConfirm() {
				isSafe, _ = m.confirmedToExec(content, "Paste multiline content?", false)
			} else {
				isSafe = true
			}

			if isSafe {
				m.Println("Pasting...")
				system.TmuxSendCommandToPane(m.ExecPane.Id, content, true) // Assuming paste needs Enter implicitly handled by content or context
				time.Sleep(1 * time.Second)                                // Consider making delay configurable or smarter
			} else {
				m.Status = ""
				return false // User cancelled
			}
		default:
			logger.Error("Unknown action step type in sequence: %s", step.Type)
		}
	}
	// --- End Action Sequence Execution ---

	// --- Handle Final State ---
	if r.State == "ExecPaneSeemsBusy" {
		m.Countdown(m.GetWaitInterval())
		accomplished := m.ProcessUserMessage("waited for 5 more seconds, here is the current pane(s) content")
		if accomplished {
			return true
		}
	}

	if r.State == "RequestAccomplished" {
		m.Status = ""
		return true
	}

	if r.State == "WaitingForUserResponse" {
		m.Status = "question"
		return false // Stop processing, wait for user input
	}

	if r.State == "NoComment" {
		return false
	}

	if !m.WatchMode {
		accomplished := m.ProcessUserMessage("sending updated pane(s) content")
		if accomplished {
			return true
		}
	}
	return false
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
