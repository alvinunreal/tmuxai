package internal

import (
	"bufio"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/alvinunreal/tmuxai/logger"
	"github.com/alvinunreal/tmuxai/system"
)

// refreshExecPaneDetails updates the ExecPane's details, including content.
func (m *Manager) refreshExecPaneDetails() error {
	if m.ExecPane == nil || m.ExecPane.Id == "" {
		return fmt.Errorf("exec pane not initialized")
	}

	content, err := m.Mux.CapturePane(m.ExecPane.Id, m.GetMaxCaptureLines())
	if err != nil {
		logger.Error("Failed to capture exec pane %s content: %v", m.ExecPane.Id, err)
		// Keep stale content if capture fails, or clear it? For now, keep.
		// m.ExecPane.Content = "" // Or some error message
	} else {
		m.ExecPane.Content = content
		lines := strings.Split(strings.TrimSpace(content), "\n")
		if len(lines) > 0 {
			m.ExecPane.LastLine = lines[len(lines)-1]
		} else {
			m.ExecPane.LastLine = ""
		}
	}

	// Get other details like PID, command, etc.
	// This might overwrite Content if PaneDetails from GetPaneDetails also includes it.
	// Ensure CapturePane's content is preserved or GetPaneDetails doesn't fetch full content.
	// For now, assume GetPaneDetails provides metadata primarily.
	details, err := m.Mux.GetPaneDetails(m.ExecPane.Id)
	if err != nil {
		logger.Error("Failed to get exec pane %s details: %v", m.ExecPane.Id, err)
		return fmt.Errorf("failed to get exec pane details for %s: %w", m.ExecPane.Id, err)
	}

	if len(details) > 0 {
		originalContent := m.ExecPane.Content // Preserve captured content
		originalLastLine := m.ExecPane.LastLine
		isPrepared := m.ExecPane.IsPrepared // Preserve IsPrepared status

		*m.ExecPane = details[0] // Update metadata

		m.ExecPane.Content = originalContent // Restore potentially more complete captured content
		m.ExecPane.LastLine = originalLastLine
		m.ExecPane.IsPrepared = isPrepared // Restore IsPrepared
	} else {
		logger.Warning("No details returned for exec pane %s", m.ExecPane.Id)
		return fmt.Errorf("no details returned for exec pane %s", m.ExecPane.Id)
	}
	return nil
}


// GetAvailablePane finds an available pane (not the AI pane itself).
// It prefers a pane that is not the current AI pane.
func (m *Manager) GetAvailablePane() (system.PaneDetails, error) {
	// Uses GetCurrentWindowPanes, which uses m.Mux.GetPaneDetails
	panes, err := m.GetCurrentWindowPanes()
	if err != nil {
		logger.Error("Failed to get current window panes: %v", err)
		return system.PaneDetails{}, err
	}

	for _, pane := range panes {
		// IsTmuxAiPane is set by GetCurrentWindowPanes
		if !pane.IsTmuxAiPane {
			logger.Info("Found available pane: %s", pane.Id)
			return pane, nil
		}
	}
	// If only the AI pane exists, that's an issue handled by InitExecPane creating a new one.
	return system.PaneDetails{}, fmt.Errorf("no available pane found (other than the AI pane itself)")
}

func (m *Manager) InitExecPane() {
	if m.ExecPane == nil { // Should have been initialized to &system.PaneDetails{} in NewManager
		m.ExecPane = &system.PaneDetails{}
	}

	availablePane, err := m.GetAvailablePane()
	if err != nil || availablePane.Id == "" {
		logger.Info("No suitable existing pane found or error: %v. Creating new pane for execution.", err)
		newPaneId, createErr := m.Mux.CreateNewPane(m.CurrentPaneId, "") // No specific command for exec pane initially
		if createErr != nil {
			logger.Fatal("Failed to create new exec pane: %v", createErr)
			// This is a fatal error, tmuxai cannot function without an exec pane.
			// Consider how to handle this, perhaps by exiting or returning an error up the chain.
			// For now, assume logger.Fatal exits or panics.
			return
		}
		logger.Info("Created new exec pane with ID: %s", newPaneId)
		// After creating, we need its details.
		newPaneDetails, detailsErr := m.Mux.GetPaneDetails(newPaneId)
		if detailsErr != nil || len(newPaneDetails) == 0 {
			logger.Fatal("Failed to get details for newly created exec pane %s: %v", newPaneId, detailsErr)
			return
		}
		m.ExecPane = &newPaneDetails[0]
	} else {
		logger.Info("Using existing pane %s as exec pane.", availablePane.Id)
		m.ExecPane = &availablePane
	}
	m.ExecPane.IsTmuxAiExecPane = true // Mark it as such
	// Preparation (like setting PS1) will be done in PrepareExecPane
}


func (m *Manager) PrepareExecPane() {
	if m.ExecPane == nil || m.ExecPane.Id == "" {
		logger.Error("PrepareExecPane: ExecPane not initialized.")
		return
	}

	if err := m.refreshExecPaneDetails(); err != nil {
		logger.Error("PrepareExecPane: Failed to refresh exec pane details: %v", err)
		// Decide if we can proceed or should return.
		// If shell is not known, PS1 modification might fail or be incorrect.
	}

	if m.ExecPane.IsPrepared && m.ExecPane.Shell != "" {
		logger.Debug("Exec pane %s already prepared for shell %s.", m.ExecPane.Id, m.ExecPane.Shell)
		return
	}

	shellCommand := m.ExecPane.Shell // Shell should be populated by refreshExecPaneDetails via GetPaneDetails
	if shellCommand == "" {
		// Attempt to get it again if it was empty
		details, err := m.Mux.GetPaneDetails(m.ExecPane.Id)
		if err == nil && len(details) > 0 {
			shellCommand = details[0].Shell
			m.ExecPane.Shell = shellCommand // Update it
		}
		if shellCommand == "" {
			logger.Warning("PrepareExecPane: Shell type for exec pane %s is unknown. Cannot set custom PS1.", m.ExecPane.Id)
			return
		}
	}


	var ps1Command string
	switch shellCommand {
	case "zsh", "bash": // Assuming similar PS1 setup for simplicity here
		// This PS1 is basic. A more robust one would check existing PS1 or use shell-specific methods.
		// Example PS1 that includes status code, time, user, host, path
		// For bash: export PS1='\[\e[32m\]\u@\h\[\e[00m\]:\[\e[34m\]\w\[\e[00m\][\A][\$?]» '
		// For zsh: export PROMPT='%F{green}%n@%m%f:%F{blue}%~%f[%D{%H:%M}][%?]» '
		// Using a simpler, compatible version for now.
		ps1Command = `PS1='AIReady[%?]» '`
		if shellCommand == "zsh" {
			ps1Command = `PROMPT='AIReady[%?]» '`
		}

	case "fish":
		// Fish prompt is a function.
		ps1Command = `function fish_prompt; set -l last_status $status; echo "AIReady["$last_status"]» "; end`
	default:
		logger.Info("Shell '%s' in exec pane %s is recognized but not yet supported for PS1 modification by this agent.", shellCommand, m.ExecPane.Id)
		// We can still mark it as prepared if we don't want to enforce PS1 for all shells.
		// m.ExecPane.IsPrepared = true
		return // Or, don't set IsPrepared and let user handle it.
	}

	logger.Debug("Preparing exec pane %s (shell: %s) with PS1 command: %s", m.ExecPane.Id, shellCommand, ps1Command)
	if err := m.Mux.SendCommandToPane(m.ExecPane.Id, ps1Command, true); err != nil {
		logger.Error("Failed to set PS1 for exec pane %s: %v", m.ExecPane.Id, err)
		return
	}
	// Send Ctrl+L to clear the screen after setting PS1
	if err := m.Mux.SendCommandToPane(m.ExecPane.Id, "C-l", false); err != nil {
		logger.Warning("Failed to send Ctrl-L to exec pane %s after PS1 set: %v", m.ExecPane.Id, err)
	}
	m.ExecPane.IsPrepared = true
	logger.Info("Exec pane %s prepared for shell %s.", m.ExecPane.Id, shellCommand)
}

func (m *Manager) ExecWaitCapture(command string) (CommandExecHistory, error) {
	if m.ExecPane == nil || m.ExecPane.Id == "" {
		return CommandExecHistory{}, fmt.Errorf("exec pane not initialized")
	}
	if !m.ExecPane.IsPrepared {
		logger.Warning("ExecWaitCapture: Exec pane %s is not prepared (PS1 might not be set). Prompt detection may fail.", m.ExecPane.Id)
	}

	if err := m.Mux.SendCommandToPane(m.ExecPane.Id, command, true); err != nil {
		return CommandExecHistory{}, fmt.Errorf("failed to send command to exec pane %s: %w", m.ExecPane.Id, err)
	}

	// Initial refresh to get the command echo if possible
	if err := m.refreshExecPaneDetails(); err != nil {
		logger.Warning("ExecWaitCapture: Failed to refresh exec pane details after sending command: %v", err)
	}

	m.Println("") // Newline in the AI pane for cleaner status update

	animChars := []string{"⋯", "⋱", "⋮", "⋰"}
	animIndex := 0
	startTime := time.Now()
	// Check for prompt, but also a timeout to avoid infinite loop
	// The prompt format 'AIReady[%?]» ' is what we expect after PrepareExecPane
	expectedPromptSuffix := "]» " // Ensure there's a space if PS1 ends with it.
								 // The regex in parseExecPaneCommandHistory handles optional space.

	for {
		if m.Status == "" { // Manager stopped
			return CommandExecHistory{}, fmt.Errorf("manager stopped while waiting for command execution")
		}
		if time.Since(startTime) > m.GetCommandTimeout() {
			logger.Error("ExecWaitCapture: Timeout waiting for command '%s' to complete in exec pane %s.", command, m.ExecPane.Id)
			// Attempt one last refresh and parse
			m.refreshExecPaneDetails() // Best effort to get final state
			m.parseExecPaneCommandHistory()
			// Return the last command from history if any, or an error indicating timeout
			if len(m.ExecHistory) > 0 {
				return m.ExecHistory[len(m.ExecHistory)-1], fmt.Errorf("timeout waiting for command to complete, last known state parsed")
			}
			return CommandExecHistory{Command: command, Output: "Error: Timeout", Code: -1}, fmt.Errorf("timeout waiting for command to complete")
		}

		fmt.Printf("\r%s%s ", m.GetPrompt(), animChars[animIndex])
		animIndex = (animIndex + 1) % len(animChars)
		time.Sleep(500 * time.Millisecond) // Polling interval

		if err := m.refreshExecPaneDetails(); err != nil {
			logger.Warning("ExecWaitCapture: Error refreshing exec pane: %v. Retrying.", err)
			continue
		}

		// Check if the last line of the refreshed content contains the prompt.
		// The prompt is 'AIReady[STATUS_CODE]» '
		// A more robust check might involve regex on m.ExecPane.LastLine
		// For example: regexp.MatchString(`AIReady\[\d+\]» $`, m.ExecPane.LastLine)
		if strings.Contains(m.ExecPane.LastLine, expectedPromptSuffix) && strings.HasPrefix(m.ExecPane.LastLine, "AIReady[") {
			logger.Debug("ExecWaitCapture: Detected prompt suffix '%s' in last line: '%s'", expectedPromptSuffix, m.ExecPane.LastLine)
			break
		}
	}
	fmt.Print("\r\033[K") // Clear animation line

	// Final refresh and parse
	if err := m.refreshExecPaneDetails(); err != nil {
		logger.Error("ExecWaitCapture: Failed to perform final refresh of exec pane %s: %v", m.ExecPane.Id, err)
		// Attempt to parse with what we have
	}
	m.parseExecPaneCommandHistory()

	if len(m.ExecHistory) == 0 {
		logger.Error("ExecWaitCapture: Command history is empty after parsing exec pane %s for command '%s'. This indicates a parsing issue.", m.ExecPane.Id, command)
		// This could happen if the prompt was detected but parsing failed to extract any command.
		// Return a generic error or a dummy CommandExecHistory.
		return CommandExecHistory{Command: command, Output: "Error: Failed to parse command output or no history found.", Code: -1},
			fmt.Errorf("failed to parse command output from exec pane %s for command '%s'", m.ExecPane.Id, command)
	}

	// Assuming the last command in history is the one we just executed.
	// This might need adjustment if commands can complete out of order or if history parsing is imperfect.
	cmdResult := m.ExecHistory[len(m.ExecHistory)-1]
	logger.Debug("Command executed: %s\nOutput: %s\nCode: %d\n", cmdResult.Command, cmdResult.Output, cmdResult.Code)
	return cmdResult, nil
}


func (m *Manager) parseExecPaneCommandHistory() {
	if m.ExecPane == nil || m.ExecPane.Id == "" {
		logger.Error("parseExecPaneCommandHistory: ExecPane not initialized.")
		return
	}
	// Ensure freshest content before parsing
	if err := m.refreshExecPaneDetails(); err != nil {
		logger.Error("parseExecPaneCommandHistory: Failed to refresh exec pane %s before parsing: %v", m.ExecPane.Id, err)
		// Depending on severity, might return or try to parse stale content
		if m.ExecPane.Content == "" { // If content is empty after failed refresh, nothing to parse
			return
		}
	}


	var history []CommandExecHistory

	var currentCommand *CommandExecHistory
	var outputBuilder strings.Builder

	// Regex: Capture status code (group 1), optionally capture command (group 2)
	// Making the command part optional handles prompts that only show status (like the last line).
	// ` ?` allows zero or one space after »
	promptRegex := regexp.MustCompile(`.*\[(\d+)\]» ?(.*)$`)

	scanner := bufio.NewScanner(strings.NewReader(m.ExecPane.Content))

	for scanner.Scan() {
		line := scanner.Text()
		match := promptRegex.FindStringSubmatch(line)

		if match != nil && len(match) >= 2 { // We need at least the status code match[1]
			// --- Found a prompt line ---
			// This prompt line *terminates* the previous command block
			// and provides its status code. It might also start a new command block.

			statusCodeStr := match[1]
			commandStr := "" // Default if only status code found (like the last line)
			if len(match) > 2 {
				commandStr = strings.TrimSpace(match[2]) // Command for the *next* block
			}

			// 1. Finalize the PREVIOUS command block (if one was active)
			if currentCommand != nil {
				// Parse the status code found on *this* line - it belongs to the *previous* command
				statusCode, err := strconv.Atoi(statusCodeStr)
				if err != nil {
					// This shouldn't happen with \d+ regex but check anyway
					fmt.Printf("Warning: Could not parse status code '%s' for previous command on line: %s\n", statusCodeStr, line)
					currentCommand.Code = -1 // Indicate parsing error
				} else {
					currentCommand.Code = statusCode // Assign correct status
				}

				// Assign collected output
				currentCommand.Output = strings.TrimSuffix(outputBuilder.String(), "\n")

				// Add the completed previous command block to results
				history = append(history, *currentCommand)

				// Reset for the next block
				outputBuilder.Reset()
				currentCommand = nil // Mark as no active command temporarily
			} else {
				// Optional: Handle status code on the very first prompt if needed.
				// Currently, the status on the first prompt is ignored as there's
				// no *previous* command within the parsed text to assign it to.
			}

			// 2. If this prompt line ALSO contains a command, start the NEW block
			if commandStr != "" {
				currentCommand = &CommandExecHistory{
					Command: commandStr,
					Code:    -1, // Default/Unknown: Status code is determined by the *next* prompt
					// Output will be collected in outputBuilder starting from the next line
				}
			} else {
				// This prompt line only indicates the end status of the previous command
				// (like the final "[i] [~/r/tmuxai][16:56][2]»" line).
				// No new command starts here, so currentCommand remains nil.
			}

		} else {
			// --- Not a prompt line - Must be output ---
			if currentCommand != nil {
				// Append this line as output to the currently active command
				outputBuilder.WriteString(line)
				outputBuilder.WriteString("\n") // Preserve line breaks
			}
			// Ignore lines before the first *actual* command starts
			// (i.e., before the first prompt line that contains a command string)
		}
	}

	// --- After the loop ---
	// Handle the case where the input ends with output lines for the last command,
	// but without a final terminating prompt line.
	if currentCommand != nil {
		currentCommand.Output = strings.TrimSuffix(outputBuilder.String(), "\n")
		// Status code remains the default (-1) because the log ended before the next prompt
		// could provide the exit status.
		history = append(history, *currentCommand)
	}

	if err := scanner.Err(); err != nil {
		logger.Error("error reading input: %v", err)
	}

	// Update the manager's command history
	m.ExecHistory = history
}
