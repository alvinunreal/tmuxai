package system

import (
	"fmt"
	"os"
	"strings"

	"github.com/alvinunreal/tmuxai/logger"
)

// MultiplexerType represents the type of terminal multiplexer
type MultiplexerType int

const (
	MultiplexerNone MultiplexerType = iota
	MultiplexerTmux
	MultiplexerZellij
)

// String returns the string representation of the multiplexer type
func (m MultiplexerType) String() string {
	switch m {
	case MultiplexerTmux:
		return "tmux"
	case MultiplexerZellij:
		return "zellij"
	default:
		return "none"
	}
}

// Multiplexer defines the interface for terminal multiplexer operations
type Multiplexer interface {
	// Session operations
	GetCurrentSession() (string, error)
	CreateSession() (string, error)
	AttachSession(sessionId string) error

	// Pane operations
	GetPaneDetails(target string) ([]PaneDetails, error)
	CreateNewPane(target string) (string, error)
	CapturePane(paneId string, maxLines int) (string, error)
	ClearPane(paneId string) error

	// Command execution
	SendCommand(paneId string, command string) error
	SendKeys(paneId string, keys string) error

	// Detection and information
	IsAvailable() bool
	GetType() MultiplexerType
	GetCurrentPaneId() (string, error)
}

// NewMultiplexer creates a new multiplexer instance based on environment detection
func NewMultiplexer() (Multiplexer, error) {
	if os.Getenv("ZELLIJ_SESSION_NAME") != "" {
		// TODO: Implement ZellijBackend in phase 1.3
		return nil, fmt.Errorf("zellij backend not yet implemented")
	}
	
	if os.Getenv("TMUX_PANE") != "" {
		return NewTmuxBackend(), nil
	}
	
	return nil, fmt.Errorf("no supported terminal multiplexer detected")
}

// PaneDetails represents the details of a pane in any terminal multiplexer
type PaneDetails struct {
	Id                 string
	CurrentPid         int
	CurrentCommand     string
	CurrentCommandArgs string
	Content            string
	Shell              string
	OS                 string
	LastLine           string
	IsActive           int
	IsTmuxAiPane       bool
	IsTmuxAiExecPane   bool
	IsPrepared         bool
	IsSubShell         bool
	HistorySize        int
	HistoryLimit       int
	MultiplexerType    MultiplexerType
}

func (p *PaneDetails) String() string {
	// ANSI color codes
	reset := "\033[0m"
	green := "\033[32m"
	cyan := "\033[36m"
	yellow := "\033[33m"
	blue := "\033[34m"
	gray := "\033[90m"

	// Format true/false values with colors
	formatBool := func(value bool) string {
		if value {
			return fmt.Sprintf("%strue%s", green, reset)
		}
		return fmt.Sprintf("%sfalse%s", gray, reset)
	}

	// Format the output with colors and clean alignment
	return fmt.Sprintf("Id: %s%s%s\n", cyan, strings.ReplaceAll(p.Id, "%", ""), reset) +
		fmt.Sprintf("Command: %s%s%s\n", yellow, p.CurrentCommand, reset) +
		fmt.Sprintf("Args: %s%s%s\n", gray, p.CurrentCommandArgs, reset) +
		fmt.Sprintf("Shell: %s%s%s\n", blue, p.Shell, reset) +
		fmt.Sprintf("OS: %s%s%s\n", gray, p.OS, reset) +
		fmt.Sprintf("TmuxAI Pane: %s\n", formatBool(p.IsTmuxAiPane)) +
		fmt.Sprintf("TmuxAI Exec Pane: %s\n", formatBool(p.IsTmuxAiExecPane)) +
		fmt.Sprintf("Prepared: %s\n", formatBool(p.IsPrepared)) +
		fmt.Sprintf("Sub Shell: %s\n", formatBool(p.IsSubShell))
}

func (p *PaneDetails) FormatInfo(f *InfoFormatter) string {
	var builder strings.Builder

	cleanId := strings.ReplaceAll(p.Id, "%", "")
	var paneTitle string
	switch {
	case p.IsTmuxAiPane:
		paneTitle = fmt.Sprintf("%s: TmuxAI", cleanId)
	case p.IsTmuxAiExecPane:
		paneTitle = fmt.Sprintf("%s: TmuxAI Exec Pane", cleanId)
	default:
		paneTitle = fmt.Sprintf("%s: Read Only", cleanId)
	}
	builder.WriteString(f.HeaderColor.Sprintf("Pane %s", paneTitle))
	builder.WriteString("\n")

	const labelWidth = 18

	// Helper function for formatted key-value pairs
	formatLine := func(key string, value any) {
		builder.WriteString(f.LabelColor.Sprintf("%-*s", labelWidth, key))
		builder.WriteString("  ")
		builder.WriteString(value.(string))
		builder.WriteString("\n")
	}

	formatLine("Command", p.CurrentCommand)
	// Add command args if present
	if p.CurrentCommandArgs != "" {
		formatLine("Args", p.CurrentCommandArgs)
	}

	// Add shell and OS info on separate lines
	formatLine("Shell", p.Shell)
	formatLine("OS", p.OS)

	// Add status flags each on their own line
	formatLine("TmuxAI", f.FormatBool(p.IsTmuxAiPane))
	formatLine("Exec Pane", f.FormatBool(p.IsTmuxAiExecPane))
	formatLine("Prepared", f.FormatBool(p.IsPrepared))
	formatLine("Sub Shell", f.FormatBool(p.IsSubShell))

	return builder.String()
}

// Refresh method to refresh pane details using any multiplexer
func (p *PaneDetails) Refresh(multiplexer Multiplexer, maxLines int) error {
	content, err := multiplexer.CapturePane(p.Id, maxLines)
	if err != nil {
		logger.Error("Failed to refresh pane %s: %v", p.Id, err)
		return err
	}

	p.Content = content
	lines := strings.Split(p.Content, "\n")
	if len(lines) > 0 {
		p.LastLine = strings.TrimSpace(lines[len(lines)-1])
	}
	p.IsPrepared = strings.HasSuffix(p.LastLine, "»")
	
	if IsShellCommand(p.CurrentCommand) {
		p.Shell = p.CurrentCommand
	}

	return nil
}