package system

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/alvinunreal/tmuxai/logger"
)

// ZellijBackend implements the Multiplexer interface for Zellij
type ZellijBackend struct {
	tempDir string
}

// NewZellijBackend creates a new ZellijBackend instance
func NewZellijBackend() *ZellijBackend {
	tempDir := os.TempDir()
	return &ZellijBackend{
		tempDir: tempDir,
	}
}

// extractShellFromCommand extracts shell name from command string
func extractShellFromCommand(command string) string {
	if command == "" || command == "shell" {
		return "bash" // default assumption
	}
	
	// Common shells
	shells := []string{"bash", "zsh", "fish", "sh", "tcsh", "csh", "ksh"}
	for _, shell := range shells {
		if strings.Contains(command, shell) {
			return shell
		}
	}
	
	// If no shell detected, return the first word of command
	parts := strings.Fields(command)
	if len(parts) > 0 {
		return parts[0]
	}
	
	return "bash" // fallback
}

// Session operations

func (z *ZellijBackend) GetCurrentSession() (string, error) {
	sessionName := os.Getenv("ZELLIJ_SESSION_NAME")
	if sessionName == "" {
		return "", fmt.Errorf("not in a Zellij session")
	}
	return sessionName, nil
}

func (z *ZellijBackend) CreateSession() (string, error) {
	// Create a new Zellij session
	cmd := exec.Command("zellij", "attach", "--create", "tmuxai-session")
	err := cmd.Run()
	if err != nil {
		logger.Error("Failed to create Zellij session: %v", err)
		return "", fmt.Errorf("failed to create Zellij session: %w", err)
	}

	// Return the session name
	return "tmuxai-session", nil
}

func (z *ZellijBackend) AttachSession(sessionId string) error {
	cmd := exec.Command("zellij", "attach", sessionId)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		logger.Error("Failed to attach to Zellij session: %v", err)
		return fmt.Errorf("failed to attach to Zellij session: %w", err)
	}
	return nil
}

// Detection and information

func (z *ZellijBackend) IsAvailable() bool {
	return os.Getenv("ZELLIJ_SESSION_NAME") != ""
}

func (z *ZellijBackend) GetType() MultiplexerType {
	return MultiplexerZellij
}

func (z *ZellijBackend) GetCurrentPaneId() (string, error) {
	paneId := os.Getenv("ZELLIJ_PANE_ID")
	if paneId == "" {
		return "", fmt.Errorf("ZELLIJ_PANE_ID environment variable not set")
	}
	// Return in the format that matches list-clients output (terminal_X)
	// Most Zellij panes are terminal panes, so we'll assume terminal type
	return "terminal_" + paneId, nil
}

// Pane operations

func (z *ZellijBackend) GetPaneDetails(target string) ([]PaneDetails, error) {
	// Use dump-layout to get pane information
	cmd := exec.Command("zellij", "action", "dump-layout")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		logger.Error("Failed to get Zellij layout: %v, stderr: %s", err, stderr.String())
		return nil, fmt.Errorf("failed to get layout: %w", err)
	}

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		return nil, fmt.Errorf("no layout found")
	}

	// Parse the layout to extract pane information
	panes, err := z.parseLayoutForPanes(output)
	if err != nil {
		logger.Error("Failed to parse Zellij layout: %v", err)
		return nil, fmt.Errorf("failed to parse layout: %w", err)
	}

	if len(panes) == 0 {
		return nil, fmt.Errorf("no panes found in layout")
	}

	return panes, nil
}

func (z *ZellijBackend) CreateNewPane(target string) (string, error) {
	// Create a new pane in Zellij
	cmd := exec.Command("zellij", "action", "new-pane", "--direction", "down")
	err := cmd.Run()
	if err != nil {
		logger.Error("Failed to create new Zellij pane: %v", err)
		return "", fmt.Errorf("failed to create new Zellij pane: %w", err)
	}

	// Return current pane ID (in a real implementation, we'd get the new pane ID)
	// Zellij doesn't provide an easy way to get the new pane ID from the command
	currentPaneId, err := z.GetCurrentPaneId()
	if err != nil {
		return "", err
	}
	return currentPaneId, nil
}

func (z *ZellijBackend) CapturePane(paneId string, maxLines int) (string, error) {
	// Get current pane to restore focus later
	currentPaneId, err := z.GetCurrentPaneId()
	if err != nil {
		logger.Error("Failed to get current pane ID: %v", err)
		currentPaneId = "" // Continue anyway
	}


	// Focus the target pane first
	// Zellij doesn't have direct pane ID targeting, so we need to navigate
	if currentPaneId != paneId {
		// Switch to the target pane using focus-next-pane
		// This works for simple 2-pane layouts
		focusCmd := exec.Command("zellij", "action", "focus-next-pane")
		err = focusCmd.Run()
		if err != nil {
			logger.Error("Failed to focus pane %s: %v", paneId, err)
			// Continue anyway - maybe it's already focused
		}
	}

	// Create a temporary file for the screen dump
	tempFile := filepath.Join(z.tempDir, fmt.Sprintf("zellij_capture_%s_%d.txt", paneId, os.Getpid()))

	// Use Zellij's dump-screen action on the now-focused pane
	var cmd *exec.Cmd
	if maxLines > 0 {
		// Zellij doesn't have a direct equivalent to limiting lines, so we dump full screen
		cmd = exec.Command("zellij", "action", "dump-screen", tempFile, "--full")
	} else {
		cmd = exec.Command("zellij", "action", "dump-screen", tempFile)
	}

	err = cmd.Run()
	if err != nil {
		logger.Error("Failed to capture Zellij screen: %v", err)
		return "", fmt.Errorf("failed to capture screen: %w", err)
	}

	// Restore focus to original pane if we know what it was and we changed it
	if currentPaneId != "" && currentPaneId != paneId {
		// Switch back to the original pane using focus-next-pane
		restoreFocusCmd := exec.Command("zellij", "action", "focus-next-pane")
		restoreFocusCmd.Run() // Ignore errors - this is best effort
	}

	// Read the captured content
	content, err := os.ReadFile(tempFile)
	if err != nil {
		logger.Error("Failed to read captured content: %v", err)
		return "", fmt.Errorf("failed to read captured content: %w", err)
	}

	// Clean up the temporary file
	os.Remove(tempFile)

	// If maxLines is specified, truncate the content
	contentStr := string(content)
	if maxLines > 0 {
		lines := strings.Split(contentStr, "\n")
		if len(lines) > maxLines {
			lines = lines[len(lines)-maxLines:]
			contentStr = strings.Join(lines, "\n")
		}
	}

	return contentStr, nil
}

func (z *ZellijBackend) ClearPane(paneId string) error {
	// Zellij doesn't have a direct clear-history equivalent
	// We'll send Ctrl+L to clear the screen
	cmd := exec.Command("zellij", "action", "write", "\x0C") // Ctrl+L
	err := cmd.Run()
	if err != nil {
		logger.Error("Failed to clear Zellij pane: %v", err)
		return fmt.Errorf("failed to clear pane: %w", err)
	}
	return nil
}

// Command execution

func (z *ZellijBackend) SendCommand(paneId string, command string) error {
	// Focus the target pane first
	currentPaneId, _ := z.GetCurrentPaneId()
	if currentPaneId != paneId {
		focusCmd := exec.Command("zellij", "action", "focus-next-pane")
		focusCmd.Run() // Ignore errors - best effort
	}

	// Send the command to Zellij
	cmd := exec.Command("zellij", "action", "write-chars", command)
	err := cmd.Run()
	if err != nil {
		logger.Error("Failed to send command to Zellij: %v", err)
		return fmt.Errorf("failed to send command: %w", err)
	}

	// Send Enter key
	cmd = exec.Command("zellij", "action", "write-chars", "\r")
	err = cmd.Run()
	if err != nil {
		logger.Error("Failed to send Enter key to Zellij: %v", err)
		return fmt.Errorf("failed to send Enter key: %w", err)
	}

	// Restore focus if we changed it
	if currentPaneId != paneId {
		restoreCmd := exec.Command("zellij", "action", "focus-next-pane")
		restoreCmd.Run() // Ignore errors - best effort
	}

	return nil
}

func (z *ZellijBackend) SendKeys(paneId string, keys string) error {
	// Focus the target pane first
	currentPaneId, _ := z.GetCurrentPaneId()
	if currentPaneId != paneId {
		focusCmd := exec.Command("zellij", "action", "focus-next-pane")
		focusCmd.Run() // Ignore errors - best effort
	}

	// Convert tmux-style key notation to Zellij equivalent
	convertedKeys := z.convertTmuxKeysToZellij(keys)

	// Send the keys to Zellij
	cmd := exec.Command("zellij", "action", "write-chars", convertedKeys)
	err := cmd.Run()
	if err != nil {
		logger.Error("Failed to send keys to Zellij: %v", err)
		return fmt.Errorf("failed to send keys: %w", err)
	}

	// Restore focus if we changed it
	if currentPaneId != paneId {
		restoreCmd := exec.Command("zellij", "action", "focus-next-pane")
		restoreCmd.Run() // Ignore errors - best effort
	}

	return nil
}

// convertTmuxKeysToZellij converts tmux-style key notation to Zellij format
func (z *ZellijBackend) convertTmuxKeysToZellij(keys string) string {
	// Handle special tmux key combinations
	switch keys {
	case "C-l":
		return "\x0C" // Ctrl+L
	case "C-c":
		return "\x03" // Ctrl+C
	case "C-a":
		return "\x01" // Ctrl+A
	case "C-d":
		return "\x04" // Ctrl+D
	case "Enter":
		return "\r"
	case "Escape":
		return "\x1B"
	case "Space":
		return " "
	case "Tab":
		return "\t"
	case "BSpace":
		return "\x08" // Backspace
	case "Up":
		return "\x1B[A"
	case "Down":
		return "\x1B[B"
	case "Right":
		return "\x1B[C"
	case "Left":
		return "\x1B[D"
	default:
		// Handle other control characters
		if strings.HasPrefix(keys, "C-") && len(keys) == 3 {
			char := keys[2]
			if char >= 'a' && char <= 'z' {
				// Convert to control character (Ctrl+A = 1, Ctrl+B = 2, etc.)
				return string(rune(char - 'a' + 1))
			}
		}
		// Handle Meta/Alt keys (M-)
		if strings.HasPrefix(keys, "M-") && len(keys) == 3 {
			char := keys[2]
			return "\x1B" + string(char) // ESC + character
		}
		// Handle function keys
		if strings.HasPrefix(keys, "F") && len(keys) > 1 {
			if fKey, err := strconv.Atoi(keys[1:]); err == nil && fKey >= 1 && fKey <= 12 {
				// Return ANSI escape sequence for function keys
				switch fKey {
				case 1:
					return "\x1BOP"
				case 2:
					return "\x1BOQ"
				case 3:
					return "\x1BOR"
				case 4:
					return "\x1BOS"
				case 5:
					return "\x1B[15~"
				case 6:
					return "\x1B[17~"
				case 7:
					return "\x1B[18~"
				case 8:
					return "\x1B[19~"
				case 9:
					return "\x1B[20~"
				case 10:
					return "\x1B[21~"
				case 11:
					return "\x1B[23~"
				case 12:
					return "\x1B[24~"
				}
			}
		}
		// Return as-is for regular characters
		return keys
	}
}

// parseLayoutForPanes parses the zellij layout output to extract pane information
func (z *ZellijBackend) parseLayoutForPanes(layoutOutput string) ([]PaneDetails, error) {
	var panes []PaneDetails
	paneCounter := 0
	
	// Get current pane ID for marking active pane
	currentPaneId, _ := z.GetCurrentPaneId()
	
	// Split into lines and look for the active tab section only
	lines := strings.Split(layoutOutput, "\n")
	
	// Find the first tab section (active tab) and only parse that
	inActiveTab := false
	tabDepth := 0
	
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		
		// Look for the first tab definition with focus=true or just the first tab
		if strings.HasPrefix(trimmed, "tab ") && (strings.Contains(trimmed, "focus=true") || !inActiveTab) {
			inActiveTab = true
			tabDepth = 0
			continue
		}
		
		// If we're in active tab, track depth with braces
		if inActiveTab {
			if strings.Contains(trimmed, "{") {
				tabDepth++
			}
			if strings.Contains(trimmed, "}") {
				tabDepth--
				// If we've closed the tab section, stop parsing
				if tabDepth < 0 {
					break
				}
			}
			
			// Only process panes within the active tab that are actual user panes
			if strings.HasPrefix(trimmed, "pane") {
				// Skip all plugin panes and UI panes
				if strings.Contains(trimmed, "plugin") || 
				   strings.Contains(trimmed, "tab-bar") || 
				   strings.Contains(trimmed, "status-bar") ||
				   strings.Contains(trimmed, "borderless=true") {
					continue
				}
				
				// Extract command if present
				var command string
				var args []string
				
				if strings.Contains(trimmed, "command=") {
					// Parse command="value"
					parts := strings.Split(trimmed, "command=")
					if len(parts) > 1 {
						cmdPart := strings.TrimSpace(parts[1])
						if strings.HasPrefix(cmdPart, "\"") {
							// Find the closing quote
							endQuote := strings.Index(cmdPart[1:], "\"")
							if endQuote != -1 {
								command = cmdPart[1 : endQuote+1]
							}
						}
					}
					
					// Look for args in the following lines
					for j := i + 1; j < len(lines) && j < i+5; j++ {
						argLine := strings.TrimSpace(lines[j])
						if strings.HasPrefix(argLine, "args ") {
							// Parse args line
							argsPart := strings.TrimPrefix(argLine, "args ")
							argsPart = strings.Trim(argsPart, "\"")
							args = strings.Fields(argsPart)
							break
						}
						// Stop if we hit another pane or closing brace
						if strings.HasPrefix(argLine, "pane") || argLine == "}" {
							break
						}
					}
				} else {
					// This is a regular shell pane
					command = "shell"
				}
				
				// Generate a pane ID (Zellij doesn't provide explicit IDs in layout)
				paneId := fmt.Sprintf("terminal_%d", paneCounter)
				paneCounter++
				
				// Determine if this is the active pane
				isActive := 0
				if paneId == currentPaneId {
					isActive = 1
				}
				
				// Build command string
				fullCommand := command
				if len(args) > 0 {
					fullCommand = command + " " + strings.Join(args, " ")
				}
				
				pane := PaneDetails{
					Id:                 paneId,
					CurrentPid:         0, // We don't have PID info from layout
					CurrentCommand:     command,
					CurrentCommandArgs: strings.Join(args, " "),
					Content:            "",
					Shell:              extractShellFromCommand(command),
					OS:                 "linux",
					LastLine:           "",
					IsActive:           isActive,
					IsTmuxAiPane:       false,
					IsTmuxAiExecPane:   false,
					IsPrepared:         false,
					IsSubShell:         false,
					HistorySize:        0,
					HistoryLimit:       0,
					MultiplexerType:    MultiplexerZellij,
				}
				
				// Detect TmuxAI panes based on command
				if strings.Contains(fullCommand, "tmuxai") || command == "go" {
					pane.IsTmuxAiPane = true
				}
				
				panes = append(panes, pane)
			}
		}
		
		// If we encounter a new section (swap_tiled_layout, etc.) and we've found some panes, stop
		if inActiveTab && (strings.HasPrefix(trimmed, "swap_") || strings.HasPrefix(trimmed, "new_tab_template")) {
			break
		}
	}
	
	return panes, nil
}