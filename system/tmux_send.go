package system

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/alvinunreal/tmuxai/logger"
)

var TmuxSendCommandToPane = func(paneId string, command string, autoenter bool) error {
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
