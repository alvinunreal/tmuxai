package system

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/alvinunreal/tmuxai/logger"
)

// commandExecutor is a package-level variable that defaults to exec.Command.
// It can be overridden in tests to mock command execution.
var commandExecutor = exec.Command

// Ensure ZellijMultiplexer implements the Multiplexer interface.
var _ Multiplexer = (*ZellijMultiplexer)(nil)

// ZellijMultiplexer implements the Multiplexer interface for Zellij.
type ZellijMultiplexer struct{}

// GetOSDetails is a placeholder for OS detection logic.
// In a real scenario, this would be more comprehensive.
func GetOSDetails() string {
	// Simplified example
	cmd := commandExecutor("uname", "-a")
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

// GetCurrentPaneId returns the ID of the current pane.
// For Zellij, this is typically the ZELLIJ_PANE_ID environment variable.
func (z *ZellijMultiplexer) GetCurrentPaneId() (string, error) {
	paneId := os.Getenv("ZELLIJ_PANE_ID")
	if paneId == "" {
		logger.Warning("ZELLIJ_PANE_ID environment variable not set. Assuming not in a Zellij pane.")
		return "", fmt.Errorf("ZELLIJ_PANE_ID not set")
	}
	return paneId, nil
}

// CreateNewPane creates a new pane.
// Zellij's `new-pane` action might not return the new pane ID directly.
// The `target` argument is likely ignored by Zellij as new panes are typically created relative to the focused pane.
func (z *ZellijMultiplexer) CreateNewPane(target string, command ...string) (string, error) {
	if target != "" {
		logger.Warning("Zellij CreateNewPane: 'target' argument (%s) is ignored. Panes are created relative to the focused pane.", target)
	}

	cliArgs := []string{"action", "new-pane"}
	if len(command) > 0 && command[0] != "" {
		// Zellij's new-pane can take a command, but it needs to be passed with --command
		// and the actual command might need to be a single string if it has args.
		// Example: zellij action new-pane --command "ls -l"
		// For simplicity, joining command and its args here.
		fullCommand := strings.Join(command, " ")
		cliArgs = append(cliArgs, "--command", fullCommand)
		logger.Debug("Zellij CreateNewPane with command: %s", fullCommand)
	}

	cmd := commandExecutor("zellij", cliArgs...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		logger.Error("Zellij failed to create new pane: %v, stderr: %s", err, stderr.String())
		return "", fmt.Errorf("zellij action new-pane failed: %w. Stderr: %s", err, stderr.String())
	}

	// Zellij does not easily return the new pane ID.
	// The new pane typically becomes focused. One *could* try to get ZELLIJ_PANE_ID
	// from a command run in that new pane, but that's overly complex for this.
	// For now, we return an empty string and log a warning.
	logger.Warning("Zellij CreateNewPane: Successfully created pane, but Zellij does not directly return the new pane ID. Returning empty string.")
	return "", nil // No direct way to get the new pane ID from Zellij's new-pane action.
}

// GetPaneDetails returns details for panes.
// This is very limited for Zellij compared to tmux.
// It primarily returns details for the *current* pane using environment variables.
// `target` is mostly ignored unless it matches the current pane ID.
func (z *ZellijMultiplexer) GetPaneDetails(target string) ([]PaneDetails, error) {
	currentPaneId, err := z.GetCurrentPaneId()
	if err != nil && target == "" { // Only error out if no target is specified and we can't get current pane
		return nil, fmt.Errorf("failed to get current Zellij pane ID and no target specified: %w", err)
	}

	// If a target is specified, and it's not the current pane, we can't get details for it easily.
	if target != "" && target != currentPaneId {
		logger.Warning("Zellij GetPaneDetails: Can only reliably get details for the current pane. Target '%s' is not the current pane '%s'. Returning empty details for target.", target, currentPaneId)
		return []PaneDetails{}, nil // Or return an error, depending on desired strictness
	}

	// If target is specified and IS the current pane, or if no target is specified, proceed for current pane.
	if currentPaneId == "" { // Could happen if ZELLIJ_PANE_ID was not set but a target was given (which wasn't current)
		if target != "" { // We already warned if target != currentPaneId. If target was current, currentPaneId wouldn't be empty.
			  // This case means target was specific, not current, and we can't fetch it.
			  return []PaneDetails{}, nil
		}
		// If currentPaneId is still empty and no target, means we are not in a pane.
		return nil, fmt.Errorf("cannot determine current Zellij pane ID")
	}


	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "unknown"
	}

	// Zellij's `list-clients` or `list-panes` (if/when available and more detailed)
	// might offer more, but parsing its output can be complex and version-dependent.
	// For now, this is a very basic implementation.
	// Other fields like CurrentPid, CurrentCommand, HistorySize, etc., are not readily available
	// from environment variables in the same way as tmux.

	details := PaneDetails{
		Id:             currentPaneId,
		IsActive:       1, // Assuming the current pane is always active from its own perspective
		Shell:          shell,
		OS:             GetOSDetails(), // Placeholder
		IsTmuxAiPane:   false, // Specific to tmuxai, needs a different detection for Zellij
		IsTmuxAiExecPane: false, // Specific to tmuxai
		IsPrepared:     false, // Specific to tmuxai
		// CurrentPid, CurrentCommand, CurrentCommandArgs, HistorySize, HistoryLimit, Content, LastLine
		// are not easily available for Zellij panes without more complex interactions or new Zellij features.
	}
	logger.Warning("Zellij GetPaneDetails: Information is limited to the current pane and environment variables. Fields like PID, Command, History, Content are not populated.")
	return []PaneDetails{details}, nil
}

// CapturePane captures the content of a specific pane.
// For Zellij, this uses `zellij action dump-screen` which dumps the *focused* pane's content.
// The `paneId` argument is used to warn if it doesn't match the current (focused) pane.
func (z *ZellijMultiplexer) CapturePane(paneId string, maxLines int) (string, error) {
	currentPaneId, _ := z.GetCurrentPaneId() // Ignore error, just for comparison
	if paneId != "" && paneId != currentPaneId {
		logger.Warning("Zellij CapturePane: `dump-screen` action captures the currently focused pane. Requested paneId '%s' might not be the focused one ('%s').", paneId, currentPaneId)
	}

	tmpFile, err := ioutil.TempFile("", "zellij-dump-*.log")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file for zellij dump: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFilePath := tmpFile.Name()
	tmpFile.Close() // Close the file so zellij can write to it

	// Zellij action dump-screen <path_to_file> [pane_id_to_dump_from]
	// As of recent Zellij versions, dump-screen can take a target pane ID.
	// However, let's assume for now it dumps the current pane if paneId is empty or if we want to be safe.
	// If paneId is provided, we can try to use it.

	var cmd *exec.Cmd
	if paneId != "" {
		// Assuming newer zellij that supports specifying pane id
		cmd = commandExecutor("zellij", "action", "dump-screen", tmpFilePath, "--pane-id", paneId)
		logger.Info("Zellij CapturePane: Attempting to dump screen for specific pane ID %s", paneId)
	} else {
		cmd = commandExecutor("zellij", "action", "dump-screen", tmpFilePath)
		logger.Info("Zellij CapturePane: Dumping screen for focused pane as no specific pane ID was provided.")
	}


	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// If the error indicates the pane was not found, it's useful to log that.
		// Zellij might exit with an error if the pane_id in dump-screen is not found.
		errMsg := stderr.String()
		logger.Error("Zellij failed to dump screen (paneId: '%s'): %v, stderr: %s", paneId, err, errMsg)
		return "", fmt.Errorf("zellij action dump-screen failed (paneId: '%s'): %w. Stderr: %s", paneId, err, errMsg)
	}

	// Wait a very short moment for the file to be fully written.
	// This is a workaround for potential race conditions.
	time.Sleep(100 * time.Millisecond)

	content, err := ioutil.ReadFile(tmpFilePath)
	if err != nil {
		logger.Error("Zellij CapturePane: Failed to read screen dump from temp file '%s': %v", tmpFilePath, err)
		return "", fmt.Errorf("failed to read screen dump from temp file: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	if maxLines > 0 && len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return strings.Join(lines, "\n"), nil
}

// SendCommandToPane sends a command (sequence of characters) to a specific pane.
// For Zellij, this uses `zellij action write-chars` which sends to the *focused* pane.
// The `paneId` argument is used to warn if it doesn't match the current (focused) pane.
func (z *ZellijMultiplexer) SendCommandToPane(paneId string, command string, autoenter bool) error {
	currentPaneId, _ := z.GetCurrentPaneId() // Ignore error, just for comparison
	if paneId != "" && paneId != currentPaneId {
		logger.Warning("Zellij SendCommandToPane: `write-chars` action sends to the currently focused pane. Requested paneId '%s' might not be the focused one ('%s').", paneId, currentPaneId)
		// Depending on strictness, one might choose to return an error here if paneId does not match currentPaneId.
		// For now, proceeding with the action, assuming the user intends to target the focused pane or knows the risk.
	}

	fullCommand := command
	if autoenter {
		fullCommand += "\n"
	}

	cmd := commandExecutor("zellij", "action", "write-chars", fullCommand)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		logger.Error("Zellij failed to send command (write-chars): %v, stderr: %s", err, stderr.String())
		return fmt.Errorf("zellij action write-chars failed: %w. Stderr: %s", err, stderr.String())
	}
	return nil
}

// CreateSession ensures a Zellij session is started.
// This is typically called when tmuxai is *not* yet inside Zellij.
// The calling code in NewManager will likely re-exec tmuxai into the new session.
func (z *ZellijMultiplexer) CreateSession(command ...string) (string, error) {
	if z.IsInsideSession() {
		logger.Info("Zellij CreateSession: Already inside a Zellij session.")
		return z.GetCurrentPaneId() // Return current pane ID if already inside
	}

	logger.Info("Zellij CreateSession: Attempting to start a new Zellij session.")
	// `zellij` command by itself usually starts a new session or attaches to an existing one.
	// We want to ensure it starts a new one if none exists, or attaches.
	// The calling logic (NewManager) is expected to handle re-exec, so this command might just
	// need to ensure Zellij server is running and a session is available.
	// `zellij setup --dump-layout default` can initialize things.
	// `zellij options --dump-layout default | zellij action new-pane --layout -` might be too complex.
	// Using `zellij attach --create` is a robust way to ensure a session exists and we attach to it,
	// or create one if none are running.

	var cmdArgs []string
	if len(command) > 0 && command[0] != "" {
		// Zellij can start with a command using `zellij -- command...`
		// However, `zellij attach --create` doesn't directly take a command for the *new* session's first pane.
		// A layout approach would be needed, or starting zellij with the command directly.
		// For now, we'll ignore the command here as `attach --create` is simpler.
		// The re-exec mechanism will handle running commands inside the new session.
		logger.Warning("Zellij CreateSession: `command` argument is currently ignored when using `attach --create`. The command should be run after re-exec into Zellij.")
	}

	// This command will block until Zellij is exited if it attaches.
	// This is problematic if called from within a Go program that expects to continue.
	// The expectation is that `tmuxai` will be re-exec'd.
	// So, this function's role is more to "ensure Zellij is running" than to return a pane ID of a new session
	// that this current Go process will then use.
	// For now, we won't execute `zellij attach --create` directly here as it would block.
	// We will rely on the calling code to re-exec `tmuxai` which, if not in Zellij,
	// would then use `zellij attach --create` or similar as its entry point.
	// So, this function might just be a no-op or a check.
	// For the purpose of this interface, returning an empty string is acceptable if the re-exec handles it.
	logger.Info("Zellij CreateSession: Relying on external mechanism (re-exec) to start/attach to Zellij session. No direct pane ID returned.")
	return "", nil // No specific pane ID to return, re-exec will determine the new context.
}

// AttachSession attaches the current terminal to an existing Zellij session.
func (z *ZellijMultiplexer) AttachSession(sessionId string) error {
	var cmd *exec.Cmd
	if sessionId != "" {
		logger.Info("Zellij AttachSession: Attaching to session '%s'.", sessionId)
		cmd = commandExecutor("zellij", "attach", sessionId)
	} else {
		logger.Info("Zellij AttachSession: Attaching to existing session or creating a new one (`zellij attach --create`).")
		cmd = commandExecutor("zellij", "attach", "--create")
	}

	// These need to be inherited for `zellij attach` to work correctly.
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		// This error will only be returned if Zellij itself fails to start/attach.
		// If it successfully attaches, this Go process will be replaced by Zellij.
		logger.Error("Zellij failed to attach to session '%s': %v", sessionId, err)
		return fmt.Errorf("zellij attach failed for session '%s': %w", sessionId, err)
	}
	// If cmd.Run() is successful, this process is usually taken over by Zellij.
	// So, code here might not be reached unless Zellij detaches immediately or errors.
	return nil
}

// ClearPane clears the content of a pane.
// Zellij doesn't have a direct "clear" or "clear-history" action like tmux.
// Sending a "clear" command or form feed (Ctrl+L) to the shell is a common workaround.
func (z *ZellijMultiplexer) ClearPane(paneId string) error {
	currentPaneId, _ := z.GetCurrentPaneId()
	if paneId != "" && paneId != currentPaneId {
		logger.Warning("Zellij ClearPane: Action will target the currently focused pane. Requested paneId '%s' might not be the focused one ('%s').", paneId, currentPaneId)
	}

	// Sending Ctrl+L (form feed character \f)
	logger.Debug("Zellij ClearPane: Sending Ctrl+L (form feed) to pane %s (focused pane).", currentPaneId)
	err := z.SendCommandToPane(paneId, "\f", false) // paneId here is for the warning, action targets focused
	if err != nil {
		// As a fallback, try sending "clear" command
		logger.Warning("Zellij ClearPane: Failed to send Ctrl+L to pane %s. Attempting to send 'clear' command.", currentPaneId)
		err = z.SendCommandToPane(paneId, "clear", true) // Auto-enter "clear" command
		if err != nil {
			logger.Error("Zellij ClearPane: Failed to send 'clear' command to pane %s: %v", currentPaneId, err)
			return fmt.Errorf("failed to clear pane %s using Ctrl+L and 'clear' command: %w", currentPaneId, err)
		}
	}
	logger.Info("Zellij ClearPane: Successfully sent clear command/char to pane %s (focused pane).", currentPaneId)
	return nil
}

// IsInsideSession checks if currently running inside a Zellij session.
// Zellij sets the ZELLIJ environment variable to "0" when inside a session.
func (z *ZellijMultiplexer) IsInsideSession() bool {
	return os.Getenv("ZELLIJ") == "0" && os.Getenv("ZELLIJ_PANE_ID") != ""
}

// GetType returns a string indicating the type of multiplexer.
func (z *ZellijMultiplexer) GetType() string {
	return "zellij"
}

// GetProcessArgs is a helper function, assumed to be available or will be made a private method.
// For now, let's assume it's in utils or similar.
// func GetProcessArgs(pid int) []string { /* ... */ }

// IsSubShell is a helper function, assumed to be available or will be made a private method.
// For now, let's assume it's in utils or similar.
// func IsSubShell(command string) bool { /* ... */ }

// GetShellName is a helper function, assumed to be available or will be made a private method.
// For now, let's assume it's in utils or similar.
// func GetShellName(pid int) string { /* ... */ }
// These are needed for PaneDetails but are hard to get in Zellij for arbitrary panes.
// The current GetPaneDetails implementation only gets basic info for the current pane.
