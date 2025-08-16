package internal

import (
	"fmt"
	"strings"

	"github.com/alvinunreal/tmuxai/config"
	"github.com/alvinunreal/tmuxai/system"
)

func (m *Manager) GetTmuxPanes() ([]system.TmuxPaneDetails, error) {
	currentPaneId, _ := system.TmuxCurrentPaneId()
	windowTarget, _ := system.TmuxCurrentWindowTarget()
	currentPanes, _ := system.TmuxPanesDetails(windowTarget)

	for i := range currentPanes {
		currentPanes[i].IsTmuxAiPane = currentPanes[i].Id == currentPaneId
		currentPanes[i].IsTmuxAiExecPane = currentPanes[i].Id == m.ExecPane.Id
		currentPanes[i].IsPrepared = currentPanes[i].Id == m.ExecPane.Id
		if currentPanes[i].IsSubShell {
			currentPanes[i].OS = "OS Unknown (subshell)"
		} else {
			currentPanes[i].OS = m.OS
		}

	}
	return currentPanes, nil
}

func (m *Manager) getTmuxPanesInXmlFn(config *config.Config) string {
	currentTmuxWindow := strings.Builder{}
	currentTmuxWindow.WriteString("<current_tmux_window_state>\n")
	panes, _ := m.GetTmuxPanes()

	// Filter out tmuxai_pane
	var filteredPanes []system.TmuxPaneDetails
	for _, p := range panes {
		if !p.IsTmuxAiPane {
			filteredPanes = append(filteredPanes, p)
		}
	}
	for _, pane := range filteredPanes {
		if !pane.IsTmuxAiPane {
			pane.Refresh(m.GetMaxCaptureLines())
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
