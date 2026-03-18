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

func (m *Manager) shouldIncludeReadPane(pane system.TmuxPaneDetails) bool {
	if pane.IsTmuxAiPane {
		return false
	}
	if len(m.ForcedReadPaneIDs) == 0 {
		return true
	}
	if pane.IsTmuxAiExecPane {
		return true
	}
	return m.ForcedReadPaneIDs[pane.Id]
}

func (m *Manager) getTmuxPanesInXmlFn(config *config.Config) string {
	currentTmuxWindow := strings.Builder{}
	currentTmuxWindow.WriteString("<current_tmux_window_state>\n")
	panes, _ := m.GetTmuxPanes()

	// Filter out tmuxai_pane
	var filteredPanes []system.TmuxPaneDetails
	for _, p := range panes {
		if m.shouldIncludeReadPane(p) {
			filteredPanes = append(filteredPanes, p)
		}
	}
	for _, pane := range filteredPanes {
		if !pane.IsTmuxAiPane {
			pane.Refresh(m.GetMaxCaptureLines())
		}
		if pane.IsTmuxAiExecPane {
			m.ExecPane = &pane
			// Re-parse exec history in prepared mode so incremental tracking stays current
			if pane.IsPrepared && m.Config.IncrementalPaneContent {
				m.parseExecPaneCommandHistory()
			}
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
			if pane.IsTmuxAiExecPane && pane.IsPrepared && m.Config.IncrementalPaneContent && len(m.ExecHistory) > 0 {
				// Incremental mode: send only the new command output since last request
				unseen := m.ExecHistory[m.LastSentHistoryIndex:]
				if len(unseen) > 0 {
					for i, cmd := range unseen {
						idx := m.LastSentHistoryIndex + i
						currentTmuxWindow.WriteString("<command_output>\n")
						fmt.Fprintf(&currentTmuxWindow, " - Index: %d\n", idx)
						fmt.Fprintf(&currentTmuxWindow, " - Command: %s\n", cmd.Command)
						fmt.Fprintf(&currentTmuxWindow, " - ExitCode: %d\n", cmd.Code)
						currentTmuxWindow.WriteString(" - Output:\n")
						currentTmuxWindow.WriteString(cmd.Output)
						currentTmuxWindow.WriteString("\n</command_output>\n")
					}
					// Update the sentinel so next request only gets newer entries
					m.LastSentHistoryIndex = len(m.ExecHistory)
				}
			} else {
				// Default: send full pane content
				currentTmuxWindow.WriteString("<pane_content>\n")
				currentTmuxWindow.WriteString(pane.Content)
				currentTmuxWindow.WriteString("\n</pane_content>\n")
			}
		}

		fmt.Fprintf(&currentTmuxWindow, "</%s>\n\n", title)
	}

	currentTmuxWindow.WriteString("</current_tmux_window_state>\n")
	return currentTmuxWindow.String()
}
