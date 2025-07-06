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

// TmuxCreateNewPane creates a new horizontal split pane in the specified window and returns its ID
func TmuxCreateNewPane(target string) (string, error) {
	cmd := exec.Command("tmux", "split-window", "-d", "-h", "-t", target, "-P", "-F", "#{pane_id}")
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

// TmuxPanesDetails gets details for all panes in a target window
func TmuxPanesDetails(target string) ([]TmuxPaneDetails, error) {
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
	paneDetails := make([]TmuxPaneDetails, 0, len(lines))

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
		isSubShell := IsSubShell(parts[3])

		paneDetail := TmuxPaneDetails{
			Id:                 id,
			IsActive:           active,
			CurrentPid:         pid,
			CurrentCommand:     parts[3],
			CurrentCommandArgs: currentCommandArgs,
			HistorySize:        historySize,
			HistoryLimit:       historyLimit,
			IsSubShell:         isSubShell,
		}

		paneDetails = append(paneDetails, paneDetail)
	}

	return paneDetails, nil
}

// TmuxCapturePane gets the content of a specific pane by ID
func TmuxCapturePane(paneId string, maxLines int) (string, error) {
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

// Return current tmux window target with session id and window id
func TmuxCurrentWindowTarget() (string, error) {
	paneId, err := TmuxCurrentPaneId()
	if err != nil {
		return "", err
	}

	cmd := exec.Command("tmux", "list-panes", "-t", paneId, "-F", "#{session_id}:#{window_index}")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get window target: %w", err)
	}

	target := strings.TrimSpace(string(output))
	if target == "" {
		return "", fmt.Errorf("empty window target returned")
	}

	if idx := strings.Index(target, "\n"); idx != -1 {
		target = target[:idx]
	}

	return target, nil
}

func TmuxCurrentPaneId() (string, error) {
	tmuxPane := os.Getenv("TMUX_PANE")
	if tmuxPane == "" {
		return "", fmt.Errorf("TMUX_PANE environment variable not set")
	}

	return tmuxPane, nil
}

// CreateTmuxSession creates a new tmux session and returns the new pane id
func TmuxCreateSession() (string, error) {
	cmd := exec.Command("tmux", "new-session", "-d", "-P", "-F", "#{pane_id}")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		logger.Error("Failed to create tmux session: %v, stderr: %s", err, stderr.String())
		return "", err
	}

	return strings.TrimSpace(stdout.String()), nil
}

// AttachToTmuxSession attaches to an existing tmux session
func TmuxAttachSession(paneId string) error {
	cmd := exec.Command("tmux", "attach-session", "-t", paneId)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		logger.Error("Failed to attach to tmux session: %v", err)
		return err
	}
	return nil
}

// TmuxClearPane sends a Ctrl+L to clear the pane's screen.
func TmuxClearPane(paneId string) error {
	err := TmuxSendCommandToPane(paneId, "C-l", false)
	if err != nil {
		logger.Error("Failed to send clear command to pane %s: %v", paneId, err)
		return err
	}
	logger.Debug("Successfully cleared pane %s", paneId)
	return nil
}

// TmuxSendCommandToPane sends a command or keystrokes to a pane.
func TmuxSendCommandToPane(paneId string, command string, autoenter bool) error {
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

// TmuxPasteToPane pastes content into a pane using a temporary buffer.
func TmuxPasteToPane(paneId string, content string) error {
	// Use a temporary buffer to avoid overwriting the user's main paste buffer
	setBufferCmd := exec.Command("tmux", "set-buffer", "-b", "tmuxai_paste_buffer", "--", content)
	if err := setBufferCmd.Run(); err != nil {
		logger.Error("Failed to set tmux paste buffer for pane %s: %v", paneId, err)
		return fmt.Errorf("failed to set paste buffer: %w", err)
	}

	// Paste from the temporary buffer
	pasteCmd := exec.Command("tmux", "paste-buffer", "-b", "tmuxai_paste_buffer", "-t", paneId)
	if err := pasteCmd.Run(); err != nil {
		logger.Error("Failed to paste buffer to pane %s: %v", paneId, err)
		return fmt.Errorf("failed to paste buffer: %w", err)
	}

	// Delete the temporary buffer
	deleteBufferCmd := exec.Command("tmux", "delete-buffer", "-b", "tmuxai_paste_buffer")
	if err := deleteBufferCmd.Run(); err != nil {
		// This is not a critical error, just log it
		logger.Info("Could not delete temporary paste buffer 'tmuxai_paste_buffer': %v", err)
	}

	return nil
}

// containsSpecialKey checks if a string contains any tmux special key notation
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
