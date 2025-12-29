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
var TmuxPanesDetails = func(target string) ([]TmuxPaneDetails, error) {
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
var TmuxCapturePane = func(paneId string, maxLines int) (string, error) {
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

var TmuxCurrentPaneId = func() (string, error) {
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

func TmuxClearPane(paneId string) error {
	paneDetails, err := TmuxPanesDetails(paneId)
	if err != nil {
		logger.Error("Failed to get pane details for %s: %v", paneId, err)
		return err
	}

	if len(paneDetails) == 0 {
		return fmt.Errorf("no pane details found for pane %s", paneId)
	}

	cmd := exec.Command("tmux", "split-window", "-vp", "100", "-t", paneId)
	if err := cmd.Run(); err != nil {
		logger.Error("Failed to split window for pane %s: %v", paneId, err)
		return err
	}

	cmd = exec.Command("tmux", "clear-history", "-t", paneId)
	if err := cmd.Run(); err != nil {
		logger.Error("Failed to clear history for pane %s: %v", paneId, err)
		return err
	}

	cmd = exec.Command("tmux", "kill-pane")
	if err := cmd.Run(); err != nil {
		logger.Error("Failed to kill temporary pane: %v", err)
		return err
	}

	logger.Debug("Successfully cleared pane %s", paneId)
	return nil
}
