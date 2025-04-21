package system

import (
	"fmt"
	"strings"

	"github.com/fatih/color"
)

// InfoFormatter provides consistent formatting for TmuxAI information displays
type InfoFormatter struct {
	// Color schemes
	HeaderColor  *color.Color
	LabelColor   *color.Color
	ValueColor   *color.Color
	SuccessColor *color.Color
	WarningColor *color.Color
	ErrorColor   *color.Color
	NeutralColor *color.Color
}

// NewInfoFormatter creates a new formatter with default color schemes
func NewInfoFormatter() *InfoFormatter {
	return &InfoFormatter{
		HeaderColor:  color.New(color.FgHiCyan, color.Bold),
		LabelColor:   color.New(color.FgHiBlue),
		ValueColor:   color.New(color.FgHiWhite),
		SuccessColor: color.New(color.FgHiGreen),
		WarningColor: color.New(color.FgHiYellow),
		ErrorColor:   color.New(color.FgHiRed),
		NeutralColor: color.New(color.FgHiBlack),
	}
}

// FormatSection prints a section header
func (f *InfoFormatter) FormatSection(title string) string {
	return fmt.Sprintf("%s\n%s\n",
		f.HeaderColor.Sprint(title),
		f.NeutralColor.Sprint(strings.Repeat("─", len(title))))
}

// FormatKeyValue prints a key-value pair with consistent formatting
func (f *InfoFormatter) FormatKeyValue(key string, value interface{}) string {
	return fmt.Sprintf("%s %s\n",
		f.LabelColor.Sprintf("%-16s:", key),
		f.ValueColor.Sprint(value))
}

// FormatProgressBar generates a visual indicator for percentage values
func (f *InfoFormatter) FormatProgressBar(percent float64, width int) string {
	if width <= 0 {
		width = 10
	}

	filled := int((percent / 100) * float64(width))
	if filled > width {
		filled = width
	}

	var bar string

	// Choose color based on percentage
	var barColor *color.Color
	switch {
	case percent >= 90:
		barColor = f.ErrorColor
	case percent >= 70:
		barColor = f.WarningColor
	default:
		barColor = f.SuccessColor
	}

	// Generate the filled portion
	if filled > 0 {
		bar += barColor.Sprint(strings.Repeat("█", filled))
	}

	// Generate the empty portion
	if width-filled > 0 {
		bar += f.NeutralColor.Sprint(strings.Repeat("░", width-filled))
	}

	return fmt.Sprintf("%s %s", bar, f.ValueColor.Sprintf("%.1f%%", percent))
}

// FormatBool formats boolean values with color
func (f *InfoFormatter) FormatBool(value bool) string {
	if value {
		return f.SuccessColor.Sprint("yes")
	}
	return f.NeutralColor.Sprint("no")
}
