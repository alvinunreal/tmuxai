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

		fmt.Fprintf(&currentTmuxWindow, "<%s>\n", title)
		fmt.Fprintf(&currentTmuxWindow, " - Id: %s\n", pane.Id)
		fmt.Fprintf(&currentTmuxWindow, " - CurrentPid: %d\n", pane.CurrentPid)
		fmt.Fprintf(&currentTmuxWindow, " - CurrentCommand: %s\n", pane.CurrentCommand)
		fmt.Fprintf(&currentTmuxWindow, " - CurrentCommandArgs: %s\n", pane.CurrentCommandArgs)
		fmt.Fprintf(&currentTmuxWindow, " - Shell: %s\n", pane.Shell)
		fmt.Fprintf(&currentTmuxWindow, " - OS: %s\n", pane.OS)
		fmt.Fprintf(&currentTmuxWindow, " - LastLine: %s\n", pane.LastLine)
		fmt.Fprintf(&currentTmuxWindow, " - IsActive: %d\n", pane.IsActive)
		fmt.Fprintf(&currentTmuxWindow, " - IsTmuxAiPane: %t\n", pane.IsTmuxAiPane)
		fmt.Fprintf(&currentTmuxWindow, " - IsTmuxAiExecPane: %t\n", pane.IsTmuxAiExecPane)
		fmt.Fprintf(&currentTmuxWindow, " - IsPrepared: %t\n", pane.IsPrepared)
		fmt.Fprintf(&currentTmuxWindow, " - IsSubShell: %t\n", pane.IsSubShell)
		fmt.Fprintf(&currentTmuxWindow, " - HistorySize: %d\n", pane.HistorySize)
		fmt.Fprintf(&currentTmuxWindow, " - HistoryLimit: %d\n", pane.HistoryLimit)

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
