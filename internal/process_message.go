package internal

import (
	"context"
	"fmt"
	"strings"
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

	currentTmuxWindow := m.GetTmuxPanesInXml(m.Config)
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
	case m.GetAgenticMode():
		history = []ChatMessage{m.agenticPrompt()}
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

	if r.CreateExecPane {
		m.CreateNewExecPane()
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
		var targetPane *system.TmuxPaneDetails
		if execCommand.PaneID != "" {
			// Find the pane details for this ID.
			panes, _ := m.GetTmuxPanes()
			found := false

			// Normalize the pane ID from the AI, which might be missing the '%' prefix.
			normalizedPaneID := execCommand.PaneID
			if !strings.HasPrefix(normalizedPaneID, "%") {
				normalizedPaneID = "%" + normalizedPaneID
			}

			for i, p := range panes {
				if p.Id == normalizedPaneID { // Compare against the normalized ID
					targetPane = &panes[i]
					found = true
					break
				}
			}
			if !found {
				// Use the original PaneID in the error message for clarity.
				m.Println(fmt.Sprintf("Error: Could not find target pane with ID %s", execCommand.PaneID))
				continue
			}
		} else {
			// Default to the primary exec pane
			targetPane = m.ExecPane
		}

		code, _ := system.HighlightCode("sh", execCommand.Command)
		m.Println(code)

		isSafe := false
		command := execCommand.Command
		confirmPrompt := "Execute this command?"
		if m.GetAgenticMode() {
			confirmPrompt = fmt.Sprintf("Execute this command in pane %s?", targetPane.Id)
		}

		if m.GetExecConfirm() {
			isSafe, command = m.confirmedToExec(execCommand.Command, confirmPrompt, true)
		} else {
			isSafe = true
		}
		if isSafe {
			m.Println(fmt.Sprintf("Executing in pane %s: %s", targetPane.Id, command))
			m.LastExecPaneID = targetPane.Id

			targetPane.Refresh(m.GetMaxCaptureLines())
			if targetPane.IsPrepared {
				m.ExecWaitCapture(command, targetPane)
			} else {
				system.TmuxSendCommandToPane(targetPane.Id, command, true)
				time.Sleep(1 * time.Second)
			}
		} else {
			m.Status = ""
			return false
		}
	}

	// Process SendKeys
	if len(r.SendKeys) > 0 {
		// Group keys by pane for confirmation
		keysByPane := make(map[string][]string)
		paneOrder := []string{} // Preserve order
		for _, sk := range r.SendKeys {
			paneID := sk.PaneID
			if paneID == "" {
				paneID = m.ExecPane.Id // Default to primary exec pane
			}
			if _, exists := keysByPane[paneID]; !exists {
				paneOrder = append(paneOrder, paneID)
			}
			keysByPane[paneID] = append(keysByPane[paneID], sk.Keys)
		}

		// Confirm and execute for each pane
		for _, paneID := range paneOrder {
			keys := keysByPane[paneID]

			// Normalize the pane ID from the AI before using it.
			normalizedPaneID := paneID
			if !strings.HasPrefix(normalizedPaneID, "%") {
				normalizedPaneID = "%" + normalizedPaneID
			}

			keysPreview := fmt.Sprintf("Keys to send to pane %s:\n", paneID)
			for i, key := range keys {
				code, _ := system.HighlightCode("txt", key)
				if i == len(keys)-1 {
					keysPreview += code
				} else {
					keysPreview += code + "\n"
				}
			}
			m.Println(keysPreview)

			confirmMessage := fmt.Sprintf("Send these keys to pane %s?", paneID)
			if len(keys) == 1 {
				confirmMessage = fmt.Sprintf("Send this key to pane %s?", paneID)
			}

			allConfirmed := true
			if m.GetSendKeysConfirm() {
				allConfirmed, _ = m.confirmedToExec("keys shown above", confirmMessage, false) // No edit for keys
				if !allConfirmed {
					m.Status = ""
					return false // Abort all further actions
				}
			}

			// Send each key with delay
			for _, sendKey := range keys {
				m.Println(fmt.Sprintf("Sending to %s: %s", paneID, sendKey))
				system.TmuxSendCommandToPane(normalizedPaneID, sendKey, false)
				time.Sleep(1 * time.Second)
			}
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

	// Process PasteMultilineContent
	if len(r.PasteMultilineContent) > 0 {
		for _, pc := range r.PasteMultilineContent {
			targetPaneID := pc.PaneID
			if targetPaneID == "" {
				targetPaneID = m.ExecPane.Id // Default to primary exec pane
			}

			// Normalize the pane ID from the AI before using it.
			normalizedPaneID := targetPaneID
			if !strings.HasPrefix(normalizedPaneID, "%") {
				normalizedPaneID = "%" + normalizedPaneID
			}

			code, _ := system.HighlightCode("txt", pc.Content)
			m.Println(fmt.Sprintf("Content to paste into pane %s:", targetPaneID))
			fmt.Println(code)

			isSafe := false
			if m.GetPasteMultilineConfirm() {
				isSafe, _ = m.confirmedToExec(pc.Content, fmt.Sprintf("Paste this content into pane %s?", targetPaneID), false)
			} else {
				isSafe = true
			}

			if isSafe {
				m.Println("Pasting...")
				system.TmuxPasteToPane(normalizedPaneID, pc.Content)
			} else {
				m.Status = ""
				return false
			}
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
	// Count state tags. Rule: Max 1 state tag.
	stateTags := 0
	if r.RequestAccomplished {
		stateTags++
	}
	if r.ExecPaneSeemsBusy {
		stateTags++
	}
	if r.WaitingForUserResponse {
		stateTags++
	}
	if r.NoComment {
		stateTags++
	}

	if stateTags > 1 {
		return "AI Error: Only one of <RequestAccomplished>, <ExecPaneSeemsBusy>, <WaitingForUserResponse>, or <NoComment> can be used at a time.", false
	}

	// Count action tags. Rule: Max 1 main action, can be combined with CreateExecPane.
	mainActionTags := 0
	if len(r.ExecCommand) > 0 {
		mainActionTags++
	}
	if len(r.SendKeys) > 0 {
		mainActionTags++
	}
	if len(r.PasteMultilineContent) > 0 {
		mainActionTags++
	}

	if mainActionTags > 1 {
		return "AI Error: Only one of <ExecCommand>, <TmuxSendKeys>, or <PasteMultilineContent> can be used at a time.", false
	}

	// Rule: State tags cannot be mixed with any action tags (including CreateExecPane).
	totalActionTags := mainActionTags
	if r.CreateExecPane {
		totalActionTags++
	}

	if stateTags > 0 && totalActionTags > 0 {
		return "AI Error: State tags (like <RequestAccomplished>) cannot be combined with action tags (like <ExecCommand> or <CreateExecPane>).", false
	}

	// Rule: A response must contain at least one tag.
	if stateTags == 0 && totalActionTags == 0 {
		return "AI Error: The response must contain at least one valid XML tag.", false
	}

	return "", true
}
