package system

import (
	"fmt"
	"strings"
)

type TmuxPaneDetails struct {
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
}

func (p *TmuxPaneDetails) String() string {
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

func (p *TmuxPaneDetails) FormatInfo(f *InfoFormatter) string {
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

func (p *TmuxPaneDetails) Refresh(maxLines int) {
	content, _ := TmuxCapturePane(p.Id, maxLines)
	p.Content = content
	p.LastLine = strings.TrimSpace(strings.Split(p.Content, "\n")[len(strings.Split(p.Content, "\n"))-1])
	p.IsPrepared = strings.HasSuffix(p.LastLine, "»")
	if IsShellCommand(p.CurrentCommand) {
		p.Shell = p.CurrentCommand
	}
}
