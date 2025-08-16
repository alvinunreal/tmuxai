package internal

import (
	"context"
	"fmt"
	"time"

	"github.com/alvinunreal/tmuxai/logger"
	"github.com/alvinunreal/tmuxai/system"
	"github.com/briandowns/spinner"
)

// Main function to process regular user messages
// Returns true if the request was accomplished and no further processing should happen
func (m *Manager) ProcessUserMessage(ctx context.Context, message string) bool {
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

	currentTmuxWindow := m.getTmuxPanesInXml(m.Config)
	execPaneEnv := ""
	if !m.ExecPane.IsSubShell {
		execPaneEnv = fmt.Sprintf("Keep in mind, you are working within the shell: %s and OS: %s", m.ExecPane.Shell, m.ExecPane.OS)
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
		history = []ChatMessage{m.chatAssistantPrompt(true)}
	default:
		history = []ChatMessage{m.chatAssistantPrompt(false)}
	}

	history = append(history, m.Messages...)

	sending := append(history, currentMessage)

	response, err := m.AiClient.GetResponseFromChatMessages(ctx, sending, m.GetOpenRouterModel())
	if err != nil {
		s.Stop()
		m.Status = ""

		if ctx.Err() == context.Canceled {
			return false
		}

		// Log both to console and debug file to capture error context
		errMsg := "Failed to get response from AI: " + err.Error()
		fmt.Println(errMsg)

		// Debug the failed request even when there's an error
		if m.Config.Debug {
			debugChatMessages(append(history, currentMessage), "ERROR: "+err.Error())
		}

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

		// Log both to console and debug file
		errMsg := "Failed to parse AI response: " + err.Error()
		fmt.Println(errMsg)

		// Debug the failed parsing even when there's an error
		if m.Config.Debug {
			debugChatMessages(append(history, currentMessage), "PARSE ERROR: "+response)
		}

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

	// did AI follow our guidelines?
	guidelineError, validResponse := m.aiFollowedGuidelines(r)
	if !validResponse {
		m.Println("AI didn't follow guidelines, trying again...")
		m.Messages = append(m.Messages, currentMessage, responseMsg)
		return m.ProcessUserMessage(ctx, guidelineError)

	}

	// colorize code blocks in the response
	if r.Message != "" {
		fmt.Println(system.Cosmetics(r.Message))
	}

	// Don't append to history if AI is waiting for the pane or is watch mode no comment
	if r.ExecPaneSeemsBusy || r.NoComment {
	} else {
		m.Messages = append(m.Messages, currentMessage, responseMsg)
	}

	// observe/prepared mode
	for _, execCommand := range r.ExecCommand {
		code, _ := system.HighlightCode("sh", execCommand)
		m.Println(code)

		isSafe := false
		command := execCommand
		if m.GetExecConfirm() {
			isSafe, command = m.confirmedToExec(execCommand, "Execute this command?", true)
		} else {
			isSafe = true
		}
		if isSafe {
			m.Println("Executing command: " + command)
			if m.ExecPane.IsPrepared {
				_, _ = m.ExecWaitCapture(command)
			} else {
				_ = system.TmuxSendCommandToPane(m.ExecPane.Id, command, true)
				time.Sleep(1 * time.Second)
			}
		} else {
			m.Status = ""
			return false
		}
	}

	// Process SendKeys
	if len(r.SendKeys) > 0 {
		// Show preview of all keys
		keysPreview := "Keys to send:\n"
		for i, sendKey := range r.SendKeys {
			code, _ := system.HighlightCode("txt", sendKey)
			if i == len(r.SendKeys)-1 {
				keysPreview += code
			} else {
				keysPreview += code + "\n"
			}
			if m.Status == "" {
				return false
			}
		}

		m.Println(keysPreview)

		// Determine confirmation message based on number of keys
		confirmMessage := "Send this key?"
		if len(r.SendKeys) > 1 {
			confirmMessage = "Send all these keys?"
		}

		// Get confirmation if required
		var allConfirmed bool
		if m.GetSendKeysConfirm() {
			allConfirmed, _ = m.confirmedToExec("keys shown above", confirmMessage, true)
			if !allConfirmed {
				m.Status = ""
				return false
			}
		}

		// Send each key with delay
		for _, sendKey := range r.SendKeys {
			m.Println("Sending keys: " + sendKey)
			_ = system.TmuxSendCommandToPane(m.ExecPane.Id, sendKey, false)
			time.Sleep(1 * time.Second)
		}
	}

	if r.ExecPaneSeemsBusy {
		m.Countdown(m.GetWaitInterval())
		// Create a new context for this recursive call
		newCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		accomplished := m.ProcessUserMessage(newCtx, "waited for 5 more seconds, here is the current pane(s) content")
		if accomplished {
			return true
		}
	}

	// observe or prepared mode
	if r.PasteMultilineContent != "" {
		code, _ := system.HighlightCode("txt", r.PasteMultilineContent)
		fmt.Println(code)

		isSafe := false
		if m.GetPasteMultilineConfirm() {
			isSafe, _ = m.confirmedToExec(r.PasteMultilineContent, "Paste multiline content?", false)
		} else {
			isSafe = true
		}

		if isSafe {
			m.Println("Pasting...")
			_ = system.TmuxSendCommandToPane(m.ExecPane.Id, r.PasteMultilineContent, true)
			time.Sleep(1 * time.Second)
		} else {
			m.Status = ""
			return false
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

	if !m.WatchMode {
		accomplished := m.ProcessUserMessage(ctx, "sending updated pane(s) content")
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

	// Create a new background context since this is a separate process
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	accomplished := m.ProcessUserMessage(ctx, desc)
	if accomplished {
		m.WatchMode = false
		m.Status = ""
	}

	// we continue running if status is still set
	if m.Status != "" && m.WatchMode {
		m.startWatchMode("")
	}
}

func (m *Manager) aiFollowedGuidelines(r AIResponse) (string, bool) {
	// Check if only one boolean is true in AI response
	boolCount := 0
	if r.RequestAccomplished {
		boolCount++
	}
	if r.ExecPaneSeemsBusy {
		boolCount++
	}
	if r.WaitingForUserResponse {
		boolCount++
	}
	if r.NoComment {
		boolCount++
	}

	if boolCount > 1 {
		return "You didn't follow the guidelines. Only one boolean flag should be set to true in your response. Pay attention!", false
	}

	// Check if only one tag is used
	tags := []int{len(r.ExecCommand), len(r.SendKeys)}
	if r.PasteMultilineContent != "" {
		tags = append(tags, 1)
	} else {
		tags = append(tags, 0)
	}
	count := 0
	for _, len := range tags {
		if len > 0 {
			count++
		}
	}

	if count > 1 {
		return "You didn't follow the guidelines. You can only use one type of XML tag in your response. Pay attention!", false
	}

	// watch mode has no xml tags, otherwise should be at least 1 xml tag in response
	if !m.WatchMode && count+boolCount == 0 {
		return "You didn't follow the guidelines. You must use at least one XML tag in your response. Pay attention!", false
	}

	return "", true
}
