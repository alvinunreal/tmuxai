package internal

import (
	"fmt"
	"strings"

	"github.com/alvinunreal/tmuxai/config"
	"github.com/alvinunreal/tmuxai/logger"
	"github.com/alvinunreal/tmuxai/system"
)

func (m *Manager) GetPanes() ([]system.PaneDetails, error) {
	logger.Debug("GetPanes: Starting to get panes")
	currentPaneId, _ := m.Multiplexer.GetCurrentPaneId()
	logger.Debug("GetPanes: Current pane ID='%s'", currentPaneId)
	
	currentPanes, err := m.Multiplexer.GetPaneDetails(m.TargetWindow)
	if err != nil {
		logger.Error("GetPanes: Error getting pane details: %v", err)
		return nil, err
	}
	
	logger.Debug("GetPanes: Retrieved %d panes from multiplexer", len(currentPanes))
	logger.Debug("GetPanes: ExecPane.Id='%s'", m.ExecPane.Id)

	for i := range currentPanes {
		oldIsTmuxAiPane := currentPanes[i].IsTmuxAiPane
		oldIsTmuxAiExecPane := currentPanes[i].IsTmuxAiExecPane
		
		currentPanes[i].IsTmuxAiPane = currentPanes[i].Id == currentPaneId
		currentPanes[i].IsTmuxAiExecPane = currentPanes[i].Id == m.ExecPane.Id
		currentPanes[i].IsPrepared = currentPanes[i].Id == m.ExecPane.Id
		
		logger.Debug("GetPanes: Pane %d - ID='%s', IsTmuxAiPane=%t->%t, IsTmuxAiExecPane=%t->%t", 
			i, currentPanes[i].Id, 
			oldIsTmuxAiPane, currentPanes[i].IsTmuxAiPane,
			oldIsTmuxAiExecPane, currentPanes[i].IsTmuxAiExecPane)
		
		if currentPanes[i].IsSubShell {
			currentPanes[i].OS = "OS Unknown (subshell)"
		} else {
			currentPanes[i].OS = m.OS
		}
	}
	return currentPanes, nil
}

func (m *Manager) GetPanesInXml(config *config.Config) string {
	currentWindow := strings.Builder{}
	currentWindow.WriteString("<current_multiplexer_window_state>\n")
	panes, err := m.GetPanes()
	if err != nil {
		currentWindow.WriteString(fmt.Sprintf("<!-- Error getting panes: %v -->\n", err))
		currentWindow.WriteString("</current_multiplexer_window_state>\n")
		return currentWindow.String()
	}

	// Filter out tmuxai_pane
	var filteredPanes []system.PaneDetails
	for _, p := range panes {
		if !p.IsTmuxAiPane {
			filteredPanes = append(filteredPanes, p)
		}
	}
	for _, pane := range filteredPanes {
		if !pane.IsTmuxAiPane {
			err := pane.Refresh(m.Multiplexer, m.GetMaxCaptureLines())
			if err != nil {
				// Log error but continue processing
				currentWindow.WriteString(fmt.Sprintf("<!-- Error refreshing pane %s: %v -->\n", pane.Id, err))
			}
		}
		if pane.IsTmuxAiExecPane {
			m.ExecPane = &pane
		}

		var title string
		if pane.IsTmuxAiExecPane {
			title = "tmuxai_exec_pane"
		} else {
			title = "read_only_pane"
		}

		currentWindow.WriteString(fmt.Sprintf("<%s>\n", title))
		currentWindow.WriteString(fmt.Sprintf(" - Id: %s\n", pane.Id))
		currentWindow.WriteString(fmt.Sprintf(" - CurrentPid: %d\n", pane.CurrentPid))
		currentWindow.WriteString(fmt.Sprintf(" - CurrentCommand: %s\n", pane.CurrentCommand))
		currentWindow.WriteString(fmt.Sprintf(" - CurrentCommandArgs: %s\n", pane.CurrentCommandArgs))
		currentWindow.WriteString(fmt.Sprintf(" - Shell: %s\n", pane.Shell))
		currentWindow.WriteString(fmt.Sprintf(" - OS: %s\n", pane.OS))
		currentWindow.WriteString(fmt.Sprintf(" - LastLine: %s\n", pane.LastLine))
		currentWindow.WriteString(fmt.Sprintf(" - IsActive: %d\n", pane.IsActive))
		currentWindow.WriteString(fmt.Sprintf(" - IsTmuxAiPane: %t\n", pane.IsTmuxAiPane))
		currentWindow.WriteString(fmt.Sprintf(" - IsTmuxAiExecPane: %t\n", pane.IsTmuxAiExecPane))
		currentWindow.WriteString(fmt.Sprintf(" - IsPrepared: %t\n", pane.IsPrepared))
		currentWindow.WriteString(fmt.Sprintf(" - IsSubShell: %t\n", pane.IsSubShell))
		currentWindow.WriteString(fmt.Sprintf(" - HistorySize: %d\n", pane.HistorySize))
		currentWindow.WriteString(fmt.Sprintf(" - HistoryLimit: %d\n", pane.HistoryLimit))
		currentWindow.WriteString(fmt.Sprintf(" - MultiplexerType: %s\n", pane.MultiplexerType.String()))

		if !pane.IsTmuxAiPane && pane.Content != "" {
			currentWindow.WriteString("<pane_content>\n")
			currentWindow.WriteString(pane.Content)
			currentWindow.WriteString("\n</pane_content>\n")
		}

		currentWindow.WriteString(fmt.Sprintf("</%s>\n\n", title))
	}

	currentWindow.WriteString("</current_multiplexer_window_state>\n")
	return currentWindow.String()
}
