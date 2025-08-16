package internal

import (
	"fmt"
	"strings"
	"time"

	"github.com/eiannone/keyboard"
	"github.com/fatih/color"
)

func (m *Manager) Countdown(seconds int) {
	highlightColor := color.New(color.FgYellow, color.Bold).SprintFunc()
	dimColor := color.New(color.FgBlue).SprintFunc()
	pauseColor := color.New(color.FgRed, color.Bold).SprintFunc()

	// Set up keyboard
	if err := keyboard.Open(); err != nil {
		fmt.Println("Error opening keyboard:", err)
		return
	}
	defer func() { _ = keyboard.Close() }()

	// Create a channel for keyboard events
	keyChan := make(chan keyboard.Key, 10)
	go func() {
		for {
			_, key, err := keyboard.GetKey()
			if err != nil {
				return
			}
			keyChan <- key
		}
	}()

	paused := false
	remaining := seconds
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	// Initial render
	renderCountdown(remaining, seconds, paused, highlightColor, dimColor, pauseColor)

	for remaining > 0 {
		select {
		case key := <-keyChan:
			switch key {
			case keyboard.KeySpace: // Space key
				paused = !paused
				renderCountdown(remaining, seconds, paused, highlightColor, dimColor, pauseColor)
			case keyboard.KeyEnter: // Enter key
				// Just continue execution without exiting the function
				remaining = 0 // Set remaining to 0 to end the countdown loop
				renderCountdown(remaining, seconds, paused, highlightColor, dimColor, pauseColor)
			case keyboard.KeyCtrlC: // Ctrl+C
				m.Status = ""
				m.WatchMode = false
				return
			}
		case <-ticker.C:
			if !paused {
				remaining--
				renderCountdown(remaining, seconds, paused, highlightColor, dimColor, pauseColor)
			}
		}
	}
}

// renderCountdown displays the current state of the countdown
func renderCountdown(remaining, total int, paused bool, highlightColor, dimColor, pauseColor func(a ...interface{}) string) {
	// Use ANSI escape sequences for complete control over line clearing
	// \033[0G moves cursor to column 0 (beginning of line)
	// \033[K clears from cursor to end of line
	fmt.Print("\033[0G\033[K")

	// Build the dot display with consistent spacing
	dots := make([]string, total)
	for j := 0; j < total; j++ {
		if j >= total-remaining {
			dots[j] = dimColor("○")
		} else {
			dots[j] = highlightColor("●")
		}
	}

	// Use simple fixed-width characters for status indicators
	var statusIndicator string
	if paused {
		statusIndicator = pauseColor("⏸")
	} else {
		statusIndicator = highlightColor("▶")
	}

	// Ensure exact character count and consistent spacing with printf
	// %2s gives a fixed width for the status indicator
	fmt.Printf("%s %s [Space: Pause/Resume | Enter: To continue]",
		statusIndicator, strings.Join(dots, " "))
}
