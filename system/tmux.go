package system

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/alvinunreal/tmuxai/logger"
)

// TmuxMultiplexer implements the Multiplexer interface for tmux.
type TmuxMultiplexer struct{}

// CreateNewPane creates a new pane.
func (tm *TmuxMultiplexer) CreateNewPane(target string, command ...string) (string, error) {
	args := []string{"split-window", "-d", "-h", "-t", target, "-P", "-F", "#{pane_id}"}
	if len(command) > 0 && command[0] != "" {
		args = append(args, "--")
		args = append(args, command...)
	}
	cmd := exec.Command("tmux", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		logger.Error("Failed to create tmux pane: %v, stderr: %s", err, stderr.String())
		return "", err
	}

	paneId := strings.TrimSpace(stdout.String())
	return paneId, nil
}

// SendCommandToPane sends a command (sequence of characters) to a specific pane.
func (tm *TmuxMultiplexer) SendCommandToPane(paneId string, command string, autoenter bool) error {
	lines := strings.Split(command, "\n")
	for i, line := range lines {

		if line != "" {
			if !containsSpecialKey(line) {
				// Only replace semicolons at the end of the line
				if strings.HasSuffix(line, ";") {
					line = line[:len(line)-1] + "\\;"
				}
				cmd := exec.Command("tmux", "send-keys", "-t", paneId, "-l", line)
				var stderr bytes.Buffer
				cmd.Stderr = &stderr
				err := cmd.Run()
				if err != nil {
					logger.Error("Failed to send command to pane %s: %v, stderr: %s", paneId, err, stderr.String())
					return fmt.Errorf("failed to send command to pane: %w", err)
				}

			} else {
				args := []string{"send-keys", "-t", paneId}
				processed := processLineWithSpecialKeys(line)
				args = append(args, processed...)
				cmd := exec.Command("tmux", args...)
				var stderr bytes.Buffer
				cmd.Stderr = &stderr
				err := cmd.Run()
				if err != nil {
					logger.Error("Failed to send command with special keys to pane %s: %v, stderr: %s", paneId, err, stderr.String())
					return fmt.Errorf("failed to send command with special keys to pane: %w", err)
				}
			}
		}

		// Send Enter key after each line except for empty lines at the end
		if autoenter {
			if i < len(lines)-1 || (i == len(lines)-1 && line != "") {
				enterCmd := exec.Command("tmux", "send-keys", "-t", paneId, "Enter")
				err := enterCmd.Run()
				if err != nil {
					logger.Error("Failed to send Enter key to pane %s: %v", paneId, err)
					return fmt.Errorf("failed to send Enter key to pane: %w", err)
				}
			}
		}
	}
	return nil
}

// containsSpecialKey checks if a string contains any tmux special key notation
// Moved from tmux_send.go
func containsSpecialKey(line string) bool {
	// Check for control or meta key combinations
	if strings.Contains(line, "C-") || strings.Contains(line, "M-") {
		return true
	}

	// Check for special key names
	for key := range getSpecialKeys() {
		if strings.Contains(line, key) {
			return true
		}
	}

	return false
}

// processLineWithSpecialKeys processes a line containing special keys
// and returns an array of arguments for tmux send-keys
// Moved from tmux_send.go
func processLineWithSpecialKeys(line string) []string {
	var result []string
	var currentText string

	// Split by spaces but keep track of what we're processing
	parts := strings.Split(line, " ")

	for _, part := range parts {
		if part == "" {
			// Preserve empty parts (consecutive spaces)
			if currentText != "" {
				currentText += " "
			}
			continue
		}

		// Check if this part is a special key
		if (strings.HasPrefix(part, "C-") || strings.HasPrefix(part, "M-")) ||
			getSpecialKeys()[part] {
			// If we have accumulated text, add it first
			if currentText != "" {
				result = append(result, currentText)
				currentText = ""
			}
			// Add the special key as a separate argument
			result = append(result, part)
		} else {
			// Regular text - append to current text with space if needed
			if currentText != "" {
				currentText += " "
			}
			currentText += part
		}
	}

	// Add any remaining text
	if currentText != "" {
		result = append(result, currentText)
	}

	return result
}

// getSpecialKeys returns a map of tmux special key names
// Moved from tmux_send.go
func getSpecialKeys() map[string]bool {
	specialKeys := map[string]bool{
		"Up": true, "Down": true, "Left": true, "Right": true,
		"BSpace": true, "BTab": true, "DC": true, "End": true,
		"Enter": true, "Escape": true, "Home": true, "IC": true,
		"NPage": true, "PageDown": true, "PgDn": true,
		"PPage": true, "PageUp": true, "PgUp": true,
		"Space": true, "Tab": true,
	}

	// Add function keys F1-F12
	for i := 1; i <= 12; i++ {
		specialKeys[fmt.Sprintf("F%d", i)] = true
	}

	return specialKeys
}

// GetPaneDetails returns details for panes.
func (tm *TmuxMultiplexer) GetPaneDetails(target string) ([]PaneDetails, error) {
	// Note: The format string now includes #{pane_shell} which might be available in newer tmux versions.
	// If not, GetShellName(pid) serves as a fallback.
	// For now, relying on GetShellName(pid) as it was in the original code.
	// We also need to ensure IsTmuxAiPane, IsTmuxAiExecPane, IsPrepared are handled if possible,
	// though the original TmuxPaneDetails didn't set these directly from list-panes.
	// These might need to be populated via other means or conventions (e.g., pane title/tag).
	cmd := exec.Command("tmux", "list-panes", "-t", target, "-F", "#{pane_id},#{pane_active},#{pane_pid},#{pane_current_command},#{history_size},#{history_limit}")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		logger.Error("Failed to get tmux pane details for target %s %v, stderr: %s", target, err, stderr.String())
		return nil, err
	}

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		return nil, fmt.Errorf("no pane details found for target %s", target)
	}

	lines := strings.Split(output, "\n")
	paneDetails := make([]PaneDetails, 0, len(lines)) // Changed TmuxPaneDetails to PaneDetails

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, ",", 6)
		if len(parts) < 5 {
			logger.Error("Invalid pane details format for line: %s", line)
			continue
		}

		id := parts[0]

		// If target starts with '%', it's a pane ID, so only include the matching pane
		if strings.HasPrefix(target, "%") && id != target {
			continue
		}

		active, _ := strconv.Atoi(parts[1])
		pid, _ := strconv.Atoi(parts[2])
		historySize, _ := strconv.Atoi(parts[4])
		historyLimit, _ := strconv.Atoi(parts[5])
		currentCommandArgs := GetProcessArgs(pid)
		// Assuming GetProcessArgs, IsSubShell, GetShellName are available in the package or via import.
		// These were not part of original tmux.go, so they are treated as external helpers.
		currentCommandArgs := GetProcessArgs(pid)
		isSubShell := IsSubShell(parts[3])
		shellName := GetShellName(pid)

		paneDetail := PaneDetails{
			Id:                 id,
			IsActive:           active,
			CurrentPid:         pid,
			CurrentCommand:     parts[3],
			CurrentCommandArgs: currentCommandArgs,
			HistorySize:        historySize,
			HistoryLimit:       historyLimit,
			IsSubShell:         isSubShell,
			Shell:              shellName,
			// OS, Content, LastLine are typically fetched by CapturePane and processed separately.
			// IsTmuxAiPane, IsTmuxAiExecPane, IsPrepared would require specific logic
			// potentially involving environment variables within the pane or tmux options/tags.
			// GetPaneDetails focuses on what `list-panes` provides directly.
		}
		paneDetails = append(paneDetails, paneDetail)
	}

	return paneDetails, nil
}

// CapturePane captures the content of a specific pane.
func (tm *TmuxMultiplexer) CapturePane(paneId string, maxLines int) (string, error) {
	cmd := exec.Command("tmux", "capture-pane", "-p", "-t", paneId, "-S", fmt.Sprintf("-%d", maxLines))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		logger.Error("Failed to capture pane content from %s: %v, stderr: %s", paneId, err, stderr.String())
		return "", err
	}

	content := strings.TrimSpace(stdout.String())
	return content, nil
}

// currentWindowTarget returns current tmux window target with session_id:window_index
func (tm *TmuxMultiplexer) currentWindowTarget() (string, error) {
	paneId, err := tm.GetCurrentPaneId()
	if err != nil {
		// If not in a tmux session, GetCurrentPaneId will error.
		// Fallback: try to list panes without a target, which works if there's only one session.
		// This is a best-effort for contexts where TMUX_PANE might not be set but we are in tmux.
		// However, for robustness, GetCurrentPaneId should ideally be the primary source.
		cmd := exec.Command("tmux", "list-panes", "-F", "#{session_id}:#{window_index}")
		output, errListPanes := cmd.Output()
		if errListPanes != nil {
			return "", fmt.Errorf("failed to get current pane ID (%v) and failed to list panes for window target: %w", err, errListPanes)
		}
		target := strings.TrimSpace(string(output))
		if target == "" {
			return "", fmt.Errorf("empty window target returned from list-panes fallback")
		}
		// If multiple lines (multiple panes in the current session), pick the first one.
		// This is a heuristic. A more precise method might be needed if this proves insufficient.
		if idx := strings.Index(target, "\n"); idx != -1 {
			target = target[:idx]
		}
		return target, nil
	}

	cmd := exec.Command("tmux", "list-panes", "-t", paneId, "-F", "#{session_id}:#{window_index}")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get window target for pane %s: %w", paneId, err)
	}

	target := strings.TrimSpace(string(output))
	if target == "" {
		return "", fmt.Errorf("empty window target returned for pane %s", paneId)
	}

	// list-panes can sometimes return multiple lines if the target is ambiguous, ensure we only take one.
	if idx := strings.Index(target, "\n"); idx != -1 {
		target = target[:idx]
	}
	return target, nil
}

// GetCurrentPaneId returns the ID of the current pane.
func (tm *TmuxMultiplexer) GetCurrentPaneId() (string, error) {
	tmuxPane := os.Getenv("TMUX_PANE")
	if tmuxPane == "" {
		// Attempt to get the current pane ID using display-message if TMUX_PANE is not set.
		// This can happen if the command is run in a context where TMUX_PANE is not exported (e.g. subshell).
		cmd := exec.Command("tmux", "display-message", "-p", "#{pane_id}")
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		if err != nil {
			errMsg := stderr.String()
			// Check if the error is because we are not in a tmux server
			if strings.Contains(errMsg, "no server") || strings.Contains(errMsg, "not a tmux command") {
				return "", fmt.Errorf("not inside a tmux session: %w (TMUX_PANE not set, display-message failed)", err)
			}
			return "", fmt.Errorf("failed to get current pane ID via display-message: %w, stderr: %s (TMUX_PANE not set)", err, errMsg)
		}
		paneId := strings.TrimSpace(stdout.String())
		if paneId == "" {
			return "", fmt.Errorf("TMUX_PANE not set and display-message returned empty pane ID")
		}
		return paneId, nil
	}
	return tmuxPane, nil
}

// CreateSession creates a new multiplexer session.
func (tm *TmuxMultiplexer) CreateSession(command ...string) (string, error) {
	cmdArgs := []string{"new-session", "-d", "-P", "-F", "#{pane_id}"}
	if len(command) > 0 && command[0] != "" {
		// Add the command to be executed in the new session's initial pane
		cmdArgs = append(cmdArgs, "--")
		cmdArgs = append(cmdArgs, command...)
	}
	cmd := exec.Command("tmux", cmdArgs...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		logger.Error("Failed to create tmux session: %v, stderr: %s", err, stderr.String())
		return "", err
	}

	return strings.TrimSpace(stdout.String()), nil
}

// AttachSession attaches the current terminal to an existing session.
func (tm *TmuxMultiplexer) AttachSession(sessionId string) error {
	cmd := exec.Command("tmux", "attach-session", "-t", sessionId)
	// For attach to work correctly, it often needs to take over the current TTY.
	// Stdin, Stdout, Stderr should be inherited if this Go program is run from a terminal
	// that can be taken over by tmux.
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		logger.Error("Failed to attach to tmux session %s: %v", sessionId, err)
		return err
	}
	return nil
}

// ClearPane clears the content and history of a pane.
func (tm *TmuxMultiplexer) ClearPane(paneId string) error {
	// Check if pane exists first by trying to get its details.
	// This is a bit indirect; a more direct "check if pane exists" might be better if available.
	_, err := tm.GetPaneDetails(paneId)
	if err != nil {
		logger.Error("Failed to get pane details for %s before clearing: %v", paneId, err)
		return fmt.Errorf("pane %s not found or error fetching details: %w", paneId, err)
	}

	// Send Ctrl+L to clear the visible screen.
	// This is a common approach but might not work for all applications inside the pane.
	if err := tm.SendCommandToPane(paneId, "C-l", false); err != nil {
		logger.Warning("Failed to send Ctrl-L to pane %s: %v. Proceeding with clear-history.", paneId, err)
		// Not returning error here as clear-history is the more critical part for tmux itself.
	}

	// Clear tmux scrollback history for the pane.
	cmdClearHist := exec.Command("tmux", "clear-history", "-t", paneId)
	var stderrClearHist bytes.Buffer
	cmdClearHist.Stderr = &stderrClearHist
	if err := cmdClearHist.Run(); err != nil {
		errMsg := stderrClearHist.String()
		logger.Error("Failed to clear history for pane %s: %v, stderr: %s", paneId, err, errMsg)
		return fmt.Errorf("failed to clear history for pane %s: %w. Stderr: %s", paneId, err, errMsg)
	}

	logger.Debug("Successfully cleared pane %s", paneId)
	return nil
}

// IsInsideSession checks if currently running inside a multiplexer session.
func (tm *TmuxMultiplexer) IsInsideSession() bool {
	// Checking TMUX_PANE is a common way, but TMUX env var itself indicates a tmux client.
	if os.Getenv("TMUX") != "" || os.Getenv("TMUX_PANE") != "" {
		// Further check: can we communicate with the server?
		cmd := exec.Command("tmux", "ls") // A lightweight command to check server presence
		err := cmd.Run()
		return err == nil // If command runs successfully, we are in a session.
	}
	return false
}

// GetType returns a string indicating the type of multiplexer.
func (tm *TmuxMultiplexer) GetType() string {
	return "tmux"
}

// Helper functions GetProcessArgs, IsSubShell, GetShellName are assumed to be
// available in the 'system' package or imported from a utility package.
// If they were originally unexported static functions within a *single* file that is now
// becoming methods of TmuxMultiplexer, they would need to become private methods.
// However, the problem description suggests they are more general utilities.
// For example:
// func GetProcessArgs(pid int) []string { /* ... */ }
// func IsSubShell(command string) bool { /* ... */ }
// func GetShellName(pid int) string { /* ... */ }
// These are not defined here as they are expected to be external to this specific file's direct responsibilities
// beyond what tmux client commands provide.
