package internal

import (
	"fmt"
	"strings"

	"github.com/alvinunreal/tmuxai/config"
	"github.com/alvinunreal/tmuxai/logger"
	"github.com/alvinunreal/tmuxai/system"
)

// GetCurrentWindowPanes retrieves details for panes in the current window.
// Note: For Zellij, this might be limited to the current pane if GetPaneDetails("") doesn't list others.
func (m *Manager) GetCurrentWindowPanes() ([]system.PaneDetails, error) {
	// m.CurrentPaneId should be reliable.
	// For Tmux, GetPaneDetails(m.CurrentPaneId) should ideally list panes in the same window.
	// If GetPaneDetails needs a window target, this might need adjustment or a new Mux method.
	// For Zellij, GetPaneDetails(m.CurrentPaneId) will likely return only the current pane.
	// Passing an empty string "" as target might signify "current context" (e.g. current window for tmux)
	// This depends on the specific multiplexer's GetPaneDetails implementation.
	// Let's assume for now passing m.CurrentPaneId is specific enough for Tmux to list window panes
	// or for Zellij to return its single pane detail. A more robust solution might involve
	// a Mux method like GetWindowPanes(windowId string) or GetCurrentWindowPanes().
	// For now, we use m.CurrentPaneId as the target, acknowledging potential limitations.
	target := m.CurrentPaneId // Or determine window target if Mux interface evolves

	currentPanes, err := m.Mux.GetPaneDetails(target)
	if err != nil {
		logger.Error("Failed to get pane details using target '%s': %v", target, err)
		return nil, err
	}

	for i := range currentPanes {
		currentPanes[i].IsTmuxAiPane = currentPanes[i].Id == m.CurrentPaneId
		if m.ExecPane != nil { // Ensure ExecPane is initialized
			currentPanes[i].IsTmuxAiExecPane = currentPanes[i].Id == m.ExecPane.Id
			currentPanes[i].IsPrepared = currentPanes[i].Id == m.ExecPane.Id && m.ExecPane.IsPrepared
		} else {
			currentPanes[i].IsTmuxAiExecPane = false
			currentPanes[i].IsPrepared = false
		}
		// Prioritize OS from PaneDetails if available, otherwise use manager's general OS.
		if currentPanes[i].OS == "" {
			currentPanes[i].OS = m.OS
		}
	}
	return currentPanes, nil
}

// GetCurrentWindowPanesInXml generates an XML representation of panes in the current window.
func (m *Manager) GetCurrentWindowPanesInXml(config *config.Config) string {
	currentTmuxWindow := strings.Builder{}
	currentTmuxWindow.WriteString("<current_tmux_window_state>\n")
	panes, err := m.GetCurrentWindowPanes()
	if err != nil {
		logger.Error("Could not get current window panes for XML: %v", err)
		currentTmuxWindow.WriteString(fmt.Sprintf("<error>Could not retrieve pane details: %v</error>\n", err))
		currentTmuxWindow.WriteString("</current_tmux_window_state>\n")
		return currentTmuxWindow.String()
	}

	// Filter out the main AI pane (where tmuxai runs)
	var filteredPanes []system.PaneDetails
	for _, p := range panes {
		if !p.IsTmuxAiPane { // IsTmuxAiPane is set in GetCurrentWindowPanes
			filteredPanes = append(filteredPanes, p)
		}
	}

	for i := range filteredPanes { // Iterate by index to modify the slice directly if needed, though CapturePane updates a field.
		pane := &filteredPanes[i] // Use a pointer to modify the pane in the slice.

		// Capture content for non-AI, non-Exec panes if not already fresh.
		// The ExecPane's content is managed by ExecWaitCapture/PrepareExecPane.
		// This avoids excessive capturing for panes not directly interacted with.
		if !pane.IsTmuxAiPane && (m.ExecPane == nil || pane.Id != m.ExecPane.Id) {
			// Only capture if content is empty or considered stale.
			// For simplicity, let's capture it here. More complex staleness logic could be added.
			content, err := m.Mux.CapturePane(pane.Id, m.GetMaxCaptureLines())
			if err != nil {
				logger.Warning("Failed to capture content for pane %s: %v", pane.Id, err)
				pane.Content = fmt.Sprintf("Error capturing content: %v", err)
			} else {
				pane.Content = content
			}
		}

		// Update m.ExecPane if this iteration is the exec pane
		if m.ExecPane != nil && pane.Id == m.ExecPane.Id {
			// Ensure m.ExecPane points to the instance in filteredPanes to reflect content updates
			// However, m.ExecPane is updated more actively by other functions.
			// Here, we primarily care about its representation in the XML.
			// If filteredPanes[i] is the exec pane, its details (like content) might have just been updated.
		}


		var title string
		if pane.IsTmuxAiExecPane { // This field is set by GetCurrentWindowPanes
			title = "tmuxai_exec_pane"
		} else {
			title = "read_only_pane"
		}

		var title string
		if pane.IsTmuxAiExecPane {
			title = "tmuxai_exec_pane"
		} else {
			title = "read_only_pane"
		}

		currentTmuxWindow.WriteString(fmt.Sprintf("<%s>\n", title))
		currentTmuxWindow.WriteString(fmt.Sprintf(" - Id: %s\n", pane.Id))
		currentTmuxWindow.WriteString(fmt.Sprintf(" - CurrentPid: %d\n", pane.CurrentPid))
		currentTmuxWindow.WriteString(fmt.Sprintf(" - CurrentCommand: %s\n", pane.CurrentCommand))
		currentTmuxWindow.WriteString(fmt.Sprintf(" - CurrentCommandArgs: %s\n", pane.CurrentCommandArgs))
		currentTmuxWindow.WriteString(fmt.Sprintf(" - Shell: %s\n", pane.Shell))
		currentTmuxWindow.WriteString(fmt.Sprintf(" - OS: %s\n", pane.OS))
		currentTmuxWindow.WriteString(fmt.Sprintf(" - LastLine: %s\n", pane.LastLine))
		currentTmuxWindow.WriteString(fmt.Sprintf(" - IsActive: %d\n", pane.IsActive))
		currentTmuxWindow.WriteString(fmt.Sprintf(" - IsTmuxAiPane: %t\n", pane.IsTmuxAiPane))
		currentTmuxWindow.WriteString(fmt.Sprintf(" - IsTmuxAiExecPane: %t\n", pane.IsTmuxAiExecPane))
		currentTmuxWindow.WriteString(fmt.Sprintf(" - IsPrepared: %t\n", pane.IsPrepared))
		currentTmuxWindow.WriteString(fmt.Sprintf(" - IsSubShell: %t\n", pane.IsSubShell))
		currentTmuxWindow.WriteString(fmt.Sprintf(" - HistorySize: %d\n", pane.HistorySize))
		currentTmuxWindow.WriteString(fmt.Sprintf(" - HistoryLimit: %d\n", pane.HistoryLimit))

		if !pane.IsTmuxAiPane && pane.Content != "" {
			currentTmuxWindow.WriteString("<pane_content>\n")
			currentTmuxWindow.WriteString(pane.Content)
			currentTmuxWindow.WriteString("\n</pane_content>\n")
		}

		currentTmuxWindow.WriteString(fmt.Sprintf("</%s>\n\n", title))
	}

	currentTmuxWindow.WriteString("</current_tmux_window_state>\n")
	return currentTmuxWindow.String()
}
